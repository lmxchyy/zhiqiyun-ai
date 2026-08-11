package smartvideo

import (
	"context"
	"fmt"
	"strings"
)

var ErrSpeechNotReady = fmt.Errorf("SMART_VIDEO_SPEECH_NOT_READY")

// SpeechAudio is raw TTS output before private-file persistence.
type SpeechAudio struct {
	Audio      []byte
	Format     string
	DurationMs int64
	SampleRate int
	Channels   int
}

// SpeechAudioSynthesizer returns audio bytes for one narration segment.
type SpeechAudioSynthesizer interface {
	SynthesizeAudio(context.Context, SpeechRequest) (SpeechAudio, error)
}

// SpeechArtifactStore uploads generated voice/caption binaries as private files.
type SpeechArtifactStore interface {
	PutSpeechArtifact(ctx context.Context, access Access, projectID, name, contentType string, data []byte) (fileID string, err error)
}

type VoiceCaptionArtifacts struct {
	VoiceFileID   string
	CaptionFileID string
	Skipped       bool
}

// VoiceCaptionBuilder builds reusable voice/caption artifacts from an edit plan.
type VoiceCaptionBuilder interface {
	Build(ctx context.Context, access Access, projectID string, plan EditPlanV1) (VoiceCaptionArtifacts, error)
}

type SpeechPrepService struct {
	builder VoiceCaptionBuilder
	plans   VersionRepository
}

func NewSpeechPrepService(builder VoiceCaptionBuilder, plans VersionRepository) *SpeechPrepService {
	return &SpeechPrepService{builder: builder, plans: plans}
}

func (s *SpeechPrepService) Prepare(ctx context.Context, access Access, task RenderTask) (VoiceCaptionArtifacts, error) {
	if strings.TrimSpace(task.VoiceFileID) != "" && strings.TrimSpace(task.CaptionFileID) != "" {
		return VoiceCaptionArtifacts{
			VoiceFileID: task.VoiceFileID, CaptionFileID: task.CaptionFileID, Skipped: true,
		}, nil
	}
	if s == nil || s.plans == nil {
		return VoiceCaptionArtifacts{}, ErrSpeechNotReady
	}
	version, err := s.plans.GetVersion(ctx, access, task.ProjectID, task.VersionID)
	if err != nil {
		return VoiceCaptionArtifacts{}, err
	}
	plan := version.PlanSnapshot
	if !plan.Voice.Enabled {
		return VoiceCaptionArtifacts{Skipped: true}, nil
	}
	if s.builder == nil {
		return VoiceCaptionArtifacts{}, ErrSpeechNotReady
	}
	return s.builder.Build(ctx, access, task.ProjectID, plan)
}
