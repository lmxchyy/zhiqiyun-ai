package operationcenter

import (
	"errors"
	"testing"
)

func TestValidatePaymentAmountsRequiresFrozenPriceAgreement(t *testing.T) {
	valid := paidOrderSnapshot{
		OrderAmountCents: 500000, PayableAmountCents: 500000, PaymentAmountCents: 500000,
		PlanPriceCents: 500000, CommercialSnapshotPaidCents: 500000,
		PriceSnapshot: JSONSnapshot{"priceCents": float64(500000)},
	}
	if err := validatePaymentAmounts(valid); err != nil {
		t.Fatal(err)
	}
	checks := []func(*paidOrderSnapshot){
		func(item *paidOrderSnapshot) { item.PaymentAmountCents-- },
		func(item *paidOrderSnapshot) { item.PlanPriceCents-- },
		func(item *paidOrderSnapshot) { item.CommercialSnapshotPaidCents-- },
		func(item *paidOrderSnapshot) { item.PriceSnapshot["priceCents"] = float64(499999) },
	}
	for index, mutate := range checks {
		item := valid
		item.PriceSnapshot = JSONSnapshot{"priceCents": float64(500000)}
		mutate(&item)
		if err := validatePaymentAmounts(item); !errors.Is(err, ErrPaymentAmountMismatch) {
			t.Fatalf("case %d error=%v", index, err)
		}
	}
}

func TestValidatePaymentAmountsRejectsMissingPriceSnapshot(t *testing.T) {
	item := paidOrderSnapshot{
		OrderAmountCents: 500000, PayableAmountCents: 500000, PaymentAmountCents: 500000,
		PlanPriceCents: 500000, CommercialSnapshotPaidCents: 500000,
		PriceSnapshot: JSONSnapshot{},
	}
	if err := validatePaymentAmounts(item); !errors.Is(err, ErrFrozenSnapshotMissing) {
		t.Fatalf("error=%v", err)
	}
}

func TestStableWorkflowKeysDoNotContainTimeOrRetry(t *testing.T) {
	first := stableWorkflowID("refund", "service-1", string(ReviewRejected))
	second := stableWorkflowID("refund", "service-1", string(ReviewRejected))
	if first != second || first == "" {
		t.Fatalf("stable IDs mismatch: %q %q", first, second)
	}
	if first == stableWorkflowID("refund", "service-2", string(ReviewRejected)) {
		t.Fatal("different service orders must not share refund idempotency")
	}
}

func TestNoopActivationHooksDoNotCreateSideEffects(t *testing.T) {
	if err := (NoopReferralEligibilityTrigger{}).MarkEligible(nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := (NoopOperationCenterActivationHook{}).AfterActivated(nil, nil, nil); err != nil {
		t.Fatal(err)
	}
}
