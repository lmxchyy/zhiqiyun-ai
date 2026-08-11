package speech_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"xianzhi-ai/backend-go/internal/provider/speech"
)

func TestSpeechClientSynthesizeSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/speech" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("auth = %q", r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		if len(body) == 0 {
			t.Fatal("empty body")
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("ID3fake-audio"))
	}))
	defer server.Close()

	client := speech.NewClient(speech.Options{
		BaseURL: server.URL, APIKey: "secret", DefaultModel: "tts-1",
		AllowedModels: []string{"tts-1"}, AllowedVoices: []string{"alloy"},
	})
	result, err := client.Synthesize(context.Background(), speech.Request{
		Text: "你好世界", ModelKey: "tts-1", VoiceKey: "alloy", Speed: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Audio) != "ID3fake-audio" || result.Format != "mp3" {
		t.Fatalf("result = %+v", result)
	}
	if result.DurationMs <= 0 {
		t.Fatalf("duration = %d", result.DurationMs)
	}
}

func TestSpeechClientRejectsDisallowedVoiceAndEmptyAudio(t *testing.T) {
	client := speech.NewClient(speech.Options{
		BaseURL: "http://example.invalid", APIKey: "secret",
		AllowedModels: []string{"tts-1"}, AllowedVoices: []string{"alloy"},
	})
	_, err := client.Synthesize(context.Background(), speech.Request{Text: "hi", ModelKey: "tts-1", VoiceKey: "clone-me"})
	if !errors.Is(err, speech.ErrInvalidVoice) {
		t.Fatalf("voice error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client = speech.NewClient(speech.Options{
		BaseURL: server.URL, APIKey: "secret", AllowedModels: []string{"tts-1"}, AllowedVoices: []string{"alloy"},
	})
	_, err = client.Synthesize(context.Background(), speech.Request{Text: "hi", ModelKey: "tts-1", VoiceKey: "alloy"})
	if !errors.Is(err, speech.ErrEmptyAudio) {
		t.Fatalf("empty audio error = %v", err)
	}
}

func TestSpeechClientMapsHTTPErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	client := speech.NewClient(speech.Options{
		BaseURL: server.URL, APIKey: "secret", AllowedModels: []string{"tts-1"}, AllowedVoices: []string{"alloy"},
	})
	_, err := client.Synthesize(context.Background(), speech.Request{Text: "hi", ModelKey: "tts-1", VoiceKey: "alloy"})
	if !errors.Is(err, speech.ErrRateLimited) {
		t.Fatalf("rate limit error = %v", err)
	}
}
