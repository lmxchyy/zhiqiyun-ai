package media

import (
	"context"
	"fmt"
	"strings"

	"xianzhi-ai/backend-go/internal/app/smartvideo"
)

// VoiceCaptionBuilder synthesizes scene narrations, merges WAV audio, and uploads
// reusable voice/caption artifacts for SmartVideo render retries.
type VoiceCaptionBuilder struct {
	Synth smartvideo.SpeechAudioSynthesizer
	Store smartvideo.SpeechArtifactStore
}

func (b *VoiceCaptionBuilder) Build(ctx context.Context, access smartvideo.Access, projectID string, plan smartvideo.EditPlanV1) (smartvideo.VoiceCaptionArtifacts, error) {
	if b == nil || b.Synth == nil || b.Store == nil {
		return smartvideo.VoiceCaptionArtifacts{}, smartvideo.ErrSpeechNotReady
	}
	scenes := make([]NarrationSceneInput, 0, len(plan.Scenes))
	for _, scene := range plan.Scenes {
		scenes = append(scenes, NarrationSceneInput{
			Index: scene.Index, Narration: scene.Narration, DurationMs: scene.DurationMs,
		})
	}
	segments := SegmentNarration(scenes, plan.Voice.Speed)
	if len(segments) == 0 {
		return smartvideo.VoiceCaptionArtifacts{Skipped: true}, nil
	}

	clips := make([][]byte, 0, len(segments))
	timed := make([]NarrationSegment, 0, len(segments))
	var cursor int64
	for _, seg := range segments {
		audio, err := b.Synth.SynthesizeAudio(ctx, smartvideo.SpeechRequest{
			Text: seg.Text, ModelKey: plan.Voice.ModelKey, VoiceKey: plan.Voice.VoiceKey, Speed: plan.Voice.Speed,
		})
		if err != nil {
			return smartvideo.VoiceCaptionArtifacts{}, err
		}
		format := strings.ToLower(strings.TrimSpace(audio.Format))
		if format == "" {
			format = "wav"
		}
		if format != "wav" {
			return smartvideo.VoiceCaptionArtifacts{}, fmt.Errorf("%w: montage speech requires wav intermediate audio", smartvideo.ErrInvalidInput)
		}
		if len(audio.Audio) == 0 {
			return smartvideo.VoiceCaptionArtifacts{}, fmt.Errorf("%w: empty speech audio", smartvideo.ErrInvalidInput)
		}
		dur := audio.DurationMs
		if dur <= 0 {
			dur = EstimateNarrationDurationMs(seg.Text, plan.Voice.Speed)
		}
		clips = append(clips, audio.Audio)
		timed = append(timed, NarrationSegment{
			SceneIndex: seg.SceneIndex, Text: seg.Text, StartMs: cursor, DurationMs: dur,
		})
		cursor += dur
	}

	merged, err := ConcatenatePCM16MonoWAVs(clips)
	if err != nil {
		return smartvideo.VoiceCaptionArtifacts{}, err
	}
	cues, err := BuildSubtitleCues(timed)
	if err != nil {
		return smartvideo.VoiceCaptionArtifacts{}, err
	}
	var caption []byte
	captionType := "application/x-subrip"
	captionName := "captions.srt"
	if plan.Subtitles.Enabled {
		ass, err := RenderASS(cues, SubtitleStyle{
			Preset: plan.Subtitles.Preset, Position: plan.Subtitles.Position,
		})
		if err != nil {
			return smartvideo.VoiceCaptionArtifacts{}, err
		}
		caption = []byte(ass)
		captionType = "application/x-ass"
		captionName = "captions.ass"
	} else {
		caption = []byte(RenderSRT(cues))
	}

	voiceID, err := b.Store.PutSpeechArtifact(ctx, access, projectID, "voice.wav", "audio/wav", merged)
	if err != nil {
		return smartvideo.VoiceCaptionArtifacts{}, err
	}
	captionID, err := b.Store.PutSpeechArtifact(ctx, access, projectID, captionName, captionType, caption)
	if err != nil {
		return smartvideo.VoiceCaptionArtifacts{}, err
	}
	return smartvideo.VoiceCaptionArtifacts{
		VoiceFileID: voiceID, CaptionFileID: captionID,
	}, nil
}
