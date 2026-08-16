package httpserver

import (
	"context"
	"encoding/base64"
	"path/filepath"
	"testing"

	"xianzhi-ai/backend-go/internal/app/generation"
	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type countingPPTAgentImageProvider struct{ calls int }

func (p *countingPPTAgentImageProvider) DefaultModel() string { return "slice-b-image-model" }

func (p *countingPPTAgentImageProvider) Generate(_ context.Context, _ generation.CreateRequest) ([]generation.GeneratedImage, error) {
	p.calls++
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		return nil, err
	}
	return []generation.GeneratedImage{{
		URL:         "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
		ContentType: "image/png", Width: 1024, Height: 1024, Source: "test-provider",
	}}, nil
}

func TestPPTV2AgentImageAssetUsesBilledIdempotentGenerationTaskAndPrivateStorage(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	files, _ := phase1StorageService()
	provider := &countingPPTAgentImageProvider{}
	service := generation.NewService(provider, nil, func(request generation.CreateRequest) (any, error) {
		return store.CreateGenerationTask(request)
	})
	resolver := pptV2AgentImageAssets{generation: service, files: files, store: store}
	scope := pptapp.GenerationJobScope{TenantID: "tenant_slice_b", UserID: "user_000003"}
	intent := pptapp.SlideAssetIntent{StableID: "asset_intent_slide_3", Kind: "image", Prompt: "EV factory", AltText: "EV factory"}

	first, err := resolver.ResolveImage(t.Context(), scope, "job_slice_b", "slide_3", intent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := files.Delete(t.Context(), storagecenter.AccessContext{TenantID: scope.TenantID, UserID: scope.UserID}, first.FileID); err != nil {
		t.Fatal(err)
	}
	restarted := pptV2AgentImageAssets{generation: service, files: files, store: store}
	replayed, err := restarted.ResolveImage(t.Context(), scope, "job_slice_b", "slide_3", intent)
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || first.ID != replayed.ID || first.FileID == replayed.FileID || replayed.TenantID != scope.TenantID || replayed.UserID != scope.UserID || replayed.FileID == "" || replayed.URI == "" {
		t.Fatalf("image generation was not durable and scoped: calls=%d first=%+v replayed=%+v", provider.calls, first, replayed)
	}
	tasks, err := store.ListGenerationTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].ClientRequestID != "ppt-v2:job_slice_b:asset_intent_slide_3" || tasks[0].BillingStatus != billingStatusCaptured || tasks[0].CapturedPoints <= 0 {
		t.Fatalf("image did not use the existing billed idempotent task path: %+v", tasks)
	}
	stored, _, err := files.ListFiles(t.Context(), storagecenter.FileFilter{TenantID: scope.TenantID, UserID: scope.UserID, BusinessType: "ppt_v2_image_asset", Status: storagecenter.StatusActive, Limit: 10})
	if err != nil || len(stored) != 1 || stored[0].Visibility != "PRIVATE" {
		t.Fatalf("image was not stored once in private storage: files=%+v err=%v", stored, err)
	}
}
