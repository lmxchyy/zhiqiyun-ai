package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	errGenerationConcurrencyLimit   = errors.New("generation concurrency limit reached")
	errGenerationQueueLimitExceeded = fmt.Errorf("%w: queue backlog limit reached", errGenerationConcurrencyLimit)
)

const (
	defaultFreeConcurrency      = 1
	defaultFreeQueueBacklog     = 10
	defaultBasicConcurrency     = 3
	defaultBasicQueueBacklog    = 30
	defaultProConcurrency       = 8
	defaultProQueueBacklog      = 50
	defaultUltimateConcurrency  = 20
	defaultUltimateQueueBacklog = 100
	defaultEnterpriseBacklog    = 200
	defaultEnterpriseCap        = 20
)

type userConcurrencyProfile struct {
	PlanID      string
	Concurrency int
	MaxQueued   int
	Expired     bool
}

type concurrencyAdmissionDecision struct {
	Profile      userConcurrencyProfile
	RunningCount int
	QueuedCount  int
	CanDispatch  bool
}

type activeGenerationTaskCounter interface {
	ActiveGenerationTaskCount(userID string) (int, error)
}

func isSubscriptionExpired(expiresAt string, now time.Time) bool {
	expiresAt = strings.TrimSpace(expiresAt)
	if expiresAt == "" {
		return false
	}
	if parsed, err := time.Parse(time.RFC3339Nano, expiresAt); err == nil {
		return parsed.Before(now)
	}
	if parsed, err := time.Parse(time.RFC3339, expiresAt); err == nil {
		return parsed.Before(now)
	}
	return false
}

func maxQueuedLimitForPlan(planID string, concurrency int) int {
	planLower := strings.ToLower(strings.TrimSpace(planID))
	switch {
	case strings.Contains(planLower, "enterprise") || concurrency == 0:
		return defaultEnterpriseBacklog
	case strings.Contains(planLower, "ultimate") || concurrency >= 20:
		return defaultUltimateQueueBacklog
	case strings.Contains(planLower, "pro") || concurrency >= 8:
		return defaultProQueueBacklog
	case strings.Contains(planLower, "basic") || strings.Contains(planLower, "month") || concurrency >= 3:
		return defaultBasicQueueBacklog
	default:
		return defaultFreeQueueBacklog
	}
}

func resolveUserConcurrencyProfile(planID string, configuredConcurrency int, expiresAt string, now time.Time) userConcurrencyProfile {
	expired := isSubscriptionExpired(expiresAt, now)
	if expired {
		return userConcurrencyProfile{
			PlanID:      planID,
			Concurrency: defaultFreeConcurrency,
			MaxQueued:   defaultFreeQueueBacklog,
			Expired:     true,
		}
	}
	concurrency := configuredConcurrency
	if concurrency < 0 {
		concurrency = defaultFreeConcurrency
	}
	maxQueued := maxQueuedLimitForPlan(planID, concurrency)
	return userConcurrencyProfile{
		PlanID:      planID,
		Concurrency: concurrency,
		MaxQueued:   maxQueued,
		Expired:     false,
	}
}

func checkPostgresGenerationAdmissionTx(ctx context.Context, tx *sql.Tx, userID string, authorization modelCallAuthorization, now time.Time) (concurrencyAdmissionDecision, error) {
	if authorization.ContextType == contextEnterprise {
		var queued int
		if err := tx.QueryRowContext(ctx, `
			select count(*)
			from xz_generation_tasks
			where user_id=$1
			  and upper(coalesce(nullif(task_status,''),status)) in ('PENDING','QUEUED','CREATED')
		`, userID).Scan(&queued); err != nil {
			return concurrencyAdmissionDecision{}, err
		}
		if queued >= defaultEnterpriseBacklog {
			return concurrencyAdmissionDecision{}, generationQueueLimitError("enterprise", queued, defaultEnterpriseBacklog)
		}
		var running int
		if err := tx.QueryRowContext(ctx, `
			select count(*)
			from xz_generation_tasks
			where user_id=$1
			  and upper(coalesce(nullif(task_status,''),status)) in ('DISPATCHING','RUNNING','PROCESSING')
		`, userID).Scan(&running); err != nil {
			return concurrencyAdmissionDecision{}, err
		}
		profile := userConcurrencyProfile{
			PlanID:      "enterprise",
			Concurrency: defaultEnterpriseCap,
			MaxQueued:   defaultEnterpriseBacklog,
		}
		return concurrencyAdmissionDecision{
			Profile:      profile,
			RunningCount: running,
			QueuedCount:  queued,
			CanDispatch:  running < defaultEnterpriseCap,
		}, nil
	}

	if _, err := tx.ExecContext(ctx, `select pg_advisory_xact_lock(hashtext($1))`, "generation-concurrency:"+userID); err != nil {
		return concurrencyAdmissionDecision{}, err
	}
	var planID string
	var configuredConcurrency int
	var expiresAt string
	if err := tx.QueryRowContext(ctx, `
		select coalesce(user_account.plan_id,''), coalesce(plan.concurrency,-1), coalesce(user_account.subscription_expires_at,'')
		from xz_users user_account
		left join xz_plans plan on plan.id=user_account.plan_id
		where user_account.id=$1
	`, userID).Scan(&planID, &configuredConcurrency, &expiresAt); err != nil {
		return concurrencyAdmissionDecision{}, err
	}

	profile := resolveUserConcurrencyProfile(planID, configuredConcurrency, expiresAt, now)

	var queued int
	if err := tx.QueryRowContext(ctx, `
		select count(*)
		from xz_generation_tasks
		where user_id=$1
		  and upper(coalesce(nullif(task_status,''),status)) in ('PENDING','QUEUED','CREATED')
	`, userID).Scan(&queued); err != nil {
		return concurrencyAdmissionDecision{}, err
	}
	if queued >= profile.MaxQueued {
		return concurrencyAdmissionDecision{}, generationQueueLimitError(profile.PlanID, queued, profile.MaxQueued)
	}

	var running int
	if err := tx.QueryRowContext(ctx, `
		select count(*)
		from xz_generation_tasks
		where user_id=$1
		  and upper(coalesce(nullif(task_status,''),status)) in ('DISPATCHING','RUNNING','PROCESSING')
	`, userID).Scan(&running); err != nil {
		return concurrencyAdmissionDecision{}, err
	}

	canDispatch := (profile.Concurrency == 0) || (running < profile.Concurrency)

	return concurrencyAdmissionDecision{
		Profile:      profile,
		RunningCount: running,
		QueuedCount:  queued,
		CanDispatch:  canDispatch,
	}, nil
}

func enforcePostgresGenerationConcurrencyTx(ctx context.Context, tx *sql.Tx, userID string, authorization modelCallAuthorization) error {
	decision, err := checkPostgresGenerationAdmissionTx(ctx, tx, userID, authorization, time.Now().UTC())
	if err != nil {
		return err
	}
	if !decision.CanDispatch {
		return generationConcurrencyError(decision.Profile.PlanID, decision.RunningCount, decision.Profile.Concurrency)
	}
	return nil
}

func enforceJSONGenerationConcurrency(data platformData, userID string) error {
	limit, enforced := jsonGenerationConcurrencyLimit(data, userID)
	if !enforced || limit == 0 {
		return nil
	}
	active := activeGenerationTaskCount(data.GenerationTasks, userID)
	if active >= limit {
		user := userMap(data.Users)[userID]
		return generationConcurrencyError(user.PlanID, active, limit)
	}
	return nil
}

func isActivelyRunningTaskStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "DISPATCHING", "RUNNING", "PROCESSING":
		return true
	default:
		return false
	}
}

func isQueuedTaskStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "QUEUED", "PENDING", "CREATED":
		return true
	default:
		return false
	}
}

func activeRunningGenerationTaskCount(tasks []generationTask, userID string) int {
	active := 0
	for _, task := range tasks {
		if task.UserID != userID {
			continue
		}
		status := firstNonEmptyString(task.TaskStatus, task.Status)
		if isActivelyRunningTaskStatus(status) && !strings.EqualFold(task.TaskStatus, taskStatusQueued) {
			active++
		}
	}
	return active
}

func queuedGenerationTaskCount(tasks []generationTask, userID string) int {
	queued := 0
	for _, task := range tasks {
		if task.UserID != userID {
			continue
		}
		status := firstNonEmptyString(task.TaskStatus, task.Status)
		if isQueuedTaskStatus(status) || strings.EqualFold(task.TaskStatus, taskStatusQueued) {
			queued++
		}
	}
	return queued
}

func activeGenerationTaskCount(tasks []generationTask, userID string) int {
	active := 0
	for _, task := range tasks {
		if task.UserID != userID {
			continue
		}
		status := firstNonEmptyString(task.TaskStatus, task.Status)
		if isRunningGenerationTaskStatus(status) || strings.EqualFold(status, taskStatusCreated) {
			active++
		}
	}
	return active
}

func (s *jsonStore) ActiveGenerationTaskCount(userID string) (int, error) {
	data, err := s.load()
	if err != nil {
		return 0, err
	}
	return activeGenerationTaskCount(data.GenerationTasks, userID), nil
}

func (s *postgresStore) ActiveGenerationTaskCount(userID string) (int, error) {
	ctx, cancel := s.withTimeout()
	defer cancel()
	if err := s.ensureReady(ctx); err != nil {
		return 0, err
	}
	var active int
	err := s.db.QueryRowContext(ctx, `
		select count(*)
		from xz_generation_tasks
		where user_id=$1
		  and upper(coalesce(nullif(task_status,''),status)) in ('DISPATCHING','RUNNING','PROCESSING')
	`, userID).Scan(&active)
	return active, err
}

func activeGenerationTaskCountForStore(store platformStore, userID string) (int, error) {
	if counter, ok := store.(activeGenerationTaskCounter); ok {
		return counter.ActiveGenerationTaskCount(userID)
	}
	tasks, err := store.ListGenerationTasks()
	if err != nil {
		return 0, err
	}
	return activeGenerationTaskCount(tasks, userID), nil
}

func adminPlanConcurrencyLimit(data adminPlatformData, user adminUser) int {
	plan, exists := planMap(data.Plans)[user.PlanID]
	if !exists {
		return 1
	}
	return plan.Concurrency
}

func jsonGenerationConcurrencyLimit(data platformData, userID string) (int, bool) {
	user, exists := userMap(data.Users)[userID]
	if !exists {
		// Low-level tests and legacy offline data may only contain a point account.
		return 0, false
	}
	plan, exists := planMap(data.Plans)[user.PlanID]
	if !exists {
		return 1, true
	}
	return plan.Concurrency, true
}

func generationConcurrencyError(planID string, active int, limit int) error {
	return fmt.Errorf("%w: package %s has %d active task(s), limit %d", errGenerationConcurrencyLimit, firstNonEmptyString(planID, "legacy_default"), active, limit)
}

func generationQueueLimitError(planID string, queued int, limit int) error {
	return fmt.Errorf("%w: package %s has %d queued task(s), backlog limit %d", errGenerationQueueLimitExceeded, firstNonEmptyString(planID, "legacy_default"), queued, limit)
}
