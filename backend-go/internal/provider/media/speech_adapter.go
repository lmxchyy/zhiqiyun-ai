package media

import (
	"bytes"
	"context"
	"strings"

	"xianzhi-ai/backend-go/internal/app/smartvideo"
	"xianzhi-ai/backend-go/internal/provider/speech"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

// SpeechClientAdapter adapts provider/speech to smartvideo.SpeechAudioSynthesizer.
type SpeechClientAdapter struct {
	Client *speech.Client
}

func (a SpeechClientAdapter) SynthesizeAudio(ctx context.Context, req smartvideo.SpeechRequest) (smartvideo.SpeechAudio, error) {
	if a.Client == nil {
		return smartvideo.SpeechAudio{}, smartvideo.ErrSpeechNotReady
	}
	format := "wav"
	modelKey := smartvideo.NormalizeSpeechModelKey(req.ModelKey)
	voiceKey := smartvideo.NormalizeSpeechVoiceKey(req.VoiceKey)
	result, err := a.Client.Synthesize(ctx, speech.Request{
		Text: req.Text, ModelKey: modelKey, VoiceKey: voiceKey, Speed: req.Speed, Format: format,
	})
	if err != nil {
		return smartvideo.SpeechAudio{}, err
	}
	return smartvideo.SpeechAudio{
		Audio: result.Audio, Format: firstNonEmpty(result.Format, format),
		DurationMs: result.DurationMs, SampleRate: result.SampleRate, Channels: result.Channels,
	}, nil
}

// SpeechArtifactUploader stores voice/caption binaries via the file center.
type SpeechArtifactUploader struct {
	Storage *storagecenter.Service
}

func (u SpeechArtifactUploader) PutSpeechArtifact(ctx context.Context, access smartvideo.Access, projectID, name, contentType string, data []byte) (string, error) {
	if u.Storage == nil {
		return "", smartvideo.ErrFileNotReady
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "speech.bin"
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	file, err := u.Storage.StoreObject(ctx, storagecenter.UploadInitInput{
		TenantID: access.TenantID, UserID: access.UserID, FileName: name,
		FileSize: int64(len(data)), MIMEType: contentType,
		BusinessType: "smart_video_speech", BusinessID: projectID, Visibility: "PRIVATE",
	}, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	return file.FileID, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
