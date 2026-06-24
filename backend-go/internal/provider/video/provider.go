package video

import (
	"context"
	"errors"

	"xianzhi-ai/backend-go/internal/app/generation"
)

var ErrNoProvider = errors.New("no video provider available")

type Task struct {
	ProviderCode   string
	ProviderTaskID string
	Status         string
	VideoURL       string
	ThumbnailURL   string
	Metadata       map[string]any
}

type Provider interface {
	Code() string
	DefaultModel() string
	Supports(generation.CreateRequest) bool
	Create(context.Context, generation.CreateRequest) (Task, error)
	Get(context.Context, string) (Task, error)
}
