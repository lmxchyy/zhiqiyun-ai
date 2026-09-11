package httpserver

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"time"

	"xianzhi-ai/backend-go/internal/config"
	pe "xianzhi-ai/backend-go/internal/providerexecution"
)

// RunGenerationReaper is explicitly opt-in. The default flag value is false;
// when disabled this goroutine does not inspect or mutate generation tasks.
// Requeued tasks reset their existing outbox event, so the normal publisher
// and consumer path performs the retry without a second provider Create.
func RunGenerationReaper(ctx context.Context, cfg config.Config, db *sql.DB) error {
	if !cfg.GenerationWorkerReaperEnabled {
		<-ctx.Done()
		return ctx.Err()
	}
	if db == nil {
		return fmt.Errorf("generation reaper database is required")
	}
	interval := parseReaperDuration(cfg.GenerationWorkerReaperInterval, 30*time.Second)
	maxAttempts, _ := strconv.Atoi(cfg.GenerationWorkerReaperMaxAttempts)
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	store := pe.NewStore(db)
	run := func() error {
		requeued, recoveries, err := store.ReapExpired(ctx, 100, maxAttempts)
		if err == nil && (len(requeued) > 0 || len(recoveries) > 0) {
			log.Printf("generation_reaper requeued=%d provider_recoveries=%d", len(requeued), len(recoveries))
		}
		return err
	}
	if err := run(); err != nil && ctx.Err() == nil {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := run(); err != nil && ctx.Err() == nil {
				return err
			}
		}
	}
}

func parseReaperDuration(value string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}
