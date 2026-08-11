package smartvideo

import (
	"fmt"
	"math"
	"strings"
)

const (
	EditPlanSchemaVersion = 1

	PlanStatusCreated    = "CREATED"
	PlanStatusQueued     = "QUEUED"
	PlanStatusProcessing = "PROCESSING"
	PlanStatusSucceeded  = "SUCCEEDED"
	PlanStatusFailed     = "FAILED"

	TargetAspectRatio9x16 = "9:16"
	TargetAspectRatio16x9 = "16:9"

	TargetResolution720p  = "720p"
	TargetResolution1080p = "1080p"

	TransitionTypeCut        = "cut"
	TransitionTypeFade       = "fade"
	TransitionTypeDissolve   = "dissolve"
	TransitionTypeWipeLeft   = "wipeleft"
	TransitionTypeWipeRight  = "wiperight"
	TransitionTypeSlideLeft  = "slideleft"
	TransitionTypeSlideRight = "slideright"

	FitModeCover   = "cover"
	FitModeContain = "contain"

	MotionStatic   = "static"
	MotionPush     = "push"
	MotionPull     = "pull"
	MotionPanLeft  = "pan_left"
	MotionPanRight = "pan_right"

	SubtitlePresetClean    = "clean"
	SubtitlePresetEmphasis = "emphasis"

	SubtitlePositionBottom = "bottom"
	SubtitlePositionCenter = "center"

	MinMontageDurationMs = 15000
	MaxMontageDurationMs = 60000

	MinScenes = 1
	MaxScenes = 40
	MinClips  = 1
	MaxClips  = 6

	MaxTransitionDurationMs = 1000

	MinSpeed = 0.8
	MaxSpeed = 1.2
)

var (
	validTransitions = map[string]bool{
		TransitionTypeCut: true, TransitionTypeFade: true, TransitionTypeDissolve: true,
		TransitionTypeWipeLeft: true, TransitionTypeWipeRight: true,
		TransitionTypeSlideLeft: true, TransitionTypeSlideRight: true,
	}
	validFitModes = map[string]bool{FitModeCover: true, FitModeContain: true}
	validMotions  = map[string]bool{
		MotionStatic: true, MotionPush: true, MotionPull: true,
		MotionPanLeft: true, MotionPanRight: true,
	}
	validSubtitlePresets   = map[string]bool{SubtitlePresetClean: true, SubtitlePresetEmphasis: true}
	validSubtitlePositions = map[string]bool{SubtitlePositionBottom: true, SubtitlePositionCenter: true}
)

type TargetSpec struct {
	AspectRatio string `json:"aspectRatio"`
	Resolution  string `json:"resolution"`
	DurationMs  int64  `json:"durationMs"`
}

type VoiceConfig struct {
	Enabled  bool    `json:"enabled"`
	ModelKey string  `json:"modelKey"`
	VoiceKey string  `json:"voiceKey"`
	Speed    float64 `json:"speed"`
}

type SubtitleConfig struct {
	Enabled  bool   `json:"enabled"`
	Preset   string `json:"preset"`
	Position string `json:"position"`
}

type AudioConfig struct {
	SourceGain float64 `json:"sourceGain"`
	VoiceGain  float64 `json:"voiceGain"`
}

type ClipV1 struct {
	AssetID           string  `json:"assetId"`
	AssetType         string  `json:"assetType"`
	SourceInMs        int64   `json:"sourceInMs"`
	SourceOutMs       int64   `json:"sourceOutMs"`
	DisplayDurationMs int64   `json:"displayDurationMs"`
	FitMode           string  `json:"fitMode"`
	Motion            string  `json:"motion"`
	OriginalAudioGain float64 `json:"originalAudioGain"`
}

type SceneTransitionV1 struct {
	Type       string `json:"type"`
	DurationMs int64  `json:"durationMs"`
}

type SceneV1 struct {
	ID         string            `json:"id"`
	Index      int               `json:"index"`
	Title      string            `json:"title"`
	DurationMs int64             `json:"durationMs"`
	Narration  string            `json:"narration"`
	Clips      []ClipV1          `json:"clips"`
	Transition SceneTransitionV1 `json:"transition"`
}

type EditPlanV1 struct {
	SchemaVersion int            `json:"schemaVersion"`
	Title         string         `json:"title"`
	Summary       string         `json:"summary"`
	Language      string         `json:"language"`
	Target        TargetSpec     `json:"target"`
	Voice         VoiceConfig    `json:"voice"`
	Subtitles     SubtitleConfig `json:"subtitles"`
	Audio         AudioConfig    `json:"audio"`
	Scenes        []SceneV1      `json:"scenes"`
}

type EditPlanValidationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *EditPlanValidationError) Error() string {
	return fmt.Sprintf("invalid_plan: %s: %s", e.Code, e.Message)
}

// NormalizeEditPlanV1 repairs common planner mistakes so validation can accept
// otherwise-usable plans (image source bounds, duration drift, missing voice keys).
func NormalizeEditPlanV1(plan EditPlanV1, ownedAssetIDs map[string]ProjectAsset) EditPlanV1 {
	if plan.SchemaVersion == 0 {
		plan.SchemaVersion = EditPlanSchemaVersion
	}
	if strings.TrimSpace(plan.Language) == "" {
		plan.Language = "zh-CN"
	}
	if plan.Target.AspectRatio == "" {
		plan.Target.AspectRatio = TargetAspectRatio9x16
	}
	if plan.Target.Resolution == "" {
		plan.Target.Resolution = TargetResolution1080p
	}
	if plan.Target.DurationMs < MinMontageDurationMs {
		plan.Target.DurationMs = MinMontageDurationMs
	}
	if plan.Target.DurationMs > MaxMontageDurationMs {
		plan.Target.DurationMs = MaxMontageDurationMs
	}

	if plan.Voice.Enabled {
		if strings.TrimSpace(plan.Voice.ModelKey) == "" {
			plan.Voice.ModelKey = "smart-video-speech"
		}
		if strings.TrimSpace(plan.Voice.VoiceKey) == "" {
			plan.Voice.VoiceKey = "alloy"
		}
		if plan.Voice.Speed == 0 {
			plan.Voice.Speed = 1
		} else if plan.Voice.Speed < MinSpeed {
			plan.Voice.Speed = MinSpeed
		} else if plan.Voice.Speed > MaxSpeed {
			plan.Voice.Speed = MaxSpeed
		}
	}
	if plan.Subtitles.Enabled {
		if !validSubtitlePresets[plan.Subtitles.Preset] {
			plan.Subtitles.Preset = SubtitlePresetClean
		}
		if !validSubtitlePositions[plan.Subtitles.Position] {
			plan.Subtitles.Position = SubtitlePositionBottom
		}
	}
	if plan.Audio.SourceGain < 0 {
		plan.Audio.SourceGain = 0
	} else if plan.Audio.SourceGain > 1 {
		plan.Audio.SourceGain = 1
	}
	if plan.Audio.VoiceGain < 0 {
		plan.Audio.VoiceGain = 0
	} else if plan.Audio.VoiceGain > 1 {
		plan.Audio.VoiceGain = 1
	} else if plan.Audio.VoiceGain == 0 && plan.Voice.Enabled {
		plan.Audio.VoiceGain = 1
	}

	for i := range plan.Scenes {
		scene := &plan.Scenes[i]
		if scene.Index < 0 {
			scene.Index = i
		}
		if strings.TrimSpace(scene.Title) == "" {
			scene.Title = fmt.Sprintf("场景 %d", i+1)
		}
		if scene.Transition.Type == "" || !validTransitions[scene.Transition.Type] {
			scene.Transition.Type = TransitionTypeCut
			scene.Transition.DurationMs = 0
		}
		if scene.Transition.DurationMs < 0 {
			scene.Transition.DurationMs = 0
		}
		if scene.Transition.DurationMs > MaxTransitionDurationMs {
			scene.Transition.DurationMs = MaxTransitionDurationMs
		}
		if scene.Transition.Type == TransitionTypeCut {
			scene.Transition.DurationMs = 0
		}
		for j := range scene.Clips {
			clip := &scene.Clips[j]
			if asset, ok := ownedAssetIDs[clip.AssetID]; ok {
				switch strings.ToUpper(strings.TrimSpace(asset.AssetType)) {
				case AssetTypeImage:
					clip.AssetType = "image"
					clip.SourceInMs = 0
					clip.SourceOutMs = 0
				case AssetTypeVideo:
					clip.AssetType = "video"
					assetMaxDuration := int64(0)
					if asset.NormalizedMetadata != nil && asset.NormalizedMetadata.Video != nil {
						assetMaxDuration = asset.NormalizedMetadata.Video.DurationMS
					}
					if assetMaxDuration <= 0 {
						assetMaxDuration = asset.DurationMS
					}
					if clip.SourceOutMs <= clip.SourceInMs {
						clip.SourceInMs = 0
						if assetMaxDuration > 0 {
							clip.SourceOutMs = assetMaxDuration
						} else if clip.DisplayDurationMs > 0 {
							clip.SourceOutMs = clip.DisplayDurationMs
						} else {
							clip.SourceOutMs = 1000
						}
					}
					if assetMaxDuration > 0 && clip.SourceOutMs > assetMaxDuration {
						clip.SourceOutMs = assetMaxDuration
						if clip.SourceInMs >= clip.SourceOutMs {
							clip.SourceInMs = 0
						}
					}
				}
			}
			if strings.EqualFold(strings.TrimSpace(clip.AssetType), "image") {
				clip.AssetType = "image"
				clip.SourceInMs = 0
				clip.SourceOutMs = 0
			}
			if !validFitModes[clip.FitMode] {
				clip.FitMode = FitModeCover
			}
			if !validMotions[clip.Motion] {
				clip.Motion = MotionStatic
			}
			if clip.OriginalAudioGain < 0 {
				clip.OriginalAudioGain = 0
			} else if clip.OriginalAudioGain > 1 {
				clip.OriginalAudioGain = 1
			}
			if clip.DisplayDurationMs <= 0 && scene.DurationMs > 0 {
				clip.DisplayDurationMs = scene.DurationMs
			}
		}
	}

	reconcileSceneDurations(&plan)
	return plan
}

func planEffectiveDurationMs(scenes []SceneV1) int64 {
	var total int64
	for i, scene := range scenes {
		effective := scene.DurationMs
		if i < len(scenes)-1 {
			effective -= scene.Transition.DurationMs
		}
		total += effective
	}
	return total
}

func reconcileSceneDurations(plan *EditPlanV1) {
	if plan == nil || len(plan.Scenes) == 0 || plan.Target.DurationMs <= 0 {
		return
	}
	diff := plan.Target.DurationMs - planEffectiveDurationMs(plan.Scenes)
	if diff == 0 {
		return
	}
	last := &plan.Scenes[len(plan.Scenes)-1]
	last.DurationMs += diff
	if last.DurationMs < 1 {
		last.DurationMs = 1
	}
	if len(last.Clips) == 1 {
		last.Clips[0].DisplayDurationMs = last.DurationMs
	} else if len(last.Clips) > 0 {
		clip := &last.Clips[len(last.Clips)-1]
		clip.DisplayDurationMs += diff
		if clip.DisplayDurationMs < 1 {
			clip.DisplayDurationMs = 1
		}
	}
}

func ValidateEditPlanV1(plan EditPlanV1, ownedAssetIDs map[string]ProjectAsset) error {
	if plan.SchemaVersion != EditPlanSchemaVersion {
		return &EditPlanValidationError{Code: "invalid_schema_version", Message: fmt.Sprintf("expected %d got %d", EditPlanSchemaVersion, plan.SchemaVersion)}
	}
	if err := validateTargetSpec(plan.Target); err != nil {
		return err
	}
	if err := validateVoiceConfig(plan.Voice); err != nil {
		return err
	}
	if err := validateSubtitleConfig(plan.Subtitles); err != nil {
		return err
	}
	if err := validateAudioConfig(plan.Audio); err != nil {
		return err
	}
	if len(plan.Title) == 0 || len(strings.TrimSpace(plan.Title)) == 0 {
		return &EditPlanValidationError{Code: "missing_title", Message: "title is required"}
	}
	if plan.Language != "zh-CN" && plan.Language != "en-US" {
		return &EditPlanValidationError{Code: "invalid_language", Message: fmt.Sprintf("unsupported language %s", plan.Language)}
	}
	if len(plan.Scenes) < MinScenes || len(plan.Scenes) > MaxScenes {
		return &EditPlanValidationError{Code: "invalid_scene_count", Message: fmt.Sprintf("scenes must be between %d and %d, got %d", MinScenes, MaxScenes, len(plan.Scenes))}
	}
	return validateScenes(plan.Scenes, plan.Target.DurationMs, ownedAssetIDs)
}

func validateTargetSpec(t TargetSpec) error {
	if t.AspectRatio != TargetAspectRatio9x16 && t.AspectRatio != TargetAspectRatio16x9 {
		return &EditPlanValidationError{Code: "invalid_aspect_ratio", Message: fmt.Sprintf("unsupported aspect ratio %s", t.AspectRatio)}
	}
	if t.Resolution != TargetResolution720p && t.Resolution != TargetResolution1080p {
		return &EditPlanValidationError{Code: "invalid_resolution", Message: fmt.Sprintf("unsupported resolution %s", t.Resolution)}
	}
	if t.DurationMs < MinMontageDurationMs || t.DurationMs > MaxMontageDurationMs {
		return &EditPlanValidationError{Code: "invalid_duration", Message: fmt.Sprintf("duration must be between %d and %d ms, got %d", MinMontageDurationMs, MaxMontageDurationMs, t.DurationMs)}
	}
	return nil
}

func validateVoiceConfig(v VoiceConfig) error {
	if !v.Enabled {
		return nil
	}
	if strings.TrimSpace(v.ModelKey) == "" {
		return &EditPlanValidationError{Code: "missing_voice_model", Message: "voice modelKey is required when voice is enabled"}
	}
	if strings.TrimSpace(v.VoiceKey) == "" {
		return &EditPlanValidationError{Code: "missing_voice_key", Message: "voice voiceKey is required when voice is enabled"}
	}
	if v.Speed < MinSpeed || v.Speed > MaxSpeed {
		return &EditPlanValidationError{Code: "invalid_voice_speed", Message: fmt.Sprintf("voice speed must be between %.1f and %.1f, got %.2f", MinSpeed, MaxSpeed, v.Speed)}
	}
	return nil
}

func validateSubtitleConfig(s SubtitleConfig) error {
	if !s.Enabled {
		return nil
	}
	if !validSubtitlePresets[s.Preset] {
		return &EditPlanValidationError{Code: "invalid_subtitle_preset", Message: fmt.Sprintf("unsupported subtitle preset %s", s.Preset)}
	}
	if !validSubtitlePositions[s.Position] {
		return &EditPlanValidationError{Code: "invalid_subtitle_position", Message: fmt.Sprintf("unsupported subtitle position %s", s.Position)}
	}
	return nil
}

func validateAudioConfig(a AudioConfig) error {
	if a.SourceGain < 0 || a.SourceGain > 1 {
		return &EditPlanValidationError{Code: "invalid_source_gain", Message: fmt.Sprintf("sourceGain must be between 0 and 1, got %.2f", a.SourceGain)}
	}
	if a.VoiceGain < 0 || a.VoiceGain > 1 {
		return &EditPlanValidationError{Code: "invalid_voice_gain", Message: fmt.Sprintf("voiceGain must be between 0 and 1, got %.2f", a.VoiceGain)}
	}
	return nil
}

func validateScenes(scenes []SceneV1, targetDurationMs int64, ownedAssetIDs map[string]ProjectAsset) error {
	seenIndices := map[int]bool{}
	var totalEffectiveDurationMs int64

	for i, scene := range scenes {
		if len(scene.Title) == 0 || len(strings.TrimSpace(scene.Title)) == 0 {
			return &EditPlanValidationError{Code: "missing_scene_title", Message: fmt.Sprintf("scene %d title is required", i)}
		}
		if scene.Index < 0 {
			return &EditPlanValidationError{Code: "invalid_scene_index", Message: fmt.Sprintf("scene %d has negative index", i)}
		}
		if seenIndices[scene.Index] {
			return &EditPlanValidationError{Code: "duplicate_scene_index", Message: fmt.Sprintf("duplicate scene index %d", scene.Index)}
		}
		seenIndices[scene.Index] = true

		if scene.DurationMs <= 0 {
			return &EditPlanValidationError{Code: "invalid_scene_duration", Message: fmt.Sprintf("scene %d has non-positive duration", i)}
		}

		if err := validateClips(scene.Clips, i, ownedAssetIDs); err != nil {
			return err
		}

		if err := validateSceneTransition(scene.Transition, i); err != nil {
			return err
		}

		effectiveDur := scene.DurationMs
		if i < len(scenes)-1 {
			effectiveDur -= scene.Transition.DurationMs
		}
		totalEffectiveDurationMs += effectiveDur
	}

	frameMs := int64(math.Round(1000.0 / 30.0))
	diff := targetDurationMs - totalEffectiveDurationMs
	if diff < 0 {
		diff = -diff
	}
	if diff > frameMs {
		return &EditPlanValidationError{Code: "duration_mismatch", Message: fmt.Sprintf("total scene duration %d ms differs from target %d ms by more than one frame", totalEffectiveDurationMs, targetDurationMs)}
	}
	return nil
}

func validateClips(clips []ClipV1, sceneIndex int, ownedAssetIDs map[string]ProjectAsset) error {
	if len(clips) < MinClips || len(clips) > MaxClips {
		return &EditPlanValidationError{Code: "invalid_clip_count", Message: fmt.Sprintf("scene %d clips must be between %d and %d, got %d", sceneIndex, MinClips, MaxClips, len(clips))}
	}
	for j, clip := range clips {
		asset, ok := ownedAssetIDs[clip.AssetID]
		if !ok {
			return &EditPlanValidationError{Code: "asset_not_owned", Message: fmt.Sprintf("scene %d clip %d references unknown asset %s", sceneIndex, j, clip.AssetID)}
		}
		if clip.AssetType != "image" && clip.AssetType != "video" {
			return &EditPlanValidationError{Code: "invalid_asset_type", Message: fmt.Sprintf("scene %d clip %d has invalid assetType %s", sceneIndex, j, clip.AssetType)}
		}
		if clip.AssetType == "image" {
			if clip.SourceInMs != 0 || clip.SourceOutMs != 0 {
				return &EditPlanValidationError{Code: "image_source_bounds", Message: fmt.Sprintf("scene %d clip %d is an image but has non-zero source in/out", sceneIndex, j)}
			}
		} else if clip.AssetType == "video" {
			assetMaxDuration := int64(0)
			if asset.NormalizedMetadata != nil && asset.NormalizedMetadata.Video != nil {
				assetMaxDuration = asset.NormalizedMetadata.Video.DurationMS
			}
			if clip.SourceInMs < 0 || clip.SourceOutMs <= clip.SourceInMs {
				return &EditPlanValidationError{Code: "invalid_source_bounds", Message: fmt.Sprintf("scene %d clip %d has invalid source bounds: in=%d out=%d", sceneIndex, j, clip.SourceInMs, clip.SourceOutMs)}
			}
			if assetMaxDuration > 0 && clip.SourceOutMs > assetMaxDuration {
				return &EditPlanValidationError{Code: "source_out_of_bounds", Message: fmt.Sprintf("scene %d clip %d source out %d exceeds asset duration %d", sceneIndex, j, clip.SourceOutMs, assetMaxDuration)}
			}
		}
		if clip.DisplayDurationMs <= 0 {
			return &EditPlanValidationError{Code: "invalid_display_duration", Message: fmt.Sprintf("scene %d clip %d has non-positive display duration", sceneIndex, j)}
		}
		if !validFitModes[clip.FitMode] {
			return &EditPlanValidationError{Code: "invalid_fit_mode", Message: fmt.Sprintf("scene %d clip %d has unsupported fit mode %s", sceneIndex, j, clip.FitMode)}
		}
		if !validMotions[clip.Motion] {
			return &EditPlanValidationError{Code: "invalid_motion", Message: fmt.Sprintf("scene %d clip %d has unsupported motion %s", sceneIndex, j, clip.Motion)}
		}
		if clip.OriginalAudioGain < 0 || clip.OriginalAudioGain > 1 {
			return &EditPlanValidationError{Code: "invalid_audio_gain", Message: fmt.Sprintf("scene %d clip %d originalAudioGain must be between 0 and 1", sceneIndex, j)}
		}
	}
	return nil
}

func validateSceneTransition(t SceneTransitionV1, sceneIndex int) error {
	if t.Type == "" {
		return nil
	}
	if !validTransitions[t.Type] {
		return &EditPlanValidationError{Code: "invalid_transition", Message: fmt.Sprintf("scene %d has unsupported transition %s", sceneIndex, t.Type)}
	}
	if t.DurationMs < 0 || t.DurationMs > MaxTransitionDurationMs {
		return &EditPlanValidationError{Code: "invalid_transition_duration", Message: fmt.Sprintf("scene %d transition duration must be between 0 and %d ms", sceneIndex, MaxTransitionDurationMs)}
	}
	return nil
}

func ValidatePlanTransition(from, to string) error {
	allowed := map[string]map[string]bool{
		PlanStatusCreated:    {PlanStatusQueued: true, PlanStatusFailed: true},
		PlanStatusQueued:     {PlanStatusProcessing: true, PlanStatusFailed: true},
		PlanStatusProcessing: {PlanStatusSucceeded: true, PlanStatusFailed: true},
		PlanStatusFailed:     {PlanStatusCreated: true},
	}
	if !allowed[from][to] {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidStateTransition, from, to)
	}
	return nil
}
