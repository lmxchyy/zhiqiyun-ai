package operationcenter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestOperationCenterWorkflowPaymentApproveRejectAndRollbackPostgres(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	t.Run("payment creates review required only and is idempotent", func(t *testing.T) {
		fixture := createWorkflowFixture(t, ctx, db, "payment")
		service := mustWorkflowService(t, db, WorkflowOptions{})
		first, err := service.RecordPaymentSucceeded(ctx, PaymentSucceededCommand{OrderID: fixture.orderID, PaymentRecordID: fixture.paymentID})
		if err != nil {
			t.Fatal(err)
		}
		if first.ServiceOrder.Status != OperationCenterServiceReviewRequired || first.ServiceOrder.RefundStatus != OperationCenterRefundNone || first.IdempotentReplay {
			t.Fatalf("unexpected first payment result: %+v", first)
		}
		second, err := service.RecordPaymentSucceeded(ctx, PaymentSucceededCommand{OrderID: fixture.orderID, PaymentRecordID: fixture.paymentID})
		if err != nil || !second.IdempotentReplay || second.ServiceOrder.ID != first.ServiceOrder.ID {
			t.Fatalf("payment replay result=%+v err=%v", second, err)
		}
		assertWorkflowCounts(t, db, fixture, workflowCounts{serviceOrders: 1, serviceTransitions: 1})
		assertNoActiveOperationCenterResources(t, db, fixture)
	})

	t.Run("amount mismatch rolls back service creation", func(t *testing.T) {
		fixture := createWorkflowFixture(t, ctx, db, "amount_mismatch")
		if _, err := db.ExecContext(ctx, `UPDATE xz_payment_records SET amount_cents=amount_cents-1 WHERE id=$1`, fixture.paymentID); err != nil {
			t.Fatal(err)
		}
		service := mustWorkflowService(t, db, WorkflowOptions{})
		if _, err := service.RecordPaymentSucceeded(ctx, PaymentSucceededCommand{OrderID: fixture.orderID, PaymentRecordID: fixture.paymentID}); !errors.Is(err, ErrPaymentAmountMismatch) {
			t.Fatalf("error=%v", err)
		}
		assertWorkflowCounts(t, db, fixture, workflowCounts{})
	})

	t.Run("approve atomically activates local resources without rewards", func(t *testing.T) {
		fixture := createWorkflowFixture(t, ctx, db, "approve")
		trigger := &recordingReferralTrigger{}
		hook := &recordingActivationHook{}
		service := mustWorkflowService(t, db, WorkflowOptions{ReferralEligibilityTrigger: trigger, ActivationHook: hook})
		paid, err := service.RecordPaymentSucceeded(ctx, PaymentSucceededCommand{OrderID: fixture.orderID, PaymentRecordID: fixture.paymentID})
		if err != nil {
			t.Fatal(err)
		}
		result, err := service.Review(ctx, ReviewCommand{
			ServiceOrderID: paid.ServiceOrder.ID, Decision: ReviewApproved,
			ExpectedStatus: OperationCenterServiceReviewRequired, IdempotencyKey: fixture.id("approve-key"),
			ReviewedBy: fixture.reviewerID, RequestID: fixture.id("approve-request"), Reason: "approved",
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.ServiceOrder.Status != OperationCenterServiceActive || result.IdempotentReplay {
			t.Fatalf("approval result=%+v", result)
		}
		if trigger.calls.Load() != 1 || hook.calls.Load() != 1 {
			t.Fatalf("hook calls referral=%d activation=%d", trigger.calls.Load(), hook.calls.Load())
		}
		assertApprovedResources(t, db, fixture)
		assertWorkflowCounts(t, db, fixture, workflowCounts{serviceOrders: 1, reviewEvents: 1, serviceTransitions: 2})
		assertNoReferralMoneyWrites(t, db, fixture)

		replay, err := service.Review(ctx, ReviewCommand{
			ServiceOrderID: paid.ServiceOrder.ID, Decision: ReviewApproved,
			ExpectedStatus: OperationCenterServiceReviewRequired, IdempotencyKey: fixture.id("approve-key"),
			ReviewedBy: fixture.reviewerID, RequestID: fixture.id("approve-request"), Reason: "approved",
		})
		if err != nil || !replay.IdempotentReplay {
			t.Fatalf("approval replay=%+v err=%v", replay, err)
		}
		if trigger.calls.Load() != 1 || hook.calls.Load() != 1 {
			t.Fatal("idempotent replay invoked activation hooks")
		}
		assertWorkflowCounts(t, db, fixture, workflowCounts{serviceOrders: 1, reviewEvents: 1, serviceTransitions: 2})

		_, err = service.Review(ctx, ReviewCommand{
			ServiceOrderID: paid.ServiceOrder.ID, Decision: ReviewRejected,
			ExpectedStatus: OperationCenterServiceReviewRequired, IdempotencyKey: fixture.id("approve-key"),
			ReviewedBy: fixture.reviewerID, RequestID: fixture.id("conflict-request"), Reason: "conflict",
		})
		if !errors.Is(err, ErrReviewDecisionConflict) {
			t.Fatalf("decision conflict error=%v", err)
		}
	})

	t.Run("activation hook failure rolls back every resource", func(t *testing.T) {
		fixture := createWorkflowFixture(t, ctx, db, "approve_rollback")
		service := mustWorkflowService(t, db, WorkflowOptions{ActivationHook: failingActivationHook{}})
		paid, err := service.RecordPaymentSucceeded(ctx, PaymentSucceededCommand{OrderID: fixture.orderID, PaymentRecordID: fixture.paymentID})
		if err != nil {
			t.Fatal(err)
		}
		_, err = service.Review(ctx, ReviewCommand{
			ServiceOrderID: paid.ServiceOrder.ID, Decision: ReviewApproved,
			ExpectedStatus: OperationCenterServiceReviewRequired, IdempotencyKey: fixture.id("rollback-key"),
			ReviewedBy: fixture.reviewerID, RequestID: fixture.id("rollback-request"), Reason: "approved",
		})
		if err == nil {
			t.Fatal("expected activation hook failure")
		}
		assertNoActiveOperationCenterResources(t, db, fixture)
		assertWorkflowCounts(t, db, fixture, workflowCounts{serviceOrders: 1, serviceTransitions: 1})
		var status string
		if err := db.QueryRowContext(ctx, `SELECT status FROM xz_operation_center_service_orders WHERE id=$1`, paid.ServiceOrder.ID).Scan(&status); err != nil || status != string(OperationCenterServiceReviewRequired) {
			t.Fatalf("service status=%s err=%v", status, err)
		}
	})

	t.Run("reject creates one full refund task without activation", func(t *testing.T) {
		fixture := createWorkflowFixture(t, ctx, db, "reject")
		service := mustWorkflowService(t, db, WorkflowOptions{})
		paid, err := service.RecordPaymentSucceeded(ctx, PaymentSucceededCommand{OrderID: fixture.orderID, PaymentRecordID: fixture.paymentID})
		if err != nil {
			t.Fatal(err)
		}
		command := ReviewCommand{
			ServiceOrderID: paid.ServiceOrder.ID, Decision: ReviewRejected,
			ExpectedStatus: OperationCenterServiceReviewRequired, IdempotencyKey: fixture.id("reject-key"),
			ReviewedBy: fixture.reviewerID, RequestID: fixture.id("reject-request"), Reason: "rejected",
		}
		result, err := service.Review(ctx, command)
		if err != nil {
			t.Fatal(err)
		}
		if result.ServiceOrder.Status != OperationCenterServiceRejected || result.ServiceOrder.RefundStatus != OperationCenterRefundPending || result.RefundTask == nil {
			t.Fatalf("reject result=%+v", result)
		}
		if result.RefundTask.Scope != RefundScopeFull || result.RefundTask.Origin != RefundOriginReviewRejection || result.RefundTask.AmountCents != 500000 {
			t.Fatalf("refund task=%+v", result.RefundTask)
		}
		assertNoActiveOperationCenterResources(t, db, fixture)
		assertWorkflowCounts(t, db, fixture, workflowCounts{serviceOrders: 1, reviewEvents: 1, refundTasks: 1, serviceTransitions: 3})
		replay, err := service.Review(ctx, command)
		if err != nil || !replay.IdempotentReplay {
			t.Fatalf("reject replay=%+v err=%v", replay, err)
		}
		assertWorkflowCounts(t, db, fixture, workflowCounts{serviceOrders: 1, reviewEvents: 1, refundTasks: 1, serviceTransitions: 3})
	})
}

func TestOperationCenterWorkflowConcurrentReviewOnlyOneDecisionPostgres(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	db.SetMaxOpenConns(8)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fixture := createWorkflowFixture(t, ctx, db, "concurrent")
	service := mustWorkflowService(t, db, WorkflowOptions{})
	paid, err := service.RecordPaymentSucceeded(ctx, PaymentSucceededCommand{OrderID: fixture.orderID, PaymentRecordID: fixture.paymentID})
	if err != nil {
		t.Fatal(err)
	}
	commands := []ReviewCommand{
		{ServiceOrderID: paid.ServiceOrder.ID, Decision: ReviewApproved, ExpectedStatus: OperationCenterServiceReviewRequired, IdempotencyKey: fixture.id("concurrent-a"), ReviewedBy: fixture.reviewerID, RequestID: fixture.id("request-a")},
		{ServiceOrderID: paid.ServiceOrder.ID, Decision: ReviewRejected, ExpectedStatus: OperationCenterServiceReviewRequired, IdempotencyKey: fixture.id("concurrent-b"), ReviewedBy: fixture.reviewerID, RequestID: fixture.id("request-b")},
	}
	var successes atomic.Int32
	var failures atomic.Int32
	var wait sync.WaitGroup
	for _, command := range commands {
		wait.Add(1)
		go func(command ReviewCommand) {
			defer wait.Done()
			if _, reviewErr := service.Review(ctx, command); reviewErr == nil {
				successes.Add(1)
			} else if errors.Is(reviewErr, ErrInvalidServiceTransition) || errors.Is(reviewErr, ErrExpectedServiceStatus) {
				failures.Add(1)
			}
		}(command)
	}
	wait.Wait()
	if successes.Load() != 1 || failures.Load() != 1 {
		t.Fatalf("successes=%d failures=%d", successes.Load(), failures.Load())
	}
	var events int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_operation_center_review_events WHERE service_order_id=$1`, paid.ServiceOrder.ID).Scan(&events); err != nil || events != 1 {
		t.Fatalf("review events=%d err=%v", events, err)
	}
}

type workflowFixture struct {
	prefix, tenantID, userID, reviewerID, orderID, orderNo, paymentID, ruleSetID, planID, planVersionID, snapshotID string
}

func (fixture workflowFixture) id(value string) string { return fixture.prefix + "_" + value }

func createWorkflowFixture(t *testing.T, ctx context.Context, db *sql.DB, name string) workflowFixture {
	return createWorkflowFixtureWithRelationship(t, ctx, db, name, `{"referrerType":"NONE"}`)
}

func createWorkflowFixtureWithRelationship(t *testing.T, ctx context.Context, db *sql.DB, name, relationshipSnapshot string) workflowFixture {
	t.Helper()
	prefix := fmt.Sprintf("ocwf_%s_%d", name, time.Now().UnixNano())
	fixture := workflowFixture{
		prefix: prefix, tenantID: "tenant_default", userID: prefix + "_user", reviewerID: prefix + "_reviewer",
		orderID: prefix + "_order", orderNo: prefix + "_ORDER", paymentID: prefix + "_payment",
		ruleSetID: prefix + "_rules", planID: prefix + "_plan", planVersionID: prefix + "_plan_version", snapshotID: prefix + "_snapshot",
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	for _, userID := range []string{fixture.userID, fixture.reviewerID} {
		if _, err := db.ExecContext(ctx, `INSERT INTO xz_users(id,email,name,role,status,operation_center_status,created_at,updated_at,raw) VALUES($1,$2,$3,'USER','ACTIVE','NONE',$4,$4,'{}'::jsonb)`, userID, userID+"@example.test", userID, now.Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO xz_user_roles(user_id,tenant_id,organization_id,role,status) VALUES($1,'tenant_default','organization_default_'||substr(md5('tenant_default'),1,16),'USER','ACTIVE')`, userID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_commercial_rule_sets(id,tenant_id,rule_code,version,name,status,effective_start_at,published_by,published_at) VALUES($1,$2,$3,1,'Workflow rules','PUBLISHED',$4,$5,$4)`, fixture.ruleSetID, fixture.tenantID, prefix, now, fixture.reviewerID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_commercial_plan_versions(id,tenant_id,rule_set_id,plan_id,version,identity_type,price_cents,currency,config) VALUES($1,$2,$3,$4,1,'OPERATION_CENTER',500000,'CNY','{"rbacRole":"OPERATION"}'::jsonb)`, fixture.planVersionID, fixture.tenantID, fixture.ruleSetID, fixture.planID); err != nil {
		t.Fatal(err)
	}
	priceSnapshot := `{"priceCents":500000,"refundPolicySnapshot":{"mode":"FULL_ONLY"}}`
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_orders(id,order_no,tenant_id,user_id,plan_id,amount_cents,payable_amount_cents,currency,status,order_status,fulfillment_status,paid_at,created_at,price_snapshot,raw) VALUES($1,$2,$3,$4,$5,500000,500000,'CNY','PAID','PAID','PENDING',$6,$7,$8::jsonb,'{}'::jsonb)`, fixture.orderID, fixture.orderNo, fixture.tenantID, fixture.userID, fixture.planID, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), priceSnapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_commercial_order_rule_snapshots(id,tenant_id,order_id,order_no,source_user_id,plan_id,plan_version_id,rule_set_id,rule_set_version,scenario_code,paid_amount_cents,relationship_snapshot,commission_rule_snapshot,business_time) VALUES($1,$2,$3,$4,$5,$6,$7,$8,1,'OPERATION_CENTER_SERVICE',500000,$9::jsonb,'{"refundPolicy":"FULL_ONLY"}'::jsonb,$10)`, fixture.snapshotID, fixture.tenantID, fixture.orderID, fixture.orderNo, fixture.userID, fixture.planID, fixture.planVersionID, fixture.ruleSetID, relationshipSnapshot, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO xz_payment_records(id,payment_no,order_id,order_no,tenant_id,user_id,payment_channel,payment_scene,amount_cents,prepay_status,provider,currency,payment_status,paid_at) VALUES($1,$2,$3,$4,$5,$6,'MOCK','OPERATION_CENTER_SERVICE',500000,'SUCCESS','MOCK','CNY','SUCCESS',$7)`, fixture.paymentID, fixture.id("payment_no"), fixture.orderID, fixture.orderNo, fixture.tenantID, fixture.userID, now); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func mustWorkflowService(t *testing.T, db *sql.DB, options WorkflowOptions) *WorkflowService {
	t.Helper()
	service, err := NewWorkflowService(db, options)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

type workflowCounts struct{ serviceOrders, reviewEvents, refundTasks, serviceTransitions int }

func assertWorkflowCounts(t *testing.T, db *sql.DB, fixture workflowFixture, want workflowCounts) {
	t.Helper()
	queries := []struct {
		query string
		want  int
	}{
		{`SELECT count(*) FROM xz_operation_center_service_orders WHERE order_id=$1`, want.serviceOrders},
		{`SELECT count(*) FROM xz_operation_center_review_events WHERE service_order_id IN (SELECT id FROM xz_operation_center_service_orders WHERE order_id=$1)`, want.reviewEvents},
		{`SELECT count(*) FROM xz_operation_center_refund_tasks WHERE order_id=$1`, want.refundTasks},
		{`SELECT count(*) FROM xz_operation_center_state_transitions WHERE entity_id IN (SELECT id FROM xz_operation_center_service_orders WHERE order_id=$1 UNION SELECT id FROM xz_operation_center_refund_tasks WHERE order_id=$1)`, want.serviceTransitions},
	}
	for _, check := range queries {
		var got int
		if err := db.QueryRow(check.query, fixture.orderID).Scan(&got); err != nil || got != check.want {
			t.Fatalf("query=%s got=%d want=%d err=%v", check.query, got, check.want, err)
		}
	}
}

func assertNoActiveOperationCenterResources(t *testing.T, db *sql.DB, fixture workflowFixture) {
	t.Helper()
	queries := []string{
		`SELECT count(*) FROM xz_user_business_identities WHERE user_id=$1 AND identity_type='OPERATION_CENTER' AND identity_status='ACTIVE'`,
		`SELECT count(*) FROM xz_operation_centers WHERE user_id=$1 AND upper(coalesce(status,''))='ACTIVE'`,
		`SELECT count(*) FROM xz_user_roles WHERE user_id=$1 AND role='OPERATION' AND upper(status)='ACTIVE'`,
		`SELECT count(*) FROM xz_referral_events WHERE referred_operation_center_user_id=$1`,
		`SELECT count(*) FROM xz_referral_rewards WHERE beneficiary_user_id=$1`,
	}
	for _, query := range queries {
		var count int
		if err := db.QueryRow(query, fixture.userID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("unexpected resource query=%s count=%d err=%v", query, count, err)
		}
	}
}

func assertApprovedResources(t *testing.T, db *sql.DB, fixture workflowFixture) {
	t.Helper()
	queries := []struct {
		query string
		args  []any
	}{
		{`SELECT count(*) FROM xz_user_business_identities WHERE user_id=$1 AND identity_type='OPERATION_CENTER' AND identity_status='ACTIVE' AND commission_enabled=true`, []any{fixture.userID}},
		{`SELECT count(*) FROM xz_operation_centers WHERE user_id=$1 AND upper(status)='ACTIVE'`, []any{fixture.userID}},
		{`SELECT count(*) FROM xz_user_roles WHERE user_id=$1 AND role='OPERATION' AND upper(status)='ACTIVE'`, []any{fixture.userID}},
		{`SELECT count(*) FROM xz_users WHERE id=$1 AND operation_center_status='ACTIVE'`, []any{fixture.userID}},
		{`SELECT count(*) FROM xz_orders WHERE id=$1 AND fulfillment_status='FULFILLED' AND order_status='COMPLETED'`, []any{fixture.orderID}},
	}
	for _, check := range queries {
		var count int
		if err := db.QueryRow(check.query, check.args...).Scan(&count); err != nil || count != 1 {
			t.Fatalf("missing approved resource query=%s count=%d err=%v", check.query, count, err)
		}
	}
}

func assertNoReferralMoneyWrites(t *testing.T, db *sql.DB, fixture workflowFixture) {
	t.Helper()
	var events, rewards, ledger int
	if err := db.QueryRow(`SELECT count(*) FROM xz_referral_events WHERE referred_operation_center_user_id=$1`, fixture.userID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM xz_referral_rewards WHERE beneficiary_user_id=$1`, fixture.userID).Scan(&rewards); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM xz_commission_wallet_ledger WHERE beneficiary_id=$1`, fixture.userID).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if events != 0 || rewards != 0 || ledger != 0 {
		t.Fatalf("referral side effects events=%d rewards=%d ledger=%d", events, rewards, ledger)
	}
}

type recordingReferralTrigger struct{ calls atomic.Int32 }

func (trigger *recordingReferralTrigger) MarkEligible(context.Context, *sql.Tx, *OperationCenterServiceOrder) error {
	trigger.calls.Add(1)
	return nil
}

type recordingActivationHook struct{ calls atomic.Int32 }

func (hook *recordingActivationHook) AfterActivated(context.Context, *sql.Tx, *OperationCenterServiceOrder) error {
	hook.calls.Add(1)
	return nil
}

type failingActivationHook struct{}

func (failingActivationHook) AfterActivated(context.Context, *sql.Tx, *OperationCenterServiceOrder) error {
	return errors.New("activation hook failed")
}
