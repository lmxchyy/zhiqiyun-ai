package smartvideo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"path"
	"sort"
	"strings"
)

const RenderManifestSchemaVersion = 1

var (
	allowedManifestVideoCodecs = map[string]bool{"libx264": true}
	allowedManifestAudioCodecs = map[string]bool{"aac": true}
	allowedManifestFormats     = map[string]bool{"mp4": true}
	allowedManifestPixelFmts   = map[string]bool{"yuv420p": true}
)

func CompileRenderManifest(input RenderManifestInput) (RenderManifestV1, error) {
	plan := input.Version.PlanSnapshot
	if plan.SchemaVersion == 0 {
		return RenderManifestV1{}, &EditPlanValidationError{Code: "missing_plan", Message: "version has no plan snapshot"}
	}
	owned := map[string]ProjectAsset{}
	for _, asset := range input.Assets {
		owned[asset.ID] = asset
	}
	plan = NormalizeEditPlanV1(plan, owned)
	if err := ValidateEditPlanV1(plan, owned); err != nil {
		return RenderManifestV1{}, err
	}
	if err := ValidateEditPlanContent(plan); err != nil {
		return RenderManifestV1{}, err
	}
	if err := validateManifestArtifactIDs(input.VoiceFileID, input.CaptionFileID); err != nil {
		return RenderManifestV1{}, err
	}

	width, height := resolutionPixels(plan.Target.AspectRatio, plan.Target.Resolution)
	assetIndex := map[string]int{}
	inputs := make([]ManifestInput, 0, len(input.Assets))
	sortedAssets := append([]ProjectAsset{}, input.Assets...)
	sort.SliceStable(sortedAssets, func(i, j int) bool {
		if sortedAssets[i].OrderIndex != sortedAssets[j].OrderIndex {
			return sortedAssets[i].OrderIndex < sortedAssets[j].OrderIndex
		}
		return sortedAssets[i].ID < sortedAssets[j].ID
	})
	for _, asset := range sortedAssets {
		if err := validateManifestAssetRef(asset); err != nil {
			return RenderManifestV1{}, err
		}
		assetIndex[asset.ID] = len(inputs)
		durationMs := asset.Metadata.DurationMS
		widthMeta, heightMeta := asset.Metadata.Width, asset.Metadata.Height
		if asset.NormalizedMetadata != nil {
			if asset.NormalizedMetadata.Video != nil {
				durationMs = asset.NormalizedMetadata.Video.DurationMS
				widthMeta = asset.NormalizedMetadata.Video.Width
				heightMeta = asset.NormalizedMetadata.Video.Height
			}
			if asset.NormalizedMetadata.Image != nil {
				widthMeta = asset.NormalizedMetadata.Image.Width
				heightMeta = asset.NormalizedMetadata.Image.Height
			}
		}
		inputs = append(inputs, ManifestInput{
			FileID: strings.TrimSpace(asset.FileID),
			ObjectKey: canonicalizeObjectKey(asset.StorageKey),
			Checksum:  strings.TrimSpace(asset.Metadata.FileHash),
			DurationMs: durationMs, Width: widthMeta, Height: heightMeta,
			AssetType: strings.ToLower(strings.TrimSpace(asset.AssetType)),
		})
	}

	sortedScenes := append([]SceneV1{}, plan.Scenes...)
	sort.SliceStable(sortedScenes, func(i, j int) bool {
		return sortedScenes[i].Index < sortedScenes[j].Index
	})

	scenes := make([]ManifestScene, 0, len(sortedScenes))
	var timelineCursor int64
	for scenePos, scene := range sortedScenes {
		cuts := make([]ManifestCut, 0, len(scene.Clips))
		for _, clip := range scene.Clips {
			idx, ok := assetIndex[clip.AssetID]
			if !ok {
				return RenderManifestV1{}, &EditPlanValidationError{Code: "asset_not_owned", Message: "clip references unknown asset"}
			}
			cuts = append(cuts, ManifestCut{
				InputIndex: idx,
				SourceInMs: clip.SourceInMs,
				SourceOutMs: clip.SourceOutMs,
				FitMode: clip.FitMode,
				Motion:  clip.Motion,
				AudioGain: normalizeGain(clip.OriginalAudioGain),
				TargetWidth: width, TargetHeight: height,
			})
		}
		transitionType := scene.Transition.Type
		if transitionType == "" {
			transitionType = TransitionTypeCut
		}
		var voiceSeg *ManifestVoiceSeg
		if plan.Voice.Enabled && strings.TrimSpace(input.VoiceFileID) != "" && strings.TrimSpace(scene.Narration) != "" {
			voiceSeg = &ManifestVoiceSeg{
				AudioFileID: strings.TrimSpace(input.VoiceFileID),
				StartMs:     timelineCursor,
				DurationMs:  scene.DurationMs,
			}
		}
		var cues []ManifestCue
		if plan.Subtitles.Enabled && strings.TrimSpace(scene.Narration) != "" {
			cues = []ManifestCue{{
				StartMs: timelineCursor,
				EndMs:   timelineCursor + scene.DurationMs,
				Text:    strings.TrimSpace(scene.Narration),
			}}
		}
		overlapMs := int64(0)
		if scenePos < len(sortedScenes)-1 {
			overlapMs = scene.Transition.DurationMs
		}
		scenes = append(scenes, ManifestScene{
			Index: scene.Index, DurationMs: scene.DurationMs, Cuts: cuts,
			VoiceSegment: voiceSeg,
			Transition: ManifestTrans{
				Type: transitionType, DurationMs: scene.Transition.DurationMs,
			},
			Subtitles: cues,
		})
		timelineCursor += scene.DurationMs
		if overlapMs > 0 {
			timelineCursor -= overlapMs
		}
		_ = overlapMs
	}

	manifest := RenderManifestV1{
		SchemaVersion: RenderManifestSchemaVersion,
		Output: ManifestOutputSpec{
			Width: width, Height: height, FrameRate: 30,
			VideoCodec: "libx264", AudioCodec: "aac", PixelFormat: "yuv420p",
			Format: "mp4", Bitrate: defaultBitrate(plan.Target.Resolution),
		},
		Inputs: inputs, Scenes: scenes,
		VoiceFileID: strings.TrimSpace(input.VoiceFileID),
		CaptionFileID: strings.TrimSpace(input.CaptionFileID),
		AudioMix: ManifestAudioMix{
			SourceGain: normalizeGain(plan.Audio.SourceGain),
			VoiceGain:  normalizeGain(plan.Audio.VoiceGain),
		},
	}
	if err := validateCompiledManifestEncoding(manifest); err != nil {
		return RenderManifestV1{}, err
	}
	hash, err := HashRenderManifest(manifest)
	if err != nil {
		return RenderManifestV1{}, err
	}
	manifest.ManifestHash = hash
	return manifest, nil
}

func HashRenderManifest(manifest RenderManifestV1) (string, error) {
	clone := manifest
	clone.ManifestHash = ""
	raw, err := marshalCanonicalJSON(clone)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func marshalCanonicalJSON(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, err
	}
	return json.Marshal(normalized)
}

func normalizeGain(v float64) float64 {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	return math.Round(v*1000) / 1000
}

func canonicalizeObjectKey(key string) string {
	key = strings.TrimSpace(key)
	key = strings.ReplaceAll(key, "\\", "/")
	return path.Clean(key)
}

func validateManifestAssetRef(asset ProjectAsset) error {
	fileID := strings.TrimSpace(asset.FileID)
	key := strings.TrimSpace(asset.StorageKey)
	if fileID == "" || key == "" {
		return &EditPlanValidationError{Code: "invalid_asset_ref", Message: "asset fileId and storageKey are required"}
	}
	if looksLikeFilesystemPath(key) || looksLikeFilterExpression(key) {
		return &EditPlanValidationError{Code: "unsafe_asset_ref", Message: "asset storageKey must be an object key, not a path or filter"}
	}
	if looksLikeFilesystemPath(fileID) || looksLikeFilterExpression(fileID) {
		return &EditPlanValidationError{Code: "unsafe_asset_ref", Message: "asset fileId must be an opaque id"}
	}
	return nil
}

func validateManifestArtifactIDs(voiceFileID, captionFileID string) error {
	for _, id := range []string{voiceFileID, captionFileID} {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if looksLikeFilesystemPath(id) || looksLikeFilterExpression(id) {
			return &EditPlanValidationError{Code: "unsafe_artifact_ref", Message: "voice/caption must be opaque file ids"}
		}
	}
	return nil
}

func validateCompiledManifestEncoding(manifest RenderManifestV1) error {
	out := manifest.Output
	if !allowedManifestVideoCodecs[out.VideoCodec] {
		return &EditPlanValidationError{Code: "invalid_encoding", Message: "video codec is not allowed"}
	}
	if !allowedManifestAudioCodecs[out.AudioCodec] {
		return &EditPlanValidationError{Code: "invalid_encoding", Message: "audio codec is not allowed"}
	}
	if !allowedManifestFormats[out.Format] {
		return &EditPlanValidationError{Code: "invalid_encoding", Message: "output format is not allowed"}
	}
	if !allowedManifestPixelFmts[out.PixelFormat] {
		return &EditPlanValidationError{Code: "invalid_encoding", Message: "pixel format is not allowed"}
	}
	if out.FrameRate != 30 {
		return &EditPlanValidationError{Code: "invalid_encoding", Message: "frame rate must be 30"}
	}
	return nil
}

func looksLikeFilesystemPath(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "file:") || strings.HasPrefix(lower, "http:") || strings.HasPrefix(lower, "https:") {
		return true
	}
	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, "\\") {
		return true
	}
	if len(value) >= 3 && value[1] == ':' && (value[2] == '\\' || value[2] == '/') {
		return true
	}
	if strings.Contains(value, "..") {
		return true
	}
	return false
}

func looksLikeFilterExpression(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return false
	}
	markers := []string{
		"xfade=", "scale=", "crop=", "pad=", "amix=", "drawtext=", "subtitles=",
		"fontfile=", "filter_complex", "[0:v]", "[0:a]",
	}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func resolutionPixels(aspectRatio, resolution string) (int, int) {
	switch resolution {
	case TargetResolution720p:
		if aspectRatio == TargetAspectRatio9x16 {
			return 720, 1280
		}
		return 1280, 720
	default:
		if aspectRatio == TargetAspectRatio9x16 {
			return 1080, 1920
		}
		return 1920, 1080
	}
}

func defaultBitrate(resolution string) string {
	if resolution == TargetResolution720p {
		return "2500k"
	}
	return "4500k"
}

func ownedAssetMap(assets []ProjectAsset) map[string]ProjectAsset {
	out := make(map[string]ProjectAsset, len(assets))
	for _, asset := range assets {
		out[asset.ID] = asset
	}
	return out
}

func requireOwnedPlan(plan *EditPlanV1, assets []ProjectAsset) error {
	if plan == nil {
		return &EditPlanValidationError{Code: "missing_plan", Message: "plan is required"}
	}
	owned := ownedAssetMap(assets)
	*plan = NormalizeEditPlanV1(*plan, owned)
	if err := ValidateEditPlanV1(*plan, owned); err != nil {
		return err
	}
	if err := ValidateEditPlanContent(*plan); err != nil {
		return err
	}
	return nil
}

func ensureManifestStable(manifest RenderManifestV1) error {
	hash, err := HashRenderManifest(manifest)
	if err != nil {
		return err
	}
	if manifest.ManifestHash == "" {
		return fmt.Errorf("%w: missing manifest hash", ErrInvalidInput)
	}
	if hash != manifest.ManifestHash {
		return fmt.Errorf("%w: manifest hash mismatch", ErrInvalidInput)
	}
	return nil
}
