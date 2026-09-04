package httpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	pe "xianzhi-ai/backend-go/internal/providerexecution"
)

// Stable video fingerprint tests.
//
// A/B/C/D prove the invariant: the same logical provider attempt hashes
// identically across submit, store enrichment, restart, redelivery and retry.
// E/F/G prove drift is still detected. H/I/J prove the Get-first recovery
// behavior with zero blind submits, including the narrow legacy-compat rule.

func stableVideoSemanticParams() map[string]any {
	return map[string]any{
		"prompt":       "a calm harbor at dawn, cinematic",
		"duration":     6,
		"resolution":   "480p",
		"aspect_ratio": "16:9",
		"first_frame":  "https://cdn.example.com/frame.png",
	}
}

// submitShapedParams mimics the live submit request: the store transaction
// mutates tenant/ledger keys in place (visible to the caller) before the
// clone-then-rebind enrichment below.
func submitShapedParams() map[string]any {
	params := stableVideoSemanticParams()
	params["tenant_id"] = "tenant_test"
	params["billing_ledger_id"] = "compute_ledger_test_001"
	return params
}

// storedVideoParams mimics the task store transaction enrichment: pricing and
// billing snapshots cloned invisibly into the stored snapshot
// (postgres_store.go createPendingGenerationTask).
func storedVideoParams(submit map[string]any) map[string]any {
	stored := cloneAnyMap(submit)
	stored["pricing_rule_id"] = "brv_test_rule"
	stored["pricing_rule_version"] = 2
	stored["pricing_billing_unit"] = "PER_SECOND"
	stored["pricing_quantity"] = 6
	stored["pricing_breakdown"] = map[string]any{"basePrice": 15, "quantity": 6}
	stored["pricing_normalized_parameters"] = map[string]any{"duration": 6, "resolution": "480p"}
	stored["billingReserved"] = true
	stored["billingReservedAt"] = "2026-09-04T05:25:49.27857295Z"
	stored["billingReservationPointCost"] = 90
	stored["billingReservationBalanceBefore"] = 756
	stored["billingReservationBalanceAfter"] = 666
	stored["billing_ledger_id"] = "compute_ledger_test_001"
	stored["tenant_id"] = "tenant_test"
	return stored
}

func fingerprintMust(t *testing.T, taskID, provider, model string, params map[string]any) string {
	t.Helper()
	fp, err := videoRequestFingerprint(taskID, provider, "video", model, params)
	if err != nil {
		t.Fatal(err)
	}
	return fp
}

// Test A: submit request fingerprint == stored snapshot fingerprint even
// though the store invisibly enriches pricing/billing keys.
func TestVideoFingerprint_SubmitEqualsStoredAfterPrepareMutation(t *testing.T) {
	submit := submitShapedParams()
	stored := storedVideoParams(submit)
	if a, b := fingerprintMust(t, "task_A", "configured", "grok-imagine-1.5-video", submit), fingerprintMust(t, "task_A", "configured", "grok-imagine-1.5-video", stored); a != b {
		t.Fatalf("STABLE_FINGERPRINT_A=FAIL: submit != stored")
	}
}

// Test B: stored == retry with retryOf injected.
func TestVideoFingerprint_RetryWithRetryOfMatches(t *testing.T) {
	stored := storedVideoParams(stableVideoSemanticParams())
	retry := cloneAnyMap(stored)
	retry["retryOf"] = "task_B"
	if a, b := fingerprintMust(t, "task_B", "configured", "grok-imagine-1.5-video", stored), fingerprintMust(t, "task_B", "configured", "grok-imagine-1.5-video", retry); a != b {
		t.Fatalf("STABLE_FINGERPRINT_B=FAIL: stored != retry")
	}
}

// jsonRoundTrip simulates process restart / Rabbit redelivery reconstruction
// from persisted JSON (numbers decode as float64).
func jsonRoundTrip(t *testing.T, params map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

// Test C/D: restart and redelivery reconstructions hash identically.
func TestVideoFingerprint_RestartAndRedeliveryMatch(t *testing.T) {
	submit := submitShapedParams()
	stored := storedVideoParams(submit)
	restarted := jsonRoundTrip(t, stored)
	redelivered := jsonRoundTrip(t, stored)
	base := fingerprintMust(t, "task_CD", "configured", "grok-imagine-1.5-video", submit)
	if got := fingerprintMust(t, "task_CD", "configured", "grok-imagine-1.5-video", restarted); got != base {
		t.Fatalf("STABLE_FINGERPRINT_C=FAIL: restart reconstruction differs")
	}
	if got := fingerprintMust(t, "task_CD", "configured", "grok-imagine-1.5-video", redelivered); got != base {
		t.Fatalf("STABLE_FINGERPRINT_D=FAIL: redelivery reconstruction differs")
	}
}

// Test E/F/G: provider-semantic drift must mismatch.
func TestVideoFingerprint_SemanticDriftMismatches(t *testing.T) {
	base := stableVideoSemanticParams()
	want := fingerprintMust(t, "task_EFG", "configured", "grok-imagine-1.5-video", base)

	drifted := cloneAnyMap(base)
	drifted["prompt"] = "a different scene entirely"
	if got := fingerprintMust(t, "task_EFG", "configured", "grok-imagine-1.5-video", drifted); got == want {
		t.Fatalf("STABLE_FINGERPRINT_E=FAIL: prompt change not detected")
	}
	if got := fingerprintMust(t, "task_EFG", "configured", "seedance-fast-2.0", base); got == want {
		t.Fatalf("STABLE_FINGERPRINT_F=FAIL: model change not detected")
	}
	drifted = cloneAnyMap(base)
	drifted["duration"] = 10
	if got := fingerprintMust(t, "task_EFG", "configured", "grok-imagine-1.5-video", drifted); got == want {
		t.Fatalf("STABLE_FINGERPRINT_G=FAIL: duration change not detected")
	}
}

// Unknown future keys participate (conservative direction): they must change
// the fingerprint rather than being silently ignored.
func TestVideoFingerprint_UnknownFutureKeyParticipates(t *testing.T) {
	base := stableVideoSemanticParams()
	want := fingerprintMust(t, "task_future", "configured", "grok-imagine-1.5-video", base)
	withFuture := cloneAnyMap(base)
	withFuture["future_provider_option"] = "x1"
	if got := fingerprintMust(t, "task_future", "configured", "grok-imagine-1.5-video", withFuture); got == want {
		t.Fatalf("unknown future key must participate in fingerprint")
	}
}

// Local/routing/billing-only differences must NOT change the fingerprint.
func TestVideoFingerprint_LocalOnlyDifferencesMatch(t *testing.T) {
	base := storedVideoParams(stableVideoSemanticParams())
	want := fingerprintMust(t, "task_local", "configured", "grok-imagine-1.5-video", base)
	variant := cloneAnyMap(base)
	variant["retryOf"] = "task_local"
	variant["provider"] = "configured"
	variant["provider_channel"] = "channel_test"
	variant["providerTask"] = map[string]any{"status": "PROCESSING"}
	variant["billingRefunded"] = false
	if got := fingerprintMust(t, "task_local", "configured", "grok-imagine-1.5-video", variant); got != want {
		t.Fatalf("local-only differences must not change fingerprint")
	}
}

// Desensitized task_000234-structure regression fixture: NO production data,
// NO production DB. Shape mirrors the observed three snapshots (submit without
// billing keys, stored with billing/pricing snapshots, retry with retryOf).
func TestVideoFingerprint_Task000234StructureSameLogicalRequest(t *testing.T) {
	const taskID = "task_fixture_000234"
	submit := map[string]any{
		"prompt":       "negative routing test user1",
		"duration":     6,
		"resolution":   "480p",
		"aspect_ratio": "16:9",
		"tenant_id":         "tenant_fixture",
		"model_name":        "grok-imagine-1.5-video",
		"billing_ledger_id": "compute_ledger_fixture_001",
	}
	stored := storedVideoParams(submit)
	stored["tenant_id"] = "tenant_fixture"
	stored["billing_ledger_id"] = "compute_ledger_fixture_001"
	stored["model_name"] = "grok-imagine-1.5-video"
	retry := cloneAnyMap(stored)
	retry["retryOf"] = taskID

	submitFP := fingerprintMust(t, taskID, "configured", "grok-imagine-1.5-video", submit)
	storedFP := fingerprintMust(t, taskID, "configured", "grok-imagine-1.5-video", stored)
	retryFP := fingerprintMust(t, taskID, "configured", "grok-imagine-1.5-video", retry)
	if submitFP != storedFP || storedFP != retryFP {
		t.Fatalf("SAME_LOGICAL_PROVIDER_REQUEST=NO: submit/stored/retry diverge")
	}
	t.Logf("SAME_LOGICAL_PROVIDER_REQUEST=YES fp=%s", submitFP)

	for name, mutate := range map[string]func(map[string]any){
		"prompt":   func(p map[string]any) { p["prompt"] = "changed" },
		"duration": func(p map[string]any) { p["duration"] = 10 },
	} {
		drifted := cloneAnyMap(stored)
		mutate(drifted)
		if got := fingerprintMust(t, taskID, "configured", "grok-imagine-1.5-video", drifted); got == submitFP {
			t.Fatalf("FINGERPRINT_MISMATCH=NO for drift %s, want YES", name)
		}
	}
	if got := fingerprintMust(t, taskID, "configured", "seedance-fast-2.0", stored); got == submitFP {
		t.Fatalf("FINGERPRINT_MISMATCH=NO for model drift, want YES")
	}
}

type staticVideoTaskParamsLookup map[string]map[string]any

func (m staticVideoTaskParamsLookup) ListGenerationTasks() ([]generationTask, error) {
	out := make([]generationTask, 0, len(m))
	for id, params := range m {
		out = append(out, generationTask{ID: id, Params: params})
	}
	return out, nil
}

func seedVideoExecutionWithFingerprint(t *testing.T, store *pe.Store, taskID, provider, model, fingerprint, requestID string, status pe.Status) int64 {
	t.Helper()
	ctx := context.Background()
	claimed, err := store.CreatePrepared(ctx, pe.Execution{
		TaskID:             taskID,
		Provider:           provider,
		ProviderModel:      model,
		Capability:         "video",
		RequestFingerprint: fingerprint,
	})
	if err != nil {
		t.Fatal(err)
	}
	if claimed, err = store.ClaimPrepared(ctx, taskID); err != nil {
		t.Fatal(err)
	}
	if err := store.Transition(ctx, claimed.ID, status, stringPtr(requestID), nil, nil); err != nil {
		t.Fatal(err)
	}
	return claimed.ID
}

func submitShapeVideoReq(taskID, model string, params map[string]any) generation.CreateRequest {
	full := cloneAnyMap(params)
	full[providerExecutionTaskParam] = taskID
	return generation.CreateRequest{UserID: "fp-user", Type: "TEXT_TO_VIDEO", Prompt: "fp prompt", Model: model, Params: full}
}

// Test H: submitted execution + same logical request -> Get (not Create).
func TestVideoFingerprint_SubmittedRecoveryGetsWithoutCreate(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()
	taskID := "fp-h-submitted-" + time.Now().UTC().Format("20060102150405.000000000")
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM provider_executions WHERE task_id=$1", taskID)
	}()
	params := stableVideoSemanticParams()
	fp := fingerprintMust(t, taskID, "configured", "mock-video", params)
	store := pe.NewStore(db)
	executionID := seedVideoExecutionWithFingerprint(t, store, taskID, "configured", "mock-video", fp, "prov-fp-h-1", pe.Submitted)
	provider := &mockVideoProvider{}
	lookup := staticVideoTaskParamsLookup{taskID: storedVideoParams(params)}
	res, err := guardedVideo(ctx, submitShapeVideoReq(taskID, "mock-video", params), provider, store, lookup)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("expected recovered result")
	}
	if provider.createCalls.Load() != 0 {
		t.Fatalf("PROVIDER_CREATE_DURING_RECOVERY=%d, want 0", provider.createCalls.Load())
	}
	if provider.getCalls.Load() != 1 {
		t.Fatalf("provider Get calls=%d, want 1", provider.getCalls.Load())
	}
	latest, err := store.GetLatestByTask(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.ID != executionID || latest.Status != pe.Succeeded {
		t.Fatalf("execution id=%d status=%s, want same row Succeeded", latest.ID, latest.Status)
	}
}

// Test I: legacy (pre-canonical) fingerprint + retry-shaped request ->
// accepted via the narrow legacy rule, Get-first, zero Creates.
func TestVideoFingerprint_LegacySubmittedRecoveryCompat(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()
	taskID := "fp-i-legacy-" + time.Now().UTC().Format("20060102150405.000000000")
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM provider_executions WHERE task_id=$1", taskID)
	}()
	stored := storedVideoParams(stableVideoSemanticParams())
	legacyFP, err := pe.Fingerprint(taskID, "configured", "mock-video", "video", stored)
	if err != nil {
		t.Fatal(err)
	}
	store := pe.NewStore(db)
	executionID := seedVideoExecutionWithFingerprint(t, store, taskID, "configured", "mock-video", legacyFP, "prov-fp-i-1", pe.Submitted)
	provider := &mockVideoProvider{}
	retryParams := cloneAnyMap(stored)
	retryParams["retryOf"] = taskID
	lookup := staticVideoTaskParamsLookup{taskID: stored}
	res, err := guardedVideo(ctx, submitShapeVideoReq(taskID, "mock-video", retryParams), provider, store, lookup)
	if err != nil {
		t.Fatalf("legacy compat recovery err=%v", err)
	}
	if res == nil {
		t.Fatal("expected recovered result via legacy compat")
	}
	if provider.createCalls.Load() != 0 {
		t.Fatalf("PROVIDER_CREATE_DURING_RECOVERY=%d, want 0", provider.createCalls.Load())
	}
	latest, err := store.GetLatestByTask(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.ID != executionID || latest.Status != pe.Succeeded || len(latest.ResultMetadata) == 0 {
		t.Fatalf("legacy execution id=%d status=%s metadata=%d, want same row Succeeded with manifest", latest.ID, latest.Status, len(latest.ResultMetadata))
	}
}

// Test J: legacy execution + semantically different request -> mismatch stands,
// no Get, no Create.
func TestVideoFingerprint_LegacyDriftStillMismatches(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()
	taskID := "fp-j-drift-" + time.Now().UTC().Format("20060102150405.000000000")
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM provider_executions WHERE task_id=$1", taskID)
	}()
	stored := storedVideoParams(stableVideoSemanticParams())
	legacyFP, err := pe.Fingerprint(taskID, "configured", "mock-video", "video", stored)
	if err != nil {
		t.Fatal(err)
	}
	store := pe.NewStore(db)
	seedVideoExecutionWithFingerprint(t, store, taskID, "configured", "mock-video", legacyFP, "prov-fp-j-1", pe.Submitted)
	provider := &mockVideoProvider{}
	drifted := cloneAnyMap(stored)
	drifted["prompt"] = "a totally different request"
	drifted["retryOf"] = taskID
	lookup := staticVideoTaskParamsLookup{taskID: stored}
	_, err = guardedVideo(ctx, submitShapeVideoReq(taskID, "mock-video", drifted), provider, store, lookup)
	if err == nil || !strings.Contains(err.Error(), "fingerprint mismatch") {
		t.Fatalf("legacy compat must NOT bypass real drift: err=%v", err)
	}
	if provider.createCalls.Load() != 0 || provider.getCalls.Load() != 0 {
		t.Fatalf("drift must not touch provider: creates=%d gets=%d", provider.createCalls.Load(), provider.getCalls.Load())
	}
	latest, err := store.GetLatestByTask(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Status != pe.Submitted {
		t.Fatalf("drift must leave execution Submitted, got %s", latest.Status)
	}
}

// Legacy rule requires the lookup: without it, mismatch stays strict.
func TestVideoFingerprint_LegacyWithoutLookupStaysStrict(t *testing.T) {
	db := openProviderExecutionHookTestDB(t)
	defer db.Close()
	ctx := context.Background()
	taskID := "fp-j2-nolookup-" + time.Now().UTC().Format("20060102150405.000000000")
	defer func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM provider_executions WHERE task_id=$1", taskID)
	}()
	stored := storedVideoParams(stableVideoSemanticParams())
	legacyFP, err := pe.Fingerprint(taskID, "configured", "mock-video", "video", stored)
	if err != nil {
		t.Fatal(err)
	}
	store := pe.NewStore(db)
	seedVideoExecutionWithFingerprint(t, store, taskID, "configured", "mock-video", legacyFP, "prov-fp-j2-1", pe.Submitted)
	provider := &mockVideoProvider{}
	retryParams := cloneAnyMap(stored)
	retryParams["retryOf"] = taskID
	_, err = guardedVideo(ctx, submitShapeVideoReq(taskID, "mock-video", retryParams), provider, store, nil)
	if err == nil || !strings.Contains(err.Error(), "fingerprint mismatch") {
		t.Fatalf("without lookup legacy must stay strict: err=%v", err)
	}
}
