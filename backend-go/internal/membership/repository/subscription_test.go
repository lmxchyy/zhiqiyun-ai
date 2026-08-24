package repository

import (
	"context"
	"testing"
)

func TestUpsertSubscriptionTxRejectsMissingTransaction(t *testing.T) {
	err := UpsertSubscriptionTx(context.Background(), nil, SubscriptionProjection{})
	if err != ErrInvalidSubscriptionProjection {
		t.Fatalf("error = %v", err)
	}
}
