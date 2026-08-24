// Package billing owns paid-product entitlement contracts independently of HTTP
// transport and payment-provider details.
package billing

import (
	"encoding/json"
	"errors"
)

var ErrInvalidPaidPointEntitlement = errors.New("paid point entitlement payload is invalid")

type PaidPointEntitlement struct {
	Points int64
}

// ParsePaidPointEntitlement validates the immutable paid-product snapshot used
// by payment fulfillment. The caller remains responsible for applying the
// entitlement inside its existing transaction.
func ParsePaidPointEntitlement(raw json.RawMessage) (PaidPointEntitlement, error) {
	var payload struct {
		TokenAmount int64 `json:"tokenAmount"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload.TokenAmount <= 0 {
		return PaidPointEntitlement{}, ErrInvalidPaidPointEntitlement
	}
	return PaidPointEntitlement{Points: payload.TokenAmount}, nil
}
