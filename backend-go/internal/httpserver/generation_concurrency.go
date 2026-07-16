package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var errGenerationConcurrencyLimit = errors.New("generation concurrency limit reached")

type activeGenerationTaskCounter interface {
	ActiveGenerationTaskCount(userID string) (int, error)
}

func enforcePostgresGenerationConcurrencyTx(ctx context.Context, tx *sql.Tx, userID string, authorization modelCallAuthorization) error {
	if authorization.ContextType == contextEnterprise {
		// Enterprise concurrency is governed by tenant compute and seat policies.
		return nil
	}
	if _, err := tx.ExecContext(ctx, `select pg_advisory_xact_lock(hashtext($1))`, "generation-concurrency:"+userID); err != nil {
		return err
	}
	var planID string
	var concurrency int
	if err := tx.QueryRowContext(ctx, `
		select coalesce(user_account.plan_id,''), coalesce(plan.concurrency,-1)
		from xz_users user_account
		left join xz_plans plan on plan.id=user_account.plan_id
		where user_account.id=$1
	`, userID).Scan(&planID, &concurrency); err != nil {
		return err
	}
	if concurrency < 0 {
		// Missing/legacy plan references receive the safest usable baseline.
		concurrency = 1
	}
	if concurrency == 0 {
		return nil
	}
	var active int
	if err := tx.QueryRowContext(ctx, `
		select count(*)
		from xz_generation_tasks
		where user_id=$1
		  and upper(coalesce(nullif(task_status,''),status)) in ('PENDING','QUEUED','RUNNING','PROCESSING','RETRYING','CREATED')
	`, userID).Scan(&active); err != nil {
		return err
	}
	if active >= concurrency {
		return generationConcurrencyError(planID, active, concurrency)
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
		  and upper(coalesce(nullif(task_status,''),status)) in ('PENDING','QUEUED','RUNNING','PROCESSING','RETRYING','CREATED')
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
