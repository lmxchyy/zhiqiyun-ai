package httpserver

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPostgresAdminCorrectionAuditIsAtomicAndIdempotent(t *testing.T) {
	db, store, ctx := openPersonalPointFixRound1Postgres(t)
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	accountID := "admin-correction-" + suffix
	userID := "user-" + accountID
	audit := PersonalPointAudit{ActorID: "admin-" + suffix, ActorRole: "SUPER_ADMIN", Action: "personal_points.admin_correction", Method: "POST", Path: "/api/v1/admin/customers/" + userID + "/point-corrections", RequestID: "request-" + suffix}
	command := PersonalPointCorrectionCommand{AccountID: accountID, UserID: userID, Points: 8, Reason: "repair test balance", IdempotencyKey: "correction-" + suffix, Audit: audit}
	result, err := store.correct(ctx, command)
	if err != nil || result.Idempotent || result.Lot == nil || result.Lot.SourceType != PointSourceAdminCorrection || !result.Lot.ExpiresAt.IsZero() {
		t.Fatalf("first correction=%+v err=%v", result, err)
	}
	replay, err := store.correct(ctx, command)
	if err != nil || !replay.Idempotent || replay.Balance.Available != 8 {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	var audits, wallets, lots int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_audit_logs WHERE resource_id=$1 AND action=$2`, accountID, audit.Action).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_wallet_ledger WHERE account_id=$1 AND idempotency_key=$2`, accountID, personalWalletKey(accountID, "correction", command.IdempotencyKey)).Scan(&wallets); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_personal_point_lots WHERE account_id=$1`, accountID).Scan(&lots); err != nil {
		t.Fatal(err)
	}
	if audits != 1 || wallets != 1 || lots != 1 {
		t.Fatalf("idempotent rows audit/wallet/lot=%d/%d/%d", audits, wallets, lots)
	}

	failAccount := "admin-correction-fail-" + suffix
	failUser := "user-" + failAccount
	functionName := "xz_test_fail_point_audit_" + suffix
	triggerName := "trg_xz_test_fail_point_audit_" + suffix
	cleanup := func() {
		_, _ = db.ExecContext(ctx, fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON xz_audit_logs`, triggerName))
		_, _ = db.ExecContext(ctx, fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, functionName))
	}
	cleanup()
	t.Cleanup(cleanup)
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
			IF NEW.resource_id = %s THEN RAISE EXCEPTION 'forced personal point audit failure'; END IF;
			RETURN NEW;
		END $$;
		CREATE TRIGGER %s BEFORE INSERT ON xz_audit_logs FOR EACH ROW EXECUTE FUNCTION %s();
	`, functionName, quoteSQLLiteral(failAccount), triggerName, functionName)); err != nil {
		t.Fatal(err)
	}
	_, err = store.correct(ctx, PersonalPointCorrectionCommand{AccountID: failAccount, UserID: failUser, Points: 5, Reason: "force rollback", IdempotencyKey: "fail-" + suffix, Audit: PersonalPointAudit{ActorID: audit.ActorID, ActorRole: audit.ActorRole, Action: audit.Action, Method: audit.Method, Path: audit.Path, RequestID: "fail-request-" + suffix}})
	if err == nil || errors.Is(err, ErrInvalidPointCommand) {
		t.Fatalf("forced audit failure err=%v", err)
	}
	var balance int64
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(sum(available),0) FROM xz_point_accounts WHERE id=$1`, failAccount).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_personal_point_lots WHERE account_id=$1`, failAccount).Scan(&lots); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_wallet_ledger WHERE account_id=$1`, failAccount).Scan(&wallets); err != nil {
		t.Fatal(err)
	}
	if balance != 0 || lots != 0 || wallets != 0 {
		t.Fatalf("audit failure leaked state balance/lots/wallets=%d/%d/%d", balance, lots, wallets)
	}
}

func TestPostgresAdminGiftUsesServerPolicyAndAtomicAudit(t *testing.T) {
	db, store, ctx := openPersonalPointFixRound1Postgres(t)
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	accountID, userID := "admin-gift-"+suffix, "user-admin-gift-"+suffix
	command := PersonalPointGrantCommand{
		AccountID: accountID, UserID: userID, Source: PointSourceAdminGift, Points: 6, Reason: "campaign grant", ReferenceType: "ADMIN_GIFT", ReferenceID: "gift-" + suffix, IdempotencyKey: "gift-" + suffix,
		Audit: PersonalPointAudit{ActorID: "admin-" + suffix, ActorRole: "SUPER_ADMIN", Action: "personal_points.admin_gift", Method: "POST", Path: "/api/v1/admin/customers/" + userID + "/point-gifts", RequestID: "request-gift-" + suffix},
	}
	result, err := store.grant(ctx, command)
	if err != nil || result.Idempotent || result.Lot.SourceType != PointSourceAdminGift || result.Lot.ExpiresAt.IsZero() || result.Lot.PolicyVersionID == "" {
		t.Fatalf("gift=%+v err=%v", result, err)
	}
	replay, err := store.grant(ctx, command)
	if err != nil || !replay.Idempotent {
		t.Fatalf("gift replay=%+v err=%v", replay, err)
	}
	var audits int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_audit_logs WHERE resource_id=$1 AND action=$2`, accountID, command.Audit.Action).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("gift audit rows=%d", audits)
	}
}

func TestPostgresAdminGiftWithExplicitValidityPreservesPolicyProvenance(t *testing.T) {
	db, store, ctx := openPersonalPointFixRound1Postgres(t)
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	accountID, userID := "admin-gift-validity-"+suffix, "user-admin-gift-validity-"+suffix
	command := PersonalPointGrantCommand{
		AccountID: accountID, UserID: userID, Source: PointSourceAdminGift, Points: 1000, Reason: "manual validity test", ReferenceType: "ADMIN_GIFT", ReferenceID: "gift-" + suffix, IdempotencyKey: "gift-validity-" + suffix,
		Audit: PersonalPointAudit{ActorID: "admin-" + suffix, ActorRole: "SUPER_ADMIN", Action: "personal_points.admin_gift", Method: "POST", Path: "/api/v1/admin/customers/" + userID + "/point-gifts", RequestID: "request-gift-validity-" + suffix},
	}

	result, err := grantAdminPointGiftWithValidity(ctx, NewPersonalPointService(store), command, 365)
	if err != nil {
		t.Fatalf("gift with explicit validity failed: %v", err)
	}
	if result.Lot.PolicyVersionID == "" || result.Lot.ExpiresAt.IsZero() || result.Lot.AvailablePoints != 1000 {
		t.Fatalf("unexpected gift result: %+v", result)
	}

	var policyVersionID string
	var lotCount, walletCount, movementCount int
	if err := db.QueryRowContext(ctx, `SELECT policy_version_id FROM xz_personal_point_lots WHERE id=$1`, result.Lot.ID).Scan(&policyVersionID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_personal_point_lots WHERE account_id=$1`, accountID).Scan(&lotCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_wallet_ledger WHERE account_id=$1`, accountID).Scan(&walletCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_personal_point_lot_movements WHERE account_id=$1`, accountID).Scan(&movementCount); err != nil {
		t.Fatal(err)
	}
	if policyVersionID == "" || lotCount != 1 || walletCount != 1 || movementCount != 1 {
		t.Fatalf("policy/ledger provenance=%q lots/wallets/movements=%d/%d/%d", policyVersionID, lotCount, walletCount, movementCount)
	}
}

func TestAdminGiftValidityUpdateDoesNotClearPolicyVersion(t *testing.T) {
	source, err := os.ReadFile("admin_manual_entitlements.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(source), "policy_version_id=NULL") {
		t.Fatal("explicit-validity ADMIN_GIFT must preserve the canonical policy_version_id")
	}
}

func quoteSQLLiteral(value string) string {
	return "'" + value + "'"
}
