package smartvideo

import (
	"context"
	"sync"
	"testing"
	"time"
)

type countingBuilder struct {
	mu    sync.Mutex
	calls int
	out   VoiceCaptionArtifacts
	err   error
}

func (b *countingBuilder) Build(context.Context, Access, string, EditPlanV1) (VoiceCaptionArtifacts, error) {
	b.mu.Lock()
	b.calls++
	b.mu.Unlock()
	return b.out, b.err
}

func seedSpeechPlan(t *testing.T, repo *MemoryRepository, voiceEnabled bool) (Access, RenderTask) {
	t.Helper()
	access := Access{TenantID: "tenant_a", UserID: "user_a"}
	now := time.Now().UTC()
	project := Project{
		ID: "vp_speech", TenantID: access.TenantID, UserID: access.UserID,
		Title: "speech", Requirement: "req", Status: ProjectStatusConfirmed,
		CurrentVersion: 1, CurrentVersionID: "svv_speech", ConfirmedVersionID: "svv_speech",
		CreatedAt: now, UpdatedAt: now,
	}
	if _, err := repo.CreateProject(context.Background(), project); err != nil {
		t.Fatalf("project: %v", err)
	}
	plan := makeValidEditPlanV1()
	plan.Voice.Enabled = voiceEnabled
	if _, err := repo.CreateImmutableVersion(context.Background(), ProjectVersion{
		ID: "svv_speech", ProjectID: project.ID, TenantID: access.TenantID, VersionNumber: 1,
		Source: VersionSourceAI, PlanSchemaVersion: 1, PlanSnapshot: plan,
		CreatedBy: access.UserID, CreatedAt: now,
	}); err != nil {
		t.Fatalf("version: %v", err)
	}
	task := RenderTask{
		ID: "svrender_1", ProjectID: project.ID, VersionID: "svv_speech",
		TenantID: access.TenantID, UserID: access.UserID, Status: RenderStatusSynthesizing,
		MaxAttempts: 3, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := repo.CreateRenderTask(context.Background(), task); err != nil {
		t.Fatalf("task: %v", err)
	}
	return access, task
}

func TestSpeechPrepSynthesizesViaBuilder(t *testing.T) {
	repo := NewMemoryRepository()
	access, task := seedSpeechPlan(t, repo, true)
	builder := &countingBuilder{out: VoiceCaptionArtifacts{VoiceFileID: "voice_1", CaptionFileID: "cap_1"}}
	service := NewSpeechPrepService(builder, repo)

	artifacts, err := service.Prepare(context.Background(), access, task)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if artifacts.VoiceFileID != "voice_1" || artifacts.CaptionFileID != "cap_1" || builder.calls != 1 {
		t.Fatalf("unexpected artifacts=%+v calls=%d", artifacts, builder.calls)
	}
}

func TestSpeechPrepReusesExistingArtifactsWithoutResynthesis(t *testing.T) {
	repo := NewMemoryRepository()
	access, task := seedSpeechPlan(t, repo, true)
	task.VoiceFileID = "voice_existing"
	task.CaptionFileID = "caption_existing"
	builder := &countingBuilder{out: VoiceCaptionArtifacts{VoiceFileID: "new", CaptionFileID: "new"}}
	service := NewSpeechPrepService(builder, repo)

	artifacts, err := service.Prepare(context.Background(), access, task)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if !artifacts.Skipped || artifacts.VoiceFileID != "voice_existing" || builder.calls != 0 {
		t.Fatalf("expected reuse, got %+v calls=%d", artifacts, builder.calls)
	}
}

func TestSpeechPrepSkipsWhenVoiceDisabled(t *testing.T) {
	repo := NewMemoryRepository()
	access, task := seedSpeechPlan(t, repo, false)
	builder := &countingBuilder{out: VoiceCaptionArtifacts{VoiceFileID: "x", CaptionFileID: "y"}}
	service := NewSpeechPrepService(builder, repo)

	artifacts, err := service.Prepare(context.Background(), access, task)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if !artifacts.Skipped || artifacts.VoiceFileID != "" || builder.calls != 0 {
		t.Fatalf("expected skip, got %+v calls=%d", artifacts, builder.calls)
	}
}
