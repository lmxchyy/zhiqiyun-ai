package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrRateLimited         = errors.New("provider_rate_limited")
	ErrProviderUnavailable = errors.New("provider_unavailable")
	ErrEmptyAudio          = errors.New("provider_empty_audio")
	ErrInvalidVoice        = errors.New("invalid_voice")
	ErrInvalidModel        = errors.New("invalid_model")
)

type Client struct {
	baseURL       string
	apiKey        string
	defaultModel  string
	allowedModels map[string]bool
	allowedVoices map[string]bool
	client        *http.Client
}

type Options struct {
	BaseURL       string
	APIKey        string
	DefaultModel  string
	AllowedModels []string
	AllowedVoices []string
	Timeout       time.Duration
}

type Request struct {
	Text     string
	ModelKey string
	VoiceKey string
	Speed    float64
	Format   string
}

type Result struct {
	Audio      []byte
	Format     string
	ModelKey   string
	VoiceKey   string
	DurationMs int64
	SampleRate int
	Channels   int
}

func NewClient(opts Options) *Client {
	if opts.Timeout <= 0 {
		opts.Timeout = 60 * time.Second
	}
	if strings.TrimSpace(opts.DefaultModel) == "" {
		opts.DefaultModel = "smart-video-speech"
	}
	if len(opts.AllowedModels) == 0 {
		opts.AllowedModels = []string{opts.DefaultModel, "tts-1", "tts-1-hd"}
	}
	if len(opts.AllowedVoices) == 0 {
		opts.AllowedVoices = []string{"alloy", "echo", "fable", "onyx", "nova", "shimmer"}
	}
	return &Client{
		baseURL:       strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/"),
		apiKey:        strings.TrimSpace(opts.APIKey),
		defaultModel:  strings.TrimSpace(opts.DefaultModel),
		allowedModels: toSet(opts.AllowedModels),
		allowedVoices: toSet(opts.AllowedVoices),
		client:        &http.Client{Timeout: opts.Timeout},
	}
}

func (c *Client) Synthesize(ctx context.Context, req Request) (Result, error) {
	if c == nil || c.baseURL == "" || c.apiKey == "" {
		return Result{}, ErrProviderUnavailable
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return Result{}, ErrEmptyAudio
	}
	model := strings.TrimSpace(req.ModelKey)
	if model == "" {
		model = c.defaultModel
	}
	if !c.allowedModels[model] {
		return Result{}, fmt.Errorf("%w: %s", ErrInvalidModel, model)
	}
	voice := strings.TrimSpace(req.VoiceKey)
	if voice == "" {
		voice = "alloy"
	}
	if !c.allowedVoices[voice] {
		return Result{}, fmt.Errorf("%w: %s", ErrInvalidVoice, voice)
	}
	speed := req.Speed
	if speed == 0 {
		speed = 1
	}
	if speed < 0.8 || speed > 1.2 {
		return Result{}, fmt.Errorf("%w: speed out of range", ErrInvalidVoice)
	}
	format := strings.ToLower(strings.TrimSpace(req.Format))
	if format == "" {
		format = "mp3"
	}
	body, err := json.Marshal(map[string]any{
		"model":           model,
		"input":           text,
		"voice":           voice,
		"speed":           speed,
		"response_format": format,
	})
	if err != nil {
		return Result{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, speechEndpoint(c.baseURL), bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json; charset=utf-8")
	httpReq.Header.Set("Accept", "audio/*,application/octet-stream")
	res, err := c.client.Do(httpReq)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if res.StatusCode == http.StatusTooManyRequests {
		return Result{}, fmt.Errorf("%w: HTTP 429", ErrRateLimited)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return Result{}, fmt.Errorf("%w: HTTP %d", ErrProviderUnavailable, res.StatusCode)
	}
	if len(raw) == 0 {
		return Result{}, ErrEmptyAudio
	}
	return Result{
		Audio:      raw,
		Format:     format,
		ModelKey:   model,
		VoiceKey:   voice,
		DurationMs: estimateDurationMs(text, speed),
		SampleRate: 24000,
		Channels:   1,
	}, nil
}

func speechEndpoint(baseURL string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return strings.TrimRight(baseURL, "/") + "/v1/audio/speech"
	}
	path := strings.TrimSuffix(parsed.Path, "/")
	if strings.HasSuffix(path, "/v1") {
		parsed.Path = path + "/audio/speech"
	} else if strings.HasSuffix(path, "/audio/speech") {
		// already complete
	} else {
		parsed.Path = path + "/v1/audio/speech"
	}
	return parsed.String()
}

func estimateDurationMs(text string, speed float64) int64 {
	chars := len([]rune(strings.TrimSpace(text)))
	if chars == 0 {
		return 0
	}
	if speed <= 0 {
		speed = 1
	}
	// Rough Mandarin pacing: ~4 chars/sec.
	seconds := float64(chars) / (4.0 * speed)
	if seconds < 0.4 {
		seconds = 0.4
	}
	return int64(seconds * 1000)
}

func toSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = true
		}
	}
	return out
}
