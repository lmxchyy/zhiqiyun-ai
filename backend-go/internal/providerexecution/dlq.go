package providerexecution

// DLQAction is the only state mutation a generation DLQ consumer is allowed
// to request. It deliberately has no release/resubmit action.
type DLQAction string

const (
	DLQFail         DLQAction = "FAILED"
	DLQExpire       DLQAction = "EXPIRED"
	DLQManualReview DLQAction = "MANUAL_REVIEW"
)

// DecideDLQAction fails closed when a provider request may still be active or
// its outcome is unknown. Such executions must never be released or submitted
// again merely because their broker message reached a DLQ.
func DecideDLQAction(executionStatus Status, expired bool) DLQAction {
	switch executionStatus {
	case Submitted, Processing, Submitting, Unknown:
		return DLQManualReview
	case Succeeded:
		// The provider result exists but local completion did not finish. Keep it
		// visible for reconciliation rather than issuing a second settlement.
		return DLQManualReview
	case Failed:
		if expired {
			return DLQExpire
		}
		return DLQFail
	default:
		if expired {
			return DLQExpire
		}
		return DLQManualReview
	}
}
