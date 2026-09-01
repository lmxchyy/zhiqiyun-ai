
package providerexecution

import "testing"

func TestCanaryProviderCapabilities(t *testing.T) {
	grok := GetProviderCapability("grok-imagine-1.5")
	if !grok.Async || grok.QuerySupported || grok.IdempotencySupported || grok.RecoveryClass != NonQueryableSync {
		t.Fatalf("grok-imagine-1.5 capability unexpected: %+v", grok)
	}
	seed := GetProviderCapability("seedance-fast-2.0")
	if !seed.Async || seed.QuerySupported || seed.IdempotencySupported || seed.RecoveryClass != NonQueryableSync {
		t.Fatalf("seedance-fast-2.0 capability unexpected: %+v", seed)
	}
	unknown := GetProviderCapability("nonexistent-provider")
	if unknown.Async || unknown.QuerySupported || unknown.IdempotencySupported || unknown.RecoveryClass != NonQueryableSync {
		t.Fatalf("unknown provider should default to conservative: %+v", unknown)
	}
}

func TestSafeToResubmitAfterProviderUnknown(t *testing.T) {
	if SafeToResubmitAfterProviderUnknown("grok-imagine-1.5") {
		t.Fatal("non-queryable sync provider must not safely resubmit after unknown")
	}
	if SafeToResubmitAfterProviderUnknown("seedance-fast-2.0") {
		t.Fatal("non-queryable sync provider must not safely resubmit after unknown")
	}
}
