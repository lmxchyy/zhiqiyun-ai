package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/smartvideo"
)

func sampleManifest(width, height int, durationMs int64) smartvideo.RenderManifestV1 {
	return smartvideo.RenderManifestV1{
		SchemaVersion: 1,
		Output: smartvideo.ManifestOutputSpec{
			Width: width, Height: height, FrameRate: 30,
			VideoCodec: "libx264", AudioCodec: "aac", PixelFormat: "yuv420p",
			Format: "mp4", Bitrate: "2500k",
		},
		Inputs: []smartvideo.ManifestInput{
			{FileID: "file_v1", ObjectKey: "obj/v1", Checksum: "abc", DurationMs: 5000, Width: width, Height: height, AssetType: "video"},
			{FileID: "file_i1", ObjectKey: "obj/i1", Checksum: "def", Width: width, Height: height, AssetType: "image"},
		},
		Scenes: []smartvideo.ManifestScene{
			{
				Index: 0, DurationMs: durationMs / 2,
				Cuts: []smartvideo.ManifestCut{{
					InputIndex: 0, SourceInMs: 0, SourceOutMs: durationMs / 2,
					FitMode: "cover", Motion: "static", AudioGain: 0.8,
					TargetWidth: width, TargetHeight: height,
				}},
				Transition: smartvideo.ManifestTrans{Type: "fade", DurationMs: 200},
			},
			{
				Index: 1, DurationMs: durationMs / 2,
				Cuts: []smartvideo.ManifestCut{{
					InputIndex: 1, SourceInMs: 0, SourceOutMs: 0,
					FitMode: "contain", Motion: "push", AudioGain: 0,
					TargetWidth: width, TargetHeight: height,
				}},
			},
		},
		AudioMix: smartvideo.ManifestAudioMix{SourceGain: 0.8, VoiceGain: 1},
	}
}

func TestBuildManifestRenderArgsDoesNotUseShell(t *testing.T) {
	manifest := sampleManifest(1280, 720, 2000)
	args, err := BuildManifestRenderArgs(
		manifest,
		[]LocalRenderInput{
			{Path: `C:\media\in;rm.mp4`, AssetType: "video"},
			{Path: `C:\media\img.jpg`, AssetType: "image"},
		},
		"", "",
		`C:\work\out;touch-pwned.mp4`,
	)
	if err != nil {
		t.Fatal(err)
	}
	if args[len(args)-1] != `C:\work\out;touch-pwned.mp4` {
		t.Fatalf("output path must remain one argv element: %#v", args[len(args)-1])
	}
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "sh -c") || strings.Contains(joined, "cmd /c") {
		t.Fatalf("shell invocation is forbidden: %s", joined)
	}
	filter := ""
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-filter_complex" {
			filter = args[i+1]
			break
		}
	}
	if !strings.Contains(filter, "xfade=") || !strings.Contains(filter, "scale=") {
		t.Fatalf("expected xfade/scale in filter graph: %s", filter)
	}
	if !strings.Contains(filter, "zoompan=") {
		t.Fatalf("expected ken burns motion filter: %s", filter)
	}
}

func TestBuildManifestRenderArgsRejectsEncodingEscape(t *testing.T) {
	manifest := sampleManifest(1280, 720, 2000)
	manifest.Output.VideoCodec = "libx265"
	_, err := BuildManifestRenderArgs(manifest, []LocalRenderInput{
		{Path: "a.mp4", AssetType: "video"}, {Path: "b.jpg", AssetType: "image"},
	}, "", "", "out.mp4")
	if err == nil {
		t.Fatal("expected encoding lock rejection")
	}
}

func TestManifestRendererProducesValidatedMP4(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	root := t.TempDir()
	video := filepath.Join(root, "clip.mp4")
	image := filepath.Join(root, "still.jpg")
	runner := ExecCommandRunner{}
	if _, err := runner.Run(ctx, "ffmpeg", []string{
		"-nostdin", "-y", "-v", "error",
		"-f", "lavfi", "-i", "color=c=blue:s=640x360:d=2:r=30",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=2",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", video,
	}, 64<<10, 64<<10); err != nil {
		t.Fatalf("create video fixture: %v", err)
	}
	if _, err := runner.Run(ctx, "ffmpeg", []string{
		"-nostdin", "-y", "-v", "error",
		"-f", "lavfi", "-i", "color=c=red:s=640x360:d=1",
		"-frames:v", "1", image,
	}, 64<<10, 64<<10); err != nil {
		t.Fatalf("create image fixture: %v", err)
	}

	manifest := sampleManifest(640, 360, 2000)
	tools := NewFFmpegAdapter(runner, "ffprobe", "ffmpeg", 30*time.Second, 2*time.Minute)
	renderer := &ManifestRenderer{Tools: tools, MaxOutputBytes: 64 << 20}
	artifact, err := renderer.RenderLocal(ctx, manifest, []LocalRenderInput{
		{Path: video, AssetType: "video"},
		{Path: image, AssetType: "image"},
	}, "", "", root)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if artifact.Width != 640 || artifact.Height != 360 || artifact.VideoCodec != "h264" || artifact.AudioCodec != "aac" {
		t.Fatalf("unexpected artifact: %+v", artifact)
	}
	for _, path := range []string{artifact.VideoPath, artifact.CoverPath} {
		info, err := os.Stat(path)
		if err != nil || info.Size() == 0 {
			t.Fatalf("missing artifact %s: %v", path, err)
		}
	}
}
