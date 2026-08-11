package smartvideo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresMontage_VersionImmutableAndOutboxIdempotent(t *testing.T) {
	dsn := os.Getenv("SMARTVIDEO_MIGRATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SMARTVIDEO_MIGRATION_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	projectID := "svp_mtg_" + suffix
	versionID := "svv_mtg_" + suffix
	taskID := "svplan_" + suffix
	access := Access{TenantID: "tenant_mtg", UserID: "user_mtg"}
	defer func() {
		_, _ = db.ExecContext(context.Background(), `delete from video_task_outbox where aggregate_id=$1`, taskID)
		_, _ = db.ExecContext(context.Background(), `delete from video_plan_tasks where project_id=$1`, projectID)
		_, _ = db.ExecContext(context.Background(), `delete from video_project_versions where project_id=$1`, projectID)
		_, _ = db.ExecContext(context.Background(), `delete from video_projects where id=$1`, projectID)
	}()

	repository := NewPostgresRepository(db)
	now := time.Now().UTC()
	if _, err := repository.CreateProject(ctx, Project{
		ID: projectID, TenantID: access.TenantID, UserID: access.UserID,
		Title: "Montage", Requirement: "一句话需求", Status: ProjectStatusMaterialReady,
		TargetSpec: TargetSpec{AspectRatio: TargetAspectRatio9x16, Resolution: TargetResolution720p, DurationMs: 15000},
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	plan := makeValidEditPlanV1()
	version := ProjectVersion{
		ID: versionID, ProjectID: projectID, TenantID: access.TenantID, VersionNumber: 1,
		Source: VersionSourceAI, PlanSchemaVersion: EditPlanSchemaVersion, PlanSnapshot: plan,
		PlannerModelKey: "smart-video-standard", PlannerRequestID: "req_" + suffix,
		CreatedBy: access.UserID, CreatedAt: now,
	}
	if _, err := repository.CreateImmutableVersion(ctx, version); err != nil {
		t.Fatal(err)
	}
	got, err := repository.GetVersion(ctx, access, projectID, versionID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PlanSnapshot.Title != plan.Title {
		t.Fatalf("plan snapshot title = %q, want %q", got.PlanSnapshot.Title, plan.Title)
	}
	if _, err := repository.GetVersion(ctx, Access{TenantID: "other", UserID: access.UserID}, projectID, versionID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant GetVersion error = %v, want ErrNotFound", err)
	}

	manifest := RenderManifestV1{
		SchemaVersion: 1,
		Output: ManifestOutputSpec{
			Width: 720, Height: 1280, FrameRate: 30,
			VideoCodec: "h264", AudioCodec: "aac", PixelFormat: "yuv420p", Format: "mp4",
		},
		ManifestHash: "hash_a",
	}
	confirmed, err := repository.AttachRenderManifest(ctx, access, projectID, versionID, manifest, "hash_a")
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.ManifestHash != "hash_a" || confirmed.RenderManifest == nil {
		t.Fatalf("unexpected confirmed version: %+v", confirmed)
	}
	if _, err := repository.AttachRenderManifest(ctx, access, projectID, versionID, manifest, "hash_b"); !errors.Is(err, ErrVersionImmutable) {
		t.Fatalf("overwrite manifest error = %v, want ErrVersionImmutable", err)
	}

	payload, _ := json.Marshal(map[string]string{"taskId": taskID})
	task := PlanTask{
		ID: taskID, TenantID: access.TenantID, ProjectID: projectID, UserID: access.UserID,
		State: PlanStatusCreated, Instruction: "再短一点", ModelKey: "smart-video-standard",
		Attempt: 1, IdempotencyKey: "idem_" + suffix, CreatedAt: now,
	}
	outbox := OutboxEvent{
		TenantID: access.TenantID, AggregateType: "plan", AggregateID: taskID,
		EventType: "enqueue_requested", Payload: payload,
	}
	if err := repository.CreatePlanTaskWithOutbox(ctx, task, outbox); err != nil {
		t.Fatal(err)
	}
	if err := repository.CreatePlanTaskWithOutbox(ctx, task, outbox); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("duplicate plan task error = %v, want ErrIdempotencyConflict", err)
	}

	events, err := repository.PublishOutbox(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range events {
		if event.AggregateID == taskID && event.AggregateType == "plan" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected outbox event to be published")
	}

	if err := repository.MarkAnalysisQueued(ctx, "missing"); err == nil {
		// no-op guard: ensure package still compiles against analysis helpers
	}

	claimed, err := repository.ClaimPlanTask(ctx, taskID, "worker_a", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.State != PlanStatusProcessing {
		t.Fatalf("claimed state = %s", claimed.State)
	}
	if _, err := repository.ClaimPlanTask(ctx, taskID, "worker_b", time.Minute); !errors.Is(err, ErrNotFound) {
		t.Fatalf("double claim error = %v, want ErrNotFound", err)
	}

	nextVersion := ProjectVersion{
		ID: "svv_mtg2_" + suffix, ProjectID: projectID, TenantID: access.TenantID, VersionNumber: 2,
		Source: VersionSourceAI, PlanSchemaVersion: EditPlanSchemaVersion, PlanSnapshot: plan,
		PlannerModelKey: "smart-video-standard", PlannerRequestID: "req2_" + suffix,
		CreatedBy: access.UserID, CreatedAt: now,
	}
	if err := repository.CompletePlanTask(ctx, taskID, "worker_a", nextVersion); err != nil {
		t.Fatal(err)
	}
	project, err := repository.GetProject(ctx, access, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if project.Status != ProjectStatusStoryboardReady || project.CurrentVersionID != nextVersion.ID {
		t.Fatalf("unexpected project after plan complete: %+v", project)
	}
}

func TestPostgresMontage_CompleteRenderDoesNotWriteAssets(t *testing.T) {
	dsn := os.Getenv("SMARTVIDEO_MIGRATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SMARTVIDEO_MIGRATION_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	projectID := "svp_rnd_" + suffix
	taskID := "svrender_" + suffix
	fileID := "file_rnd_" + suffix
	access := Access{TenantID: "tenant_rnd", UserID: "user_rnd"}
	defer func() {
		_, _ = db.ExecContext(context.Background(), `delete from video_render_tasks where id=$1`, taskID)
		_, _ = db.ExecContext(context.Background(), `delete from video_projects where id=$1`, projectID)
		_, _ = db.ExecContext(context.Background(), `delete from xz_assets where id=$1`, "asset_"+taskID)
		_, _ = db.ExecContext(context.Background(), `delete from xz_file_objects where file_id=$1`, fileID)
	}()

	if _, err := db.ExecContext(ctx, `insert into xz_file_objects(file_id) values($1)`, fileID); err != nil {
		t.Fatalf("file fixture: %v", err)
	}
	repository := NewPostgresRepository(db)
	now := time.Now().UTC()
	if _, err := repository.CreateProject(ctx, Project{
		ID: projectID, TenantID: access.TenantID, UserID: access.UserID,
		Title: "Render", Requirement: "export", Status: ProjectStatusConfirmed,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	spec, _ := json.Marshal(RenderSpecification{Width: 720, Height: 1280, FrameRate: 30, Format: "mp4", VideoCodec: "h264", AudioCodec: "aac", DurationMS: 15000})
	_, err = db.ExecContext(ctx, `insert into video_render_tasks
		(id,project_id,tenant_id,user_id,client_request_id,status,progress,step,stage,attempt_count,max_attempts,run_after,specification,
		 quoted_tokens,reserved_tokens,captured_tokens,released_tokens,quoted_points,reserved_points,captured_points,released_points,
		 attempt,created_at,updated_at,lease_owner,lease_expires_at)
		values($1,$2,$3,$4,$5,'UPLOADING',80,'uploading','uploading',1,3,now(),$6::jsonb,
		 0,0,0,0,100,100,0,0,1,now(),now(),'worker_r',now()+interval '5 minutes')`,
		taskID, projectID, access.TenantID, access.UserID, "client_"+suffix, spec)
	if err != nil {
		t.Fatal(err)
	}

	completed, err := repository.CompleteRenderTask(ctx, taskID, "worker_r", RenderOutput{
		VideoFileID: fileID, CoverFileID: fileID, DurationMS: 15000, Width: 720, Height: 1280, FrameRate: 30,
		VideoCodec: "h264", AudioCodec: "aac", PixelFormat: "yuv420p",
	})
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != RenderStatusSucceeded || completed.OutputFileID != fileID {
		t.Fatalf("unexpected completed task: %+v", completed)
	}
	if completed.OutputAssetID != "" {
		t.Fatalf("repository must not invent output asset id, got %q", completed.OutputAssetID)
	}
	var assetCount int
	if err := db.QueryRowContext(ctx, `select count(*) from xz_assets where id=$1 or task_id=$2`, "asset_"+taskID, taskID).Scan(&assetCount); err != nil {
		t.Fatal(err)
	}
	if assetCount != 0 {
		t.Fatalf("xz_assets rows = %d, want 0", assetCount)
	}
}
