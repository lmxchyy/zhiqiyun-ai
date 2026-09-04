package httpserver

import (
	"strings"

	pe "xianzhi-ai/backend-go/internal/providerexecution"
)

// Stable video provider-request fingerprint.
//
// ROOT CAUSE (task_000234 class): request_fingerprint used to hash the entire
// mutable task params JSON. Three proven mutation points make the same logical
// provider attempt hash differently at submit / stored / retry time:
//
//	M-a  postgres_store.go createPendingGenerationTask: addGenerationPricingSnapshot
//	     and generationBillingReservationParams CLONE params before rebinding
//	     (store.go cloneAnyMap). The pricing/billing snapshot keys exist only in
//	     the stored snapshot, never in the live submit request.
//	M-b  api.go runVideoGenerationTask: appends provider/provider_channel to the
//	     prepared params after Prepare, so completed tasks store keys the submit
//	     fingerprint never saw.
//	M-c  asset_center_api.go retryGenerationTask: injects retryOf into the retry
//	     request, which the old narrow delete-list did not strip.
//
// INVARIANT: the fingerprint represents STABLE PROVIDER-SIDE REQUEST SEMANTICS.
// The same logical attempt (first submit, process restart, Rabbit redelivery,
// manual retry/recovery) must hash identically; any change to provider-visible
// inputs must hash differently.
//
// SAFETY DIRECTION: unknown future params keys PARTICIPATE (conservative). Only
// keys proven provider-invisible are excluded. A future local key therefore
// fails closed with a mismatch (safe); a future provider-semantic key
// participates, so drift is still detected (safe).
//
// SCOPE: video only. guardedImage keeps the historical behavior byte-identical
// so image executions (including task_000232) are unaffected.

// videoFingerprintExcludedParams lists params keys that never describe
// provider-side video request semantics and must not participate in the
// video request fingerprint.
func videoFingerprintExcludedParams() map[string]struct{} {
	excluded := map[string]struct{}{
		// Local recovery/bookkeeping markers, never sent to the provider.
		"retryOf":                  {},
		"retryAttempt":             {},
		"terminal":                 {},
		providerExecutionTaskParam: {},
		"_async_canary_provider":   {},
		// Routing echoes written after Prepare. Provider identity is carried
		// by the fingerprint scalar (providerName), not by these copies.
		"provider":         {},
		"providerName":     {},
		"provider_channel": {},
		"channel":          {},
		"channel_id":       {},
		// Provider result manifest written after Prepare.
		"providerTask": {},
		// Billing/pricing/ledger snapshots written invisibly by the task
		// store transaction (clone-then-rebind). The provider never sees
		// them, so they cannot distinguish provider operations.
		generationBillingReservedKey:                 {},
		generationBillingReservedAtKey:               {},
		generationBillingReservationPointCostKey:     {},
		generationBillingReservationBalanceBeforeKey: {},
		generationBillingReservationBalanceAfterKey:  {},
		generationBillingRefundedKey:                 {},
		generationBillingRefundedAtKey:               {},
		generationBillingRefundBalanceBeforeKey:      {},
		generationBillingRefundBalanceAfterKey:       {},
		// Pricing snapshots written invisibly by addGenerationPricingSnapshot
		// (ai_capability.go, called from the store transaction with
		// clone-then-rebind, so the live submit request never sees them).
		"pricing_rule_id":               {},
		"pricing_rule_version":          {},
		"pricing_billing_unit":          {},
		"pricing_quantity":              {},
		"pricing_breakdown":             {},
		"pricing_normalized_parameters": {},
	}
	return excluded
}

// canonicalVideoFingerprintParams projects params onto the stable,
// provider-semantic subset used for video request fingerprints.
func canonicalVideoFingerprintParams(params map[string]any) map[string]any {
	next := cloneAnyMap(params)
	if next == nil {
		next = map[string]any{}
	}
	for key := range videoFingerprintExcludedParams() {
		delete(next, key)
	}
	return next
}

// videoRequestFingerprint hashes the canonical (stable, provider-semantic)
// video request identity. Same logical attempt across submit, restart,
// redelivery and retry yields the same fingerprint.
func videoRequestFingerprint(taskID, provider, capability, model string, params map[string]any) (string, error) {
	return pe.Fingerprint(taskID, provider, model, capability, canonicalVideoFingerprintParams(params))
}

// videoTaskParamsLookup is the narrow read surface the legacy-compat check
// needs. platformStore already satisfies it, so no interface change is
// required.
type videoTaskParamsLookup interface {
	ListGenerationTasks() ([]generationTask, error)
}

// acceptLegacyVideoExecution decides whether a fingerprint mismatch may be
// forgiven for a pre-canonical execution WITHOUT weakening drift detection.
//
// Legacy executions (created before canonical fingerprints) carry fingerprints
// computed over params snapshots that no longer exist (e.g. the live
// pre-reservation request), so they can never be reproduced. Blanket acceptance
// would let a different provider request reuse the execution. Acceptance is
// therefore allowed ONLY when ALL hold:
//
//  1. A durable provider_request_id exists (proof of submission).
//  2. The execution is in a non-terminal-ambiguous state
//     (Submitted, Submitting, Processing, Unknown, or Succeeded).
//     Failed executions keep the existing strict rules untouched.
//  3. The attempt number matches (no attempt escalation on the legacy path).
//  4. The current request is canonically identical to the task's OWN stored
//     params. Any semantic drift (prompt, model-visible options, routing)
//     still mismatches and is rejected.
//
// When accepted, the caller proceeds to the normal Get-first recovery: the
// provider is queried by the durable request id, and only a proven provider
// success may finalize. Nothing is ever resubmitted on this path.
func acceptLegacyVideoExecution(lookup videoTaskParamsLookup, taskID string, current pe.Execution, latest pe.Execution) bool {
	if lookup == nil {
		return false
	}
	if latest.ProviderRequestID == nil || strings.TrimSpace(*latest.ProviderRequestID) == "" {
		return false
	}
	switch latest.Status {
	case pe.Submitted, pe.Submitting, pe.Processing, pe.Unknown, pe.Succeeded:
	default:
		return false
	}
	if latest.Attempt != current.Attempt {
		return false
	}
	tasks, err := lookup.ListGenerationTasks()
	if err != nil {
		return false
	}
	for _, task := range tasks {
		if task.ID != taskID {
			continue
		}
		expected, err := videoRequestFingerprint(taskID, current.Provider, current.Capability, current.ProviderModel, task.Params)
		if err != nil {
			return false
		}
		return expected == current.RequestFingerprint
	}
	return false
}
