package video

import (
	"context"
	"errors"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
)

var ErrUnsupportedModel = errors.New("unsupported video model")

type MockProvider struct{}

func NewMockProvider() MockProvider {
	return MockProvider{}
}

func (MockProvider) DefaultModel() string {
	return "mock-video"
}

func (MockProvider) Create(ctx context.Context, req generation.CreateRequest) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	model := strings.TrimSpace(req.Model)
	if model != "" && !strings.EqualFold(model, "mock-video") {
		return nil, ErrUnsupportedModel
	}
	return map[string]any{
		"provider":       "local-mock-video",
		"providerTaskId": "mock-video-" + time.Now().UTC().Format("20060102150405"),
		"status":         "SUCCEEDED",
		"videoUrl":       "/admin/static/mock-video.mp4",
		"thumbnailUrl":   "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 320 180'%3E%3Crect width='320' height='180' fill='%230f172a'/%3E%3Ccircle cx='160' cy='90' r='38' fill='%2314b8a6'/%3E%3Cpolygon points='151,70 151,110 187,90' fill='white'/%3E%3Ctext x='160' y='150' text-anchor='middle' font-size='18' font-family='Arial' fill='white'%3EMock Video%3C/text%3E%3C/svg%3E",
		"metadata": map[string]any{
			"duration":        req.Params["duration"],
			"aspect_ratio":    req.Params["aspect_ratio"],
			"resolution":      req.Params["resolution"],
			"fps":             req.Params["fps"],
			"generate_audio":  req.Params["generate_audio"],
			"motion_strength": req.Params["motion_strength"],
			"camera_movement": req.Params["camera_movement"],
		},
	}, nil
}
