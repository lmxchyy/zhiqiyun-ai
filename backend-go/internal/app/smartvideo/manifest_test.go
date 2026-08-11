package smartvideo

import (
	"errors"
	"strings"
	"testing"
)

func manifestAssetsPortrait() []ProjectAsset {
	return []ProjectAsset{
		{
			ID: "asset-video", FileID: "file_video_1", StorageKey: "tenants/t1/assets/video1.mp4",
			AssetType: "VIDEO", OrderIndex: 0,
			Metadata: AssetMetadata{FileHash: "hash_video_1", DurationMS: 60000, Width: 1080, Height: 1920},
			NormalizedMetadata: &NormalizedMediaMetadata{
				Kind: "video",
				Video: &VideoMetadata{DurationMS: 60000, Width: 1080, Height: 1920},
			},
		},
		{
			ID: "asset-image", FileID: "file_image_1", StorageKey: "tenants/t1/assets/image1.jpg",
			AssetType: "IMAGE", OrderIndex: 1,
			Metadata: AssetMetadata{FileHash: "hash_image_1", Width: 1080, Height: 1920},
			NormalizedMetadata: &NormalizedMediaMetadata{
				Kind: "image",
				Image: &ImageMetadata{Width: 1080, Height: 1920},
			},
		},
	}
}

func manifestAssetsLandscape() []ProjectAsset {
	return []ProjectAsset{
		{
			ID: "asset-video", FileID: "file_video_1", StorageKey: "tenants/t1/assets/video1.mp4",
			AssetType: "VIDEO", OrderIndex: 0,
			Metadata: AssetMetadata{FileHash: "hash_video_1", DurationMS: 60000, Width: 1920, Height: 1080},
			NormalizedMetadata: &NormalizedMediaMetadata{
				Kind: "video",
				Video: &VideoMetadata{DurationMS: 60000, Width: 1920, Height: 1080},
			},
		},
		{
			ID: "asset-image", FileID: "file_image_1", StorageKey: "tenants/t1/assets/image1.jpg",
			AssetType: "IMAGE", OrderIndex: 1,
			Metadata: AssetMetadata{FileHash: "hash_image_1", Width: 1920, Height: 1080},
			NormalizedMetadata: &NormalizedMediaMetadata{
				Kind: "image",
				Image: &ImageMetadata{Width: 1920, Height: 1080},
			},
		},
	}
}

func canaryPlan(aspect, resolution string) EditPlanV1 {
	plan := makeValidEditPlanV1()
	plan.Target.AspectRatio = aspect
	plan.Target.Resolution = resolution
	plan.Scenes[0].Clips[0].AssetID = "asset-video"
	plan.Scenes[0].Clips[0].AssetType = "video"
	plan.Scenes[0].Clips[0].SourceInMs = 1000
	plan.Scenes[0].Clips[0].SourceOutMs = 16000
	plan.Scenes[0].Clips[0].Motion = MotionPanLeft
	plan.Scenes[0].Clips[0].OriginalAudioGain = 0.4567
	plan.Scenes[0].Transition = SceneTransitionV1{Type: TransitionTypeDissolve, DurationMs: 500}
	plan.Scenes[1].Clips[0].AssetID = "asset-image"
	plan.Scenes[1].Clips[0].AssetType = "image"
	plan.Scenes[1].Clips[0].Motion = MotionPush
	plan.Scenes[1].Clips[0].OriginalAudioGain = 0
	plan.Audio = AudioConfig{SourceGain: 0.3333, VoiceGain: 0.8888}
	plan.Voice.Enabled = true
	plan.Subtitles.Enabled = true
	return plan
}

func TestCompileRenderManifestCanaries(t *testing.T) {
	cases := []struct {
		name       string
		aspect     string
		resolution string
		assets     []ProjectAsset
		wantW      int
		wantH      int
	}{
		{"9x16_1080p", TargetAspectRatio9x16, TargetResolution1080p, manifestAssetsPortrait(), 1080, 1920},
		{"16x9_720p", TargetAspectRatio16x9, TargetResolution720p, manifestAssetsLandscape(), 1280, 720},
		{"16x9_1080p", TargetAspectRatio16x9, TargetResolution1080p, manifestAssetsLandscape(), 1920, 1080},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan := canaryPlan(tc.aspect, tc.resolution)
			first, err := CompileRenderManifest(RenderManifestInput{
				Version: ProjectVersion{PlanSnapshot: plan},
				Assets:  tc.assets,
				VoiceFileID: "file_voice_1", CaptionFileID: "file_caption_1",
			})
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			second, err := CompileRenderManifest(RenderManifestInput{
				Version: ProjectVersion{PlanSnapshot: plan},
				Assets:  reverseAssets(tc.assets),
				VoiceFileID: "file_voice_1", CaptionFileID: "file_caption_1",
			})
			if err != nil {
				t.Fatalf("compile shuffled: %v", err)
			}
			if first.ManifestHash == "" || first.ManifestHash != second.ManifestHash {
				t.Fatalf("hash unstable: %s vs %s", first.ManifestHash, second.ManifestHash)
			}
			if first.Output.Width != tc.wantW || first.Output.Height != tc.wantH {
				t.Fatalf("output size = %dx%d, want %dx%d", first.Output.Width, first.Output.Height, tc.wantW, tc.wantH)
			}
			if first.Scenes[0].Cuts[0].SourceInMs != 1000 || first.Scenes[0].Cuts[0].Motion != MotionPanLeft {
				t.Fatalf("video crop/motion missing: %+v", first.Scenes[0].Cuts[0])
			}
			if first.Scenes[0].Cuts[0].AudioGain != 0.457 {
				t.Fatalf("gain not normalized: %v", first.Scenes[0].Cuts[0].AudioGain)
			}
			if first.Scenes[0].Transition.Type != TransitionTypeDissolve || first.Scenes[0].Transition.DurationMs != 500 {
				t.Fatalf("transition missing: %+v", first.Scenes[0].Transition)
			}
			if first.AudioMix.SourceGain != 0.333 || first.AudioMix.VoiceGain != 0.889 {
				t.Fatalf("audio mix not normalized: %+v", first.AudioMix)
			}
			if first.Scenes[0].VoiceSegment == nil || first.Scenes[0].VoiceSegment.StartMs != 0 {
				t.Fatalf("voice segment timeline missing: %+v", first.Scenes[0].VoiceSegment)
			}
			// Transition overlap pulls next scene voice start earlier than raw sum.
			if first.Scenes[1].VoiceSegment == nil || first.Scenes[1].VoiceSegment.StartMs != 14500 {
				t.Fatalf("overlap timeline = %v, want 14500", first.Scenes[1].VoiceSegment)
			}
			if first.Scenes[1].Cuts[0].Motion != MotionPush {
				t.Fatalf("image motion missing: %+v", first.Scenes[1].Cuts[0])
			}
			if err := ensureManifestStable(first); err != nil {
				t.Fatalf("stable check: %v", err)
			}
		})
	}
}

func reverseAssets(assets []ProjectAsset) []ProjectAsset {
	out := append([]ProjectAsset{}, assets...)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func TestCompileRenderManifestRejectsUnsafeRefs(t *testing.T) {
	plan := canaryPlan(TargetAspectRatio16x9, TargetResolution1080p)
	assets := manifestAssetsLandscape()
	assets[0].StorageKey = `C:\secret\video.mp4`
	_, err := CompileRenderManifest(RenderManifestInput{
		Version: ProjectVersion{PlanSnapshot: plan}, Assets: assets,
	})
	var validation *EditPlanValidationError
	if !errors.As(err, &validation) || validation.Code != "unsafe_asset_ref" {
		t.Fatalf("path rejection error = %v", err)
	}

	assets = manifestAssetsLandscape()
	assets[0].StorageKey = "scale=1920:1080,xfade=transition=fade"
	_, err = CompileRenderManifest(RenderManifestInput{
		Version: ProjectVersion{PlanSnapshot: plan}, Assets: assets,
	})
	if !errors.As(err, &validation) || validation.Code != "unsafe_asset_ref" {
		t.Fatalf("filter rejection error = %v", err)
	}

	assets = manifestAssetsLandscape()
	_, err = CompileRenderManifest(RenderManifestInput{
		Version: ProjectVersion{PlanSnapshot: plan}, Assets: assets,
		VoiceFileID: "fontfile=/usr/share/fonts/evil.ttf",
	})
	if !errors.As(err, &validation) || validation.Code != "unsafe_artifact_ref" {
		t.Fatalf("artifact rejection error = %v", err)
	}
}

func TestCompileRenderManifestLocksEncoding(t *testing.T) {
	plan := canaryPlan(TargetAspectRatio16x9, TargetResolution1080p)
	manifest, err := CompileRenderManifest(RenderManifestInput{
		Version: ProjectVersion{PlanSnapshot: plan},
		Assets:  manifestAssetsLandscape(),
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if manifest.Output.VideoCodec != "libx264" || manifest.Output.AudioCodec != "aac" ||
		manifest.Output.PixelFormat != "yuv420p" || manifest.Output.Format != "mp4" || manifest.Output.FrameRate != 30 {
		t.Fatalf("encoding escaped server lock: %+v", manifest.Output)
	}
}

func TestCompileRenderManifestIsDeterministicFromVersionServiceFixture(t *testing.T) {
	version := ProjectVersion{PlanSnapshot: makeValidEditPlanV1()}
	assets := []ProjectAsset{}
	for id, asset := range ownedAssets() {
		asset.ID = id
		asset.FileID = "file_" + id
		asset.StorageKey = "obj/" + id
		assets = append(assets, asset)
	}
	first, err := CompileRenderManifest(RenderManifestInput{Version: version, Assets: assets})
	if err != nil {
		t.Fatalf("compile first: %v", err)
	}
	second, err := CompileRenderManifest(RenderManifestInput{Version: version, Assets: assets})
	if err != nil {
		t.Fatalf("compile second: %v", err)
	}
	if first.ManifestHash == "" || first.ManifestHash != second.ManifestHash {
		t.Fatalf("hash not stable: %s vs %s", first.ManifestHash, second.ManifestHash)
	}
	if strings.Contains(first.ManifestHash, " ") {
		t.Fatalf("hash should be hex, got %q", first.ManifestHash)
	}
}
