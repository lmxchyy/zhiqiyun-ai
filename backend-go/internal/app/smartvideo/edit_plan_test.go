package smartvideo

import (
	"testing"
)

func makeValidEditPlanV1() EditPlanV1 {
	return EditPlanV1{
		SchemaVersion: 1,
		Title:         "测试混剪方案",
		Summary:       "一个测试用的混剪方案",
		Language:      "zh-CN",
		Target: TargetSpec{
			AspectRatio: "16:9",
			Resolution:  "1080p",
			DurationMs:  30000,
		},
		Voice: VoiceConfig{
			Enabled:  true,
			ModelKey: "tts-1",
			VoiceKey: "alloy",
			Speed:    1.0,
		},
		Subtitles: SubtitleConfig{
			Enabled:  true,
			Preset:   "clean",
			Position: "bottom",
		},
		Audio: AudioConfig{
			SourceGain: 0.8,
			VoiceGain:  1.0,
		},
		Scenes: []SceneV1{
			{
				ID:         "scene-1",
				Index:      0,
				Title:      "开场",
				DurationMs: 15000,
				Narration:  "欢迎观看",
				Clips: []ClipV1{
					{
						AssetID:           "asset-1",
						AssetType:         "video",
						SourceInMs:        0,
						SourceOutMs:       15000,
						DisplayDurationMs: 15000,
						FitMode:           "cover",
						Motion:            "static",
						OriginalAudioGain: 0.5,
					},
				},
				Transition: SceneTransitionV1{
					Type:       "fade",
					DurationMs: 500,
				},
			},
			{
				ID:         "scene-2",
				Index:      1,
				Title:      "结尾",
				DurationMs: 15500,
				Narration:  "谢谢观看",
				Clips: []ClipV1{
					{
						AssetID:           "asset-2",
						AssetType:         "image",
						SourceInMs:        0,
						SourceOutMs:       0,
						DisplayDurationMs: 15500,
						FitMode:           "contain",
						Motion:            "push",
						OriginalAudioGain: 0,
					},
				},
				Transition: SceneTransitionV1{},
			},
		},
	}
}

func ownedAssets() map[string]ProjectAsset {
	return map[string]ProjectAsset{
		"asset-1": {
			ID:        "asset-1",
			AssetType: "VIDEO",
			NormalizedMetadata: &NormalizedMediaMetadata{
				Kind: "video",
				Video: &VideoMetadata{
					DurationMS: 60000,
					Width:      1920,
					Height:     1080,
				},
			},
		},
		"asset-2": {
			ID:        "asset-2",
			AssetType: "IMAGE",
		},
	}
}

func TestValidateEditPlanV1_Valid(t *testing.T) {
	plan := makeValidEditPlanV1()
	if err := ValidateEditPlanV1(plan, ownedAssets()); err != nil {
		t.Fatalf("expected valid plan, got error: %v", err)
	}
}

func TestValidateEditPlanV1_InvalidSchemaVersion(t *testing.T) {
	plan := makeValidEditPlanV1()
	plan.SchemaVersion = 2
	err := ValidateEditPlanV1(plan, ownedAssets())
	if err == nil {
		t.Fatal("expected error for invalid schema version")
	}
	ve, ok := err.(*EditPlanValidationError)
	if !ok || ve.Code != "invalid_schema_version" {
		t.Fatalf("expected invalid_schema_version, got %v", err)
	}
}

func TestValidateEditPlanV1_UnknownAssetID(t *testing.T) {
	plan := makeValidEditPlanV1()
	plan.Scenes[0].Clips[0].AssetID = "nonexistent"
	err := ValidateEditPlanV1(plan, ownedAssets())
	if err == nil {
		t.Fatal("expected error for unknown asset ID")
	}
	ve, ok := err.(*EditPlanValidationError)
	if !ok || ve.Code != "asset_not_owned" {
		t.Fatalf("expected asset_not_owned, got %v", err)
	}
}

func TestValidateEditPlanV1_ImageWithSourceBounds(t *testing.T) {
	plan := makeValidEditPlanV1()
	plan.Scenes[1].Clips[0].AssetType = "image"
	plan.Scenes[1].Clips[0].SourceInMs = 1000
	plan.Scenes[1].Clips[0].SourceOutMs = 5000
	plan = NormalizeEditPlanV1(plan, ownedAssets())
	if plan.Scenes[1].Clips[0].SourceInMs != 0 || plan.Scenes[1].Clips[0].SourceOutMs != 0 {
		t.Fatalf("expected image source bounds normalized to 0, got in=%d out=%d", plan.Scenes[1].Clips[0].SourceInMs, plan.Scenes[1].Clips[0].SourceOutMs)
	}
	if err := ValidateEditPlanV1(plan, ownedAssets()); err != nil {
		t.Fatalf("expected normalized image plan to pass validation, got %v", err)
	}
}

func TestValidateEditPlanV1_VideoSourceOutOfBounds(t *testing.T) {
	plan := makeValidEditPlanV1()
	plan.Scenes[0].Clips[0].SourceOutMs = 120000
	err := ValidateEditPlanV1(plan, ownedAssets())
	if err == nil {
		t.Fatal("expected error for video source out of bounds")
	}
	ve, ok := err.(*EditPlanValidationError)
	if !ok || ve.Code != "source_out_of_bounds" {
		t.Fatalf("expected source_out_of_bounds, got %v", err)
	}
}

func TestValidateEditPlanV1_InvalidDuration(t *testing.T) {
	plan := makeValidEditPlanV1()
	plan.Target.DurationMs = 5000
	err := ValidateEditPlanV1(plan, ownedAssets())
	if err == nil {
		t.Fatal("expected error for duration too short")
	}
	ve, ok := err.(*EditPlanValidationError)
	if !ok || ve.Code != "invalid_duration" {
		t.Fatalf("expected invalid_duration, got %v", err)
	}

	plan.Target.DurationMs = 120000
	err = ValidateEditPlanV1(plan, ownedAssets())
	if err == nil {
		t.Fatal("expected error for duration too long")
	}
}

func TestValidateEditPlanV1_InvalidAspectRatio(t *testing.T) {
	plan := makeValidEditPlanV1()
	plan.Target.AspectRatio = "4:3"
	err := ValidateEditPlanV1(plan, ownedAssets())
	if err == nil {
		t.Fatal("expected error for invalid aspect ratio")
	}
	ve, ok := err.(*EditPlanValidationError)
	if !ok || ve.Code != "invalid_aspect_ratio" {
		t.Fatalf("expected invalid_aspect_ratio, got %v", err)
	}
}

func TestValidateEditPlanV1_InvalidTransition(t *testing.T) {
	plan := makeValidEditPlanV1()
	plan.Scenes[0].Transition.Type = "explode"
	err := ValidateEditPlanV1(plan, ownedAssets())
	if err == nil {
		t.Fatal("expected error for invalid transition")
	}
	ve, ok := err.(*EditPlanValidationError)
	if !ok || ve.Code != "invalid_transition" {
		t.Fatalf("expected invalid_transition, got %v", err)
	}
}

func TestValidateEditPlanV1_TransitionDurationExceeded(t *testing.T) {
	plan := makeValidEditPlanV1()
	plan.Scenes[0].Transition.DurationMs = 2000
	err := ValidateEditPlanV1(plan, ownedAssets())
	if err == nil {
		t.Fatal("expected error for excessive transition duration")
	}
	ve, ok := err.(*EditPlanValidationError)
	if !ok || ve.Code != "invalid_transition_duration" {
		t.Fatalf("expected invalid_transition_duration, got %v", err)
	}
}

func TestValidateEditPlanV1_DuplicateSceneIndex(t *testing.T) {
	plan := makeValidEditPlanV1()
	plan.Scenes[1].Index = 0
	err := ValidateEditPlanV1(plan, ownedAssets())
	if err == nil {
		t.Fatal("expected error for duplicate scene index")
	}
	ve, ok := err.(*EditPlanValidationError)
	if !ok || ve.Code != "duplicate_scene_index" {
		t.Fatalf("expected duplicate_scene_index, got %v", err)
	}
}

func TestValidateEditPlanV1_DurationMismatch(t *testing.T) {
	plan := makeValidEditPlanV1()
	plan.Target.DurationMs = 60000
	err := ValidateEditPlanV1(plan, ownedAssets())
	if err == nil {
		t.Fatal("expected error for duration mismatch")
	}
	ve, ok := err.(*EditPlanValidationError)
	if !ok || ve.Code != "duration_mismatch" {
		t.Fatalf("expected duration_mismatch, got %v", err)
	}
}

func TestValidateEditPlanV1_InvalidVoiceSpeed(t *testing.T) {
	plan := makeValidEditPlanV1()
	plan.Voice.Speed = 2.0
	err := ValidateEditPlanV1(plan, ownedAssets())
	if err == nil {
		t.Fatal("expected error for invalid voice speed")
	}
	ve, ok := err.(*EditPlanValidationError)
	if !ok || ve.Code != "invalid_voice_speed" {
		t.Fatalf("expected invalid_voice_speed, got %v", err)
	}
}

func TestValidateEditPlanV1_MissingVoiceModel(t *testing.T) {
	plan := makeValidEditPlanV1()
	plan.Voice.ModelKey = ""
	err := ValidateEditPlanV1(plan, ownedAssets())
	if err == nil {
		t.Fatal("expected error for missing voice model")
	}
	ve, ok := err.(*EditPlanValidationError)
	if !ok || ve.Code != "missing_voice_model" {
		t.Fatalf("expected missing_voice_model, got %v", err)
	}
}

func TestValidateEditPlanV1_VoiceDisabled(t *testing.T) {
	plan := makeValidEditPlanV1()
	plan.Voice.Enabled = false
	plan.Voice.ModelKey = ""
	plan.Voice.VoiceKey = ""
	if err := ValidateEditPlanV1(plan, ownedAssets()); err != nil {
		t.Fatalf("expected valid plan with voice disabled, got error: %v", err)
	}
}

func TestValidateEditPlanV1_TooFewScenes(t *testing.T) {
	plan := makeValidEditPlanV1()
	plan.Scenes = []SceneV1{}
	err := ValidateEditPlanV1(plan, ownedAssets())
	if err == nil {
		t.Fatal("expected error for too few scenes")
	}
	ve, ok := err.(*EditPlanValidationError)
	if !ok || ve.Code != "invalid_scene_count" {
		t.Fatalf("expected invalid_scene_count, got %v", err)
	}
}

func TestValidatePlanTransition(t *testing.T) {
	tests := []struct {
		from, to string
		valid    bool
	}{
		{PlanStatusCreated, PlanStatusQueued, true},
		{PlanStatusCreated, PlanStatusFailed, true},
		{PlanStatusCreated, PlanStatusSucceeded, false},
		{PlanStatusQueued, PlanStatusProcessing, true},
		{PlanStatusQueued, PlanStatusFailed, true},
		{PlanStatusProcessing, PlanStatusSucceeded, true},
		{PlanStatusProcessing, PlanStatusFailed, true},
		{PlanStatusFailed, PlanStatusCreated, true},
		{PlanStatusFailed, PlanStatusSucceeded, false},
		{PlanStatusSucceeded, PlanStatusCreated, false},
	}
	for _, tc := range tests {
		err := ValidatePlanTransition(tc.from, tc.to)
		if tc.valid && err != nil {
			t.Errorf("expected valid transition %s -> %s, got %v", tc.from, tc.to, err)
		}
		if !tc.valid && err == nil {
			t.Errorf("expected invalid transition %s -> %s", tc.from, tc.to)
		}
	}
}

func TestValidateProjectTransition_NewStates(t *testing.T) {
	tests := []struct {
		from, to string
		valid    bool
	}{
		{ProjectStatusDraft, ProjectStatusAnalyzing, true},
		{ProjectStatusAnalyzing, ProjectStatusMaterialReady, true},
		{ProjectStatusAnalyzing, ProjectStatusFailed, true},
		{ProjectStatusMaterialReady, ProjectStatusPlanning, true},
		{ProjectStatusMaterialReady, ProjectStatusDraft, true},
		{ProjectStatusPlanning, ProjectStatusStoryboardReady, true},
		{ProjectStatusPlanning, ProjectStatusFailed, true},
		{ProjectStatusStoryboardReady, ProjectStatusConfirmed, true},
		{ProjectStatusStoryboardReady, ProjectStatusPlanning, true},
		{ProjectStatusConfirmed, ProjectStatusRendering, true},
		{ProjectStatusConfirmed, ProjectStatusStoryboardReady, true},
		{ProjectStatusRendering, ProjectStatusCompleted, true},
		{ProjectStatusRendering, ProjectStatusFailed, true},
		{ProjectStatusCompleted, ProjectStatusDraft, true},
		{ProjectStatusFailed, ProjectStatusPlanning, true},
		{ProjectStatusFailed, ProjectStatusDraft, true},
		{ProjectStatusDraft, ProjectStatusPlanning, false},
		{ProjectStatusPlanning, ProjectStatusCompleted, false},
		{ProjectStatusMaterialReady, ProjectStatusConfirmed, false},
	}
	for _, tc := range tests {
		err := ValidateProjectTransition(tc.from, tc.to)
		if tc.valid && err != nil {
			t.Errorf("expected valid transition %s -> %s, got %v", tc.from, tc.to, err)
		}
		if !tc.valid && err == nil {
			t.Errorf("expected invalid transition %s -> %s", tc.from, tc.to)
		}
	}
}

func TestValidateRenderTransition_NewStates(t *testing.T) {
	tests := []struct {
		from, to string
		valid    bool
	}{
		{RenderStatusCreated, RenderStatusQueued, true},
		{RenderStatusQueued, RenderStatusProcessing, true},
		{RenderStatusProcessing, RenderStatusSynthesizing, true},
		{RenderStatusSynthesizing, RenderStatusRendering, true},
		{RenderStatusRendering, RenderStatusUploading, true},
		{RenderStatusUploading, RenderStatusPublishing, true},
		{RenderStatusPublishing, RenderStatusSucceeded, true},
		{RenderStatusSynthesizing, RenderStatusCancelled, true},
		{RenderStatusRendering, RenderStatusCancelled, false},
		{RenderStatusUploading, RenderStatusCancelled, false},
		{RenderStatusPublishing, RenderStatusCancelled, false},
	}
	for _, tc := range tests {
		err := ValidateRenderTransition(tc.from, tc.to)
		if tc.valid && err != nil {
			t.Errorf("expected valid transition %s -> %s, got %v", tc.from, tc.to, err)
		}
		if !tc.valid && err == nil {
			t.Errorf("expected invalid transition %s -> %s", tc.from, tc.to)
		}
	}
}
