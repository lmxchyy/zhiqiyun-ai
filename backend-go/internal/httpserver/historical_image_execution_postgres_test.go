package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/messaging"
	pe "xianzhi-ai/backend-go/internal/providerexecution"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

// These are source-level regression proofs, NOT permission to exempt history
// from SafeDrain. The disposable, loopback DB must be fully migrated; CI sets
// HISTORICAL_IMAGE_REQUIRE_POSTGRES=1 so missing dependencies fail, not skip.
func historicalImageDB(t *testing.T) *sql.DB {
	t.Helper()
	if os.Getenv("XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL") == "" {
		if os.Getenv("HISTORICAL_IMAGE_REQUIRE_POSTGRES") == "1" {
			t.Fatal("historical behavior requires a fully migrated disposable loopback test PostgreSQL; release remains BLOCKED")
		}
		t.Skip("disposable PostgreSQL unavailable; historical behavior NOT certified")
	}
	return issue190DB(t)
}

type historicalImageStorage struct {
	*generatedStorageTestProvider
	puts int
}

func (s *historicalImageStorage) PutObject(ctx context.Context, key string, source io.Reader, size int64, contentType string) (storagecenter.ObjectMetadata, error) {
	s.puts++
	return s.generatedStorageTestProvider.PutObject(ctx, key, source, size, contentType)
}

// Seed unrelated synthetic history using the normal funded task/execution
// barriers first, then reproduce a later DISPATCHING generation in this DB.
// No quarantine row is inserted and no production identity is used.
func historicalImageSetup(t *testing.T, db *sql.DB, status pe.Status, manifest bool) (*issue190Fixture, pe.Execution, int64, string, *historicalImageStorage) {
	t.Helper()
	f := issue190Setup(t, db, imageDispatchNormal, false)
	g, owner, err := claimGenerationTaskOwnership(f.store, f.task.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.req.Params[providerExecutionTaskParam] = f.task.ID
	ctx := pe.WithGenerationOwnership(context.Background(), f.task.ID, owner, g)
	identity, _, err := executionIdentity(f.req, "image", providerName(f.req))
	if err != nil {
		t.Fatal(err)
	}
	store := pe.NewStore(db)
	e, err := store.CreatePreparedForGenerationTask(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	e, err = store.ClaimPreparedForGenerationTask(ctx, f.task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status == pe.Succeeded && manifest {
		raw, err := json.Marshal(issue190Images())
		if err != nil {
			t.Fatal(err)
		}
		err = store.SaveSucceededResult(ctx, e.ID, nil, raw)
		if err != nil {
			t.Fatal(err)
		}
	} else if err = store.Transition(ctx, e.ID, status, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339Nano)
	if _, err = db.Exec(`UPDATE xz_generation_tasks SET status='PROCESSING',task_status='DISPATCHING',execution_generation=$2,worker_id='synthetic-unrelated-worker',lease_until=NULL,last_heartbeat_at=NULL,updated_at=$3,raw=raw||jsonb_build_object('status','PROCESSING','updatedAt',$3::text) WHERE id=$1`, f.task.ID, g+2, old); err != nil {
		t.Fatal(err)
	}
	f.task.Status = "PROCESSING"
	f.task.TaskStatus = "DISPATCHING"
	f.task.UpdatedAt = old
	f.a.unknownGenerationGrace = time.Hour
	storage := &historicalImageStorage{generatedStorageTestProvider: &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}}}
	f.a.fileService = storagecenter.NewService(storagecenter.NewMemoryRepository(), generatedStorageTestFactory{provider: storage}, storagecenter.Options{DefaultProvider: "s3", Endpoint: "https://storage.example", AccessKey: "test", SecretKey: "test", Bucket: "test-private", DefaultQuotaBytes: 1 << 20, MaxUploadBytes: 1 << 20, MasterKey: "0123456789abcdef0123456789abcdef"})
	e, err = store.GetByID(context.Background(), e.ID)
	if err != nil {
		t.Fatal(err)
	}
	return f, e, g, owner, storage
}

// Compare complete row bytes, not merely counts. A single read-only snapshot
// covers task/result/raw, provider/correlations, assets, billing and transport.
func historicalImageSnapshot(t *testing.T, f *issue190Fixture) map[string]string {
	t.Helper()
	tx, err := f.db.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	projections := map[string]string{
		"task":        `SELECT * FROM xz_generation_tasks WHERE id=$1`,
		"execution":   `SELECT * FROM provider_executions WHERE task_id=$1`,
		"correlation": `SELECT * FROM provider_execution_correlations WHERE execution_id IN (SELECT id FROM provider_executions WHERE task_id=$1)`,
		"asset":       `SELECT * FROM xz_assets WHERE task_id=$1`,
		"lifecycle":   `SELECT * FROM xz_billing_lifecycle_events WHERE task_id=$1`,
		"account":     `SELECT * FROM xz_point_accounts WHERE id=$2`,
		"wallet":      `SELECT * FROM xz_wallet_ledger WHERE account_id=$2`,
		"lot":         `SELECT * FROM xz_personal_point_lots WHERE account_id=$2`,
		"reservation": `SELECT * FROM xz_personal_point_reservations WHERE account_id=$2`,
		"allocation":  `SELECT * FROM xz_personal_point_reservation_allocations WHERE account_id=$2`,
		"movement":    `SELECT * FROM xz_personal_point_lot_movements WHERE account_id=$2`,
		"outbox":      `SELECT * FROM outbox_events WHERE aggregate_id=$1`,
		"inbox":       `SELECT * FROM consumer_inbox WHERE event_id IN (SELECT event_id FROM outbox_events WHERE aggregate_id=$1) OR event_id LIKE 'issue190:'||$1||':%'`,
		"quarantine":  `SELECT * FROM provider_execution_quarantine WHERE task_id=$1`,
	}
	result := make(map[string]string, len(projections))
	for name, projection := range projections {
		// Declare both parameter types even in projections using only one.
		query := `WITH identity AS (SELECT $1::text AS task_id,$2::text AS account_id) SELECT coalesce(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text),'[]'::jsonb)::text FROM (` + projection + `) r`
		var raw string
		if err := tx.QueryRow(query, f.task.ID, f.task.PersonalPointAccountID).Scan(&raw); err != nil {
			t.Fatalf("snapshot %s: %v", name, err)
		}
		result[name] = raw
	}
	return result
}

func assertHistoricalImageUnchanged(t *testing.T, f *issue190Fixture, storage *historicalImageStorage, before map[string]string) {
	t.Helper()
	after := historicalImageSnapshot(t, f)
	for name, raw := range before {
		if raw != after[name] {
			t.Errorf("historical %s projection changed", name)
		}
	}
	if f.provider.calls.Load() != 0 || f.provider.gets.Load() != 0 || storage.puts != 0 || len(storage.objects) != 0 {
		t.Fatalf("external effects generate=%d get=%d storage_put=%d objects=%d", f.provider.calls.Load(), f.provider.gets.Load(), storage.puts, len(storage.objects))
	}
}

func TestHistoricalImageSucceededExecutionFencesPostgres(t *testing.T) {
	db := historicalImageDB(t)
	for _, manifest := range []bool{false, true} {
		for _, binding := range []string{"older", "future"} {
			t.Run(fmt.Sprintf("%s/manifest=%t", binding, manifest), func(t *testing.T) {
				f, e, oldGen, oldOwner, storage := historicalImageSetup(t, db, pe.Succeeded, manifest)
				current, _, _, _ := f.taskState(t)
				if binding == "future" {
					if _, err := db.Exec(`UPDATE provider_executions SET task_execution_generation=$2 WHERE id=$1`, e.ID, current+1); err != nil {
						t.Fatal(err)
					}
					e.TaskGeneration = new(int64)
					*e.TaskGeneration = current + 1
				}
				before := historicalImageSnapshot(t, f)
				ctx := pe.WithGenerationOwnership(context.Background(), f.task.ID, oldOwner, oldGen)
				for _, dispatch := range []int64{0, oldGen, current, current + 1} {
					_, _, err := claimGenerationTaskOwnershipForDispatch(f.store, f.task.ID, dispatch)
					requireFenced(t, err, "historical claim")
					assertHistoricalImageUnchanged(t, f, storage, before)
				}
				if _, err := guardedImage(ctx, f.req, f.provider, pe.NewStore(db)); !errors.Is(err, ErrFencedStaleExecution) {
					t.Fatalf("historical provider boundary: %v", err)
				}
				requireFenced(t, f.a.recoverSucceededGenerationTask(f.task, e), "historical local recovery")
				assertHistoricalImageUnchanged(t, f, storage, before)
				f.a.repairStaleGenerationTasksWithContext(context.Background(), time.Minute)
				assertHistoricalImageUnchanged(t, f, storage, before)
				if n, err := f.scheduler.RecoverStaleDispatches(context.Background()); err != nil || n != 0 {
					t.Fatalf("historical scheduler recovery n=%d err=%v", n, err)
				}
				if n, err := f.scheduler.dispatchUserTx(context.Background(), f.task.UserID, time.Now().UTC()); err != nil || n != 0 {
					t.Fatalf("historical scheduler admission n=%d err=%v", n, err)
				}
				requireFenced(t, f.a.runGenerationTaskForDispatch(f.task.ID, f.a.generationService, f.req, current), "historical worker")
				requireFenced(t, renewGenerationLease(f.store, f.task.ID, oldOwner, oldGen), "historical heartbeat")
				req := f.req
				req.GeneratedImages = issue190Images()
				_, err := f.store.completeGenerationTaskOwned(f.task.ID, req, oldGen, oldOwner)
				requireFenced(t, err, "historical owned settlement")
				assertHistoricalImageUnchanged(t, f, storage, before)
				f.assertEffects(t, false, 0, 0)
			})
		}
	}
}

func TestHistoricalImageSucceededCompatibilityPostgres(t *testing.T) {
	db := historicalImageDB(t)
	for _, binding := range []string{"equal", "legacy_null"} {
		t.Run(binding, func(t *testing.T) {
			f, e, _, _, storage := historicalImageSetup(t, db, pe.Succeeded, true)
			current, _, _, _ := f.taskState(t)
			var bound any = current
			if binding == "legacy_null" {
				bound = nil
			}
			if _, err := db.Exec(`UPDATE provider_executions SET task_execution_generation=$2 WHERE id=$1`, e.ID, bound); err != nil {
				t.Fatal(err)
			}
			// A valid unowned legacy handoff: arbitrary unmatched owners are
			// intentionally fenced independently of the execution binding.
			if _, err := db.Exec(`UPDATE xz_generation_tasks SET worker_id=NULL WHERE id=$1`, f.task.ID); err != nil {
				t.Fatal(err)
			}
			// Startup/watchdog must still complete eligible local success, not
			// turn into a no-op in order to pass the historical challenge.
			f.a.repairStaleGenerationTasksWithContext(context.Background(), time.Minute)
			if f.provider.calls.Load() != 0 || f.provider.gets.Load() != 0 || storage.puts != 1 {
				t.Fatalf("local completion generate=%d get=%d storage_put=%d", f.provider.calls.Load(), f.provider.gets.Load(), storage.puts)
			}
			f.assertEffects(t, true, 0, 0)
			g, state, _, _ := f.taskState(t)
			if g != current || state != taskStatusSucceeded {
				t.Fatalf("compatible completion g=%d state=%s", g, state)
			}
		})
	}
}

func TestHistoricalImageLiveOwnerAndUnknownControlsPostgres(t *testing.T) {
	db := historicalImageDB(t)
	for _, status := range []pe.Status{pe.Succeeded, pe.Unknown} {
		for _, live := range []bool{false, true} {
			if status == pe.Succeeded && !live {
				continue // Covered by the completion control above.
			}
			t.Run(fmt.Sprintf("%s/live=%t", status, live), func(t *testing.T) {
				f, e, _, _, storage := historicalImageSetup(t, db, status, status == pe.Succeeded)
				current, _, _, _ := f.taskState(t)
				if _, err := db.Exec(`UPDATE provider_executions SET task_execution_generation=$2 WHERE id=$1`, e.ID, current); err != nil {
					t.Fatal(err)
				}
				if live {
					if _, err := db.Exec(`UPDATE xz_generation_tasks SET task_status='RUNNING',worker_id='synthetic-live-worker',lease_until=now()+interval '1 hour' WHERE id=$1`, f.task.ID); err != nil {
						t.Fatal(err)
					}
				}
				before := historicalImageSnapshot(t, f)
				f.a.repairStaleGenerationTasksWithContext(context.Background(), time.Minute)
				assertHistoricalImageUnchanged(t, f, storage, before)
				if live {
					_, _, err := claimGenerationTaskOwnership(f.store, f.task.ID)
					if !errors.Is(err, errGenerationOwnershipBusy) {
						t.Fatalf("live owner claim: %v", err)
					}
					assertHistoricalImageUnchanged(t, f, storage, before)
				}
			})
		}
	}
}

func TestHistoricalImageConsumerTransportPostgres(t *testing.T) {
	db := historicalImageDB(t)
	t.Run("raw-terminal-column-running-is-not-immutable-history", func(t *testing.T) {
		// generationTaskForUpdate reads Status from raw, not the status column.
		// Do not alter terminal callback semantics: drain classification must
		// exclude this counterexample before claiming immutable history.
		f, _, _, _, storage := historicalImageSetup(t, db, pe.Succeeded, true)
		if _, err := db.Exec(`UPDATE xz_generation_tasks SET raw=raw||'{"status":"COMPLETED"}'::jsonb WHERE id=$1`, f.task.ID); err != nil {
			t.Fatal(err)
		}
		before := historicalImageSnapshot(t, f)
		current, _, _, _ := f.taskState(t)
		event := "issue190:" + f.task.ID + ":raw-terminal"
		envelope := &messaging.Envelope{EventID: event, AggregateType: "generation_task", AggregateID: f.task.ID, EventType: messaging.GenerationImageNormalRoutingKey, Data: map[string]any{"task_id": f.task.ID, "dispatch_mode": imageDispatchNormal, "execution_generation": current}}
		if err := f.a.processGenerationImageMessage(context.Background(), messaging.NewInboxStore(db), envelope, imageDispatchNormal); err != nil {
			t.Fatalf("actual raw terminal branch: %v", err)
		}
		var result string
		var terminal bool
		if err := db.QueryRow(`SELECT result,metadata->>'terminal'='true' FROM consumer_inbox WHERE event_id=$1 AND consumer_name=$2 AND processed_at IS NOT NULL`, event, generationImageNormalConsumer).Scan(&result, &terminal); err != nil || result != "completed" || !terminal {
			t.Fatalf("raw terminal transport effect result=%s terminal=%t err=%v", result, terminal, err)
		}
		after := historicalImageSnapshot(t, f)
		if before["inbox"] == after["inbox"] {
			t.Fatal("unsafe raw drift counterexample did not change inbox")
		}
		delete(before, "inbox")
		delete(after, "inbox")
		for name, raw := range before {
			if raw != after[name] {
				t.Errorf("terminal branch unexpectedly changed %s", name)
			}
		}
		if f.provider.calls.Load() != 0 || f.provider.gets.Load() != 0 || storage.puts != 0 {
			t.Fatal("raw terminal branch called provider/storage")
		}
	})
	for _, kind := range []string{"TEXT_TO_IMAGE", "IMAGE_TO_IMAGE"} {
		for _, mode := range []string{imageDispatchNormal, imageDispatchCanary} {
			for _, legacy := range []bool{false, true} {
				for _, worker := range []any{nil, "", "synthetic-unrelated-owner"} {
					t.Run(fmt.Sprintf("%s/%s/legacy=%t/owner=%v", kind, mode, legacy, worker), func(t *testing.T) {
						f, _, _, _, storage := historicalImageSetup(t, db, pe.Succeeded, true)
						params := cloneAnyMap(f.task.Params)
						params[imageDispatchModeParam] = mode
						params["generation_async_canary"] = mode == imageDispatchCanary
						delete(params, "_generation_dispatch_owner")
						if legacy {
							delete(params, imageDispatchModeParam)
							if mode == imageDispatchNormal {
								delete(params, "generation_async_canary")
								delete(params, imageFairScheduledParam)
							}
						}
						raw, _ := json.Marshal(params)
						if _, err := db.Exec(`UPDATE xz_generation_tasks SET type=$2,params=$3,worker_id=$4,lease_until=now()-interval '1 hour',raw=raw||jsonb_build_object('type',$2::text,'params',$3::jsonb) WHERE id=$1`, f.task.ID, kind, raw, worker); err != nil {
							t.Fatal(err)
						}
						current, _, _, _ := f.taskState(t)
						before := historicalImageSnapshot(t, f)
						for _, g := range []int64{current - 1, current, current + 1} {
							route := messaging.GenerationImageNormalRoutingKey
							if mode == imageDispatchCanary {
								route = messaging.GenerationCanaryRoutingKey
							}
							envelope := &messaging.Envelope{EventID: fmt.Sprintf("issue190:%s:history-%s-%d", f.task.ID, mode, g), AggregateType: "generation_task", AggregateID: f.task.ID, EventType: route, Data: map[string]any{"task_id": f.task.ID, "dispatch_mode": mode, "execution_generation": g}}
							requireFenced(t, f.a.processGenerationImageMessage(context.Background(), messaging.NewInboxStore(db), envelope, mode), "consumer before inbox commit")
							assertHistoricalImageUnchanged(t, f, storage, before)
						}
					})
				}
			}
		}
	}
}
