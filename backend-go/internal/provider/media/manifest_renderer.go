package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"xianzhi-ai/backend-go/internal/app/smartvideo"
)

type LocalRenderInput struct {
	Path      string
	AssetType string
}

type ManifestSource interface {
	LoadRenderManifest(ctx context.Context, access smartvideo.Access, projectID, versionID string) (smartvideo.RenderManifestV1, error)
}

type RenderMediaStore interface {
	OpenObject(ctx context.Context, access smartvideo.Access, fileID string) (io.ReadCloser, int64, string, error)
}

type ManifestRenderer struct {
	Tools          *FFmpegAdapter
	Manifests      ManifestSource
	Store          RenderMediaStore
	MaxOutputBytes int64
}

func (r *ManifestRenderer) Render(ctx context.Context, task smartvideo.RenderTask, workDir string) (smartvideo.RenderArtifact, error) {
	if r == nil || r.Tools == nil || r.Manifests == nil || r.Store == nil {
		return smartvideo.RenderArtifact{}, fmt.Errorf("manifest renderer is not configured")
	}
	access := smartvideo.Access{TenantID: task.TenantID, UserID: task.UserID}
	manifest, err := r.Manifests.LoadRenderManifest(ctx, access, task.ProjectID, task.VersionID)
	if err != nil {
		return smartvideo.RenderArtifact{}, err
	}
	if strings.TrimSpace(task.ManifestHash) != "" && manifest.ManifestHash != "" && task.ManifestHash != manifest.ManifestHash {
		return smartvideo.RenderArtifact{}, fmt.Errorf("%w: render task manifest hash mismatch", smartvideo.ErrInvalidInput)
	}
	if err := ensureManifestEncodingLocked(manifest); err != nil {
		return smartvideo.RenderArtifact{}, err
	}

	inputDir := filepath.Join(workDir, "inputs")
	if err := os.MkdirAll(inputDir, 0o700); err != nil {
		return smartvideo.RenderArtifact{}, err
	}
	locals := make([]LocalRenderInput, 0, len(manifest.Inputs))
	for index, input := range manifest.Inputs {
		dest := filepath.Join(inputDir, fmt.Sprintf("%02d_%s", index, sanitizeFileName(input.FileID)))
		if err := r.downloadChecked(ctx, access, input.FileID, input.Checksum, dest); err != nil {
			return smartvideo.RenderArtifact{}, err
		}
		locals = append(locals, LocalRenderInput{Path: dest, AssetType: input.AssetType})
	}
	voicePath := ""
	if id := firstNonEmpty(task.VoiceFileID, manifest.VoiceFileID); id != "" {
		voicePath = filepath.Join(workDir, "voice.wav")
		if err := r.downloadChecked(ctx, access, id, "", voicePath); err != nil {
			return smartvideo.RenderArtifact{}, err
		}
	}
	captionPath := ""
	if id := firstNonEmpty(task.CaptionFileID, manifest.CaptionFileID); id != "" {
		captionPath = filepath.Join(workDir, "captions.ass")
		if err := r.downloadChecked(ctx, access, id, "", captionPath); err != nil {
			return smartvideo.RenderArtifact{}, err
		}
	}
	return r.RenderLocal(ctx, manifest, locals, voicePath, captionPath, workDir)
}

func (r *ManifestRenderer) RenderLocal(
	ctx context.Context,
	manifest smartvideo.RenderManifestV1,
	inputs []LocalRenderInput,
	voicePath, captionPath, workDir string,
) (smartvideo.RenderArtifact, error) {
	if r == nil || r.Tools == nil {
		return smartvideo.RenderArtifact{}, fmt.Errorf("manifest renderer tools unavailable")
	}
	if err := ensureManifestEncodingLocked(manifest); err != nil {
		return smartvideo.RenderArtifact{}, err
	}
	videoPath := filepath.Join(workDir, "montage_output.mp4")
	args, err := BuildManifestRenderArgs(manifest, inputs, voicePath, captionPath, videoPath)
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
	if err := validateRenderedOutput(manifest, metadata); err != nil {
		return smartvideo.RenderArtifact{}, err
	}
	coverPath := filepath.Join(workDir, "montage_cover.jpg")
	coverArgs := []string{
		"-nostdin", "-y", "-v", "error", "-ss", "0.2", "-i", videoPath,
		"-frames:v", "1", "-q:v", "2", coverPath,
	}
	if _, err = r.Tools.Runner.Run(ctx, r.Tools.FFmpegPath, coverArgs, maxToolStdout, maxToolStderr); err != nil {
		return smartvideo.RenderArtifact{}, fmt.Errorf("SMARTVIDEO_RENDER_COVER_FAILED: %w", err)
	}
	return smartvideo.RenderArtifact{
		VideoPath: videoPath, CoverPath: coverPath, DurationMS: metadata.DurationMS,
		Width: metadata.Width, Height: metadata.Height, FrameRate: manifest.Output.FrameRate,
		FileSize: info.Size(), VideoCodec: metadata.VideoCodec, AudioCodec: metadata.AudioCodec,
		PixelFormat: metadata.PixelFormat,
	}, nil
}

func (r *ManifestRenderer) downloadChecked(ctx context.Context, access smartvideo.Access, fileID, wantChecksum, dest string) error {
	reader, _, checksum, err := r.Store.OpenObject(ctx, access, fileID)
	if err != nil {
		return err
	}
	defer reader.Close()
	file, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	hasher := sha256.New()
	writer := io.MultiWriter(file, hasher)
	if _, err := io.Copy(writer, io.LimitReader(reader, 2<<30)); err != nil {
		return err
	}
	got := hex.EncodeToString(hasher.Sum(nil))
	wantChecksum = strings.TrimSpace(wantChecksum)
	checksum = strings.TrimSpace(checksum)
	if wantChecksum != "" && !checksumEqual(wantChecksum, got) && !checksumEqual(wantChecksum, checksum) {
		return fmt.Errorf("%w: object checksum mismatch for %s", smartvideo.ErrInvalidInput, fileID)
	}
	return nil
}

func checksumEqual(left, right string) bool {
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}

func BuildManifestRenderArgs(
	manifest smartvideo.RenderManifestV1,
	inputs []LocalRenderInput,
	voicePath, captionPath, outputPath string,
) ([]string, error) {
	if strings.TrimSpace(outputPath) == "" {
		return nil, smartvideo.ErrInvalidInput
	}
	if len(manifest.Scenes) == 0 {
		return nil, fmt.Errorf("%w: manifest has no scenes", smartvideo.ErrInvalidInput)
	}
	if len(inputs) != len(manifest.Inputs) {
		return nil, fmt.Errorf("%w: local input count mismatch", smartvideo.ErrInvalidInput)
	}
	for _, input := range inputs {
		if strings.TrimSpace(input.Path) == "" {
			return nil, smartvideo.ErrInvalidInput
		}
	}
	if err := ensureManifestEncodingLocked(manifest); err != nil {
		return nil, err
	}

	width := manifest.Output.Width
	height := manifest.Output.Height
	args := []string{"-nostdin", "-y", "-v", "error"}
	for _, input := range inputs {
		if strings.EqualFold(input.AssetType, "image") {
			args = append(args, "-loop", "1", "-t", formatSeconds(maxSceneDuration(manifest)), "-i", input.Path)
		} else {
			args = append(args, "-i", input.Path)
		}
	}
	voiceInput := -1
	if strings.TrimSpace(voicePath) != "" {
		voiceInput = len(inputs)
		args = append(args, "-i", voicePath)
	}

	filter, videoLabel, audioLabel, err := buildManifestFilterComplex(manifest, inputs, voiceInput, captionPath)
	if err != nil {
		return nil, err
	}
	args = append(args,
		"-filter_complex", filter,
		"-map", videoLabel,
		"-map", audioLabel,
		"-r", strconv.Itoa(manifest.Output.FrameRate),
		"-c:v", "libx264", "-preset", "medium", "-crf", "20",
		"-pix_fmt", "yuv420p", "-profile:v", "high", "-level", "4.0",
		"-c:a", "aac", "-b:a", "128k", "-ar", "48000", "-ac", "2",
		"-movflags", "+faststart",
		"-t", formatSeconds(manifestTimelineDurationMs(manifest)),
		outputPath,
	)
	_ = width
	_ = height
	return args, nil
}

func buildManifestFilterComplex(
	manifest smartvideo.RenderManifestV1,
	inputs []LocalRenderInput,
	voiceInput int,
	captionPath string,
) (filter string, videoLabel string, audioLabel string, err error) {
	width := manifest.Output.Width
	height := manifest.Output.Height
	var b strings.Builder
	sceneLabels := make([]string, 0, len(manifest.Scenes))
	audioLabels := make([]string, 0, len(manifest.Scenes))
	var cursor int64

	for sceneIdx, scene := range manifest.Scenes {
		if len(scene.Cuts) == 0 {
			return "", "", "", fmt.Errorf("%w: scene %d has no cuts", smartvideo.ErrInvalidInput, scene.Index)
		}
		cutLabels := make([]string, 0, len(scene.Cuts))
		cutAudio := make([]string, 0, len(scene.Cuts))
		for cutIdx, cut := range scene.Cuts {
			if cut.InputIndex < 0 || cut.InputIndex >= len(inputs) {
				return "", "", "", fmt.Errorf("%w: invalid input index", smartvideo.ErrInvalidInput)
			}
			in := inputs[cut.InputIndex]
			vLabel := fmt.Sprintf("s%dc%dv", sceneIdx, cutIdx)
			aLabel := fmt.Sprintf("s%dc%da", sceneIdx, cutIdx)
			fit := fitFilter(cut.FitMode, width, height)
			motion := motionFilter(cut.Motion, width, height, scene.DurationMs)
			if strings.EqualFold(in.AssetType, "image") {
				b.WriteString(fmt.Sprintf("[%d:v]%s,%s,fps=30,format=yuv420p,setsar=1[%s];",
					cut.InputIndex, fit, motion, vLabel))
				b.WriteString(fmt.Sprintf("anullsrc=channel_layout=stereo:sample_rate=48000,atrim=0:%s,asetpts=PTS-STARTPTS,volume=0[%s];",
					formatSeconds(scene.DurationMs), aLabel))
			} else {
				start := formatSeconds(cut.SourceInMs)
				end := formatSeconds(cut.SourceOutMs)
				if cut.SourceOutMs <= cut.SourceInMs {
					end = formatSeconds(cut.SourceInMs + scene.DurationMs)
				}
				b.WriteString(fmt.Sprintf("[%d:v]trim=start=%s:end=%s,setpts=PTS-STARTPTS,%s,%s,fps=30,format=yuv420p,setsar=1[%s];",
					cut.InputIndex, start, end, fit, motion, vLabel))
				gain := cut.AudioGain
				if gain <= 0 {
					gain = manifest.AudioMix.SourceGain
				}
				b.WriteString(fmt.Sprintf("[%d:a]atrim=start=%s:end=%s,asetpts=PTS-STARTPTS,volume=%s[%s];",
					cut.InputIndex, start, end, formatFloat(gain), aLabel))
			}
			cutLabels = append(cutLabels, "["+vLabel+"]")
			cutAudio = append(cutAudio, "["+aLabel+"]")
		}
		sceneV := fmt.Sprintf("scene%dv", sceneIdx)
		sceneA := fmt.Sprintf("scene%da", sceneIdx)
		if len(cutLabels) == 1 {
			b.WriteString(fmt.Sprintf("%sformat=yuv420p[%s];", cutLabels[0], sceneV))
			b.WriteString(fmt.Sprintf("%svolume=1[%s];", cutAudio[0], sceneA))
		} else {
			b.WriteString(fmt.Sprintf("%sconcat=n=%d:v=1:a=0[%s];", strings.Join(cutLabels, ""), len(cutLabels), sceneV))
			b.WriteString(fmt.Sprintf("%samix=inputs=%d:duration=longest:dropout_transition=0[%s];", strings.Join(cutAudio, ""), len(cutAudio), sceneA))
		}
		sceneLabels = append(sceneLabels, sceneV)
		audioLabels = append(audioLabels, sceneA)
		cursor += scene.DurationMs
		if sceneIdx < len(manifest.Scenes)-1 && scene.Transition.DurationMs > 0 {
			cursor -= scene.Transition.DurationMs
		}
	}

	currentV := sceneLabels[0]
	currentA := audioLabels[0]
	offsetMs := manifest.Scenes[0].DurationMs
	for i := 1; i < len(sceneLabels); i++ {
		prev := manifest.Scenes[i-1]
		trans := strings.ToLower(strings.TrimSpace(prev.Transition.Type))
		dur := prev.Transition.DurationMs
		outV := fmt.Sprintf("xfv%d", i)
		outA := fmt.Sprintf("xfa%d", i)
		if dur > 0 && trans != "" && trans != "cut" {
			xfade := mapTransition(trans)
			b.WriteString(fmt.Sprintf("[%s][%s]xfade=transition=%s:duration=%s:offset=%s[%s];",
				currentV, sceneLabels[i], xfade, formatSeconds(dur), formatSeconds(offsetMs-dur), outV))
			b.WriteString(fmt.Sprintf("[%s][%s]acrossfade=d=%s[%s];",
				currentA, audioLabels[i], formatSeconds(dur), outA))
			offsetMs = offsetMs - dur + manifest.Scenes[i].DurationMs
		} else {
			b.WriteString(fmt.Sprintf("[%s][%s]concat=n=2:v=1:a=0[%s];", currentV, sceneLabels[i], outV))
			b.WriteString(fmt.Sprintf("[%s][%s]concat=n=2:v=0:a=1[%s];", currentA, audioLabels[i], outA))
			offsetMs += manifest.Scenes[i].DurationMs
		}
		currentV, currentA = outV, outA
	}

	mixedA := "aout"
	if voiceInput >= 0 {
		voiceGain := manifest.AudioMix.VoiceGain
		if voiceGain <= 0 {
			voiceGain = 1
		}
		b.WriteString(fmt.Sprintf("[%d:a]volume=%s[voice];", voiceInput, formatFloat(voiceGain)))
		b.WriteString(fmt.Sprintf("[%s][voice]amix=inputs=2:duration=first:dropout_transition=0[%s];", currentA, mixedA))
	} else {
		b.WriteString(fmt.Sprintf("[%s]volume=1[%s];", currentA, mixedA))
	}

	finalV := "vout"
	if strings.TrimSpace(captionPath) != "" {
		escaped := escapeFilterPath(captionPath)
		b.WriteString(fmt.Sprintf("[%s]ass='%s'[%s]", currentV, escaped, finalV))
	} else {
		b.WriteString(fmt.Sprintf("[%s]format=yuv420p[%s]", currentV, finalV))
	}

	return strings.TrimSuffix(b.String(), ";"), "[" + finalV + "]", "[" + mixedA + "]", nil
}

func fitFilter(mode string, width, height int) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "contain":
		return fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2", width, height, width, height)
	default:
		return fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d", width, height, width, height)
	}
}

func motionFilter(motion string, width, height int, durationMs int64) string {
	frames := int(durationMs) * 30 / 1000
	if frames < 1 {
		frames = 1
	}
	switch strings.ToLower(strings.TrimSpace(motion)) {
	case "push":
		return fmt.Sprintf("zoompan=z='min(zoom+0.0015,1.2)':x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':d=%d:s=%dx%d:fps=30", frames, width, height)
	case "pull":
		return fmt.Sprintf("zoompan=z='if(eq(on,1),1.2,max(zoom-0.0015,1))':x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':d=%d:s=%dx%d:fps=30", frames, width, height)
	case "pan_left":
		return fmt.Sprintf("zoompan=z=1.1:x='if(eq(on,1),iw*0.1,x-0.5)':y='ih/2-(ih/zoom/2)':d=%d:s=%dx%d:fps=30", frames, width, height)
	case "pan_right":
		return fmt.Sprintf("zoompan=z=1.1:x='if(eq(on,1),0,x+0.5)':y='ih/2-(ih/zoom/2)':d=%d:s=%dx%d:fps=30", frames, width, height)
	default:
		return "format=yuv420p"
	}
}

func mapTransition(name string) string {
	switch name {
	case "fade":
		return "fade"
	case "dissolve":
		return "dissolve"
	case "wipeleft":
		return "wipeleft"
	case "wiperight":
		return "wiperight"
	case "slideleft":
		return "slideleft"
	case "slideright":
		return "slideright"
	default:
		return "fade"
	}
}

func ensureManifestEncodingLocked(manifest smartvideo.RenderManifestV1) error {
	out := manifest.Output
	if out.VideoCodec != "libx264" || out.AudioCodec != "aac" || out.PixelFormat != "yuv420p" ||
		out.Format != "mp4" || out.FrameRate != 30 {
		return fmt.Errorf("%w: manifest encoding is not server-locked", smartvideo.ErrInvalidInput)
	}
	if out.Width <= 0 || out.Height <= 0 {
		return fmt.Errorf("%w: invalid output size", smartvideo.ErrInvalidInput)
	}
	return nil
}

func validateRenderedOutput(manifest smartvideo.RenderManifestV1, metadata smartvideo.VideoMetadata) error {
	if metadata.Width != manifest.Output.Width || metadata.Height != manifest.Output.Height {
		return fmt.Errorf("SMARTVIDEO_RENDER_VALIDATION_FAILED: resolution")
	}
	if metadata.VideoCodec != "h264" || metadata.AudioCodec != "aac" {
		return fmt.Errorf("SMARTVIDEO_RENDER_VALIDATION_FAILED: codec")
	}
	if metadata.PixelFormat != "" && metadata.PixelFormat != "yuv420p" && metadata.PixelFormat != "yuvj420p" {
		return fmt.Errorf("SMARTVIDEO_RENDER_VALIDATION_FAILED: pix_fmt")
	}
	want := manifestTimelineDurationMs(manifest)
	if metadata.DurationMS < want-500 || metadata.DurationMS > want+500 {
		return fmt.Errorf("SMARTVIDEO_RENDER_VALIDATION_FAILED: duration got=%d want=%d", metadata.DurationMS, want)
	}
	return nil
}

func manifestTimelineDurationMs(manifest smartvideo.RenderManifestV1) int64 {
	var total int64
	for i, scene := range manifest.Scenes {
		total += scene.DurationMs
		if i < len(manifest.Scenes)-1 && scene.Transition.DurationMs > 0 &&
			scene.Transition.Type != "" && scene.Transition.Type != "cut" {
			total -= scene.Transition.DurationMs
		}
	}
	if total <= 0 {
		return 1000
	}
	return total
}

func maxSceneDuration(manifest smartvideo.RenderManifestV1) int64 {
	var max int64
	for _, scene := range manifest.Scenes {
		if scene.DurationMs > max {
			max = scene.DurationMs
		}
	}
	if max <= 0 {
		return 1000
	}
	return max
}

func formatSeconds(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	return strconv.FormatFloat(float64(ms)/1000.0, 'f', 3, 64)
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', 3, 64)
}

func escapeFilterPath(value string) string {
	value = strings.ReplaceAll(value, `\`, `/`)
	value = strings.ReplaceAll(value, ":", `\:`)
	value = strings.ReplaceAll(value, "'", `\'`)
	return value
}

func sanitizeFileName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "object.bin"
	}
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		return "object.bin"
	}
	return out
}

// VersionManifestSource loads confirmed manifests from project versions.
type VersionManifestSource struct {
	Versions smartvideo.VersionRepository
}

func (s VersionManifestSource) LoadRenderManifest(ctx context.Context, access smartvideo.Access, projectID, versionID string) (smartvideo.RenderManifestV1, error) {
	if s.Versions == nil {
		return smartvideo.RenderManifestV1{}, smartvideo.ErrNotFound
	}
	version, err := s.Versions.GetVersion(ctx, access, projectID, versionID)
	if err != nil {
		return smartvideo.RenderManifestV1{}, err
	}
	if version.RenderManifest == nil || version.ManifestHash == "" {
		return smartvideo.RenderManifestV1{}, fmt.Errorf("%w: version has no confirmed manifest", smartvideo.ErrProjectNotConfirmed)
	}
	return *version.RenderManifest, nil
}

// StorageRenderMediaStore downloads private objects through the file center.
type StorageRenderMediaStore struct {
	Open func(ctx context.Context, access smartvideo.Access, fileID string) (io.ReadCloser, int64, string, error)
}

func (s StorageRenderMediaStore) OpenObject(ctx context.Context, access smartvideo.Access, fileID string) (io.ReadCloser, int64, string, error) {
	if s.Open == nil {
		return nil, 0, "", smartvideo.ErrFileNotReady
	}
	return s.Open(ctx, access, fileID)
}

func NewStorageRenderMediaStoreFromFileService(openObject func(ctx context.Context, tenantID, userID, fileID string) (io.ReadCloser, int64, string, error)) StorageRenderMediaStore {
	return StorageRenderMediaStore{
		Open: func(ctx context.Context, access smartvideo.Access, fileID string) (io.ReadCloser, int64, string, error) {
			return openObject(ctx, access.TenantID, access.UserID, fileID)
		},
	}
}