package media

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"xianzhi-ai/backend-go/internal/app/smartvideo"
)

type SmokeRenderer struct {
	Tools          *FFmpegAdapter
	FontPath       string
	MaxOutputBytes int64
}

func BuildSmokeRenderArgs(fontPath, outputPath string) ([]string, error) {
	if strings.TrimSpace(fontPath) == "" || strings.TrimSpace(outputPath) == "" {
		return nil, smartvideo.ErrInvalidInput
	}
	font := escapeFilterValue(fontPath)
	filter := "drawtext=fontfile='" + font + "':text='知启云AI':fontcolor=white:fontsize=108:x=(w-text_w)/2:y=620," +
		"drawtext=fontfile='" + font + "':text='让AI成为企业生产力':fontcolor=white@0.92:fontsize=48:x=(w-text_w)/2:y=790," +
		"drawtext=fontfile='" + font + "':text='企业级AI创作与智能体平台':fontcolor=white@0.88:fontsize=42:x=(w-text_w)/2:y=1500"
	return []string{
		"-nostdin", "-y", "-v", "error",
		"-f", "lavfi", "-i", "gradients=s=1080x1920:c0=0x6d28d9:c1=0x2563eb:x0=0:y0=0:x1=1080:y1=1920:d=5:r=30",
		"-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=48000",
		"-vf", filter, "-map", "0:v:0", "-map", "1:a:0", "-t", "5",
		"-r", "30", "-c:v", "libx264", "-preset", "medium", "-crf", "20",
		"-pix_fmt", "yuv420p", "-profile:v", "high", "-level", "4.0",
		"-c:a", "aac", "-b:a", "128k", "-ar", "48000", "-ac", "2",
		"-movflags", "+faststart", "-shortest", outputPath,
	}, nil
}

func (r *SmokeRenderer) Render(ctx context.Context, task smartvideo.RenderTask, workDir string) (smartvideo.RenderArtifact, error) {
	spec := task.Specification
	if spec.Width != 1080 || spec.Height != 1920 || spec.FrameRate != 30 || spec.DurationMS != 5000 ||
		strings.ToLower(spec.Format) != "mp4" || strings.ToLower(spec.VideoCodec) != "h264" ||
		strings.ToLower(spec.AudioCodec) != "aac" {
		return smartvideo.RenderArtifact{}, fmt.Errorf("%w: smoke render specification must be 1080x1920, 30fps, 5s, H.264/AAC MP4", smartvideo.ErrInvalidInput)
	}
	if r.Tools == nil {
		return smartvideo.RenderArtifact{}, fmt.Errorf("render tools unavailable")
	}
	if info, err := os.Stat(r.FontPath); err != nil || info.IsDir() {
		return smartvideo.RenderArtifact{}, fmt.Errorf("configured Chinese font unavailable")
	}
	videoPath := filepath.Join(workDir, "ZhiqiyunSmartVideoSmoke.mp4")
	args, err := BuildSmokeRenderArgs(r.FontPath, videoPath)
	if err != nil {
		return smartvideo.RenderArtifact{}, err
	}
	if _, err = r.Tools.Runner.Run(ctx, r.Tools.FFmpegPath, args, maxToolStdout, maxToolStderr); err != nil {
		return smartvideo.RenderArtifact{}, fmt.Errorf("SMARTVIDEO_RENDER_FFMPEG_FAILED: %w", err)
	}
	info, err := os.Stat(videoPath)
	if err != nil {
		return smartvideo.RenderArtifact{}, err
	}
	if r.MaxOutputBytes > 0 && info.Size() > r.MaxOutputBytes {
		return smartvideo.RenderArtifact{}, fmt.Errorf("SMARTVIDEO_RENDER_OUTPUT_TOO_LARGE")
	}
	metadata, _, err := r.Tools.ProbeVideo(ctx, smartvideo.LocalMedia{Path: videoPath, AssetType: smartvideo.AssetTypeVideo})
	if err != nil {
		return smartvideo.RenderArtifact{}, err
	}
	if metadata.Width != 1080 || metadata.Height != 1920 || metadata.VideoCodec != "h264" ||
		metadata.AudioCodec != "aac" || metadata.FPSNumerator*1 != 30*metadata.FPSDenominator ||
		metadata.DurationMS < 4900 || metadata.DurationMS > 5100 {
		return smartvideo.RenderArtifact{}, fmt.Errorf("SMARTVIDEO_RENDER_VALIDATION_FAILED")
	}
	coverPath := filepath.Join(workDir, "ZhiqiyunSmartVideoSmoke.jpg")
	coverArgs := []string{"-nostdin", "-y", "-v", "error", "-ss", "1", "-i", videoPath, "-frames:v", "1", "-q:v", strconv.Itoa(2), coverPath}
	if _, err = r.Tools.Runner.Run(ctx, r.Tools.FFmpegPath, coverArgs, maxToolStdout, maxToolStderr); err != nil {
		return smartvideo.RenderArtifact{}, fmt.Errorf("SMARTVIDEO_RENDER_COVER_FAILED: %w", err)
	}
	return smartvideo.RenderArtifact{
		VideoPath: videoPath, CoverPath: coverPath, DurationMS: metadata.DurationMS,
		Width: metadata.Width, Height: metadata.Height, FrameRate: 30, FileSize: info.Size(),
		VideoCodec: metadata.VideoCodec, AudioCodec: metadata.AudioCodec, PixelFormat: metadata.PixelFormat,
	}, nil
}

func escapeFilterValue(value string) string {
	value = strings.ReplaceAll(value, `\`, "/")
	value = strings.ReplaceAll(value, ":", `\:`)
	value = strings.ReplaceAll(value, "'", `\'`)
	return value
}
