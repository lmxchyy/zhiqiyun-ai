package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"xianzhi-ai/backend-go/internal/messaging"
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
	db      *sql.DB
	outbox  *messaging.OutboxStore
	options GenerationSchedulerOptions
	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
	doneCh  chan struct{}
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

func (s *GenerationScheduler) DispatchOnce(ctx context.Context) (int, error) {
	if s.db == nil {
		return 0, errors.New("scheduler database is nil")
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, min(created_at) as oldest_task_at
		FROM xz_generation_tasks
		WHERE upper(coalesce(nullif(task_status,''), status)) = 'QUEUED'
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

	totalDispatched := 0
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
		SELECT id, coalesce(type,''), coalesce(params,'{}'::jsonb)
		FROM xz_generation_tasks
		WHERE user_id=$1
		  AND upper(coalesce(nullif(task_status,''), status)) = 'QUEUED'
		ORDER BY created_at ASC
		LIMIT $2
		FOR UPDATE SKIP LOCKED
	`, userID, limit)
	if err != nil {
		return 0, err
	}
	defer taskRows.Close()

	type claimedTask struct {
		id       string
		taskType string
		params   string
	}
	var claimed []claimedTask
	for taskRows.Next() {
		var item claimedTask
		if scanErr := taskRows.Scan(&item.id, &item.taskType, &item.params); scanErr == nil {
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
	for _, item := range claimed {
		if _, updateErr := tx.ExecContext(ctx, `
			UPDATE xz_generation_tasks
			SET task_status = 'DISPATCHING',
			    updated_at = $2
			WHERE id = $1
		`, item.id, nowStr); updateErr != nil {
			return 0, updateErr
		}

		eventType := "x.ai.generation.image.canary.requested"
		eventPrefix := "generation.image.requested:"
		if isVideoGenerationRequest(item.taskType) {
			eventType = messaging.GenerationVideoCanaryRoutingKey
			eventPrefix = "generation.video.requested:"
		} else if strings.EqualFold(item.taskType, "PPT_GENERATION") || strings.EqualFold(item.taskType, "ppt") {
			eventType = messaging.GenerationPPTCanaryRoutingKey
			eventPrefix = "generation.ppt.requested:"
		}

		e := &messaging.Envelope{
			EventID:       eventPrefix + item.id,
			EventType:     eventType,
			Version:       1,
			OccurredAt:    now.Format(time.RFC3339),
			Producer:      s.options.Owner,
			AggregateType: "generation_task",
			AggregateID:   item.id,
			Data:          map[string]interface{}{"task_id": item.id},
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

func (s *GenerationScheduler) RecoverStaleDispatches(ctx context.Context) (int, error) {
	if s.db == nil {
		return 0, errors.New("scheduler database is nil")
	}
	threshold := time.Now().UTC().Add(-s.options.StaleDispatchTimeout)
	res, err := s.db.ExecContext(ctx, `
		UPDATE xz_generation_tasks
		SET task_status = 'QUEUED',
		    updated_at = $1
		WHERE upper(coalesce(nullif(task_status,''), status)) = 'DISPATCHING'
		  AND updated_at < $2
	`, time.Now().UTC().Format(time.RFC3339Nano), threshold.Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}
