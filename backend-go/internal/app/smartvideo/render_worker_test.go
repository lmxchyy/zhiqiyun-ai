package smartvideo

import (
	"context"
	"sync"
	"testing"
	"time"
)

type memoryRenderRepo struct {
	mu    sync.Mutex
	tasks map[string]RenderTask
}

func newMemoryRenderRepo(task RenderTask) *memoryRenderRepo {
	return &memoryRenderRepo{tasks: map[string]RenderTask{task.ID: task}}
}

func (r *memoryRenderRepo) GetRenderTask(_ context.Context, access Access, projectID, taskID string) (RenderTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[taskID]
	if !ok || task.ProjectID != projectID || task.TenantID != access.TenantID || task.UserID != access.UserID {
		return RenderTask{}, ErrNotFound
	}
	return task, nil
}
func (r *memoryRenderRepo) MarkRenderQueued(context.Context, string) error { return nil }
func (r *memoryRenderRepo) AcquireRenderTask(_ context.Context, taskID, workerID string, _ time.Duration) (RenderTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[taskID]
	if !ok {
		return RenderTask{}, ErrNotFound
	}
	task.Status = RenderStatusProcessing
	task.AttemptCount++
	task.Attempt++
	r.tasks[taskID] = task
	_ = workerID
	return task, nil
}
func (r *memoryRenderRepo) HeartbeatRenderTask(context.Context, string, string, time.Duration) error {
	return nil
}
func (r *memoryRenderRepo) AdvanceRenderTask(_ context.Context, taskID, _, from, to, stage string, progress int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[taskID]
	if !ok || task.Status != from {
		return ErrInvalidStateTransition
	}
	if err := ValidateRenderTransition(from, to); err != nil {
		return err
	}
	task.Status, task.Stage, task.Progress = to, stage, progress
	r.tasks[taskID] = task
	return nil
}
func (r *memoryRenderRepo) AttachVoiceCaptionArtifacts(_ context.Context, taskID, _, voiceFileID, captionFileID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[taskID]
	if !ok {
		return ErrNotFound
	}
	task.VoiceFileID, task.CaptionFileID = voiceFileID, captionFileID
	r.tasks[taskID] = task
	return nil
}
func (r *memoryRenderRepo) CompleteRenderTask(_ context.Context, taskID, _ string, output RenderOutput) (RenderTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task := r.tasks[taskID]
	task.Status = RenderStatusSucceeded
	task.Output = output
	task.OutputFileID = firstNonEmpty(output.VideoFileID, task.OutputFileID)
	task.CoverFileID = firstNonEmpty(output.CoverFileID, task.CoverFileID)
	r.tasks[taskID] = task
	return task, nil
}
func (r *memoryRenderRepo) PersistRenderOutput(_ context.Context, taskID, _ string, output RenderOutput) (RenderTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task := r.tasks[taskID]
	task.Status = RenderStatusPublishing
	task.OutputFileID = output.VideoFileID
	task.CoverFileID = output.CoverFileID
	task.Output = output
	r.tasks[taskID] = task
	return task, nil
}
func (r *memoryRenderRepo) MarkPointsCaptured(_ context.Context, taskID string, points int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	task := r.tasks[taskID]
	task.CapturedPoints = points
	r.tasks[taskID] = task
	return nil
}
func (r *memoryRenderRepo) MarkPointsReleased(_ context.Context, taskID string, points int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	task := r.tasks[taskID]
	task.ReleasedPoints += points
	r.tasks[taskID] = task
	return nil
}
func (r *memoryRenderRepo) MarkRenderWorkPublished(_ context.Context, taskID, _, workID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	task := r.tasks[taskID]
	task.WorkID = workID
	task.OutputAssetID = workID
	r.tasks[taskID] = task
	return nil
}
func (r *memoryRenderRepo) FailRenderTask(_ context.Context, taskID, _, code, message string, _ time.Time, _ bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	task := r.tasks[taskID]
	task.Status = RenderStatusFailed
	task.ErrorCode, task.ErrorMessage = code, message
	r.tasks[taskID] = task
	return nil
}
func (r *memoryRenderRepo) RetryRenderTask(context.Context, Access, string, string) (RenderTask, error) {
	return RenderTask{}, ErrNotFound
}

type stubRenderQueue struct{}

func (stubRenderQueue) Enqueue(context.Context, RenderJob, time.Duration) error { return nil }
func (stubRenderQueue) Run(context.Context, func(context.Context, RenderJob) QueueDecision) error {
	return nil
}

type stubRenderer struct{}

func (stubRenderer) Render(context.Context, RenderTask, string) (RenderArtifact, error) {
	return RenderArtifact{VideoPath: "out.mp4", DurationMS: 1000, Width: 1080, Height: 1920, FrameRate: 30}, nil
}

type stubPublisher struct{}

func (stubPublisher) Publish(context.Context, RenderTask, RenderArtifact) (RenderOutput, error) {
	return RenderOutput{VideoFileID: "file_video", CoverFileID: "file_cover"}, nil
}

func TestRenderWorkerSpeechReuseOnRetry(t *testing.T) {
	repo := NewMemoryRepository()
	access, baseTask := seedSpeechPlan(t, repo, true)
	baseTask.Status = RenderStatusQueued
	baseTask.VoiceFileID = "voice_reuse"
	baseTask.CaptionFileID = "caption_reuse"
	renderRepo := newMemoryRenderRepo(baseTask)
	builder := &countingBuilder{out: VoiceCaptionArtifacts{VoiceFileID: "new_voice", CaptionFileID: "new_cap"}}
	speech := NewSpeechPrepService(builder, repo)
	worker := NewRenderWorker(renderRepo, stubRenderQueue{}, stubRenderer{}, stubPublisher{}, RenderWorkerOptions{
		WorkerID: "worker_1", TempDir: t.TempDir(), LeaseDuration: time.Minute, TaskTimeout: time.Minute,
	}).SetSpeechPrep(speech)

	decision := worker.handle(context.Background(), RenderJob{TaskID: baseTask.ID})
	if decision.Dead || decision.RetryAfter > 0 {
		t.Fatalf("unexpected decision: %+v", decision)
	}
	if builder.calls != 0 {
		t.Fatalf("retry must reuse voice/caption artifacts, builder calls=%d", builder.calls)
	}
	final, err := renderRepo.GetRenderTask(context.Background(), access, baseTask.ProjectID, baseTask.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if final.Status != RenderStatusSucceeded || final.VoiceFileID != "voice_reuse" {
		t.Fatalf("unexpected final task: %+v", final)
	}
}

func TestRenderWorkerSynthesizesThenRenders(t *testing.T) {
	repo := NewMemoryRepository()
	_, baseTask := seedSpeechPlan(t, repo, true)
	baseTask.Status = RenderStatusQueued
	renderRepo := newMemoryRenderRepo(baseTask)
	builder := &countingBuilder{out: VoiceCaptionArtifacts{VoiceFileID: "voice_new", CaptionFileID: "cap_new"}}
	speech := NewSpeechPrepService(builder, repo)
	worker := NewRenderWorker(renderRepo, stubRenderQueue{}, stubRenderer{}, stubPublisher{}, RenderWorkerOptions{
		WorkerID: "worker_1", TempDir: t.TempDir(), LeaseDuration: time.Minute, TaskTimeout: time.Minute,
	}).SetSpeechPrep(speech)

	decision := worker.handle(context.Background(), RenderJob{TaskID: baseTask.ID})
	if decision.Dead || decision.RetryAfter > 0 {
		t.Fatalf("unexpected decision: %+v", decision)
	}
	if builder.calls == 0 {
		t.Fatal("expected speech synthesis on first render")
	}
	final := renderRepo.tasks[baseTask.ID]
	if final.VoiceFileID != "voice_new" || final.CaptionFileID != "cap_new" || final.Status != RenderStatusSucceeded {
		t.Fatalf("expected persisted speech artifacts: %+v", final)
	}
}
