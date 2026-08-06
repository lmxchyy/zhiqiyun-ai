package httpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	"xianzhi-ai/backend-go/internal/app/ppt/skills"
)

const (
	pptAgentGenerationRunTimeout     = 2 * time.Minute
	pptAgentGenerationCancelWait     = 5 * time.Second
	pptAgentGenerationCleanupTimeout = 10 * time.Second
)

type pptAgentConfirmOutlineRequest struct{}
type pptAgentCancelRequest struct{}

type pptAgentGenerationState interface {
	GetTask(context.Context, pptapp.OwnerScope, string) (pptapp.Task, error)
	BeginGenerationClaim(context.Context, pptapp.OwnerScope, string, string, string) (pptapp.OperationClaim, pptapp.Task, error)
	FailGenerationClaim(context.Context, pptapp.OwnerScope, string, pptapp.OperationClaim, string) (pptapp.Task, error)
	BindGenerationBilling(context.Context, pptapp.OwnerScope, string, pptapp.OperationClaim, string) (pptapp.Task, error)
	ClaimGenerationRun(context.Context, pptapp.OwnerScope, string, time.Time) (pptapp.GenerationClaim, pptapp.Task, error)
	RenewGenerationRun(context.Context, pptapp.OwnerScope, string, pptapp.GenerationClaim, time.Time, time.Duration) (pptapp.GenerationClaim, pptapp.Task, error)
	AcquireGenerationCleanupFence(context.Context, pptapp.OwnerScope, string, pptapp.GenerationClaim, time.Time) (pptapp.GenerationClaim, pptapp.Task, error)
	PersistGeneratedSlide(context.Context, pptapp.OwnerScope, string, pptapp.GenerationClaim, pptapp.Slide) (pptapp.Task, error)
	CompleteGenerationAfterCapture(context.Context, pptapp.OwnerScope, string, pptapp.GenerationClaim) (pptapp.Task, error)
	BeginCancel(context.Context, pptapp.OwnerScope, string, string, string) (pptapp.CancelClaim, pptapp.Task, error)
	BeginCancelAfterStaleGenerationClaim(context.Context, pptapp.OwnerScope, string, pptapp.OperationClaim, string, string, time.Time) (pptapp.CancelClaim, pptapp.Task, error)
	CompleteCancel(context.Context, pptapp.OwnerScope, string, pptapp.CancelClaim) (pptapp.Task, error)
	FailGenerationAfterRelease(context.Context, pptapp.OwnerScope, string, pptapp.GenerationClaim, string) (pptapp.Task, error)
}

type pptAgentBillingStore interface {
	CreatePendingGenerationTask(createGenerationTaskRequest) (generationTask, error)
	CompleteGenerationTask(string, createGenerationTaskRequest) (generationTask, error)
	FailGenerationTask(string, string) (generationTask, error)
	ListGenerationTasks() ([]generationTask, error)
}

type pptAgentImageRunner func(context.Context, pptapp.Task) error

type pptAgentGenerationStateContextKey struct{}
type pptAgentBillingContextKey struct{}
type pptAgentImageRunnerContextKey struct{}

type pptAgentGenerationServiceAdapter struct {
	service *pptapp.Service
}

func (a pptAgentGenerationServiceAdapter) GetTask(_ context.Context, owner pptapp.OwnerScope, taskID string) (pptapp.Task, error) {
	return a.service.GetTask(owner, taskID)
}

func (a pptAgentGenerationServiceAdapter) BeginGenerationClaim(ctx context.Context, owner pptapp.OwnerScope, taskID, key, requestHash string) (pptapp.OperationClaim, pptapp.Task, error) {
	return a.service.BeginGenerationClaim(ctx, owner, taskID, key, requestHash)
}

func (a pptAgentGenerationServiceAdapter) FailGenerationClaim(ctx context.Context, owner pptapp.OwnerScope, taskID string, claim pptapp.OperationClaim, errorCode string) (pptapp.Task, error) {
	return a.service.FailGenerationClaim(ctx, owner, taskID, claim, errorCode)
}

func (a pptAgentGenerationServiceAdapter) BindGenerationBilling(ctx context.Context, owner pptapp.OwnerScope, taskID string, claim pptapp.OperationClaim, billingTaskID string) (pptapp.Task, error) {
	return a.service.BindGenerationBilling(ctx, owner, taskID, claim, billingTaskID)
}

func (a pptAgentGenerationServiceAdapter) ClaimGenerationRun(ctx context.Context, owner pptapp.OwnerScope, taskID string, now time.Time) (pptapp.GenerationClaim, pptapp.Task, error) {
	return a.service.ClaimGenerationRun(ctx, owner, taskID, now)
}

func (a pptAgentGenerationServiceAdapter) RenewGenerationRun(ctx context.Context, owner pptapp.OwnerScope, taskID string, claim pptapp.GenerationClaim, now time.Time, leaseDuration time.Duration) (pptapp.GenerationClaim, pptapp.Task, error) {
	return a.service.RenewGenerationRun(ctx, owner, taskID, claim, now, leaseDuration)
}

func (a pptAgentGenerationServiceAdapter) AcquireGenerationCleanupFence(ctx context.Context, owner pptapp.OwnerScope, taskID string, claim pptapp.GenerationClaim, now time.Time) (pptapp.GenerationClaim, pptapp.Task, error) {
	return a.service.AcquireGenerationCleanupFence(ctx, owner, taskID, claim, now)
}

func (a pptAgentGenerationServiceAdapter) PersistGeneratedSlide(ctx context.Context, owner pptapp.OwnerScope, taskID string, claim pptapp.GenerationClaim, slide pptapp.Slide) (pptapp.Task, error) {
	return a.service.PersistGeneratedSlide(ctx, owner, taskID, claim, slide)
}

func (a pptAgentGenerationServiceAdapter) CompleteGenerationAfterCapture(ctx context.Context, owner pptapp.OwnerScope, taskID string, claim pptapp.GenerationClaim) (pptapp.Task, error) {
	return a.service.CompleteGenerationAfterCapture(ctx, owner, taskID, claim)
}

func (a pptAgentGenerationServiceAdapter) BeginCancel(ctx context.Context, owner pptapp.OwnerScope, taskID, key, requestHash string) (pptapp.CancelClaim, pptapp.Task, error) {
	return a.service.BeginCancel(ctx, owner, taskID, key, requestHash)
}

func (a pptAgentGenerationServiceAdapter) BeginCancelAfterStaleGenerationClaim(ctx context.Context, owner pptapp.OwnerScope, taskID string, generationClaim pptapp.OperationClaim, key, requestHash string, now time.Time) (pptapp.CancelClaim, pptapp.Task, error) {
	return a.service.BeginCancelAfterStaleGenerationClaim(ctx, owner, taskID, generationClaim, key, requestHash, now)
}

func (a pptAgentGenerationServiceAdapter) CompleteCancel(ctx context.Context, owner pptapp.OwnerScope, taskID string, claim pptapp.CancelClaim) (pptapp.Task, error) {
	return a.service.CompleteCancel(ctx, owner, taskID, claim)
}

func (a pptAgentGenerationServiceAdapter) FailGenerationAfterRelease(ctx context.Context, owner pptapp.OwnerScope, taskID string, claim pptapp.GenerationClaim, errorCode string) (pptapp.Task, error) {
	return a.service.FailGenerationAfterRelease(ctx, owner, taskID, claim, errorCode)
}

type pptAgentGenerationRun struct {
	state          pptAgentGenerationState
	billing        pptAgentBillingStore
	imageRunner    pptAgentImageRunner
	owner          pptapp.OwnerScope
	taskID         string
	claim          pptapp.GenerationClaim
	billingTask    generationTask
	billingRequest createGenerationTaskRequest
}

type pptAgentPageExecutionError struct {
	code string
	err  error
}

func (e pptAgentPageExecutionError) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	return e.code
}

func (e pptAgentPageExecutionError) Unwrap() error { return e.err }

type pptAgentGenerationControl struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func (a api) confirmPPTSessionOutline(w http.ResponseWriter, r *http.Request) {
	data, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	_, err = pptAgentIdempotencyKey(r)
	if err != nil {
		writePPTAgentError(w, err)
		return
	}
	var body *pptAgentConfirmOutlineRequest
	if err := decodePPTAgentJSON(r, &body); err != nil {
		writePPTAgentError(w, newPPTAgentError(http.StatusBadRequest, "PPT_REQUEST_INVALID", "请求格式无效"))
		return
	}
	if body == nil {
		writePPTAgentError(w, newPPTAgentError(http.StatusBadRequest, "PPT_REQUEST_INVALID", "请求格式无效"))
		return
	}
	taskID := strings.TrimSpace(r.PathValue("taskId"))
	state := pptAgentGenerationStateForRequest(a, r)
	owner, err := pptOwnerForCapability(a.store, user)
	if err != nil {
		writePPTAgentError(w, newPPTAgentError(http.StatusServiceUnavailable, "PPT_TENANT_CONTEXT_UNAVAILABLE", "PPT 会话租户上下文暂时不可用"))
		return
	}
	task, err := state.GetTask(r.Context(), owner, taskID)
	if err != nil {
		writePPTAgentStateError(w, err)
		return
	}
	if task.Stage == pptapp.StageFailed || task.Stage == pptapp.StageDraft {
		writePPTAgentStateError(w, pptapp.ErrInvalidStage)
		return
	}
	if task.Stage == pptapp.StageCancelled {
		writePPTAgentStateError(w, pptapp.ErrSessionCancelled)
		return
	}
	skill, ok := skills.Resolve(strings.TrimSpace(task.SkillCode))
	if !ok {
		writePPTAgentError(w, newPPTAgentError(http.StatusNotFound, "PPT_SKILL_NOT_FOUND", "未找到指定的 PPT Skill"))
		return
	}
	pageCount, err := validatePPTAgentConfirmedOutline(task, skill)
	if err != nil {
		writePPTAgentError(w, err)
		return
	}
	capability, err := a.preparePPTCapabilityRequest(data, user, task.Prompt, task.TextModel, pageCount, pptImagesEnabled(task.ImageSource), len(task.SourceFileIDs) > 0)
	if err != nil {
		writePPTAgentError(w, newPPTAgentError(http.StatusBadRequest, "PPT_CAPABILITY_DENIED", "当前账户无法使用该 PPT 能力"))
		return
	}
	if capabilityPages := int(anyFloatOrDefault(capability.Params["page_count"], 0)); capabilityPages != pageCount {
		writePPTAgentError(w, newPPTAgentError(http.StatusBadRequest, "PPT_CAPABILITY_DENIED", "当前账户无法生成该页数的 PPT"))
		return
	}
	tenantContext, err := pptAgentTenantContextFromCapability(capability)
	if err != nil {
		writePPTAgentError(w, newPPTAgentError(http.StatusServiceUnavailable, "PPT_TENANT_CONTEXT_UNAVAILABLE", "PPT 会话租户上下文暂时不可用"))
		return
	}
	if !pptAgentTaskMatchesTenantContext(task, tenantContext) {
		writePPTAgentError(w, newPPTAgentError(http.StatusConflict, "PPT_TENANT_CONTEXT_MISMATCH", "当前租户上下文与 PPT 会话不匹配"))
		return
	}
	started, err := a.beginPPTAgentGeneration(
		r.Context(), state, pptAgentBillingForRequest(a, r), pptAgentImageRunnerForRequest(a, r, user),
		owner, task, capability, pageCount,
	)
	if err != nil {
		writePPTAgentStateError(w, err)
		return
	}
	writeJSON(w, pptTaskResponse(started))
}

func (a api) beginPPTAgentGeneration(ctx context.Context, state pptAgentGenerationState, billing pptAgentBillingStore, imageRunner pptAgentImageRunner, owner pptapp.OwnerScope, task pptapp.Task, capability createGenerationTaskRequest, pageCount int) (pptapp.Task, error) {
	taskID := strings.TrimSpace(task.TaskID)
	confirmKey := "ppt-confirm:" + taskID
	requestHash := pptAgentConfirmHash(task)
	generationClaim, claimedTask, err := state.BeginGenerationClaim(ctx, owner, taskID, confirmKey, requestHash)
	if err != nil {
		return pptapp.Task{}, err
	}
	if generationClaim.CompletedReplay || claimedTask.Stage == pptapp.StageReady {
		return claimedTask, nil
	}
	task = claimedTask
	billingRequest := pptAgentConfirmBillingRequest(task, capability, pageCount)
	billingTask, err := billing.CreatePendingGenerationTask(billingRequest)
	if err != nil {
		current, found, lookupErr := pptAgentBillingTaskByClientRequest(billing, owner.UserID, billingRequest.ClientRequestID)
		if lookupErr != nil {
			return pptapp.Task{}, newPPTAgentError(http.StatusServiceUnavailable, "PPT_BILLING_FINALIZE_FAILED", "PPT 费用预留状态暂时无法确认，请稍后重试")
		}
		if found {
			billingTask = current
		} else {
			if _, failErr := state.FailGenerationClaim(ctx, owner, taskID, generationClaim, "PPT_BILLING_RESERVATION_FAILED"); failErr != nil {
				return pptapp.Task{}, failErr
			}
			return pptapp.Task{}, newPPTAgentError(http.StatusPaymentRequired, "PPT_BILLING_RESERVATION_FAILED", "PPT 生成费用预留失败，请检查额度后重试")
		}
	}
	if err := validatePPTAgentBillingIdentity(billingTask, task, billingRequest); err != nil {
		return pptapp.Task{}, err
	}
	generating, err := bindPPTAgentGenerationBilling(ctx, state, billing, owner, taskID, generationClaim, billingTask)
	if err != nil {
		return pptapp.Task{}, err
	}
	if generating.Stage == pptapp.StageReady {
		return generating, nil
	}
	if recovered, handled, recoveryErr := recoverPPTAgentBillingTerminal(ctx, state, owner, generating, billingTask); handled {
		return recovered, recoveryErr
	}
	claim, claimedTask, err := state.ClaimGenerationRun(ctx, owner, taskID, time.Now().UTC())
	if errors.Is(err, pptapp.ErrGenerationAlreadyRunning) {
		return generating, nil
	}
	if err != nil {
		return pptapp.Task{}, err
	}
	a.startPPTAgentGeneration(pptAgentGenerationRun{
		state: state, billing: billing, imageRunner: imageRunner,
		owner: owner, taskID: taskID, claim: claim, billingTask: billingTask, billingRequest: billingRequest,
	})
	return claimedTask, nil
}

func (a api) cancelPPTSession(w http.ResponseWriter, r *http.Request) {
	_, user, err := a.authenticatedUser(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	key, err := pptAgentIdempotencyKey(r)
	if err != nil {
		writePPTAgentError(w, err)
		return
	}
	var body *pptAgentCancelRequest
	if err := decodePPTAgentJSON(r, &body); err != nil {
		writePPTAgentError(w, newPPTAgentError(http.StatusBadRequest, "PPT_REQUEST_INVALID", "请求格式无效"))
		return
	}
	if body == nil {
		writePPTAgentError(w, newPPTAgentError(http.StatusBadRequest, "PPT_REQUEST_INVALID", "请求格式无效"))
		return
	}
	taskID := strings.TrimSpace(r.PathValue("taskId"))
	state := pptAgentGenerationStateForRequest(a, r)
	owner, err := pptOwnerForCapability(a.store, user)
	if err != nil {
		writePPTAgentError(w, newPPTAgentError(http.StatusServiceUnavailable, "PPT_TENANT_CONTEXT_UNAVAILABLE", "PPT 会话租户上下文暂时不可用"))
		return
	}
	task, err := state.GetTask(r.Context(), owner, taskID)
	if err != nil {
		writePPTAgentStateError(w, err)
		return
	}
	billing := pptAgentBillingForRequest(a, r)
	if task.Stage == pptapp.StageGenerating && strings.TrimSpace(task.BillingTaskID) == "" {
		writePPTAgentError(w, newPPTAgentError(http.StatusServiceUnavailable, "PPT_BILLING_FINALIZE_FAILED", "PPT 费用状态暂时无法确认，请稍后重试"))
		return
	}
	cancelHash := pptAgentCancelHash(task)
	claim, claimedTask, err := state.BeginCancel(r.Context(), owner, taskID, key, cancelHash)
	if errors.Is(err, pptapp.ErrOperationInProgress) {
		latest, latestErr := state.GetTask(r.Context(), owner, taskID)
		generationClaim, hasClaim := pptAgentLiveGenerationClaim(latest)
		if latestErr == nil && latest.Stage == pptapp.StageOutlineReady && hasClaim && pptAgentGenerationClaimIsStale(latest, generationClaim, time.Now().UTC()) {
			billingTask, found, lookupErr := pptAgentBillingTaskByClientRequest(billing, owner.UserID, "ppt-confirm:"+taskID)
			if lookupErr != nil {
				writePPTAgentError(w, newPPTAgentError(http.StatusServiceUnavailable, "PPT_BILLING_FINALIZE_FAILED", "PPT 费用预留状态暂时无法确认，请稍后重试"))
				return
			}
			if found && pptAgentBillingCaptured(billingTask) {
				writePPTAgentError(w, newPPTAgentError(http.StatusServiceUnavailable, "PPT_BILLING_FINALIZE_FAILED", "PPT 费用已确认但生成状态不完整，请稍后重试"))
				return
			}
			if found {
				if identityErr := validatePPTAgentStoredBillingIdentity(billingTask, latest); identityErr != nil {
					writePPTAgentError(w, identityErr)
					return
				}
				if _, bindErr := state.BindGenerationBilling(r.Context(), owner, taskID, generationClaim, billingTask.ID); bindErr != nil {
					writePPTAgentStateError(w, bindErr)
					return
				}
			} else if _, failErr := state.FailGenerationClaim(r.Context(), owner, taskID, generationClaim, pptapp.ErrSessionCancelled.Error()); failErr != nil {
				writePPTAgentStateError(w, failErr)
				return
			}
			latest, err = state.GetTask(r.Context(), owner, taskID)
			if err == nil {
				claim, claimedTask, err = state.BeginCancel(r.Context(), owner, taskID, key, pptAgentCancelHash(latest))
			}
		}
	}
	if err != nil {
		writePPTAgentStateError(w, err)
		return
	}
	if claimedTask.Stage == pptapp.StageCancelled {
		writeJSON(w, pptTaskResponse(claimedTask))
		return
	}
	a.cancelPPTAgentGenerationRun(taskID)
	if strings.TrimSpace(claimedTask.BillingTaskID) != "" {
		settled, settleErr := finalizePPTAgentBillingCancellation(billing, claimedTask.BillingTaskID)
		if settleErr != nil {
			writePPTAgentError(w, newPPTAgentError(http.StatusServiceUnavailable, "PPT_BILLING_FINALIZE_FAILED", "PPT 费用释放失败，请稍后重试"))
			return
		}
		if pptAgentBillingCaptured(settled) {
			runClaim, exact := pptAgentGenerationClaimForCompleteTask(claimedTask)
			if !exact {
				writePPTAgentError(w, newPPTAgentError(http.StatusServiceUnavailable, "PPT_BILLING_FINALIZE_FAILED", "PPT 费用已确认但生成状态不完整，请稍后重试"))
				return
			}
			if _, finalizeErr := state.CompleteGenerationAfterCapture(r.Context(), owner, taskID, runClaim); finalizeErr != nil {
				writePPTAgentStateError(w, finalizeErr)
				return
			}
			writePPTAgentStateError(w, pptapp.ErrBillingAlreadyCaptured)
			return
		}
		if !pptAgentBillingReleased(settled) {
			writePPTAgentError(w, newPPTAgentError(http.StatusServiceUnavailable, "PPT_BILLING_FINALIZE_FAILED", "PPT 费用状态暂时无法确认，请稍后重试"))
			return
		}
	}
	cancelled, err := state.CompleteCancel(r.Context(), owner, taskID, claim)
	if err != nil {
		writePPTAgentStateError(w, err)
		return
	}
	writeJSON(w, pptTaskResponse(cancelled))
}

func (a api) startPPTAgentGeneration(run pptAgentGenerationRun) {
	ctx, cancel := context.WithTimeout(context.Background(), pptAgentGenerationRunTimeout)
	control := &pptAgentGenerationControl{cancel: cancel, done: make(chan struct{})}
	if a.pptGenerationRuns != nil {
		if _, loaded := a.pptGenerationRuns.LoadOrStore(run.taskID, control); loaded {
			cancel()
			return
		}
	}
	go func() {
		defer cancel()
		defer close(control.done)
		if a.pptGenerationRuns != nil {
			defer a.pptGenerationRuns.Delete(run.taskID)
		}
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("ppt agent generation panic task_id=%s", run.taskID)
			}
		}()
		runPPTAgentGeneration(ctx, run)
	}()
}

func (a api) cancelPPTAgentGenerationRun(taskID string) {
	if a.pptGenerationRuns == nil {
		return
	}
	value, ok := a.pptGenerationRuns.LoadAndDelete(strings.TrimSpace(taskID))
	if !ok {
		return
	}
	control, ok := value.(*pptAgentGenerationControl)
	if !ok || control == nil {
		return
	}
	control.cancel()
	timer := time.NewTimer(pptAgentGenerationCancelWait)
	defer timer.Stop()
	select {
	case <-control.done:
	case <-timer.C:
	}
}

func runPPTAgentGeneration(ctx context.Context, run pptAgentGenerationRun) {
	settled := false
	failureCode := "PPT_GENERATION_FAILED"
	defer func() {
		recovered := recover()
		if !settled {
			cleanupPPTAgentGeneration(run, failureCode)
		}
		if recovered != nil {
			log.Printf("ppt agent generation panic task_id=%s", run.taskID)
		}
	}()

	task, err := run.state.GetTask(ctx, run.owner, run.taskID)
	if err != nil || ctx.Err() != nil || task.Stage != pptapp.StageGenerating {
		return
	}
	currentBilling, ok, err := pptAgentBillingTaskByID(run.billing, run.billingTask.ID)
	if err != nil || !ok {
		return
	}
	run.billingTask = currentBilling
	if pptAgentBillingCaptured(run.billingTask) {
		if claim, exact := pptAgentGenerationClaimForCompleteTask(task); exact {
			ready, completeErr := run.state.CompleteGenerationAfterCapture(ctx, run.owner, run.taskID, claim)
			if completeErr == nil {
				settled = true
				if run.imageRunner != nil {
					_ = run.imageRunner(ctx, ready)
				}
			}
		}
		return
	}
	if pptAgentBillingReleased(run.billingTask) {
		if _, failErr := run.state.FailGenerationAfterRelease(ctx, run.owner, run.taskID, run.claim, "PPT_BILLING_FINALIZE_FAILED"); failErr == nil {
			settled = true
		}
		return
	}
	_, err = executePPTAgentPages(ctx, run.state, run.owner, run.taskID, run.claim)
	if err != nil {
		var pageErr pptAgentPageExecutionError
		if errors.As(err, &pageErr) {
			failureCode = pageErr.code
		}
		return
	}
	if err := requirePPTAgentGenerationCaptureReady(ctx, run.state, run.owner, run.taskID, run.claim); err != nil {
		var pageErr pptAgentPageExecutionError
		if errors.As(err, &pageErr) {
			failureCode = pageErr.code
		}
		return
	}
	completedBilling, err := run.billing.CompleteGenerationTask(run.billingTask.ID, run.billingRequest)
	if err != nil {
		recoveredBilling, recoveryErr := run.billing.CreatePendingGenerationTask(run.billingRequest)
		if recoveryErr != nil || !pptAgentBillingCaptured(recoveredBilling) {
			failureCode = "PPT_BILLING_FINALIZE_FAILED"
			return
		}
		completedBilling = recoveredBilling
	}
	if !pptAgentBillingCaptured(completedBilling) {
		failureCode = "PPT_BILLING_FINALIZE_FAILED"
		return
	}
	ready, err := run.state.CompleteGenerationAfterCapture(ctx, run.owner, run.taskID, run.claim)
	if err != nil {
		return
	}
	settled = true
	if run.imageRunner != nil {
		_ = run.imageRunner(ctx, ready)
	}
}

func bindPPTAgentGenerationBilling(ctx context.Context, state pptAgentGenerationState, billing pptAgentBillingStore, owner pptapp.OwnerScope, taskID string, claim pptapp.OperationClaim, billingTask generationTask) (pptapp.Task, error) {
	generating, err := state.BindGenerationBilling(ctx, owner, taskID, claim, billingTask.ID)
	if err == nil {
		return generating, nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), pptAgentGenerationCleanupTimeout)
	defer cancel()
	if recovered, retryErr := state.BindGenerationBilling(cleanupCtx, owner, taskID, claim, billingTask.ID); retryErr == nil {
		return recovered, nil
	}
	settled, settleErr := finalizePPTAgentBillingFailure(billing, billingTask.ID, "ppt agent billing binding failed")
	if settleErr != nil || pptAgentBillingCaptured(settled) || !pptAgentBillingReleased(settled) {
		return pptapp.Task{}, newPPTAgentError(http.StatusServiceUnavailable, "PPT_BILLING_FINALIZE_FAILED", "PPT 费用绑定状态暂时无法确认，请稍后重试")
	}
	if _, failErr := state.FailGenerationClaim(cleanupCtx, owner, taskID, claim, "PPT_BILLING_BINDING_FAILED"); failErr != nil {
		return pptapp.Task{}, newPPTAgentError(http.StatusServiceUnavailable, "PPT_BILLING_FINALIZE_FAILED", "PPT 费用已释放，但生成状态清理失败，请稍后重试")
	}
	return pptapp.Task{}, newPPTAgentError(http.StatusServiceUnavailable, "PPT_BILLING_FINALIZE_FAILED", "PPT 费用绑定失败，已释放预留，请重试")
}

func requirePPTAgentGenerationCaptureReady(ctx context.Context, state pptAgentGenerationState, owner pptapp.OwnerScope, taskID string, claim pptapp.GenerationClaim) error {
	latest, err := state.GetTask(ctx, owner, taskID)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if pptAgentTaskHasLiveCancel(latest) || latest.Stage == pptapp.StageCancelled {
		return pptapp.ErrSessionCancelled
	}
	if !pptAgentRunOwnsTask(latest, claim) {
		return pptapp.ErrGenerationRunMismatch
	}
	if _, exact := pptAgentGenerationClaimForCompleteTask(latest); !exact {
		return pptAgentPageExecutionError{code: "PPT_GENERATION_INCOMPLETE", err: pptapp.ErrGenerationIncomplete}
	}
	return nil
}

// executePPTAgentPages is the shared text-page executor. It owns no billing
// transition and deliberately stops at GENERATING after exact page coverage;
// the caller remains the sole authority for capture/release and READY/FAILED.
func executePPTAgentPages(ctx context.Context, state pptAgentGenerationState, owner pptapp.OwnerScope, taskID string, claim pptapp.GenerationClaim) (pptapp.Task, error) {
	task, err := state.GetTask(ctx, owner, taskID)
	if err != nil {
		return pptapp.Task{}, err
	}
	if ctx.Err() != nil {
		return pptapp.Task{}, ctx.Err()
	}
	if !pptAgentRunOwnsTask(task, claim) {
		if pptAgentTaskHasLiveCancel(task) || task.Stage == pptapp.StageCancelled {
			return pptapp.Task{}, pptapp.ErrSessionCancelled
		}
		return pptapp.Task{}, pptapp.ErrGenerationRunMismatch
	}
	if task.Outline == nil || len(task.Outline.Slides) != task.SlideCount {
		return pptapp.Task{}, pptAgentPageExecutionError{code: "PPT_GENERATION_INCOMPLETE", err: pptapp.ErrGenerationIncomplete}
	}
	request := pptAgentGenerateRequest(task)
	for _, outlineSlide := range task.Outline.Slides {
		if ctx.Err() != nil {
			return pptapp.Task{}, ctx.Err()
		}
		latest, getErr := state.GetTask(ctx, owner, taskID)
		if getErr != nil {
			return pptapp.Task{}, getErr
		}
		if !pptAgentRunOwnsTask(latest, claim) {
			if pptAgentTaskHasLiveCancel(latest) || latest.Stage == pptapp.StageCancelled {
				return pptapp.Task{}, pptapp.ErrSessionCancelled
			}
			return pptapp.Task{}, pptapp.ErrGenerationRunMismatch
		}
		if pptAgentTaskHasSlide(latest, outlineSlide.Page) {
			continue
		}
		slide := canonicalPPTAgentGeneratedSlide(pptapp.SlideFromOutline(outlineSlide, request), owner.TenantID)
		if _, persistErr := state.PersistGeneratedSlide(ctx, owner, taskID, claim, slide); persistErr != nil {
			if ctx.Err() != nil || errors.Is(persistErr, pptapp.ErrSessionCancelled) || errors.Is(persistErr, pptapp.ErrGenerationRunMismatch) {
				return pptapp.Task{}, persistErr
			}
			return pptapp.Task{}, pptAgentPageExecutionError{code: "PPT_SESSION_STORAGE_UNAVAILABLE", err: persistErr}
		}
	}
	latest, err := state.GetTask(ctx, owner, taskID)
	if err != nil {
		return pptapp.Task{}, err
	}
	if !pptAgentRunOwnsTask(latest, claim) {
		if pptAgentTaskHasLiveCancel(latest) || latest.Stage == pptapp.StageCancelled {
			return pptapp.Task{}, pptapp.ErrSessionCancelled
		}
		return pptapp.Task{}, pptapp.ErrGenerationRunMismatch
	}
	if _, exact := pptAgentGenerationClaimForCompleteTask(latest); !exact {
		return pptapp.Task{}, pptAgentPageExecutionError{code: "PPT_GENERATION_INCOMPLETE", err: pptapp.ErrGenerationIncomplete}
	}
	return latest, nil
}

func canonicalPPTAgentGeneratedSlide(slide pptapp.Slide, expectedTenantID string) pptapp.Slide {
	blocks := make([]pptapp.SlideBlock, 0, len(slide.Blocks))
	for _, block := range slide.Blocks {
		if block.Type == "image" {
			if err := pptapp.ValidateVisualStorageReference(expectedTenantID, block.ImageRef); err != nil {
				continue
			}
		}
		blocks = append(blocks, block)
	}
	slide.Blocks = blocks
	return pptapp.NormalizeSlideIR(slide)
}

func cleanupPPTAgentGeneration(run pptAgentGenerationRun, errorCode string) {
	ctx, cancel := context.WithTimeout(context.Background(), pptAgentGenerationCleanupTimeout)
	defer cancel()
	currentBilling, ok, err := pptAgentBillingTaskByID(run.billing, run.billingTask.ID)
	if err != nil || !ok {
		return
	}
	if pptAgentBillingCaptured(currentBilling) {
		latest, getErr := run.state.GetTask(ctx, run.owner, run.taskID)
		if getErr == nil && pptAgentRunOwnsCurrentLease(latest, run.claim) {
			if claim, exact := pptAgentGenerationClaimForCompleteTask(latest); exact {
				_, _ = run.state.CompleteGenerationAfterCapture(ctx, run.owner, run.taskID, claim)
			}
		}
		return
	}
	if pptAgentBillingReleased(currentBilling) {
		_, _ = run.state.FailGenerationAfterRelease(ctx, run.owner, run.taskID, run.claim, errorCode)
		return
	}
	cleanupClaim, _, err := run.state.AcquireGenerationCleanupFence(ctx, run.owner, run.taskID, run.claim, time.Now().UTC())
	if err != nil {
		return
	}
	settled, err := finalizePPTAgentBillingFailure(run.billing, run.billingTask.ID, "ppt agent generation failed")
	if err != nil {
		return
	}
	if pptAgentBillingCaptured(settled) {
		latest, getErr := run.state.GetTask(ctx, run.owner, run.taskID)
		if getErr == nil && pptAgentRunOwnsCurrentLease(latest, cleanupClaim) {
			if claim, exact := pptAgentGenerationClaimForCompleteTask(latest); exact {
				_, _ = run.state.CompleteGenerationAfterCapture(ctx, run.owner, run.taskID, claim)
			}
		}
		return
	}
	if !pptAgentBillingReleased(settled) {
		return
	}
	_, _ = run.state.FailGenerationAfterRelease(ctx, run.owner, run.taskID, cleanupClaim, errorCode)
}

func recoverPPTAgentBillingTerminal(ctx context.Context, state pptAgentGenerationState, owner pptapp.OwnerScope, task pptapp.Task, billingTask generationTask) (pptapp.Task, bool, error) {
	if pptAgentBillingCaptured(billingTask) {
		claim, exact := pptAgentGenerationClaimForCompleteTask(task)
		if !exact {
			return pptapp.Task{}, true, newPPTAgentError(http.StatusServiceUnavailable, "PPT_BILLING_FINALIZE_FAILED", "PPT 费用已确认但生成状态不完整，请稍后重试")
		}
		ready, err := state.CompleteGenerationAfterCapture(ctx, owner, task.TaskID, claim)
		return ready, true, err
	}
	if pptAgentBillingReleased(billingTask) && task.GenerationLease != nil {
		claim := pptapp.GenerationClaim{RunToken: task.GenerationLease.RunToken, LeaseUntil: task.GenerationLease.LeaseUntil}
		failed, err := state.FailGenerationAfterRelease(ctx, owner, task.TaskID, claim, "PPT_BILLING_FINALIZE_FAILED")
		return failed, true, err
	}
	return pptapp.Task{}, false, nil
}

func finalizePPTAgentBillingCancellation(store pptAgentBillingStore, billingTaskID string) (generationTask, error) {
	return finalizePPTAgentBillingFailure(store, billingTaskID, "ppt agent generation cancelled")
}

func finalizePPTAgentBillingFailure(store pptAgentBillingStore, billingTaskID, message string) (generationTask, error) {
	current, ok, lookupErr := pptAgentBillingTaskByID(store, billingTaskID)
	if lookupErr != nil {
		return generationTask{}, lookupErr
	}
	if !ok {
		return generationTask{}, errors.New("ppt billing task not found")
	}
	if pptAgentBillingCaptured(current) || pptAgentBillingReleased(current) {
		return current, nil
	}
	settled, err := store.FailGenerationTask(billingTaskID, message)
	if err == nil && (pptAgentBillingCaptured(settled) || pptAgentBillingReleased(settled)) {
		return settled, nil
	}
	current, ok, lookupErr = pptAgentBillingTaskByID(store, billingTaskID)
	if lookupErr != nil {
		return generationTask{}, lookupErr
	}
	if !ok || (!pptAgentBillingCaptured(current) && !pptAgentBillingReleased(current)) {
		if err != nil {
			return generationTask{}, err
		}
		return generationTask{}, errors.New("ppt billing task has no terminal state")
	}
	return current, nil
}

func validatePPTAgentConfirmedOutline(task pptapp.Task, skill skills.Skill) (int, error) {
	if task.Outline == nil || len(task.Outline.Slides) == 0 {
		return 0, newPPTAgentError(http.StatusConflict, "PPT_OUTLINE_REQUIRED", "请先生成并确认 PPT 大纲")
	}
	pageCount := len(task.Outline.Slides)
	if task.SlideCount != pageCount || pageCount > skill.MaxSlides {
		return 0, newPPTAgentError(http.StatusBadRequest, "PPT_CAPABILITY_DENIED", "PPT 大纲页数超出当前能力限制")
	}
	for index, slide := range task.Outline.Slides {
		if slide.Page != index+1 || strings.TrimSpace(slide.Title) == "" || strings.TrimSpace(slide.Summary) == "" || len(slide.BulletPoints) == 0 {
			return 0, newPPTAgentError(http.StatusConflict, "PPT_OUTLINE_REQUIRED", "PPT 大纲内容不完整")
		}
	}
	return pageCount, nil
}

func pptAgentConfirmBillingRequest(task pptapp.Task, capability createGenerationTaskRequest, pageCount int) createGenerationTaskRequest {
	request := cloneGenerationCreateRequest(capability)
	request.ClientRequestID = "ppt-confirm:" + strings.TrimSpace(task.TaskID)
	request.UserID = strings.TrimSpace(task.UserID)
	request.Type = "PPT_GENERATION"
	request.ModuleCode = modulePPTGeneration
	request.Prompt = strings.TrimSpace(task.Prompt)
	if request.Params == nil {
		request.Params = map[string]any{}
	}
	request.Params["page_count"] = pageCount
	request.Params["with_images"] = task.WithImages()
	request.Params["source_type"] = "ppt_agent"
	request.Params["source_task_id"] = strings.TrimSpace(task.TaskID)
	return request
}

func validatePPTAgentBillingIdentity(billingTask generationTask, task pptapp.Task, request createGenerationTaskRequest) error {
	if strings.TrimSpace(billingTask.ID) == "" ||
		strings.TrimSpace(billingTask.UserID) != strings.TrimSpace(task.UserID) ||
		strings.TrimSpace(billingTask.ClientRequestID) != "ppt-confirm:"+strings.TrimSpace(task.TaskID) ||
		!strings.EqualFold(strings.TrimSpace(billingTask.Type), "PPT_GENERATION") ||
		canonicalModuleCode(billingTask.ModuleCode) != modulePPTGeneration ||
		strings.TrimSpace(billingTask.TenantID) != strings.TrimSpace(task.TenantID) ||
		strings.TrimSpace(billingTask.OrganizationID) != strings.TrimSpace(task.OrganizationID) ||
		!strings.EqualFold(strings.TrimSpace(billingTask.BillingAccountType), strings.TrimSpace(task.BillingScope)) ||
		strings.TrimSpace(billingTask.BillingAccountID) != strings.TrimSpace(task.BillingAccountID) ||
		strings.TrimSpace(request.UserID) != strings.TrimSpace(task.UserID) ||
		strings.TrimSpace(stringValue(request.Params["tenant_id"])) != strings.TrimSpace(task.TenantID) ||
		strings.TrimSpace(stringValue(request.Params["organization_id"])) != strings.TrimSpace(task.OrganizationID) ||
		!strings.EqualFold(strings.TrimSpace(stringValue(request.Params["billing_scope"])), strings.TrimSpace(task.BillingScope)) ||
		strings.TrimSpace(stringValue(request.Params["billing_account_id"])) != strings.TrimSpace(task.BillingAccountID) {
		return newPPTAgentError(http.StatusServiceUnavailable, "PPT_BILLING_FINALIZE_FAILED", "PPT 费用预留身份校验失败，请稍后重试")
	}
	return nil
}

func validatePPTAgentStoredBillingIdentity(billingTask generationTask, task pptapp.Task) error {
	request := createGenerationTaskRequest{
		UserID: billingTask.UserID,
		Params: map[string]any{
			"tenant_id": billingTask.TenantID, "organization_id": billingTask.OrganizationID,
			"billing_scope": billingTask.BillingAccountType, "billing_account_id": billingTask.BillingAccountID,
		},
	}
	return validatePPTAgentBillingIdentity(billingTask, task, request)
}

func pptAgentConfirmHash(task pptapp.Task) string {
	payload := struct {
		TaskID           string                `json:"taskId"`
		TenantID         string                `json:"tenantId"`
		OrganizationID   string                `json:"organizationId"`
		ContextType      string                `json:"contextType"`
		BillingScope     string                `json:"billingScope"`
		BillingAccountID string                `json:"billingAccountId"`
		SkillCode        string                `json:"skillCode"`
		SlideCount       int                   `json:"slideCount"`
		Outline          []pptapp.OutlineSlide `json:"outline"`
	}{
		TaskID: strings.TrimSpace(task.TaskID), TenantID: strings.TrimSpace(task.TenantID), OrganizationID: strings.TrimSpace(task.OrganizationID),
		ContextType: strings.ToUpper(strings.TrimSpace(task.ContextType)), BillingScope: strings.ToUpper(strings.TrimSpace(task.BillingScope)),
		BillingAccountID: strings.TrimSpace(task.BillingAccountID), SkillCode: strings.TrimSpace(task.SkillCode), SlideCount: task.SlideCount,
	}
	if task.Outline != nil {
		payload.Outline = append([]pptapp.OutlineSlide(nil), task.Outline.Slides...)
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func pptAgentCancelHash(task pptapp.Task) string {
	value := strings.Join([]string{
		strings.TrimSpace(task.TaskID), strings.TrimSpace(task.TenantID), strings.TrimSpace(task.OrganizationID),
		strings.ToUpper(strings.TrimSpace(task.ContextType)), strings.ToUpper(strings.TrimSpace(task.BillingScope)), strings.TrimSpace(task.BillingAccountID),
	}, "\x00")
	sum := sha256.Sum256([]byte("cancel\x00" + value))
	return hex.EncodeToString(sum[:])
}

func pptAgentGenerateRequest(task pptapp.Task) pptapp.GenerateRequest {
	return pptapp.GenerateRequest{
		Owner:    pptapp.OwnerScope{TenantID: task.TenantID, UserID: task.UserID},
		TenantID: task.TenantID, UserID: task.UserID, ClientRequestID: task.ClientRequestID, Prompt: task.Prompt, SlideCount: task.SlideCount,
		Language: task.Language, Tone: task.Tone, TextContent: task.TextContent, Audience: task.Audience, Scenario: task.Scenario,
		GenerationAspectRatio: task.GenerationAspectRatio, Theme: task.Theme, AutoThemeEnabled: task.AutoThemeEnabled,
		EnableWebSearch: task.EnableWebSearch, ImageSource: task.ImageSource, TextModel: task.TextModel, ImageModel: task.ImageModel,
		ImageStyle: task.ImageStyle, PeopleStyle: task.PeopleStyle, ImageLighting: task.ImageLighting,
		ImageComposition: task.ImageComposition, TextInImage: task.TextInImage, Outline: task.Outline,
	}
}

func pptAgentGenerationStateForRequest(a api, r *http.Request) pptAgentGenerationState {
	if state, ok := r.Context().Value(pptAgentGenerationStateContextKey{}).(pptAgentGenerationState); ok && state != nil {
		return state
	}
	return pptAgentGenerationServiceAdapter{service: a.pptService}
}

func pptAgentBillingForRequest(a api, r *http.Request) pptAgentBillingStore {
	if billing, ok := r.Context().Value(pptAgentBillingContextKey{}).(pptAgentBillingStore); ok && billing != nil {
		return billing
	}
	return a.store
}

func pptAgentImageRunnerForRequest(a api, r *http.Request, user adminUser) pptAgentImageRunner {
	if runner, ok := r.Context().Value(pptAgentImageRunnerContextKey{}).(pptAgentImageRunner); ok && runner != nil {
		return runner
	}
	return func(ctx context.Context, task pptapp.Task) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !pptAutoImageEnabled(a.cfg) || !shouldAutoGeneratePPTImages(pptAgentGenerateRequest(task), a.cfg) {
			return nil
		}
		a.runPPTTaskImageGeneration(user, task)
		return nil
	}
}

func pptAgentIdempotencyKey(r *http.Request) (string, error) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		return "", newPPTAgentError(http.StatusBadRequest, "PPT_IDEMPOTENCY_KEY_REQUIRED", "缺少 Idempotency-Key")
	}
	if len(key) > 256 || strings.ContainsAny(key, "\r\n") {
		return "", newPPTAgentError(http.StatusBadRequest, "PPT_IDEMPOTENCY_KEY_INVALID", "Idempotency-Key 无效")
	}
	return key, nil
}

func pptAgentBillingTaskByID(store pptAgentBillingStore, id string) (generationTask, bool, error) {
	tasks, err := store.ListGenerationTasks()
	if err != nil {
		return generationTask{}, false, err
	}
	for _, task := range tasks {
		if strings.TrimSpace(task.ID) == strings.TrimSpace(id) {
			return task, true, nil
		}
	}
	return generationTask{}, false, nil
}

func pptAgentBillingTaskByClientRequest(store pptAgentBillingStore, userID, clientRequestID string) (generationTask, bool, error) {
	tasks, err := store.ListGenerationTasks()
	if err != nil {
		return generationTask{}, false, err
	}
	userID = strings.TrimSpace(userID)
	clientRequestID = strings.TrimSpace(clientRequestID)
	for _, task := range tasks {
		if strings.TrimSpace(task.UserID) == userID && strings.TrimSpace(task.ClientRequestID) == clientRequestID {
			return task, true, nil
		}
	}
	return generationTask{}, false, nil
}

func pptAgentBillingCaptured(task generationTask) bool {
	return strings.EqualFold(strings.TrimSpace(task.Status), "SUCCEEDED") ||
		strings.EqualFold(strings.TrimSpace(task.TaskStatus), taskStatusSucceeded) ||
		strings.EqualFold(strings.TrimSpace(task.BillingStatus), billingStatusCaptured)
}

func pptAgentBillingReleased(task generationTask) bool {
	return strings.EqualFold(strings.TrimSpace(task.BillingStatus), billingStatusReleased) ||
		(strings.EqualFold(strings.TrimSpace(task.Status), "FAILED") && !pptAgentBillingCaptured(task))
}

func pptAgentRunOwnsTask(task pptapp.Task, claim pptapp.GenerationClaim) bool {
	return pptAgentRunOwnsCurrentLease(task, claim) && !pptAgentTaskHasLiveCancel(task)
}

func pptAgentRunOwnsCurrentLease(task pptapp.Task, claim pptapp.GenerationClaim) bool {
	return task.Stage == pptapp.StageGenerating && task.GenerationLease != nil &&
		strings.TrimSpace(task.GenerationLease.RunToken) == strings.TrimSpace(claim.RunToken)
}

func pptAgentTaskHasLiveCancel(task pptapp.Task) bool {
	for _, record := range task.IdempotencyRecords {
		if strings.EqualFold(strings.TrimSpace(record.Scope), "cancel") && strings.EqualFold(strings.TrimSpace(record.State), "processing") {
			return true
		}
	}
	return false
}

func pptAgentLiveGenerationClaim(task pptapp.Task) (pptapp.OperationClaim, bool) {
	for _, record := range task.IdempotencyRecords {
		if strings.EqualFold(strings.TrimSpace(record.Scope), "confirm-outline") &&
			strings.EqualFold(strings.TrimSpace(record.State), "processing") && strings.TrimSpace(record.OperationToken) != "" {
			return pptapp.OperationClaim{
				Scope: record.Scope, Key: record.Key, RequestHash: record.RequestHash, OperationToken: record.OperationToken,
			}, true
		}
	}
	return pptapp.OperationClaim{}, false
}

func pptAgentGenerationClaimIsStale(task pptapp.Task, claim pptapp.OperationClaim, now time.Time) bool {
	for _, record := range task.IdempotencyRecords {
		if !strings.EqualFold(strings.TrimSpace(record.Scope), strings.TrimSpace(claim.Scope)) ||
			strings.TrimSpace(record.Key) != strings.TrimSpace(claim.Key) ||
			strings.TrimSpace(record.OperationToken) != strings.TrimSpace(claim.OperationToken) {
			continue
		}
		updatedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(record.UpdatedAt))
		return err == nil && !updatedAt.Add(5*time.Minute).After(now.UTC())
	}
	return false
}

func pptAgentTaskHasSlide(task pptapp.Task, page int) bool {
	for _, slide := range task.Slides {
		if slide.Page == page && strings.TrimSpace(slide.ID) != "" {
			return true
		}
	}
	return false
}

func pptAgentGenerationClaimForCompleteTask(task pptapp.Task) (pptapp.GenerationClaim, bool) {
	if task.GenerationLease == nil || strings.TrimSpace(task.GenerationLease.RunToken) == "" || task.SlideCount <= 0 || len(task.Slides) != task.SlideCount {
		return pptapp.GenerationClaim{}, false
	}
	pages := make(map[int]struct{}, task.SlideCount)
	ids := make(map[string]struct{}, task.SlideCount)
	for _, slide := range task.Slides {
		id := strings.TrimSpace(slide.ID)
		if id == "" || slide.Page < 1 || slide.Page > task.SlideCount {
			return pptapp.GenerationClaim{}, false
		}
		if _, exists := pages[slide.Page]; exists {
			return pptapp.GenerationClaim{}, false
		}
		if _, exists := ids[id]; exists {
			return pptapp.GenerationClaim{}, false
		}
		pages[slide.Page] = struct{}{}
		ids[id] = struct{}{}
	}
	return pptapp.GenerationClaim{RunToken: task.GenerationLease.RunToken, LeaseUntil: task.GenerationLease.LeaseUntil}, len(pages) == task.SlideCount
}
