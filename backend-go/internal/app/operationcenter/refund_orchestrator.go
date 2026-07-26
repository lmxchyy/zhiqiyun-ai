package operationcenter

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/app/payment"
)

type RefundOrchestratorOptions struct {
	ProviderLeaseDuration time.Duration
	TemporaryRetryDelay   time.Duration
	UnknownSafetyWait     time.Duration
	VerificationInterval  time.Duration
}

type RefundSagaCommand struct {
	ServiceOrderID, RefundTaskID string
	OperatorID, RequestID        string
	TransactionGroupID           string
	Reason                       string
}

type RefundSagaResult struct {
	ServiceOrderID, RefundTaskID, ProviderRefundNo string
	ServiceStatus                                  OperationCenterServiceStatus
	RefundStatus                                   OperationCenterRefundStatus
	ProviderOutcome                                *RefundProviderResult
	ProviderCalled, ProviderCallSkipped            bool
	IdempotentReplay, InProgress                   bool
	RewardReversal                                 *ReferralRewardReversalResult
}

type RefundProviderCallError struct {
	Outcome RefundProviderResult
	Cause   error
}

func (item *RefundProviderCallError) Error() string {
	if item == nil || item.Cause == nil {
		return "refund provider call failed"
	}
	return item.Cause.Error()
}

func (item *RefundProviderCallError) Unwrap() error {
	if item == nil {
		return nil
	}
	return item.Cause
}

func NewRefundProviderCallError(outcome RefundProviderResult, cause error) error {
	if outcome != RefundProviderTemporaryFailure && outcome != RefundProviderUnsupported && outcome != RefundProviderUnknown {
		return fmt.Errorf("%w: %s", ErrInvalidProviderResult, outcome)
	}
	if cause == nil {
		cause = errors.New("refund provider call failed")
	}
	return &RefundProviderCallError{Outcome: outcome, Cause: cause}
}

func classifyRefundProviderCall(result payment.RefundPaymentResult, callErr error) RefundProviderResult {
	switch result.Outcome {
	case payment.RefundSuccess:
		if strings.TrimSpace(result.ProviderRefundID) != "" {
			return RefundProviderSuccess
		}
		return RefundProviderUnknown
	case payment.RefundTemporaryFailure:
		return RefundProviderTemporaryFailure
	case payment.RefundUnsupported:
		return RefundProviderUnsupported
	case payment.RefundUnknown:
		return RefundProviderUnknown
	}
	if callErr == nil {
		if result.Status == payment.PaymentRefunded && strings.TrimSpace(result.ProviderRefundID) != "" {
			return RefundProviderSuccess
		}
		return RefundProviderUnknown
	}
	var classified *RefundProviderCallError
	if errors.As(callErr, &classified) {
		switch classified.Outcome {
		case RefundProviderTemporaryFailure, RefundProviderUnsupported, RefundProviderUnknown:
			return classified.Outcome
		}
	}
	return RefundProviderUnknown
}

func stableProviderRefundNo(task OperationCenterRefundTask) string {
	return stableWorkflowID("operation_center_provider_refund", task.ID, task.IdempotencyKey)
}
