package httpserver

// Execution-generation fencing (Issue #145, GO_IMPLEMENT).
//
// The task row guard before this change was row-lock serialization +
// terminal-first-wins only: any live-or-resurrected holder of a task id could
// commit final state, artifacts, and billing after its lease expired and the
// task had been requeued to a new owner.
//
// Fencing token: xz_generation_tasks.execution_generation (monotonic per
// task, starts at 1). Every ownership transfer -- scheduler dispatch, worker
// claim, requeue/redrive -- bumps it atomically and returns the new value to
// the owner. Settlement (Complete / Fail / Durable / UnknownGrace /
// MANUAL_REVIEW / Capture / Release) carries the presented generation and
// commits only when it still matches the stored one; otherwise the tx rolls
// back with ErrFencedStaleExecution before any asset or billing write.
//
// Owner/heartbeat protocol:
//   - worker id allocation: hostname-pid-random hex (allocateGenerationWorkerID)
//   - lease TTL: generationLeaseTTL (10m); heartbeat cadence:
//     generationHeartbeatInterval (2m); both well inside the image (12m) and
//     video (20m) provider windows because renewal is periodic.
//   - renewal SQL: UPDATE ... WHERE id AND worker_id AND generation
//     (0 rows = ownership lost, settle will fence).
//   - claim SQL: SELECT ... FOR UPDATE, bump generation+1, set owner/lease.
//   - release: terminal settlement keeps the owner for audit; requeue clears
//     owner/lease while bumping the generation.
//
// Cutover: rows with NULL lease_until use the legacy updated_at age backstop
// (mixed-version safe during rollout); rows carrying a lease are
// lease-authoritative. fencingCutoverSnapshot exposes the per-path counters.

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	providerexecution "xianzhi-ai/backend-go/internal/providerexecution"
)

const (
	// generationLeaseTTL bounds how long a claimed owner may go silent
	// before the reaper may requeue the task to a new generation.
	generationLeaseTTL = 10 * time.Minute
	// generationHeartbeatInterval is the renewal cadence for live workers.
	generationHeartbeatInterval = 2 * time.Minute
	// generationInitial is the fencing token for new/legacy tasks.
	generationInitial int64 = 1
)

// ErrFencedStaleExecution is returned when a settlement presents a task
// generation that no longer matches the stored one. The tx is rolled back
// with no state, artifact, or billing mutation. It aliases the provider
// package sentinel so execution- and task-layer fencing share one identity.
var ErrFencedStaleExecution = providerexecution.ErrFencedStaleExecution

// fencingCutoverCounters tracks the lease/age cutover per path. Read via
// fencingCutoverSnapshot (tests + operator logs), mutated atomically.
type fencingCutoverCounters struct {
	leaseSkips        atomic.Int64 // reaper skipped: valid lease held
	leaseExpired      atomic.Int64 // reaper eligible: lease present but expired
	ageBackstop       atomic.Int64 // reaper eligible: no lease, age backstop decided
	staleSettlements  atomic.Int64 // settlement txs fenced (stale generation)
	renewalConflicts  atomic.Int64 // heartbeat renewals that lost ownership
	staleRedeliveries atomic.Int64 // outbox/inbox redeliveries older than current gen
	providerGateSkips atomic.Int64 // succeeded executions ignored: bound gen != current
}

var fencingCutover = &fencingCutoverCounters{}

// fencingCutoverSnapshot returns a point-in-time copy of the cutover metrics.
// stale_redeliveries counts only envelope redeliveries older than the stored
// generation; provider_gate_skips (stale succeeded executions ignored at the
// recovery gate) is reported separately and never folded in.
func fencingCutoverSnapshot() map[string]int64 {
	return map[string]int64{
		"lease_skips":         fencingCutover.leaseSkips.Load(),
		"lease_expired":       fencingCutover.leaseExpired.Load(),
		"age_backstop":        fencingCutover.ageBackstop.Load(),
		"stale_settlements":   fencingCutover.staleSettlements.Load(),
		"renewal_conflicts":   fencingCutover.renewalConflicts.Load(),
		"stale_redeliveries":  fencingCutover.staleRedeliveries.Load(),
		"provider_gate_skips": fencingCutover.providerGateSkips.Load(),
	}
}

// normalizeTaskGeneration maps legacy/unread generations (<=0) to the
// initial fencing token. List/summary scans do not project fencing columns,
// so tasks read through those paths normalize to 1 until re-read under lock.
func normalizeTaskGeneration(value int64) int64 {
	if value <= 0 {
		return generationInitial
	}
	return value
}

// fencingGeneration returns the normalized fencing token of a task value.
func (task generationTask) fencingGeneration() int64 {
	return normalizeTaskGeneration(task.ExecutionGeneration)
}

// allocateGenerationWorkerID mints an owner id. Hostname grounds operator
// logs, pid separates co-located processes, randomness separates restarts
// that reuse (host, pid).
func allocateGenerationWorkerID() string {
	host, _ := os.Hostname()
	host = strings.TrimSpace(host)
	if host == "" {
		host = "unknown-host"
	}
	var randomness [8]byte
	_, _ = rand.Read(randomness[:])
	return fmt.Sprintf("%s-%d-%s", host, os.Getpid(), hex.EncodeToString(randomness[:]))
}

// taskFencing carries the ownership/lease projection of a task row.
type taskFencing struct {
	Generation      int64
	WorkerID        string
	LeaseUntil      *time.Time
	LastHeartbeatAt *time.Time
	Status          string
	UpdatedAt       string
	Found           bool
}

// scanTaskFencingRow scans a fencing projection row (nullable lease times).
func scanTaskFencingRow(scanner sqlRowScanner, fencing *taskFencing) error {
	var generation sql.NullInt64
	var workerID sql.NullString
	var leaseUntil, lastHeartbeat sql.NullTime
	if err := scanner.Scan(&generation, &workerID, &leaseUntil, &lastHeartbeat); err != nil {
		return err
	}
	if generation.Valid {
		fencing.Generation = generation.Int64
	}
	if workerID.Valid {
		fencing.WorkerID = workerID.String
	}
	if leaseUntil.Valid {
		value := leaseUntil.Time
		fencing.LeaseUntil = &value
	}
	if lastHeartbeat.Valid {
		value := lastHeartbeat.Time
		fencing.LastHeartbeatAt = &value
	}
	fencing.Generation = normalizeTaskGeneration(fencing.Generation)
	fencing.Found = true
	return nil
}

// getTaskFencing reads the fencing projection without locking (eligibility
// probes, watchers). Settlement paths must use the locked variant.
func getTaskFencing(ctx context.Context, db *sql.DB, taskID string) (taskFencing, error) {
	var fencing taskFencing
	err := scanTaskFencingRow(
		db.QueryRowContext(ctx, `SELECT execution_generation, worker_id, lease_until, last_heartbeat_at FROM xz_generation_tasks WHERE id=$1`, taskID),
		&fencing,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return taskFencing{}, sql.ErrNoRows
	}
	return fencing, err
}

// getTaskFencingForUpdate locks the task row and returns its fencing
// projection. Callers compare the presented generation before any write.
func getTaskFencingForUpdate(ctx context.Context, tx *sql.Tx, taskID string) (taskFencing, error) {
	var fencing taskFencing
	err := scanTaskFencingRow(
		tx.QueryRowContext(ctx, `SELECT execution_generation, worker_id, lease_until, last_heartbeat_at FROM xz_generation_tasks WHERE id=$1 FOR UPDATE`, taskID),
		&fencing,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return taskFencing{}, sql.ErrNoRows
	}
	return fencing, err
}

// assertTaskGenerationTx is the canonical settlement predicate: under the
// task row lock, the presented generation must still match the stored one,
// verified by value comparison plus a row-count assert
// (WHERE id AND execution_generation) before the caller performs any asset
// or billing write. expectedGen <= 0 means an unfenced legacy caller
// (compat) and skips the comparison. Mismatch rolls back via the returned
// error before the caller performs any asset or billing write.
func assertTaskGenerationTx(ctx context.Context, tx *sql.Tx, taskID string, expectedGen int64) (taskFencing, error) {
	fencing, err := getTaskFencingForUpdate(ctx, tx, taskID)
	if err != nil {
		return fencing, err
	}
	if expectedGen <= 0 {
		return fencing, nil
	}
	if fencing.Generation != expectedGen {
		fencingCutover.staleSettlements.Add(1)
		return fencing, fmt.Errorf("%w: task %s presented generation %d, stored generation %d",
			ErrFencedStaleExecution, taskID, expectedGen, fencing.Generation)
	}
	// Row-count arm of the predicate: the presenting generation must own
	// exactly one row before any settlement write follows.
	var owned int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM xz_generation_tasks WHERE id=$1 AND execution_generation=$2`, taskID, expectedGen).Scan(&owned); err != nil {
		return fencing, err
	}
	if owned != 1 {
		fencingCutover.staleSettlements.Add(1)
		return fencing, fmt.Errorf("%w: task %s ownership assert found %d rows for generation %d",
			ErrFencedStaleExecution, taskID, owned, expectedGen)
	}
	return fencing, nil
}

// assertTaskGenerationCommitted re-checks the predicate immediately before
// commit (row-count assert). Under the held row lock this always matches
// unless the tx itself moved the generation; it guards future refactors that
// might write between predicate and commit.
func assertTaskGenerationCommitted(ctx context.Context, tx *sql.Tx, taskID string, expectedGen int64) error {
	if expectedGen <= 0 {
		return nil
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM xz_generation_tasks WHERE id=$1 AND execution_generation=$2`, taskID, expectedGen).Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		fencingCutover.staleSettlements.Add(1)
		return fmt.Errorf("%w: task %s commit assert found %d rows for generation %d",
			ErrFencedStaleExecution, taskID, count, expectedGen)
	}
	return nil
}

// claimGenerationOwnershipTx bumps the generation and installs the owner
// lease under the task row lock. Terminal tasks are never claimable: bumping
// them would resurrect fencing state on settled rows.
func claimGenerationOwnershipTx(ctx context.Context, tx *sql.Tx, taskID, workerID string, ttl time.Duration) (taskFencing, error) {
	var fencing taskFencing
	if strings.TrimSpace(workerID) == "" {
		return fencing, fmt.Errorf("generation claim requires a worker id")
	}
	if ttl <= 0 {
		ttl = generationLeaseTTL
	}
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(status,'') FROM xz_generation_tasks WHERE id=$1 FOR UPDATE`, taskID).Scan(&status); err != nil {
		return fencing, err
	}
	if !isRunningGenerationTaskStatus(status) {
		return fencing, fmt.Errorf("generation task %s is terminal (%s)", taskID, status)
	}
	leaseSeconds := int64(ttl / time.Second)
	if leaseSeconds <= 0 {
		leaseSeconds = int64((generationLeaseTTL / time.Second))
	}
	row := tx.QueryRowContext(ctx, `
		UPDATE xz_generation_tasks
		SET execution_generation = execution_generation + 1,
		    worker_id = $2,
		    lease_until = now() + ($3 || ' seconds')::interval,
		    last_heartbeat_at = now(),
		    updated_at = $4
		WHERE id = $1
		RETURNING execution_generation, worker_id, lease_until, last_heartbeat_at
	`, taskID, workerID, fmt.Sprint(leaseSeconds), time.Now().UTC().Format(time.RFC3339Nano))
	var generation sql.NullInt64
	var worker sql.NullString
	var leaseUntil, lastHeartbeat sql.NullTime
	if err := row.Scan(&generation, &worker, &leaseUntil, &lastHeartbeat); err != nil {
		return fencing, err
	}
	fencing.Generation = normalizeTaskGeneration(generation.Int64)
	fencing.WorkerID = worker.String
	if leaseUntil.Valid {
		value := leaseUntil.Time
		fencing.LeaseUntil = &value
	}
	if lastHeartbeat.Valid {
		value := lastHeartbeat.Time
		fencing.LastHeartbeatAt = &value
	}
	fencing.Status = status
	fencing.Found = true
	return fencing, nil
}

// renewGenerationLeaseTx extends the owner lease. It is conditional on
// (worker_id, generation): 0 rows means ownership moved on and the caller
// must stop (its settlement will fence).
func renewGenerationLeaseTx(ctx context.Context, tx *sql.Tx, taskID, workerID string, expectedGen int64, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = generationLeaseTTL
	}
	leaseSeconds := int64(ttl / time.Second)
	if leaseSeconds <= 0 {
		leaseSeconds = int64((generationLeaseTTL / time.Second))
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE xz_generation_tasks
		SET lease_until = now() + ($4 || ' seconds')::interval,
		    last_heartbeat_at = now()
		WHERE id = $1 AND worker_id = $2 AND execution_generation = $3
	`, taskID, workerID, expectedGen, fmt.Sprint(leaseSeconds))
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		fencingCutover.renewalConflicts.Add(1)
		return fmt.Errorf("%w: task %s lease renewal for worker %s generation %d affected %d rows",
			ErrFencedStaleExecution, taskID, workerID, expectedGen, affected)
	}
	return nil
}

// requeueGenerationTx performs the generation-atomic requeue: exactly one
// reaper wins the conditional bump (WHERE id AND generation); losers observe
// 0 rows and abort. Owner/lease are cleared so the next claim starts clean.
// An empty toStatus preserves the current task_status (operator redrive);
// reapers pass an explicit target such as QUEUED.
func requeueGenerationTx(ctx context.Context, tx *sql.Tx, taskID string, expectedGen int64, toStatus string) (int64, error) {
	var nextGen sql.NullInt64
	err := tx.QueryRowContext(ctx, `
		UPDATE xz_generation_tasks
		SET execution_generation = execution_generation + 1,
		    worker_id = NULL,
		    lease_until = NULL,
		    last_heartbeat_at = NULL,
		    task_status = coalesce(nullif($3,''), task_status),
		    updated_at = $4
		WHERE id = $1 AND execution_generation = $2
		RETURNING execution_generation
	`, taskID, expectedGen, toStatus, time.Now().UTC().Format(time.RFC3339Nano)).Scan(&nextGen)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("%w: task %s requeue for generation %d lost the race",
			ErrFencedStaleExecution, taskID, expectedGen)
	}
	if err != nil {
		return 0, err
	}
	return normalizeTaskGeneration(nextGen.Int64), nil
}

// reaperEligibleForFencing decides whether the reaper may act on the observed
// fencing state. Lease-carrying rows are lease-authoritative against DB
// now(); rows without a lease fall back to the updated_at age backstop so
// mixed-version writers stay reapable during rollout.
func reaperEligibleForFencing(fencing taskFencing, updatedAt time.Time, now time.Time, maxAge time.Duration) bool {
	if fencing.Found && fencing.LeaseUntil != nil {
		if fencing.LeaseUntil.After(now) {
			fencingCutover.leaseSkips.Add(1)
			return false
		}
		fencingCutover.leaseExpired.Add(1)
		return true
	}
	fencingCutover.ageBackstop.Add(1)
	return now.Sub(updatedAt.UTC()) >= maxAge
}

// fencingPostgres returns the postgres store when fencing SQL is available,
// nil for legacy/memory stores (which keep unfenced compat behavior).
func fencingPostgres(store platformStore) *postgresStore {
	if pg, ok := store.(*postgresStore); ok && pg != nil && pg.db != nil {
		return pg
	}
	return nil
}

// observeGenerationTaskGeneration returns the current stored generation, or
// 0 when unavailable (legacy path: settlement stays unfenced).
func observeGenerationTaskGeneration(store platformStore, taskID string) int64 {
	pg := fencingPostgres(store)
	if pg == nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	fencing, err := getTaskFencing(ctx, pg.db, strings.TrimSpace(taskID))
	if err != nil || !fencing.Found {
		return 0
	}
	return fencing.Generation
}

// claimGenerationTaskOwnership claims (bumps + leases) a task for the
// calling worker. Returns (0, "", nil) for non-postgres stores (compat).
func claimGenerationTaskOwnership(store platformStore, taskID string) (int64, string, error) {
	pg := fencingPostgres(store)
	if pg == nil {
		return 0, "", nil
	}
	workerID := allocateGenerationWorkerID()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := pg.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = tx.Rollback() }()
	fencing, err := claimGenerationOwnershipTx(ctx, tx, strings.TrimSpace(taskID), workerID, generationLeaseTTL)
	if err != nil {
		return 0, "", err
	}
	if err := tx.Commit(); err != nil {
		return 0, "", err
	}
	return fencing.Generation, workerID, nil
}

// renewGenerationLease extends a live worker's lease (heartbeat).
func renewGenerationLease(store platformStore, taskID, workerID string, expectedGen int64) error {
	pg := fencingPostgres(store)
	if pg == nil || strings.TrimSpace(workerID) == "" || expectedGen <= 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := pg.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := renewGenerationLeaseTx(ctx, tx, strings.TrimSpace(taskID), workerID, expectedGen, generationLeaseTTL); err != nil {
		return err
	}
	return tx.Commit()
}

// completeGenerationTaskWithFencing routes settlement through the fenced
// store path when available, else the legacy path (gen ignored).
func completeGenerationTaskWithFencing(store platformStore, taskID string, req createGenerationTaskRequest, expectedGen int64) (generationTask, error) {
	if pg := fencingPostgres(store); pg != nil {
		return pg.CompleteGenerationTaskFenced(taskID, req, expectedGen)
	}
	return store.CompleteGenerationTask(taskID, req)
}

// failGenerationTaskWithFencing routes failure settlement through fencing.
func failGenerationTaskWithFencing(store platformStore, taskID, message string, expectedGen int64) (generationTask, error) {
	if pg := fencingPostgres(store); pg != nil {
		return pg.FailGenerationTaskFenced(taskID, message, expectedGen)
	}
	return store.FailGenerationTask(taskID, message)
}

// failGenerationTaskDurableWithFencing routes durable failure through fencing.
func failGenerationTaskDurableWithFencing(store platformStore, taskID, message string, expectedGen int64) (generationTask, error) {
	if pg := fencingPostgres(store); pg != nil {
		return pg.FailGenerationTaskDurableFenced(taskID, message, expectedGen)
	}
	return store.FailGenerationTaskDurable(taskID, message)
}

// failGenerationTaskUnknownGraceWithFencing routes unknown-grace failure.
func failGenerationTaskUnknownGraceWithFencing(store platformStore, taskID, message string, grace time.Duration, expectedGen int64) (generationTask, error) {
	if pg := fencingPostgres(store); pg != nil {
		return pg.FailGenerationTaskUnknownGraceFenced(taskID, message, grace, expectedGen)
	}
	return store.FailGenerationTaskUnknownGrace(taskID, message, grace)
}
