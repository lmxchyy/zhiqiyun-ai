package media_test

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"

	"xianzhi-ai/backend-go/internal/app/smartvideo"
	"xianzhi-ai/backend-go/internal/provider/media"
)

type fakeSpeechSynth struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeSpeechSynth) SynthesizeAudio(_ context.Context, req smartvideo.SpeechRequest) (smartvideo.SpeechAudio, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	dur := media.EstimateNarrationDurationMs(req.Text, req.Speed)
	if dur <= 0 {
		dur = 500
	}
	return smartvideo.SpeechAudio{
		Audio: media.SilencePCM16MonoWAV(dur, 24000), Format: "wav",
		DurationMs: dur, SampleRate: 24000, Channels: 1,
	}, nil
}

type fakeSpeechStore struct {
	mu    sync.Mutex
	files map[string][]byte
	seq   int
}

func (s *fakeSpeechStore) PutSpeechArtifact(_ context.Context, _ smartvideo.Access, _, name, _ string, data []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.files == nil {
		s.files = map[string][]byte{}
	}
	s.seq++
	id := "file_speech_" + strings.ReplaceAll(name, ".", "_") + "_" + strconv.Itoa(s.seq)
	s.files[id] = append([]byte{}, data...)
	return id, nil
}

func TestVoiceCaptionBuilderSynthesizesMergedWavAndASS(t *testing.T) {
	synth := &fakeSpeechSynth{}
	store := &fakeSpeechStore{}
	builder := &media.VoiceCaptionBuilder{Synth: synth, Store: store}
	plan := smartvideo.EditPlanV1{
		SchemaVersion: 1, Title: "t", Language: "zh-CN",
		Target: smartvideo.TargetSpec{AspectRatio: "16:9", Resolution: "1080p", DurationMs: 30000},
		Voice:  smartvideo.VoiceConfig{Enabled: true, ModelKey: "tts-1", VoiceKey: "alloy", Speed: 1},
		Subtitles: smartvideo.SubtitleConfig{Enabled: true, Preset: "clean", Position: "bottom"},
		Scenes: []smartvideo.SceneV1{
			{Index: 0, Title: "a", DurationMs: 15000, Narration: "欢迎观看。"},
			{Index: 1, Title: "b", DurationMs: 15000, Narration: "自动混剪。"},
		},
	}
	artifacts, err := builder.Build(context.Background(), smartvideo.Access{TenantID: "t", UserID: "u"}, "vp_1", plan)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if artifacts.VoiceFileID == "" || artifacts.CaptionFileID == "" || synth.calls == 0 {
		t.Fatalf("unexpected artifacts=%+v calls=%d", artifacts, synth.calls)
	}
	if !strings.Contains(string(store.files[artifacts.CaptionFileID]), "Dialogue:") {
		t.Fatalf("expected ASS output")
	}
	if !strings.HasPrefix(string(store.files[artifacts.VoiceFileID]), "RIFF") {
		t.Fatal("merged voice should be wav")
	}
}
