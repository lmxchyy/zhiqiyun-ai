package billing

import (
	"context"
	"encoding/json"
	"testing"
)

func TestApplyPaidPointFulfillmentRejectsMissingTransaction(t *testing.T) {
	err := ApplyPaidPointFulfillment(context.Background(), nil, PaidPointFulfillment{Payload: json.RawMessage(`{"tokenAmount":40000}`)}, nil)
	if err == nil {
		t.Fatal("missing transaction unexpectedly accepted")
	}
}
