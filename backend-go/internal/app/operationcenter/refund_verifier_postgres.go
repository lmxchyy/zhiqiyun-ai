package operationcenter

import (
	"context"
	"fmt"
	"strings"

	"xianzhi-ai/backend-go/internal/app/payment"
)

type preparedRefundVerification struct {
	service  OperationCenterServiceOrder
	task     OperationCenterRefundTask
	provider payment.RefundProvider
	request  payment.QueryRefundRequest
}

func (orchestrator *RefundOrchestrator) VerifyUnknownRefund(ctx context.Context, command RefundSagaCommand) (RefundVerificationResult, error) {
	prepared, result, err := orchestrator.prepareVerification(ctx, command)
	if err != nil || prepared == nil {
		return result, err
	}
	queryResult := payment.QueryRefundResult{Outcome: payment.QueryRefundUnsupported, ResponseSummary: payment.ProviderResponseSummary{"provider": strings.ToLower(prepared.task.PaymentChannel), "code": "PROVIDER_NOT_CONFIGURED"}}
	if prepared.provider != nil {
		queryResult, err = prepared.provider.QueryRefund(ctx, prepared.request)
		if err != nil {
			queryResult = payment.QueryRefundResult{Outcome: payment.QueryRefundUnknown, ResponseSummary: payment.ProviderResponseSummary{"provider": prepared.provider.GetProviderName(), "code": "QUERY_CALL_ERROR"}}
		}
		result.QueryCalled = true
	} else {
		result.QuerySkipped = true
	}
	return orchestrator.finalizeVerification(ctx, command, *prepared, queryResult, result)
}

func (orchestrator *RefundOrchestrator) prepareVerification(ctx context.Context, command RefundSagaCommand) (_ *preparedRefundVerification, result RefundVerificationResult, err error) {
	if orchestrator == nil || strings.TrimSpace(command.ServiceOrderID) == "" || strings.TrimSpace(command.RefundTaskID) == "" || strings.TrimSpace(command.OperatorID) == "" || strings.TrimSpace(command.TransactionGroupID) == "" {
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
	result = RefundVerificationResult{ServiceOrderID: service.ID, RefundTaskID: task.ID, RefundStatus: task.Status, ProviderRefundNo: stringValue(task.ProviderRefundNo)}
	if task.ServiceOrderID != service.ID || task.OrderID != service.OrderID {
		return nil, result, ErrRefundTaskMismatch
	}
	if task.Status != OperationCenterRefundUnknownVerifying {
		if task.Status == OperationCenterRefundSucceeded || task.Status == OperationCenterRefundManualRequired || task.Status == OperationCenterRefundProviderPending || task.Status == OperationCenterRefundRetryable {
			result.IdempotentReplay = true
			result.QuerySkipped = true
			return nil, result, nil
		}
		return nil, result, fmt.Errorf("%w: refund status %s", ErrRefundSagaNotReady, task.Status)
	}
	now, err := databaseNow(ctx, tx)
	if err != nil {
		return nil, result, err
	}
	if task.NextRetryAt != nil && task.NextRetryAt.After(now) {
		result.QuerySkipped = true
		return nil, result, nil
	}
	if task.LeaseExpiresAt != nil && task.LeaseExpiresAt.After(now) && (task.LeaseOwner == nil || *task.LeaseOwner != command.TransactionGroupID) {
		result.InProgress = true
		result.QuerySkipped = true
		return nil, result, nil
	}
	task.VerificationAttemptCount++
	task.LeaseOwner = optionalString(command.TransactionGroupID)
	task.LeaseExpiresAt = timePointer(now.Add(orchestrator.options.ProviderLeaseDuration))
	task.StateVersion++
	task.UpdatedAt = now
	if err := bound.UpdateRefundTaskVerification(ctx, task); err != nil {
		return nil, result, err
	}
	if err := tx.Commit(); err != nil {
		return nil, result, err
	}
	provider := orchestrator.providers[strings.ToLower(strings.TrimSpace(task.PaymentChannel))]
	prepared := &preparedRefundVerification{service: *service, task: *task, provider: provider, request: payment.QueryRefundRequest{
		OrderNo: service.OrderNo, PaymentNo: stringValue(task.PaymentRecordID), RefundNo: stableProviderRefundNo(*task),
		ProviderRefundID: stringValue(task.ProviderRefundNo), PayerID: service.ApplicantUserID,
	}}
	return prepared, result, nil
}

func (orchestrator *RefundOrchestrator) finalizeVerification(ctx context.Context, command RefundSagaCommand, prepared preparedRefundVerification, query payment.QueryRefundResult, result RefundVerificationResult) (RefundVerificationResult, error) {
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
	if task.Status != OperationCenterRefundUnknownVerifying {
		result.IdempotentReplay = true
		result.RefundStatus = task.Status
		return result, nil
	}
	if task.LeaseOwner == nil || *task.LeaseOwner != command.TransactionGroupID {
		return result, ErrRefundSagaInProgress
	}
	now, err := databaseNow(ctx, tx)
	if err != nil {
		return result, err
	}
	query.Outcome = normalizeQueryRefundOutcome(query)
	task.ProviderQueryOutcome = &query.Outcome
	task.ProviderQueryResponseSummary = snapshotProviderSummary(query.ResponseSummary)
	task.LastVerificationAt = timePointer(now)
	task.LeaseOwner = nil
	task.LeaseExpiresAt = nil
	task.NextRetryAt = nil
	if strings.TrimSpace(query.ProviderRefundID) != "" {
		task.ProviderRefundNo = optionalString(query.ProviderRefundID)
	}
	if task.UnknownSince == nil {
		task.UnknownSince = timePointer(now)
	}
	transitioned := false
	switch query.Outcome {
	case payment.QueryRefundSucceeded:
		if task.ProviderRefundNo == nil {
			query.Outcome = payment.QueryRefundUnknown
			task.ProviderQueryOutcome = &query.Outcome
			task.NextRetryAt = timePointer(now.Add(orchestrator.options.VerificationInterval))
			break
		}
		providerOutcome := RefundProviderSuccess
		task.ProviderOutcome = &providerOutcome
		task.CompletedAt = timePointer(now)
		task.ProviderRefundedAt = timePointer(now)
		task.FailureClass = nil
		task.FailureDetail = JSONSnapshot{}
		task.UnknownSince = nil
		if err := advanceRefundTask(ctx, bound, task, OperationCenterRefundSucceeded, "REFUND_QUERY_SUCCEEDED", command, now); err != nil {
			return result, err
		}
		transitioned = true
	case payment.QueryRefundNotFound:
		if now.Sub(*task.UnknownSince) >= orchestrator.options.UnknownSafetyWait {
			task.FailureClass = nil
			task.FailureDetail = JSONSnapshot{"queryOutcome": string(query.Outcome)}
			task.UnknownSince = nil
			if err := advanceRefundTask(ctx, bound, task, OperationCenterRefundProviderPending, "REFUND_QUERY_NOT_FOUND_SAFE_RETRY", command, now); err != nil {
				return result, err
			}
			transitioned = true
		} else {
			next := task.UnknownSince.Add(orchestrator.options.UnknownSafetyWait)
			task.NextRetryAt = timePointer(next)
		}
	case payment.QueryRefundProcessing, payment.QueryRefundUnknown:
		task.NextRetryAt = timePointer(now.Add(orchestrator.options.VerificationInterval))
	case payment.QueryRefundFailed:
		failure := RefundFailureTemporary
		task.FailureClass = &failure
		task.NextRetryAt = timePointer(now.Add(orchestrator.options.TemporaryRetryDelay))
		if err := advanceRefundTask(ctx, bound, task, OperationCenterRefundRetryable, "REFUND_QUERY_FAILED_RETRYABLE", command, now); err != nil {
			return result, err
		}
		transitioned = true
	case payment.QueryRefundUnsupported:
		failure := RefundFailureProviderUnsupported
		task.FailureClass = &failure
		if err := advanceRefundTask(ctx, bound, task, OperationCenterRefundManualRequired, "REFUND_QUERY_UNSUPPORTED", command, now); err != nil {
			return result, err
		}
		transitioned = true
	}
	if !transitioned {
		task.StateVersion++
		task.UpdatedAt = now
		if err := bound.UpdateRefundTaskVerification(ctx, task); err != nil {
			return result, err
		}
	}
	service.RefundStatus = task.Status
	service.ProviderRefundNo = task.ProviderRefundNo
	service.RefundFailureClass = task.FailureClass
	service.RefundFailureDetail = task.FailureDetail
	service.NextRefundRetryAt = task.NextRetryAt
	service.StateVersion++
	service.UpdatedAt = now
	if err := bound.UpdateServiceOrderRefundProjection(ctx, service); err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	result.RefundStatus = task.Status
	result.QueryOutcome = query.Outcome
	result.ProviderRefundNo = stringValue(task.ProviderRefundNo)
	return result, nil
}

func normalizeQueryRefundOutcome(result payment.QueryRefundResult) payment.QueryRefundOutcome {
	switch result.Outcome {
	case payment.QueryRefundSucceeded, payment.QueryRefundNotFound, payment.QueryRefundProcessing, payment.QueryRefundFailed, payment.QueryRefundUnsupported, payment.QueryRefundUnknown:
		return result.Outcome
	default:
		return payment.QueryRefundUnknown
	}
}
