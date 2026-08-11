package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const maxShareVideoNormalizeBytes = 512 << 20

var (
	videoShareLookPath = exec.LookPath
	videoShareCommand  = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, name, args...)
		return cmd.CombinedOutput()
	}
)

func isVideoAsset(item asset) bool {
	if strings.EqualFold(strings.TrimSpace(item.MediaType), "video") {
		return true
	}
	contentType := strings.ToLower(stringMetadataValue(item, "contentType"))
	return strings.HasPrefix(contentType, "video/")
}

func sanitizeVideoDownloadFilename(filename string) string {
	name := strings.TrimSpace(filename)
	name = regexpReplaceInvalidFilenameChars(name)
	name = strings.Trim(name, ". ")
	if name == "" {
		return "video.mp4"
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4", ".m4v", ".mov", ".webm", ".avi", ".mkv", ".mpeg", ".mpg":
		name = strings.TrimSuffix(name, filepath.Ext(name))
	}
	name = strings.Trim(name, ". ")
	if name == "" {
		name = "video"
	}
	return name + ".mp4"
}

func regexpReplaceInvalidFilenameChars(name string) string {
	var builder strings.Builder
	builder.Grow(len(name))
	for _, r := range name {
		switch r {
		case '\\', '/', ':', '*', '?', '"', '<', '>', '|':
			builder.WriteByte('-')
		default:
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func sourceLooksLikeM4V(sourceURL string, contentType string) bool {
	lowerType := strings.ToLower(strings.TrimSpace(contentType))
	if strings.Contains(lowerType, "m4v") || strings.Contains(lowerType, "quicktime") {
		return true
	}
	path := strings.ToLower(strings.TrimSpace(sourceURL))
	if query := strings.IndexByte(path, '?'); query >= 0 {
		path = path[:query]
	}
	return strings.HasSuffix(path, ".m4v") || strings.HasSuffix(path, ".mov")
}

func normalizeVideoBytesForShare(ctx context.Context, raw []byte, sourceURL string) ([]byte, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty video payload")
	}
	if len(raw) > maxShareVideoNormalizeBytes {
		return raw, nil
	}
	ffmpegPath, err := videoShareLookPath(firstNonEmptyString(os.Getenv("SMARTVIDEO_FFMPEG_PATH"), "ffmpeg"))
	if err != nil {
		return raw, nil
	}
	ffprobePath, probeErr := videoShareLookPath(firstNonEmptyString(os.Getenv("SMARTVIDEO_FFPROBE_PATH"), "ffprobe"))

	dir, err := os.MkdirTemp("", "xz-video-share-*")
	if err != nil {
		return raw, err
	}
	defer os.RemoveAll(dir)

	inputExt := ".mp4"
	if sourceLooksLikeM4V(sourceURL, "") {
		inputExt = ".m4v"
	}
	inputPath := filepath.Join(dir, "input"+inputExt)
	outputPath := filepath.Join(dir, "share.mp4")
	if err := os.WriteFile(inputPath, raw, 0o600); err != nil {
		return raw, err
	}

	codec := ""
	if probeErr == nil {
		codec = probeShareVideoCodec(ctx, ffprobePath, inputPath)
	}
	needsTranscode := codecNeedsWeChatTranscode(codec)

	if !needsTranscode {
		if remuxed, remuxErr := runShareFFmpeg(ctx, ffmpegPath, outputPath,
			"-y", "-i", inputPath, "-c", "copy", "-movflags", "+faststart", outputPath,
		); remuxErr == nil && len(remuxed) > 0 {
			return remuxed, nil
		}
	}

	transcoded, transcodeErr := runShareFFmpeg(ctx, ffmpegPath, outputPath,
		"-y", "-i", inputPath,
		"-c:v", "libx264", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-movflags", "+faststart",
		outputPath,
	)
	if transcodeErr != nil {
		if !needsTranscode {
			return raw, nil
		}
		return raw, transcodeErr
	}
	if len(transcoded) == 0 {
		return raw, nil
	}
	return transcoded, nil
}

func codecNeedsWeChatTranscode(codec string) bool {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "hevc", "h265", "av1", "vp9", "vp8", "mpeg4", "mpeg2video":
		return true
	default:
		return false
	}
}

func probeShareVideoCodec(ctx context.Context, ffprobePath, inputPath string) string {
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := videoShareCommand(probeCtx, ffprobePath,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_name",
		"-of", "json",
		inputPath,
	)
	if err != nil || len(out) == 0 {
		return ""
	}
	var payload struct {
		Streams []struct {
			CodecName string `json:"codec_name"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return ""
	}
	if len(payload.Streams) == 0 {
		return ""
	}
	return strings.TrimSpace(payload.Streams[0].CodecName)
}

func runShareFFmpeg(ctx context.Context, ffmpegPath, outputPath string, args ...string) ([]byte, error) {
	runCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if _, err := videoShareCommand(runCtx, ffmpegPath, args...); err != nil {
		return nil, err
	}
	out, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("ffmpeg produced empty output")
	}
	return out, nil
}
