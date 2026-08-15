package ppt

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func phase2PostgresDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("PPT_TEST_DATABASE_URL"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("XIANZHI_TEST_DATABASE_URL"))
	}
	if dsn == "" {
		t.Skip("PPT_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	return db
}

func applyPhase2GenerationMigration(t *testing.T, db *sql.DB) {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve integration test path")
	}
	path := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "..", "database", "migrations", "109-ppt-v2-durable-generation.sql"))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, string(raw)); err != nil {
		t.Fatalf("apply Phase 2 migration: %v", err)
	}
}

func phase2PostgresTask(t *testing.T, db *sql.DB, suffix string) (*Service, Task) {
	t.Helper()
	service := NewPostgresService(db, "")
	response, err := service.Generate(GenerateRequest{
		UserID: "pptv2_user_" + suffix, TenantID: "pptv2_tenant_" + suffix, OrganizationID: "pptv2_org_" + suffix,
		ClientRequestID: "pptv2_task_request_" + suffix, Prompt: "PPT V2 Phase 2", SlideCount: 2,
		Outline: &Outline{Title: "PPT V2", Slides: []OutlineSlide{{Page: 1, Title: "Cover", Layout: "cover"}, {Page: 2, Title: "Durable", Layout: "content"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	task, err := service.GetTask("pptv2_user_"+suffix, response.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	return service, task
}

func cleanupPhase2Postgres(t *testing.T, db *sql.DB, task Task) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = db.ExecContext(ctx, `delete from xz_ppt_v2_generation_jobs where existing_task_id=$1`, task.TaskID)
		_, _ = db.ExecContext(ctx, `delete from xz_ppt_tasks where task_id=$1`, task.TaskID)
	})
}

func TestPostgresGenerationJobLeaseFencingRestartCancelAndIsolation(t *testing.T) {
	db := phase2PostgresDatabase(t)
	service, task := phase2PostgresTask(t, db, time.Now().UTC().Format("20060102150405.000000000"))
	_ = service
	applyPhase2GenerationMigration(t, db)
	cleanupPhase2Postgres(t, db, task)
	store, err := NewPostgresGenerationJobStore(db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	input := CreateGenerationJobInput{
		TenantID: task.TenantID, UserID: task.UserID, OrganizationID: task.OrganizationID, ExistingTaskID: task.TaskID,
		ClientRequestID: "phase2-postgres-client", IdempotencyKey: "phase2-postgres-idempotency-" + task.TaskID,
		MaxAttempts: 3, SlideCount: 2, Now: now,
	}
	job, created, err := store.Create(t.Context(), input)
	if err != nil || !created {
		t.Fatalf("create postgres job: created=%v job=%+v err=%v", created, job, err)
	}
	replayed, created, err := store.Create(t.Context(), input)
	if err != nil || created || replayed.ID != job.ID {
		t.Fatalf("idempotent postgres replay: created=%v replayed=%+v err=%v", created, replayed, err)
	}
	scope := GenerationJobScope{TenantID: task.TenantID, UserID: task.UserID}
	first, err := store.Claim(t.Context(), scope, job.ID, "postgres_worker_old", now, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	renewed, err := store.Renew(t.Context(), first, now.Add(time.Second), 3*time.Second)
	if err != nil || !renewed.LeaseExpiresAt.Equal(now.Add(4*time.Second)) {
		t.Fatalf("renew postgres lease: lease=%+v err=%v", renewed, err)
	}
	if _, err := store.Claim(t.Context(), scope, job.ID, "postgres_worker_new", now.Add(3*time.Second), time.Minute); !errors.Is(err, ErrGenerationJobLeaseHeld) {
		t.Fatalf("active postgres lease error = %v", err)
	}
	restartedStore, err := NewPostgresGenerationJobStore(db)
	if err != nil {
		t.Fatal(err)
	}
	second, err := restartedStore.Claim(t.Context(), scope, job.ID, "postgres_worker_new", now.Add(5*time.Second), time.Minute)
	if err != nil || second.FencingToken <= first.FencingToken {
		t.Fatalf("restart reclaim: old=%+v new=%+v err=%v", first, second, err)
	}
	if _, err := store.Checkpoint(t.Context(), first, GenerationCheckpoint{NextStage: GenerationStageTaskLoaded, InputSnapshot: []byte(`{}`), Now: now.Add(6 * time.Second)}); !errors.Is(err, ErrGenerationJobLeaseLost) {
		t.Fatalf("stale postgres fence error = %v", err)
	}
	checkpointGenerationStage(t, restartedStore, second, GenerationStageTaskLoaded, now.Add(6*time.Second))
	bundle, err := restartedStore.Get(t.Context(), scope, job.ID)
	if err != nil || bundle.Job.Stage != GenerationStageTaskLoaded || len(bundle.Attempts) != 2 || len(bundle.History) != 2 {
		t.Fatalf("restart checkpoint missing: bundle=%+v err=%v", bundle, err)
	}
	if _, err := restartedStore.Get(t.Context(), GenerationJobScope{TenantID: "other", UserID: task.UserID}, job.ID); !errors.Is(err, ErrGenerationJobNotFound) {
		t.Fatalf("cross-tenant postgres read error = %v", err)
	}
	cancelled, err := restartedStore.Cancel(t.Context(), scope, job.ID, now.Add(7*time.Second))
	if err != nil || cancelled.Status != GenerationJobCancelled {
		t.Fatalf("postgres cancel: job=%+v err=%v", cancelled, err)
	}
	if _, err := restartedStore.Checkpoint(t.Context(), second, GenerationCheckpoint{NextStage: GenerationStageRendered, Now: now.Add(8 * time.Second)}); !errors.Is(err, ErrGenerationJobCancelled) {
		t.Fatalf("cancelled postgres job accepted checkpoint: %v", err)
	}
	if _, err := restartedStore.Claim(t.Context(), scope, job.ID, "worker_after_cancel", now.Add(time.Hour), time.Minute); !errors.Is(err, ErrGenerationJobCancelled) {
		t.Fatalf("cancelled postgres job reopened: %v", err)
	}
}

func requirePostgresUniqueConstraint(t *testing.T, err error, constraint string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" || pgErr.ConstraintName != constraint {
		t.Fatalf("unique constraint %s error = %v", constraint, err)
	}
}

func TestPostgresGenerationJobArtifactConstraintsAndTransactionRollback(t *testing.T) {
	db := phase2PostgresDatabase(t)
	suffix := fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	service, task := phase2PostgresTask(t, db, "constraints_"+suffix)
	applyPhase2GenerationMigration(t, db)
	cleanupPhase2Postgres(t, db, task)
	store, err := NewPostgresGenerationJobStore(db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	job, _, err := store.Create(t.Context(), CreateGenerationJobInput{
		TenantID: task.TenantID, UserID: task.UserID, OrganizationID: task.OrganizationID, ExistingTaskID: task.TaskID,
		ClientRequestID: "phase2-constraints-client-" + suffix, IdempotencyKey: "phase2-constraints-idempotency-" + suffix,
		MaxAttempts: 2, SlideCount: 2, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assetIDs := []string{"pptv2_constraint_asset_1_" + suffix, "pptv2_constraint_asset_2_" + suffix}
	fileIDs := []string{"pptv2_constraint_file_1_" + suffix, "pptv2_constraint_file_2_" + suffix}
	t.Cleanup(func() {
		_, _ = db.Exec(`delete from xz_assets where id = any($1)`, assetIDs)
		_, _ = db.Exec(`delete from xz_file_objects where file_id = any($1)`, fileIDs)
	})
	metadata := fmt.Sprintf(`{"source":"ppt-v2","pptV2GenerationJobId":%q}`, job.ID)
	createdAt := now.Format(time.RFC3339Nano)
	insertAsset := `insert into xz_assets(id,user_id,tenant_id,organization_id,task_id,name,media_type,url,metadata,created_at,updated_at,raw) values($1,$2,$3,$4,$5,$1,'pptx','https://example.invalid/pptx',$6::jsonb,$7,$7,'{}'::jsonb)`
	if _, err := db.ExecContext(t.Context(), insertAsset, assetIDs[0], task.UserID, task.TenantID, task.OrganizationID, task.TaskID, metadata, createdAt); err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(t.Context(), insertAsset, assetIDs[1], task.UserID, task.TenantID, task.OrganizationID, task.TaskID, metadata, createdAt)
	requirePostgresUniqueConstraint(t, err, "uq_ppt_v2_work_center_asset_per_job")

	insertFile := `insert into xz_file_objects(file_id,tenant_id,user_id,storage_config_id,provider,bucket,object_key,original_name,stored_name,business_type,business_id,visibility,status,metadata) values($1,$2,$3,'pptv2-test-config','minio','pptv2-test',$4,'deck.pptx',$1,'ppt_v2_generation',$5,'PRIVATE','ACTIVE','{}'::jsonb)`
	if _, err := db.ExecContext(t.Context(), insertFile, fileIDs[0], task.TenantID, task.UserID, "pptv2/"+fileIDs[0], job.ID); err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(t.Context(), insertFile, fileIDs[1], task.TenantID, task.UserID, "pptv2/"+fileIDs[1], job.ID)
	requirePostgresUniqueConstraint(t, err, "uq_ppt_v2_file_per_job")

	scope := GenerationJobScope{TenantID: task.TenantID, UserID: task.UserID}
	lease := claimGenerationJobFixture(t, store, scope, job.ID, "postgres_rollback_worker", now)
	checkpointGenerationStage(t, store, lease, GenerationStageTaskLoaded, now.Add(time.Second))
	checkpointGenerationStage(t, store, lease, GenerationStageRendered, now.Add(2*time.Second))
	checkpointGenerationStage(t, store, lease, GenerationStageFileStored, now.Add(3*time.Second))
	checkpointGenerationStage(t, store, lease, GenerationStageAssetCreated, now.Add(4*time.Second))

	functionName := "pptv2_relation_rollback_" + suffix
	triggerName := "pptv2_relation_rollback_trigger_" + suffix
	if _, err := db.ExecContext(t.Context(), fmt.Sprintf(`create function %s() returns trigger as $$ begin if new.task_id = TG_ARGV[0] then raise exception 'forced Phase 2 relation rollback'; end if; return new; end $$ language plpgsql`, functionName)); err != nil {
		t.Fatal(err)
	}
	dropRollbackFixture := func() {
		_, _ = db.Exec(fmt.Sprintf(`drop trigger if exists %s on xz_ppt_tasks`, triggerName))
		_, _ = db.Exec(fmt.Sprintf(`drop function if exists %s()`, functionName))
	}
	t.Cleanup(dropRollbackFixture)
	taskIDLiteral := strings.ReplaceAll(task.TaskID, "'", "''")
	if _, err := db.ExecContext(t.Context(), fmt.Sprintf(`create trigger %s before update on xz_ppt_tasks for each row execute function %s('%s')`, triggerName, functionName, taskIDLiteral)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RelateTaskArtifact(t.Context(), lease, V2ArtifactRelation{DeckID: "deck_phase2", Revision: 1, PPTXAssetID: "asset_phase2"}, now.Add(5*time.Second)); err == nil {
		t.Fatal("forced Task relation failure unexpectedly committed")
	}
	bundle, err := store.Get(t.Context(), scope, job.ID)
	if err != nil || bundle.Job.Stage != GenerationStageAssetCreated || bundle.Job.CompletedWorkUnits != 4 {
		t.Fatalf("Task relation rollback leaked job checkpoint: bundle=%+v err=%v", bundle, err)
	}
	persisted, err := service.GetTask(task.UserID, task.TaskID)
	if err != nil || persisted.V2DeckID != "" || persisted.V2Revision != 0 || persisted.PPTXAssetID != "" {
		t.Fatalf("Task relation rollback leaked relation: task=%+v err=%v", persisted, err)
	}
}

func TestPostgresGenerationJobRetryAndAtomicTaskRelation(t *testing.T) {
	db := phase2PostgresDatabase(t)
	service, task := phase2PostgresTask(t, db, "relation_"+time.Now().UTC().Format("20060102150405.000000000"))
	applyPhase2GenerationMigration(t, db)
	cleanupPhase2Postgres(t, db, task)
	store, err := NewPostgresGenerationJobStore(db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	job, _, err := store.Create(t.Context(), CreateGenerationJobInput{
		TenantID: task.TenantID, UserID: task.UserID, OrganizationID: task.OrganizationID, ExistingTaskID: task.TaskID,
		ClientRequestID: "phase2-relation-client", IdempotencyKey: "phase2-relation-idempotency-" + task.TaskID,
		MaxAttempts: 2, SlideCount: 2, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	scope := GenerationJobScope{TenantID: task.TenantID, UserID: task.UserID}
	first := claimGenerationJobFixture(t, store, scope, job.ID, "postgres_retry_1", now)
	retry, err := store.Fail(t.Context(), first, GenerationJobError{Code: "TEMPORARY", Message: "retry", Retryable: true}, now.Add(time.Second), 0)
	if err != nil || retry.Status != GenerationJobRetryWait {
		t.Fatalf("postgres retry state: job=%+v err=%v", retry, err)
	}
	second := claimGenerationJobFixture(t, store, scope, job.ID, "postgres_retry_2", now.Add(2*time.Second))
	checkpointGenerationStage(t, store, second, GenerationStageTaskLoaded, now.Add(3*time.Second))
	checkpointGenerationStage(t, store, second, GenerationStageRendered, now.Add(4*time.Second))
	checkpointGenerationStage(t, store, second, GenerationStageFileStored, now.Add(5*time.Second))
	checkpointGenerationStage(t, store, second, GenerationStageAssetCreated, now.Add(6*time.Second))
	related, err := store.RelateTaskArtifact(t.Context(), second, V2ArtifactRelation{DeckID: "deck_phase2", Revision: 1, PPTXAssetID: "asset_phase2"}, now.Add(7*time.Second))
	if err != nil || related.Stage != GenerationStageTaskRelated || related.CompletedWorkUnits != 5 {
		t.Fatalf("atomic relation: job=%+v err=%v", related, err)
	}
	completed, err := store.Checkpoint(t.Context(), second, GenerationCheckpoint{NextStage: GenerationStageCompleted, Now: now.Add(8 * time.Second)})
	if err != nil || completed.Status != GenerationJobSucceeded || completed.Progress() != 100 {
		t.Fatalf("postgres completion: job=%+v err=%v", completed, err)
	}
	persisted, err := service.GetTask(task.UserID, task.TaskID)
	if err != nil || persisted.TenantID != task.TenantID || persisted.V2DeckID != "deck_phase2" || persisted.PPTXAssetID != "asset_phase2" {
		t.Fatalf("atomic Task relation missing: task=%+v err=%v", persisted, err)
	}
	if _, err := store.RelateTaskArtifact(t.Context(), second, V2ArtifactRelation{DeckID: "other", Revision: 1, PPTXAssetID: "other"}, now.Add(9*time.Second)); !errors.Is(err, ErrGenerationJobTerminal) {
		t.Fatalf("terminal relation rewrite error = %v", err)
	}
}
