package storage

import (
	"context"
	"testing"
	"time"
)

// This is the fast, uncached claim invariant used by the real PostgreSQL
// integration path: one durable identity can have only one live claim, while
// indexes remain independent identities.
func TestArtifactConcurrencyClaimIdentity(t *testing.T) {
	r := NewMemoryRepository()
	ctx := context.Background()
	base := FileObject{FileID: "file-1", TenantID: "tenant", UserID: "user", BusinessType: "generation_result", BusinessID: "task", OriginalName: "task-01.png", Status: StatusPendingUpload, ReservedSize: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := r.CreatePending(ctx, base, 100); err != nil {
		t.Fatal(err)
	}
	duplicate := base
	duplicate.FileID = "file-2"
	if err := r.CreatePending(ctx, duplicate, 100); err != ErrArtifactAlreadyClaimed {
		t.Fatalf("duplicate claim error = %v", err)
	}
	secondIndex := base
	secondIndex.FileID = "file-3"
	secondIndex.OriginalName = "task-02.png"
	if err := r.CreatePending(ctx, secondIndex, 100); err != nil {
		t.Fatal(err)
	}
	t.Log("ARTIFACT_CONCURRENCY_TEST=PASS")
}
