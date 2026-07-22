package httpserver

import (
	"context"
	"database/sql"
	"log"
	"time"
)

// StartIdentityDowngradeWorker rechecks scheduled and settlement-waiting
// requests. Execution remains transactional and re-runs every blocker check.
func StartIdentityDowngradeWorker(ctx context.Context, db *sql.DB, interval time.Duration) {
	if db == nil {
		return
	}
	if interval <= 0 {
		interval = time.Minute
	}
	store := &postgresStore{db: db, ready: true}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				workCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
				err := store.ProcessDueIdentityDowngrades(workCtx, 20)
				cancel()
				if err != nil {
					log.Printf("identity downgrade worker: %v", err)
				}
			}
		}
	}()
}
