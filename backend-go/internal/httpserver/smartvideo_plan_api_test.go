package httpserver

import (
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/smartvideo"
)

func TestSmartVideoPlanReviseConfirmEstimate(t *testing.T) {
	repo := smartvideo.NewMemoryRepository()
	access := smartvideo.Access{TenantID: "tenant_a", UserID: "user_a"}
	now := time.Now().UTC()
	project := smartvideo.Project{
		ID: "vp_plan", TenantID: access.TenantID, UserID: access.UserID,
		Title: "plan", Requirement: "req", Status: smartvideo.ProjectStatusStoryboardReady,
		CurrentVersion: 1, CurrentVersionID: "svv_plan", CreatedAt: now, UpdatedAt: now,
	}
	if _, err := repo.CreateProject(t.Context(), project); err != nil {
		t.Fatalf("create project: %v", err)
	}
	assets := []smartvideo.ProjectAsset{
		{
			ID: "asset-1", ProjectID: project.ID, TenantID: access.TenantID, UserID: access.UserID,
			FileID: "file_1", StorageKey: "obj/1", AssetType: "VIDEO",
			NormalizedMetadata: &smartvideo.NormalizedMediaMetadata{
				Kind: "video", Video: &smartvideo.VideoMetadata{DurationMS: 60000, Width: 1920, Height: 1080},
			},
		},
		{
			ID: "asset-2", ProjectID: project.ID, TenantID: access.TenantID, UserID: access.UserID,
			FileID: "file_2", StorageKey: "obj/2", AssetType: "IMAGE",
		},
	}
	for _, asset := range assets {
		if _, err := repo.CreateAsset(t.Context(), asset); err != nil {
			t.Fatalf("create asset: %v", err)
		}
	}
	plan := smartvideo.EditPlanV1{
		SchemaVersion: 1, Title: "测试混剪方案", Summary: "摘要", Language: "zh-CN",
		Target:    smartvideo.TargetSpec{AspectRatio: "16:9", Resolution: "1080p", DurationMs: 30000},
		Voice:     smartvideo.VoiceConfig{Enabled: true, ModelKey: "tts-1", VoiceKey: "alloy", Speed: 1},
		Subtitles: smartvideo.SubtitleConfig{Enabled: true, Preset: "clean", Position: "bottom"},
		Audio:     smartvideo.AudioConfig{SourceGain: 0.8, VoiceGain: 1},
		Scenes: []smartvideo.SceneV1{
			{
				ID: "scene-1", Index: 0, Title: "开场", DurationMs: 15000, Narration: "欢迎观看",
				Clips: []smartvideo.ClipV1{{
					AssetID: "asset-1", AssetType: "video", SourceInMs: 0, SourceOutMs: 15000,
					DisplayDurationMs: 15000, FitMode: "cover", Motion: "static", OriginalAudioGain: 0.5,
				}},
				Transition: smartvideo.SceneTransitionV1{Type: "fade", DurationMs: 500},
			},
			{
				ID: "scene-2", Index: 1, Title: "结尾", DurationMs: 15500, Narration: "谢谢观看",
				Clips: []smartvideo.ClipV1{{
					AssetID: "asset-2", AssetType: "image", DisplayDurationMs: 15500,
					FitMode: "contain", Motion: "push",
				}},
			},
		},
	}
	if _, err := repo.CreateImmutableVersion(t.Context(), smartvideo.ProjectVersion{
		ID: "svv_plan", ProjectID: project.ID, TenantID: access.TenantID, VersionNumber: 1,
		Source: smartvideo.VersionSourceAI, PlanSchemaVersion: 1, PlanSnapshot: plan,
		CreatedBy: access.UserID, CreatedAt: now,
	}); err != nil {
		t.Fatalf("create version: %v", err)
	}

	api := newSmartVideoAPI(
		smartvideo.NewService(repo, nil),
		nil,
		smartvideo.NewPlanService(repo, nil, repo, nil),
		smartvideo.NewExportService(repo, repo, repo, smartvideo.NewMemoryPointsLifecycle(10_000)),
		fileCenterAPI{},
	)

	quote, err := api.plans.EstimateRender(t.Context(), access, "vp_plan", "svv_plan")
	if err != nil {
		t.Fatalf("estimate: %v", err)
	}
	if quote.Points != 108 {
		t.Fatalf("points = %d, want 108", quote.Points)
	}

	next := plan
	next.Title = "HTTP修订"
	child, err := api.plans.RevisePlan(t.Context(), access, "vp_plan", "svv_plan", smartvideo.RevisePlanInput{
		Plan: next, ChangeNote: "api revise",
	})
	if err != nil {
		t.Fatalf("revise: %v", err)
	}
	if child.PlanSnapshot.Title != "HTTP修订" {
		t.Fatalf("unexpected child title: %s", child.PlanSnapshot.Title)
	}

	projectOut, versionOut, err := api.plans.ConfirmPlan(t.Context(), access, "vp_plan", child.ID)
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if projectOut.Status != smartvideo.ProjectStatusConfirmed || versionOut.ManifestHash == "" {
		t.Fatalf("confirm incomplete: project=%+v version=%+v", projectOut, versionOut)
	}
}
