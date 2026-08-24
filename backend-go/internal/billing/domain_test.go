package billing

import (
	"encoding/json"
	"testing"
)

func TestParsePaidPointEntitlementRequiresPositiveGrant(t *testing.T) {
	got, err := ParsePaidPointEntitlement(json.RawMessage(`{"tokenAmount":40000}`))
	if err != nil {
		t.Fatalf("valid paid entitlement rejected: %v", err)
	}
	if got.Points != 40000 {
		t.Fatalf("points = %d, want 40000", got.Points)
	}
}

func TestParsePaidPointEntitlementRejectsMissingOrNonPositiveGrant(t *testing.T) {
	for _, raw := range []string{`{}`, `{"tokenAmount":0}`, `{"tokenAmount":-1}`, `{invalid}`} {
		if _, err := ParsePaidPointEntitlement(json.RawMessage(raw)); err == nil {
			t.Fatalf("payload %s unexpectedly accepted", raw)
		}
	}
}
