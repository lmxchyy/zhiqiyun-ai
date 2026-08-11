package operationcenter

import "xianzhi-ai/backend-go/internal/app/payment"

type RefundVerificationResult struct {
	ServiceOrderID, RefundTaskID string
	RefundStatus                 OperationCenterRefundStatus
	QueryOutcome                 payment.QueryRefundOutcome
	ProviderRefundNo             string
	QueryCalled, QuerySkipped    bool
	IdempotentReplay, InProgress bool
}

func snapshotProviderSummary(summary payment.ProviderResponseSummary) JSONSnapshot {
	result := JSONSnapshot{}
	for key, value := range payment.SanitizeProviderResponseSummary(summary) {
		result[key] = value
	}
	return result
}
