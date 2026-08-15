package ppt

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newGenerationJobFixture(t *testing.T, store GenerationJobStore, now time.Time, maxAttempts int) (GenerationJobScope, GenerationJob) {
	t.Helper()
	scope := GenerationJobScope{TenantID: "tenant_phase2", UserID: "user_phase2"}
	job, created, err := store.Create(context.Background(), CreateGenerationJobInput{
		JobID: "pptv2_job_test", TenantID: scope.TenantID, UserID: scope.UserID, OrganizationID: "org_phase2",
		ExistingTaskID: "ppt_task_phase2", ClientRequestID: "client_phase2", IdempotencyKey: "phase2-idempotency",
		MaxAttempts: maxAttempts, SlideCount: 2, Now: now,
	})
	if err != nil || !created {
		t.Fatalf("create job: created=%v job=%+v err=%v", created, job, err)
	}
	return scope, job
}

func claimGenerationJobFixture(t *testing.T, store GenerationJobStore, scope GenerationJobScope, jobID, worker string, now time.Time) GenerationLease {
	t.Helper()
	lease, err := store.Claim(context.Background(), scope, jobID, worker, now, time.Minute)
	if err != nil {
		t.Fatalf("claim job: %v", err)
	}
	return lease
}

func checkpointGenerationStage(t *testing.T, store GenerationJobStore, lease GenerationLease, stage string, now time.Time) GenerationJob {
	t.Helper()
	checkpoint := GenerationCheckpoint{NextStage: stage, Now: now}
	switch stage {
	case GenerationStageTaskLoaded:
		checkpoint.InputSnapshot = []byte(`{"taskContext":{"taskId":"ppt_task_phase2"}}`)
		checkpoint.SourceSlideIDs = []string{"slide_1", "slide_2"}
	case GenerationStageRendered:
		checkpoint.DeckID = "deck_phase2"
		checkpoint.Revision = 1
		checkpoint.SlideCount = 2
		checkpoint.RenderSHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		checkpoint.RenderBytes = []byte("PK-pptx")
	case GenerationStageFileStored:
		checkpoint.FileID = "file_phase2"
	case GenerationStageAssetCreated:
		checkpoint.AssetID = "asset_phase2"
	}
	job, err := store.Checkpoint(context.Background(), lease, checkpoint)
	if err != nil {
		t.Fatalf("checkpoint %s: %v", stage, err)
	}
	return job
}

func TestGenerationJobCreationIsIdempotentAndIdentityIsStable(t *testing.T) {
	now := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	store := NewMemoryGenerationJobStore()
	scope, first := newGenerationJobFixture(t, store, now, 3)
	replayed, created, err := store.Create(t.Context(), CreateGenerationJobInput{
		JobID: "ignored_replay_id", TenantID: scope.TenantID, UserID: scope.UserID, OrganizationID: "org_phase2",
		ExistingTaskID: first.ExistingTaskID, ClientRequestID: first.ClientRequestID, IdempotencyKey: first.IdempotencyKey,
		MaxAttempts: 3, SlideCount: 2, Now: now.Add(time.Hour),
	})
	if err != nil || created || replayed.ID != first.ID {
		t.Fatalf("idempotent replay: created=%v first=%+v replayed=%+v err=%v", created, first, replayed, err)
	}
	bundle, err := store.Get(t.Context(), scope, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Deck.ID != first.ID+":deck" || len(bundle.Slides) != 2 || bundle.Slides[0].ID != first.ID+":slide:1" || bundle.Slides[1].ID != first.ID+":slide:2" {
		t.Fatalf("unstable DeckJob/SlideJob identity: %+v", bundle)
	}
	_, _, err = store.Create(t.Context(), CreateGenerationJobInput{
		TenantID: scope.TenantID, UserID: scope.UserID, OrganizationID: "org_phase2", ExistingTaskID: "other_task",
		IdempotencyKey: first.IdempotencyKey, MaxAttempts: 3, SlideCount: 2, Now: now,
	})
	if !errors.Is(err, ErrGenerationJobIdempotencyConflict) {
		t.Fatalf("conflicting idempotency error = %v", err)
	}
}

func TestGenerationJobTransitionsProgressAndTerminalCannotReopen(t *testing.T) {
	now := time.Date(2026, 8, 15, 11, 0, 0, 0, time.UTC)
	store := NewMemoryGenerationJobStore()
	scope, job := newGenerationJobFixture(t, store, now, 3)
	lease := claimGenerationJobFixture(t, store, scope, job.ID, "worker_a", now)
	if _, err := store.Checkpoint(t.Context(), lease, GenerationCheckpoint{NextStage: GenerationStageRendered, Now: now.Add(time.Second)}); !errors.Is(err, ErrGenerationJobTransition) {
		t.Fatalf("invalid transition error = %v", err)
	}
	stages := []string{GenerationStageTaskLoaded, GenerationStageRendered, GenerationStageFileStored, GenerationStageAssetCreated, GenerationStageTaskRelated, GenerationStageCompleted}
	for index, stage := range stages {
		current := checkpointGenerationStage(t, store, lease, stage, now.Add(time.Duration(index+1)*time.Second))
		expectedUnits := generationStageWorkUnits(stage)
		if current.CompletedWorkUnits != expectedUnits || current.Progress() != expectedUnits*100/GenerationTotalWorkUnits {
			t.Fatalf("stage %s uses fake/incorrect progress: job=%+v", stage, current)
		}
	}
	bundle, err := store.Get(t.Context(), scope, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Job.Status != GenerationJobSucceeded || bundle.Job.Progress() != 100 || len(bundle.History) != 7 {
		t.Fatalf("unexpected terminal bundle: %+v", bundle)
	}
	if _, err := store.Claim(t.Context(), scope, job.ID, "worker_b", now.Add(time.Hour), time.Minute); !errors.Is(err, ErrGenerationJobTerminal) {
		t.Fatalf("terminal reopen error = %v", err)
	}
}

func TestGenerationJobLeaseRenewExpireReclaimAndStaleFenceRejected(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	store := NewMemoryGenerationJobStore()
	scope, job := newGenerationJobFixture(t, store, now, 3)
	first, err := store.Claim(t.Context(), scope, job.ID, "worker_old", now, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(t.Context(), scope, job.ID, "worker_new", now.Add(time.Second), time.Minute); !errors.Is(err, ErrGenerationJobLeaseHeld) {
		t.Fatalf("concurrent claim error = %v", err)
	}
	renewed, err := store.Renew(t.Context(), first, now.Add(5*time.Second), 10*time.Second)
	if err != nil || !renewed.LeaseExpiresAt.Equal(now.Add(15*time.Second)) {
		t.Fatalf("renewed lease=%+v err=%v", renewed, err)
	}
	second, err := store.Claim(t.Context(), scope, job.ID, "worker_new", now.Add(16*time.Second), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if second.FencingToken <= first.FencingToken || second.AttemptID == first.AttemptID {
		t.Fatalf("reclaim did not advance fence/attempt: old=%+v new=%+v", first, second)
	}
	bundle, err := store.Get(t.Context(), scope, job.ID)
	if err != nil || len(bundle.Attempts) != 2 || bundle.Attempts[0].Status != GenerationAttemptRetryWait || bundle.Attempts[0].Error == nil || bundle.Attempts[0].Error.Code != "LEASE_EXPIRED" {
		t.Fatalf("expired attempt was not closed durably: bundle=%+v err=%v", bundle, err)
	}
	if _, err := store.Checkpoint(t.Context(), first, GenerationCheckpoint{NextStage: GenerationStageTaskLoaded, InputSnapshot: []byte(`{}`), Now: now.Add(17 * time.Second)}); !errors.Is(err, ErrGenerationJobLeaseLost) {
		t.Fatalf("stale fence write error = %v", err)
	}
	checkpointGenerationStage(t, store, second, GenerationStageTaskLoaded, now.Add(17*time.Second))
}

func TestGenerationJobRetryPolicyFailFastAndMaxAttempts(t *testing.T) {
	now := time.Date(2026, 8, 15, 13, 0, 0, 0, time.UTC)
	store := NewMemoryGenerationJobStore()
	scope, job := newGenerationJobFixture(t, store, now, 2)
	first := claimGenerationJobFixture(t, store, scope, job.ID, "worker_1", now)
	retrying, err := store.Fail(t.Context(), first, GenerationJobError{Code: "RENDER_TEMPORARY", Message: "renderer unavailable", Retryable: true}, now.Add(time.Second), time.Minute)
	if err != nil || retrying.Status != GenerationJobRetryWait || retrying.AttemptCount != 1 {
		t.Fatalf("retryable failure: job=%+v err=%v", retrying, err)
	}
	if _, err := store.Claim(t.Context(), scope, job.ID, "worker_2", now.Add(30*time.Second), time.Minute); !errors.Is(err, ErrGenerationJobNotReady) {
		t.Fatalf("early retry claim error = %v", err)
	}
	second := claimGenerationJobFixture(t, store, scope, job.ID, "worker_2", now.Add(2*time.Minute))
	failed, err := store.Fail(t.Context(), second, GenerationJobError{Code: "RENDER_TEMPORARY", Message: "still unavailable", Retryable: true}, now.Add(2*time.Minute+time.Second), time.Minute)
	if err != nil || failed.Status != GenerationJobFailed || !failed.Terminal() || failed.LastError == nil || failed.LastError.AttemptID != second.AttemptID {
		t.Fatalf("max-attempt failure: job=%+v err=%v", failed, err)
	}

	otherStore := NewMemoryGenerationJobStore()
	otherScope, other := newGenerationJobFixture(t, otherStore, now, 3)
	lease := claimGenerationJobFixture(t, otherStore, otherScope, other.ID, "worker", now)
	failFast, err := otherStore.Fail(t.Context(), lease, GenerationJobError{Code: "INVALID_INPUT", Message: "invalid task", Retryable: false}, now.Add(time.Second), time.Minute)
	if err != nil || failFast.Status != GenerationJobFailed || failFast.AttemptCount != 1 {
		t.Fatalf("non-retryable failure: job=%+v err=%v", failFast, err)
	}
}

func TestGenerationJobRestartRecoveryCancelAndTenantIsolation(t *testing.T) {
	now := time.Date(2026, 8, 15, 14, 0, 0, 0, time.UTC)
	store := NewMemoryGenerationJobStore()
	scope, job := newGenerationJobFixture(t, store, now, 3)
	first := claimGenerationJobFixture(t, store, scope, job.ID, "process_before_restart", now)
	checkpointGenerationStage(t, store, first, GenerationStageTaskLoaded, now.Add(time.Second))

	resumed, err := store.Claim(t.Context(), scope, job.ID, "process_after_restart", now.Add(2*time.Minute), time.Minute)
	if err != nil || resumed.Job.Stage != GenerationStageTaskLoaded || string(resumed.Job.InputSnapshot) == "" {
		t.Fatalf("restart recovery lost checkpoint: lease=%+v err=%v", resumed, err)
	}
	cancelled, err := store.Cancel(t.Context(), scope, job.ID, now.Add(2*time.Minute+time.Second))
	if err != nil || cancelled.Status != GenerationJobCancelled || cancelled.CancelRequestedAt.IsZero() {
		t.Fatalf("cancel result=%+v err=%v", cancelled, err)
	}
	if _, err := store.Checkpoint(t.Context(), resumed, GenerationCheckpoint{
		NextStage: GenerationStageRendered, DeckID: "deck", Revision: 1, SlideCount: 2,
		RenderSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", RenderBytes: []byte("PK"),
		Now: now.Add(2*time.Minute + 2*time.Second),
	}); !errors.Is(err, ErrGenerationJobCancelled) {
		t.Fatalf("cancelled job accepted later stage: %v", err)
	}
	if _, err := store.Get(t.Context(), GenerationJobScope{TenantID: "other_tenant", UserID: scope.UserID}, job.ID); !errors.Is(err, ErrGenerationJobNotFound) {
		t.Fatalf("cross-tenant read error = %v", err)
	}
	if _, err := store.Cancel(t.Context(), GenerationJobScope{TenantID: scope.TenantID, UserID: "other_user"}, job.ID, now); !errors.Is(err, ErrGenerationJobNotFound) {
		t.Fatalf("cross-owner cancel error = %v", err)
	}
}
