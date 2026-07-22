package httpserver

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestAdminIdentityChangeHTTPContract(t *testing.T) {
	databaseURL := os.Getenv("XIANZHI_IDENTITY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("XIANZHI_IDENTITY_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	adminID := "identity_http_admin_" + suffix
	userID := "identity_http_user_" + suffix
	seedIdentityChangeUser(t, db, adminID, "SUPER_ADMIN")
	seedIdentityChangeUser(t, db, userID, "MEMBER")
	api := newIdentityChangeAPI(&postgresStore{db: db, ready: true})

	previewRequest := identityChangePreviewRequest{
		Action:         "UPGRADE",
		Method:         identityMethodOnlyIdentity,
		TargetIdentity: "AGENT",
		Reason:         "verify admin frontend contract",
	}
	previewBody, err := json.Marshal(previewRequest)
	if err != nil {
		t.Fatal(err)
	}
	previewHTTP := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/"+userID+"/identity-change/preview", bytes.NewReader(previewBody))
	previewHTTP.SetPathValue("id", userID)
	previewHTTP = previewHTTP.WithContext(context.WithValue(context.WithValue(previewHTTP.Context(), actorIDContextKey, adminID), actorRoleContextKey, "SUPER_ADMIN"))
	previewRecorder := httptest.NewRecorder()
	api.preview(previewRecorder, previewHTTP)
	if previewRecorder.Code != http.StatusOK {
		t.Fatalf("preview returned %d: %s", previewRecorder.Code, previewRecorder.Body.String())
	}
	var previewResponse struct {
		Item identityChangePreviewResult `json:"item"`
	}
	if err := json.Unmarshal(previewRecorder.Body.Bytes(), &previewResponse); err != nil {
		t.Fatal(err)
	}
	if previewResponse.Item.PreviewToken == "" || previewResponse.Item.EffectiveAt != "ON_CONFIRMATION" || previewResponse.Item.Status != "READY" {
		t.Fatalf("unexpected preview response contract: %+v", previewResponse.Item)
	}

	confirmBody, err := json.Marshal(identityChangeConfirmRequest{PreviewToken: previewResponse.Item.PreviewToken})
	if err != nil {
		t.Fatal(err)
	}
	confirmHTTP := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/"+userID+"/identity-change/confirm", bytes.NewReader(confirmBody))
	confirmHTTP.SetPathValue("id", userID)
	confirmHTTP = confirmHTTP.WithContext(context.WithValue(context.WithValue(confirmHTTP.Context(), actorIDContextKey, adminID), actorRoleContextKey, "SUPER_ADMIN"))
	confirmRecorder := httptest.NewRecorder()
	api.confirm(confirmRecorder, confirmHTTP)
	if confirmRecorder.Code != http.StatusOK {
		t.Fatalf("confirm returned %d: %s", confirmRecorder.Code, confirmRecorder.Body.String())
	}
	var confirmResponse struct {
		Item identityChangeConfirmResult `json:"item"`
	}
	if err := json.Unmarshal(confirmRecorder.Body.Bytes(), &confirmResponse); err != nil {
		t.Fatal(err)
	}
	if confirmResponse.Item.Status != "SUCCEEDED" || confirmResponse.Item.UserID != userID || confirmResponse.Item.ExecutionID == "" {
		t.Fatalf("unexpected confirm response contract: %+v", confirmResponse.Item)
	}
}

func TestAdminIdentityChangeNormalAbnormalDuplicateConcurrentAndPermission(t *testing.T) {
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
	admin1 := "identity_admin_1_" + suffix
	admin2 := "identity_admin_2_" + suffix
	for _, item := range []struct{ id, role string }{{admin1, "SUPER_ADMIN"}, {admin2, "SUPER_ADMIN"}} {
		seedIdentityChangeUser(t, db, item.id, item.role)
	}

	t.Run("normal only identity and duplicate confirm", func(t *testing.T) {
		userID := "identity_only_" + suffix
		seedIdentityChangeUser(t, db, userID, "MEMBER")
		preview, err := store.PreviewAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangePreviewRequest{Action: "UPGRADE", Method: identityMethodOnlyIdentity, TargetIdentity: "AGENT", Reason: "manual qualification verified"})
		if err != nil || preview.Status != "READY" || preview.PaidAmountCents != 0 || preview.TokenDelta != 0 || preview.CommissionGenerated {
			t.Fatalf("unexpected preview: %+v err=%v", preview, err)
		}
		confirmed, err := store.ConfirmAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangeConfirmRequest{PreviewToken: preview.PreviewToken})
		if err != nil || confirmed.Status != "SUCCEEDED" || confirmed.OrderID != "" {
			t.Fatalf("unexpected confirmation: %+v err=%v", confirmed, err)
		}
		repeated, err := store.ConfirmAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangeConfirmRequest{PreviewToken: preview.PreviewToken})
		if err != nil || !repeated.Idempotent || repeated.ExecutionID != confirmed.ExecutionID {
			t.Fatalf("duplicate confirmation was not idempotent: %+v err=%v", repeated, err)
		}
		assertIdentityChangeArtifacts(t, db, userID, confirmed.ExecutionID, 0, 0, 0)
	})

	t.Run("abnormal request and blocked offline proof", func(t *testing.T) {
		userID := "identity_invalid_" + suffix
		seedIdentityChangeUser(t, db, userID, "MEMBER")
		if _, err := store.PreviewAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangePreviewRequest{Action: "UPGRADE", Method: identityMethodOnlyIdentity, TargetIdentity: "AGENT"}); !errors.Is(err, errIdentityChangeInvalid) {
			t.Fatalf("expected invalid reason error, got %v", err)
		}
		preview, err := store.PreviewAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangePreviewRequest{Action: "UPGRADE", Method: identityMethodOfflineOrder, TargetIdentity: "AGENT", PlanID: "plan_agent_join_996", PaidAmountCents: 99600, Reason: "offline payment"})
		if err != nil || preview.Status != "BLOCKED" || len(preview.Blockers) == 0 {
			t.Fatalf("missing payment proof should block preview: %+v err=%v", preview, err)
		}
		if _, err := store.ConfirmAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangeConfirmRequest{PreviewToken: preview.PreviewToken, HighRiskConfirmed: true}); !errors.Is(err, errIdentityChangeBlocked) {
			t.Fatalf("blocked preview was confirmed: %v", err)
		}
	})

	t.Run("operation center requires super administrator", func(t *testing.T) {
		userID := "identity_operation_permission_" + suffix
		seedIdentityChangeUser(t, db, userID, "MEMBER")
		_, err := store.PreviewAdminIdentityChange(admin1, "ADMIN", userID, identityChangePreviewRequest{Action: "UPGRADE", Method: identityMethodOnlyIdentity, TargetIdentity: "OPERATION_CENTER", Reason: "operation qualification"})
		if !errors.Is(err, errIdentityPermission) {
			t.Fatalf("expected operation center permission error, got %v", err)
		}
		_, err = store.PreviewAdminIdentityChange(admin1, "MEMBER", userID, identityChangePreviewRequest{Action: "UPGRADE", Method: identityMethodOnlyIdentity, TargetIdentity: "AGENT", Reason: "unauthorized attempt"})
		if !errors.Is(err, errIdentityPermission) {
			t.Fatalf("expected non-admin permission error, got %v", err)
		}
	})

	t.Run("concurrent confirmation creates one execution", func(t *testing.T) {
		userID := "identity_concurrent_" + suffix
		seedIdentityChangeUser(t, db, userID, "MEMBER")
		preview, err := store.PreviewAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangePreviewRequest{Action: "UPGRADE", Method: identityMethodOnlyIdentity, TargetIdentity: "AGENT", Reason: "concurrency test"})
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		results := make(chan identityChangeConfirmResult, 2)
		errorsCh := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				item, confirmErr := store.ConfirmAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangeConfirmRequest{PreviewToken: preview.PreviewToken})
				results <- item
				errorsCh <- confirmErr
			}()
		}
		wg.Wait()
		close(results)
		close(errorsCh)
		for confirmErr := range errorsCh {
			if confirmErr != nil {
				t.Fatalf("concurrent confirmation failed: %v", confirmErr)
			}
		}
		var executionID string
		for item := range results {
			if executionID == "" {
				executionID = item.ExecutionID
			} else if executionID != item.ExecutionID {
				t.Fatalf("concurrent confirmations created different executions: %s %s", executionID, item.ExecutionID)
			}
		}
		var count int
		if err := db.QueryRow(`SELECT count(*) FROM xz_identity_change_executions WHERE user_id=$1`, userID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("expected one execution, count=%d err=%v", count, err)
		}
		if err := db.QueryRow(`SELECT count(*) FROM xz_user_business_identities WHERE user_id=$1 AND identity_type='AGENT' AND identity_status='ACTIVE' AND ended_at IS NULL`, userID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("expected one active identity, count=%d err=%v", count, err)
		}
		if err := db.QueryRow(`SELECT count(*) FROM xz_user_roles WHERE user_id=$1 AND role='AGENT' AND upper(status)='ACTIVE'`, userID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("expected one active agent role, count=%d err=%v", count, err)
		}
	})

	t.Run("late failure rolls back identity profile relationship and RBAC", func(t *testing.T) {
		userID := "identity_rollback_" + suffix
		seedIdentityChangeUser(t, db, userID, "MEMBER")
		functionName := "reject_identity_change_" + suffix
		triggerName := "reject_identity_change_trigger_" + suffix
		if _, err := db.Exec(fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger AS $$ BEGIN IF NEW.user_id='%s' THEN RAISE EXCEPTION 'forced identity rollback'; END IF; RETURN NEW; END; $$ LANGUAGE plpgsql`, functionName, userID)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(fmt.Sprintf(`CREATE TRIGGER %s BEFORE INSERT ON xz_identity_change_records FOR EACH ROW EXECUTE FUNCTION %s()`, triggerName, functionName)); err != nil {
			t.Fatal(err)
		}
		defer func() {
			_, _ = db.Exec(fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON xz_identity_change_records`, triggerName))
			_, _ = db.Exec(fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, functionName))
		}()
		preview, err := store.PreviewAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangePreviewRequest{Action: "UPGRADE", Method: identityMethodOnlyIdentity, TargetIdentity: "AGENT", Reason: "transaction rollback injection"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.ConfirmAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangeConfirmRequest{PreviewToken: preview.PreviewToken}); err == nil {
			t.Fatal("expected injected late transaction failure")
		}
		var identityCount, profileCount, relationCount, roleCount int
		_ = db.QueryRow(`SELECT count(*) FROM xz_user_business_identities WHERE user_id=$1`, userID).Scan(&identityCount)
		_ = db.QueryRow(`SELECT count(*) FROM xz_channel_agents WHERE user_id=$1`, userID).Scan(&profileCount)
		_ = db.QueryRow(`SELECT count(*) FROM xz_user_relationships WHERE user_id=$1`, userID).Scan(&relationCount)
		_ = db.QueryRow(`SELECT count(*) FROM xz_user_roles WHERE user_id=$1 AND role='AGENT'`, userID).Scan(&roleCount)
		if identityCount != 0 || profileCount != 0 || relationCount != 0 || roleCount != 0 {
			t.Fatalf("partial transaction escaped rollback identity=%d profile=%d relationship=%d role=%d", identityCount, profileCount, relationCount, roleCount)
		}
	})

	t.Run("special grant creates independent token ledger", func(t *testing.T) {
		userID := "identity_special_" + suffix
		seedIdentityChangeUser(t, db, userID, "MEMBER")
		preview, err := store.PreviewAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangePreviewRequest{Action: "UPGRADE", Method: identityMethodSpecialGrant, TargetIdentity: "AGENT", GiftTokenAmount: 50, Reason: "approved launch support"})
		if err != nil || preview.Status != "READY" || preview.TokenDelta != 50 || preview.PaidAmountCents != 0 || preview.CommissionGenerated || !preview.HighRisk {
			t.Fatalf("unexpected special grant preview: %+v err=%v", preview, err)
		}
		confirmed, err := store.ConfirmAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangeConfirmRequest{PreviewToken: preview.PreviewToken, HighRiskConfirmed: true})
		if err != nil {
			t.Fatal(err)
		}
		assertIdentityChangeArtifacts(t, db, userID, confirmed.ExecutionID, 0, 1, 0)
		var independent bool
		if err := db.QueryRow(`SELECT coalesce((raw->>'independentGift')::boolean,false) FROM xz_token_records WHERE idempotency_key=$1`, "identity-token:"+confirmed.ExecutionID).Scan(&independent); err != nil || !independent {
			t.Fatalf("special gift ledger is not independent: %v %v", independent, err)
		}
	})

	t.Run("offline order persists payment token and commission", func(t *testing.T) {
		userID := "identity_offline_" + suffix
		seedIdentityChangeUser(t, db, userID, "MEMBER")
		preview, err := store.PreviewAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangePreviewRequest{Action: "UPGRADE", Method: identityMethodOfflineOrder, TargetIdentity: "AGENT", PlanID: "plan_agent_join_996", PaidAmountCents: 99600, GrantPackageToken: true, PaymentProof: identityPaymentProof{Reference: "BANK-TEST-001"}, Reason: "offline bank transfer verified"})
		if err != nil || preview.Status != "READY" || preview.TokenDelta <= 0 || !preview.CommissionGenerated || !preview.PaymentRequired {
			t.Fatalf("unexpected offline preview: %+v err=%v", preview, err)
		}
		if _, err := store.ConfirmAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangeConfirmRequest{PreviewToken: preview.PreviewToken}); !errors.Is(err, errIdentityHighRiskConfirm) {
			t.Fatalf("offline order should require second confirmation, got %v", err)
		}
		confirmed, err := store.ConfirmAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangeConfirmRequest{PreviewToken: preview.PreviewToken, HighRiskConfirmed: true})
		if err != nil {
			t.Fatal(err)
		}
		assertIdentityChangeArtifacts(t, db, userID, confirmed.ExecutionID, 1, 1, len(preview.EstimatedCommissions))
		var paymentCount int
		if err := db.QueryRow(`SELECT count(*) FROM xz_payment_records WHERE order_id=$1 AND provider='OFFLINE' AND payment_status='SUCCEEDED'`, confirmed.OrderID).Scan(&paymentCount); err != nil || paymentCount != 1 {
			t.Fatalf("offline payment record mismatch count=%d err=%v", paymentCount, err)
		}
	})

	t.Run("package conversion requires independent review and no duplicate collection", func(t *testing.T) {
		userID := "identity_conversion_" + suffix
		seedIdentityChangeUser(t, db, userID, "MEMBER")
		sourceOrderID := "member_source_" + suffix
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := db.Exec(`INSERT INTO xz_orders(id,order_no,tenant_id,user_id,plan_id,amount_cents,token_grant_amount,token_amount,status,paid_at,created_at,price_snapshot,raw) VALUES($1,$2,'tenant_default',$3,'plan_ai_creator_996',99600,40000,40000,'PAID',$4,$4,'{}','{}')`, sourceOrderID, "MEMBER-"+suffix, userID, now); err != nil {
			t.Fatal(err)
		}
		preview, err := store.PreviewAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangePreviewRequest{Action: "UPGRADE", Method: identityMethodPackageConversion, TargetIdentity: "AGENT", PlanID: "plan_agent_join_996", ConversionTokenPolicy: "KEEP_EXISTING", Reason: "member conversion approved"})
		if err != nil || preview.Status != "REVIEW_REQUIRED" || preview.PaidAmountCents != 0 || preview.TokenDelta != 0 || preview.SourceMembershipOrderID != sourceOrderID {
			t.Fatalf("unexpected conversion preview: %+v err=%v", preview, err)
		}
		if _, err := store.ConfirmAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangeConfirmRequest{PreviewToken: preview.PreviewToken}); !errors.Is(err, errIdentityReviewRequired) {
			t.Fatalf("unreviewed conversion was not blocked: %v", err)
		}
		if _, err := store.ReviewAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangeReviewRequest{PreviewToken: preview.PreviewToken, Decision: "APPROVED", Reason: "self review"}); !errors.Is(err, errIdentityPermission) {
			t.Fatalf("self review should be denied: %v", err)
		}
		if _, err := store.ReviewAdminIdentityChange(admin2, "SUPER_ADMIN", userID, identityChangeReviewRequest{PreviewToken: preview.PreviewToken, Decision: "APPROVED", Reason: "documents checked"}); err != nil {
			t.Fatal(err)
		}
		confirmed, err := store.ConfirmAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangeConfirmRequest{PreviewToken: preview.PreviewToken, HighRiskConfirmed: true})
		if err != nil {
			t.Fatal(err)
		}
		assertIdentityChangeArtifacts(t, db, userID, confirmed.ExecutionID, 0, 0, 0)
		second, err := store.PreviewAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangePreviewRequest{Action: "UPGRADE", Method: identityMethodPackageConversion, TargetIdentity: "AGENT", PlanID: "plan_agent_join_996", ConversionTokenPolicy: "KEEP_EXISTING", Reason: "duplicate conversion"})
		if err != nil || second.Status != "BLOCKED" {
			t.Fatalf("duplicate conversion should be blocked: %+v err=%v", second, err)
		}
	})

	t.Run("agent upgrade to operation center terminates agent identity", func(t *testing.T) {
		userID := "identity_operation_" + suffix
		seedIdentityChangeUser(t, db, userID, "MEMBER")
		agentPreview, err := store.PreviewAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangePreviewRequest{Action: "UPGRADE", Method: identityMethodOnlyIdentity, TargetIdentity: "AGENT", Reason: "agent qualification"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.ConfirmAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangeConfirmRequest{PreviewToken: agentPreview.PreviewToken}); err != nil {
			t.Fatal(err)
		}
		operationPreview, err := store.PreviewAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangePreviewRequest{Action: "UPGRADE", Method: identityMethodOnlyIdentity, TargetIdentity: "OPERATION_CENTER", Reason: "operation center qualification"})
		if err != nil || !operationPreview.HighRisk {
			t.Fatalf("unexpected operation preview: %+v err=%v", operationPreview, err)
		}
		if _, err := store.ConfirmAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangeConfirmRequest{PreviewToken: operationPreview.PreviewToken, HighRiskConfirmed: true}); err != nil {
			t.Fatal(err)
		}
		var agentTerminated, operationActive int
		if err := db.QueryRow(`SELECT count(*) FILTER(WHERE identity_type='AGENT' AND identity_status='TERMINATED'),count(*) FILTER(WHERE identity_type='OPERATION_CENTER' AND identity_status='ACTIVE') FROM xz_user_business_identities WHERE user_id=$1`, userID).Scan(&agentTerminated, &operationActive); err != nil || agentTerminated != 1 || operationActive != 1 {
			t.Fatalf("operation upgrade identity state mismatch agent=%d operation=%d err=%v", agentTerminated, operationActive, err)
		}
	})

	t.Run("freeze restore and direct terminate is blocked", func(t *testing.T) {
		userID := "identity_lifecycle_" + suffix
		seedIdentityChangeUser(t, db, userID, "MEMBER")
		upgrade, err := store.PreviewAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangePreviewRequest{Action: "UPGRADE", Method: identityMethodOnlyIdentity, TargetIdentity: "AGENT", Reason: "initial qualification"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.ConfirmAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangeConfirmRequest{PreviewToken: upgrade.PreviewToken}); err != nil {
			t.Fatal(err)
		}
		freeze, err := store.PreviewAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangePreviewRequest{Action: "FREEZE", Method: identityMethodOnlyIdentity, Reason: "risk investigation"})
		if err != nil || !freeze.HighRisk {
			t.Fatalf("unexpected freeze preview: %+v err=%v", freeze, err)
		}
		if _, err := store.ConfirmAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangeConfirmRequest{PreviewToken: freeze.PreviewToken, HighRiskConfirmed: true}); err != nil {
			t.Fatal(err)
		}
		var commissionEnabled bool
		if err := db.QueryRow(`SELECT commission_enabled FROM xz_user_business_identities WHERE user_id=$1 AND identity_type='AGENT' AND ended_at IS NULL`, userID).Scan(&commissionEnabled); err != nil || commissionEnabled {
			t.Fatalf("frozen identity must not be commission eligible: enabled=%v err=%v", commissionEnabled, err)
		}
		restore, err := store.PreviewAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangePreviewRequest{Action: "RESTORE", Method: identityMethodOnlyIdentity, Reason: "risk cleared"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.ConfirmAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangeConfirmRequest{PreviewToken: restore.PreviewToken}); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(`SELECT commission_enabled FROM xz_user_business_identities WHERE user_id=$1 AND identity_type='AGENT' AND ended_at IS NULL`, userID).Scan(&commissionEnabled); err != nil || !commissionEnabled {
			t.Fatalf("restored identity must regain commission eligibility: enabled=%v err=%v", commissionEnabled, err)
		}
		terminate, err := store.PreviewAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangePreviewRequest{Action: "TERMINATE", Method: identityMethodOnlyIdentity, Reason: "contract ended"})
		if err != nil || !terminate.HighRisk || terminate.Status != "BLOCKED" || len(terminate.Blockers) == 0 {
			t.Fatalf("unexpected terminate preview: %+v err=%v", terminate, err)
		}
		if _, err := store.ConfirmAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangeConfirmRequest{PreviewToken: terminate.PreviewToken, HighRiskConfirmed: true}); !errors.Is(err, errIdentityChangeBlocked) {
			t.Fatalf("direct terminate should be blocked: %v", err)
		}
		var status string
		var ended bool
		if err := db.QueryRow(`SELECT identity_status,ended_at IS NOT NULL FROM xz_user_business_identities WHERE user_id=$1 AND identity_type='AGENT' ORDER BY identity_version DESC LIMIT 1`, userID).Scan(&status, &ended); err != nil || status != "ACTIVE" || ended {
			t.Fatalf("lifecycle final state mismatch status=%s ended=%v err=%v", status, ended, err)
		}
		var changeCount int
		if err := db.QueryRow(`SELECT count(*) FROM xz_identity_change_records WHERE user_id=$1`, userID).Scan(&changeCount); err != nil || changeCount != 3 {
			t.Fatalf("blocked terminate must not create a record, count=%d err=%v", changeCount, err)
		}
	})

	t.Run("relationship history and cycle blocker", func(t *testing.T) {
		userA := "identity_relation_a_" + suffix
		userB := "identity_relation_b_" + suffix
		seedIdentityChangeUser(t, db, userA, "MEMBER")
		seedIdentityChangeUser(t, db, userB, "MEMBER")
		for _, userID := range []string{userA, userB} {
			preview, err := store.PreviewAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangePreviewRequest{Action: "UPGRADE", Method: identityMethodOnlyIdentity, TargetIdentity: "AGENT", Reason: "relationship test agent"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.ConfirmAdminIdentityChange(admin1, "SUPER_ADMIN", userID, identityChangeConfirmRequest{PreviewToken: preview.PreviewToken}); err != nil {
				t.Fatal(err)
			}
		}
		var agentA, agentB string
		if err := db.QueryRow(`SELECT id FROM xz_channel_agents WHERE user_id=$1`, userA).Scan(&agentA); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(`SELECT id FROM xz_channel_agents WHERE user_id=$1`, userB).Scan(&agentB); err != nil {
			t.Fatal(err)
		}
		adjustA, err := store.PreviewAdminIdentityChange(admin1, "SUPER_ADMIN", userA, identityChangePreviewRequest{Action: "ADJUST_PARENT_AGENT", Method: identityMethodOnlyIdentity, ParentAgentID: agentB, Reason: "reassign agent hierarchy"})
		if err != nil || adjustA.Status != "READY" || !adjustA.HighRisk {
			t.Fatalf("unexpected relationship preview: %+v err=%v", adjustA, err)
		}
		if _, err := store.ConfirmAdminIdentityChange(admin1, "SUPER_ADMIN", userA, identityChangeConfirmRequest{PreviewToken: adjustA.PreviewToken, HighRiskConfirmed: true}); err != nil {
			t.Fatal(err)
		}
		cycle, err := store.PreviewAdminIdentityChange(admin1, "SUPER_ADMIN", userB, identityChangePreviewRequest{Action: "ADJUST_PARENT_AGENT", Method: identityMethodOnlyIdentity, ParentAgentID: agentA, Reason: "cycle attempt"})
		if err != nil || cycle.Status != "BLOCKED" || len(cycle.Blockers) == 0 {
			t.Fatalf("cycle relationship should be blocked: %+v err=%v", cycle, err)
		}
		var active, ended int
		if err := db.QueryRow(`SELECT count(*) FILTER(WHERE status='ACTIVE' AND ended_at IS NULL),count(*) FILTER(WHERE status='ENDED' AND ended_at IS NOT NULL) FROM xz_user_relationships WHERE user_id=$1`, userA).Scan(&active, &ended); err != nil || active != 1 {
			t.Fatalf("relationship history mismatch active=%d ended=%d err=%v", active, ended, err)
		}
	})
}

func TestCommercialIdentityAccessAndRBACLifecycle(t *testing.T) {
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
	adminID, userID := "rbac_security_admin_"+suffix, "rbac_security_user_"+suffix
	seedIdentityChangeUser(t, db, adminID, "SUPER_ADMIN")
	seedIdentityChangeUser(t, db, userID, "MEMBER")

	confirmChange := func(request identityChangePreviewRequest) {
		t.Helper()
		preview, err := store.PreviewAdminIdentityChange(adminID, "SUPER_ADMIN", userID, request)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.ConfirmAdminIdentityChange(adminID, "SUPER_ADMIN", userID, identityChangeConfirmRequest{PreviewToken: preview.PreviewToken, HighRiskConfirmed: preview.HighRisk}); err != nil {
			t.Fatal(err)
		}
	}
	roleStatus := func(role string) string {
		t.Helper()
		var status string
		if err := db.QueryRow(`SELECT upper(status) FROM xz_user_roles WHERE user_id=$1 AND role=$2`, userID, role).Scan(&status); err != nil {
			t.Fatal(err)
		}
		return status
	}

	confirmChange(identityChangePreviewRequest{Action: "UPGRADE", Method: identityMethodOnlyIdentity, TargetIdentity: "AGENT", Reason: "verified member agent qualification"})
	var legacyRole string
	if err := db.QueryRow(`SELECT role FROM xz_users WHERE id=$1`, userID).Scan(&legacyRole); err != nil || legacyRole != "MEMBER" {
		t.Fatalf("legacy role changed=%q err=%v", legacyRole, err)
	}
	if principal, found, err := store.GetChannelWorkbenchAgentForUser(userID); err != nil || !found || principal.UserID != userID {
		t.Fatalf("active agent cannot access workbench principal=%+v found=%v err=%v", principal, found, err)
	}
	sessions := newLocalAuthSessions()
	if err := sessions.Put(t.Context(), "rbac-security-token", userID, time.Minute); err != nil {
		t.Fatal(err)
	}
	requestWorkbench := func() *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/api/v1/channel/customers", nil)
		request.Header.Set("Authorization", "Bearer rbac-security-token")
		recorder := httptest.NewRecorder()
		newChannelAPI(store, sessions).customers(recorder, request)
		return recorder
	}
	if recorder := requestWorkbench(); recorder.Code != http.StatusOK {
		t.Fatalf("member upgraded to agent cannot access endpoint: %d %s", recorder.Code, recorder.Body.String())
	}
	if roleStatus("AGENT") != "ACTIVE" {
		t.Fatal("AGENT RBAC role was not activated")
	}
	if _, err := db.Exec(`UPDATE xz_user_role_context SET current_role_code='AGENT',context_type='AGENT' WHERE user_id=$1`, userID); err != nil {
		t.Fatal(err)
	}

	confirmChange(identityChangePreviewRequest{Action: "FREEZE", Method: identityMethodOnlyIdentity, Reason: "security investigation freeze"})
	if roleStatus("AGENT") == "ACTIVE" {
		t.Fatal("frozen agent retained active RBAC role")
	}
	var currentRole string
	if err := db.QueryRow(`SELECT current_role_code FROM xz_user_role_context WHERE user_id=$1`, userID).Scan(&currentRole); err != nil || currentRole != "USER" {
		t.Fatalf("frozen current role=%q err=%v", currentRole, err)
	}
	if _, found, err := store.GetChannelWorkbenchAgentForUser(userID); err != nil || found {
		t.Fatalf("frozen agent retained workbench access found=%v err=%v", found, err)
	}
	if recorder := requestWorkbench(); recorder.Code != http.StatusForbidden {
		t.Fatalf("frozen agent endpoint status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	confirmChange(identityChangePreviewRequest{Action: "RESTORE", Method: identityMethodOnlyIdentity, Reason: "security investigation cleared"})
	if roleStatus("AGENT") != "ACTIVE" {
		t.Fatal("restored agent role was not reactivated")
	}
	confirmChange(identityChangePreviewRequest{Action: "UPGRADE", Method: identityMethodOnlyIdentity, TargetIdentity: "OPERATION_CENTER", Reason: "operation center qualification approved"})
	if roleStatus("AGENT") == "ACTIVE" || roleStatus("OPERATION") != "ACTIVE" {
		t.Fatalf("operation upgrade roles agent=%s operation=%s", roleStatus("AGENT"), roleStatus("OPERATION"))
	}
	if principal, found, err := store.GetChannelWorkbenchAgentForUser(userID); err != nil || !found || principal.OperationCenterID == "" {
		t.Fatalf("operation center inheritance failed principal=%+v found=%v err=%v", principal, found, err)
	}
	if recorder := requestWorkbench(); recorder.Code != http.StatusOK {
		t.Fatalf("operation center inherited workbench status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	preview, err := store.PreviewAdminIdentityDowngrade(adminID, "SUPER_ADMIN", userID, identityDowngradeRequest{TargetIdentity: "AGENT", ChildStrategy: downgradeKeepHistory, Reason: "controlled operation center downgrade"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConfirmAdminIdentityDowngrade(adminID, "SUPER_ADMIN", userID, identityDowngradeConfirmRequest{PreviewToken: preview.PreviewToken, HighRiskConfirmed: true}); err != nil {
		t.Fatal(err)
	}
	if roleStatus("AGENT") != "ACTIVE" || roleStatus("OPERATION") == "ACTIVE" {
		t.Fatalf("agent downgrade roles agent=%s operation=%s", roleStatus("AGENT"), roleStatus("OPERATION"))
	}

	preview, err = store.PreviewAdminIdentityDowngrade(adminID, "SUPER_ADMIN", userID, identityDowngradeRequest{ChildStrategy: downgradeKeepHistory, Reason: "complete commercial exit"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConfirmAdminIdentityDowngrade(adminID, "SUPER_ADMIN", userID, identityDowngradeConfirmRequest{PreviewToken: preview.PreviewToken, HighRiskConfirmed: true}); err != nil {
		t.Fatal(err)
	}
	if roleStatus("AGENT") == "ACTIVE" || roleStatus("OPERATION") == "ACTIVE" || roleStatus("USER") != "ACTIVE" {
		t.Fatalf("full downgrade roles user=%s agent=%s operation=%s", roleStatus("USER"), roleStatus("AGENT"), roleStatus("OPERATION"))
	}
}

func TestIdentityChangePermissionMapping(t *testing.T) {
	for path, want := range map[string]string{
		"/api/v1/admin/users/u1/identity-change/preview": "identity:change:preview",
		"/api/v1/admin/users/u1/identity-change/review":  "identity:change:review",
		"/api/v1/admin/users/u1/identity-change/confirm": "identity:change:confirm",
	} {
		req, _ := http.NewRequest(http.MethodPost, path, nil)
		if got := adminPermissionForRequest(req); got != want {
			t.Fatalf("permission for %s = %s, want %s", path, got, want)
		}
	}
}

func seedIdentityChangeUser(t *testing.T, db *sql.DB, userID, role string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	raw := `{"id":"` + userID + `","email":"` + userID + `@example.test","name":"Identity Test","role":"` + role + `","status":"ACTIVE","memberLevel":"FREE","agentStatus":"NONE","operationCenterStatus":"NONE","createdAt":"` + now + `","updatedAt":"` + now + `"}`
	if _, err := db.Exec(`INSERT INTO xz_users(id,email,name,role,status,member_level,agent_status,operation_center_status,created_at,updated_at,raw) VALUES($1,$2,'Identity Test',$3,'ACTIVE','FREE','NONE','NONE',$4,$4,$5::jsonb)`, userID, userID+"@example.test", role, now, raw); err != nil {
		t.Fatal(err)
	}
}

func assertIdentityChangeArtifacts(t *testing.T, db *sql.DB, userID, executionID string, orders, tokens, commissions int) {
	t.Helper()
	var identityCount, changeCount, auditCount int
	if err := db.QueryRow(`SELECT count(*) FROM xz_user_business_identities WHERE user_id=$1 AND identity_type='AGENT' AND identity_status='ACTIVE'`, userID).Scan(&identityCount); err != nil || identityCount != 1 {
		t.Fatalf("identity artifact mismatch count=%d err=%v", identityCount, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM xz_identity_change_records WHERE user_id=$1 AND request_id=$2`, userID, executionID).Scan(&changeCount); err != nil || changeCount != 1 {
		t.Fatalf("change record mismatch count=%d err=%v", changeCount, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM xz_audit_logs WHERE resource='user_identity' AND resource_id=$1 AND action='admin.identity_change.confirm'`, userID).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("audit record mismatch count=%d err=%v", auditCount, err)
	}
	var orderCount, tokenCount, commissionCount int
	_ = db.QueryRow(`SELECT count(*) FROM xz_orders WHERE idempotency_key IN (SELECT preview_id FROM xz_identity_change_executions WHERE user_id=$1)`, userID).Scan(&orderCount)
	_ = db.QueryRow(`SELECT count(*) FROM xz_token_records WHERE idempotency_key=$1`, "identity-token:"+executionID).Scan(&tokenCount)
	_ = db.QueryRow(`SELECT count(*) FROM xz_commission_records WHERE source_user_id=$1 AND order_id IN (SELECT order_id FROM xz_identity_change_executions WHERE user_id=$1)`, userID).Scan(&commissionCount)
	if orderCount != orders || tokenCount != tokens || commissionCount != commissions {
		t.Fatalf("unexpected financial artifacts orders=%d tokens=%d commissions=%d", orderCount, tokenCount, commissionCount)
	}
}
