package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	"xianzhi-ai/backend-go/internal/messaging"
	pe "xianzhi-ai/backend-go/internal/providerexecution"
)

const (
	generationPPTCanaryConsumer = "generation-ppt-canary-worker"
	// pptGenerationTimeout bounds a full deck run (outline + bounded slide pool).
	// Per-step contexts (plan 35s, image 115s) still apply inside each slide.
	pptGenerationTimeout = 30 * time.Minute
	// pptSlideImagePoolSize caps concurrent slide provider calls per message.
	// It mirrors the synchronous semaphore=3 so one 30-page deck can never
	// fan out to 30 simultaneous provider calls.
	pptSlideImagePoolSize = 3

	pptOutlineCapability    = "ppt_outline"
	pptVisualPlanCapability = "ppt_visual_plan"
)

// pptFingerprintExcludedParams lists params keys that never describe PPT
// provider-side semantics. It mirrors videoFingerprintExcludedParams so PPT
// executions get the same canonical-fingerprint stability across submit,
// restart, redelivery and retry.
func pptFingerprintExcludedParams() map[string]struct{} {
	return map[string]struct{}{
		"retryOf":                     {},
		"retryAttempt":                {},
		"terminal":                    {},
		providerExecutionTaskParam:    {},
		"_async_canary_provider":      {},
		"provider":                    {},
		"providerName":                {},
		"provider_channel":            {},
		"channel":                     {},
		"channel_id":                  {},
		"tenant_id":                   {},
		"organization_id":             {},
		"billing_scope":               {},
		"billing_account_id":          {},
		"billing_ledger_id":           {},
		"generation_async_canary":     {},
		"generation_ppt_async_canary": {},
		"module_code":                 {},
		"moduleCode":                  {},
	}
}

// canonicalPPTFingerprintParams projects params onto the stable,
// provider-semantic subset used for PPT outline/plan fingerprints.
func canonicalPPTFingerprintParams(params map[string]any) map[string]any {
	next := cloneAnyMap(params)
	if next == nil {
		next = map[string]any{}
	}
	for key := range pptFingerprintExcludedParams() {
		delete(next, key)
	}
	return next
}

// pptChatStageOutcome describes what a guarded chat-model stage (outline or
// visual plan) decided. Chat providers expose no async query API, so unlike
// image/video there is no Get-first path: a non-terminal same-fingerprint row
// is re-run under the SAME execution row (never a new row), bounded by the MQ
// retry budget. Only terminal succeeded rows skip the call.
type pptChatStageOutcome int

const (
	pptChatStageReused pptChatStageOutcome = iota
	pptChatStageExecuted
)

// runPPTChatStageGuarded executes a synchronous chat-model call under a
// durable pe row keyed by taskKey. Semantics:
//   - no row, or latest Failed with safe pre-submit class: create prepared→submitting
//     row, run call once.
//     Success → Transition(succeeded) + SaveSucceededResult(manifest).
//     Definitive failure → Transition(failed), return terminal error.
//     Ambiguous/timeout failure → MarkUnknown, return ErrUnknownResubmitBlocked.
//   - latest Succeeded + same fingerprint: skip the call entirely (zero POSTs);
//     return persisted manifest with outcome pptChatStageReused.
//   - latest Submitting / Unknown / Submitted / Processing: do NOT resubmit;
//     mark Unknown if Submitting, and return ErrUnknownResubmitBlocked (zero POSTs).
//   - fingerprint mismatch + non-Failed latest: hard error, never overwrite.
func (a api) runPPTChatStageGuarded(ctx context.Context, taskKey, provider, model, capability string, semanticParams map[string]any, call func(context.Context) ([]byte, error)) ([]byte, pptChatStageOutcome, error) {
	db := a.pgDB()
	store := pe.NewStore(db)
	fp, err := pe.Fingerprint(taskKey, provider, model, capability, canonicalPPTFingerprintParams(semanticParams))
	if err != nil {
		return nil, pptChatStageExecuted, fmt.Errorf("ppt chat fingerprint: %w", err)
	}
	latest, err := store.GetLatestByTask(ctx, taskKey)
	if err == nil {
		if latest.Status != pe.Failed && latest.RequestFingerprint != fp {
			return nil, pptChatStageExecuted, fmt.Errorf("ppt provider execution fingerprint mismatch for %s", taskKey)
		}
		if latest.Status == pe.Succeeded && latest.RequestFingerprint == fp {
			return latest.ResultMetadata, pptChatStageReused, nil
		}
		// In-flight or ambiguous: Submitting, Unknown, Submitted, Processing.
		// Chat providers have no async query API; never issue a blind second POST.
		// Mark unknown if not already transitioned, and block resubmission.
		if latest.Status == pe.Submitting || latest.Status == pe.Unknown || latest.Status == pe.Submitted || latest.Status == pe.Processing {
			if latest.Status == pe.Submitting {
				_ = store.MarkUnknown(ctx, latest.ID, pe.ProviderUnknown, "submission outcome unknown after crash before transition")
			}
			return nil, pptChatStageExecuted, pe.ErrUnknownResubmitBlocked
		}
		if latest.Status == pe.Failed {
			if latest.ErrorClass == nil || (*latest.ErrorClass != string(pe.DefinitiveNotSubmitted) && *latest.ErrorClass != string(pe.RetryableBeforeSubmit)) {
				return nil, pptChatStageExecuted, pe.ErrUnknownResubmitBlocked
			}
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, pptChatStageExecuted, err
	}
	attempt := 1
	if err == nil {
		attempt = latest.Attempt + 1
	}
	if _, err := store.CreatePrepared(ctx, pe.Execution{
		TaskID: taskKey, Provider: provider, ProviderModel: model,
		Capability: capability, Attempt: attempt, RequestFingerprint: fp,
	}); err != nil {
		return nil, pptChatStageExecuted, err
	}
	claimed, err := store.ClaimPrepared(ctx, taskKey)
	if err != nil {
		return nil, pptChatStageExecuted, err
	}
	return a.executePPTChatCall(ctx, store, claimed, call)
}

func (a api) executePPTChatCall(ctx context.Context, store *pe.Store, execRow pe.Execution, call func(context.Context) ([]byte, error)) ([]byte, pptChatStageOutcome, error) {
	manifest, err := call(ctx)
	if err != nil {
		if shouldFallbackPPTOutline(err) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			// Ambiguous / timeout error: submission may or may not have reached
			// provider. Mark Unknown and block blind resubmit.
			_ = store.MarkUnknown(ctx, execRow.ID, pe.ProviderUnknown, err.Error())
			return nil, pptChatStageExecuted, pe.ErrUnknownResubmitBlocked
		}
		if transitionErr := store.Transition(ctx, execRow.ID, pe.Failed, nil, pptStrPtr(string(pe.DefinitiveNotSubmitted)), pptStrPtr(err.Error())); transitionErr != nil {
			return nil, pptChatStageExecuted, transitionErr
		}
		return nil, pptChatStageExecuted, fmt.Errorf("ppt chat stage terminal failure: %w", err)
	}
	if transitionErr := store.Transition(ctx, execRow.ID, pe.Succeeded, nil, nil, nil); transitionErr != nil {
		return nil, pptChatStageExecuted, transitionErr
	}
	if saveErr := store.SaveSucceededResult(ctx, execRow.ID, nil, manifest); saveErr != nil {
		return nil, pptChatStageExecuted, saveErr
	}
	return manifest, pptChatStageExecuted, nil
}

// pptSlideExecKey returns the stable ProviderExecution key for a slide visual
// at the given revision, or "" when the slide/task identity is incomplete.
func pptSlideExecKey(pptTaskID, slideID string, revision int) string {
	return pptSlideExecutionKey(pptTaskID, slideID, revision)
}

// pptChildClientRequestID returns the deterministic child image task identity
// so crash redelivery replays (not duplicates) the per-slide generation task
// and its billing lifecycle.
func pptChildClientRequestID(pptTaskID, slideID string, revision int) string {
	return fmt.Sprintf("pptimg:%s:%s:rev:%d", strings.TrimSpace(pptTaskID), strings.TrimSpace(slideID), revision)
}

// isPPTCanaryTask reports whether a loaded parent task carries the PPT async
// canary markers written by the PR2 creation path.
func isPPTCanaryTask(params map[string]any) bool {
	if params == nil {
		return false
	}
	video, _ := params["generation_ppt_async_canary"].(bool)
	generic, _ := params["generation_async_canary"].(bool)
	return video && generic
}

// completePPTCanaryInboxIfTerminal mirrors completeCanaryInboxIfTerminal for
// the PPT consumer: when the parent task already left running status, complete
// the inbox row in the same TX and report terminal=true so the caller ACKs
// instead of retrying. It never touches the image consumer name.
func (a api) completePPTCanaryInboxIfTerminal(inbox *messaging.InboxStore, eventID, taskID string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := a.pgDB().BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	task, err := generationTaskForUpdate(ctx, tx, taskID)
	if err != nil {
		return false, err
	}
	if isRunningGenerationTaskStatus(task.Status) {
		return false, nil
	}
	if err := inbox.CompleteTx(ctx, tx, generationPPTCanaryConsumer, eventID, "completed", map[string]any{"task_id": taskID, "terminal": true}); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// pptOutlineRequestFromDetail rebuilds the outline model request from the
// durable ppt detail so the worker never depends on the original HTTP body.
func pptOutlineRequestFromDetail(detail pptapp.Task) pptOutlineGenerateRequest {
	return pptOutlineGenerateRequest{
		Prompt:      strings.TrimSpace(detail.Prompt),
		SlideCount:  detail.SlideCount,
		Language:    detail.Language,
		TextModel:   detail.TextModel,
		ImageSource: detail.ImageSource,
		ImageModel:  detail.ImageModel,
	}
}

// pptOutlineSemanticParams returns the fingerprinted semantic subset for the
// outline stage of a deck.
// pptStrPtr is a production helper (strPtr lives only in _test files and is
// invisible to go build).
func pptStrPtr(value string) *string { return &value }

func pptOutlineSemanticParams(req pptOutlineGenerateRequest) map[string]any {
	return map[string]any{
		"prompt":      strings.TrimSpace(req.Prompt),
		"slide_count": req.SlideCount,
		"language":    strings.TrimSpace(req.Language),
		"model":       strings.TrimSpace(req.TextModel),
	}
}
