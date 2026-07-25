package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/app/smartvideo"
)

const (
	maxProbeStdout = 1 << 20
	maxToolStdout  = 64 << 10
	maxToolStderr  = 32 << 10
)

type FFmpegAdapter struct {
	Runner         CommandRunner
	FFprobePath    string
	FFmpegPath     string
	ProbeTimeout   time.Duration
	ProcessTimeout time.Duration
}

func NewFFmpegAdapter(runner CommandRunner, ffprobePath, ffmpegPath string, probeTimeout, processTimeout time.Duration) *FFmpegAdapter {
	if runner == nil {
		runner = ExecCommandRunner{}
	}
	if strings.TrimSpace(ffprobePath) == "" {
		ffprobePath = "ffprobe"
	}
	if strings.TrimSpace(ffmpegPath) == "" {
		ffmpegPath = "ffmpeg"
	}
	if probeTimeout <= 0 {
		probeTimeout = 30 * time.Second
	}
	if processTimeout <= 0 {
		processTimeout = 5 * time.Minute
	}
	return &FFmpegAdapter{
		Runner: runner, FFprobePath: ffprobePath, FFmpegPath: ffmpegPath,
		ProbeTimeout: probeTimeout, ProcessTimeout: processTimeout,
	}
}

type probeDocument struct {
	Streams []probeStream `json:"streams"`
	Format  probeFormat   `json:"format"`
}

type probeStream struct {
	CodecType    string          `json:"codec_type"`
	CodecName    string          `json:"codec_name"`
	PixelFormat  string          `json:"pix_fmt"`
	Width        int             `json:"width"`
	Height       int             `json:"height"`
	AverageFPS   string          `json:"avg_frame_rate"`
	RealFPS      string          `json:"r_frame_rate"`
	Bitrate      string          `json:"bit_rate"`
	SampleRate   string          `json:"sample_rate"`
	Channels     int             `json:"channels"`
	FrameCount   string          `json:"nb_frames"`
	Tags         probeStreamTags `json:"tags"`
	SideDataList []probeSideData `json:"side_data_list"`
}

type probeStreamTags struct {
	Rotate      string `json:"rotate"`
	Orientation string `json:"orientation"`
}

type probeSideData struct {
	Rotation int `json:"rotation"`
}

type probeFormat struct {
	Name     string          `json:"format_name"`
	LongName string          `json:"format_long_name"`
	Duration string          `json:"duration"`
	Bitrate  string          `json:"bit_rate"`
	Tags     probeFormatTags `json:"tags"`
}

type probeFormatTags struct {
	Title            string `json:"title"`
	Encoder          string `json:"encoder"`
	CreationTime     string `json:"creation_time"`
	MajorBrand       string `json:"major_brand"`
	CompatibleBrands string `json:"compatible_brands"`
}

func (a *FFmpegAdapter) ProbeVideo(ctx context.Context, input smartvideo.LocalMedia) (smartvideo.VideoMetadata, smartvideo.FilteredProbeResult, error) {
	document, version, err := a.probe(ctx, input.Path)
	if err != nil {
		return smartvideo.VideoMetadata{}, smartvideo.FilteredProbeResult{}, err
	}
	var video *probeStream
	var audio *probeStream
	codecs := []string{}
	for index := range document.Streams {
		stream := &document.Streams[index]
		if stream.CodecName != "" {
			codecs = append(codecs, stream.CodecType+":"+stream.CodecName)
		}
		if stream.CodecType == "video" && video == nil {
			video = stream
		}
		if stream.CodecType == "audio" && audio == nil {
			audio = stream
		}
	}
	if video == nil {
		return smartvideo.VideoMetadata{}, smartvideo.FilteredProbeResult{}, mediaError(smartvideo.MediaErrorNoVideoStream, "视频文件中没有可用的视频流", nil)
	}
	rotation := streamRotation(*video)
	displayWidth, displayHeight := video.Width, video.Height
	if normalizedRotation(rotation) == 90 || normalizedRotation(rotation) == 270 {
		displayWidth, displayHeight = video.Height, video.Width
	}
	numerator, denominator := parseRatio(firstNonZeroRatio(video.AverageFPS, video.RealFPS))
	metadata := smartvideo.VideoMetadata{
		Format: document.Format.Name, MIMEType: videoMIME(document.Format.Name),
		DurationMS: durationMilliseconds(document.Format.Duration),
		Width:      video.Width, Height: video.Height, DisplayWidth: displayWidth, DisplayHeight: displayHeight,
		Rotation: rotation, FPSNumerator: numerator, FPSDenominator: denominator,
		VideoCodec: video.CodecName, PixelFormat: video.PixelFormat,
		Bitrate:  firstPositive(parseInt64(video.Bitrate), parseInt64(document.Format.Bitrate)),
		HasAudio: audio != nil, ProbeVersion: version,
		Container: smartvideo.ContainerMetadata{
			Title: document.Format.Tags.Title, Encoder: document.Format.Tags.Encoder,
			CreationTime: document.Format.Tags.CreationTime, MajorBrand: document.Format.Tags.MajorBrand,
			Compatible: document.Format.Tags.CompatibleBrands,
		},
	}
	if audio != nil {
		metadata.AudioCodec = audio.CodecName
		metadata.AudioSampleRate = int(parseInt64(audio.SampleRate))
		metadata.AudioChannels = audio.Channels
	}
	filtered := smartvideo.FilteredProbeResult{
		FormatName: document.Format.Name, FormatLongName: document.Format.LongName, StreamCodecs: codecs,
	}
	if video.AverageFPS != "" && video.RealFPS != "" && video.AverageFPS != video.RealFPS {
		filtered.Warnings = []string{"variable_frame_rate_detected"}
	}
	return metadata, filtered, nil
}

func (a *FFmpegAdapter) ProbeImage(ctx context.Context, input smartvideo.LocalMedia) (smartvideo.ImageMetadata, smartvideo.FilteredProbeResult, error) {
	document, version, err := a.probe(ctx, input.Path)
	if err != nil {
		return smartvideo.ImageMetadata{}, smartvideo.FilteredProbeResult{}, err
	}
	var image *probeStream
	for index := range document.Streams {
		if document.Streams[index].CodecType == "video" {
			image = &document.Streams[index]
			break
		}
	}
	if image == nil || image.Width <= 0 || image.Height <= 0 {
		return smartvideo.ImageMetadata{}, smartvideo.FilteredProbeResult{}, mediaError(smartvideo.MediaErrorUnsupported, "图片文件无法识别", nil)
	}
	frameCount := int(parseInt64(image.FrameCount))
	if frameCount <= 0 {
		frameCount = 1
	}
	orientation := int(parseInt64(image.Tags.Orientation))
	if orientation <= 0 {
		orientation = 1
	}
	metadata := smartvideo.ImageMetadata{
		Format: image.CodecName, MIMEType: imageMIME(image.CodecName),
		Width: image.Width, Height: image.Height, Orientation: orientation,
		Animated: frameCount > 1, FrameCount: frameCount, ColorSpace: image.PixelFormat, ProbeVersion: version,
	}
	return metadata, smartvideo.FilteredProbeResult{
		FormatName: document.Format.Name, FormatLongName: document.Format.LongName,
		StreamCodecs: []string{"video:" + image.CodecName},
	}, nil
}

func (a *FFmpegAdapter) GenerateThumbnail(ctx context.Context, input smartvideo.LocalMedia, outputPath string, options smartvideo.ThumbnailOptions) error {
	if options.MaxWidth <= 0 || options.MaxHeight <= 0 || strings.TrimSpace(outputPath) == "" {
		return mediaError(smartvideo.MediaErrorPreprocessFailed, "缩略图参数无效", nil)
	}
	args := []string{
		"-nostdin", "-y", "-v", "error", "-i", input.Path,
		"-frames:v", "1", "-vf", fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease", options.MaxWidth, options.MaxHeight),
		"-q:v", strconv.Itoa(clamp(options.Quality, 2, 31)), outputPath,
	}
	return a.runProcess(ctx, args, "缩略图生成失败")
}

func (a *FFmpegAdapter) GenerateProxy(ctx context.Context, input smartvideo.LocalMedia, outputPath string, options smartvideo.ProxyOptions) error {
	if options.MaxWidth <= 0 || !safeBitrate(options.VideoBitrate) || !safeBitrate(options.AudioBitrate) || strings.TrimSpace(outputPath) == "" {
		return mediaError(smartvideo.MediaErrorPreprocessFailed, "代理文件参数无效", nil)
	}
	args := []string{
		"-nostdin", "-y", "-v", "error", "-i", input.Path,
		"-map", "0:v:0", "-map", "0:a?", "-vf", fmt.Sprintf("scale=%d:-2:force_original_aspect_ratio=decrease", options.MaxWidth),
		"-c:v", "libx264", "-preset", "veryfast", "-b:v", options.VideoBitrate,
		"-c:a", "aac", "-b:a", options.AudioBitrate, "-movflags", "+faststart",
		"-metadata:s:v:0", "rotate=0", outputPath,
	}
	return a.runProcess(ctx, args, "低清代理生成失败")
}

func (a *FFmpegAdapter) GetToolVersion(ctx context.Context) (string, string, error) {
	probe, err := a.run(ctx, a.FFprobePath, []string{"-version"}, a.ProbeTimeout, maxToolStdout)
	if err != nil {
		return "", "", err
	}
	ffmpeg, err := a.run(ctx, a.FFmpegPath, []string{"-version"}, a.ProbeTimeout, maxToolStdout)
	if err != nil {
		return "", "", err
	}
	return firstLine(string(probe.Stdout)), firstLine(string(ffmpeg.Stdout)), nil
}

func (a *FFmpegAdapter) probe(ctx context.Context, path string) (probeDocument, string, error) {
	if strings.TrimSpace(path) == "" {
		return probeDocument{}, "", mediaError(smartvideo.MediaErrorProbeFailed, "媒体输入无效", nil)
	}
	args := []string{
		"-v", "error", "-print_format", "json", "-show_format", "-show_streams",
		"-show_entries", "stream=codec_type,codec_name,pix_fmt,width,height,avg_frame_rate,r_frame_rate,bit_rate,sample_rate,channels,nb_frames:stream_tags=rotate,orientation:stream_side_data=rotation:format=format_name,format_long_name,duration,bit_rate:format_tags=title,encoder,creation_time,major_brand,compatible_brands",
		path,
	}
	result, err := a.run(ctx, a.FFprobePath, args, a.ProbeTimeout, maxProbeStdout)
	if err != nil {
		return probeDocument{}, "", err
	}
	var document probeDocument
	if err := json.Unmarshal(result.Stdout, &document); err != nil {
		return probeDocument{}, "", mediaError(smartvideo.MediaErrorInvalidJSON, "媒体探测返回格式无效", err)
	}
	versionResult, err := a.run(ctx, a.FFprobePath, []string{"-version"}, a.ProbeTimeout, maxToolStdout)
	if err != nil {
		return probeDocument{}, "", err
	}
	return document, firstLine(string(versionResult.Stdout)), nil
}

func (a *FFmpegAdapter) runProcess(ctx context.Context, args []string, message string) error {
	_, err := a.run(ctx, a.FFmpegPath, args, a.ProcessTimeout, maxToolStdout)
	if err != nil {
		var mediaErr *smartvideo.MediaError
		if errors.As(err, &mediaErr) && mediaErr.Code == smartvideo.MediaErrorToolMissing {
			return err
		}
		return mediaError(smartvideo.MediaErrorPreprocessFailed, message, err)
	}
	return nil
}

func (a *FFmpegAdapter) run(ctx context.Context, executable string, args []string, timeout time.Duration, stdoutLimit int64) (CommandResult, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := a.Runner.Run(runCtx, executable, args, stdoutLimit, maxToolStderr)
	if err == nil {
		return result, nil
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return result, mediaError(smartvideo.MediaErrorTimeout, "媒体处理超时", err)
	}
	if errors.Is(err, os.ErrNotExist) {
		return result, mediaError(smartvideo.MediaErrorToolMissing, "媒体处理工具不可用", err)
	}
	if errors.Is(err, errOutputLimit) {
		return result, mediaError(smartvideo.MediaErrorProbeFailed, "媒体处理输出超过安全限制", err)
	}
	return result, mediaError(smartvideo.MediaErrorProbeFailed, "媒体处理工具执行失败", err)
}

func mediaError(code, message string, cause error) error {
	return &smartvideo.MediaError{Code: code, Message: message, Cause: cause}
}

func streamRotation(stream probeStream) int {
	for _, sideData := range stream.SideDataList {
		if sideData.Rotation != 0 {
			return sideData.Rotation
		}
	}
	return int(parseInt64(stream.Tags.Rotate))
}

func normalizedRotation(value int) int {
	value %= 360
	if value < 0 {
		value += 360
	}
	return value
}

func durationMilliseconds(raw string) int64 {
	value, _ := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	return int64(math.Round(value * 1000))
}

func parseRatio(raw string) (int64, int64) {
	parts := strings.Split(strings.TrimSpace(raw), "/")
	if len(parts) != 2 {
		return 0, 1
	}
	numerator, _ := strconv.ParseInt(parts[0], 10, 64)
	denominator, _ := strconv.ParseInt(parts[1], 10, 64)
	if denominator == 0 {
		denominator = 1
	}
	return numerator, denominator
}

func firstNonZeroRatio(values ...string) string {
	for _, value := range values {
		numerator, _ := parseRatio(value)
		if numerator > 0 {
			return value
		}
	}
	return "0/1"
}

func parseInt64(raw string) int64 {
	value, _ := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	return value
}

func firstPositive(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func videoMIME(format string) string {
	lower := strings.ToLower(format)
	switch {
	case strings.Contains(lower, "webm"):
		return "video/webm"
	case strings.Contains(lower, "quicktime"):
		return "video/quicktime"
	default:
		return "video/mp4"
	}
}

func imageMIME(format string) string {
	switch strings.ToLower(format) {
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

func safeBitrate(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) < 2 || len(value) > 12 {
		return false
	}
	suffix := value[len(value)-1]
	if suffix != 'k' && suffix != 'm' {
		return false
	}
	_, err := strconv.Atoi(value[:len(value)-1])
	return err == nil
}

func clamp(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func firstLine(value string) string {
	line := strings.TrimSpace(strings.SplitN(value, "\n", 2)[0])
	if len(line) > 256 {
		line = line[:256]
	}
	return line
}
