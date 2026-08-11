package payment

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestGrantTokenMissingPersonalPointHookFailsClosed(t *testing.T) {
	order := Order{
		OrderNo:            "payment_without_point_hook",
		UserID:             "user_without_point_hook",
		FulfillmentPayload: json.RawMessage(`{"tokenAmount":100}`),
	}

	err := grantTokenTx(context.Background(), nil, order, nil)
	if !errors.Is(err, ErrPersonalPointGrantHookUnavailable) {
		t.Fatalf("missing personal point hook error = %v", err)
	}
}
