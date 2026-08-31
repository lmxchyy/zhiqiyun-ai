package providerexecution

// RecoveryClass describes whether an adapter can prove a request outcome.
type RecoveryClass string

const (
	QueryableAsync       RecoveryClass = "QUERYABLE_ASYNC"
	NonQueryableSync     RecoveryClass = "NON_QUERYABLE_SYNC"
	IdempotentSubmission RecoveryClass = "IDEMPOTENT_SUBMISSION"
)

type ProviderDescriptor struct {
	Provider                 string
	Capability               string
	Async                    bool
	CreatesProviderRequestID bool
	QuerySupported           bool
	CancelSupported          bool
	IdempotencyKeySupported  bool
	RecoveryClass            RecoveryClass
}

func (p ProviderDescriptor) SafeToResubmitAfterUnknown() bool {
	return p.IdempotencyKeySupported || (p.QuerySupported && p.RecoveryClass == QueryableAsync)
}
