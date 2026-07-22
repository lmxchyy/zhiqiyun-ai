package httpserver

import (
	"database/sql"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestIdentityPhase2PricingDowngradeAndConsistency(t *testing.T) {
	databaseURL := os.Getenv("XIANZHI_IDENTITY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("XIANZHI_IDENTITY_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := &postgresStore{db: db, ready: true}
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	superID := "phase2_super_" + suffix
	adminID := "phase2_admin_" + suffix
	seedIdentityChangeUser(t, db, superID, "SUPER_ADMIN")
	seedIdentityChangeUser(t, db, adminID, "ADMIN")
	proof := identityPaymentProof{
		Reference:      "BANK-PHASE2-" + suffix,
		PayerName:      "Phase 2 Tester",
		PaidAt:         time.Now().UTC().Format(time.RFC3339),
		PaymentChannel: "BANK_TRANSFER",
	}

	t.Run("offline order defaults to catalog price and special price requires permission and review", func(t *testing.T) {
		exactUserID := "phase2_exact_" + suffix
		seedIdentityChangeUser(t, db, exactUserID, "MEMBER")
		exact, err := store.PreviewAdminIdentityChange(superID, "SUPER_ADMIN", exactUserID, identityChangePreviewRequest{
			Action: "UPGRADE", Method: identityMethodOfflineOrder, TargetIdentity: "AGENT",
			PlanID: "plan_agent_join_996", PaidAmountCents: 99600, PaymentProof: proof, Reason: "verified exact package payment",
		})
		if err != nil || exact.Status != "READY" || exact.SpecialPrice || exact.OriginalAmountCents != 99600 || exact.PayableAmountCents != 99600 {
			t.Fatalf("exact price preview=%+v err=%v", exact, err)
		}

		blockedUserID := "phase2_discount_blocked_" + suffix
		seedIdentityChangeUser(t, db, blockedUserID, "MEMBER")
		blocked, err := store.PreviewAdminIdentityChange(adminID, "ADMIN", blockedUserID, identityChangePreviewRequest{
			Action: "UPGRADE", Method: identityMethodOfflineOrder, TargetIdentity: "AGENT",
			PlanID: "plan_agent_join_996", PaidAmountCents: 90000, PaymentProof: proof, DiscountReason: "approved campaign exception", Reason: "special price permission test",
		})
		if err != nil || blocked.Status != "BLOCKED" || !blocked.SpecialPrice || len(blocked.Blockers) == 0 {
			t.Fatalf("unauthorized special price preview=%+v err=%v", blocked, err)
		}

		reviewUserID := "phase2_discount_review_" + suffix
		seedIdentityChangeUser(t, db, reviewUserID, "MEMBER")
		review, err := store.PreviewAdminIdentityChange(superID, "SUPER_ADMIN", reviewUserID, identityChangePreviewRequest{
			Action: "UPGRADE", Method: identityMethodOfflineOrder, TargetIdentity: "AGENT",
			PlanID: "plan_agent_join_996", PaidAmountCents: 90000, PaymentProof: proof, DiscountReason: "approved campaign exception", Reason: "special price review test",
		})
		if err != nil || review.Status != "REVIEW_REQUIRED" || !review.ReviewRequired || review.DiscountAmountCents != 9600 {
			t.Fatalf("authorized special price preview=%+v err=%v", review, err)
		}
	})

	t.Run("downgrade strategy has no implicit default", func(t *testing.T) {
		userID := "phase2_downgrade_" + suffix
		seedIdentityChangeUser(t, db, userID, "MEMBER")
		upgradeIdentityForDowngradeTest(t, store, superID, userID, "AGENT")
		_, err := store.PreviewAdminIdentityDowngrade(superID, "SUPER_ADMIN", userID, identityDowngradeRequest{Reason: "explicit strategy is required"})
		if !errors.Is(err, errIdentityDowngradeInvalid) {
			t.Fatalf("empty strategy should be rejected, got %v", err)
		}
	})

	t.Run("read only consistency checker reports legacy split identity", func(t *testing.T) {
		userID := "phase2_legacy_split_" + suffix
		seedIdentityChangeUser(t, db, userID, "AGENT_L1")
		items, err := store.ListAdminIdentityConsistency(superID, "SUPER_ADMIN", identityConsistencyFilter{Code: "ACTIVE_AGENT_WITH_LEGACY_ROLE_ONLY", UserID: userID})
		if err != nil || len(items) != 1 || items[0].UserID != userID {
			t.Fatalf("consistency items=%+v err=%v", items, err)
		}
		if _, err := store.ListAdminIdentityConsistency(adminID, "CUSTOMER_SERVICE", identityConsistencyFilter{UserID: userID}); !errors.Is(err, errIdentityPermission) {
			t.Fatalf("consistency permission should be enforced in store, got %v", err)
		}
	})

	t.Run("operation center profile update cannot change business identity", func(t *testing.T) {
		userID := "phase2_center_profile_" + suffix
		seedIdentityChangeUser(t, db, userID, "MEMBER")
		preview, err := store.PreviewAdminIdentityChange(superID, "SUPER_ADMIN", userID, identityChangePreviewRequest{Action: "UPGRADE", Method: identityMethodOnlyIdentity, TargetIdentity: "OPERATION_CENTER", Reason: "verified operation center qualification"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = store.ConfirmAdminIdentityChange(superID, "SUPER_ADMIN", userID, identityChangeConfirmRequest{PreviewToken: preview.PreviewToken, HighRiskConfirmed: true}); err != nil {
			t.Fatal(err)
		}
		centerID := identityEntityIDForTest(t, db, "xz_operation_centers", userID)
		if _, err = store.UpdateOperationCenterProfile(adminID, "CUSTOMER_SERVICE", centerID, operationCenterProfileUpdate{Name: "Forbidden"}); !errors.Is(err, errIdentityPermission) {
			t.Fatalf("profile permission should be enforced, got %v", err)
		}
		updated, err := store.UpdateOperationCenterProfile(adminID, "ADMIN", centerID, operationCenterProfileUpdate{Name: "华东运营中心", Region: "华东", ResponsiblePerson: "测试负责人", AgreementStatus: "SIGNED"})
		if err != nil || updated.Name != "华东运营中心" || updated.Status != "ACTIVE" {
			t.Fatalf("profile update=%+v err=%v", updated, err)
		}
		var active int
		if err = db.QueryRow(`SELECT count(*) FROM xz_user_business_identities WHERE user_id=$1 AND identity_type='OPERATION_CENTER' AND identity_status='ACTIVE' AND ended_at IS NULL`, userID).Scan(&active); err != nil || active != 1 {
			t.Fatalf("business identity changed active=%d err=%v", active, err)
		}
	})

	t.Run("waiting downgrade can be cancelled and history remains", func(t *testing.T) {
		userID := "phase2_wait_cancel_" + suffix
		seedIdentityChangeUser(t, db, userID, "MEMBER")
		upgradeIdentityForDowngradeTest(t, store, superID, userID, "AGENT")
		agentID := identityEntityIDForTest(t, db, "xz_channel_agents", userID)
		if _, err := db.Exec(`INSERT INTO xz_commission_wallet_accounts(id,tenant_id,beneficiary_type,beneficiary_id,expected_cents) VALUES($1,'tenant_default','AGENT',$2,100)`, "phase2_wait_wallet_"+suffix, agentID); err != nil {
			t.Fatal(err)
		}
		preview, err := store.PreviewAdminIdentityDowngrade(superID, "SUPER_ADMIN", userID, identityDowngradeRequest{ChildStrategy: downgradeKeepHistory, WaitForSettlement: true, Reason: "wait and cancel workflow test"})
		if err != nil || preview.Status != "WAITING" {
			t.Fatalf("waiting preview=%+v err=%v", preview, err)
		}
		created, err := store.ConfirmAdminIdentityDowngrade(superID, "SUPER_ADMIN", userID, identityDowngradeConfirmRequest{PreviewToken: preview.PreviewToken, HighRiskConfirmed: true})
		if err != nil || created.Status != "WAITING" {
			t.Fatalf("waiting request=%+v err=%v", created, err)
		}
		cancelled, err := store.CancelAdminIdentityDowngrade(superID, "SUPER_ADMIN", userID, created.RequestID)
		if err != nil || cancelled.Status != "CANCELLED" {
			t.Fatalf("cancelled request=%+v err=%v", cancelled, err)
		}
		var count int
		if err = db.QueryRow(`SELECT count(*) FROM xz_identity_downgrade_requests WHERE id=$1 AND status='CANCELLED'`, created.RequestID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("cancelled history missing count=%d err=%v", count, err)
		}
	})
}

func TestIdentityConsistencySummary(t *testing.T) {
	summary := identityConsistencySummary([]identityConsistencyIssue{{Severity: "CRITICAL"}, {Severity: "HIGH"}, {Severity: "HIGH"}, {Severity: "MEDIUM"}})
	if summary["total"] != 4 || summary["critical"] != 1 || summary["high"] != 2 || summary["medium"] != 1 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
}
