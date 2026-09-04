package httpserver

import (
	"errors"
	"strings"
	"testing"

	pe "xianzhi-ai/backend-go/internal/providerexecution"
)

func TestRecoveryObservabilityStagesAndCodes(t *testing.T) {
	resetRecoveryCountersForTest()
	defer resetRecoveryCountersForTest()

	called := false
	diags := make([]RecoveryDiagnosis, 0)
	recoveryObserveFunc = func(d RecoveryDiagnosis) {
		called = true
		diags = append(diags, d)
	}
	defer func() { recoveryObserveFunc = defaultObserveRecovery }()

	wrapRecoveryError(RecoveryCodeIdentityError, RecoveryStageIdentity,
		RecoveryDiagnosis{Capability: "video", TaskID: "task_recovery_test"}, errors.New("identity error"))
	if !called {
		t.Fatal("observeRecovery was not called")
	}
	if len(diags) != 1 || diags[0].Stage != RecoveryStageIdentity || diags[0].Code != RecoveryCodeIdentityError {
		t.Fatalf("unexpected diagnosis: %+v", diags)
	}
}

func TestRecoveryDiagnosticsCaptureBooleanPresence(t *testing.T) {
	resetRecoveryCountersForTest()
	defer resetRecoveryCountersForTest()

	called := false
	recoveryObserveFunc = func(d RecoveryDiagnosis) { called = true }
	defer func() { recoveryObserveFunc = defaultObserveRecovery }()

	_ = wrapRecoveryError(RecoveryCodeFingerprintMismatch, RecoveryStageIdentity,
		RecoveryDiagnosis{Capability: "video", TaskID: "task_fp", ExecutionFound: true, HasRequestID: true, FingerprintMatch: false, LegacyEvaluated: true, LegacyAccepted: false, ProviderGet: false, ProviderCreate: false, Finalization: false}, errors.New("mismatch"))
	if !called {
		t.Fatal("observeRecovery was not called")
	}
}

func TestRecoveryCodesAreStableAndNonEmpty(t *testing.T) {
	codes := []string{
		RecoveryCodeRetryResumeActive, RecoveryCodeRetryRejectTerminal, RecoveryCodeRetryRejectUnknown,
		RecoveryCodeRetryNewChild, RecoveryCodeTerminalSkip, RecoveryCodeIdentityError,
		RecoveryCodeNoTaskSyncCreate, RecoveryCodeFingerprintMismatch, RecoveryCodeLegacyAccepted,
		RecoveryCodeLegacyRejected, RecoveryCodeGetFailed, RecoveryCodeGetProcessing,
		RecoveryCodeGetProviderFailed, RecoveryCodeGetSucceeded, RecoveryCodeCreateFailed,
		RecoveryCodeCreateSubmitted, RecoveryCodeUnknownBlocked, RecoveryCodeFinalizeSaved,
		RecoveryCodeFinalizeFailed, RecoveryCodeSettleCaptureRequested, RecoveryCodeSettleFailRequested,
		RecoveryCodeSettleFailed, RecoveryCodeResultReturned,
	}
	for _, code := range codes {
		if strings.TrimSpace(code) == "" {
			t.Fatalf("empty recovery code")
		}
		if !strings.HasPrefix(code, "retry_") && !strings.HasPrefix(code, "terminal_") &&
			!strings.HasPrefix(code, "identity_") && !strings.HasPrefix(code, "no_task_") &&
			!strings.HasPrefix(code, "fingerprint_") && !strings.HasPrefix(code, "legacy_") &&
			!strings.HasPrefix(code, "provider_") && !strings.HasPrefix(code, "unknown_") &&
			!strings.HasPrefix(code, "finalize_") && !strings.HasPrefix(code, "settle_") &&
			!strings.HasPrefix(code, "result_") {
			t.Fatalf("unexpected code prefix for %s", code)
		}
	}
}

func TestRecoveryErrorUnwrapAndCodeOf(t *testing.T) {
	base := errors.New("base failure")
	err := wrapRecoveryError(RecoveryCodeFingerprintMismatch, RecoveryStageIdentity,
		RecoveryDiagnosis{Capability: "video", TaskID: "task_unwrap"}, base)
	if RecoveryCodeOf(err) != RecoveryCodeFingerprintMismatch {
		t.Fatalf("expected code, got %q", RecoveryCodeOf(err))
	}
	if !errors.Is(err, base) {
		t.Fatal("expected errors.Is to unwrap to base")
	}
}

func TestRecoveryObservePanicDoesNotAffectBusiness(t *testing.T) {
	resetRecoveryCountersForTest()
	defer resetRecoveryCountersForTest()

	called := false
	recoveryObserveFunc = func(d RecoveryDiagnosis) {
		called = true
		panic("test panic")
	}
	defer func() { recoveryObserveFunc = defaultObserveRecovery }()

	// observeRecovery must not panic; the panic is recovered internally.
	base := errors.New("base")
	err := wrapRecoveryError(RecoveryCodeFingerprintMismatch, RecoveryStageIdentity,
		RecoveryDiagnosis{Capability: "video", TaskID: "task_panic"}, base)
	if err == nil {
		t.Fatal("expected error")
	}
	if RecoveryCodeOf(err) != RecoveryCodeFingerprintMismatch {
		t.Fatalf("expected code, got %q", RecoveryCodeOf(err))
	}
	if !errors.Is(err, base) {
		t.Fatal("expected errors.Is to unwrap to base")
	}
	if !called {
		t.Fatal("observeRecovery was called despite panic")
	}
}

type fakeTaskLookup struct {
	tasks map[string]map[string]any
	err   error
}

func (f *fakeTaskLookup) ListGenerationTasks() ([]generationTask, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([]generationTask, 0, len(f.tasks))
	for id, params := range f.tasks {
		t := generationTask{ID: id, Params: params}
		out = append(out, t)
	}
	return out, nil
}

func TestLegacyVideoExecutionReturnsReason(t *testing.T) {
	tasks := &fakeTaskLookup{tasks: map[string]map[string]any{
		"task_test": stableVideoSemanticParams(),
	}}
	tests := []struct {
		name     string
		lookup   videoTaskParamsLookup
		current  pe.Execution
		latest   pe.Execution
		wantOK   bool
		wantCode string
	}{
		{
			name:     "nil lookup rejects with no_lookup",
			lookup:   nil,
			current:  pe.Execution{Attempt: 1, RequestFingerprint: "fp"},
			latest:   pe.Execution{Status: pe.Submitted, ProviderRequestID: strPtr("req"), Attempt: 1},
			wantOK:   false,
			wantCode: legacyRejectNoLookup,
		},
		{
			name:     "empty request id rejects with no_request_id",
			lookup:   tasks,
			current:  pe.Execution{Attempt: 1, RequestFingerprint: "fp"},
			latest:   pe.Execution{Status: pe.Submitted, ProviderRequestID: strPtr(""), Attempt: 1},
			wantOK:   false,
			wantCode: legacyRejectNoRequestID,
		},
		{
			name:     "unexpected state rejects",
			lookup:   tasks,
			current:  func() pe.Execution { fp, _ := videoRequestFingerprint("task_test", "grok", "video", "model", stableVideoSemanticParams()); return pe.Execution{Attempt: 1, RequestFingerprint: fp, Provider: "grok", Capability: "video", ProviderModel: "model", ProviderChannel: "chan"} }(),
			latest:   pe.Execution{Status: pe.Prepared, ProviderRequestID: strPtr("req"), Attempt: 1, Provider: "grok", Capability: "video", ProviderModel: "model", ProviderChannel: "chan"},
			wantOK:   false,
			wantCode: legacyRejectUnexpectedState,
		},
		{
			name:     "attempt mismatch rejects",
			lookup:   tasks,
			current:  func() pe.Execution { fp, _ := videoRequestFingerprint("task_test", "grok", "video", "model", stableVideoSemanticParams()); return pe.Execution{Attempt: 2, RequestFingerprint: fp, Provider: "grok", Capability: "video", ProviderModel: "model", ProviderChannel: "chan"} }(),
			latest:   pe.Execution{Status: pe.Submitted, ProviderRequestID: strPtr("req"), Attempt: 1, Provider: "grok", ProviderModel: "model", ProviderChannel: "chan"},
			wantOK:   false,
			wantCode: legacyRejectAttemptMismatch,
		},
		{
			name:     "accepted when canonical match",
			lookup:   tasks,
			current:  func() pe.Execution { fp, _ := videoRequestFingerprint("task_test", "grok", "video", "model", stableVideoSemanticParams()); return pe.Execution{Attempt: 1, RequestFingerprint: fp, Provider: "grok", Capability: "video", ProviderModel: "model", ProviderChannel: "chan"} }(),
			latest:   pe.Execution{Status: pe.Submitted, ProviderRequestID: strPtr("req"), Attempt: 1, Provider: "grok", ProviderModel: "model", ProviderChannel: "chan"},
			wantOK:   true,
			wantCode: legacyAcceptReason,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ok, code := acceptLegacyVideoExecution(tc.lookup, "task_test", tc.current, tc.latest)
			if ok != tc.wantOK || code != tc.wantCode {
				t.Fatalf("got ok=%v code=%q want ok=%v code=%q", ok, code, tc.wantOK, tc.wantCode)
			}
		})
	}
}

func strPtr(s string) *string { return &s }
