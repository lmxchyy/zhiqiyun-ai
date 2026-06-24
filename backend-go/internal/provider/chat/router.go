package chat

import (
	"context"
	"errors"
	"strings"

	"xianzhi-ai/backend-go/internal/app/generation"
)

type Router struct {
	providers []Provider
}

func NewRouter(providers ...Provider) Router {
	items := make([]Provider, 0, len(providers))
	for _, provider := range providers {
		if provider != nil {
			items = append(items, provider)
		}
	}
	return Router{providers: items}
}

func (r Router) DefaultModel() string {
	for _, provider := range r.providers {
		model := strings.TrimSpace(provider.DefaultModel())
		if model != "" {
			return model
		}
	}
	return ""
}

func (r Router) Chat(ctx context.Context, req generation.CreateRequest) (Response, error) {
	var lastErr error
	for _, provider := range r.providers {
		if !provider.Supports(req) {
			continue
		}
		response, err := provider.Chat(ctx, req)
		if err == nil {
			return response, nil
		}
		lastErr = err
		if !isFallbackEligible(err) {
			return Response{}, err
		}
	}
	if lastErr != nil {
		return Response{}, lastErr
	}
	return Response{}, ErrNoProvider
}

func (r Router) StreamChat(ctx context.Context, req generation.CreateRequest) (<-chan Chunk, error) {
	for _, provider := range r.providers {
		if provider.Supports(req) {
			return provider.StreamChat(ctx, req)
		}
	}
	return nil, ErrNoProvider
}

func isFallbackEligible(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, context.DeadlineExceeded)
}
