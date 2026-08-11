package operationcenter

import (
	"errors"
	"testing"

	"xianzhi-ai/backend-go/internal/app/payment"
)

func TestRefundProviderClassificationAndStableRefundNumber(t *testing.T) {
	task := OperationCenterRefundTask{ID: "refund-task", IdempotencyKey: "stable-business-refund"}
	first := stableProviderRefundNo(task)
	second := stableProviderRefundNo(task)
	if first == "" || first != second {
		t.Fatalf("unstable refund number %q/%q", first, second)
	}
	if got := classifyRefundProviderCall(payment.RefundPaymentResult{ProviderRefundID: "provider-refund", Status: payment.PaymentRefunded}, nil); got != RefundProviderSuccess {
		t.Fatalf("success classification=%s", got)
	}
	for _, outcome := range []RefundProviderResult{RefundProviderTemporaryFailure, RefundProviderUnsupported, RefundProviderUnknown} {
		err := NewRefundProviderCallError(outcome, errors.New("provider error"))
		if got := classifyRefundProviderCall(payment.RefundPaymentResult{}, err); got != outcome {
			t.Fatalf("classification got=%s want=%s", got, outcome)
		}
	}
	if got := classifyRefundProviderCall(payment.RefundPaymentResult{}, errors.New("unclassified")); got != RefundProviderUnknown {
		t.Fatalf("unclassified error must be UNKNOWN, got=%s", got)
	}
}
