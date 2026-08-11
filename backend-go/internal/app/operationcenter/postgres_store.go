package operationcenter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"xianzhi-ai/backend-go/internal/app/payment"
)

type sqlRunner interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type rowScanner interface {
	Scan(...any) error
}

type PostgresStore struct {
	db     *sql.DB
	tx     *sql.Tx
	runner sqlRunner
}

var operationCenterSavepointSequence atomic.Uint64

func NewPostgresStore(db *sql.DB) (*PostgresStore, error) {
	if db == nil {
		return nil, fmt.Errorf("create operation center store: nil database")
	}
	return &PostgresStore{db: db, runner: db}, nil
}

func (store *PostgresStore) BindTx(tx *sql.Tx) (Store, error) {
	if tx == nil {
		return nil, fmt.Errorf("bind operation center store: nil transaction")
	}
	return &PostgresStore{db: store.db, tx: tx, runner: tx}, nil
}

func (store *PostgresStore) requireTx() error {
	if store.tx == nil {
		return ErrTransactionRequired
	}
	return nil
}

func (store *PostgresStore) mutate(ctx context.Context, operation string, action func() error) error {
	if store.tx == nil {
		return mapPostgresStoreError(operation, action())
	}
	name := fmt.Sprintf("oc_store_%d", operationCenterSavepointSequence.Add(1))
	if _, err := store.tx.ExecContext(ctx, "SAVEPOINT "+name); err != nil {
		return fmt.Errorf("%s create savepoint: %w", operation, err)
	}
	if err := action(); err != nil {
		mapped := mapPostgresStoreError(operation, err)
		if _, rollbackErr := store.tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+name); rollbackErr != nil {
			return fmt.Errorf("%s rollback savepoint after %v: %w", operation, mapped, rollbackErr)
		}
		if _, releaseErr := store.tx.ExecContext(ctx, "RELEASE SAVEPOINT "+name); releaseErr != nil {
			return fmt.Errorf("%s release rolled back savepoint after %v: %w", operation, mapped, releaseErr)
		}
		return mapped
	}
	if _, err := store.tx.ExecContext(ctx, "RELEASE SAVEPOINT "+name); err != nil {
		return fmt.Errorf("%s release savepoint: %w", operation, err)
	}
	return nil
}

func mapPostgresStoreError(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%s: %w", operation, ErrNotFound)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return fmt.Errorf("%s: %w", operation, err)
	}
	switch pgErr.Code {
	case "23505":
		if strings.Contains(strings.ToLower(pgErr.ConstraintName), "idempotency") {
			return fmt.Errorf("%s: %w", operation, ErrIdempotencyConflict)
		}
		return fmt.Errorf("%s: %w", operation, ErrUniqueConflict)
	case "23503":
		return fmt.Errorf("%s: %w", operation, ErrForeignKeyConflict)
	case "23514", "P0001":
		return fmt.Errorf("%s: %w", operation, ErrConstraintViolation)
	default:
		return fmt.Errorf("%s: postgres %s: %w", operation, pgErr.Code, err)
	}
}

const serviceOrderColumns = `
id, tenant_id, order_id, order_no, applicant_user_id,
technical_service_fee_cents, currency, status, paid_at, reviewed_at,
reviewed_by, activated_at, revoked_at, refund_order_id, state_version,
metadata, created_at, updated_at, refund_status, commercial_rule_set_id,
commercial_rule_set_version, plan_version_id, commercial_order_snapshot_id,
relationship_snapshot, refund_policy_snapshot, review_idempotency_key,
refund_idempotency_key, payment_channel, provider_refund_no, refund_failure_class,
refund_failure_detail, refund_attempt_count, next_refund_retry_at,
manual_refund_voucher_reference, manual_refund_voucher_file_hash,
manual_refund_submitted_by, manual_refund_approved_by, current_refund_task_id`

func (store *PostgresStore) CreateServiceOrder(ctx context.Context, item *OperationCenterServiceOrder) error {
	if item == nil || !isDatabaseServiceStatus(item.Status) || !isDatabaseRefundStatus(item.RefundStatus) {
		return fmt.Errorf("create service order: %w", ErrConstraintViolation)
	}
	return store.mutate(ctx, "create service order", func() error {
		_, err := store.runner.ExecContext(ctx, `
			INSERT INTO xz_operation_center_service_orders (
				id, tenant_id, order_id, order_no, applicant_user_id,
				technical_service_fee_cents, currency, status, paid_at, reviewed_at,
				reviewed_by, activated_at, revoked_at, refund_order_id, state_version,
				metadata, created_at, updated_at, refund_status, commercial_rule_set_id,
				commercial_rule_set_version, plan_version_id, commercial_order_snapshot_id,
				relationship_snapshot, refund_policy_snapshot, review_idempotency_key,
				refund_idempotency_key, payment_channel, provider_refund_no, refund_failure_class,
				refund_failure_detail, refund_attempt_count, next_refund_retry_at,
				manual_refund_voucher_reference, manual_refund_voucher_file_hash,
				manual_refund_submitted_by, manual_refund_approved_by, current_refund_task_id
			) VALUES (
				$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,
				$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34,$35,$36,$37,$38
			)`, serviceOrderArgs(item)...)
		return err
	})
}

func serviceOrderArgs(item *OperationCenterServiceOrder) []any {
	return []any{
		item.ID, item.TenantID, item.OrderID, item.OrderNo, item.ApplicantUserID,
		item.TechnicalServiceFeeCents, item.Currency, item.Status, item.PaidAt, item.ReviewedAt,
		item.ReviewedBy, item.ActivatedAt, item.RevokedAt, item.RefundOrderID, item.StateVersion,
		item.Metadata, item.CreatedAt, item.UpdatedAt, item.RefundStatus, item.CommercialRuleSetID,
		item.CommercialRuleSetVersion, item.PlanVersionID, item.CommercialOrderSnapshotID,
		item.RelationshipSnapshot, item.RefundPolicySnapshot, item.ReviewIdempotencyKey,
		item.RefundIdempotencyKey, item.PaymentChannel, item.ProviderRefundNo, item.RefundFailureClass,
		item.RefundFailureDetail, item.RefundAttemptCount, item.NextRefundRetryAt,
		item.ManualRefundVoucherReference, item.ManualRefundVoucherFileHash,
		item.ManualRefundSubmittedBy, item.ManualRefundApprovedBy, item.CurrentRefundTaskID,
	}
}

func scanServiceOrder(row rowScanner) (*OperationCenterServiceOrder, error) {
	var item OperationCenterServiceOrder
	var paidAt, reviewedAt, activatedAt, revokedAt, nextRetryAt sql.NullTime
	var reviewedBy, refundOrderID, ruleSetID, planVersionID, orderSnapshotID sql.NullString
	var reviewKey, refundKey, paymentChannel, providerRefundNo, failureClass sql.NullString
	var voucherReference, voucherHash, submittedBy, approvedBy, currentTaskID sql.NullString
	var ruleVersion sql.NullInt64
	err := row.Scan(
		&item.ID, &item.TenantID, &item.OrderID, &item.OrderNo, &item.ApplicantUserID,
		&item.TechnicalServiceFeeCents, &item.Currency, &item.Status, &paidAt, &reviewedAt,
		&reviewedBy, &activatedAt, &revokedAt, &refundOrderID, &item.StateVersion,
		&item.Metadata, &item.CreatedAt, &item.UpdatedAt, &item.RefundStatus, &ruleSetID,
		&ruleVersion, &planVersionID, &orderSnapshotID, &item.RelationshipSnapshot,
		&item.RefundPolicySnapshot, &reviewKey, &refundKey, &paymentChannel,
		&providerRefundNo, &failureClass, &item.RefundFailureDetail, &item.RefundAttemptCount,
		&nextRetryAt, &voucherReference, &voucherHash, &submittedBy, &approvedBy, &currentTaskID,
	)
	if err != nil {
		return nil, mapPostgresStoreError("scan service order", err)
	}
	item.PaidAt = nullableTimePointer(paidAt)
	item.ReviewedAt = nullableTimePointer(reviewedAt)
	item.ActivatedAt = nullableTimePointer(activatedAt)
	item.RevokedAt = nullableTimePointer(revokedAt)
	item.NextRefundRetryAt = nullableTimePointer(nextRetryAt)
	item.ReviewedBy = nullableStringPointer(reviewedBy)
	item.RefundOrderID = nullableStringPointer(refundOrderID)
	item.CommercialRuleSetID = nullableStringPointer(ruleSetID)
	item.CommercialRuleSetVersion = nullableIntPointer(ruleVersion)
	item.PlanVersionID = nullableStringPointer(planVersionID)
	item.CommercialOrderSnapshotID = nullableStringPointer(orderSnapshotID)
	item.ReviewIdempotencyKey = nullableStringPointer(reviewKey)
	item.RefundIdempotencyKey = nullableStringPointer(refundKey)
	item.PaymentChannel = nullableStringPointer(paymentChannel)
	item.ProviderRefundNo = nullableStringPointer(providerRefundNo)
	if failureClass.Valid {
		value := RefundFailureClass(failureClass.String)
		item.RefundFailureClass = &value
	}
	item.ManualRefundVoucherReference = nullableStringPointer(voucherReference)
	item.ManualRefundVoucherFileHash = nullableStringPointer(voucherHash)
	item.ManualRefundSubmittedBy = nullableStringPointer(submittedBy)
	item.ManualRefundApprovedBy = nullableStringPointer(approvedBy)
	item.CurrentRefundTaskID = nullableStringPointer(currentTaskID)
	return &item, nil
}

func (store *PostgresStore) GetServiceOrderForUpdate(ctx context.Context, id string) (*OperationCenterServiceOrder, error) {
	if err := store.requireTx(); err != nil {
		return nil, err
	}
	return scanServiceOrder(store.runner.QueryRowContext(ctx, `SELECT `+serviceOrderColumns+` FROM xz_operation_center_service_orders WHERE id=$1 FOR UPDATE`, id))
}

func (store *PostgresStore) FindServiceOrderByCommercialOrderID(ctx context.Context, orderID string) (*OperationCenterServiceOrder, error) {
	return scanServiceOrder(store.runner.QueryRowContext(ctx, `SELECT `+serviceOrderColumns+` FROM xz_operation_center_service_orders WHERE order_id=$1`, orderID))
}

func (store *PostgresStore) FindServiceOrderByPaymentRecordID(ctx context.Context, paymentRecordID string) (*OperationCenterServiceOrder, error) {
	return scanServiceOrder(store.runner.QueryRowContext(ctx, `SELECT `+serviceOrderColumns+` FROM xz_operation_center_service_orders WHERE order_id=(SELECT order_id FROM xz_payment_records WHERE id=$1)`, paymentRecordID))
}

func (store *PostgresStore) UpdateServiceOrder(ctx context.Context, item *OperationCenterServiceOrder, transition *OperationCenterStateTransition) error {
	if err := store.requireTx(); err != nil {
		return err
	}
	if item == nil || transition == nil || transition.FromStatus == nil || transition.EntityType != StateEntityServiceOrder || transition.EntityID != item.ID || transition.ToStatus != string(item.Status) {
		return fmt.Errorf("update service order audit contract: %w", ErrConstraintViolation)
	}
	if err := ValidateOperationCenterServiceTransition(OperationCenterServiceStatus(*transition.FromStatus), item.Status); err != nil {
		return err
	}
	return store.mutate(ctx, "update service order", func() error {
		result, err := store.runner.ExecContext(ctx, `
			UPDATE xz_operation_center_service_orders SET
				status=$2, paid_at=$3, reviewed_at=$4, reviewed_by=$5, activated_at=$6,
				revoked_at=$7, refund_order_id=$8, state_version=$9, metadata=$10,
				updated_at=$11, refund_status=$12, commercial_rule_set_id=$13,
				commercial_rule_set_version=$14, plan_version_id=$15,
				commercial_order_snapshot_id=$16, relationship_snapshot=$17,
				refund_policy_snapshot=$18, review_idempotency_key=$19,
				refund_idempotency_key=$20, payment_channel=$21, provider_refund_no=$22,
				refund_failure_class=$23, refund_failure_detail=$24, refund_attempt_count=$25,
				next_refund_retry_at=$26, manual_refund_voucher_reference=$27,
				manual_refund_voucher_file_hash=$28, manual_refund_submitted_by=$29,
				manual_refund_approved_by=$30, current_refund_task_id=$31
			WHERE id=$1 AND state_version=$32`,
			item.ID, item.Status, item.PaidAt, item.ReviewedAt, item.ReviewedBy, item.ActivatedAt,
			item.RevokedAt, item.RefundOrderID, item.StateVersion, item.Metadata, item.UpdatedAt,
			item.RefundStatus, item.CommercialRuleSetID, item.CommercialRuleSetVersion,
			item.PlanVersionID, item.CommercialOrderSnapshotID, item.RelationshipSnapshot,
			item.RefundPolicySnapshot, item.ReviewIdempotencyKey, item.RefundIdempotencyKey,
			item.PaymentChannel, item.ProviderRefundNo, item.RefundFailureClass,
			item.RefundFailureDetail, item.RefundAttemptCount, item.NextRefundRetryAt,
			item.ManualRefundVoucherReference, item.ManualRefundVoucherFileHash,
			item.ManualRefundSubmittedBy, item.ManualRefundApprovedBy, item.CurrentRefundTaskID,
			item.StateVersion-1)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrUniqueConflict
		}
		return store.appendStateTransitionRaw(ctx, transition)
	})
}

func (store *PostgresStore) UpdateServiceOrderRefundProjection(ctx context.Context, item *OperationCenterServiceOrder) error {
	if err := store.requireTx(); err != nil {
		return err
	}
	if item == nil || !isDatabaseRefundStatus(item.RefundStatus) || item.StateVersion <= 0 {
		return ErrConstraintViolation
	}
	return store.mutate(ctx, "update service order refund projection", func() error {
		result, err := store.runner.ExecContext(ctx, `
			UPDATE xz_operation_center_service_orders SET
				refund_status=$2,refund_order_id=$3,refund_idempotency_key=$4,
				provider_refund_no=$5,refund_failure_class=$6,refund_failure_detail=$7,
				refund_attempt_count=$8,next_refund_retry_at=$9,current_refund_task_id=$10,
				manual_refund_voucher_reference=$11,manual_refund_voucher_file_hash=$12,
				manual_refund_submitted_by=$13,manual_refund_approved_by=$14,
				state_version=$15,updated_at=$16
			WHERE id=$1 AND state_version=$17
		`, item.ID, item.RefundStatus, item.RefundOrderID, item.RefundIdempotencyKey,
			item.ProviderRefundNo, item.RefundFailureClass, item.RefundFailureDetail,
			item.RefundAttemptCount, item.NextRefundRetryAt, item.CurrentRefundTaskID,
			item.ManualRefundVoucherReference, item.ManualRefundVoucherFileHash,
			item.ManualRefundSubmittedBy, item.ManualRefundApprovedBy,
			item.StateVersion, item.UpdatedAt, item.StateVersion-1)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrUniqueConflict
		}
		return nil
	})
}

const reviewEventColumns = `id, tenant_id, service_order_id, decision, event_status, reviewed_by, request_id, idempotency_key, failure_class, failure_detail, event_snapshot, applied_at, created_at, updated_at`

func (store *PostgresStore) CreateReviewEvent(ctx context.Context, event *OperationCenterReviewEvent) error {
	if event == nil {
		return fmt.Errorf("create review event: %w", ErrConstraintViolation)
	}
	if existing, err := store.GetReviewEventByIdempotencyKey(ctx, event.TenantID, event.IdempotencyKey); err == nil && existing != nil {
		return ErrIdempotencyConflict
	} else if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	return store.mutate(ctx, "create review event", func() error {
		_, err := store.runner.ExecContext(ctx, `INSERT INTO xz_operation_center_review_events (`+reviewEventColumns+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
			event.ID, event.TenantID, event.ServiceOrderID, event.Decision, event.Status, event.ReviewedBy,
			event.RequestID, event.IdempotencyKey, event.FailureClass, event.FailureDetail,
			event.EventSnapshot, event.AppliedAt, event.CreatedAt, event.UpdatedAt)
		return err
	})
}

func scanReviewEvent(row rowScanner) (*OperationCenterReviewEvent, error) {
	var event OperationCenterReviewEvent
	var requestID, failureClass sql.NullString
	var appliedAt sql.NullTime
	err := row.Scan(&event.ID, &event.TenantID, &event.ServiceOrderID, &event.Decision, &event.Status,
		&event.ReviewedBy, &requestID, &event.IdempotencyKey, &failureClass, &event.FailureDetail,
		&event.EventSnapshot, &appliedAt, &event.CreatedAt, &event.UpdatedAt)
	if err != nil {
		return nil, mapPostgresStoreError("scan review event", err)
	}
	event.RequestID = nullableStringPointer(requestID)
	event.AppliedAt = nullableTimePointer(appliedAt)
	if failureClass.Valid {
		value := RefundFailureClass(failureClass.String)
		event.FailureClass = &value
	}
	return &event, nil
}

func (store *PostgresStore) GetReviewEventByIdempotencyKey(ctx context.Context, tenantID, key string) (*OperationCenterReviewEvent, error) {
	return scanReviewEvent(store.runner.QueryRowContext(ctx, `SELECT `+reviewEventColumns+` FROM xz_operation_center_review_events WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, key))
}

func (store *PostgresStore) UpdateReviewEvent(ctx context.Context, event *OperationCenterReviewEvent) error {
	if event == nil {
		return ErrConstraintViolation
	}
	return store.mutate(ctx, "update review event", func() error {
		_, err := store.runner.ExecContext(ctx, `UPDATE xz_operation_center_review_events SET event_status=$2,failure_class=$3,failure_detail=$4,event_snapshot=$5,applied_at=$6,updated_at=$7 WHERE id=$1`,
			event.ID, event.Status, event.FailureClass, event.FailureDetail, event.EventSnapshot, event.AppliedAt, event.UpdatedAt)
		return err
	})
}

const refundTaskColumns = `
id, tenant_id, service_order_id, order_id, payment_record_id, commercial_rule_set_id,
origin_type, refund_scope, amount_cents, currency, payment_channel, provider_payment_no,
provider_refund_no, provider_outcome, refund_status, failure_class, failure_detail,
idempotency_key, attempt_count, next_retry_at, lease_owner, lease_expires_at,
unknown_since, prepared_at, completed_at, provider_refunded_at,
provider_response_summary, provider_query_outcome, provider_query_response_summary,
verification_attempt_count, last_verification_at,
manual_provider_transaction_id,
manual_voucher_reference, manual_voucher_file_hash, manual_submitted_by,
manual_approved_by, state_version, created_at, updated_at`

func (store *PostgresStore) CreateRefundTask(ctx context.Context, task *OperationCenterRefundTask) error {
	if task == nil || !isDatabaseRefundStatus(task.Status) || task.Status == OperationCenterRefundNone {
		return fmt.Errorf("create refund task: %w", ErrConstraintViolation)
	}
	if existing, err := store.GetRefundTaskByIdempotencyKey(ctx, task.IdempotencyKey); err == nil && existing != nil {
		return ErrIdempotencyConflict
	} else if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	return store.mutate(ctx, "create refund task", func() error {
		_, err := store.runner.ExecContext(ctx, `INSERT INTO xz_operation_center_refund_tasks (`+refundTaskColumns+`) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,
			$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34,$35,$36,$37,$38,$39)`, refundTaskArgs(task)...)
		return err
	})
}

func refundTaskArgs(task *OperationCenterRefundTask) []any {
	return []any{
		task.ID, task.TenantID, task.ServiceOrderID, task.OrderID, task.PaymentRecordID,
		task.CommercialRuleSetID, task.Origin, task.Scope, task.AmountCents, task.Currency,
		task.PaymentChannel, task.ProviderPaymentNo, task.ProviderRefundNo, task.ProviderOutcome,
		task.Status, task.FailureClass, task.FailureDetail, task.IdempotencyKey, task.AttemptCount,
		task.NextRetryAt, task.LeaseOwner, task.LeaseExpiresAt, task.UnknownSince, task.PreparedAt,
		task.CompletedAt, task.ProviderRefundedAt, task.ProviderResponseSummary, task.ProviderQueryOutcome,
		task.ProviderQueryResponseSummary, task.VerificationAttemptCount, task.LastVerificationAt,
		task.ManualProviderTransactionID, task.ManualVoucherReference,
		task.ManualVoucherFileHash, task.ManualSubmittedBy, task.ManualApprovedBy,
		task.StateVersion, task.CreatedAt, task.UpdatedAt,
	}
}

func scanRefundTask(row rowScanner) (*OperationCenterRefundTask, error) {
	var task OperationCenterRefundTask
	var paymentRecordID, providerPaymentNo, providerRefundNo, providerOutcome sql.NullString
	var failureClass, leaseOwner, manualProviderTransactionID, manualVoucherReference sql.NullString
	var manualVoucherFileHash, manualSubmittedBy, manualApprovedBy sql.NullString
	var nextRetryAt, leaseExpiresAt, unknownSince, preparedAt, completedAt, providerRefundedAt sql.NullTime
	var providerQueryOutcome sql.NullString
	var lastVerificationAt sql.NullTime
	err := row.Scan(
		&task.ID, &task.TenantID, &task.ServiceOrderID, &task.OrderID, &paymentRecordID,
		&task.CommercialRuleSetID, &task.Origin, &task.Scope, &task.AmountCents, &task.Currency,
		&task.PaymentChannel, &providerPaymentNo, &providerRefundNo, &providerOutcome, &task.Status,
		&failureClass, &task.FailureDetail, &task.IdempotencyKey, &task.AttemptCount, &nextRetryAt,
		&leaseOwner, &leaseExpiresAt, &unknownSince, &preparedAt, &completedAt, &providerRefundedAt,
		&task.ProviderResponseSummary, &providerQueryOutcome, &task.ProviderQueryResponseSummary,
		&task.VerificationAttemptCount, &lastVerificationAt,
		&manualProviderTransactionID, &manualVoucherReference, &manualVoucherFileHash,
		&manualSubmittedBy, &manualApprovedBy, &task.StateVersion, &task.CreatedAt, &task.UpdatedAt,
	)
	if err != nil {
		return nil, mapPostgresStoreError("scan refund task", err)
	}
	task.PaymentRecordID = nullableStringPointer(paymentRecordID)
	task.ProviderPaymentNo = nullableStringPointer(providerPaymentNo)
	task.ProviderRefundNo = nullableStringPointer(providerRefundNo)
	if providerOutcome.Valid {
		value := RefundProviderResult(providerOutcome.String)
		task.ProviderOutcome = &value
	}
	if failureClass.Valid {
		value := RefundFailureClass(failureClass.String)
		task.FailureClass = &value
	}
	task.NextRetryAt = nullableTimePointer(nextRetryAt)
	task.LeaseOwner = nullableStringPointer(leaseOwner)
	task.LeaseExpiresAt = nullableTimePointer(leaseExpiresAt)
	task.UnknownSince = nullableTimePointer(unknownSince)
	task.PreparedAt = nullableTimePointer(preparedAt)
	task.CompletedAt = nullableTimePointer(completedAt)
	task.ProviderRefundedAt = nullableTimePointer(providerRefundedAt)
	if providerQueryOutcome.Valid {
		value := payment.QueryRefundOutcome(providerQueryOutcome.String)
		task.ProviderQueryOutcome = &value
	}
	task.LastVerificationAt = nullableTimePointer(lastVerificationAt)
	task.ManualProviderTransactionID = nullableStringPointer(manualProviderTransactionID)
	task.ManualVoucherReference = nullableStringPointer(manualVoucherReference)
	task.ManualVoucherFileHash = nullableStringPointer(manualVoucherFileHash)
	task.ManualSubmittedBy = nullableStringPointer(manualSubmittedBy)
	task.ManualApprovedBy = nullableStringPointer(manualApprovedBy)
	return &task, nil
}

func (store *PostgresStore) GetRefundTaskByIdempotencyKey(ctx context.Context, key string) (*OperationCenterRefundTask, error) {
	return scanRefundTask(store.runner.QueryRowContext(ctx, `SELECT `+refundTaskColumns+` FROM xz_operation_center_refund_tasks WHERE idempotency_key=$1`, key))
}

func (store *PostgresStore) GetRefundTaskForUpdate(ctx context.Context, id string) (*OperationCenterRefundTask, error) {
	if err := store.requireTx(); err != nil {
		return nil, err
	}
	return scanRefundTask(store.runner.QueryRowContext(ctx, `SELECT `+refundTaskColumns+` FROM xz_operation_center_refund_tasks WHERE id=$1 FOR UPDATE`, id))
}

func (store *PostgresStore) UpdateRefundTask(ctx context.Context, task *OperationCenterRefundTask, transition *OperationCenterStateTransition) error {
	if err := store.requireTx(); err != nil {
		return err
	}
	if task == nil || transition == nil || transition.FromStatus == nil || transition.EntityType != StateEntityRefundTask || transition.EntityID != task.ID || transition.ToStatus != string(task.Status) {
		return fmt.Errorf("update refund task audit contract: %w", ErrConstraintViolation)
	}
	if err := ValidateOperationCenterRefundTransition(OperationCenterRefundStatus(*transition.FromStatus), task.Status); err != nil {
		return err
	}
	return store.mutate(ctx, "update refund task", func() error {
		result, err := store.runner.ExecContext(ctx, `UPDATE xz_operation_center_refund_tasks SET
			provider_payment_no=$2,provider_refund_no=$3,provider_outcome=$4,refund_status=$5,
			failure_class=$6,failure_detail=$7,attempt_count=$8,next_retry_at=$9,lease_owner=$10,
			lease_expires_at=$11,unknown_since=$12,prepared_at=$13,completed_at=$14,
			provider_refunded_at=$15,provider_response_summary=$16,provider_query_outcome=$17,
			provider_query_response_summary=$18,verification_attempt_count=$19,last_verification_at=$20,
			manual_provider_transaction_id=$21,manual_voucher_reference=$22,
			manual_voucher_file_hash=$23,manual_submitted_by=$24,manual_approved_by=$25,
			state_version=$26,updated_at=$27 WHERE id=$1 AND state_version=$28`,
			task.ID, task.ProviderPaymentNo, task.ProviderRefundNo, task.ProviderOutcome, task.Status,
			task.FailureClass, task.FailureDetail, task.AttemptCount, task.NextRetryAt, task.LeaseOwner,
			task.LeaseExpiresAt, task.UnknownSince, task.PreparedAt, task.CompletedAt,
			task.ProviderRefundedAt, task.ProviderResponseSummary, task.ProviderQueryOutcome,
			task.ProviderQueryResponseSummary, task.VerificationAttemptCount, task.LastVerificationAt,
			task.ManualProviderTransactionID, task.ManualVoucherReference, task.ManualVoucherFileHash,
			task.ManualSubmittedBy, task.ManualApprovedBy, task.StateVersion, task.UpdatedAt,
			task.StateVersion-1)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrUniqueConflict
		}
		return store.appendStateTransitionRaw(ctx, transition)
	})
}

func (store *PostgresStore) UpdateRefundTaskLease(ctx context.Context, task *OperationCenterRefundTask) error {
	if err := store.requireTx(); err != nil {
		return err
	}
	if task == nil || task.Status != OperationCenterRefundProviderPending || task.StateVersion <= 0 {
		return ErrConstraintViolation
	}
	return store.mutate(ctx, "update refund task lease", func() error {
		result, err := store.runner.ExecContext(ctx, `
			UPDATE xz_operation_center_refund_tasks SET
				attempt_count=$2,lease_owner=$3,lease_expires_at=$4,prepared_at=$5,
				state_version=$6,updated_at=$7
			WHERE id=$1 AND refund_status='PROVIDER_PENDING' AND state_version=$8
		`, task.ID, task.AttemptCount, task.LeaseOwner, task.LeaseExpiresAt, task.PreparedAt,
			task.StateVersion, task.UpdatedAt, task.StateVersion-1)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrUniqueConflict
		}
		return nil
	})
}

func (store *PostgresStore) UpdateRefundTaskVerification(ctx context.Context, task *OperationCenterRefundTask) error {
	if err := store.requireTx(); err != nil {
		return err
	}
	if task == nil || task.Status != OperationCenterRefundUnknownVerifying || task.StateVersion <= 0 {
		return ErrConstraintViolation
	}
	return store.mutate(ctx, "update refund task verification", func() error {
		result, err := store.runner.ExecContext(ctx, `
			UPDATE xz_operation_center_refund_tasks SET
				provider_refund_no=$2,provider_query_outcome=$3,provider_query_response_summary=$4,
				verification_attempt_count=$5,last_verification_at=$6,next_retry_at=$7,
				lease_owner=$8,lease_expires_at=$9,unknown_since=$10,failure_class=$11,
				failure_detail=$12,state_version=$13,updated_at=$14
			WHERE id=$1 AND refund_status='UNKNOWN_VERIFYING' AND state_version=$15
		`, task.ID, task.ProviderRefundNo, task.ProviderQueryOutcome, task.ProviderQueryResponseSummary,
			task.VerificationAttemptCount, task.LastVerificationAt, task.NextRetryAt,
			task.LeaseOwner, task.LeaseExpiresAt, task.UnknownSince, task.FailureClass,
			task.FailureDetail, task.StateVersion, task.UpdatedAt, task.StateVersion-1)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrUniqueConflict
		}
		return nil
	})
}

func (store *PostgresStore) ClaimRefundTasks(ctx context.Context, now time.Time, owner string, leaseUntil time.Time, limit int) ([]OperationCenterRefundTask, error) {
	if err := store.requireTx(); err != nil {
		return nil, err
	}
	if limit <= 0 || strings.TrimSpace(owner) == "" || !leaseUntil.After(now) {
		return nil, ErrConstraintViolation
	}
	rows, err := store.runner.QueryContext(ctx, `
		WITH claimed AS (
			SELECT id FROM xz_operation_center_refund_tasks
			WHERE refund_status IN ('PROVIDER_PENDING','REFUND_RETRYABLE')
			  AND (next_retry_at IS NULL OR next_retry_at <= $1)
			  AND (lease_expires_at IS NULL OR lease_expires_at <= $1)
			ORDER BY COALESCE(next_retry_at, created_at), created_at, id
			FOR UPDATE SKIP LOCKED LIMIT $4
		)
		UPDATE xz_operation_center_refund_tasks task
		SET lease_owner=$2, lease_expires_at=$3, updated_at=$1
		FROM claimed WHERE task.id=claimed.id
		RETURNING `+qualifiedColumns("task", refundTaskColumns), now, owner, leaseUntil, limit)
	if err != nil {
		return nil, mapPostgresStoreError("claim refund tasks", err)
	}
	defer rows.Close()
	var result []OperationCenterRefundTask
	for rows.Next() {
		item, scanErr := scanRefundTask(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, mapPostgresStoreError("iterate refund tasks", err)
	}
	return result, nil
}

const manualRefundColumns = `id, tenant_id, refund_task_id, payment_channel, amount_cents, currency, provider_transaction_id, provider_refund_no, voucher_reference, voucher_file_hash, status, submitted_by, submitted_at, approved_by, approved_at, rejection_reason, remark, created_at, updated_at`

func (store *PostgresStore) SubmitManualRefund(ctx context.Context, item *OperationCenterManualRefund) error {
	if item == nil || item.Status != ManualRefundSubmitted {
		return ErrConstraintViolation
	}
	return store.mutate(ctx, "submit manual refund", func() error {
		_, err := store.runner.ExecContext(ctx, `INSERT INTO xz_operation_center_manual_refunds (`+manualRefundColumns+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`,
			item.ID, item.TenantID, item.RefundTaskID, item.PaymentChannel, item.AmountCents,
			item.Currency, item.ProviderTransactionID, item.ProviderRefundNo, item.VoucherReference,
			item.VoucherFileHash, item.Status, item.SubmittedBy, item.SubmittedAt, item.ApprovedBy,
			item.ApprovedAt, item.RejectionReason, item.Remark, item.CreatedAt, item.UpdatedAt)
		return err
	})
}

func scanManualRefund(row rowScanner) (*OperationCenterManualRefund, error) {
	var item OperationCenterManualRefund
	var providerRefundNo, approvedBy, rejectionReason, remark sql.NullString
	var approvedAt sql.NullTime
	err := row.Scan(&item.ID, &item.TenantID, &item.RefundTaskID, &item.PaymentChannel,
		&item.AmountCents, &item.Currency, &item.ProviderTransactionID, &providerRefundNo,
		&item.VoucherReference, &item.VoucherFileHash, &item.Status, &item.SubmittedBy,
		&item.SubmittedAt, &approvedBy, &approvedAt, &rejectionReason, &remark,
		&item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return nil, mapPostgresStoreError("scan manual refund", err)
	}
	item.ProviderRefundNo = nullableStringPointer(providerRefundNo)
	item.ApprovedBy = nullableStringPointer(approvedBy)
	item.ApprovedAt = nullableTimePointer(approvedAt)
	item.RejectionReason = nullableStringPointer(rejectionReason)
	item.Remark = nullableStringPointer(remark)
	return &item, nil
}

func (store *PostgresStore) ApproveManualRefund(ctx context.Context, id, approvedBy string, approvedAt time.Time, transition OperationCenterStateTransition) (*OperationCenterManualRefund, error) {
	if err := store.requireTx(); err != nil {
		return nil, err
	}
	var approved *OperationCenterManualRefund
	err := store.mutate(ctx, "approve manual refund", func() error {
		current, err := scanManualRefund(store.runner.QueryRowContext(ctx, `SELECT `+manualRefundColumns+` FROM xz_operation_center_manual_refunds WHERE id=$1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		if current.SubmittedBy == approvedBy {
			return ErrManualRefundSelfApproval
		}
		approved, err = scanManualRefund(store.runner.QueryRowContext(ctx, `UPDATE xz_operation_center_manual_refunds SET status='APPROVED',approved_by=$2,approved_at=$3,updated_at=$3 WHERE id=$1 RETURNING `+manualRefundColumns, id, approvedBy, approvedAt))
		if err != nil {
			return err
		}
		return store.appendStateTransitionRaw(ctx, &transition)
	})
	return approved, err
}

const rewardReleaseColumns = `id, tenant_id, referral_reward_id, idempotency_key, release_status, execute_at, attempt_count, next_retry_at, lease_owner, lease_expires_at, failure_class, failure_detail, started_at, completed_at, cancellation_reason, cancelled_at, created_at, updated_at`

func (store *PostgresStore) CreateRewardReleaseTask(ctx context.Context, task *ReferralRewardReleaseTask) error {
	if task == nil {
		return ErrConstraintViolation
	}
	if existing, err := store.GetRewardReleaseTaskByIdempotencyKey(ctx, task.TenantID, task.IdempotencyKey); err == nil && existing != nil {
		return ErrIdempotencyConflict
	} else if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	return store.mutate(ctx, "create reward release task", func() error {
		_, err := store.runner.ExecContext(ctx, `INSERT INTO xz_referral_reward_release_tasks (`+rewardReleaseColumns+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
			task.ID, task.TenantID, task.ReferralRewardID, task.IdempotencyKey, task.Status,
			task.ExecuteAt, task.AttemptCount, task.NextRetryAt, task.LeaseOwner, task.LeaseExpiresAt,
			task.FailureClass, task.FailureDetail, task.StartedAt, task.CompletedAt,
			task.CancellationReason, task.CancelledAt, task.CreatedAt, task.UpdatedAt)
		return err
	})
}

func scanRewardReleaseTask(row rowScanner) (*ReferralRewardReleaseTask, error) {
	var task ReferralRewardReleaseTask
	var leaseOwner, failureClass sql.NullString
	var nextRetryAt, leaseExpiresAt, startedAt, completedAt, cancelledAt sql.NullTime
	var cancellationReason sql.NullString
	err := row.Scan(&task.ID, &task.TenantID, &task.ReferralRewardID, &task.IdempotencyKey,
		&task.Status, &task.ExecuteAt, &task.AttemptCount, &nextRetryAt, &leaseOwner, &leaseExpiresAt,
		&failureClass, &task.FailureDetail, &startedAt, &completedAt, &cancellationReason, &cancelledAt, &task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		return nil, mapPostgresStoreError("scan reward release task", err)
	}
	task.NextRetryAt = nullableTimePointer(nextRetryAt)
	task.LeaseOwner = nullableStringPointer(leaseOwner)
	task.LeaseExpiresAt = nullableTimePointer(leaseExpiresAt)
	task.StartedAt = nullableTimePointer(startedAt)
	task.CompletedAt = nullableTimePointer(completedAt)
	task.CancellationReason = nullableStringPointer(cancellationReason)
	task.CancelledAt = nullableTimePointer(cancelledAt)
	if failureClass.Valid {
		value := RefundFailureClass(failureClass.String)
		task.FailureClass = &value
	}
	return &task, nil
}

func (store *PostgresStore) GetRewardReleaseTaskByIdempotencyKey(ctx context.Context, tenantID, key string) (*ReferralRewardReleaseTask, error) {
	return scanRewardReleaseTask(store.runner.QueryRowContext(ctx, `SELECT `+rewardReleaseColumns+` FROM xz_referral_reward_release_tasks WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, key))
}

func (store *PostgresStore) UpdateRewardReleaseTask(ctx context.Context, task *ReferralRewardReleaseTask, transition *OperationCenterStateTransition) error {
	if err := store.requireTx(); err != nil {
		return err
	}
	if task == nil || transition == nil || transition.EntityType != StateEntityRewardReleaseTask || transition.EntityID != task.ID || transition.ToStatus != string(task.Status) {
		return ErrConstraintViolation
	}
	return store.mutate(ctx, "update reward release task", func() error {
		_, err := store.runner.ExecContext(ctx, `UPDATE xz_referral_reward_release_tasks SET release_status=$2,attempt_count=$3,next_retry_at=$4,lease_owner=$5,lease_expires_at=$6,failure_class=$7,failure_detail=$8,started_at=$9,completed_at=$10,cancellation_reason=$11,cancelled_at=$12,updated_at=$13 WHERE id=$1`,
			task.ID, task.Status, task.AttemptCount, task.NextRetryAt, task.LeaseOwner,
			task.LeaseExpiresAt, task.FailureClass, task.FailureDetail, task.StartedAt,
			task.CompletedAt, task.CancellationReason, task.CancelledAt, task.UpdatedAt)
		if err != nil {
			return err
		}
		return store.appendStateTransitionRaw(ctx, transition)
	})
}

func (store *PostgresStore) ClaimDueRewardReleaseTasks(ctx context.Context, now time.Time, owner string, leaseUntil time.Time, limit int) ([]ReferralRewardReleaseTask, error) {
	if err := store.requireTx(); err != nil {
		return nil, err
	}
	if limit <= 0 || strings.TrimSpace(owner) == "" || !leaseUntil.After(now) {
		return nil, ErrConstraintViolation
	}
	rows, err := store.runner.QueryContext(ctx, `
		WITH claimed AS (
			SELECT task.id FROM xz_referral_reward_release_tasks task
			JOIN xz_referral_rewards reward ON reward.id=task.referral_reward_id
			WHERE (
			       task.release_status='PENDING'
			       OR (task.release_status='FAILED' AND task.failure_class='TEMPORARY_FAILURE')
			       OR (task.release_status='PROCESSING' AND task.lease_expires_at <= $1)
			      )
			  AND reward.status='FROZEN' AND task.execute_at <= $1
			  AND (task.next_retry_at IS NULL OR task.next_retry_at <= $1)
			  AND (task.lease_expires_at IS NULL OR task.lease_expires_at <= $1)
			ORDER BY task.execute_at, task.created_at, task.id
			FOR UPDATE OF task SKIP LOCKED LIMIT $4
		)
		UPDATE xz_referral_reward_release_tasks task
		SET release_status='PROCESSING',lease_owner=$2,lease_expires_at=$3,
		    failure_class=NULL,failure_detail='{}'::jsonb,
		    started_at=COALESCE(task.started_at,$1),updated_at=$1
		FROM claimed WHERE task.id=claimed.id
		RETURNING `+qualifiedColumns("task", rewardReleaseColumns), now, owner, leaseUntil, limit)
	if err != nil {
		return nil, mapPostgresStoreError("claim reward release tasks", err)
	}
	defer rows.Close()
	var result []ReferralRewardReleaseTask
	for rows.Next() {
		item, scanErr := scanRewardReleaseTask(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, mapPostgresStoreError("iterate reward release tasks", err)
	}
	return result, nil
}

func (store *PostgresStore) AppendStateTransition(ctx context.Context, transition *OperationCenterStateTransition) error {
	if err := store.requireTx(); err != nil {
		return err
	}
	if transition == nil {
		return ErrConstraintViolation
	}
	return store.mutate(ctx, "append state transition", func() error {
		return store.appendStateTransitionRaw(ctx, transition)
	})
}

func (store *PostgresStore) appendStateTransitionRaw(ctx context.Context, transition *OperationCenterStateTransition) error {
	_, err := store.runner.ExecContext(ctx, `INSERT INTO xz_operation_center_state_transitions (
		id,tenant_id,entity_type,entity_id,from_status,to_status,action,actor_id,request_id,
		idempotency_key,transition_group_key,metadata,created_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		transition.ID, transition.TenantID, transition.EntityType, transition.EntityID,
		transition.FromStatus, transition.ToStatus, transition.TransitionReason,
		transition.OperatorID, transition.RequestID, transition.IdempotencyKey,
		transition.TransactionGroupID, transition.Metadata, transition.CreatedAt)
	return err
}

func qualifiedColumns(alias, columns string) string {
	parts := strings.Split(columns, ",")
	for index, part := range parts {
		parts[index] = alias + "." + strings.TrimSpace(part)
	}
	return strings.Join(parts, ", ")
}
