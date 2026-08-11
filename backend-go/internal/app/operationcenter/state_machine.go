package operationcenter

import "fmt"

type OperationCenterServiceStatus string

const (
	OperationCenterServicePendingPayment        OperationCenterServiceStatus = "PENDING_PAYMENT"
	OperationCenterServiceReviewRequired        OperationCenterServiceStatus = "REVIEW_REQUIRED"
	OperationCenterServiceActive                OperationCenterServiceStatus = "ACTIVE"
	OperationCenterServiceRejected              OperationCenterServiceStatus = "REJECTED"
	OperationCenterServiceRevoking              OperationCenterServiceStatus = "REVOKING"
	OperationCenterServiceRevoked               OperationCenterServiceStatus = "REVOKED"
	OperationCenterServiceLegacyRefundReversing OperationCenterServiceStatus = "REFUND_REVERSING"
	OperationCenterServiceLegacyRefunding       OperationCenterServiceStatus = "REFUNDING"
	OperationCenterServiceLegacyRefunded        OperationCenterServiceStatus = "REFUNDED"
)

type OperationCenterRefundStatus string

const (
	OperationCenterRefundNone             OperationCenterRefundStatus = "NONE"
	OperationCenterRefundPending          OperationCenterRefundStatus = "PENDING"
	OperationCenterRefundReversing        OperationCenterRefundStatus = "REVERSING"
	OperationCenterRefundProviderPending  OperationCenterRefundStatus = "PROVIDER_PENDING"
	OperationCenterRefundRetryable        OperationCenterRefundStatus = "REFUND_RETRYABLE"
	OperationCenterRefundUnknownVerifying OperationCenterRefundStatus = "UNKNOWN_VERIFYING"
	OperationCenterRefundManualRequired   OperationCenterRefundStatus = "MANUAL_REQUIRED"
	OperationCenterRefundManualSubmitted  OperationCenterRefundStatus = "MANUAL_SUBMITTED"
	OperationCenterRefundSucceeded        OperationCenterRefundStatus = "SUCCEEDED"
	OperationCenterRefundCancelled        OperationCenterRefundStatus = "CANCELLED"
)

// RefundTaskStatus shares the canonical database values with the service-order refund status.
type RefundTaskStatus = OperationCenterRefundStatus

type RefundProviderResult string

const (
	RefundProviderSuccess          RefundProviderResult = "SUCCESS"
	RefundProviderTemporaryFailure RefundProviderResult = "TEMPORARY_FAILURE"
	RefundProviderUnsupported      RefundProviderResult = "UNSUPPORTED"
	RefundProviderUnknown          RefundProviderResult = "UNKNOWN"
)

type ReviewDecision string

const (
	ReviewApproved ReviewDecision = "APPROVED"
	ReviewRejected ReviewDecision = "REJECTED"
)

type ReviewEventStatus string

const (
	ReviewEventPending ReviewEventStatus = "PENDING"
	ReviewEventApplied ReviewEventStatus = "APPLIED"
	ReviewEventFailed  ReviewEventStatus = "FAILED"
)

type ManualRefundStatus string

const (
	ManualRefundSubmitted ManualRefundStatus = "SUBMITTED"
	ManualRefundApproved  ManualRefundStatus = "APPROVED"
	ManualRefundRejected  ManualRefundStatus = "REJECTED"
)

type ReferralRewardReleaseTaskStatus string

const (
	ReferralRewardReleasePending    ReferralRewardReleaseTaskStatus = "PENDING"
	ReferralRewardReleaseProcessing ReferralRewardReleaseTaskStatus = "PROCESSING"
	ReferralRewardReleaseSucceeded  ReferralRewardReleaseTaskStatus = "SUCCEEDED"
	ReferralRewardReleaseFailed     ReferralRewardReleaseTaskStatus = "FAILED"
	ReferralRewardReleaseCancelled  ReferralRewardReleaseTaskStatus = "CANCELLED"
)

type RefundOrigin string

const (
	RefundOriginReviewRejection  RefundOrigin = "REVIEW_REJECTION"
	RefundOriginActiveRevocation RefundOrigin = "ACTIVE_REVOCATION"
)

type RefundScope string

const RefundScopeFull RefundScope = "FULL"

type RefundFailureClass string

const (
	RefundFailureProviderUnsupported RefundFailureClass = "PROVIDER_UNSUPPORTED"
	RefundFailureTemporary           RefundFailureClass = "TEMPORARY_FAILURE"
	RefundFailureUnknown             RefundFailureClass = "UNKNOWN"
	RefundFailureManualRequired      RefundFailureClass = "MANUAL_REQUIRED"
	RefundFailureValidation          RefundFailureClass = "VALIDATION_FAILURE"
)

type StateTransitionEntityType string

const (
	StateEntityServiceOrder      StateTransitionEntityType = "SERVICE_ORDER"
	StateEntityRefundTask        StateTransitionEntityType = "REFUND_TASK"
	StateEntityReferralReward    StateTransitionEntityType = "REFERRAL_REWARD"
	StateEntityRewardReleaseTask StateTransitionEntityType = "REWARD_RELEASE_TASK"
)

func DatabaseOperationCenterServiceStatuses() []OperationCenterServiceStatus {
	return []OperationCenterServiceStatus{
		OperationCenterServicePendingPayment,
		OperationCenterServiceReviewRequired,
		OperationCenterServiceActive,
		OperationCenterServiceRejected,
		OperationCenterServiceRevoking,
		OperationCenterServiceLegacyRefundReversing,
		OperationCenterServiceLegacyRefunding,
		OperationCenterServiceLegacyRefunded,
		OperationCenterServiceRevoked,
	}
}

func DatabaseOperationCenterRefundStatuses() []OperationCenterRefundStatus {
	return []OperationCenterRefundStatus{
		OperationCenterRefundNone,
		OperationCenterRefundPending,
		OperationCenterRefundReversing,
		OperationCenterRefundProviderPending,
		OperationCenterRefundRetryable,
		OperationCenterRefundUnknownVerifying,
		OperationCenterRefundManualRequired,
		OperationCenterRefundManualSubmitted,
		OperationCenterRefundSucceeded,
		OperationCenterRefundCancelled,
	}
}

func ValidateOperationCenterServiceTransition(from, to OperationCenterServiceStatus) error {
	valid := map[OperationCenterServiceStatus]map[OperationCenterServiceStatus]struct{}{
		OperationCenterServicePendingPayment: {OperationCenterServiceReviewRequired: {}},
		OperationCenterServiceReviewRequired: {
			OperationCenterServiceActive:   {},
			OperationCenterServiceRejected: {},
		},
		OperationCenterServiceActive:   {OperationCenterServiceRevoking: {}},
		OperationCenterServiceRevoking: {OperationCenterServiceRevoked: {}},
	}
	if targets, ok := valid[from]; ok {
		if _, ok := targets[to]; ok {
			return nil
		}
	}
	return fmt.Errorf("%w: %s -> %s", ErrInvalidServiceTransition, from, to)
}

func ValidateOperationCenterRefundTransition(from, to OperationCenterRefundStatus) error {
	valid := map[OperationCenterRefundStatus]map[OperationCenterRefundStatus]struct{}{
		OperationCenterRefundNone:      {OperationCenterRefundPending: {}},
		OperationCenterRefundPending:   {OperationCenterRefundReversing: {}, OperationCenterRefundCancelled: {}},
		OperationCenterRefundReversing: {OperationCenterRefundProviderPending: {}},
		OperationCenterRefundProviderPending: {
			OperationCenterRefundSucceeded:        {},
			OperationCenterRefundRetryable:        {},
			OperationCenterRefundUnknownVerifying: {},
			OperationCenterRefundManualRequired:   {},
		},
		OperationCenterRefundRetryable: {OperationCenterRefundProviderPending: {}, OperationCenterRefundManualRequired: {}},
		OperationCenterRefundUnknownVerifying: {
			OperationCenterRefundProviderPending: {},
			OperationCenterRefundRetryable:       {},
			OperationCenterRefundManualRequired:  {},
			OperationCenterRefundSucceeded:       {},
		},
		OperationCenterRefundManualRequired:  {OperationCenterRefundManualSubmitted: {}},
		OperationCenterRefundManualSubmitted: {OperationCenterRefundSucceeded: {}, OperationCenterRefundManualRequired: {}},
	}
	if targets, ok := valid[from]; ok {
		if _, ok := targets[to]; ok {
			return nil
		}
	}
	return fmt.Errorf("%w: %s -> %s", ErrInvalidRefundTransition, from, to)
}

func OperationCenterRefundStatusForProviderResult(result RefundProviderResult) (OperationCenterRefundStatus, error) {
	switch result {
	case RefundProviderSuccess:
		return OperationCenterRefundSucceeded, nil
	case RefundProviderTemporaryFailure:
		return OperationCenterRefundRetryable, nil
	case RefundProviderUnsupported:
		return OperationCenterRefundManualRequired, nil
	case RefundProviderUnknown:
		return OperationCenterRefundUnknownVerifying, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrInvalidProviderResult, result)
	}
}

func isDatabaseServiceStatus(status OperationCenterServiceStatus) bool {
	for _, candidate := range DatabaseOperationCenterServiceStatuses() {
		if candidate == status {
			return true
		}
	}
	return false
}

func isDatabaseRefundStatus(status OperationCenterRefundStatus) bool {
	for _, candidate := range DatabaseOperationCenterRefundStatuses() {
		if candidate == status {
			return true
		}
	}
	return false
}
