package httpserver

import (
	"context"
	"math"
	"testing"

	"xianzhi-ai/backend-go/internal/app/generation"
)

type providerChannelVideoProvider struct{}

func (providerChannelVideoProvider) DefaultModel() string {
	return "seedance-fast-2.0"
}

func (providerChannelVideoProvider) Create(context.Context, generation.CreateRequest) (any, error) {
	return map[string]any{
		"provider":       "channel_api_grok",
		"providerTaskId": "provider-task-grok",
		"status":         "SUCCEEDED",
		"videoUrl":       "https://example.test/generated.mp4",
	}, nil
}

func TestRunVideoGenerationTaskRecordsActualProviderChannel(t *testing.T) {
	store := newBillingAcceptanceStore(t)
	if _, err := store.UpdateProviderCost("pcost_newapi_grok_imagine_15_video", providerCostMutation{
		Channel: "channel_api_grok",
	}); err != nil {
		t.Fatalf("bind provider cost to actual channel: %v", err)
	}
	req := videoAcceptanceRequest("provider-channel-completion")
	req.Model = "grok-imagine-1.5-video"
	req.Params["duration"] = 6
	req.Params["resolution"] = "480p"
	req.Params["provider_channel"] = "channel_runtime_env"
	pending, err := store.CreatePendingGenerationTask(req)
	if err != nil {
		t.Fatalf("create pending task: %v", err)
	}

	service := generation.NewServiceWithOptions(generation.ServiceOptions{
		VideoProvider: providerChannelVideoProvider{},
	})
	api{store: store}.runVideoGenerationTask(pending.ID, service, req)

	tasks, err := store.ListGenerationTasks()
	if err != nil {
		t.Fatalf("list generation tasks: %v", err)
	}
	completed := generationBillingTaskByID(t, tasks, pending.ID)
	if completed.Status != "SUCCEEDED" {
		t.Fatalf("completed status = %q, want SUCCEEDED", completed.Status)
	}
	if completed.ProviderChannel != "channel_api_grok" {
		t.Fatalf("provider channel = %q, want channel_api_grok", completed.ProviderChannel)
	}
	if completed.UpstreamProvider != "channel_api_grok" {
		t.Fatalf("upstream provider = %q, want channel_api_grok", completed.UpstreamProvider)
	}
	if completed.SupplierCost == nil || math.Abs(*completed.SupplierCost-0.78) > 0.000001 {
		t.Fatalf("supplier cost = %v, want 0.78", completed.SupplierCost)
	}
}
