package operationcenter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/app/payment"
)

type RefundOrchestrator struct {
	db        *sql.DB
	store     *PostgresStore
	reversal  *ReferralRewardReversalService
	providers map[string]payment.RefundProvider
	options   RefundOrchestratorOptions
}

type preparedProviderRefund struct {
	service      OperationCenterServiceOrder
	task         OperationCenterRefundTask
	provider     payment.RefundProvider
	request      payment.RefundPaymentRequest
	reversal     *ReferralRewardReversalResult
	providerName string
}

type providerRefundExecution struct {
	result       payment.RefundPaymentResult
	err          error
	outcome      RefundProviderResult
	providerName string
}

func NewRefundOrchestrator(db *sql.DB, store *PostgresStore, reversal *ReferralRewardReversalService, providers map[string]payment.RefundProvider, options RefundOrchestratorOptions) (*RefundOrchestrator, error) {
	if db == nil || store == nil || reversal == nil || options.ProviderLeaseDuration <= 0 || options.TemporaryRetryDelay <= 0 || options.UnknownSafetyWait <= 0 || options.VerificationInterval <= 0 {
		return nil, ErrConstraintViolation
	}
	configured := make(map[string]payment.RefundProvider, len(providers))
	for name, provider := range providers {
		if provider != nil {
			configured[strings.ToLower(strings.TrimSpace(name))] = provider
		}
	}
	return &RefundOrchestrator{db: db, store: store, reversal: reversal, providers: configured, options: options}, nil
}

func (orchestrator *RefundOrchestrator) Execute(ctx context.Context, command RefundSagaCommand) (RefundSagaResult, error) {
	prepared, result, err := orchestrator.prepare(ctx, command)
	if err != nil || prepared == nil {
		return result, err
	}
	execution := orchestrator.callProvider(ctx, *prepared)
	result, err = orchestrator.finalize(ctx, command, *prepared, execution)
	result.ProviderCalled = prepared.provider != nil
	if prepared.provider == nil {
		result.ProviderCallSkipped = true
	}
	return result, err
}

func (orchestrator *RefundOrchestrator) prepare(ctx context.Context, command RefundSagaCommand) (_ *preparedProviderRefund, result RefundSagaResult, err error) {
	if orchestrator == nil || orchestrator.db == nil || strings.TrimSpace(command.ServiceOrderID) == "" || strings.TrimSpace(command.RefundTaskID) == "" || strings.TrimSpace(command.OperatorID) == "" || strings.TrimSpace(command.TransactionGroupID) == "" || strings.TrimSpace(command.Reason) == "" {
		return nil, result, ErrConstraintViolation
	}
	tx, err := orchestrator.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, result, err
	}
	defer func() { _ = tx.Rollback() }()
	bound, err := orchestrator.store.BindTx(tx)
	if err != nil {
		return nil, result, err
	}
	service, err := bound.GetServiceOrderForUpdate(ctx, command.ServiceOrderID)
	if err != nil {
		return nil, result, err
	}
	task, err := bound.GetRefundTaskForUpdate(ctx, command.RefundTaskID)
	if err != nil {
		return nil, result, err
	}
	result = sagaResult(*service, *task)
	if task.ServiceOrderID != service.ID || task.OrderID != service.OrderID || task.TenantID != service.TenantID || task.Scope != RefundScopeFull || task.AmountCents != service.TechnicalServiceFeeCents || task.Currency != service.Currency {
		return nil, result, ErrRefundTaskMismatch
	}
	now, err := databaseNow(ctx, tx)
	if err != nil {
		return nil, result, err
	}
	switch task.Status {
	case OperationCenterRefundSucceeded:
		result.IdempotentReplay = true
		return nil, result, nil
	case OperationCenterRefundUnknownVerifying, OperationCenterRefundManualRequired, OperationCenterRefundManualSubmitted:
		result.IdempotentReplay = true
		result.ProviderCallSkipped = true
		return nil, result, nil
	case OperationCenterRefundRetryable:
		if task.NextRetryAt != nil && task.NextRetryAt.After(now) {
			result.ProviderCallSkipped = true
			return nil, result, nil
		}
	case OperationCenterRefundProviderPending:
		if task.LeaseExpiresAt != nil && task.LeaseExpiresAt.After(now) {
			result.InProgress = true
			result.ProviderCallSkipped = true
			return nil, result, nil
		}
	case OperationCenterRefundPending:
	default:
		return nil, result, fmt.Errorf("%w: refund status %s", ErrRefundSagaNotReady, task.Status)
	}

	var reversalResult *ReferralRewardReversalResult
	if task.Status == OperationCenterRefundPending {
		if err := advanceRefundTask(ctx, bound, task, OperationCenterRefundReversing, "REFUND_SAGA_REVERSING", command, now); err != nil {
			return nil, result, err
		}
		switch task.Origin {
		case RefundOriginActiveRevocation:
			if service.Status != OperationCenterServiceActive {
				return nil, result, fmt.Errorf("%w: active revocation requires ACTIVE service", ErrRefundSagaNotReady)
			}
			service.RefundStatus = OperationCenterRefundReversing
			service.CurrentRefundTaskID = optionalString(task.ID)
			service.RefundIdempotencyKey = optionalString(task.IdempotencyKey)
			service.RefundOrderID = optionalString(task.ID)
			if err := advanceRefundService(ctx, bound, service, OperationCenterServiceRevoking, "OPERATION_CENTER_REFUND_REVERSING", command, now); err != nil {
				return nil, result, err
			}
			if err := revokeOperationCenterResourcesTx(ctx, tx, service, command, now); err != nil {
				return nil, result, err
			}
			eventID, eventStatus, eventErr := lockReferralEventForRefund(ctx, tx, service.OrderID)
			if eventErr != nil {
				return nil, result, eventErr
			}
			if eventID != "" {
				if eventStatus != "REWARDED" && eventStatus != "REVERSED" && eventStatus != "CANCELLED" {
					return nil, result, fmt.Errorf("%w: referral event status %s", ErrRefundSagaNotReady, eventStatus)
				}
				if eventStatus != "CANCELLED" {
					reversed, reverseErr := orchestrator.reversal.ReverseReferralRewardsForRefund(ctx, tx, ReferralRewardReversalCommand{
						RefundTaskID: task.ID, OperationCenterServiceOrderID: service.ID, ReferralEventID: eventID,
						RefundAmountCents: task.AmountCents, ReversalReason: command.Reason,
						OperatorID: command.OperatorID, TransactionGroupID: command.TransactionGroupID,
					})
					if reverseErr != nil {
						return nil, result, reverseErr
					}
					reversalResult = &reversed
				}
			}
			service.RefundStatus = OperationCenterRefundProviderPending
			service.RevokedAt = timePointer(now)
			if err := advanceRefundService(ctx, bound, service, OperationCenterServiceRevoked, "OPERATION_CENTER_REFUND_REVOKED", command, now); err != nil {
				return nil, result, err
			}
		case RefundOriginReviewRejection:
			if service.Status != OperationCenterServiceRejected {
				return nil, result, fmt.Errorf("%w: review refund requires REJECTED service", ErrRefundSagaNotReady)
			}
		default:
			return nil, result, ErrRefundTaskMismatch
		}
	}

	leaseUntil := now.Add(orchestrator.options.ProviderLeaseDuration)
	if task.Status == OperationCenterRefundProviderPending {
		task.AttemptCount++
		task.LeaseOwner = optionalString(command.TransactionGroupID)
		task.LeaseExpiresAt = timePointer(leaseUntil)
		if task.PreparedAt == nil {
			task.PreparedAt = timePointer(now)
		}
		task.StateVersion++
		task.UpdatedAt = now
		if err := bound.UpdateRefundTaskLease(ctx, task); err != nil {
			return nil, result, err
		}
	} else {
		task.AttemptCount++
		task.LeaseOwner = optionalString(command.TransactionGroupID)
		task.LeaseExpiresAt = timePointer(leaseUntil)
		if task.PreparedAt == nil {
			task.PreparedAt = timePointer(now)
		}
		if err := advanceRefundTask(ctx, bound, task, OperationCenterRefundProviderPending, "REFUND_PROVIDER_PENDING", command, now); err != nil {
			return nil, result, err
		}
	}
	if service.Status == OperationCenterServiceRejected {
		service.RefundStatus = OperationCenterRefundProviderPending
		service.CurrentRefundTaskID = optionalString(task.ID)
		service.RefundIdempotencyKey = optionalString(task.IdempotencyKey)
		service.RefundOrderID = optionalString(task.ID)
		service.RefundAttemptCount = task.AttemptCount
		service.NextRefundRetryAt = nil
		service.StateVersion++
		service.UpdatedAt = now
		if err := bound.UpdateServiceOrderRefundProjection(ctx, service); err != nil {
			return nil, result, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, result, err
	}
	providerName := strings.ToLower(strings.TrimSpace(task.PaymentChannel))
	provider := orchestrator.providers[providerName]
	prepared := &preparedProviderRefund{
		service: *service, task: *task, provider: provider, providerName: providerName, reversal: reversalResult,
		request: payment.RefundPaymentRequest{
			OrderNo: service.OrderNo, PaymentNo: stringValue(task.PaymentRecordID), RefundNo: stableProviderRefundNo(*task),
			Amount: task.AmountCents, Currency: task.Currency, Description: command.Reason,
		},
	}
	return prepared, sagaResult(*service, *task), nil
}

func (orchestrator *RefundOrchestrator) callProvider(ctx context.Context, prepared preparedProviderRefund) providerRefundExecution {
	if prepared.provider == nil {
		err := NewRefundProviderCallError(RefundProviderUnsupported, fmt.Errorf("payment provider %q is not configured", prepared.providerName))
		return providerRefundExecution{err: err, outcome: RefundProviderUnsupported, providerName: prepared.providerName}
	}
	result, err := prepared.provider.RefundPayment(ctx, prepared.request)
	return providerRefundExecution{result: result, err: err, outcome: classifyRefundProviderCall(result, err), providerName: prepared.provider.GetProviderName()}
}

func (orchestrator *RefundOrchestrator) finalize(ctx context.Context, command RefundSagaCommand, prepared preparedProviderRefund, execution providerRefundExecution) (result RefundSagaResult, err error) {
	tx, err := orchestrator.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	bound, err := orchestrator.store.BindTx(tx)
	if err != nil {
		return result, err
	}
	service, err := bound.GetServiceOrderForUpdate(ctx, command.ServiceOrderID)
	if err != nil {
		return result, err
	}
	task, err := bound.GetRefundTaskForUpdate(ctx, command.RefundTaskID)
	if err != nil {
		return result, err
	}
	result = sagaResult(*service, *task)
	if task.Status != OperationCenterRefundProviderPending {
		result.IdempotentReplay = true
		return result, nil
	}
	if task.LeaseOwner == nil || *task.LeaseOwner != command.TransactionGroupID {
		return result, ErrRefundSagaInProgress
	}
	now, err := databaseNow(ctx, tx)
	if err != nil {
		return result, err
	}
	outcome := execution.outcome
	if outcome == RefundProviderSuccess && strings.TrimSpace(execution.result.ProviderRefundID) == "" {
		outcome = RefundProviderUnknown
	}
	task.ProviderOutcome = &outcome
	task.LeaseOwner = nil
	task.LeaseExpiresAt = nil
	task.NextRetryAt = nil
	task.FailureClass = nil
	task.FailureDetail = JSONSnapshot{"provider": execution.providerName}
	task.ProviderResponseSummary = snapshotProviderSummary(execution.result.ResponseSummary)
	if execution.err != nil {
		task.FailureDetail["message"] = execution.err.Error()
	}
	var target OperationCenterRefundStatus
	switch outcome {
	case RefundProviderSuccess:
		target = OperationCenterRefundSucceeded
		task.ProviderRefundNo = optionalString(execution.result.ProviderRefundID)
		task.CompletedAt = timePointer(now)
		task.ProviderRefundedAt = timePointer(now)
		task.UnknownSince = nil
	case RefundProviderTemporaryFailure:
		target = OperationCenterRefundRetryable
		failure := RefundFailureTemporary
		task.FailureClass = &failure
		task.NextRetryAt = timePointer(now.Add(orchestrator.options.TemporaryRetryDelay))
	case RefundProviderUnsupported:
		target = OperationCenterRefundManualRequired
		failure := RefundFailureProviderUnsupported
		task.FailureClass = &failure
	case RefundProviderUnknown:
		target = OperationCenterRefundUnknownVerifying
		failure := RefundFailureUnknown
		task.FailureClass = &failure
		task.UnknownSince = timePointer(now)
	default:
		return result, ErrRefundProviderInvariant
	}
	if err := advanceRefundTask(ctx, bound, task, target, "REFUND_PROVIDER_RESULT_"+string(outcome), command, now); err != nil {
		return result, err
	}
	service.RefundStatus = target
	service.ProviderRefundNo = task.ProviderRefundNo
	service.RefundFailureClass = task.FailureClass
	service.RefundFailureDetail = task.FailureDetail
	service.RefundAttemptCount = task.AttemptCount
	service.NextRefundRetryAt = task.NextRetryAt
	service.CurrentRefundTaskID = optionalString(task.ID)
	service.StateVersion++
	service.UpdatedAt = now
	if service.Status == OperationCenterServiceActive {
		return result, ErrRefundProviderInvariant
	}
	if err := bound.UpdateServiceOrderRefundProjection(ctx, service); err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	result = sagaResult(*service, *task)
	result.ProviderOutcome = &outcome
	result.ProviderRefundNo = stringValue(task.ProviderRefundNo)
	result.RewardReversal = prepared.reversal
	return result, nil
}

func advanceRefundTask(ctx context.Context, store Store, task *OperationCenterRefundTask, target OperationCenterRefundStatus, reason string, command RefundSagaCommand, now time.Time) error {
	previous := task.Status
	if err := ValidateOperationCenterRefundTransition(previous, target); err != nil {
		return err
	}
	transitionVersion := task.StateVersion + 1
	task.Status = target
	task.StateVersion = transitionVersion
	task.UpdatedAt = now
	transition := OperationCenterStateTransition{
		ID: stableWorkflowID("refund_saga_task_transition", task.ID, fmt.Sprint(transitionVersion)), TenantID: task.TenantID,
		EntityType: StateEntityRefundTask, EntityID: task.ID, FromStatus: optionalString(string(previous)), ToStatus: string(target),
		TransitionReason: reason, TransactionGroupID: command.TransactionGroupID, OperatorID: optionalString(command.OperatorID), RequestID: optionalString(command.RequestID),
		IdempotencyKey: fmt.Sprintf("refund-saga-task:%s:%d", task.ID, transitionVersion), Metadata: JSONSnapshot{"reason": command.Reason}, CreatedAt: now,
	}
	return store.UpdateRefundTask(ctx, task, &transition)
}

func advanceRefundService(ctx context.Context, store Store, service *OperationCenterServiceOrder, target OperationCenterServiceStatus, reason string, command RefundSagaCommand, now time.Time) error {
	previous := service.Status
	if err := ValidateOperationCenterServiceTransition(previous, target); err != nil {
		return err
	}
	service.Status = target
	service.StateVersion++
	service.UpdatedAt = now
	transition := OperationCenterStateTransition{
		ID: stableWorkflowID("refund_saga_service_transition", service.ID, string(target)), TenantID: service.TenantID,
		EntityType: StateEntityServiceOrder, EntityID: service.ID, FromStatus: optionalString(string(previous)), ToStatus: string(target),
		TransitionReason: reason, TransactionGroupID: command.TransactionGroupID, OperatorID: optionalString(command.OperatorID), RequestID: optionalString(command.RequestID),
		IdempotencyKey: "refund-saga-service:" + service.ID + ":" + string(target), Metadata: JSONSnapshot{"refundTaskId": command.RefundTaskID}, CreatedAt: now,
	}
	return store.UpdateServiceOrder(ctx, service, &transition)
}

func revokeOperationCenterResourcesTx(ctx context.Context, tx *sql.Tx, service *OperationCenterServiceOrder, command RefundSagaCommand, now time.Time) error {
	if service.PlanVersionID == nil {
		return ErrFrozenSnapshotMissing
	}
	var role string
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(config->>'rbacRole','') FROM xz_commercial_plan_versions WHERE id=$1 AND identity_type='OPERATION_CENTER'`, *service.PlanVersionID).Scan(&role); err != nil {
		return err
	}
	if strings.TrimSpace(role) == "" {
		return fmt.Errorf("operation center RBAC role is not configured: %w", ErrConstraintViolation)
	}
	identityResult, err := tx.ExecContext(ctx, `UPDATE xz_user_business_identities SET identity_status='TERMINATED',commission_enabled=false,ended_at=$3,status_reason=$4,updated_at=$3 WHERE tenant_id=$1 AND user_id=$2 AND identity_type='OPERATION_CENTER' AND identity_status='ACTIVE'`, service.TenantID, service.ApplicantUserID, now, command.Reason)
	if err != nil {
		return err
	}
	if count, countErr := identityResult.RowsAffected(); countErr != nil || count != 1 {
		if countErr != nil {
			return countErr
		}
		return fmt.Errorf("active operation center identity not found: %w", ErrConstraintViolation)
	}
	if result, err := tx.ExecContext(ctx, `UPDATE xz_users SET operation_center_status='REVOKED',updated_at=$2,raw=jsonb_set(coalesce(raw,'{}'::jsonb),'{operationCenterStatus}','"REVOKED"'::jsonb,true) WHERE id=$1`, service.ApplicantUserID, now.Format(time.RFC3339Nano)); err != nil {
		return err
	} else if count, countErr := result.RowsAffected(); countErr != nil || count != 1 {
		return ErrConstraintViolation
	}
	if result, err := tx.ExecContext(ctx, `UPDATE xz_operation_centers SET status='REVOKED',updated_at=$2 WHERE user_id=$1 AND status='ACTIVE'`, service.ApplicantUserID, now.Format(time.RFC3339Nano)); err != nil {
		return err
	} else if count, countErr := result.RowsAffected(); countErr != nil || count != 1 {
		return ErrConstraintViolation
	}
	roleResult, err := tx.ExecContext(ctx, `UPDATE xz_user_roles SET status='INACTIVE',updated_at=$4 WHERE user_id=$1 AND tenant_id=$2 AND role=$3 AND upper(status)='ACTIVE'`, service.ApplicantUserID, service.TenantID, role, now)
	if err != nil {
		return err
	}
	if count, countErr := roleResult.RowsAffected(); countErr != nil || count == 0 {
		return ErrConstraintViolation
	}
	if _, err := tx.ExecContext(ctx, `UPDATE xz_user_role_context SET current_role_code='USER',updated_at=$3 WHERE user_id=$1 AND tenant_id=$2 AND current_role_code=$4`, service.ApplicantUserID, service.TenantID, now, role); err != nil {
		return err
	}
	return nil
}

func lockReferralEventForRefund(ctx context.Context, tx *sql.Tx, orderID string) (string, string, error) {
	var id, status string
	err := tx.QueryRowContext(ctx, `SELECT id,status FROM xz_referral_events WHERE source_order_id=$1 FOR UPDATE`, orderID).Scan(&id, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	return id, status, err
}

func sagaResult(service OperationCenterServiceOrder, task OperationCenterRefundTask) RefundSagaResult {
	return RefundSagaResult{ServiceOrderID: service.ID, RefundTaskID: task.ID, ServiceStatus: service.Status, RefundStatus: task.Status, ProviderRefundNo: stringValue(task.ProviderRefundNo), ProviderOutcome: task.ProviderOutcome}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
