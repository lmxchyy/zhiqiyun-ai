package operationcenter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresStoreServiceOrderReviewRefundManualReleaseAuditAndRollback(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	store, err := NewPostgresStore(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	fixture := createOperationCenterStoreFixture(t, ctx, db, "domain_store")

	t.Run("service order create read lock update and payment lookup", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		txStore, err := store.BindTx(tx)
		if err != nil {
			t.Fatal(err)
		}
		item := fixture.serviceOrder("service_primary", "order_primary", "ORDER-PRIMARY")
		if err := txStore.CreateServiceOrder(ctx, &item); err != nil {
			t.Fatal(err)
		}
		got, err := txStore.GetServiceOrderForUpdate(ctx, item.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != OperationCenterServicePendingPayment || got.ReviewedAt != nil || got.CurrentRefundTaskID != nil {
			t.Fatalf("nullable service fields were not preserved: %+v", got)
		}
		if got.RelationshipSnapshot["operationCenterId"] != fixture.operationCenterID {
			t.Fatalf("relationship JSON was not restored: %#v", got.RelationshipSnapshot)
		}

		now := fixture.now.Add(time.Minute)
		got.Status = OperationCenterServiceReviewRequired
		got.PaidAt = &now
		got.StateVersion++
		got.UpdatedAt = now
		transition := fixture.transition("transition_service_review", StateEntityServiceOrder, item.ID, string(OperationCenterServicePendingPayment), string(OperationCenterServiceReviewRequired))
		if err := txStore.UpdateServiceOrder(ctx, got, &transition); err != nil {
			t.Fatal(err)
		}
		byOrder, err := txStore.FindServiceOrderByCommercialOrderID(ctx, item.OrderID)
		if err != nil || byOrder.ID != item.ID {
			t.Fatalf("find by order: item=%+v err=%v", byOrder, err)
		}
		byPayment, err := txStore.FindServiceOrderByPaymentRecordID(ctx, fixture.paymentRecordID)
		if err != nil || byPayment.ID != item.ID {
			t.Fatalf("find by payment: item=%+v err=%v", byPayment, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}

		var auditCount int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_operation_center_state_transitions WHERE id=$1`, transition.ID).Scan(&auditCount); err != nil {
			t.Fatal(err)
		}
		if auditCount != 1 {
			t.Fatalf("service state update must append one audit row, got %d", auditCount)
		}
	})

	t.Run("review event idempotency", func(t *testing.T) {
		tx, txStore := beginOperationCenterStoreTestTx(t, ctx, db, store)
		defer tx.Rollback()
		event := fixture.reviewEvent("review_event_1", "service_primary", "review-key")
		if err := txStore.CreateReviewEvent(ctx, &event); err != nil {
			t.Fatal(err)
		}
		duplicate := event
		duplicate.ID = fixture.id("review_event_2")
		if err := txStore.CreateReviewEvent(ctx, &duplicate); !errors.Is(err, ErrIdempotencyConflict) {
			t.Fatalf("duplicate review error=%v", err)
		}
		got, err := txStore.GetReviewEventByIdempotencyKey(ctx, fixture.tenantID, event.IdempotencyKey)
		if err != nil || got.ID != event.ID {
			t.Fatalf("get review event: got=%+v err=%v", got, err)
		}
	})

	t.Run("refund task idempotency full scope lock and errors", func(t *testing.T) {
		tx, txStore := beginOperationCenterStoreTestTx(t, ctx, db, store)
		defer tx.Rollback()
		task := fixture.refundTask("refund_1", "service_primary", "order_primary", "refund-key")
		if err := txStore.CreateRefundTask(ctx, &task); err != nil {
			t.Fatal(err)
		}
		duplicateKey := task
		duplicateKey.ID = fixture.id("refund_duplicate_key")
		duplicateKey.ServiceOrderID = fixture.id("missing_service_for_idempotency")
		if err := txStore.CreateRefundTask(ctx, &duplicateKey); !errors.Is(err, ErrIdempotencyConflict) {
			t.Fatalf("duplicate refund key error=%v", err)
		}
		duplicateScope := task
		duplicateScope.ID = fixture.id("refund_duplicate_scope")
		duplicateScope.IdempotencyKey = fixture.id("refund-key-2")
		if err := txStore.CreateRefundTask(ctx, &duplicateScope); !errors.Is(err, ErrUniqueConflict) {
			t.Fatalf("duplicate full refund scope error=%v", err)
		}
		got, err := txStore.GetRefundTaskForUpdate(ctx, task.ID)
		if err != nil || got.ID != task.ID {
			t.Fatalf("lock refund: got=%+v err=%v", got, err)
		}
		byKey, err := txStore.GetRefundTaskByIdempotencyKey(ctx, task.IdempotencyKey)
		if err != nil || byKey.ID != task.ID {
			t.Fatalf("get refund by key: got=%+v err=%v", byKey, err)
		}

		badFK := task
		badFK.ID = fixture.id("refund_bad_fk")
		badFK.ServiceOrderID = fixture.id("service_missing")
		badFK.IdempotencyKey = fixture.id("refund-bad-fk-key")
		if err := txStore.CreateRefundTask(ctx, &badFK); !errors.Is(err, ErrForeignKeyConflict) {
			t.Fatalf("foreign key error=%v", err)
		}
		badStatus := task
		badStatus.ID = fixture.id("refund_bad_status")
		badStatus.ServiceOrderID = fixture.id("service_missing_status")
		badStatus.IdempotencyKey = fixture.id("refund-bad-status-key")
		badStatus.Status = OperationCenterRefundStatus("INVALID")
		if err := txStore.CreateRefundTask(ctx, &badStatus); !errors.Is(err, ErrConstraintViolation) {
			t.Fatalf("constraint error=%v", err)
		}
	})

	t.Run("manual refund submit approve and self approval rejection", func(t *testing.T) {
		service := fixture.serviceOrder("service_manual", "order_manual", "ORDER-MANUAL")
		task := fixture.refundTask("refund_manual", "service_manual", "order_manual", "refund-manual-key")
		seedOperationCenterServiceAndRefund(t, ctx, db, store, service, task)

		tx, txStore := beginOperationCenterStoreTestTx(t, ctx, db, store)
		defer tx.Rollback()
		manual := fixture.manualRefund("manual_1", task.ID)
		if err := txStore.SubmitManualRefund(ctx, &manual); err != nil {
			t.Fatal(err)
		}
		transition := fixture.transition("transition_manual", StateEntityRefundTask, task.ID, string(OperationCenterRefundManualRequired), string(OperationCenterRefundManualSubmitted))
		if _, err := txStore.ApproveManualRefund(ctx, manual.ID, manual.SubmittedBy, fixture.now.Add(time.Minute), transition); !errors.Is(err, ErrManualRefundSelfApproval) {
			t.Fatalf("self approval error=%v", err)
		}
		approved, err := txStore.ApproveManualRefund(ctx, manual.ID, fixture.approverID, fixture.now.Add(2*time.Minute), transition)
		if err != nil {
			t.Fatal(err)
		}
		if approved.Status != ManualRefundApproved || approved.ApprovedBy == nil || *approved.ApprovedBy != fixture.approverID {
			t.Fatalf("manual approval not persisted: %+v", approved)
		}
	})

	t.Run("reward release task idempotency", func(t *testing.T) {
		rewardID := fixture.createFrozenReferralReward(t, ctx, db, "release_reward")
		tx, txStore := beginOperationCenterStoreTestTx(t, ctx, db, store)
		defer tx.Rollback()
		task := fixture.releaseTask("release_task_1", rewardID, "release-key")
		if err := txStore.CreateRewardReleaseTask(ctx, &task); err != nil {
			t.Fatal(err)
		}
		duplicate := task
		duplicate.ID = fixture.id("release_task_2")
		if err := txStore.CreateRewardReleaseTask(ctx, &duplicate); !errors.Is(err, ErrIdempotencyConflict) {
			t.Fatalf("duplicate release task error=%v", err)
		}
		got, err := txStore.GetRewardReleaseTaskByIdempotencyKey(ctx, fixture.tenantID, task.IdempotencyKey)
		if err != nil || got.ID != task.ID {
			t.Fatalf("get release by key: got=%+v err=%v", got, err)
		}
	})

	t.Run("transaction rollback leaves no service or audit", func(t *testing.T) {
		tx, txStore := beginOperationCenterStoreTestTx(t, ctx, db, store)
		item := fixture.serviceOrder("service_rollback", "order_rollback", "ORDER-ROLLBACK")
		if err := txStore.CreateServiceOrder(ctx, &item); err != nil {
			t.Fatal(err)
		}
		transition := fixture.transition("transition_rollback", StateEntityServiceOrder, item.ID, "", string(OperationCenterServicePendingPayment))
		if err := txStore.AppendStateTransition(ctx, &transition); err != nil {
			t.Fatal(err)
		}
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
		var serviceCount, auditCount int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_operation_center_service_orders WHERE id=$1`, item.ID).Scan(&serviceCount); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_operation_center_state_transitions WHERE id=$1`, transition.ID).Scan(&auditCount); err != nil {
			t.Fatal(err)
		}
		if serviceCount != 0 || auditCount != 0 {
			t.Fatalf("rollback leaked service=%d audit=%d", serviceCount, auditCount)
		}
	})
}

func TestPostgresStoreRefundAndReleaseTaskClaimsUseSkipLockedAndExpiredLeases(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	store, err := NewPostgresStore(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fixture := createOperationCenterStoreFixture(t, ctx, db, "domain_claim")
	service := fixture.serviceOrder("service_claim", "order_claim", "ORDER-CLAIM")
	task := fixture.refundTask("refund_claim", "service_claim", "order_claim", "refund-claim-key")
	prepared := fixture.now
	task.Status = OperationCenterRefundProviderPending
	task.PreparedAt = &prepared
	seedOperationCenterServiceAndRefund(t, ctx, db, store, service, task)

	tx1, txStore1 := beginOperationCenterStoreTestTx(t, ctx, db, store)
	claimed1, err := txStore1.ClaimRefundTasks(ctx, fixture.now, "worker-1", fixture.now.Add(5*time.Minute), 1)
	if err != nil || len(claimed1) != 1 {
		t.Fatalf("first refund claim got=%d err=%v", len(claimed1), err)
	}

	tx2, txStore2 := beginOperationCenterStoreTestTx(t, ctx, db, store)
	claimed2, err := txStore2.ClaimRefundTasks(ctx, fixture.now, "worker-2", fixture.now.Add(5*time.Minute), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed2) != 0 {
		t.Fatalf("SKIP LOCKED allowed duplicate claim: %+v", claimed2)
	}
	_ = tx2.Rollback()
	if err := tx1.Commit(); err != nil {
		t.Fatal(err)
	}

	if _, err := db.ExecContext(ctx, `UPDATE xz_operation_center_refund_tasks SET lease_expires_at=$2 WHERE id=$1`, task.ID, fixture.now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	tx3, txStore3 := beginOperationCenterStoreTestTx(t, ctx, db, store)
	claimed3, err := txStore3.ClaimRefundTasks(ctx, fixture.now, "worker-3", fixture.now.Add(5*time.Minute), 1)
	if err != nil || len(claimed3) != 1 || claimed3[0].ID != task.ID {
		t.Fatalf("expired lease not reclaimed: %+v err=%v", claimed3, err)
	}
	_ = tx3.Rollback()

	rewardID := fixture.createFrozenReferralReward(t, ctx, db, "claim_release_reward")
	release := fixture.releaseTask("release_claim", rewardID, "release-claim-key")
	tx4, txStore4 := beginOperationCenterStoreTestTx(t, ctx, db, store)
	if err := txStore4.CreateRewardReleaseTask(ctx, &release); err != nil {
		t.Fatal(err)
	}
	if err := tx4.Commit(); err != nil {
		t.Fatal(err)
	}
	tx5, txStore5 := beginOperationCenterStoreTestTx(t, ctx, db, store)
	releases, err := txStore5.ClaimDueRewardReleaseTasks(ctx, fixture.now, "release-worker", fixture.now.Add(5*time.Minute), 1)
	if err != nil || len(releases) != 1 || releases[0].ID != release.ID {
		t.Fatalf("due release claim: %+v err=%v", releases, err)
	}
	_ = tx5.Rollback()
	if _, err := db.ExecContext(ctx, `UPDATE xz_referral_reward_release_tasks SET release_status='CANCELLED',lease_owner=NULL,lease_expires_at=NULL,cancellation_reason='TEST_CLEANUP',cancelled_at=now(),completed_at=now() WHERE id=$1`, release.ID); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStoreDomainStatusesMatchMigration089Checks(t *testing.T) {
	db := openOperationCenterStoreTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	assertConstraintContainsStatuses(t, ctx, db, "xz_operation_center_service_orders", "ck_xz_oc_service_orders_service_status_089", serviceStatusStrings(DatabaseOperationCenterServiceStatuses()))
	assertConstraintContainsStatuses(t, ctx, db, "xz_operation_center_service_orders", "ck_xz_oc_service_orders_refund_status_089", refundStatusStrings(DatabaseOperationCenterRefundStatuses()))
}

type operationCenterStoreFixture struct {
	prefix            string
	tenantID          string
	applicantID       string
	reviewerID        string
	approverID        string
	operationCenterID string
	ruleSetID         string
	planVersionID     string
	paymentRecordID   string
	now               time.Time
}

func createOperationCenterStoreFixture(t *testing.T, ctx context.Context, db *sql.DB, prefix string) operationCenterStoreFixture {
	t.Helper()
	suffix := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	fixture := operationCenterStoreFixture{
		prefix: suffix, tenantID: "tenant_default", now: time.Now().UTC().Truncate(time.Microsecond),
		applicantID: suffix + "_applicant", reviewerID: suffix + "_reviewer", approverID: suffix + "_approver",
		operationCenterID: suffix + "_oc", paymentRecordID: suffix + "_payment",
	}
	if err := db.QueryRowContext(ctx, `SELECT id FROM xz_commercial_rule_sets ORDER BY version DESC,id LIMIT 1`).Scan(&fixture.ruleSetID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT id FROM xz_commercial_plan_versions WHERE rule_set_id=$1 ORDER BY version DESC,id LIMIT 1`, fixture.ruleSetID).Scan(&fixture.planVersionID); err != nil {
		t.Fatal(err)
	}
	for _, userID := range []string{fixture.applicantID, fixture.reviewerID, fixture.approverID} {
		if _, err := db.ExecContext(ctx, `INSERT INTO xz_users(id) VALUES($1)`, userID); err != nil {
			t.Fatal(err)
		}
	}
	orders := []struct{ id, no string }{
		{fixture.id("order_primary"), "ORDER-PRIMARY"},
		{fixture.id("order_manual"), "ORDER-MANUAL"},
		{fixture.id("order_rollback"), "ORDER-ROLLBACK"},
		{fixture.id("order_claim"), "ORDER-CLAIM"},
	}
	for _, order := range orders {
		if _, err := db.ExecContext(ctx, `INSERT INTO xz_orders(id,user_id,order_no,amount_cents) VALUES($1,$2,$3,500000)`, order.id, fixture.applicantID, fixture.prefix+"_"+order.no); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_payment_records(
		  id,payment_no,order_id,order_no,tenant_id,user_id,payment_channel,payment_scene,amount_cents
		) VALUES($1,$2,$3,$4,$5,$6,'MOCK','OPERATION_CENTER_SERVICE',500000)
	`, fixture.paymentRecordID, fixture.id("payment_no"), fixture.id("order_primary"), fixture.prefix+"_ORDER-PRIMARY", fixture.tenantID, fixture.applicantID); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func (f operationCenterStoreFixture) id(value string) string { return f.prefix + "_" + value }

func (f operationCenterStoreFixture) serviceOrder(id, orderID, orderNo string) OperationCenterServiceOrder {
	ruleVersion := 1
	return OperationCenterServiceOrder{
		ID: f.id(id), TenantID: f.tenantID, OrderID: f.id(orderID), OrderNo: f.prefix + "_" + orderNo,
		ApplicantUserID: f.applicantID, TechnicalServiceFeeCents: 500000, Currency: "CNY",
		Status: OperationCenterServicePendingPayment, RefundStatus: OperationCenterRefundNone,
		CommercialRuleSetID: &f.ruleSetID, CommercialRuleSetVersion: &ruleVersion, PlanVersionID: &f.planVersionID,
		RelationshipSnapshot: JSONSnapshot{"operationCenterId": f.operationCenterID},
		RefundPolicySnapshot: JSONSnapshot{"mode": "FULL_ONLY"}, RefundFailureDetail: JSONSnapshot{}, Metadata: JSONSnapshot{},
		CreatedAt: f.now, UpdatedAt: f.now,
	}
}

func (f operationCenterStoreFixture) reviewEvent(id, serviceID, key string) OperationCenterReviewEvent {
	return OperationCenterReviewEvent{
		ID: f.id(id), TenantID: f.tenantID, ServiceOrderID: f.id(serviceID), Decision: ReviewApproved,
		Status: ReviewEventPending, ReviewedBy: f.reviewerID, IdempotencyKey: f.id(key),
		FailureDetail: JSONSnapshot{}, EventSnapshot: JSONSnapshot{"source": "test"}, CreatedAt: f.now, UpdatedAt: f.now,
	}
}

func (f operationCenterStoreFixture) refundTask(id, serviceID, orderID, key string) OperationCenterRefundTask {
	return OperationCenterRefundTask{
		ID: f.id(id), TenantID: f.tenantID, ServiceOrderID: f.id(serviceID), OrderID: f.id(orderID),
		PaymentRecordID: &f.paymentRecordID, CommercialRuleSetID: f.ruleSetID, Origin: RefundOriginReviewRejection,
		Scope: RefundScopeFull, AmountCents: 500000, Currency: "CNY", PaymentChannel: "MOCK",
		Status: OperationCenterRefundPending, FailureDetail: JSONSnapshot{}, IdempotencyKey: f.id(key),
		CreatedAt: f.now, UpdatedAt: f.now,
	}
}

func (f operationCenterStoreFixture) manualRefund(id, refundTaskID string) OperationCenterManualRefund {
	return OperationCenterManualRefund{
		ID: f.id(id), TenantID: f.tenantID, RefundTaskID: refundTaskID, PaymentChannel: "MOCK",
		AmountCents: 500000, Currency: "CNY", ProviderTransactionID: f.id("manual_provider_tx"),
		VoucherReference: f.id("voucher"), VoucherFileHash: strings.Repeat("a", 64), Status: ManualRefundSubmitted,
		SubmittedBy: f.reviewerID, SubmittedAt: f.now, CreatedAt: f.now, UpdatedAt: f.now,
	}
}

func (f operationCenterStoreFixture) releaseTask(id, rewardID, key string) ReferralRewardReleaseTask {
	return ReferralRewardReleaseTask{
		ID: f.id(id), TenantID: f.tenantID, ReferralRewardID: rewardID, IdempotencyKey: f.id(key),
		Status: ReferralRewardReleasePending, ExecuteAt: f.now, FailureDetail: JSONSnapshot{}, CreatedAt: f.now, UpdatedAt: f.now,
	}
}

func (f operationCenterStoreFixture) transition(id string, entity StateTransitionEntityType, entityID, from, to string) OperationCenterStateTransition {
	return OperationCenterStateTransition{
		ID: f.id(id), TenantID: f.tenantID, EntityType: entity, EntityID: entityID,
		FromStatus: stringPointer(from), ToStatus: to, TransitionReason: "integration_test",
		TransactionGroupID: f.id("tx_group"), OperatorID: &f.reviewerID, IdempotencyKey: f.id(id + "_key"),
		Metadata: JSONSnapshot{"test": true}, CreatedAt: f.now,
	}
}

func (f operationCenterStoreFixture) createFrozenReferralReward(t *testing.T, ctx context.Context, db *sql.DB, name string) string {
	t.Helper()
	var ruleID string
	if err := db.QueryRowContext(ctx, `SELECT id FROM xz_referral_reward_rule_versions WHERE rule_set_id=$1 ORDER BY id LIMIT 1`, f.ruleSetID).Scan(&ruleID); err != nil {
		t.Fatal(err)
	}
	eventID := f.id(name + "_event")
	rewardID := f.id(name)
	orderID := f.id("order_primary")
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_referral_events(
		  id,tenant_id,referred_operation_center_user_id,referrer_type,referrer_user_id,
		  source_order_id,source_order_no,payment_status_snapshot,review_status_snapshot,
		  operation_center_status_snapshot,relationship_snapshot,triggered_at,status,idempotency_key
		) VALUES($1,$2,$3,'OPERATION_CENTER',$4,$5,$6,'PAID','APPROVED','ACTIVE','{}'::jsonb,$7,'REWARDED',$8)
	`, eventID, f.tenantID, f.applicantID, f.reviewerID, orderID, f.id(name+"_order_no"), f.now, f.id(name+"_event_key")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO xz_referral_rewards(
		  id,tenant_id,referral_event_id,reward_rule_id,beneficiary_type,beneficiary_user_id,
		  amount_cents,record_type,status,freeze_until,idempotency_key,commercial_rule_set_id
		) VALUES($1,$2,$3,$4,'OPERATION_CENTER',$5,100,'REWARD','FROZEN',$6,$7,$8)
	`, rewardID, f.tenantID, eventID, ruleID, f.applicantID, f.now.Add(-time.Minute), f.id(name+"_reward_key"), f.ruleSetID); err != nil {
		t.Fatal(err)
	}
	return rewardID
}

func seedOperationCenterServiceAndRefund(t *testing.T, ctx context.Context, db *sql.DB, store *PostgresStore, service OperationCenterServiceOrder, task OperationCenterRefundTask) {
	t.Helper()
	tx, txStore := beginOperationCenterStoreTestTx(t, ctx, db, store)
	if err := txStore.CreateServiceOrder(ctx, &service); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := txStore.CreateRefundTask(ctx, &task); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func beginOperationCenterStoreTestTx(t *testing.T, ctx context.Context, db *sql.DB, store *PostgresStore) (*sql.Tx, Store) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	txStore, err := store.BindTx(tx)
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	return tx, txStore
}

func openOperationCenterStoreTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("XIANZHI_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("XIANZHI_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	return db
}

func assertConstraintContainsStatuses(t *testing.T, ctx context.Context, db *sql.DB, table, constraint string, statuses []string) {
	t.Helper()
	var definition string
	if err := db.QueryRowContext(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid=$1::regclass AND conname=$2`, table, constraint).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	for _, status := range statuses {
		if !strings.Contains(definition, "'"+status+"'") {
			t.Fatalf("constraint %s is missing domain status %s: %s", constraint, status, definition)
		}
	}
}

func serviceStatusStrings(values []OperationCenterServiceStatus) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, string(value))
	}
	return result
}

func refundStatusStrings(values []OperationCenterRefundStatus) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, string(value))
	}
	return result
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
