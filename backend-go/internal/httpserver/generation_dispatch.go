package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	"xianzhi-ai/backend-go/internal/messaging"
)

const imageDispatchModeParam = "generation_dispatch_mode"
const imageFairScheduledParam = "_generation_fair_scheduled"
const imageDispatchNormal = "normal"
const imageDispatchCanary = "canary"

var errGenerationOwnershipBusy = errors.New("generation ownership is active")

func imageDispatchMode(params map[string]any) (string, error) {
	mode := stringValue(params[imageDispatchModeParam])
	if mode == "" { // persisted pre-protocol tasks retain their real legacy mode
		if canaryTaskMarker(params) {
			return imageDispatchCanary, nil
		}
		return imageDispatchNormal, nil
	}
	if mode != imageDispatchNormal && mode != imageDispatchCanary {
		return "", fmt.Errorf("invalid image dispatch mode %q", mode)
	}
	if (mode == imageDispatchCanary) != canaryTaskMarker(params) {
		return "", errors.New("image dispatch mode and canary admission disagree")
	}
	return mode, nil
}
func imageDispatchRouting(params map[string]any) (string, string, error) {
	mode, err := imageDispatchMode(params)
	if err != nil {
		return "", "", err
	}
	if mode == imageDispatchCanary {
		return messaging.GenerationCanaryRoutingKey, mode, nil
	}
	return messaging.GenerationImageNormalRoutingKey, mode, nil
}
func validateImageDispatchEnvelope(e *messaging.Envelope, mode string) error {
	if e == nil || e.AggregateType != "generation_task" || e.AggregateID == "" {
		return errors.New("invalid image dispatch envelope")
	}
	route := messaging.GenerationCanaryRoutingKey
	if mode == imageDispatchNormal {
		route = messaging.GenerationImageNormalRoutingKey
	}
	if e.EventType != route {
		return errors.New("image dispatch route mismatch")
	}
	value, present := e.Data["dispatch_mode"]
	declared, valid := value.(string)
	if present && !valid {
		return errors.New("image dispatch mode must be a string")
	}
	if id, present := e.Data["task_id"]; present {
		if text, ok := id.(string); !ok || text != e.AggregateID {
			return errors.New("image dispatch task identity mismatch")
		}
	}
	if declared != "" && declared != mode {
		return errors.New("image dispatch mode mismatch")
	}
	if mode == imageDispatchNormal && (declared != mode || envelopeExecutionGeneration(e.Data) <= 0) {
		return errors.New("normal dispatch requires mode and generation")
	}
	return nil
}
func validateGenerationClaim(state, worker, dispatchOwner string, generation, expectedDispatch int64, live, image bool) error {
	if expectedDispatch > generation {
		return fmt.Errorf("%w: future dispatch generation", ErrFencedStaleExecution)
	}
	if state == "DISPATCHING" {
		legacyScheduler := worker == "generation-fair-scheduler" || worker == "generation-worker-scheduler"
		if image && (expectedDispatch <= 0 || expectedDispatch != generation || (dispatchOwner != worker && !(dispatchOwner == "" && legacyScheduler))) {
			return fmt.Errorf("%w: dispatch handoff identity mismatch", ErrFencedStaleExecution)
		}
		if live && worker != "" && dispatchOwner != worker && !legacyScheduler {
			return errGenerationOwnershipBusy
		}
		return nil
	}
	if live && worker != "" {
		return errGenerationOwnershipBusy
	}
	if state == "QUEUED" && expectedDispatch > 0 && expectedDispatch != generation {
		return fmt.Errorf("%w: stale queued dispatch", ErrFencedStaleExecution)
	}
	if !strings.EqualFold(state, "RUNNING") && !strings.EqualFold(state, "QUEUED") && state != "" && !strings.EqualFold(state, "PROCESSING") && !strings.EqualFold(state, "PENDING") {
		return fmt.Errorf("%w: task is not claimable (%s)", ErrFencedStaleExecution, state)
	}
	return nil
}

// On intentional defer the original message remains retryable. Relinquish only
// this caller's lease, never bump/rebind the durable operation or settle it.
func relinquishGenerationOwnership(store platformStore, taskID, worker string, generation int64) {
	pg := fencingPostgres(store)
	if pg == nil || worker == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := pg.db.ExecContext(ctx, `UPDATE xz_generation_tasks SET lease_until=now(),worker_id=NULL WHERE id=$1 AND execution_generation=$2 AND worker_id=$3`, taskID, generation, worker); err != nil {
		log.Printf("generation lease relinquish failed task_id=%s generation=%d owner=%s error=%q", taskID, generation, worker, err)
	}
}

// The caller holds the task row lock. A stale watchdog observation is not an
// execution claim: generation, owner, lease and heartbeat must still match,
// and no live owner may be reaped, even within the same generation.
func assertGenerationReaperTx(ctx context.Context, tx *sql.Tx, taskID string, observed taskFencing) error {
	if !observed.Found || observed.Generation <= 0 {
		return fmt.Errorf("%w: reaper has no ownership observation", ErrFencedStaleExecution)
	}
	var matches bool
	err := tx.QueryRowContext(ctx, `SELECT execution_generation=$2 AND coalesce(worker_id,'')=$3
	 AND lease_until IS NOT DISTINCT FROM $4::timestamptz
	 AND last_heartbeat_at IS NOT DISTINCT FROM $5::timestamptz
	 AND (lease_until IS NULL OR lease_until<=now())
	 AND upper(status) IN ('PENDING','QUEUED','PROCESSING','RUNNING')
	 AND upper(coalesce(nullif(task_status,''),status)) IN ('PENDING','QUEUED','PROCESSING','RUNNING','DISPATCHING')
	 FROM xz_generation_tasks WHERE id=$1`, taskID, observed.Generation, observed.WorkerID, observed.LeaseUntil, observed.LastHeartbeatAt).Scan(&matches)
	if err != nil {
		return err
	}
	if !matches {
		return fmt.Errorf("%w: reaper ownership changed task %s", ErrFencedStaleExecution, taskID)
	}
	return nil
}

func claimImageRecoveryOwnership(store platformStore, taskID string, observed taskFencing) (int64, string, error) {
	pg := fencingPostgres(store)
	if pg == nil {
		return 0, "", nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := pg.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, "", err
	}
	defer tx.Rollback()
	if _, err = assertTaskGenerationTx(ctx, tx, taskID, observed.Generation); err != nil {
		return 0, "", err
	}
	if err = assertGenerationReaperTx(ctx, tx, taskID, observed); err != nil {
		return 0, "", err
	}
	owner := allocateGenerationWorkerID()
	fencing, err := claimGenerationDispatchOwnershipTx(ctx, tx, taskID, owner, generationLeaseTTL, observed.Generation)
	if err != nil {
		return 0, "", err
	}
	if err = tx.Commit(); err != nil {
		return 0, "", err
	}
	return fencing.Generation, owner, nil
}

func failGenerationTaskForWatchdog(store platformStore, task generationTask, message string, grace *time.Duration, observed taskFencing) (generationTask, error) {
	if grace != nil && *grace <= 0 {
		return generationTask{}, fmt.Errorf("unknown grace must be positive")
	}
	if pg := fencingPostgres(store); pg != nil && isImageGenerationRequest(task.Type) {
		return pg.failGenerationTaskDurableChecked(task.ID, message, grace, observed.Generation, "", &observed)
	}
	if grace != nil {
		return failGenerationTaskUnknownGraceWithFencing(store, task.ID, message, *grace, observed.Generation)
	}
	return failGenerationTaskDurableWithFencing(store, task.ID, message, observed.Generation)
}
