package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	operationcenter "xianzhi-ai/backend-go/internal/app/operationcenter"
)

func TestOperationCenterFinalPaymentEntryReviewAndRewardPostgres(t *testing.T) {
	dsn := os.Getenv("XIANZHI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}

	fixture := seedOperationCenterFinalEntryFixture(t, ctx, db)
	store := newPostgresPrimaryStore(db, "")
	callback := map[string]any{
		"eventId": "event_" + fixture.suffix, "providerTransactionId": "wx_tx_" + fixture.suffix,
		"paidAmountCents": 500000,
	}
	first, err := store.MarkAdminOrderPaid(fixture.orderID, callback)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != "PAID" || first.FulfillmentStatus != string(operationcenter.OperationCenterServiceReviewRequired) {
		t.Fatalf("payment entry status=%s fulfillment=%s", first.Status, first.FulfillmentStatus)
	}
	assertOperationCenterPaymentGate(t, ctx, db, fixture, 1, 1)

	replayed, err := store.MarkAdminOrderPaid(fixture.orderID, callback)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.FulfillmentStatus != string(operationcenter.OperationCenterServiceReviewRequired) {
		t.Fatalf("replayed fulfillment=%s", replayed.FulfillmentStatus)
	}
	assertOperationCenterPaymentGate(t, ctx, db, fixture, 1, 1)

	var serviceOrderID string
	if err := db.QueryRowContext(ctx, `SELECT id FROM xz_operation_center_service_orders WHERE order_id=$1`, fixture.orderID).Scan(&serviceOrderID); err != nil {
		t.Fatal(err)
	}
	workflow, err := operationcenter.NewWorkflowService(db, operationcenter.WorkflowOptions{})
	if err != nil {
		t.Fatal(err)
	}
	approved, err := workflow.Review(ctx, operationcenter.ReviewCommand{
		ServiceOrderID: serviceOrderID, Decision: operationcenter.ReviewApproved,
		ExpectedStatus: operationcenter.OperationCenterServiceReviewRequired,
		IdempotencyKey: "approve_" + fixture.suffix, ReviewedBy: fixture.reviewerID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if approved.ServiceOrder.Status != operationcenter.OperationCenterServiceActive {
		t.Fatalf("approved status=%s", approved.ServiceOrder.Status)
	}
	assertOperationCenterApprovedRewardChain(t, ctx, db, fixture)

	if _, err := workflow.Review(ctx, operationcenter.ReviewCommand{
		ServiceOrderID: serviceOrderID, Decision: operationcenter.ReviewApproved,
		ExpectedStatus: operationcenter.OperationCenterServiceReviewRequired,
		IdempotencyKey: "approve_" + fixture.suffix, ReviewedBy: fixture.reviewerID,
	}); err != nil {
		t.Fatal(err)
	}
	assertOperationCenterApprovedRewardChain(t, ctx, db, fixture)
}

type operationCenterFinalEntryFixture struct {
	suffix, tenantID, userID, reviewerID, beneficiaryID string
	orderID, orderNo, paymentID, ruleSetID              string
	planVersionID, snapshotID                           string
}

func seedOperationCenterFinalEntryFixture(t *testing.T, ctx context.Context, db *sql.DB) operationCenterFinalEntryFixture {
	t.Helper()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	prefix := "ocfinal_" + suffix
	fixture := operationCenterFinalEntryFixture{
		suffix: suffix, tenantID: "tenant_default", userID: prefix + "_user",
		reviewerID: prefix + "_reviewer", beneficiaryID: prefix + "_beneficiary",
		orderID: prefix + "_order", orderNo: "OCFINAL" + suffix, paymentID: prefix + "_payment",
		ruleSetID: prefix + "_rules", planVersionID: prefix + "_plan_version", snapshotID: prefix + "_snapshot",
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	for _, userID := range []string{fixture.userID, fixture.reviewerID, fixture.beneficiaryID} {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO xz_users(id,email,name,role,status,operation_center_status,created_at,updated_at,raw)
			VALUES($1,$2,$1,'USER','ACTIVE','NONE',$3,$3,'{}'::jsonb)
		`, userID, userID+"@example.test", now.Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_user_roles(user_id,tenant_id,organization_id,role,status)
		VALUES($1,$2,'organization_default_'||substr(md5($2),1,16),'USER','ACTIVE'),
		      ($3,$2,'organization_default_'||substr(md5($2),1,16),'USER','ACTIVE')
	`, fixture.userID, fixture.tenantID, fixture.reviewerID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_commercial_rule_sets(id,tenant_id,rule_code,version,name,status,effective_start_at,published_by,published_at)
		VALUES($1,$2,$3,1,'Final entry rules','PUBLISHED',$4,$5,$4)
	`, fixture.ruleSetID, fixture.tenantID, prefix, now, fixture.reviewerID); err != nil {
		t.Fatal(err)
	}
	planVersion := int(time.Now().UnixNano()%1000000000) + 1000
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_commercial_plan_versions(id,tenant_id,rule_set_id,plan_id,version,identity_type,price_cents,currency,config)
		VALUES($1,$2,$3,'plan_operation_center_5000',$4,'OPERATION_CENTER',500000,'CNY','{"rbacRole":"OPERATION"}'::jsonb)
	`, fixture.planVersionID, fixture.tenantID, fixture.ruleSetID, planVersion); err != nil {
		t.Fatal(err)
	}
	relationship := fmt.Sprintf(`{"referrerType":"OPERATION_CENTER","referrerUserId":"%s","referrerOperationCenterUserId":"%s"}`, fixture.beneficiaryID, fixture.beneficiaryID)
	priceSnapshot := map[string]any{"priceCents": 500000, "refundPolicySnapshot": map[string]any{"mode": "FULL_ONLY"}}
	rawOrder, err := json.Marshal(adminOrder{
		ID: fixture.orderID, OrderNo: fixture.orderNo, TenantID: fixture.tenantID,
		UserID: fixture.userID, BuyerUserID: fixture.userID, PlanID: "plan_operation_center_5000",
		Amount: 500000, AmountCents: 500000, Status: "PENDING", FulfillmentStatus: "PENDING",
		CreatedAt: now.Format(time.RFC3339Nano), PriceSnapshot: priceSnapshot,
	})
	if err != nil {
		t.Fatal(err)
	}
	priceSnapshotJSON, err := json.Marshal(priceSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_orders(
		  id,order_no,tenant_id,user_id,plan_id,amount_cents,payable_amount_cents,currency,
		  status,order_status,fulfillment_status,created_at,price_snapshot,raw
		) VALUES($1,$2,$3,$4,'plan_operation_center_5000',500000,500000,'CNY',
		         'PENDING','PENDING','PENDING',$5,$6::jsonb,$7::jsonb)
	`, fixture.orderID, fixture.orderNo, fixture.tenantID, fixture.userID,
		now.Format(time.RFC3339Nano), string(priceSnapshotJSON), string(rawOrder)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_commercial_order_rule_snapshots(
		  id,tenant_id,order_id,order_no,source_user_id,plan_id,plan_version_id,rule_set_id,
		  rule_set_version,scenario_code,paid_amount_cents,relationship_snapshot,commission_rule_snapshot,business_time
		) VALUES($1,$2,$3,$4,$5,'plan_operation_center_5000',$6,$7,1,
		         'OPERATION_CENTER_SERVICE',500000,$8::jsonb,'{"refundPolicy":"FULL_ONLY"}'::jsonb,$9)
	`, fixture.snapshotID, fixture.tenantID, fixture.orderID, fixture.orderNo, fixture.userID,
		fixture.planVersionID, fixture.ruleSetID, relationship, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_payment_records(
		  id,payment_no,order_id,order_no,tenant_id,user_id,payment_channel,payment_scene,
		  amount_cents,prepay_status,provider,currency,payment_status
		) VALUES($1,$2,$3,$4,$5,$6,'WECHAT_VIRTUAL','OPERATION_CENTER_SERVICE',
		         500000,'SIGNED','WECHAT_VIRTUAL','CNY','PENDING')
	`, fixture.paymentID, prefix+"_payment_no", fixture.orderID, fixture.orderNo, fixture.tenantID, fixture.userID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_referral_reward_rule_versions(
		  id,tenant_id,rule_set_id,rule_code,version,referrer_type,beneficiary_type,
		  beneficiary_relation,amount_cents,freeze_days,status
		) VALUES($1,$2,$3,$4,1,'OPERATION_CENTER','OPERATION_CENTER','REFERRER',300000,7,'PUBLISHED')
	`, prefix+"_reward_rule", fixture.tenantID, fixture.ruleSetID, prefix+"_reward"); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func assertOperationCenterPaymentGate(t *testing.T, ctx context.Context, db *sql.DB, fixture operationCenterFinalEntryFixture, services, transitions int) {
	t.Helper()
	var orderStatus, fulfillmentStatus, paymentStatus, serviceStatus, refundStatus string
	if err := db.QueryRowContext(ctx, `SELECT order_status,fulfillment_status FROM xz_orders WHERE id=$1`, fixture.orderID).Scan(&orderStatus, &fulfillmentStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT payment_status FROM xz_payment_records WHERE id=$1`, fixture.paymentID).Scan(&paymentStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT status,refund_status FROM xz_operation_center_service_orders WHERE order_id=$1`, fixture.orderID).Scan(&serviceStatus, &refundStatus); err != nil {
		t.Fatal(err)
	}
	if orderStatus != "PAID" || fulfillmentStatus != "REVIEW_REQUIRED" || paymentStatus != "SUCCESS" ||
		serviceStatus != "REVIEW_REQUIRED" || refundStatus != "NONE" {
		t.Fatalf("payment gate order=%s/%s payment=%s service=%s refund=%s", orderStatus, fulfillmentStatus, paymentStatus, serviceStatus, refundStatus)
	}
	checks := []struct {
		query string
		want  int
	}{
		{`SELECT count(*) FROM xz_operation_center_service_orders WHERE order_id=$1`, services},
		{`SELECT count(*) FROM xz_operation_center_state_transitions WHERE entity_id IN (SELECT id FROM xz_operation_center_service_orders WHERE order_id=$1)`, transitions},
		{`SELECT count(*) FROM xz_user_business_identities WHERE source_order_id=$1 AND identity_type='OPERATION_CENTER'`, 0},
		{`SELECT count(*) FROM xz_operation_centers WHERE user_id=(SELECT user_id FROM xz_orders WHERE id=$1)`, 0},
		{`SELECT count(*) FROM xz_user_roles WHERE user_id=(SELECT user_id FROM xz_orders WHERE id=$1) AND role='OPERATION' AND status='ACTIVE'`, 0},
		{`SELECT count(*) FROM xz_referral_events WHERE source_order_id=$1`, 0},
		{`SELECT count(*) FROM xz_referral_rewards reward JOIN xz_referral_events event ON event.id=reward.referral_event_id WHERE event.source_order_id=$1`, 0},
		{`SELECT count(*) FROM xz_order_settlement_engine_decisions WHERE order_id=$1`, 0},
		{`SELECT count(*) FROM xz_order_settlement_write_sources WHERE order_id=$1`, 0},
		{`SELECT count(*) FROM xz_commissions WHERE order_id=$1`, 0},
	}
	for _, check := range checks {
		var count int
		if err := db.QueryRowContext(ctx, check.query, fixture.orderID).Scan(&count); err != nil || count != check.want {
			t.Fatalf("gate query=%s count=%d want=%d err=%v", check.query, count, check.want, err)
		}
	}
}

func assertOperationCenterApprovedRewardChain(t *testing.T, ctx context.Context, db *sql.DB, fixture operationCenterFinalEntryFixture) {
	t.Helper()
	checks := []struct {
		query string
		args  []any
		want  int
	}{
		{`SELECT count(*) FROM xz_operation_center_service_orders WHERE order_id=$1 AND status='ACTIVE'`, []any{fixture.orderID}, 1},
		{`SELECT count(*) FROM xz_user_business_identities WHERE source_order_id=$1 AND identity_type='OPERATION_CENTER' AND identity_status='ACTIVE'`, []any{fixture.orderID}, 1},
		{`SELECT count(*) FROM xz_operation_centers WHERE user_id=$1 AND status='ACTIVE'`, []any{fixture.userID}, 1},
		{`SELECT count(*) FROM xz_user_roles WHERE user_id=$1 AND role='OPERATION' AND status='ACTIVE'`, []any{fixture.userID}, 1},
		{`SELECT count(*) FROM xz_referral_events WHERE source_order_id=$1`, []any{fixture.orderID}, 1},
		{`SELECT count(*) FROM xz_referral_eligibilities WHERE referral_event_id IN (SELECT id FROM xz_referral_events WHERE source_order_id=$1)`, []any{fixture.orderID}, 1},
		{`SELECT count(*) FROM xz_referral_rewards WHERE referral_event_id IN (SELECT id FROM xz_referral_events WHERE source_order_id=$1) AND status='FROZEN' AND amount_cents=300000`, []any{fixture.orderID}, 1},
		{`SELECT count(*) FROM xz_commission_wallet_ledger WHERE referral_event_id IN (SELECT id FROM xz_referral_events WHERE source_order_id=$1) AND frozen_delta_cents=300000`, []any{fixture.orderID}, 1},
		{`SELECT count(*) FROM xz_referral_reward_release_tasks WHERE referral_reward_id IN (SELECT id FROM xz_referral_rewards WHERE referral_event_id IN (SELECT id FROM xz_referral_events WHERE source_order_id=$1))`, []any{fixture.orderID}, 1},
		{`SELECT count(*) FROM xz_commissions WHERE order_id=$1`, []any{fixture.orderID}, 0},
	}
	for _, check := range checks {
		var count int
		if err := db.QueryRowContext(ctx, check.query, check.args...).Scan(&count); err != nil || count != check.want {
			t.Fatalf("approved query=%s count=%d want=%d err=%v", check.query, count, check.want, err)
		}
	}
	var frozen, available, settled, recoverable int64
	if err := db.QueryRowContext(ctx, `
		SELECT frozen_cents,available_cents,settled_cents,recoverable_cents
		FROM xz_commission_wallet_accounts
		WHERE tenant_id=$1 AND beneficiary_id=$2
	`, fixture.tenantID, fixture.beneficiaryID).Scan(&frozen, &available, &settled, &recoverable); err != nil {
		t.Fatal(err)
	}
	if frozen != 300000 || available != 0 || settled != 0 || recoverable != 0 {
		t.Fatalf("wallet buckets frozen=%d available=%d settled=%d recoverable=%d", frozen, available, settled, recoverable)
	}
}
