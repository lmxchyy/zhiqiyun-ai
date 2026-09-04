package httpserver

import (
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
)

// Structured observability for generation retry/recovery paths.
//
// This file is observability-only: it adds stage markers, stable error codes,
// and counters. It must not change business semantics, points handling,
// provider-execution transitions, or retry behavior.
//
// Privacy: diagnosis records operational identifiers (task id) and boolean
// presence flags only. It never records prompts, API keys, tokens, URLs, or
// request/response bodies. Metric labels carry capability/stage/code only
// (no task, user, or model labels) to avoid cardinality and leakage.

// Recovery stages mark where in the retry/recovery pipeline a diagnosis was
// emitted.
const (
	RecoveryStageRetryDecision = "retry_decision"
	RecoveryStageTerminalCheck = "terminal_check"
	RecoveryStageIdentity      = "identity"
	RecoveryStageGet           = "provider_get"
	RecoveryStageCreate        = "provider_create"
	RecoveryStageFinalize      = "finalize"
	RecoveryStageSettle        = "settle"
)

// Stable recovery codes. Each code is emitted for exactly one decision
// outcome so operators and tests can distinguish paths without parsing
// natural-language messages.
const (
	RecoveryCodeRetryResumeActive      = "retry_resume_active"
	RecoveryCodeRetryRejectTerminal    = "retry_reject_terminal"
	RecoveryCodeRetryRejectUnknown     = "retry_reject_unknown"
	RecoveryCodeRetryNewChild          = "retry_new_child"
	RecoveryCodeTerminalSkip           = "terminal_skip"
	RecoveryCodeIdentityError          = "identity_error"
	RecoveryCodeNoTaskSyncCreate       = "no_task_sync_create"
	RecoveryCodeFingerprintMismatch    = "fingerprint_mismatch"
	RecoveryCodeLegacyAccepted         = "legacy_accepted"
	RecoveryCodeLegacyRejected         = "legacy_rejected"
	RecoveryCodeGetFailed              = "provider_get_failed"
	RecoveryCodeGetProcessing          = "provider_processing"
	RecoveryCodeGetProviderFailed      = "provider_failed"
	RecoveryCodeGetSucceeded           = "provider_succeeded"
	RecoveryCodeCreateFailed           = "provider_create_failed"
	RecoveryCodeCreateSubmitted        = "provider_submitted"
	RecoveryCodeUnknownBlocked         = "unknown_resubmit_blocked"
	RecoveryCodeFinalizeSaved          = "finalize_saved"
	RecoveryCodeTransitionFailed       = "transition_failed"
	RecoveryCodeFinalizeFailed         = "finalize_failed"
	RecoveryCodeSettleCaptureRequested = "settle_capture_requested"
	RecoveryCodeSettleFailRequested    = "settle_fail_requested"
	RecoveryCodeSettleFailed           = "settle_failed"
	RecoveryCodeResultReturned         = "result_returned"
)

// RecoveryPointsAction describes the settlement boundary a recovery step
// invoked. Values name the invoked function, not the ledger outcome.
const (
	RecoveryPointsNone            = "none"
	RecoveryPointsCaptureRequested = "capture_requested"
	RecoveryPointsFailRequested    = "fail_requested"
)

// RecoveryDiagnosis is the structured record of one retry/recovery decision.
type RecoveryDiagnosis struct {
	Capability       string
	TaskID           string
	Stage            string
	Code             string
	Detail           string
	ExecutionFound   bool
	HasRequestID     bool
	FingerprintMatch bool
	LegacyEvaluated  bool
	LegacyAccepted   bool
	ProviderGet      bool
	ProviderCreate   bool
	Finalization     bool
	PointsAction     string
}

// RecoveryError attaches a stable code and diagnosis to an error without
// changing its handling. Callers match on errors.Is/As exactly as before.
type RecoveryError struct {
	Code  string
	Stage string
	Diag  RecoveryDiagnosis
	Err   error
}

func (e *RecoveryError) Error() string {
	if e == nil || e.Err == nil {
		return e.Code
	}
	return e.Code + ": " + e.Err.Error()
}

func (e *RecoveryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// RecoveryCodeOf returns the stable recovery code carried by err, if any.
func RecoveryCodeOf(err error) string {
	var re *RecoveryError
	if errors.As(err, &re) && re != nil {
		return re.Code
	}
	return ""
}

func wrapRecoveryError(code, stage string, diag RecoveryDiagnosis, err error) error {
	diag.Stage = stage
	diag.Code = code
	observeRecovery(diag)
	return &RecoveryError{Code: code, Stage: stage, Diag: diag, Err: err}
}

// recoveryObserveFunc is the emission sink. Tests override it to capture
// diagnoses; production logs structured fields and bumps counters.
var recoveryObserveFunc = defaultObserveRecovery

func withRecoveryCode(diag RecoveryDiagnosis, stage, code string) RecoveryDiagnosis {
	diag.Stage = stage
	diag.Code = code
	return diag
}

func observeRecovery(diag RecoveryDiagnosis) {
	if recoveryObserveFunc == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			// Observability emission must never affect business paths.
			slog.Warn("recovery observation panicked", "code", diag.Code, "stage", diag.Stage, "recover", r)
		}
	}()
	recoveryObserveFunc(diag)
}

func defaultObserveRecovery(diag RecoveryDiagnosis) {
	recordRecoveryCounter(diag.Capability, diag.Stage, diag.Code)
	slog.Info("generation recovery diagnosis",
		"capability", diag.Capability,
		"task_id", diag.TaskID,
		"stage", diag.Stage,
		"code", diag.Code,
		"detail", diag.Detail,
		"execution_found", diag.ExecutionFound,
		"has_request_id", diag.HasRequestID,
		"fingerprint_match", diag.FingerprintMatch,
		"legacy_evaluated", diag.LegacyEvaluated,
		"legacy_accepted", diag.LegacyAccepted,
		"provider_get", diag.ProviderGet,
		"provider_create", diag.ProviderCreate,
		"finalization", diag.Finalization,
		"points_action", diag.PointsAction,
	)
}

type recoveryCounterKey struct {
	capability string
	stage      string
	code       string
}

var recoveryCounters = struct {
	sync.Mutex
	counts map[recoveryCounterKey]uint64
}{counts: map[recoveryCounterKey]uint64{}}

func recordRecoveryCounter(capability, stage, code string) {
	if strings.TrimSpace(code) == "" {
		return
	}
	recoveryCounters.Lock()
	recoveryCounters.counts[recoveryCounterKey{capability, stage, code}]++
	recoveryCounters.Unlock()
}

func snapshotRecoveryCounters() map[recoveryCounterKey]uint64 {
	recoveryCounters.Lock()
	defer recoveryCounters.Unlock()
	out := make(map[recoveryCounterKey]uint64, len(recoveryCounters.counts))
	for key, value := range recoveryCounters.counts {
		out[key] = value
	}
	return out
}

func resetRecoveryCountersForTest() {
	recoveryCounters.Lock()
	recoveryCounters.counts = map[recoveryCounterKey]uint64{}
	recoveryCounters.Unlock()
}

// renderRecoveryMetrics emits xianzhi_generation_recovery_total. Labels carry
// capability/stage/code only.
func renderRecoveryMetrics(rendered *strings.Builder) {
	counts := snapshotRecoveryCounters()
	keys := make([]recoveryCounterKey, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].capability != keys[j].capability {
			return keys[i].capability < keys[j].capability
		}
		if keys[i].stage != keys[j].stage {
			return keys[i].stage < keys[j].stage
		}
		return keys[i].code < keys[j].code
	})
	writeMetricFamily(rendered, "xianzhi_generation_recovery_total", "Generation retry/recovery decisions by non-sensitive stage and code.", "counter", func() {
		for _, key := range keys {
			fmt.Fprintf(rendered, "xianzhi_generation_recovery_total{capability=%q,stage=%q,code=%q} %d\n",
				key.capability, key.stage, key.code, counts[key])
		}
	})
}
