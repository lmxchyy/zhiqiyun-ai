package operationcenter

import "errors"

var (
	ErrInvalidServiceTransition       = errors.New("invalid operation center service transition")
	ErrInvalidRefundTransition        = errors.New("invalid operation center refund transition")
	ErrInvalidProviderResult          = errors.New("invalid refund provider result")
	ErrInvalidJSONSnapshot            = errors.New("invalid JSON snapshot")
	ErrIdempotencyConflict            = errors.New("operation center idempotency conflict")
	ErrUniqueConflict                 = errors.New("operation center unique conflict")
	ErrForeignKeyConflict             = errors.New("operation center foreign key conflict")
	ErrConstraintViolation            = errors.New("operation center constraint violation")
	ErrNotFound                       = errors.New("operation center record not found")
	ErrTransactionRequired            = errors.New("operation center transaction required")
	ErrManualRefundSelfApproval       = errors.New("manual refund submitter cannot approve the same refund")
	ErrRefundSagaInProgress           = errors.New("operation center refund saga is already in progress")
	ErrRefundSagaNotReady             = errors.New("operation center refund saga is not ready")
	ErrRefundTaskMismatch             = errors.New("operation center refund task does not match service order")
	ErrRefundProviderInvariant        = errors.New("operation center refund provider result violates invariants")
	ErrRefundPolicyNotFullOnly        = errors.New("operation center refund policy is not FULL_ONLY")
	ErrRefundAlreadyRequested         = errors.New("operation center full refund was already requested with a different idempotency key")
	ErrRefundSchedulerDisabled        = errors.New("operation center refund scheduler is disabled")
	ErrRefundProviderUnavailable      = errors.New("operation center refund provider is unavailable")
	ErrRewardReleaseSchedulerDisabled = errors.New("operation center referral reward release scheduler is disabled")
	ErrManualRefundNotSubmitted       = errors.New("operation center manual refund is not submitted")
	ErrManualRefundConflict           = errors.New("operation center manual refund action conflicts with existing action")
)
