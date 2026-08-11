package operationcenter

import (
	"errors"
	"testing"
)

func TestOperationCenterServiceTransitionMatrix(t *testing.T) {
	valid := map[[2]OperationCenterServiceStatus]bool{
		{OperationCenterServicePendingPayment, OperationCenterServiceReviewRequired}: true,
		{OperationCenterServiceReviewRequired, OperationCenterServiceActive}:         true,
		{OperationCenterServiceReviewRequired, OperationCenterServiceRejected}:       true,
		{OperationCenterServiceActive, OperationCenterServiceRevoking}:               true,
		{OperationCenterServiceRevoking, OperationCenterServiceRevoked}:              true,
	}
	statuses := []OperationCenterServiceStatus{
		OperationCenterServicePendingPayment,
		OperationCenterServiceReviewRequired,
		OperationCenterServiceActive,
		OperationCenterServiceRevoking,
		OperationCenterServiceRevoked,
		OperationCenterServiceRejected,
		OperationCenterServiceLegacyRefundReversing,
		OperationCenterServiceLegacyRefunding,
		OperationCenterServiceLegacyRefunded,
	}
	for _, from := range statuses {
		for _, to := range statuses {
			err := ValidateOperationCenterServiceTransition(from, to)
			if valid[[2]OperationCenterServiceStatus{from, to}] {
				if err != nil {
					t.Fatalf("expected legal service transition %s -> %s: %v", from, to, err)
				}
				continue
			}
			if !errors.Is(err, ErrInvalidServiceTransition) {
				t.Fatalf("expected domain error for service transition %s -> %s, got %v", from, to, err)
			}
		}
	}
}

func TestOperationCenterRefundTransitionMatrix(t *testing.T) {
	valid := map[[2]OperationCenterRefundStatus]bool{
		{OperationCenterRefundNone, OperationCenterRefundPending}:                     true,
		{OperationCenterRefundPending, OperationCenterRefundReversing}:                true,
		{OperationCenterRefundPending, OperationCenterRefundCancelled}:                true,
		{OperationCenterRefundReversing, OperationCenterRefundProviderPending}:        true,
		{OperationCenterRefundProviderPending, OperationCenterRefundSucceeded}:        true,
		{OperationCenterRefundProviderPending, OperationCenterRefundRetryable}:        true,
		{OperationCenterRefundProviderPending, OperationCenterRefundUnknownVerifying}: true,
		{OperationCenterRefundProviderPending, OperationCenterRefundManualRequired}:   true,
		{OperationCenterRefundRetryable, OperationCenterRefundProviderPending}:        true,
		{OperationCenterRefundRetryable, OperationCenterRefundManualRequired}:         true,
		{OperationCenterRefundUnknownVerifying, OperationCenterRefundProviderPending}: true,
		{OperationCenterRefundUnknownVerifying, OperationCenterRefundRetryable}:       true,
		{OperationCenterRefundUnknownVerifying, OperationCenterRefundManualRequired}:  true,
		{OperationCenterRefundUnknownVerifying, OperationCenterRefundSucceeded}:       true,
		{OperationCenterRefundManualRequired, OperationCenterRefundManualSubmitted}:   true,
		{OperationCenterRefundManualSubmitted, OperationCenterRefundSucceeded}:        true,
		{OperationCenterRefundManualSubmitted, OperationCenterRefundManualRequired}:   true,
	}
	statuses := DatabaseOperationCenterRefundStatuses()
	for _, from := range statuses {
		for _, to := range statuses {
			err := ValidateOperationCenterRefundTransition(from, to)
			if valid[[2]OperationCenterRefundStatus{from, to}] {
				if err != nil {
					t.Fatalf("expected legal refund transition %s -> %s: %v", from, to, err)
				}
				continue
			}
			if !errors.Is(err, ErrInvalidRefundTransition) {
				t.Fatalf("expected domain error for refund transition %s -> %s, got %v", from, to, err)
			}
		}
	}
}

func TestRefundProviderResultMapsToPersistedStatus(t *testing.T) {
	tests := []struct {
		result RefundProviderResult
		want   OperationCenterRefundStatus
	}{
		{RefundProviderSuccess, OperationCenterRefundSucceeded},
		{RefundProviderTemporaryFailure, OperationCenterRefundRetryable},
		{RefundProviderUnsupported, OperationCenterRefundManualRequired},
		{RefundProviderUnknown, OperationCenterRefundUnknownVerifying},
	}
	for _, test := range tests {
		got, err := OperationCenterRefundStatusForProviderResult(test.result)
		if err != nil {
			t.Fatalf("map provider result %s: %v", test.result, err)
		}
		if got != test.want {
			t.Fatalf("provider result %s mapped to %s, want %s", test.result, got, test.want)
		}
	}
}

func TestUnknownProviderResultCannotBecomeSuccessOrRetryable(t *testing.T) {
	got, err := OperationCenterRefundStatusForProviderResult(RefundProviderUnknown)
	if err != nil {
		t.Fatal(err)
	}
	if got == OperationCenterRefundSucceeded || got == OperationCenterRefundRetryable {
		t.Fatalf("UNKNOWN must require verification, got %s", got)
	}
	if got != OperationCenterRefundUnknownVerifying {
		t.Fatalf("UNKNOWN mapped to %s", got)
	}
}
