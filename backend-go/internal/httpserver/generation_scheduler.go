package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"xianzhi-ai/backend-go/internal/config"
	"xianzhi-ai/backend-go/internal/messaging"
	"xianzhi-ai/backend-go/internal/providerexecution"
)

type GenerationSchedulerOptions struct {
	PollInterval         time.Duration
	BatchUsers           int
	BatchTasksPerUser    int
	StaleDispatchTimeout time.Duration
	Owner                string
}

func DefaultGenerationSchedulerOptions() GenerationSchedulerOptions {
	return GenerationSchedulerOptions{
		PollInterval:         500 * time.Millisecond,
		BatchUsers:           50,
		BatchTasksPerUser:    10,
		StaleDispatchTimeout: 60 * time.Second,
		Owner:                "generation-fair-scheduler",
	}
}

type GenerationScheduler struct {
	db              *sql.DB
	outbox          *messaging.OutboxStore
	options         GenerationSchedulerOptions
	mu              sync.Mutex
	running         bool
	stopCh          chan struct{}
	doneCh          chan struct{}
	lastRecoveredID string
}

func NewGenerationScheduler(db *sql.DB, options ...GenerationSchedulerOptions) *GenerationScheduler {
	opts := DefaultGenerationSchedulerOptions()
	if len(options) > 0 {
		if options[0].PollInterval > 0 {
			opts.PollInterval = options[0].PollInterval
		}
		if options[0].BatchUsers > 0 {
			opts.BatchUsers = options[0].BatchUsers
		}
		if options[0].BatchTasksPerUser > 0 {
			opts.BatchTasksPerUser = options[0].BatchTasksPerUser
		}
		if options[0].StaleDispatchTimeout > 0 {
			opts.StaleDispatchTimeout = options[0].StaleDispatchTimeout
		}
		if options[0].Owner != "" {
			opts.Owner = options[0].Owner
		}
	}
	var outbox *messaging.OutboxStore
	if db != nil {
		outbox = messaging.NewOutboxStore(db)
	}
	return &GenerationScheduler{
		db:      db,
		outbox:  outbox,
		options: opts,
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
}

func (s *GenerationScheduler) Run(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.New("scheduler is already running")
	}
	s.running = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		close(s.doneCh)
	}()

	ticker := time.NewTicker(s.options.PollInterval)
	defer ticker.Stop()

	staleInterval := s.options.StaleDispatchTimeout / 2
	if staleInterval <= 0 {
		staleInterval = 30 * time.Second
	}
	staleTicker := time.NewTicker(staleInterval)
	defer staleTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.stopCh:
			return nil
		case <-staleTicker.C:
			if _, err := s.RecoverStaleDispatches(ctx); err != nil && ctx.Err() == nil {
				log.Printf("scheduler stale recovery error: %v", err)
			}
		case <-ticker.C:
			if _, err := s.DispatchOnce(ctx); err != nil && ctx.Err() == nil {
				log.Printf("scheduler dispatch tick error: %v", err)
			}
		}
	}
}

func (s *GenerationScheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	close(s.stopCh)
	<-s.doneCh
}

func (s *GenerationScheduler) DispatchOnce(ctx context.Context) (totalDispatched int, finalErr error) {
	defer func() {
		schedulerMetrics.dispatched.Add(uint64(totalDispatched))
		if finalErr != nil {
			schedulerMetrics.errors.Add(1)
		}
	}()
	if s.db == nil {
		return 0, errors.New("scheduler database is nil")
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, min(created_at) as oldest_task_at
		FROM xz_generation_tasks
		WHERE upper(coalesce(nullif(task_status,''), status)) = 'QUEUED'
		  AND upper(coalesce(type, '')) NOT IN ('PPT_GENERATION', 'PPT')
		GROUP BY user_id
		ORDER BY oldest_task_at ASC
		LIMIT $1
	`, s.options.BatchUsers)
	if err != nil {
		return 0, fmt.Errorf("query queued users: %w", err)
	}
	defer rows.Close()

	var candidateUserIDs []string
	for rows.Next() {
		var uid, oldest string
		if scanErr := rows.Scan(&uid, &oldest); scanErr == nil && strings.TrimSpace(uid) != "" {
			candidateUserIDs = append(candidateUserIDs, strings.TrimSpace(uid))
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	totalDispatched = 0
	now := time.Now().UTC()

	for _, userID := range candidateUserIDs {
		if ctx.Err() != nil {
			break
		}
		dispatched, userErr := s.dispatchUserTx(ctx, userID, now)
		if userErr != nil {
			log.Printf("scheduler dispatch failed for user=%s: %v", userID, userErr)
			continue
		}
		totalDispatched += dispatched
	}

	return totalDispatched, nil
}

func (s *GenerationScheduler) dispatchUserTx(ctx context.Context, userID string, now time.Time) (int, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "generation-concurrency:"+userID); err != nil {
		return 0, err
	}

	var planID string
	var configuredConcurrency int
	var expiresAt string
	if err := tx.QueryRowContext(ctx, `
		SELECT coalesce(user_account.plan_id,''), coalesce(plan.concurrency,-1), coalesce(user_account.subscription_expires_at,'')
		FROM xz_users user_account
		LEFT JOIN xz_plans plan ON plan.id=user_account.plan_id
		WHERE user_account.id=$1
	`, userID).Scan(&planID, &configuredConcurrency, &expiresAt); err != nil {
		return 0, err
	}

	profile := resolveUserConcurrencyProfile(planID, configuredConcurrency, expiresAt, now)

	var running int
	if err := tx.QueryRowContext(ctx, `
		SELECT count(*)
		FROM xz_generation_tasks
		WHERE user_id=$1
		  AND upper(coalesce(nullif(task_status,''), status)) IN ('DISPATCHING','RUNNING','PROCESSING')
	`, userID).Scan(&running); err != nil {
		return 0, err
	}

	available := profile.Concurrency - running
	if profile.Concurrency == 0 {
		available = defaultEnterpriseCap - running
	}
	if available <= 0 {
		return 0, nil
	}

	limit := available
	if limit > s.options.BatchTasksPerUser {
		limit = s.options.BatchTasksPerUser
	}

	taskRows, err := tx.QueryContext(ctx, `
		SELECT id, coalesce(type,''), coalesce(params,'{}'::jsonb),execution_generation
		FROM xz_generation_tasks t
		WHERE user_id=$1
		  AND upper(coalesce(nullif(task_status,''), status)) = 'QUEUED'
		  AND upper(status) IN ('PENDING','PROCESSING','RUNNING','QUEUED')
		  AND execution_generation > 0
		  AND (lease_until IS NULL OR lease_until <= now())
		  AND (coalesce(worker_id,'')='' OR lease_until <= now())
		  AND (upper(coalesce(type,'')) NOT IN ('TEXT_TO_IMAGE','IMAGE_TO_IMAGE') OR NOT EXISTS (SELECT 1 FROM provider_executions pe WHERE pe.task_id=t.id) OR (params->>'_generation_reconcile_only'='true' AND EXISTS (SELECT 1 FROM provider_executions pe WHERE pe.task_id=t.id AND (pe.task_execution_generation IS NULL OR pe.task_execution_generation=t.execution_generation) AND (pe.status IN ('prepared','succeeded') OR (pe.status IN ('unknown','submitting','submitted','processing') AND pe.provider_request_id IS NOT NULL)))))
		  AND upper(coalesce(type, '')) NOT IN ('PPT_GENERATION', 'PPT')
		ORDER BY created_at ASC
		LIMIT $2
		FOR UPDATE SKIP LOCKED
	`, userID, limit)
	if err != nil {
		return 0, err
	}
	defer taskRows.Close()

	type claimedTask struct {
		id         string
		taskType   string
		params     string
		generation int64
	}
	var claimed []claimedTask
	for taskRows.Next() {
		var item claimedTask
		if scanErr := taskRows.Scan(&item.id, &item.taskType, &item.params, &item.generation); scanErr == nil {
			claimed = append(claimed, item)
		}
	}
	if err := taskRows.Err(); err != nil {
		return 0, err
	}
	if len(claimed) == 0 {
		return 0, nil
	}

	nowStr := now.Format(time.RFC3339Nano)
	dispatchLeaseSeconds := int64(s.options.StaleDispatchTimeout / time.Second)
	if dispatchLeaseSeconds <= 0 {
		dispatchLeaseSeconds = 60
	}
	for _, item := range claimed {
		var params map[string]any
		if err := json.Unmarshal([]byte(item.params), &params); err != nil {
			return 0, err
		}
		if params == nil {
			params = map[string]any{}
		}
		eventType := messaging.GenerationCanaryRoutingKey
		mode := ""
		if isImageGenerationRequest(item.taskType) {
			var err error
			eventType, mode, err = imageDispatchRouting(params)
			if err != nil {
				return 0, err
			}
			params[imageDispatchModeParam] = mode
		}
		params["_generation_dispatch_owner"] = s.options.Owner
		encodedParams, err := json.Marshal(params)
		if err != nil {
			return 0, err
		}
		bump := int64(1)
		if isImageGenerationRequest(item.taskType) && boolValue(params["_generation_reconcile_only"]) {
			bump = 0
		}
		// Fresh dispatch advances generation and installs the lease atomically.
		// GET-only reconciliation retains its bound operation generation. The new
		// generation travels in the outbox envelope as the execution
		// identity; stale redeliveries observe the mismatch and skip work.
		var dispatchedGen int64
		if err := tx.QueryRowContext(ctx, `
			UPDATE xz_generation_tasks
			SET task_status = 'DISPATCHING',
			    execution_generation = execution_generation + $6,
			    params = $7::jsonb,
			    worker_id = $3,
			    lease_until = now() + ($4 || ' seconds')::interval,
			    last_heartbeat_at = now(),
			    updated_at = $2
			WHERE id = $1 AND execution_generation=$5 AND upper(coalesce(nullif(task_status,''),status))='QUEUED'
			  AND upper(status) IN ('PENDING','PROCESSING','RUNNING','QUEUED')
			  AND (lease_until IS NULL OR lease_until <= now())
			  AND (coalesce(worker_id,'')='' OR lease_until <= now())
			RETURNING execution_generation
		`, item.id, nowStr, s.options.Owner, fmt.Sprint(dispatchLeaseSeconds), item.generation, bump, string(encodedParams)).Scan(&dispatchedGen); err != nil {
			return 0, err
		}
		if dispatchedGen <= 0 {
			dispatchedGen = 1
		}

		if isVideoGenerationRequest(item.taskType) {
			eventType = messaging.GenerationVideoCanaryRoutingKey
		} else if strings.EqualFold(item.taskType, "PPT_GENERATION") || strings.EqualFold(item.taskType, "ppt") {
			eventType = messaging.GenerationPPTCanaryRoutingKey
		}

		eventID := fmt.Sprintf("generation.dispatched:%s:%d", item.id, dispatchedGen)
		if bump == 0 {
			eventID = fmt.Sprintf("generation.reconcile:%s:%d:%d", item.id, dispatchedGen, intValue(params["_generation_reconcile_sequence"]))
		}

		e := &messaging.Envelope{
			EventID:       eventID,
			EventType:     eventType,
			Version:       1,
			OccurredAt:    now.Format(time.RFC3339),
			Producer:      s.options.Owner,
			AggregateType: "generation_task",
			AggregateID:   item.id,
			// Execution identity threading: consumers compare the
			// envelope generation against the stored one and skip
			// stale redeliveries without provider work.
			Data: map[string]interface{}{"task_id": item.id, "execution_generation": dispatchedGen, "dispatch_mode": mode},
		}

		if s.outbox != nil {
			if insertErr := s.outbox.InsertTx(ctx, tx, e, "generation_task", item.id, ""); insertErr != nil {
				return 0, insertErr
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return len(claimed), nil
}

func (s *GenerationScheduler) RecoverStaleDispatches(ctx context.Context) (recovered int, finalErr error) {
	defer func() {
		schedulerMetrics.recovered.Add(uint64(recovered))
		if finalErr != nil {
			schedulerMetrics.errors.Add(1)
		}
	}()
	if s.db == nil {
		return 0, errors.New("scheduler database is nil")
	}

	threshold := time.Now().UTC().Add(-s.options.StaleDispatchTimeout)
	thresholdStr := threshold.Format(time.RFC3339Nano)
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)

	s.mu.Lock()
	cursor := s.lastRecoveredID
	s.mu.Unlock()

	batchSize := s.options.BatchUsers
	if batchSize <= 0 {
		batchSize = 50
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	// SQL eligibility prefilter (Issue #147): only candidate virgin dispatches or
	// due provider-reconciliation tasks are selected. A due execution with a
	// request id is GET-only recovery work; it must be allowed back through the
	// queue even when its original outbox event was already published.
	// Keyset cursor ensures forward progress across large datasets.
	query := `
		SELECT t.id, t.execution_generation, coalesce(o.event_id, ''), EXISTS (
		SELECT 1 FROM provider_executions reconcile_pe
		WHERE reconcile_pe.task_id = t.id
		  AND ((reconcile_pe.status IN ('unknown','submitting','submitted','processing') AND reconcile_pe.provider_request_id IS NOT NULL)
		    OR (upper(t.type) IN ('TEXT_TO_IMAGE','IMAGE_TO_IMAGE') AND reconcile_pe.status IN ('prepared','succeeded')))
		  AND (reconcile_pe.next_check_at IS NULL OR reconcile_pe.next_check_at <= now())
	) AS provider_reconcile
		FROM xz_generation_tasks t
		LEFT JOIN LATERAL (SELECT * FROM outbox_events oe WHERE oe.aggregate_id=t.id AND oe.aggregate_type='generation_task' ORDER BY oe.created_at DESC,oe.event_id DESC LIMIT 1) o ON true
		WHERE (upper(coalesce(nullif(t.task_status,''), t.status)) = 'DISPATCHING' OR (upper(t.task_status)='RUNNING' AND upper(t.type) IN ('TEXT_TO_IMAGE','IMAGE_TO_IMAGE') AND EXISTS (SELECT 1 FROM provider_executions r WHERE r.task_id=t.id AND (r.status IN ('prepared','succeeded') OR (r.status IN ('unknown','submitting','submitted','processing') AND r.provider_request_id IS NOT NULL)))))
		  AND (upper(coalesce(t.type,'')) NOT IN ('TEXT_TO_IMAGE','IMAGE_TO_IMAGE') OR NOT EXISTS (SELECT 1 FROM provider_executions bound_pe WHERE bound_pe.task_id=t.id AND bound_pe.task_execution_generation IS NOT NULL AND bound_pe.task_execution_generation<>t.execution_generation AND bound_pe.attempt=(SELECT max(latest_pe.attempt) FROM provider_executions latest_pe WHERE latest_pe.task_id=t.id)))
		  AND ((t.lease_until IS NULL AND t.updated_at < $1) OR (t.lease_until IS NOT NULL AND t.lease_until < now()))
		  AND (
		    (o.id IS NOT NULL
		     AND o.status = 'pending'
		     AND o.attempt_count = 0
		     AND o.claimed_at IS NULL
		     AND o.claim_owner IS NULL
		     AND o.published_at IS NULL
		     AND NOT EXISTS (SELECT 1 FROM provider_executions pe WHERE pe.task_id = t.id)
		     AND NOT EXISTS (SELECT 1 FROM consumer_inbox ci WHERE ci.event_id = o.event_id)
		     AND NOT EXISTS (SELECT 1 FROM outbox_events other WHERE other.aggregate_id = t.id AND other.event_id <> o.event_id)
		    )
		    OR
		    (o.id IS NULL
		     AND NOT EXISTS (SELECT 1 FROM outbox_events oe WHERE oe.aggregate_id = t.id)
		     AND NOT EXISTS (SELECT 1 FROM provider_executions pe WHERE pe.task_id = t.id)
		    )
		    OR
		    EXISTS (
		      SELECT 1 FROM provider_executions pe
		      WHERE pe.task_id = t.id
		        AND ((pe.status IN ('unknown','submitting','submitted','processing') AND pe.provider_request_id IS NOT NULL)
		          OR (upper(t.type) IN ('TEXT_TO_IMAGE','IMAGE_TO_IMAGE') AND pe.status IN ('prepared','succeeded')))
		        AND (pe.next_check_at IS NULL OR pe.next_check_at <= now())
		    )
		  )
		  AND ($2 = '' OR t.id > $2)
		ORDER BY t.id ASC
		LIMIT $3
		FOR UPDATE OF t SKIP LOCKED
	`

	rows, err := tx.QueryContext(ctx, query, thresholdStr, cursor, batchSize)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type recoverableTask struct {
		id                string
		gen               int64
		eventID           string
		providerReconcile bool
	}
	var candidates []recoverableTask
	var lastID string
	for rows.Next() {
		var item recoverableTask
		if err := rows.Scan(&item.id, &item.gen, &item.eventID, &item.providerReconcile); err != nil {
			return 0, err
		}
		candidates = append(candidates, item)
		lastID = item.id
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	rows.Close()

	s.mu.Lock()
	if len(candidates) < batchSize {
		s.lastRecoveredID = ""
	} else {
		s.lastRecoveredID = lastID
	}
	s.mu.Unlock()

	if len(candidates) == 0 {
		return 0, nil
	}

	recovered = 0
	for _, item := range candidates {
		if err := providerexecution.RejectTask(ctx, tx, item.id, "scheduler_recovery"); err != nil {
			if errors.Is(err, providerexecution.ErrQuarantined) {
				continue
			}
			return 0, err
		}
		if item.eventID != "" {
			res, delErr := tx.ExecContext(ctx, `
				DELETE FROM outbox_events
				WHERE event_id = $1
				  AND aggregate_id = $2
				  AND status = 'pending'
				  AND attempt_count = 0
				  AND claimed_at IS NULL
				  AND claim_owner IS NULL
				  AND published_at IS NULL
			`, item.eventID, item.id)
			if delErr != nil {
				return 0, delErr
			}
			n, _ := res.RowsAffected()
			if n != 1 && !item.providerReconcile {
				continue
			}
		}

		// Issue #145 fencing: bump generation on requeue to fence deposed attempts
		updRes, updErr := tx.ExecContext(ctx, `
			UPDATE xz_generation_tasks
			SET task_status = 'QUEUED',
			    execution_generation = execution_generation + CASE WHEN $4 AND upper(type) IN ('TEXT_TO_IMAGE','IMAGE_TO_IMAGE') THEN 0 ELSE 1 END,
			    params = CASE WHEN $4 AND upper(type) IN ('TEXT_TO_IMAGE','IMAGE_TO_IMAGE') THEN coalesce(params,'{}'::jsonb)||jsonb_build_object('_generation_reconcile_only',true,'_generation_reconcile_sequence',coalesce((params->>'_generation_reconcile_sequence')::bigint,0)+1) ELSE params END,
			    worker_id = NULL,
			    lease_until = NULL,
			    last_heartbeat_at = NULL,
			    updated_at = $1
			WHERE id = $2 AND execution_generation = $3
		`, nowStr, item.id, item.gen, item.providerReconcile)
		if updErr != nil {
			return 0, updErr
		}
		n, _ := updRes.RowsAffected()
		if n == 1 {
			recovered++
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return recovered, nil
}

// RunConfiguredGenerationScheduler reads an immutable startup configuration.
// Disabled processes keep consumers/publishers alive but never admit queue work.
func RunConfiguredGenerationScheduler(ctx context.Context, db *sql.DB, cfg config.Config, options ...GenerationSchedulerOptions) error {
	if !cfg.GenerationFairSchedulerEnabled {
		<-ctx.Done()
		return ctx.Err()
	}
	return NewGenerationScheduler(db, options...).Run(ctx)
}
