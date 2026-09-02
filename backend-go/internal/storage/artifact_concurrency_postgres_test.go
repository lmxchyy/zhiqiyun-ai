package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// This is intentionally an uncached, broker-independent PostgreSQL test. It
// exercises the production partial unique index and the quota row lock.
func TestArtifactConcurrencyClaimIdentityPostgres(t *testing.T) {
	dsn := os.Getenv("XIANZHI_STORAGE_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("XIANZHI_TEST_DATABASE_URL")
	}
	if dsn == "" {
		if os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") == "true" {
			t.Fatal("XIANZHI_STORAGE_TEST_DATABASE_URL is required in CI")
		}
		t.Skip("XIANZHI_STORAGE_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	r := NewPostgresRepository(db)
	tenant := fmt.Sprintf("artifact-concurrency-%d", time.Now().UnixNano())
	businessID := "task-replay"
	defer db.ExecContext(context.Background(), "delete from xz_file_objects where tenant_id=$1", tenant)
	defer db.ExecContext(context.Background(), "delete from xz_tenant_storage_quotas where tenant_id=$1", tenant)

	base := FileObject{TenantID: tenant, UserID: "test-user", StorageConfigID: "test-config", Provider: "minio", Bucket: "test", ObjectKey: "tenants/test/artifacts/replay.png", OriginalName: "task-01.png", FileHash: "sha256-test", HashAlgorithm: "sha256", BusinessType: "generation_result", BusinessID: businessID, Visibility: "PRIVATE", Status: StatusPendingUpload, ReservedSize: 4, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	start := make(chan struct{})
	errs := make(chan error, 4)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			claim := base
			claim.FileID = fmt.Sprintf("file-%d", i)
			errs <- r.CreatePending(ctx, claim, 1<<20)
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	var claimed, duplicates int
	for err := range errs {
		if err == nil {
			claimed++
		} else if err == ErrArtifactAlreadyClaimed {
			duplicates++
		} else {
			t.Errorf("claim: %v", err)
		}
	}
	if claimed != 1 || duplicates != 3 {
		t.Fatalf("claims=%d duplicates=%d", claimed, duplicates)
	}
	var count int
	if err := db.QueryRowContext(ctx, "select count(*) from xz_file_objects where tenant_id=$1 and business_id=$2 and original_name=$3", tenant, businessID, base.OriginalName).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("logical artifact rows=%d", count)
	}
	for i := 0; i < 3; i++ {
		duplicate := base
		duplicate.FileID = fmt.Sprintf("replay-%d", i)
		if err := r.CreatePending(ctx, duplicate, 1<<20); err != ErrArtifactAlreadyClaimed {
			t.Fatalf("replay %d error=%v", i, err)
		}
	}
	t.Log("POSTGRES_ARTIFACT_CONCURRENCY=PASS")
}
