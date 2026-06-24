package image

import (
	"context"
	"errors"

	"xianzhi-ai/backend-go/internal/app/generation"
)

var ErrNoProvider = errors.New("no image provider available")

type Provider interface {
	Code() string
	DefaultModel() string
	Supports(generation.CreateRequest) bool
	Generate(context.Context, generation.CreateRequest) ([]generation.GeneratedImage, error)
}
