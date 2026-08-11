package httpserver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeVideoDownloadFilenameForcesMP4(t *testing.T) {
	cases := map[string]string{
		"": "video.mp4",
		"aa95a59232ae386532bb86581a8e55067cca10e5c8c3d27b74ad3ad69bfb5594.m4v": "aa95a59232ae386532bb86581a8e55067cca10e5c8c3d27b74ad3ad69bfb5594.mp4",
		"clip.MOV":               "clip.mp4",
		"demo.mp4":               "demo.mp4",
		`bad/name:with*chars.m4v`: "bad-name-with-chars.mp4",
	}
	for input, want := range cases {
		if got := sanitizeVideoDownloadFilename(input); got != want {
			t.Fatalf("sanitizeVideoDownloadFilename(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestCodecNeedsWeChatTranscode(t *testing.T) {
	if !codecNeedsWeChatTranscode("hevc") || !codecNeedsWeChatTranscode("h265") {
		t.Fatal("hevc should require transcode")
	}
	if codecNeedsWeChatTranscode("h264") || codecNeedsWeChatTranscode("avc1") {
		t.Fatal("h264 should not require transcode")
	}
}

func TestNormalizeVideoBytesForShareFallsBackWithoutFFmpeg(t *testing.T) {
	prev := videoShareLookPath
	t.Cleanup(func() { videoShareLookPath = prev })
	videoShareLookPath = func(string) (string, error) { return "", errors.New("missing") }

	raw := []byte("fake-m4v-bytes")
	got, err := normalizeVideoBytesForShare(context.Background(), raw, "https://cdn.example/a.m4v")
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if string(got) != string(raw) {
		t.Fatalf("expected original bytes when ffmpeg missing")
	}
}

func TestNormalizeVideoBytesForShareRemuxesWithFFmpeg(t *testing.T) {
	prevLook := videoShareLookPath
	prevCmd := videoShareCommand
	t.Cleanup(func() {
		videoShareLookPath = prevLook
		videoShareCommand = prevCmd
	})

	videoShareLookPath = func(file string) (string, error) {
		switch filepath.Base(file) {
		case "ffmpeg", "ffprobe":
			return file, nil
		default:
			return file, nil
		}
	}
	videoShareCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if filepath.Base(name) == "ffprobe" {
			return []byte(`{"streams":[{"codec_name":"h264"}]}`), nil
		}
		output := args[len(args)-1]
		if err := os.WriteFile(output, []byte("remuxed-mp4"), 0o600); err != nil {
			return nil, err
		}
		return []byte("ok"), nil
	}

	got, err := normalizeVideoBytesForShare(context.Background(), []byte("source-m4v"), "https://cdn.example/hash.m4v")
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if string(got) != "remuxed-mp4" {
		t.Fatalf("got %q, want remuxed-mp4", got)
	}
}

func TestDownloadAssetNameStripsM4V(t *testing.T) {
	name := downloadAssetName(asset{
		ID:        "asset_1",
		Name:      "aa95a59232ae386532bb86581a8e55067cca10e5c8c3d27b74ad3ad69bfb5594.m4v",
		MediaType: "video",
	}, "video/mp4")
	if name != "aa95a59232ae386532bb86581a8e55067cca10e5c8c3d27b74ad3ad69bfb5594.mp4" {
		t.Fatalf("downloadAssetName = %q", name)
	}
}
