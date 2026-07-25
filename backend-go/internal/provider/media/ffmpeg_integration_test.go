package media

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/smartvideo"
)

func TestFFmpegOptionalProbeThumbnailAndProxyIntegration(t *testing.T) {
	if os.Getenv("SMARTVIDEO_FFMPEG_INTEGRATION") != "1" {
		t.Skip("SMARTVIDEO_FFMPEG_INTEGRATION=1 is not configured")
	}
	ffmpegPath := os.Getenv("SMARTVIDEO_FFMPEG_PATH")
	ffprobePath := os.Getenv("SMARTVIDEO_FFPROBE_PATH")
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	if ffprobePath == "" {
		ffprobePath = "ffprobe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	root := t.TempDir()
	source := filepath.Join(root, "source.mp4")
	result, err := (ExecCommandRunner{}).Run(ctx, ffmpegPath, []string{
		"-nostdin", "-y", "-v", "error",
		"-f", "lavfi", "-i", "color=c=blue:s=320x240:d=1",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", source,
	}, 64<<10, 64<<10)
	if err != nil {
		t.Fatalf("create tiny integration media: %v stderr=%s", err, string(result.Stderr))
	}
	adapter := NewFFmpegAdapter(ExecCommandRunner{}, ffprobePath, ffmpegPath, 10*time.Second, 20*time.Second)
	metadata, _, err := adapter.ProbeVideo(ctx, smartvideo.LocalMedia{Path: source, AssetType: smartvideo.AssetTypeVideo})
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Width != 320 || metadata.Height != 240 || metadata.DurationMS < 900 || !metadata.HasAudio {
		t.Fatalf("unexpected integration metadata: %+v", metadata)
	}
	thumbnail := filepath.Join(root, "thumbnail.jpg")
	if err := adapter.GenerateThumbnail(ctx, smartvideo.LocalMedia{Path: source, AssetType: smartvideo.AssetTypeVideo}, thumbnail, smartvideo.ThumbnailOptions{
		MaxWidth: 160, MaxHeight: 160, Quality: 4,
	}); err != nil {
		t.Fatal(err)
	}
	proxy := filepath.Join(root, "proxy.mp4")
	if err := adapter.GenerateProxy(ctx, smartvideo.LocalMedia{Path: source, AssetType: smartvideo.AssetTypeVideo}, proxy, smartvideo.ProxyOptions{
		MaxWidth: 160, VideoBitrate: "300k", AudioBitrate: "48k",
	}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{thumbnail, proxy} {
		info, err := os.Stat(path)
		if err != nil || info.Size() <= 0 {
			t.Fatalf("invalid generated file %s: info=%v err=%v", filepath.Base(path), info, err)
		}
	}
}
