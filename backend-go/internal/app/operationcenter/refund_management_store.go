package operationcenter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (store *PostgresStore) GetServiceOrder(ctx context.Context, id string) (*OperationCenterServiceOrder, error) {
	return scanServiceOrder(store.runner.QueryRowContext(ctx, `SELECT `+serviceOrderColumns+` FROM xz_operation_center_service_orders WHERE id=$1`, id))
}

func (store *PostgresStore) GetRefundTask(ctx context.Context, id string) (*OperationCenterRefundTask, error) {
	return scanRefundTask(store.runner.QueryRowContext(ctx, `SELECT `+refundTaskColumns+` FROM xz_operation_center_refund_tasks WHERE id=$1`, id))
}

func (store *PostgresStore) ClaimRetryableRefundTasks(ctx context.Context, now time.Time, owner string, leaseUntil time.Time, limit int) ([]OperationCenterRefundTask, error) {
	return store.claimRefundTasksByStatus(ctx, OperationCenterRefundRetryable, now, owner, leaseUntil, limit)
}

func (store *PostgresStore) ClaimVerificationRefundTasks(ctx context.Context, now time.Time, owner string, leaseUntil time.Time, limit int) ([]OperationCenterRefundTask, error) {
	return store.claimRefundTasksByStatus(ctx, OperationCenterRefundUnknownVerifying, now, owner, leaseUntil, limit)
}

func (store *PostgresStore) claimRefundTasksByStatus(ctx context.Context, status OperationCenterRefundStatus, now time.Time, owner string, leaseUntil time.Time, limit int) ([]OperationCenterRefundTask, error) {
	if err := store.requireTx(); err != nil {
		return nil, err
	}
	if status != OperationCenterRefundRetryable && status != OperationCenterRefundUnknownVerifying {
		return nil, ErrConstraintViolation
	}
	if limit <= 0 || strings.TrimSpace(owner) == "" || !leaseUntil.After(now) {
		return nil, ErrConstraintViolation
	}
	rows, err := store.runner.QueryContext(ctx, `
		WITH claimed AS (
			SELECT id FROM xz_operation_center_refund_tasks
			WHERE refund_status=$1
			  AND (next_retry_at IS NULL OR next_retry_at <= $2)
			  AND (lease_expires_at IS NULL OR lease_expires_at <= $2)
			ORDER BY COALESCE(next_retry_at,created_at),created_at,id
			FOR UPDATE SKIP LOCKED LIMIT $5
		)
		UPDATE xz_operation_center_refund_tasks task
		SET lease_owner=$3,lease_expires_at=$4,updated_at=$2
		FROM claimed WHERE task.id=claimed.id
		RETURNING `+qualifiedColumns("task", refundTaskColumns), status, now, owner, leaseUntil, limit)
	if err != nil {
		return nil, mapPostgresStoreError("claim refund management tasks", err)
	}
	defer rows.Close()
	result := make([]OperationCenterRefundTask, 0, limit)
	for rows.Next() {
		item, scanErr := scanRefundTask(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, mapPostgresStoreError("iterate refund management tasks", err)
	}
	return result, nil
}

func (store *PostgresStore) CreateRefundRequestEvent(ctx context.Context, event *RefundRequestEvent) error {
	if event == nil || strings.TrimSpace(event.IdempotencyKey) == "" || event.ExpectedServiceStatus != OperationCenterServiceActive {
		return ErrConstraintViolation
	}
	return store.mutate(ctx, "create refund request event", func() error {
		_, err := store.runner.ExecContext(ctx, `INSERT INTO xz_operation_center_refund_request_events(
			id,tenant_id,service_order_id,refund_task_id,requested_by,request_id,idempotency_key,
			reason,expected_service_status,request_snapshot,created_at
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, event.ID, event.TenantID,
			event.ServiceOrderID, event.RefundTaskID, event.RequestedBy, nullableText(event.RequestID),
			event.IdempotencyKey, event.Reason, event.ExpectedServiceStatus, event.Snapshot, event.CreatedAt)
		return err
	})
}

func scanRefundRequestEvent(row rowScanner) (*RefundRequestEvent, error) {
	var event RefundRequestEvent
	var requestID sql.NullString
	err := row.Scan(&event.ID, &event.TenantID, &event.ServiceOrderID, &event.RefundTaskID,
		&event.RequestedBy, &requestID, &event.IdempotencyKey, &event.Reason,
		&event.ExpectedServiceStatus, &event.Snapshot, &event.CreatedAt)
	if err != nil {
		return nil, mapPostgresStoreError("scan refund request event", err)
	}
	if requestID.Valid {
		event.RequestID = requestID.String
	}
	return &event, nil
}

func (store *PostgresStore) GetRefundRequestEventByIdempotencyKey(ctx context.Context, tenantID, key string) (*RefundRequestEvent, error) {
	return scanRefundRequestEvent(store.runner.QueryRowContext(ctx, `SELECT id,tenant_id,service_order_id,refund_task_id,requested_by,request_id,idempotency_key,reason,expected_service_status,request_snapshot,created_at FROM xz_operation_center_refund_request_events WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, key))
}

func (store *PostgresStore) GetManualRefundByID(ctx context.Context, id string) (*OperationCenterManualRefund, error) {
	return scanManualRefund(store.runner.QueryRowContext(ctx, `SELECT `+manualRefundColumns+` FROM xz_operation_center_manual_refunds WHERE id=$1`, id))
}

func (store *PostgresStore) GetManualRefundByProviderTransactionForUpdate(ctx context.Context, taskID, providerTransactionID string) (*OperationCenterManualRefund, error) {
	if err := store.requireTx(); err != nil {
		return nil, err
	}
	return scanManualRefund(store.runner.QueryRowContext(ctx, `SELECT `+manualRefundColumns+` FROM xz_operation_center_manual_refunds WHERE refund_task_id=$1 AND provider_transaction_id=$2 FOR UPDATE`, taskID, providerTransactionID))
}

func (store *PostgresStore) GetLatestManualRefundForUpdate(ctx context.Context, taskID string) (*OperationCenterManualRefund, error) {
	if err := store.requireTx(); err != nil {
		return nil, err
	}
	return scanManualRefund(store.runner.QueryRowContext(ctx, `SELECT `+manualRefundColumns+` FROM xz_operation_center_manual_refunds WHERE refund_task_id=$1 ORDER BY created_at DESC,id DESC LIMIT 1 FOR UPDATE`, taskID))
}

func (store *PostgresStore) UpdateManualRefundRecord(ctx context.Context, item *OperationCenterManualRefund) error {
	if err := store.requireTx(); err != nil {
		return err
	}
	if item == nil {
		return ErrConstraintViolation
	}
	return store.mutate(ctx, "update manual refund record", func() error {
		result, err := store.runner.ExecContext(ctx, `UPDATE xz_operation_center_manual_refunds SET
			provider_refund_no=$2,voucher_reference=$3,voucher_file_hash=$4,status=$5,
			submitted_by=$6,submitted_at=$7,approved_by=$8,approved_at=$9,
			rejection_reason=$10,remark=$11,updated_at=$12 WHERE id=$1`, item.ID,
			item.ProviderRefundNo, item.VoucherReference, item.VoucherFileHash, item.Status,
			item.SubmittedBy, item.SubmittedAt, item.ApprovedBy, item.ApprovedAt,
			item.RejectionReason, item.Remark, item.UpdatedAt)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrNotFound
		}
		return nil
	})
}

func (store *PostgresStore) CreateManualRefundEvent(ctx context.Context, event *ManualRefundEvent) error {
	if event == nil || strings.TrimSpace(event.IdempotencyKey) == "" || strings.TrimSpace(event.Reason) == "" {
		return ErrConstraintViolation
	}
	return store.mutate(ctx, "create manual refund event", func() error {
		_, err := store.runner.ExecContext(ctx, `INSERT INTO xz_operation_center_manual_refund_events(
			id,tenant_id,refund_task_id,manual_refund_id,event_type,actor_id,request_id,
			idempotency_key,reason,before_status,after_status,event_snapshot,created_at
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, event.ID, event.TenantID,
			event.RefundTaskID, event.ManualRefundID, event.EventType, event.ActorID,
			nullableText(event.RequestID), event.IdempotencyKey, event.Reason, event.BeforeStatus,
			event.AfterStatus, event.Snapshot, event.CreatedAt)
		return err
	})
}

func scanManualRefundEvent(row rowScanner) (*ManualRefundEvent, error) {
	var event ManualRefundEvent
	var requestID sql.NullString
	err := row.Scan(&event.ID, &event.TenantID, &event.RefundTaskID, &event.ManualRefundID,
		&event.EventType, &event.ActorID, &requestID, &event.IdempotencyKey, &event.Reason,
		&event.BeforeStatus, &event.AfterStatus, &event.Snapshot, &event.CreatedAt)
	if err != nil {
		return nil, mapPostgresStoreError("scan manual refund event", err)
	}
	if requestID.Valid {
		event.RequestID = requestID.String
	}
	return &event, nil
}

func (store *PostgresStore) GetManualRefundEventByIdempotencyKey(ctx context.Context, tenantID, key string) (*ManualRefundEvent, error) {
	return scanManualRefundEvent(store.runner.QueryRowContext(ctx, `SELECT id,tenant_id,refund_task_id,manual_refund_id,event_type,actor_id,request_id,idempotency_key,reason,before_status,after_status,event_snapshot,created_at FROM xz_operation_center_manual_refund_events WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, key))
}

func (store *PostgresStore) listRefundTaskIDs(ctx context.Context, filter RefundListFilter) ([]string, error) {
	where := []string{"1=1"}
	args := make([]any, 0, 10)
	add := func(condition string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(condition, len(args)))
	}
	if filter.TenantID != "" {
		add("task.tenant_id=$%d", filter.TenantID)
	}
	if filter.ServiceOrderID != "" {
		add("task.service_order_id=$%d", filter.ServiceOrderID)
	}
	if filter.ServiceStatus != "" {
		add("service.status=$%d", filter.ServiceStatus)
	}
	if filter.RefundStatus != "" {
		add("task.refund_status=$%d", filter.RefundStatus)
	}
	if filter.ProviderResult != "" {
		add("task.provider_outcome=$%d", filter.ProviderResult)
	}
	if filter.FailureClass != "" {
		add("task.failure_class=$%d", filter.FailureClass)
	}
	if filter.PaymentChannel != "" {
		add("lower(task.payment_channel)=lower($%d)", filter.PaymentChannel)
	}
	if filter.NextRetryBefore != nil {
		add("task.next_retry_at<=$%d", *filter.NextRetryBefore)
	}
	if filter.CreatedFrom != nil {
		add("task.created_at>=$%d", *filter.CreatedFrom)
	}
	if filter.CreatedTo != nil {
		add("task.created_at<=$%d", *filter.CreatedTo)
	}
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	args = append(args, limit, filter.Offset)
	query := `SELECT task.id FROM xz_operation_center_refund_tasks task JOIN xz_operation_center_service_orders service ON service.id=task.service_order_id WHERE ` + strings.Join(where, " AND ") + fmt.Sprintf(" ORDER BY task.created_at DESC,task.id LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	rows, err := store.runner.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, mapPostgresStoreError("list refund task ids", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func nullableText(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}

var _ = errors.Is
