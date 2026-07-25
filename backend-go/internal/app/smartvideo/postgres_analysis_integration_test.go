package smartvideo

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresAnalysisRepositoryIsolationIdempotencyAndLease(t *testing.T) {
	dsn := os.Getenv("SMARTVIDEO_MIGRATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SMARTVIDEO_MIGRATION_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	projectID, assetID, fileID := "svp_pg_"+suffix, "sva_pg_"+suffix, "file_pg_"+suffix
	defer func() {
		_, _ = db.ExecContext(context.Background(), `delete from video_asset_analysis_tasks where project_id=$1`, projectID)
		_, _ = db.ExecContext(context.Background(), `delete from video_project_assets where project_id=$1`, projectID)
		_, _ = db.ExecContext(context.Background(), `delete from video_projects where id=$1`, projectID)
		_, _ = db.ExecContext(context.Background(), `delete from xz_file_objects where file_id=$1`, fileID)
	}()
	if _, err := db.ExecContext(ctx, `insert into xz_file_objects(file_id) values($1)`, fileID); err != nil {
		t.Fatalf("insert isolated file fixture: %v", err)
	}

	repository := NewPostgresRepository(db)
	now := time.Now().UTC()
	access := Access{TenantID: "tenant_pg", UserID: "user_pg"}
	if _, err := repository.CreateProject(ctx, Project{
		ID: projectID, TenantID: access.TenantID, UserID: access.UserID,
		Title: "Postgres analysis", Status: ProjectStatusDraft, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	asset, err := repository.CreateAsset(ctx, ProjectAsset{
		ID: assetID, ProjectID: projectID, TenantID: access.TenantID, UserID: access.UserID,
		FileID: fileID, StorageKey: "tenants/tenant_pg/source.mp4", AssetType: AssetTypeVideo,
		Metadata: AssetMetadata{MIMEType: "video/mp4", FileSize: 128}, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.GetAsset(ctx, Access{TenantID: "other", UserID: access.UserID}, projectID, assetID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant GetAsset error = %v, want ErrNotFound", err)
	}

	first, err := repository.EnsureAnalysisTask(ctx, access, asset, "sha256:test", "request_1", 2)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.EnsureAnalysisTask(ctx, access, asset, "sha256:test", "request_1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("idempotent task IDs differ: %s != %s", first.ID, second.ID)
	}
	if err := repository.MarkAnalysisQueued(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	acquired, _, err := repository.AcquireAnalysisTask(ctx, first.ID, "worker_1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if acquired.Status != AnalysisStatusRunning || acquired.AttemptCount != 1 {
		t.Fatalf("unexpected acquired task: %+v", acquired)
	}
	if _, _, err := repository.AcquireAnalysisTask(ctx, first.ID, "worker_2", time.Minute); !errors.Is(err, ErrAnalysisNotReady) {
		t.Fatalf("concurrent acquire error = %v, want ErrAnalysisNotReady", err)
	}
	if err := repository.FailAnalysisTask(ctx, first.ID, "worker_1", MediaErrorProbeFailed, "媒体探测失败", now, true); err != nil {
		t.Fatal(err)
	}
	retried, err := repository.RetryAnalysisTask(ctx, access, projectID, assetID)
	if err != nil {
		t.Fatal(err)
	}
	if retried.Status != AnalysisStatusPending || retried.AttemptCount != 0 {
		t.Fatalf("unexpected retried task: %+v", retried)
	}
}
