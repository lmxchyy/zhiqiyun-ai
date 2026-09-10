package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEnrichAssetAvailability_ManagedVideo(t *testing.T) {
	item := asset{
		ID:        "asset_managed",
		MediaType: "video",
		URL:       "https://ai.example.com/api/v1/files/file_123",
		Metadata: map[string]any{
			"fileId":         "file_123",
			"storageManaged": true,
		},
	}
	enriched := enrichAssetAvailability(item)
	if enriched.Availability != AssetAvailabilityAvailable {
		t.Fatalf("expected availability AVAILABLE, got %s", enriched.Availability)
	}
	if enriched.VideoStatus != AssetAvailabilityAvailable {
		t.Fatalf("expected videoStatus AVAILABLE, got %s", enriched.VideoStatus)
	}
	if enriched.URL == "" {
		t.Fatalf("expected URL to be preserved, got empty")
	}
}

func TestEnrichAssetAvailability_ExpiredVideo(t *testing.T) {
	pastTime := time.Now().UTC().Add(-72 * time.Hour).Format(time.RFC3339Nano)
	rawProviderURL := "https://upstream.example.com/temp-video.mp4?signature=expired"
	item := asset{
		ID:        "asset_task_000238",
		TaskID:    "task_000238",
		MediaType: "video",
		URL:       rawProviderURL,
		CreatedAt: pastTime,
		Metadata: map[string]any{
			"model": "grok-imagine-1.5-video",
		},
	}
	enriched := enrichAssetAvailability(item)
	if enriched.Availability != AssetAvailabilityExpired {
		t.Fatalf("expected availability EXPIRED, got %s", enriched.Availability)
	}
	if enriched.VideoStatus != AssetAvailabilityExpired {
		t.Fatalf("expected videoStatus EXPIRED, got %s", enriched.VideoStatus)
	}
	if enriched.URL != "" {
		t.Fatalf("expected URL to be cleared for expired video, got %s", enriched.URL)
	}
	if enriched.Metadata["expiredSourceUrl"] != rawProviderURL {
		t.Fatalf("expected expiredSourceUrl in metadata, got %#v", enriched.Metadata["expiredSourceUrl"])
	}
	if !strings.Contains(enriched.AvailabilityReason, "已失效") {
		t.Fatalf("expected friendly availabilityReason, got %s", enriched.AvailabilityReason)
	}
}

func TestEnrichAssetAvailability_RecentTempVideo(t *testing.T) {
	recentTime := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339Nano)
	rawProviderURL := "https://upstream.example.com/temp-video.mp4?signature=valid"
	item := asset{
		ID:        "asset_recent",
		MediaType: "video",
		URL:       rawProviderURL,
		CreatedAt: recentTime,
		Metadata: map[string]any{
			"model": "grok-imagine-1.5-video",
		},
	}
	enriched := enrichAssetAvailability(item)
	if enriched.Availability != AssetAvailabilityProviderTempURL {
		t.Fatalf("expected availability PROVIDER_TEMP_URL, got %s", enriched.Availability)
	}
	if enriched.VideoStatus != "TEMP" {
		t.Fatalf("expected videoStatus TEMP, got %s", enriched.VideoStatus)
	}
	if enriched.URL == "" {
		t.Fatalf("expected URL to be preserved for recent temp video, got empty")
	}
}

func TestEnrichAssetAvailability_MissingURLVideo(t *testing.T) {
	item := asset{
		ID:        "asset_empty",
		MediaType: "video",
		URL:       "",
		Metadata:  map[string]any{},
	}
	enriched := enrichAssetAvailability(item)
	if enriched.Availability != AssetAvailabilityMissing {
		t.Fatalf("expected availability MISSING, got %s", enriched.Availability)
	}
	if enriched.VideoStatus != AssetAvailabilityMissing {
		t.Fatalf("expected videoStatus MISSING, got %s", enriched.VideoStatus)
	}
}

func TestWriteAssetDownload_ExpiredVideoReturnsGone(t *testing.T) {
	a := api{}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/assets/asset_expired/download", nil)
	item := asset{
		ID:           "asset_expired",
		MediaType:    "video",
		Availability: AssetAvailabilityExpired,
		Metadata: map[string]any{
			"availability": AssetAvailabilityExpired,
		},
	}
	a.writeAssetDownload(w, r, item)
	if w.Code != http.StatusGone {
		t.Fatalf("expected status 410 Gone for expired video download, got %d (body: %s)", w.Code, w.Body.String())
	}
	var res map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !strings.Contains(stringValue(res["error"]), "已失效") {
		t.Fatalf("expected error message to contain '已失效', got %s", stringValue(res["error"]))
	}
}
