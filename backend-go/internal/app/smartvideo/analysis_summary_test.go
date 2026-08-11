package smartvideo

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestBuildAssetAnalysisSummary_VideoCandidates(t *testing.T) {
	summary := BuildAssetAnalysisSummary(NormalizedMediaMetadata{
		Kind: "VIDEO",
		Video: &VideoMetadata{
			Width: 1080, Height: 1920, DurationMS: 12000, Format: "mp4", AudioCodec: "aac",
		},
	}, "thumb_1")
	if summary.Kind != "video" || summary.DurationMs != 12000 || !summary.HasAudio {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if len(summary.CandidateClips) < 2 {
		t.Fatalf("expected candidate clips, got %+v", summary.CandidateClips)
	}
	if len(summary.RepresentativeFrames) != 1 || summary.RepresentativeFrames[0] != "thumb_1" {
		t.Fatalf("representative frames = %+v", summary.RepresentativeFrames)
	}
	if summary.CompositionHints[0] != "portrait" {
		t.Fatalf("composition = %+v", summary.CompositionHints)
	}
}

func TestBuildAssetAnalysisSummary_ImageHasNoClips(t *testing.T) {
	summary := BuildAssetAnalysisSummary(NormalizedMediaMetadata{
		Kind:  "IMAGE",
		Image: &ImageMetadata{Width: 1600, Height: 900, Format: "png"},
	}, "")
	if summary.Kind != "image" || summary.DurationMs != 0 || len(summary.CandidateClips) != 0 {
		t.Fatalf("unexpected image summary: %+v", summary)
	}
}

func TestValidateAssetQuota(t *testing.T) {
	existing := make([]ProjectAsset, MaxProjectAssets)
	err := ValidateAssetQuota(existing, FileReference{Metadata: AssetMetadata{FileSize: 1}}, AssetTypeImage)
	if err == nil || !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("count quota error = %v", err)
	}
	err = ValidateAssetQuota(nil, FileReference{Metadata: AssetMetadata{FileSize: MaxVideoFileBytes + 1}}, AssetTypeVideo)
	if err == nil || !strings.Contains(err.Error(), "video file exceeds") {
		t.Fatalf("video size error = %v", err)
	}
	err = ValidateAssetQuota(nil, FileReference{Metadata: AssetMetadata{FileSize: 10, DurationMS: MaxVideoDurationMs + 1}}, AssetTypeVideo)
	if err == nil || !strings.Contains(err.Error(), "duration exceeds") {
		t.Fatalf("duration error = %v", err)
	}
}

func TestAnalysisCompleteMarksMaterialReady(t *testing.T) {
	service, repository, queue, access, project, _ := preparedAnalysisService(t)
	if _, err := service.RequestProjectAnalysis(context.Background(), access, project.ID, "request_1"); err != nil {
		t.Fatal(err)
	}
	for _, job := range queue.Jobs() {
		task, asset, err := repository.AcquireAnalysisTask(context.Background(), job.TaskID, "worker_1", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		result := AnalysisResult{AnalyzerVersion: "v1", ThumbnailFileID: "thumb"}
		if asset.AssetType == AssetTypeVideo {
			result.Metadata = NormalizedMediaMetadata{
				Kind:  "VIDEO",
				Video: &VideoMetadata{Width: 720, Height: 1280, DurationMS: 5000, Format: "mp4"},
			}
		} else {
			result.Metadata = NormalizedMediaMetadata{
				Kind:  "IMAGE",
				Image: &ImageMetadata{Width: 800, Height: 600, Format: "png"},
			}
		}
		if err := repository.CompleteAnalysisTask(context.Background(), task.ID, "worker_1", result); err != nil {
			t.Fatal(err)
		}
	}
	updated, err := repository.GetProject(context.Background(), access, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != ProjectStatusMaterialReady {
		t.Fatalf("status = %s, want MATERIAL_READY", updated.Status)
	}
}
