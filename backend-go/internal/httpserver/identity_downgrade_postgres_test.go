package httpserver

import (
	"database/sql"
	"errors"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestControlledIdentityDowngrade(t *testing.T) {
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
	adminID := "downgrade_admin_" + suffix
	seedIdentityChangeUser(t, db, adminID, "SUPER_ADMIN")

	t.Run("agent downgrade preserves benefits and relationship history", func(t *testing.T) {
		userID := "downgrade_agent_" + suffix
		childID := "downgrade_member_" + suffix
		seedIdentityChangeUser(t, db, userID, "MEMBER")
		seedIdentityChangeUser(t, db, childID, "MEMBER")
		upgradeIdentityForDowngradeTest(t, store, adminID, userID, "AGENT")
		agentID := identityEntityIDForTest(t, db, "xz_channel_agents", userID)
		seedDowngradeRelationship(t, db, "rel_preserve_"+suffix, childID, agentID, "")
		_, err := db.Exec(`INSERT INTO xz_membership_entitlement_records(id,tenant_id,user_id,member_level,effective_at,expires_at,source_order_no,idempotency_key) VALUES($1,'tenant_default',$2,'PRO',now()-interval '1 day',now()+interval '30 days',$3,$4)`, "membership_child_"+suffix, childID, "membership_order_"+suffix, "membership_key_"+suffix)
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.Exec(`INSERT INTO xz_user_wallets(user_id,token_balance,cash_balance_cents) VALUES($1,12345,6789) ON CONFLICT(user_id) DO UPDATE SET token_balance=12345,cash_balance_cents=6789`, userID)
		if err != nil {
			t.Fatal(err)
		}
		preview, err := store.PreviewAdminIdentityDowngrade(adminID, "SUPER_ADMIN", userID, identityDowngradeRequest{ChildStrategy: downgradeKeepHistory, Reason: "controlled partner exit"})
		if err != nil {
			t.Fatal(err)
		}
		if preview.Status != "READY" || preview.DownlineMembers != 1 || preview.MigrationCount != 1 {
			t.Fatalf("unexpected preview: %+v", preview)
		}
		result, err := store.ConfirmAdminIdentityDowngrade(adminID, "SUPER_ADMIN", userID, identityDowngradeConfirmRequest{PreviewToken: preview.PreviewToken, HighRiskConfirmed: true})
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != "SUCCEEDED" || result.MigratedRelationships != 1 {
			t.Fatalf("unexpected result: %+v", result)
		}
		repeated, err := store.ConfirmAdminIdentityDowngrade(adminID, "SUPER_ADMIN", userID, identityDowngradeConfirmRequest{PreviewToken: preview.PreviewToken, HighRiskConfirmed: true})
		if err != nil || !repeated.Idempotent || repeated.RequestID != result.RequestID {
			t.Fatalf("duplicate confirmation was not idempotent: %+v %v", repeated, err)
		}
		var token, cash int64
		if err := db.QueryRow(`SELECT token_balance,cash_balance_cents FROM xz_user_wallets WHERE user_id=$1`, userID).Scan(&token, &cash); err != nil || token != 12345 || cash != 6789 {
			t.Fatalf("benefits changed token=%d cash=%d err=%v", token, cash, err)
		}
		var status string
		var commission bool
		if err := db.QueryRow(`SELECT identity_status,commission_enabled FROM xz_user_business_identities WHERE user_id=$1 AND identity_type='AGENT' ORDER BY created_at DESC LIMIT 1`, userID).Scan(&status, &commission); err != nil || status != "TERMINATED" || commission {
			t.Fatalf("identity not terminated: %s %v %v", status, commission, err)
		}
		var ended int
		if err := db.QueryRow(`SELECT count(*) FROM xz_user_relationships WHERE id=$1 AND status='ENDED'`, "rel_preserve_"+suffix).Scan(&ended); err != nil || ended != 1 {
			t.Fatalf("history was not ended and preserved: %d %v", ended, err)
		}
	})

	t.Run("waiting request executes after settlement clears", func(t *testing.T) {
		userID := "downgrade_wait_" + suffix
		seedIdentityChangeUser(t, db, userID, "MEMBER")
		upgradeIdentityForDowngradeTest(t, store, adminID, userID, "AGENT")
		agentID := identityEntityIDForTest(t, db, "xz_channel_agents", userID)
		_, err := db.Exec(`INSERT INTO xz_commission_wallet_accounts(id,tenant_id,beneficiary_type,beneficiary_id,expected_cents) VALUES($1,'tenant_default','AGENT',$2,900)`, "commission_account_"+suffix, agentID)
		if err != nil {
			t.Fatal(err)
		}
		blocked, err := store.PreviewAdminIdentityDowngrade(adminID, "SUPER_ADMIN", userID, identityDowngradeRequest{ChildStrategy: downgradeKeepHistory, Reason: "wait for settlement"})
		if err != nil || blocked.Status != "BLOCKED" {
			t.Fatalf("expected blocker: %+v %v", blocked, err)
		}
		waiting, err := store.PreviewAdminIdentityDowngrade(adminID, "SUPER_ADMIN", userID, identityDowngradeRequest{ChildStrategy: downgradeKeepHistory, WaitForSettlement: true, Reason: "wait for settlement"})
		if err != nil || waiting.Status != "WAITING" {
			t.Fatalf("expected waiting: %+v %v", waiting, err)
		}
		result, err := store.ConfirmAdminIdentityDowngrade(adminID, "SUPER_ADMIN", userID, identityDowngradeConfirmRequest{PreviewToken: waiting.PreviewToken, HighRiskConfirmed: true})
		if err != nil || result.Status != "WAITING" {
			t.Fatalf("waiting confirm: %+v %v", result, err)
		}
		if _, err = db.Exec(`UPDATE xz_commission_wallet_accounts SET expected_cents=0 WHERE beneficiary_id=$1`, agentID); err != nil {
			t.Fatal(err)
		}
		if err = store.ProcessDueIdentityDowngrades(t.Context(), 10); err != nil {
			t.Fatal(err)
		}
		var requestStatus string
		if err = db.QueryRow(`SELECT status FROM xz_identity_downgrade_requests WHERE id=$1`, result.RequestID).Scan(&requestStatus); err != nil || requestStatus != "SUCCEEDED" {
			t.Fatalf("automatic downgrade failed: %s %v", requestStatus, err)
		}
	})

	t.Run("cycle target is blocked and leaves relationships unchanged", func(t *testing.T) {
		parentUser := "downgrade_parent_" + suffix
		childAgentUser := "downgrade_child_agent_" + suffix
		seedIdentityChangeUser(t, db, parentUser, "MEMBER")
		seedIdentityChangeUser(t, db, childAgentUser, "MEMBER")
		upgradeIdentityForDowngradeTest(t, store, adminID, parentUser, "AGENT")
		upgradeIdentityForDowngradeTest(t, store, adminID, childAgentUser, "AGENT")
		parentAgent := identityEntityIDForTest(t, db, "xz_channel_agents", parentUser)
		childAgent := identityEntityIDForTest(t, db, "xz_channel_agents", childAgentUser)
		seedDowngradeRelationship(t, db, "rel_cycle_"+suffix, childAgentUser, parentAgent, "")
		preview, err := store.PreviewAdminIdentityDowngrade(adminID, "SUPER_ADMIN", parentUser, identityDowngradeRequest{ChildStrategy: downgradeTransferAgent, TargetAgentID: childAgent, Reason: "invalid circular migration"})
		if err != nil {
			t.Fatal(err)
		}
		if preview.Status != "BLOCKED" {
			t.Fatalf("cycle was not blocked: %+v", preview)
		}
		var active int
		if err = db.QueryRow(`SELECT count(*) FROM xz_user_relationships WHERE id=$1 AND status='ACTIVE'`, "rel_cycle_"+suffix).Scan(&active); err != nil || active != 1 {
			t.Fatalf("blocked preview changed data: %d %v", active, err)
		}
	})

	t.Run("bulk transfer revalidates target and reports result", func(t *testing.T) {
		parentUser := "downgrade_bulk_parent_" + suffix
		targetUser := "downgrade_bulk_target_" + suffix
		childOne := "downgrade_bulk_child_1_" + suffix
		childTwo := "downgrade_bulk_child_2_" + suffix
		for _, userID := range []string{parentUser, targetUser, childOne, childTwo} {
			seedIdentityChangeUser(t, db, userID, "MEMBER")
		}
		upgradeIdentityForDowngradeTest(t, store, adminID, parentUser, "AGENT")
		upgradeIdentityForDowngradeTest(t, store, adminID, targetUser, "AGENT")
		parentAgent := identityEntityIDForTest(t, db, "xz_channel_agents", parentUser)
		targetAgent := identityEntityIDForTest(t, db, "xz_channel_agents", targetUser)
		seedDowngradeRelationship(t, db, "rel_bulk_1_"+suffix, childOne, parentAgent, "")
		seedDowngradeRelationship(t, db, "rel_bulk_2_"+suffix, childTwo, parentAgent, "")
		preview, err := store.PreviewAdminIdentityDowngrade(adminID, "SUPER_ADMIN", parentUser, identityDowngradeRequest{ChildStrategy: downgradeTransferAgent, TargetAgentID: targetAgent, Reason: "bulk transfer qualification"})
		if err != nil || preview.Status != "READY" || preview.MigrationCount != 2 {
			t.Fatalf("bulk preview: %+v %v", preview, err)
		}
		if _, err = db.Exec(`UPDATE xz_channel_agents SET status='TERMINATED' WHERE id=$1`, targetAgent); err != nil {
			t.Fatal(err)
		}
		_, err = store.ConfirmAdminIdentityDowngrade(adminID, "SUPER_ADMIN", parentUser, identityDowngradeConfirmRequest{PreviewToken: preview.PreviewToken, HighRiskConfirmed: true})
		if !errors.Is(err, errIdentityDowngradeBlocked) {
			t.Fatalf("target qualification drift should block: %v", err)
		}
		var originalCount int
		if err = db.QueryRow(`SELECT count(*) FROM xz_user_relationships WHERE parent_agent_id=$1 AND status='ACTIVE'`, parentAgent).Scan(&originalCount); err != nil || originalCount != 2 {
			t.Fatalf("blocked migration was not rolled back: %d %v", originalCount, err)
		}
		if _, err = db.Exec(`UPDATE xz_channel_agents SET status='ACTIVE' WHERE id=$1`, targetAgent); err != nil {
			t.Fatal(err)
		}
		preview, err = store.PreviewAdminIdentityDowngrade(adminID, "SUPER_ADMIN", parentUser, identityDowngradeRequest{ChildStrategy: downgradeTransferAgent, TargetAgentID: targetAgent, Reason: "bulk transfer confirmed"})
		if err != nil {
			t.Fatal(err)
		}
		result, err := store.ConfirmAdminIdentityDowngrade(adminID, "SUPER_ADMIN", parentUser, identityDowngradeConfirmRequest{PreviewToken: preview.PreviewToken, HighRiskConfirmed: true})
		if err != nil || result.Status != "SUCCEEDED" || result.MigratedRelationships != 2 {
			t.Fatalf("bulk result: %+v %v", result, err)
		}
		var transferred int
		if err = db.QueryRow(`SELECT count(*) FROM xz_user_relationships WHERE parent_agent_id=$1 AND status='ACTIVE'`, targetAgent).Scan(&transferred); err != nil || transferred != 2 {
			t.Fatalf("bulk transfer missing: %d %v", transferred, err)
		}
	})

	t.Run("downgrade requires super administrator and second confirmation", func(t *testing.T) {
		userID := "downgrade_permission_" + suffix
		seedIdentityChangeUser(t, db, userID, "MEMBER")
		upgradeIdentityForDowngradeTest(t, store, adminID, userID, "AGENT")
		_, err := store.PreviewAdminIdentityDowngrade(adminID, "ADMIN", userID, identityDowngradeRequest{ChildStrategy: downgradeKeepHistory, Reason: "unauthorized downgrade"})
		if !errors.Is(err, errIdentityDowngradePermission) {
			t.Fatalf("expected permission error: %v", err)
		}
		preview, err := store.PreviewAdminIdentityDowngrade(adminID, "SUPER_ADMIN", userID, identityDowngradeRequest{ChildStrategy: downgradeKeepHistory, Reason: "missing second confirmation"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = store.ConfirmAdminIdentityDowngrade(adminID, "SUPER_ADMIN", userID, identityDowngradeConfirmRequest{PreviewToken: preview.PreviewToken})
		if !errors.Is(err, errIdentityHighRiskConfirm) {
			t.Fatalf("expected high-risk confirmation error: %v", err)
		}
	})

	t.Run("concurrent confirmation is idempotent", func(t *testing.T) {
		userID := "downgrade_concurrent_" + suffix
		seedIdentityChangeUser(t, db, userID, "MEMBER")
		upgradeIdentityForDowngradeTest(t, store, adminID, userID, "AGENT")
		preview, err := store.PreviewAdminIdentityDowngrade(adminID, "SUPER_ADMIN", userID, identityDowngradeRequest{ChildStrategy: downgradeKeepHistory, Reason: "concurrent controlled downgrade"})
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		results := make(chan identityDowngradeResult, 2)
		errorsCh := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				item, confirmErr := store.ConfirmAdminIdentityDowngrade(adminID, "SUPER_ADMIN", userID, identityDowngradeConfirmRequest{PreviewToken: preview.PreviewToken, HighRiskConfirmed: true})
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
		requestID := ""
		idempotentSeen := false
		for item := range results {
			if requestID == "" {
				requestID = item.RequestID
			} else if requestID != item.RequestID {
				t.Fatalf("different request ids: %s %s", requestID, item.RequestID)
			}
			idempotentSeen = idempotentSeen || item.Idempotent
		}
		if !idempotentSeen {
			t.Fatal("concurrent duplicate did not return idempotent result")
		}
	})

	t.Run("scheduled operation center downgrade becomes agent", func(t *testing.T) {
		userID := "downgrade_center_" + suffix
		seedIdentityChangeUser(t, db, userID, "MEMBER")
		upgradeIdentityForDowngradeTest(t, store, adminID, userID, "OPERATION_CENTER")
		future := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
		preview, err := store.PreviewAdminIdentityDowngrade(adminID, "SUPER_ADMIN", userID, identityDowngradeRequest{TargetIdentity: "AGENT", ChildStrategy: downgradeKeepHistory, EffectiveAt: future, Reason: "scheduled center downgrade"})
		if err != nil || preview.Status != "SCHEDULED" {
			t.Fatalf("scheduled preview: %+v %v", preview, err)
		}
		result, err := store.ConfirmAdminIdentityDowngrade(adminID, "SUPER_ADMIN", userID, identityDowngradeConfirmRequest{PreviewToken: preview.PreviewToken, HighRiskConfirmed: true})
		if err != nil || result.Status != "SCHEDULED" {
			t.Fatalf("scheduled confirm: %+v %v", result, err)
		}
		if _, err = db.Exec(`UPDATE xz_identity_downgrade_requests SET effective_at=now()-interval '1 second' WHERE id=$1`, result.RequestID); err != nil {
			t.Fatal(err)
		}
		if err = store.ProcessDueIdentityDowngrades(t.Context(), 10); err != nil {
			t.Fatal(err)
		}
		var activeAgent, terminatedCenter int
		if err = db.QueryRow(`SELECT count(*) FROM xz_user_business_identities WHERE user_id=$1 AND identity_type='AGENT' AND identity_status='ACTIVE'`, userID).Scan(&activeAgent); err != nil {
			t.Fatal(err)
		}
		if err = db.QueryRow(`SELECT count(*) FROM xz_user_business_identities WHERE user_id=$1 AND identity_type='OPERATION_CENTER' AND identity_status='TERMINATED'`, userID).Scan(&terminatedCenter); err != nil {
			t.Fatal(err)
		}
		if activeAgent != 1 || terminatedCenter != 1 {
			t.Fatalf("wrong identities agent=%d center=%d", activeAgent, terminatedCenter)
		}
	})
}

func upgradeIdentityForDowngradeTest(t *testing.T, store *postgresStore, adminID, userID, target string) {
	t.Helper()
	preview, err := store.PreviewAdminIdentityChange(adminID, "SUPER_ADMIN", userID, identityChangePreviewRequest{Action: "UPGRADE", Method: identityMethodOnlyIdentity, TargetIdentity: target, Reason: "seed downgrade identity"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ConfirmAdminIdentityChange(adminID, "SUPER_ADMIN", userID, identityChangeConfirmRequest{PreviewToken: preview.PreviewToken, HighRiskConfirmed: preview.HighRisk})
	if err != nil {
		t.Fatal(err)
	}
}
func identityEntityIDForTest(t *testing.T, db *sql.DB, table, userID string) string {
	t.Helper()
	var id string
	if err := db.QueryRow(`SELECT id FROM `+table+` WHERE user_id=$1`, userID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
func seedDowngradeRelationship(t *testing.T, db *sql.DB, id, userID, parentAgentID, centerID string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO xz_user_relationships(id,tenant_id,user_id,parent_agent_id,operation_center_id,effective_at,status,source_type,created_by) VALUES($1,'tenant_default',$2,nullif($3,''),nullif($4,''),now(),'ACTIVE','TEST','test')`, id, userID, parentAgentID, centerID)
	if err != nil {
		t.Fatal(err)
	}
}
