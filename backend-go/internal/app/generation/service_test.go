package generation

import (
	"context"
	"errors"
	"testing"
)

func TestServiceRoutesImageTasksToTaskCreator(t *testing.T) {
	called := false
	service := NewService(nil, nil, func(req CreateRequest) (any, error) {
		called = true
		if req.Type != "TEXT_TO_IMAGE" {
			t.Fatalf("type = %q", req.Type)
		}
		if req.Model != "mock-standard" {
			t.Fatalf("model = %q", req.Model)
		}
		return "ok", nil
	})

	result, err := service.Create(context.Background(), CreateRequest{Prompt: "cat"})
	if err != nil {
		t.Fatal(err)
	}
	if result != "ok" || !called {
		t.Fatalf("task creator was not called: result=%v called=%v", result, called)
	}
}

func TestServiceRejectsVideoWithoutProvider(t *testing.T) {
	service := NewService(nil, nil, func(req CreateRequest) (any, error) {
		t.Fatal("task creator should not be called for unsupported video task")
		return nil, nil
	})

	_, err := service.Create(context.Background(), CreateRequest{Type: "TEXT_TO_VIDEO", Prompt: "cat video"})
	if !errors.Is(err, ErrUnsupportedCapability) {
		t.Fatalf("err = %v, want ErrUnsupportedCapability", err)
	}
}

func TestServiceRejectsChatWithoutProvider(t *testing.T) {
	service := NewService(nil, nil, func(req CreateRequest) (any, error) {
		t.Fatal("task creator should not be called for unsupported chat task")
		return nil, nil
	})

	_, err := service.Create(context.Background(), CreateRequest{Type: "CHAT_COMPLETION", Prompt: "hello"})
	if !errors.Is(err, ErrUnsupportedCapability) {
		t.Fatalf("err = %v, want ErrUnsupportedCapability", err)
	}
}
