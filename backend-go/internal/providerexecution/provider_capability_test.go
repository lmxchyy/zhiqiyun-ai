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

func TestUnknownRecoveryPolicyBlocksAutomaticResubmit(t *testing.T) {
	if UnknownRecoveryPolicy != "BLOCK_AUTO_RESUBMIT" {
		t.Fatalf("unexpected unknown recovery policy: %s", UnknownRecoveryPolicy)
	}
	for _, provider := range []string{"grok-imagine-1.5", "seedance-fast-2.0", "nonexistent-provider"} {
		capability := GetProviderCapability(provider)
		if capability.QuerySupported || capability.IdempotencySupported {
			t.Fatalf("%s must not permit automatic replay after unknown: %+v", provider, capability)
		}
	}
}
