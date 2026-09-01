package providerexecution

// ProviderCapability describes the safety properties of a provider.
type ProviderCapability struct {
	Provider              string
	Async                 bool
	QuerySupported        bool
	IdempotencySupported  bool
	RecoveryClass         RecoveryClass
}

// Canary provider capabilities for the generation async canary.
var CanaryProviderCapabilities = map[string]ProviderCapability{
	"grok-imagine-1.5": {
		Provider:             "grok-imagine-1.5",
		Async:                true,
		QuerySupported:       false,
		IdempotencySupported: false,
		RecoveryClass:        NonQueryableSync,
	},
	"seedance-fast-2.0": {
		Provider:             "seedance-fast-2.0",
		Async:                true,
		QuerySupported:       false,
		IdempotencySupported: false,
		RecoveryClass:        NonQueryableSync,
	},
}

// GetProviderCapability returns the capability for a provider name.
// Falls back to a conservative default if the provider is not in the matrix.
func GetProviderCapability(provider string) ProviderCapability {
	if cap, ok := CanaryProviderCapabilities[provider]; ok {
		return cap
	}
	return ProviderCapability{
		Provider:             provider,
		Async:                false,
		QuerySupported:       false,
		IdempotencySupported: false,
		RecoveryClass:        NonQueryableSync,
	}
}

// SafeToResubmitAfterProviderUnknown returns true only if the named provider
// supports native idempotency or is a queryable async provider.
func SafeToResubmitAfterProviderUnknown(provider string) bool {
	c := GetProviderCapability(provider)
	return c.IdempotencySupported || (c.QuerySupported && c.RecoveryClass == QueryableAsync)
}
