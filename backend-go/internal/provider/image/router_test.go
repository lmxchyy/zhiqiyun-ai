package image

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"xianzhi-ai/backend-go/internal/app/generation"
	pe "xianzhi-ai/backend-go/internal/providerexecution"
)

type routerTestProvider struct {
	calls atomic.Int32
	err   error
}

func (p *routerTestProvider) DefaultModel() string                   { return "router-test" }
func (p *routerTestProvider) Code() string                           { return "router-test" }
func (p *routerTestProvider) Supports(generation.CreateRequest) bool { return true }
func (p *routerTestProvider) Generate(context.Context, generation.CreateRequest) ([]generation.GeneratedImage, error) {
	p.calls.Add(1)
	if p.err != nil {
		return nil, p.err
	}
	return []generation.GeneratedImage{{URL: "https://example.test/image.png"}}, nil
}

func TestRouterDoesNotFallbackInsideDurableExecution(t *testing.T) {
	first := &routerTestProvider{err: pe.ClassifiedError{Class: pe.DefinitiveNotSubmitted, Err: errors.New("pre-submit")}}
	second := &routerTestProvider{}
	router := NewRouter(first, second)
	_, err := router.Generate(context.Background(), generation.CreateRequest{Params: map[string]any{
		"_provider_execution_task_id": "task-router-test",
	}})
	if err == nil || first.calls.Load() != 1 || second.calls.Load() != 0 {
		t.Fatalf("durable router fallback calls=%d/%d err=%v", first.calls.Load(), second.calls.Load(), err)
	}
}

func TestRouterCanFallbackWithoutDurableExecutionGuard(t *testing.T) {
	first := &routerTestProvider{err: pe.ClassifiedError{Class: pe.DefinitiveNotSubmitted, Err: errors.New("pre-submit")}}
	second := &routerTestProvider{}
	router := NewRouter(first, second)
	if _, err := router.Generate(context.Background(), generation.CreateRequest{}); err != nil {
		t.Fatal(err)
	}
	if first.calls.Load() != 1 || second.calls.Load() != 1 {
		t.Fatalf("ordinary router fallback calls=%d/%d", first.calls.Load(), second.calls.Load())
	}
}
