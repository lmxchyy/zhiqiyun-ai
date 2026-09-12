package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	pe "xianzhi-ai/backend-go/internal/providerexecution"
)

const (
	recoveryActionDiagnose       = "DIAGNOSE"
	recoveryActionRedrive        = "REDRIVE"
	recoveryActionResolveRelease = "RESOLVE_RELEASE"
	recoveryActionResolveCapture = "RESOLVE_CAPTURE"
	recoveryActionManualReview   = "MARK_MANUAL_REVIEW"
)

type recoveryActionRequest struct {
	Action   string         `json:"action"`
	Reason   string         `json:"reason"`
	Evidence map[string]any `json:"evidence,omitempty"`
}

type recoveryDiagnosisResponse struct {
	Task        map[string]any `json:"task"`
	Provider    map[string]any `json:"providerExecution"`
	Messaging   map[string]any `json:"messaging"`
	PPT         map[string]any `json:"ppt"`
	Recovery    map[string]any `json:"recovery"`
	GeneratedAt string         `json:"generatedAt"`
}

func (a api) generationRecoveryDiagnosis(w http.ResponseWriter, r *http.Request) {
	diagnosis, task, err := a.buildGenerationRecoveryDiagnosis(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		recoveryWriteError(w, err)
		return
	}
	writeJSON(w, diagnosis)
	_ = task
}

func (a api) generationRecoveryAction(w http.ResponseWriter, r *http.Request) {
	var req recoveryActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		recoveryWriteError(w, fmt.Errorf("invalid recovery action: %w", err))
		return
	}
	req.Action = strings.ToUpper(strings.TrimSpace(req.Action))
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" {
		recoveryWriteError(w, errors.New("recovery action reason is required"))
		return
	}
	if !validRecoveryAction(req.Action) {
		recoveryWriteError(w, fmt.Errorf("unsupported recovery action %q", req.Action))
		return
	}
	taskID := strings.TrimSpace(r.PathValue("id"))
	before, task, err := a.buildGenerationRecoveryDiagnosis(r.Context(), taskID)
	if err != nil {
		recoveryWriteError(w, err)
		return
	}
	if err := a.validateRecoveryAction(req, before); err != nil {
		recoveryWriteError(w, err)
		return
	}

	actorID, actorRole := actorFromRequest(r)
	if actorID == "" {
		recoveryWriteErrorStatus(w, http.StatusUnauthorized, errors.New("operator identity is required"))
		return
	}
	if req.Action == recoveryActionDiagnose {
		if auditErr := insertRecoveryAudit(r.Context(), a.pgDB(), actorID, actorRole, taskID, req, before, before); auditErr != nil {
			recoveryWriteError(w, fmt.Errorf("recovery audit failed: %w", auditErr))
			return
		}
		writeJSON(w, map[string]any{"diagnosis": before, "action": req.Action, "applied": false})
		return
	}

	var result generationTask
	switch req.Action {
	case recoveryActionManualReview:
		result, err = a.markGenerationManualReview(task, req)
	case recoveryActionRedrive:
		result, err = a.redriveGenerationEvent(task, req)
	case recoveryActionResolveCapture:
		result, err = a.resolveGenerationCapture(task, req)
	case recoveryActionResolveRelease:
		result, err = a.resolveGenerationRelease(task, req)
	}
	if err != nil {
		recoveryWriteError(w, err)
		return
	}
	after, _, afterErr := a.buildGenerationRecoveryDiagnosis(r.Context(), taskID)
	if afterErr != nil {
		after = before
	}
	if auditErr := insertRecoveryAudit(r.Context(), a.pgDB(), actorID, actorRole, taskID, req, before, after); auditErr != nil {
		recoveryWriteError(w, fmt.Errorf("recovery audit failed: %w", auditErr))
		return
	}
	writeJSON(w, map[string]any{"action": req.Action, "applied": true, "task": result, "diagnosis": after})
}

func (a api) buildGenerationRecoveryDiagnosis(ctx context.Context, taskID string) (recoveryDiagnosisResponse, generationTask, error) {
	if taskID == "" {
		return recoveryDiagnosisResponse{}, generationTask{}, errors.New("generation task id is required")
	}
	tasks, err := a.store.ListGenerationTasks()
	if err != nil {
		return recoveryDiagnosisResponse{}, generationTask{}, err
	}
	var task generationTask
	found := false
	for _, item := range tasks {
		if item.ID == taskID {
			task, found = item, true
			break
		}
	}
	if !found {
		return recoveryDiagnosisResponse{}, generationTask{}, sql.ErrNoRows
	}

	provider := map[string]any{"present": false, "status": "none", "queryable": false, "providerRequestIDPresent": false, "fingerprintMatch": "not_evaluated"}
	var execution pe.Execution
	hasExecution := false
	if db := a.pgDB(); db != nil {
		execution, err = pe.NewStore(db).GetLatestByTask(ctx, taskID)
		if err == nil {
			hasExecution = true
			requestIDPresent := execution.ProviderRequestID != nil && strings.TrimSpace(*execution.ProviderRequestID) != ""
			provider = map[string]any{
				"present": true, "status": string(execution.Status), "attempt": execution.Attempt,
				"providerRequestIdPresent": requestIDPresent, "queryable": requestIDPresent,
				"fingerprintMatch": "not_evaluated", "fingerprintStatus": "not_evaluated",
				"capability": execution.Capability, "provider": execution.Provider,
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return recoveryDiagnosisResponse{}, generationTask{}, err
		}
	}

	messaging := map[string]any{"outboxState": "unknown", "inboxState": "unknown", "retry": false, "dlq": false}
	if db := a.pgDB(); db != nil {
		var eventID, eventType, status, lastError string
		var attempts int
		qErr := db.QueryRowContext(ctx, `SELECT event_id,event_type,status,attempt_count,coalesce(last_error,'') FROM outbox_events WHERE aggregate_id=$1 ORDER BY created_at DESC LIMIT 1`, taskID).Scan(&eventID, &eventType, &status, &attempts, &lastError)
		if qErr == nil {
			dlq := strings.Contains(strings.ToLower(lastError), "dlq") || strings.Contains(strings.ToLower(eventType), "dead")
			messaging = map[string]any{"eventId": eventID, "eventType": eventType, "outboxState": status, "attemptCount": attempts, "lastErrorPresent": lastError != "", "retry": attempts > 0, "dlq": dlq}
			var processedAt sql.NullTime
			var inboxResult, inboxError sql.NullString
			_ = db.QueryRowContext(ctx, `SELECT processed_at,result,error_message FROM consumer_inbox WHERE event_id=$1 ORDER BY created_at DESC LIMIT 1`, eventID).Scan(&processedAt, &inboxResult, &inboxError)
			if processedAt.Valid || inboxResult.Valid || inboxError.Valid {
				messaging["inboxState"] = firstNonEmptyString(inboxResult.String, "claimed")
				messaging["inboxErrorPresent"] = inboxError.Valid && inboxError.String != ""
			}
		}
	}

	ppt := map[string]any{"applicable": false}
	if isPPTGenerationType(task.Type) && a.pptService != nil {
		detail, detailErr := a.pptService.GetTask(task.UserID, task.ID)
		if detailErr == nil {
			completed, degraded := 0, 0
			for _, slide := range detail.Slides {
				if strings.EqualFold(strings.TrimSpace(slide.VisualStatus), "success") && (slide.ImageURL != "" || slide.VisualPlan != nil) {
					completed++
				}
				if strings.EqualFold(strings.TrimSpace(slide.VisualStatus), "failed") {
					degraded++
				}
			}
			ppt = map[string]any{
				"applicable": true, "outlineCheckpoint": detail.Outline != nil,
				"slideTotal": len(detail.Slides), "slidePlannedCount": detail.PlannedPages,
				"slideVisualCompletedCount": completed, "failedDegradedCount": degraded,
				"artifactStatus": detail.ArtifactStatus, "assetId": detail.AssetID, "storageRefPresent": detail.StorageRef != "",
				"stage": detail.Stage, "progress": detail.Progress, "currentPage": detail.CurrentPage,
			}
		}
	}

	allowed, blocked, evidenceRequired := allowedRecoveryActions(task, execution, hasExecution, messaging)
	response := recoveryDiagnosisResponse{
		Task:     map[string]any{"taskId": task.ID, "type": task.Type, "status": task.Status, "billingStatus": task.BillingStatus, "reservedPoints": task.ReservedPoints, "capturedPoints": task.CapturedPoints, "releasedPoints": task.ReleasedPoints, "progress": task.Progress, "updatedAt": task.UpdatedAt},
		Provider: provider, Messaging: messaging, PPT: ppt,
		Recovery:    map[string]any{"allowedActions": allowed, "blockedReason": blocked, "requiresManualEvidence": evidenceRequired},
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	return response, task, nil
}

func allowedRecoveryActions(task generationTask, execution pe.Execution, hasExecution bool, messaging map[string]any) ([]string, string, bool) {
	allowed := []string{recoveryActionDiagnose}
	if isRunningGenerationTaskStatus(task.Status) || strings.EqualFold(strings.TrimSpace(task.Status), "MANUAL_REVIEW") {
		allowed = append(allowed, recoveryActionManualReview)
	}
	if hasExecution && execution.Status == pe.Unknown {
		return allowed, "provider execution is UNKNOWN; external evidence is required", true
	}
	if (isRunningGenerationTaskStatus(task.Status) || strings.EqualFold(strings.TrimSpace(task.Status), "MANUAL_REVIEW")) && hasExecution && execution.Status == pe.Succeeded && strings.EqualFold(task.BillingStatus, "RESERVED") {
		return append(allowed, recoveryActionResolveCapture), "", true
	}
	if isRunningGenerationTaskStatus(task.Status) && hasExecution && execution.Status == pe.Failed && execution.ErrorClass != nil && (string(*execution.ErrorClass) == string(pe.DefinitiveNotSubmitted) || string(*execution.ErrorClass) == string(pe.RetryableBeforeSubmit)) {
		allowed = append(allowed, recoveryActionRedrive)
	}
	if isRunningGenerationTaskStatus(task.Status) && messaging["dlq"] == true && (!hasExecution || execution.Status == pe.Failed) {
		allowed = append(allowed, recoveryActionRedrive)
	}
	if (isRunningGenerationTaskStatus(task.Status) || strings.EqualFold(strings.TrimSpace(task.Status), "MANUAL_REVIEW")) && strings.EqualFold(task.BillingStatus, "RESERVED") && !hasExecution {
		allowed = append(allowed, recoveryActionResolveRelease)
	}
	return uniqueStrings(allowed), "", false
}

func (a api) validateRecoveryAction(req recoveryActionRequest, diagnosis recoveryDiagnosisResponse) error {
	allowed, _ := diagnosis.Recovery["allowedActions"].([]string)
	// JSON-shaped maps are built in-process, but keep validation defensive.
	if len(allowed) == 0 {
		if raw, ok := diagnosis.Recovery["allowedActions"].([]any); ok {
			for _, item := range raw {
				allowed = append(allowed, fmt.Sprint(item))
			}
		}
	}
	if req.Action == recoveryActionDiagnose || recoveryContainsString(allowed, req.Action) {
		return nil
	}
	providerStatus, _ := diagnosis.Provider["status"].(string)
	if providerStatus == string(pe.Unknown) && (req.Action == recoveryActionResolveCapture || req.Action == recoveryActionResolveRelease) {
		outcome := strings.TrimSpace(fmt.Sprint(req.Evidence["providerOutcome"]))
		if req.Action == recoveryActionResolveCapture && outcome == "succeeded" {
			return nil
		}
		if req.Action == recoveryActionResolveRelease && (outcome == "not_submitted" || outcome == "definitely_failed") {
			return nil
		}
	}
	return fmt.Errorf("recovery action %s is blocked by current durable state", req.Action)
}

func (a api) markGenerationManualReview(task generationTask, req recoveryActionRequest) (generationTask, error) {
	result, err := a.updateGenerationRecoveryState(task.ID, "MANUAL_REVIEW", req)
	if err != nil {
		return generationTask{}, err
	}
	if isPPTGenerationType(task.Type) && a.pptService != nil {
		if _, err := a.pptService.SetDeckManualReview(task.UserID, task.ID); err != nil {
			return generationTask{}, err
		}
	}
	return result, nil
}

func (a api) updateGenerationRecoveryState(taskID, state string, req recoveryActionRequest) (generationTask, error) {
	pg := a.pgDB()
	if pg == nil {
		return generationTask{}, errors.New("recovery actions require PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := pg.BeginTx(ctx, nil)
	if err != nil {
		return generationTask{}, err
	}
	defer tx.Rollback()
	task, err := generationTaskForUpdate(ctx, tx, taskID)
	if err != nil {
		return generationTask{}, err
	}
	if task.Params == nil {
		task.Params = map[string]any{}
	}
	task.Params["recovery_state"] = state
	task.Params["recovery_reason"] = req.Reason
	task.Params["recovery_evidence"] = req.Evidence
	if state == "MANUAL_REVIEW" {
		task.Status, task.TaskStatus = "MANUAL_REVIEW", "MANUAL_REVIEW"
	}
	if err := insertGenerationTask(ctx, tx, task); err != nil {
		return generationTask{}, err
	}
	if err := tx.Commit(); err != nil {
		return generationTask{}, err
	}
	return task, nil
}

func (a api) redriveGenerationEvent(task generationTask, req recoveryActionRequest) (generationTask, error) {
	if a.pgDB() == nil {
		return generationTask{}, errors.New("redrive requires PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var out generationTask
	tx, err := a.pgDB().BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	var eventID string
	if err := tx.QueryRowContext(ctx, `SELECT event_id FROM outbox_events WHERE aggregate_id=$1 ORDER BY created_at DESC LIMIT 1 FOR UPDATE`, task.ID).Scan(&eventID); err != nil {
		return out, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE outbox_events SET status='pending',next_attempt_at=now(),published_at=NULL,claimed_at=NULL,claim_owner=NULL,last_error=NULL,updated_at=now() WHERE event_id=$1`, eventID); err != nil {
		return out, err
	}
	if err := tx.Commit(); err != nil {
		return out, err
	}
	return task, nil
}

func (a api) resolveGenerationCapture(task generationTask, req recoveryActionRequest) (generationTask, error) {
	if strings.TrimSpace(fmt.Sprint(req.Evidence["providerOutcome"])) != "succeeded" {
		return generationTask{}, errors.New("RESOLVE_CAPTURE requires evidence.providerOutcome=succeeded")
	}
	if db := a.pgDB(); db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if latest, err := pe.NewStore(db).GetLatestByTask(ctx, task.ID); err == nil && latest.Status == pe.Unknown {
			requestID := strings.TrimSpace(fmt.Sprint(req.Evidence["providerRequestId"]))
			if requestID == "" {
				return generationTask{}, errors.New("RESOLVE_CAPTURE requires evidence.providerRequestId for UNKNOWN")
			}
			class := string(pe.ProviderSucceeded)
			if err := pe.NewStore(db).Transition(ctx, latest.ID, pe.Succeeded, &requestID, &class, nil); err != nil {
				return generationTask{}, err
			}
		}
	}
	result, err := a.store.CompleteGenerationTask(task.ID, createGenerationTaskRequest{UserID: task.UserID, Type: task.Type, Prompt: task.Prompt, Model: task.Model, Params: cloneAnyMap(task.Params)})
	if err == nil && isPPTGenerationType(task.Type) && a.pptService != nil {
		_, err = a.pptService.SetDeckStatus(task.UserID, task.ID, pptapp.StatusSuccess)
	}
	return result, err
}

func (a api) resolveGenerationRelease(task generationTask, req recoveryActionRequest) (generationTask, error) {
	outcome := strings.TrimSpace(fmt.Sprint(req.Evidence["providerOutcome"]))
	if outcome != "not_submitted" && outcome != "definitely_failed" {
		return generationTask{}, errors.New("RESOLVE_RELEASE requires evidence.providerOutcome=not_submitted or definitely_failed")
	}
	if db := a.pgDB(); db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if latest, err := pe.NewStore(db).GetLatestByTask(ctx, task.ID); err == nil && latest.Status == pe.Unknown {
			class := string(pe.DefinitiveNotSubmitted)
			message := strings.TrimSpace(fmt.Sprint(req.Evidence["reference"]))
			if message == "" {
				message = req.Reason
			}
			if err := pe.NewStore(db).Transition(ctx, latest.ID, pe.Failed, nil, &class, &message); err != nil {
				return generationTask{}, err
			}
		}
	}
	result, err := a.store.FailGenerationTaskDurable(task.ID, req.Reason)
	if err == nil && isPPTGenerationType(task.Type) && a.pptService != nil {
		_, err = a.pptService.SetDeckStatus(task.UserID, task.ID, pptapp.StatusFailed)
	}
	return result, err
}

func insertRecoveryAudit(ctx context.Context, db *sql.DB, actorID, actorRole, taskID string, req recoveryActionRequest, before, after recoveryDiagnosisResponse) error {
	if db == nil {
		return nil
	}
	metadata := map[string]any{"reason": req.Reason, "evidence": req.Evidence, "before": before, "after": after}
	return insertAuditDirect(ctx, db, actorID, actorRole, "generation.recovery."+strings.ToLower(req.Action), "generation_task", taskID, "POST", "/api/v1/admin/generation-tasks/"+taskID+"/recovery-actions", http.StatusOK, metadata)
}

func recoveryWriteError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, sql.ErrNoRows) {
		status = http.StatusNotFound
	}
	recoveryWriteErrorStatus(w, status, err)
}
func recoveryWriteErrorStatus(w http.ResponseWriter, status int, err error) {
	writeError(w, status, err)
}
func validRecoveryAction(action string) bool {
	switch action {
	case recoveryActionDiagnose, recoveryActionRedrive, recoveryActionResolveRelease, recoveryActionResolveCapture, recoveryActionManualReview:
		return true
	}
	return false
}
func recoveryContainsString(items []string, wanted string) bool {
	for _, item := range items {
		if item == wanted {
			return true
		}
	}
	return false
}
func uniqueStrings(items []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			out = append(out, item)
		}
	}
	return out
}

var _ = sql.ErrNoRows
