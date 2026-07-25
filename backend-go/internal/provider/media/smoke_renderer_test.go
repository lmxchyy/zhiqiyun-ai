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

func TestBuildSmokeRenderArgsDoesNotUseShell(t *testing.T) {
	args, err := BuildSmokeRenderArgs(`C:\fonts\safe.ttf`, `C:\work\out;touch-pwned.mp4`)
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
}

func TestSmokeRendererProducesValidatedMP4(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe unavailable")
	}
	font := os.Getenv("SMARTVIDEO_SMOKE_FONT")
	if font == "" {
		t.Skip("SMARTVIDEO_SMOKE_FONT is not set")
	}
	tools := NewFFmpegAdapter(ExecCommandRunner{}, "ffprobe", "ffmpeg", 30*time.Second, 2*time.Minute)
	renderer := &SmokeRenderer{Tools: tools, FontPath: font, MaxOutputBytes: 64 << 20}
	artifact, err := renderer.Render(context.Background(), smartvideo.RenderTask{Specification: smartvideo.RenderSpecification{
		Width: 1080, Height: 1920, FrameRate: 30, DurationMS: 5000,
		Format: "mp4", VideoCodec: "h264", AudioCodec: "aac",
	}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if artifact.DurationMS < 4900 || artifact.DurationMS > 5100 || artifact.Width != 1080 ||
		artifact.Height != 1920 || artifact.FrameRate != 30 || artifact.VideoCodec != "h264" ||
		artifact.AudioCodec != "aac" || artifact.PixelFormat != "yuv420p" {
		t.Fatalf("unexpected artifact: %+v", artifact)
	}
	for _, path := range []string{artifact.VideoPath, artifact.CoverPath} {
		if info, err := os.Stat(filepath.Clean(path)); err != nil || info.Size() == 0 {
			t.Fatalf("missing artifact %s: %v", path, err)
		}
	}
}
