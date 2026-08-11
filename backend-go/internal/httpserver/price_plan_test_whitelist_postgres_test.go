package httpserver

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPricePlanTestWhitelistStorePostgresLifecycleAndAudit(t *testing.T) {
	db, ctx := openPhase2ETestPostgres(t)
	applyPhase2EMigrationForTest(t, ctx, db)
	fixture := insertPhase2EPricePlanFixture(t, ctx, db, "TEST")
	store := &postgresStore{db: db, ready: true}
	revisionZero := int64(0)
	validFrom := time.Now().UTC().Add(5 * time.Minute)
	validUntil := validFrom.Add(time.Hour)

	created, err := store.createPricePlanTestWhitelist(ctx, fixture.pricePlanID, pricePlanTestWhitelistCreateMutation{
		Revision: &revisionZero, UserID: fixture.userID, Reason: "pilot member",
		ValidFrom: &validFrom, ValidUntil: &validUntil, ChangeReason: "start controlled TEST access",
	}, fixture.actorID, "SUPER_ADMIN")
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.PlanID != fixture.planID || created.PricePlanID != fixture.pricePlanID ||
		created.UserID != fixture.userID || created.Status != "PENDING" || created.Revision != 1 ||
		created.Reason != "pilot member" || created.CreatedBy != fixture.actorID {
		t.Fatalf("unexpected created whitelist: %+v", created)
	}
	var createRevisionBefore, createRevisionAfter int64
	if err := db.QueryRowContext(ctx, `
		select revision_before,revision_after from xz_audit_logs
		where whitelist_entry_id=$1 and action='price_plan.test_whitelist.create'
	`, created.ID).Scan(&createRevisionBefore, &createRevisionAfter); err != nil {
		t.Fatal(err)
	}
	if createRevisionBefore != 0 || createRevisionAfter != 1 {
		t.Fatalf("create audit revisions=%d/%d want=0/1", createRevisionBefore, createRevisionAfter)
	}

	_, err = store.createPricePlanTestWhitelist(ctx, fixture.pricePlanID, pricePlanTestWhitelistCreateMutation{
		Revision: &revisionZero, UserID: fixture.userID, Reason: "duplicate pilot",
		ValidUntil: &validUntil, ChangeReason: "duplicate controlled access",
	}, fixture.actorID, "SUPER_ADMIN")
	requireBusinessError(t, err, http.StatusConflict, "WHITELIST_ACTIVE_EXISTS")

	items, err := store.listPricePlanTestWhitelist(ctx, fixture.pricePlanID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != created.ID || items[0].Status != "PENDING" {
		t.Fatalf("unexpected whitelist list: %+v", items)
	}

	updatedReason := "pilot member cohort 2"
	extendedUntil := validUntil.Add(time.Hour)
	revisionOne := int64(1)
	updated, err := store.updatePricePlanTestWhitelist(ctx, fixture.pricePlanID, created.ID, pricePlanTestWhitelistUpdateMutation{
		Revision: &revisionOne, Reason: &updatedReason, ValidUntil: &extendedUntil,
		ChangeReason: "extend controlled TEST access",
	}, fixture.actorID, "SUPER_ADMIN")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 || updated.Reason != updatedReason || updated.ValidUntil == nil || absDuration(updated.ValidUntil.Sub(extendedUntil)) > time.Microsecond {
		t.Fatalf("unexpected updated whitelist: %+v", updated)
	}

	_, err = store.updatePricePlanTestWhitelist(ctx, fixture.pricePlanID, created.ID, pricePlanTestWhitelistUpdateMutation{
		Revision: &revisionOne, Reason: &updatedReason, ChangeReason: "stale edit",
	}, fixture.actorID, "SUPER_ADMIN")
	requireBusinessError(t, err, http.StatusConflict, "REVISION_CONFLICT")

	revisionTwo := int64(2)
	disabled, alreadyDisabled, err := store.disablePricePlanTestWhitelist(ctx, fixture.pricePlanID, created.ID, pricePlanTestWhitelistDisableMutation{
		Revision: &revisionTwo, ChangeReason: "end controlled TEST access",
	}, fixture.actorID, "SUPER_ADMIN")
	if err != nil {
		t.Fatal(err)
	}
	if alreadyDisabled || disabled.Status != "DISABLED" || disabled.Revision != 3 || disabled.DisabledBy != fixture.actorID || disabled.DisabledAt == nil {
		t.Fatalf("unexpected disabled whitelist: already=%v item=%+v", alreadyDisabled, disabled)
	}

	var auditsBefore int
	if err := db.QueryRowContext(ctx, `select count(*) from xz_audit_logs where whitelist_entry_id=$1 and domain='PRICING_TEST_WHITELIST'`, created.ID).Scan(&auditsBefore); err != nil {
		t.Fatal(err)
	}
	retried, alreadyDisabled, err := store.disablePricePlanTestWhitelist(ctx, fixture.pricePlanID, created.ID, pricePlanTestWhitelistDisableMutation{
		Revision: &revisionTwo, ChangeReason: "repeat disable request",
	}, fixture.actorID, "SUPER_ADMIN")
	if err != nil {
		t.Fatal(err)
	}
	var auditsAfter int
	if err := db.QueryRowContext(ctx, `select count(*) from xz_audit_logs where whitelist_entry_id=$1 and domain='PRICING_TEST_WHITELIST'`, created.ID).Scan(&auditsAfter); err != nil {
		t.Fatal(err)
	}
	if !alreadyDisabled || retried.Revision != 3 || auditsAfter != auditsBefore {
		t.Fatalf("idempotent disable dirtied state: already=%v revision=%d audits=%d->%d", alreadyDisabled, retried.Revision, auditsBefore, auditsAfter)
	}

	revisionThree := int64(3)
	_, err = store.updatePricePlanTestWhitelist(ctx, fixture.pricePlanID, created.ID, pricePlanTestWhitelistUpdateMutation{
		Revision: &revisionThree, Reason: &updatedReason, ChangeReason: "terminal edit forbidden",
	}, fixture.actorID, "SUPER_ADMIN")
	requireBusinessError(t, err, http.StatusConflict, "WHITELIST_ENTRY_TERMINAL")

	var auditAction, auditReason, auditPlanID, auditPricePlanID, auditWhitelistID, auditEnvironment string
	var revisionBefore, revisionAfter int64
	var beforeSnapshot, afterSnapshot []byte
	if err := db.QueryRowContext(ctx, `
		select action,change_reason,plan_id,price_plan_id,whitelist_entry_id,environment,
		       revision_before,revision_after,before_snapshot,after_snapshot
		from xz_audit_logs
		where whitelist_entry_id=$1 and action='price_plan.test_whitelist.update'
		order by created_at desc limit 1
	`, created.ID).Scan(&auditAction, &auditReason, &auditPlanID, &auditPricePlanID, &auditWhitelistID,
		&auditEnvironment, &revisionBefore, &revisionAfter, &beforeSnapshot, &afterSnapshot); err != nil {
		t.Fatal(err)
	}
	if auditAction == "" || auditReason != "extend controlled TEST access" || auditPlanID != fixture.planID ||
		auditPricePlanID != fixture.pricePlanID || auditWhitelistID != created.ID || auditEnvironment != "SANDBOX" ||
		revisionBefore != 1 || revisionAfter != 2 || len(beforeSnapshot) == 0 || len(afterSnapshot) == 0 {
		t.Fatalf("structured audit incomplete: action=%s reason=%s ids=%s/%s/%s env=%s revisions=%d/%d before=%s after=%s",
			auditAction, auditReason, auditPlanID, auditPricePlanID, auditWhitelistID, auditEnvironment,
			revisionBefore, revisionAfter, beforeSnapshot, afterSnapshot)
	}
	for _, expected := range []struct {
		action, method, beforeLifecycle, afterLifecycle string
	}{
		{action: "price_plan.test_whitelist.create", method: http.MethodPost, afterLifecycle: "ACTIVE"},
		{action: "price_plan.test_whitelist.update", method: http.MethodPatch, beforeLifecycle: "ACTIVE", afterLifecycle: "ACTIVE"},
		{action: "price_plan.test_whitelist.disable", method: http.MethodPost, beforeLifecycle: "ACTIVE", afterLifecycle: "DISABLED"},
	} {
		var method string
		var beforeLifecycle, afterLifecycle sql.NullString
		if err := db.QueryRowContext(ctx, `
			select method,before_snapshot->>'lifecycleStatus',after_snapshot->>'lifecycleStatus'
			from xz_audit_logs where whitelist_entry_id=$1 and action=$2
			order by created_at desc limit 1
		`, created.ID, expected.action).Scan(&method, &beforeLifecycle, &afterLifecycle); err != nil {
			t.Fatal(err)
		}
		if method != expected.method || beforeLifecycle.String != expected.beforeLifecycle || afterLifecycle.String != expected.afterLifecycle {
			t.Fatalf("audit %s method/lifecycle=%s/%s->%s want=%s/%s->%s", expected.action,
				method, beforeLifecycle.String, afterLifecycle.String, expected.method, expected.beforeLifecycle, expected.afterLifecycle)
		}
	}
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

func TestPricePlanTestWhitelistStorePostgresRetiresElapsedActiveBeforeCreate(t *testing.T) {
	db, ctx := openPhase2ETestPostgres(t)
	applyPhase2EMigrationForTest(t, ctx, db)
	fixture := insertPhase2EPricePlanFixture(t, ctx, db, "TEST")
	store := &postgresStore{db: db, ready: true}
	revisionZero := int64(0)
	pastFrom := time.Now().UTC().Add(-2 * time.Hour)
	pastUntil := pastFrom.Add(time.Hour)

	elapsed, err := store.createPricePlanTestWhitelist(ctx, fixture.pricePlanID, pricePlanTestWhitelistCreateMutation{
		Revision: &revisionZero, UserID: fixture.userID, Reason: "elapsed pilot",
		ValidFrom: &pastFrom, ValidUntil: &pastUntil, ChangeReason: "seed elapsed controlled access",
	}, fixture.actorID, "SUPER_ADMIN")
	if err != nil {
		t.Fatal(err)
	}
	if elapsed.Status != "EXPIRED" {
		t.Fatalf("elapsed API status=%s", elapsed.Status)
	}

	futureUntil := time.Now().UTC().Add(time.Hour)
	current, err := store.createPricePlanTestWhitelist(ctx, fixture.pricePlanID, pricePlanTestWhitelistCreateMutation{
		Revision: &revisionZero, UserID: fixture.userID, Reason: "replacement pilot",
		ValidUntil: &futureUntil, ChangeReason: "replace elapsed controlled access",
	}, fixture.actorID, "SUPER_ADMIN")
	if err != nil {
		t.Fatal(err)
	}
	if current.ID == elapsed.ID || current.Status != "ACTIVE" {
		t.Fatalf("unexpected replacement: %+v", current)
	}
	var oldStatus string
	if err := db.QueryRowContext(ctx, `select lifecycle_status from xz_price_plan_user_whitelist where id=$1`, elapsed.ID).Scan(&oldStatus); err != nil {
		t.Fatal(err)
	}
	if oldStatus != "EXPIRED" {
		t.Fatalf("elapsed stored lifecycle=%s", oldStatus)
	}
	var retirementAudits int
	if err := db.QueryRowContext(ctx, `
		select count(*) from xz_audit_logs
		where whitelist_entry_id=$1 and action='price_plan.test_whitelist.auto_expire'
	`, elapsed.ID).Scan(&retirementAudits); err != nil {
		t.Fatal(err)
	}
	if retirementAudits != 1 {
		t.Fatalf("auto-expire audits=%d", retirementAudits)
	}
	var method string
	var beforeLifecycle, afterLifecycle sql.NullString
	if err := db.QueryRowContext(ctx, `
		select method,before_snapshot->>'lifecycleStatus',after_snapshot->>'lifecycleStatus'
		from xz_audit_logs
		where whitelist_entry_id=$1 and action='price_plan.test_whitelist.auto_expire'
	`, elapsed.ID).Scan(&method, &beforeLifecycle, &afterLifecycle); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPost || beforeLifecycle.String != "ACTIVE" || afterLifecycle.String != "EXPIRED" {
		t.Fatalf("auto-expire audit method/lifecycle=%s/%s->%s", method, beforeLifecycle.String, afterLifecycle.String)
	}
}

func TestPricePlanTestWhitelistStoreRejectsNonTestPricePlan(t *testing.T) {
	db, ctx := openPhase2ETestPostgres(t)
	applyPhase2EMigrationForTest(t, ctx, db)
	fixture := insertPhase2EPricePlanFixture(t, ctx, db, "NORMAL")
	store := &postgresStore{db: db, ready: true}
	revisionZero := int64(0)
	_, err := store.createPricePlanTestWhitelist(ctx, fixture.pricePlanID, pricePlanTestWhitelistCreateMutation{
		Revision: &revisionZero, UserID: fixture.userID, Reason: "not a TEST plan", ChangeReason: "must reject",
	}, fixture.actorID, "SUPER_ADMIN")
	requireBusinessError(t, err, http.StatusUnprocessableEntity, "PRICE_PLAN_TEST_REQUIRED")
}

func TestPricePlanTestWhitelistStoreRejectsPricePlanWhoseExactVersionIsUnmanaged(t *testing.T) {
	db, ctx := openPhase2ETestPostgres(t)
	applyPhase2EMigrationForTest(t, ctx, db)
	fixture := insertPhase2EPricePlanFixture(t, ctx, db, "TEST")
	suffix := fixture.suffix + "_exact"
	agentPlanID := "plan_agent_" + suffix
	agentVersionID := "version_agent_" + suffix
	if _, err := db.ExecContext(ctx, `
		insert into xz_plans(id,code,name,plan_type,active)
		values($1,$1,'mismatched agent plan','AGENT_JOIN_PACKAGE',true)
	`, agentPlanID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into xz_plan_versions(
			id,plan_id,version_no,business_type,rights_snapshot,agent_level,token_amount,duration_days,status
		) values($1,$2,1,'AGENT','{"agentLevel":"L1","tokenAmount":100,"durationDays":30}'::jsonb,'L1',100,30,'ACTIVE')
	`, agentVersionID, agentPlanID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `update xz_price_plans set plan_version_id=$2 where id=$1`, fixture.pricePlanID, agentVersionID); err != nil {
		t.Fatal(err)
	}
	store := &postgresStore{db: db, ready: true}
	revisionZero := int64(0)
	_, err := store.createPricePlanTestWhitelist(ctx, fixture.pricePlanID, pricePlanTestWhitelistCreateMutation{
		Revision: &revisionZero, UserID: fixture.userID, Reason: "mismatched exact version", ChangeReason: "must reject exact version mismatch",
	}, fixture.actorID, "SUPER_ADMIN")
	requireBusinessError(t, err, http.StatusNotFound, "PRICE_PLAN_NOT_MANAGED")
}

func TestPricePlanTestWhitelistStoreConcurrentCreateKeepsOneActive(t *testing.T) {
	db, ctx := openPhase2ETestPostgres(t)
	applyPhase2EMigrationForTest(t, ctx, db)
	fixture := insertPhase2EPricePlanFixture(t, ctx, db, "TEST")
	store := &postgresStore{db: db, ready: true}
	revisionZero := int64(0)
	start := make(chan struct{})
	type createResult struct {
		item pricePlanTestWhitelistView
		err  error
	}
	results := make(chan createResult, 2)
	var wg sync.WaitGroup
	for index := 0; index < 2; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			item, err := store.createPricePlanTestWhitelist(ctx, fixture.pricePlanID, pricePlanTestWhitelistCreateMutation{
				Revision: &revisionZero, UserID: fixture.userID, Reason: "concurrent pilot",
				ChangeReason: "concurrent create " + string(rune('A'+index)),
			}, fixture.actorID, "SUPER_ADMIN")
			results <- createResult{item: item, err: err}
		}(index)
	}
	close(start)
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for result := range results {
		if result.err == nil {
			successes++
			continue
		}
		var businessErr *businessPlanAdminError
		if errors.As(result.err, &businessErr) && businessErr.status == http.StatusConflict && businessErr.code == "WHITELIST_ACTIVE_EXISTS" {
			conflicts++
			continue
		}
		t.Fatalf("unexpected concurrent create result: item=%+v err=%v", result.item, result.err)
	}
	var activeRows int
	if err := db.QueryRowContext(ctx, `
		select count(*) from xz_price_plan_user_whitelist
		where price_plan_id=$1 and user_id=$2 and lifecycle_status='ACTIVE'
	`, fixture.pricePlanID, fixture.userID).Scan(&activeRows); err != nil {
		t.Fatal(err)
	}
	if successes != 1 || conflicts != 1 || activeRows != 1 {
		t.Fatalf("concurrent create successes/conflicts/active=%d/%d/%d want=1/1/1", successes, conflicts, activeRows)
	}
}

func TestPricePlanTestWhitelistPostgresPermitsMultipleTerminalHistoryRows(t *testing.T) {
	db, ctx := openPhase2ETestPostgres(t)
	applyPhase2EMigrationForTest(t, ctx, db)
	fixture := insertPhase2EPricePlanFixture(t, ctx, db, "TEST")
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for index, lifecycle := range []string{"EXPIRED", "DISABLED", "EXPIRED"} {
		if _, err := tx.ExecContext(ctx, `
			insert into xz_price_plan_user_whitelist(
				id,price_plan_id,user_id,enabled,lifecycle_status,reason,created_by,updated_by
			) values($1,$2,$3,false,$4,'terminal history',$5,$5)
		`, fixture.pricePlanID+"_terminal_"+string(rune('A'+index)), fixture.pricePlanID, fixture.userID, lifecycle, fixture.actorID); err != nil {
			t.Fatal(err)
		}
	}
	var terminalRows int
	if err := tx.QueryRowContext(ctx, `
		select count(*) from xz_price_plan_user_whitelist
		where price_plan_id=$1 and user_id=$2 and lifecycle_status in('EXPIRED','DISABLED')
	`, fixture.pricePlanID, fixture.userID).Scan(&terminalRows); err != nil {
		t.Fatal(err)
	}
	if terminalRows != 3 {
		t.Fatalf("terminal history rows=%d want=3", terminalRows)
	}
}

func TestPricePlanTestWhitelistStoreRollsBackStateWhenAuditInsertFails(t *testing.T) {
	db, ctx := openPhase2ETestPostgres(t)
	applyPhase2EMigrationForTest(t, ctx, db)
	fixture := insertPhase2EPricePlanFixture(t, ctx, db, "TEST")
	if _, err := db.ExecContext(ctx, `
		drop trigger if exists trg_xz_test_fail_whitelist_audit_100 on xz_audit_logs;
		drop function if exists xz_test_fail_whitelist_audit_100();
		create function xz_test_fail_whitelist_audit_100()
		returns trigger language plpgsql as $$
		begin
			raise exception using errcode='P0001',message='FORCED_WHITELIST_AUDIT_FAILURE';
		end;
		$$;
		create trigger trg_xz_test_fail_whitelist_audit_100
		before insert on xz_audit_logs
		for each row when (new.actor_role='ROLLBACK_TEST')
		execute function xz_test_fail_whitelist_audit_100();
	`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`
			drop trigger if exists trg_xz_test_fail_whitelist_audit_100 on xz_audit_logs;
			drop function if exists xz_test_fail_whitelist_audit_100();
		`)
	})
	store := &postgresStore{db: db, ready: true}
	revisionZero := int64(0)
	_, err := store.createPricePlanTestWhitelist(ctx, fixture.pricePlanID, pricePlanTestWhitelistCreateMutation{
		Revision: &revisionZero, UserID: fixture.userID, Reason: "rollback pilot", ChangeReason: "force audit rollback",
	}, fixture.actorID, "ROLLBACK_TEST")
	if err == nil || !strings.Contains(err.Error(), "FORCED_WHITELIST_AUDIT_FAILURE") {
		t.Fatalf("forced audit failure err=%v", err)
	}
	var whitelistRows int
	if err := db.QueryRowContext(ctx, `
		select count(*) from xz_price_plan_user_whitelist where price_plan_id=$1 and user_id=$2
	`, fixture.pricePlanID, fixture.userID).Scan(&whitelistRows); err != nil {
		t.Fatal(err)
	}
	if whitelistRows != 0 {
		t.Fatalf("whitelist rows after audit failure=%d want=0", whitelistRows)
	}
}

func requireBusinessError(t *testing.T, err error, status int, code string) {
	t.Helper()
	var businessErr *businessPlanAdminError
	if !errors.As(err, &businessErr) || businessErr.status != status || businessErr.code != code {
		t.Fatalf("business error status/code=%v/%v err=%v want=%d/%s", func() int {
			if businessErr != nil {
				return businessErr.status
			}
			return 0
		}(), func() string {
			if businessErr != nil {
				return businessErr.code
			}
			return ""
		}(), err, status, code)
	}
}
