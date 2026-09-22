package httpserver

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/config"
	videoprovider "xianzhi-ai/backend-go/internal/provider/video"
)

func canonicalDownstreamPreparedRequest(t *testing.T) (api, adminPlatformData, adminUser, generation.CreateRequest) {
	t.Helper()
	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	data, err := store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	user := videoEstimateTestUser(t, data)
	server := newAPI(store, config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "api.json"), StaticDir: t.TempDir()}, newLocalAuthSessions(), nil)
	prepared, err := server.prepareGenerationRequest(data, user, generation.CreateRequest{
		Type: "TEXT_TO_VIDEO", Prompt: "生成30秒 9:16 720p 视频", Model: "mock-video",
		Params: map[string]any{"duration": 5, "aspect_ratio": "16:9", "resolution": "480p"},
	})
	if err != nil {
		t.Fatalf("prepare canonical request: %v", err)
	}
	return server, data, user, prepared
}

func TestCanonicalVideoPreparedRequestPersistsTaskSnapshotAndPreservesExecution(t *testing.T) {
	_, _, _, prepared := canonicalDownstreamPreparedRequest(t)
	canonical, ok := canonicalVideoRequestFromParams(prepared.Params)
	if !ok {
		t.Fatalf("missing canonical snapshot: %#v", prepared.Params)
	}
	if canonical.Execution.Model != "mock-video" || canonical.Execution.DurationSeconds != 5 || canonical.Execution.AspectRatio != "16:9" || canonical.Execution.Resolution != "480p" || canonical.Execution.InputMode != "TEXT_TO_VIDEO" {
		t.Fatalf("canonical execution = %#v", canonical.Execution)
	}
	if !containsCanonicalWarningCode(canonical.ConsistencyResult.WarningCodes, "VIDEO_PROMPT_DURATION_MISMATCH") || !containsCanonicalWarningCode(canonical.ConsistencyResult.WarningCodes, "VIDEO_PROMPT_ASPECT_RATIO_MISMATCH") {
		t.Fatalf("warning codes = %#v", canonical.ConsistencyResult.WarningCodes)
	}
}

func TestCanonicalVideoTaskPersistenceUsesCanonicalSnapshot(t *testing.T) {
	store := newJSONStore(filepath.Join(t.TempDir(), "store.json"))
	data, err := store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	user := videoEstimateTestUser(t, data)
	if _, err := store.PersonalPointService().Grant(context.Background(), PersonalPointGrantCommand{AccountID: "canonical-task-account", UserID: user.ID, Source: PointSourceRecharge, Points: 1000, IdempotencyKey: "canonical-task-grant", GrantedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	server := newAPI(store, config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "api.json"), StaticDir: t.TempDir()}, newLocalAuthSessions(), nil)
	prepared, err := server.prepareGenerationRequest(data, user, generation.CreateRequest{
		UserID: user.ID, Type: "TEXT_TO_VIDEO", Prompt: "生成30秒视频", Model: "mock-video",
		Params: map[string]any{"duration": 5, "aspect_ratio": "16:9", "resolution": "480p"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ensureVideoPromptExecutionSnapshot(&prepared) {
		t.Fatal("missing canonical prompt execution snapshot")
	}
	task, err := store.CreatePendingGenerationTask(prepared)
	if err != nil {
		t.Fatal(err)
	}
	canonical, ok := canonicalVideoRequestFromParams(task.Params)
	if !ok || canonical.Execution.DurationSeconds != 5 || canonical.Execution.AspectRatio != "16:9" || canonical.Execution.Resolution != "480p" {
		t.Fatalf("persisted canonical task = %#v", task.Params)
	}
	if snapshot, ok := videoPromptExecutionFromParams(task.Params, task.Prompt); !ok || snapshot.GuardVersion != videoPromptGuardVersion {
		t.Fatalf("persisted prompt execution snapshot = %#v", task.Params[videoPromptExecutionParam])
	}
}

func TestCanonicalVideoBillingFingerprintAndProviderProjectionUseSameExecution(t *testing.T) {
	_, data, _, prepared := canonicalDownstreamPreparedRequest(t)
	canonical, ok := canonicalVideoRequestFromParams(prepared.Params)
	if !ok {
		t.Fatal("missing canonical snapshot")
	}
	// Deliberately corrupt the legacy map after the immutable snapshot. Every
	// migrated downstream projection must still read canonical execution.
	prepared.Params["duration"] = 30
	prepared.Params["aspect_ratio"] = "9:16"
	prepared.Params["resolution"] = "720p"

	quote, err := generationQuoteForRequest(prepared, data)
	if err != nil {
		t.Fatalf("quote canonical request: %v", err)
	}
	canonicalRequest, canonicalPath := canonicalVideoDownstreamRequest(prepared)
	if !canonicalPath {
		t.Fatal("expected canonical downstream path")
	}
	if canonicalRequest.Model != canonical.Execution.Model || canonicalRequest.Type != canonical.Execution.InputMode || parameterInt(canonicalRequest.Params, "duration") != canonical.Execution.DurationSeconds || parameterString(canonicalRequest.Params, "aspect_ratio") != canonical.Execution.AspectRatio || parameterString(canonicalRequest.Params, "resolution") != canonical.Execution.Resolution {
		t.Fatalf("downstream projection = %#v, canonical = %#v", canonicalRequest, canonical.Execution)
	}
	if quote.NormalizedParameters["duration"] != canonical.Execution.DurationSeconds {
		t.Fatalf("billing duration = %#v, canonical = %d", quote.NormalizedParameters, canonical.Execution.DurationSeconds)
	}
	providerOutput, err := videoprovider.NewMockProvider().Create(context.Background(), canonicalRequest)
	if err != nil {
		t.Fatalf("provider create: %v", err)
	}
	providerMetadata := providerOutput.(map[string]any)["metadata"].(map[string]any)
	if providerMetadata["duration"] != canonical.Execution.DurationSeconds || providerMetadata["aspect_ratio"] != canonical.Execution.AspectRatio || providerMetadata["resolution"] != canonical.Execution.Resolution {
		t.Fatalf("provider metadata = %#v, canonical = %#v", providerMetadata, canonical.Execution)
	}
	fingerprintA, err := videoRequestFingerprint("task-canonical", "mock", "video", canonical.Execution.Model, prepared.Params)
	if err != nil {
		t.Fatal(err)
	}
	prepared.Params["canonical_video_request"].(map[string]any)["consistency_result"] = map[string]any{"warning_codes": []string{"UI_ONLY_DIAGNOSTIC"}}
	fingerprintB, err := videoRequestFingerprint("task-canonical", "mock", "video", canonical.Execution.Model, prepared.Params)
	if err != nil {
		t.Fatal(err)
	}
	if fingerprintA != fingerprintB {
		t.Fatalf("diagnostics changed canonical fingerprint: %q / %q", fingerprintA, fingerprintB)
	}
}

func TestCanonicalVideoLegacyFallbackRetainsExistingParams(t *testing.T) {
	legacy := generation.CreateRequest{Type: "TEXT_TO_VIDEO", Prompt: "legacy", Model: "mock-video", Params: map[string]any{"duration": 5, "aspect_ratio": "16:9", "resolution": "480p"}}
	projected, ok := canonicalVideoDownstreamRequest(legacy)
	if ok {
		t.Fatal("legacy request unexpectedly entered canonical path")
	}
	if parameterInt(projected.Params, "duration") != 5 || parameterString(projected.Params, "aspect_ratio") != "16:9" || parameterString(projected.Params, "resolution") != "480p" {
		t.Fatalf("legacy fallback changed params: %#v", projected.Params)
	}
}
