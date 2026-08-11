package httpserver

import "testing"

func TestV132DoesNotWriteLegacyCommissionProjection(t *testing.T) {
	if shouldWriteLegacyCommissionProjection(adminOrder{PriceSnapshot: map[string]any{"settlementEngine": "V132"}}) {
		t.Fatal("V1.3.2 order must not write the Legacy commission wallet projection")
	}
	if !shouldWriteLegacyCommissionProjection(adminOrder{PriceSnapshot: map[string]any{"settlementEngine": "LEGACY"}}) {
		t.Fatal("Legacy order must preserve the existing commission wallet projection")
	}
	if !shouldWriteLegacyCommissionProjection(adminOrder{}) {
		t.Fatal("orders without a V1.3.2 decision must default to the Legacy projection")
	}
}
