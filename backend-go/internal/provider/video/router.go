package video

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

func (r Router) Create(ctx context.Context, req generation.CreateRequest) (Task, error) {
	var lastErr error
	for _, provider := range r.providers {
		if !provider.Supports(req) {
			continue
		}
		task, err := provider.Create(ctx, req)
		if err == nil {
			return task, nil
		}
		lastErr = err
		if !isFallbackEligible(err) {
			return Task{}, err
		}
	}
	if lastErr != nil {
		return Task{}, lastErr
	}
	return Task{}, ErrNoProvider
}

func (r Router) Get(ctx context.Context, providerTaskID string) (Task, error) {
	for _, provider := range r.providers {
		task, err := provider.Get(ctx, providerTaskID)
		if err == nil {
			return task, nil
		}
	}
	return Task{}, ErrNoProvider
}

func isFallbackEligible(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, context.DeadlineExceeded)
}
