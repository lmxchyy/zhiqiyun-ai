package ppt

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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
	for _, migration := range []string{"109-ppt-v2-durable-generation.sql", "110-ppt-v2-agent-outline-approval.sql"} {
		path := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "..", "database", "migrations", migration))
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if _, err := db.ExecContext(ctx, string(raw)); err != nil {
			cancel()
			t.Fatalf("apply %s: %v", migration, err)
		}
		cancel()
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
	_, task := phase2PostgresTask(t, db, "constraints_"+suffix)
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
	_, mutationErr := store.RelateTaskArtifact(t.Context(), lease, V2ArtifactRelation{DeckID: "deck_phase2", Revision: 1, PPTXAssetID: "asset_phase2"}, now.Add(5*time.Second))
	var mutationPGError *pgconn.PgError
	if !errors.As(mutationErr, &mutationPGError) || mutationPGError.Code != "P0001" || !strings.Contains(mutationPGError.Message, "forced Phase 2 relation rollback") {
		t.Fatalf("forced Task relation rollback error = %v", mutationErr)
	}

	verifyCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	verifyTx, err := db.BeginTx(verifyCtx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("begin rollback verification transaction: %v", err)
	}
	defer func() { _ = verifyTx.Rollback() }()
	var persistedRaw []byte
	if err := verifyTx.QueryRowContext(verifyCtx, `select raw from xz_ppt_tasks where task_id=$1 and user_id=$2`, task.TaskID, task.UserID).Scan(&persistedRaw); err != nil {
		t.Fatalf("read Task after forced rollback: %v", err)
	}
	persisted, err := taskFromPostgresRaw(persistedRaw, task.UserID)
	if err != nil {
		t.Fatalf("decode Task after forced rollback: %v", err)
	}
	if persisted.V2DeckID != "" || persisted.V2Revision != 0 || persisted.PPTXAssetID != "" {
		t.Fatalf("Task relation rollback leaked relation: task=%+v", persisted)
	}
	var persistedStage string
	var persistedWorkUnits int
	if err := verifyTx.QueryRowContext(verifyCtx, `select stage,completed_work_units from xz_ppt_v2_generation_jobs where id=$1`, job.ID).Scan(&persistedStage, &persistedWorkUnits); err != nil {
		t.Fatalf("read GenerationJob after forced rollback: %v", err)
	}
	if persistedStage != GenerationStageAssetCreated || persistedWorkUnits != 4 {
		t.Fatalf("Task relation rollback leaked job checkpoint: stage=%s workUnits=%d", persistedStage, persistedWorkUnits)
	}
	var taskRelatedTransitions int
	if err := verifyTx.QueryRowContext(verifyCtx, `select count(*) from xz_ppt_v2_generation_transitions where generation_job_id=$1 and to_stage=$2`, job.ID, GenerationStageTaskRelated).Scan(&taskRelatedTransitions); err != nil {
		t.Fatalf("read Task relation transitions after forced rollback: %v", err)
	}
	if taskRelatedTransitions != 0 {
		t.Fatalf("Task relation rollback leaked %d TASK_RELATED transition(s)", taskRelatedTransitions)
	}
	if err := verifyTx.Commit(); err != nil {
		t.Fatalf("commit rollback verification transaction: %v", err)
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

func TestPostgresGenerationJobAgentOutlineApprovalRecoveryAndIsolation(t *testing.T) {
	db := phase2PostgresDatabase(t)
	applyPhase2GenerationMigration(t, db)
	now := time.Now().UTC()
	suffix := now.Format("20060102150405.000000000")
	store, err := NewPostgresGenerationJobStore(db)
	if err != nil {
		t.Fatal(err)
	}
	firstProvider := &countingResearchProvider{pack: agentResearchFixture(t)}
	firstPlanning := &semanticPlanningFixture{}
	service, err := NewAgentPlanningService(store, firstProvider, firstPlanning, firstPlanning, AgentPlanningOptions{WorkerID: "postgres_agent_planner_1", LeaseDuration: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	request := GuideAgentRequest{
		TenantID: "pptv2_agent_tenant_" + suffix, UserID: "pptv2_agent_user_" + suffix,
		OrganizationID: "pptv2_agent_org_" + suffix, IdempotencyKey: "pptv2_agent_guide_" + suffix,
		Request: IntentRequest{Text: "帮我做一份10页的新能源汽车行业分析，给公司管理层汇报。"}, Now: now,
	}
	guided, err := service.Guide(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessReady(t.Context(), now.Add(time.Second), 10); err != nil {
		t.Fatal(err)
	}
	state, err := service.Get(t.Context(), GenerationJobScope{TenantID: request.TenantID, UserID: request.UserID}, guided.State.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	guided.State = &state
	if guided.State == nil || guided.State.Job.Status != GenerationJobWaitingForOutlineApproval || guided.State.Job.Stage != GenerationStageOutlinePlanned || len(guided.State.Outline.Slides) != 10 {
		t.Fatalf("postgres planning did not reach approval gate: %+v", guided)
	}
	jobID := guided.State.Job.ID
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = db.ExecContext(ctx, `delete from xz_ppt_v2_generation_jobs where id=$1`, jobID)
	})
	if firstProvider.calls != 1 || guided.State.ResearchExecutionCount != 1 {
		t.Fatalf("initial research execution mismatch: provider=%d persisted=%d", firstProvider.calls, guided.State.ResearchExecutionCount)
	}
	if _, err := store.Claim(t.Context(), GenerationJobScope{TenantID: request.TenantID, UserID: request.UserID}, jobID, "premature_slide_worker", now.Add(time.Second), time.Minute); !errors.Is(err, ErrGenerationJobAwaitingOutlineApproval) {
		t.Fatalf("postgres approval gate allowed generation to continue: %v", err)
	}

	restartedStore, err := NewPostgresGenerationJobStore(db)
	if err != nil {
		t.Fatal(err)
	}
	restartedProvider := &countingResearchProvider{pack: agentResearchFixture(t)}
	restartedPlanning := &semanticPlanningFixture{}
	restartedService, err := NewAgentPlanningService(restartedStore, restartedProvider, restartedPlanning, restartedPlanning, AgentPlanningOptions{WorkerID: "postgres_agent_planner_2", LeaseDuration: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	request.Now = now.Add(time.Hour)
	restored, err := restartedService.Guide(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if restored.State == nil || restored.State.Job.ID != jobID || restored.State.Outline.ID != guided.State.Outline.ID || restored.State.Outline.Revision != guided.State.Outline.Revision {
		t.Fatalf("restart did not restore the same outline: first=%+v restored=%+v", guided.State, restored.State)
	}
	if restartedProvider.calls != 0 || restored.State.ResearchExecutionCount != 1 {
		t.Fatalf("restart repeated research: provider=%d persisted=%d", restartedProvider.calls, restored.State.ResearchExecutionCount)
	}
	wrongScope := GenerationJobScope{TenantID: "other_tenant", UserID: request.UserID}
	if _, err := restartedService.Get(t.Context(), wrongScope, jobID); !errors.Is(err, ErrGenerationJobNotFound) {
		t.Fatalf("cross-tenant planning read error = %v", err)
	}
	if _, err := restartedService.UpdateOutline(t.Context(), wrongScope, jobID, 1, []OutlineEditCommand{{
		Type: OutlineCommandMoveSlide, SlideID: restored.State.Outline.Slides[2].SlideID, ToIndex: 2,
	}}, now.Add(60*time.Minute)); !errors.Is(err, ErrGenerationJobNotFound) {
		t.Fatalf("cross-tenant planning update error = %v", err)
	}
	if _, err := restartedService.ApproveOutline(t.Context(), wrongScope, jobID, 1, now.Add(60*time.Minute)); !errors.Is(err, ErrGenerationJobNotFound) {
		t.Fatalf("cross-tenant planning approve error = %v", err)
	}
	scope := GenerationJobScope{TenantID: request.TenantID, UserID: request.UserID}
	updated, err := restartedService.UpdateOutline(t.Context(), scope, jobID, 1, []OutlineEditCommand{{
		Type: OutlineCommandMoveSlide, SlideID: restored.State.Outline.Slides[2].SlideID, ToIndex: 2,
	}}, now.Add(61*time.Minute))
	if err != nil || updated.Outline.Revision != 2 {
		t.Fatalf("postgres outline update failed: state=%+v err=%v", updated, err)
	}
	if _, err := restartedService.ApproveOutline(t.Context(), scope, jobID, 1, now.Add(62*time.Minute)); !errors.Is(err, ErrStaleOutlineRevision) {
		t.Fatalf("postgres stale approval error = %v", err)
	}
	approved, err := restartedService.ApproveOutline(t.Context(), scope, jobID, 2, now.Add(63*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if approved.Job.Status != GenerationJobQueued || approved.Job.Stage != GenerationStageOutlineApproved || approved.ApprovedOutline == nil || approved.ApprovedOutline.Revision != 2 {
		t.Fatalf("postgres approved state mismatch: %+v", approved)
	}
	var approvedAtBeforeReplay time.Time
	var revisionCountBeforeReplay, transitionCountBeforeReplay int
	if err := db.QueryRowContext(t.Context(), `select approved_at from xz_ppt_v2_outline_revisions where generation_job_id=$1 and revision=$2`, jobID, 2).Scan(&approvedAtBeforeReplay); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `select count(*) from xz_ppt_v2_outline_revisions where generation_job_id=$1`, jobID).Scan(&revisionCountBeforeReplay); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `select count(*) from xz_ppt_v2_generation_transitions where generation_job_id=$1 and to_stage=$2`, jobID, GenerationStageOutlineApproved).Scan(&transitionCountBeforeReplay); err != nil {
		t.Fatal(err)
	}
	replayed, err := restartedService.ApproveOutline(t.Context(), scope, jobID, 2, now.Add(64*time.Minute))
	if err != nil || !reflect.DeepEqual(replayed, approved) {
		t.Fatalf("postgres duplicate approval is not idempotent: state=%+v err=%v", replayed, err)
	}
	var approvedAtAfterReplay time.Time
	var revisionCountAfterReplay, transitionCountAfterReplay int
	if err := db.QueryRowContext(t.Context(), `select approved_at from xz_ppt_v2_outline_revisions where generation_job_id=$1 and revision=$2`, jobID, 2).Scan(&approvedAtAfterReplay); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `select count(*) from xz_ppt_v2_outline_revisions where generation_job_id=$1`, jobID).Scan(&revisionCountAfterReplay); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `select count(*) from xz_ppt_v2_generation_transitions where generation_job_id=$1 and to_stage=$2`, jobID, GenerationStageOutlineApproved).Scan(&transitionCountAfterReplay); err != nil {
		t.Fatal(err)
	}
	if !approvedAtAfterReplay.Equal(approvedAtBeforeReplay) || revisionCountAfterReplay != revisionCountBeforeReplay || transitionCountAfterReplay != transitionCountBeforeReplay {
		t.Fatalf("postgres duplicate approval mutated persistence: approvedAt=%s->%s revisions=%d->%d transitions=%d->%d", approvedAtBeforeReplay, approvedAtAfterReplay, revisionCountBeforeReplay, revisionCountAfterReplay, transitionCountBeforeReplay, transitionCountAfterReplay)
	}
	if _, err := restartedService.UpdateOutline(t.Context(), scope, jobID, 2, []OutlineEditCommand{{Type: OutlineCommandDeleteSlide, SlideID: approved.Outline.Slides[1].SlideID}}, now.Add(65*time.Minute)); !errors.Is(err, ErrOutlinePlanApproved) {
		t.Fatalf("postgres approved outline was mutable: %v", err)
	}
	var planCount, revisionCount, deckCount, slideCount int
	if err := db.QueryRowContext(t.Context(), `select count(*) from xz_ppt_v2_agent_plans where generation_job_id=$1`, jobID).Scan(&planCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `select count(*) from xz_ppt_v2_outline_revisions where generation_job_id=$1`, jobID).Scan(&revisionCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `select count(*) from xz_ppt_v2_deck_jobs where generation_job_id=$1`, jobID).Scan(&deckCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `select count(*) from xz_ppt_v2_slide_jobs where generation_job_id=$1`, jobID).Scan(&slideCount); err != nil {
		t.Fatal(err)
	}
	if planCount != 1 || revisionCount != 2 || deckCount != 0 || slideCount != 0 {
		t.Fatalf("Slice A postgres persistence mismatch: plans=%d revisions=%d decks=%d slides=%d", planCount, revisionCount, deckCount, slideCount)
	}
}

func TestPostgresGenerationJobAgentPlanningWorkerRecoveryRetryAndFencing(t *testing.T) {
	db := phase2PostgresDatabase(t)
	applyPhase2GenerationMigration(t, db)
	now := time.Now().UTC()
	suffix := now.Format("20060102150405.000000000")
	store, err := NewPostgresGenerationJobStore(db)
	if err != nil {
		t.Fatal(err)
	}
	research := &countingResearchProvider{pack: agentResearchFixture(t)}
	firstPlanning := &semanticPlanningFixture{storylineErrs: []error{
		NewAgentWorkflowError(PlanningProviderUnavailable, "Planning service is temporarily unavailable.", true, errors.New("fixture outage")),
	}}
	firstService, err := NewAgentPlanningService(store, research, firstPlanning, firstPlanning, AgentPlanningOptions{
		WorkerID: "postgres_a1_first", LeaseDuration: time.Minute, RetryDelay: time.Nanosecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := GuideAgentRequest{
		TenantID: "pptv2_a1_tenant_" + suffix, UserID: "pptv2_a1_user_" + suffix,
		OrganizationID: "pptv2_a1_org_" + suffix, IdempotencyKey: "pptv2_a1_guide_" + suffix,
		Request: IntentRequest{Text: "Create a 10-page electric vehicle industry analysis for company management.", PageCount: 10, Language: "en"}, Now: now,
	}
	created, err := firstService.Guide(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	jobID := created.State.Job.ID
	jobIDs := []string{jobID}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, cleanupJobID := range jobIDs {
			_, _ = db.ExecContext(ctx, `delete from xz_ppt_v2_generation_jobs where id=$1`, cleanupJobID)
		}
	})
	if created.State.Job.Stage != GenerationStageCreated || created.State.Job.Status != GenerationJobQueued {
		t.Fatalf("guide did not persist CREATED: %+v", created.State.Job)
	}
	if err := firstService.ProcessReady(t.Context(), now.Add(time.Second), 10); err != nil {
		t.Fatal(err)
	}
	scope := GenerationJobScope{TenantID: request.TenantID, UserID: request.UserID}
	afterStorylineFailure, err := firstService.Get(t.Context(), scope, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if afterStorylineFailure.Job.Stage != GenerationStageResearched || afterStorylineFailure.Job.Status != GenerationJobRetryWait || afterStorylineFailure.Job.Error == nil || afterStorylineFailure.Job.Error.Code != PlanningProviderUnavailable {
		t.Fatalf("storyline failure was not durable: %+v", afterStorylineFailure.Job)
	}
	if research.calls != 1 || afterStorylineFailure.ResearchExecutionCount != 1 {
		t.Fatalf("research checkpoint mismatch: provider=%d persisted=%d", research.calls, afterStorylineFailure.ResearchExecutionCount)
	}

	restartedStore, err := NewPostgresGenerationJobStore(db)
	if err != nil {
		t.Fatal(err)
	}
	restartedResearch := &countingResearchProvider{pack: agentResearchFixture(t)}
	secondPlanning := &semanticPlanningFixture{outlineErrs: []error{
		NewAgentWorkflowError(PlanningInvalidOutput, "Planning provider returned an invalid response.", true, errors.New("invalid json")),
	}}
	secondService, err := NewAgentPlanningService(restartedStore, restartedResearch, secondPlanning, secondPlanning, AgentPlanningOptions{
		WorkerID: "postgres_a1_second", LeaseDuration: time.Minute, RetryDelay: time.Nanosecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := secondService.Retry(t.Context(), scope, jobID, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := secondService.ProcessReady(t.Context(), now.Add(3*time.Second), 10); err != nil {
		t.Fatal(err)
	}
	afterOutlineFailure, err := secondService.Get(t.Context(), scope, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if afterOutlineFailure.Job.Stage != GenerationStageStorylinePlanned || afterOutlineFailure.Job.Status != GenerationJobRetryWait || afterOutlineFailure.Job.Error == nil || afterOutlineFailure.Job.Error.Code != PlanningInvalidOutput {
		t.Fatalf("outline failure was not durable: %+v", afterOutlineFailure.Job)
	}
	if restartedResearch.calls != 0 || secondPlanning.storylineCalls != 1 || secondPlanning.outlineCalls != 1 {
		t.Fatalf("restart repeated checkpointed work: research=%d storyline=%d outline=%d", restartedResearch.calls, secondPlanning.storylineCalls, secondPlanning.outlineCalls)
	}

	finalStore, err := NewPostgresGenerationJobStore(db)
	if err != nil {
		t.Fatal(err)
	}
	finalResearch := &countingResearchProvider{pack: agentResearchFixture(t)}
	finalPlanning := &semanticPlanningFixture{}
	finalService, err := NewAgentPlanningService(finalStore, finalResearch, finalPlanning, finalPlanning, AgentPlanningOptions{
		WorkerID: "postgres_a1_final", LeaseDuration: time.Minute, RetryDelay: time.Nanosecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := finalService.Retry(t.Context(), scope, jobID, now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := finalService.ProcessReady(t.Context(), now.Add(5*time.Second), 10); err != nil {
		t.Fatal(err)
	}
	completed, err := finalService.Get(t.Context(), scope, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Job.Stage != GenerationStageOutlinePlanned || completed.Job.Status != GenerationJobWaitingForOutlineApproval {
		t.Fatalf("worker did not resume at outline checkpoint: %+v", completed.Job)
	}
	if finalResearch.calls != 0 || finalPlanning.storylineCalls != 0 || finalPlanning.outlineCalls != 1 || completed.ResearchExecutionCount != 1 {
		t.Fatalf("final restart repeated checkpointed work: research=%d storyline=%d outline=%d persistedResearch=%d", finalResearch.calls, finalPlanning.storylineCalls, finalPlanning.outlineCalls, completed.ResearchExecutionCount)
	}
	for _, stage := range []string{GenerationStageIntentResolved, GenerationStageResearched, GenerationStageStorylinePlanned, GenerationStageOutlinePlanned} {
		var count int
		if err := db.QueryRowContext(t.Context(), `select count(*) from xz_ppt_v2_generation_transitions where generation_job_id=$1 and to_stage=$2`, jobID, stage).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("durable stage %s transition count=%d", stage, count)
		}
	}

	staleRequest := request
	staleRequest.IdempotencyKey += "_fence"
	staleRequest.Request.Text = "Create a separate 10-page market analysis for management."
	staleRequest.Now = now.Add(10 * time.Second)
	staleCreated, err := finalService.Guide(t.Context(), staleRequest)
	if err != nil {
		t.Fatal(err)
	}
	jobIDs = append(jobIDs, staleCreated.State.Job.ID)
	staleScope := GenerationJobScope{TenantID: staleRequest.TenantID, UserID: staleRequest.UserID}
	firstLease, err := finalStore.Claim(t.Context(), staleScope, staleCreated.State.Job.ID, "stale_worker", now.Add(11*time.Second), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	secondLease, err := finalStore.Claim(t.Context(), staleScope, staleCreated.State.Job.ID, "replacement_worker", now.Add(13*time.Second), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := finalStore.SaveAgentIntent(t.Context(), firstLease, staleCreated.State.Intent, now.Add(13*time.Second)); !errors.Is(err, ErrGenerationJobLeaseLost) {
		t.Fatalf("stale worker output was not fenced: %v", err)
	}
	if _, err := finalStore.SaveAgentIntent(t.Context(), secondLease, staleCreated.State.Intent, now.Add(13*time.Second)); err != nil {
		t.Fatalf("replacement worker could not checkpoint: %v", err)
	}
}
