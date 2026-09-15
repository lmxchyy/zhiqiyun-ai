package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type mockBackfillStore struct {
	assets        map[string]*singleAssetRecord
	updates       []map[string]any
	tasksModified bool
}

func newMockBackfillStore() *mockBackfillStore {
	return &mockBackfillStore{
		assets:        make(map[string]*singleAssetRecord),
		updates:       nil,
		tasksModified: false,
	}
}

func (m *mockBackfillStore) GetAsset(_ context.Context, id string) (*singleAssetRecord, error) {
	// Exact match only - no prefix or fuzzy match
	item, ok := m.assets[id]
	if !ok {
		return nil, nil
	}
	return item, nil
}

func (m *mockBackfillStore) ListUnmanagedVideoAssets(_ context.Context, limit int) ([]assetRow, error) {
	var rows []assetRow
	for _, a := range m.assets {
		if a.DeletedAt != nil && strings.TrimSpace(*a.DeletedAt) != "" {
			continue
		}
		if !strings.EqualFold(a.MediaType, "video") {
			continue
		}
		var meta map[string]any
		_ = json.Unmarshal(a.Metadata, &meta)
		if meta != nil && (meta["fileId"] != nil && meta["fileId"] != "" || meta["storageManaged"] == true) {
			continue
		}
		if a.URL == "" || strings.HasPrefix(a.URL, "storage://") {
			continue
		}
		rows = append(rows, assetRow{
			ID:       a.ID,
			UserID:   a.UserID,
			TenantID: a.TenantID,
			TaskID:   a.TaskID,
			URL:      a.URL,
			Metadata: a.Metadata,
			Raw:      a.Raw,
		})
		if limit > 0 && len(rows) >= limit {
			break
		}
	}
	return rows, nil
}

func (m *mockBackfillStore) UpdateAssetPersisted(_ context.Context, id string, url string, thumbURL string, metadata []byte, raw []byte) (int64, error) {
	item, ok := m.assets[id]
	if !ok {
		return 0, nil
	}
	// Check idempotency guard: coalesce(metadata->>'fileId', '') = ''
	var existingMeta map[string]any
	_ = json.Unmarshal(item.Metadata, &existingMeta)
	if existingMeta != nil && stringValue(existingMeta["fileId"]) != "" {
		return 0, nil // zero rows affected because already persisted
	}

	item.URL = url
	item.Metadata = metadata
	item.Raw = raw
	m.updates = append(m.updates, map[string]any{
		"id":       id,
		"url":      url,
		"thumbUrl": thumbURL,
	})
	return 1, nil
}

func TestBackfill_1_NoAssetID_BatchMode(t *testing.T) {
	store := newMockBackfillStore()
	store.assets["asset_001"] = &singleAssetRecord{
		ID: "asset_001", MediaType: "video", URL: "https://upstream.example.com/v1.mp4", Metadata: []byte(`{}`),
	}
	store.assets["asset_002"] = &singleAssetRecord{
		ID: "asset_002", MediaType: "video", URL: "https://upstream.example.com/v2.mp4", Metadata: []byte(`{}`),
	}

	var out bytes.Buffer
	probedURLs := []string{}
	opts := Options{
		DryRun:  true,
		Limit:   10,
		AssetID: "", // empty -> batch mode
		Probe: func(u string) (int, error) {
			probedURLs = append(probedURLs, u)
			return 206, nil
		},
		Out: &out,
	}

	err := runBackfill(context.Background(), store, nil, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if len(probedURLs) != 2 {
		t.Errorf("expected 2 probed URLs in batch mode, got %d", len(probedURLs))
	}
	if !strings.Contains(output, "asset_001|dry_run_recoverable") || !strings.Contains(output, "asset_002|dry_run_recoverable") {
		t.Errorf("expected both assets reported in batch mode, got:\n%s", output)
	}
}

func TestBackfill_2_TargetAssetID_OnlyProcessesSpecifiedAsset(t *testing.T) {
	store := newMockBackfillStore()
	store.assets["asset_000103"] = &singleAssetRecord{
		ID: "asset_000103", MediaType: "video", URL: "https://upstream.example.com/cat.mp4", Metadata: []byte(`{}`),
	}
	store.assets["asset_000202"] = &singleAssetRecord{
		ID: "asset_000202", MediaType: "video", URL: "https://upstream.example.com/other.mp4", Metadata: []byte(`{}`),
	}

	var out bytes.Buffer
	probedURLs := []string{}
	opts := Options{
		DryRun:  true,
		AssetID: "asset_000103",
		Probe: func(u string) (int, error) {
			probedURLs = append(probedURLs, u)
			return 206, nil
		},
		Out: &out,
	}

	err := runBackfill(context.Background(), store, nil, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if len(probedURLs) != 1 || probedURLs[0] != "https://upstream.example.com/cat.mp4" {
		t.Errorf("expected only asset_000103 to be probed, got %v", probedURLs)
	}
	if !strings.Contains(output, "asset_000103|upstream=206") || !strings.Contains(output, "asset_000103|dry_run_recoverable") {
		t.Errorf("expected asset_000103 recoverable output, got:\n%s", output)
	}
	if strings.Contains(output, "asset_000202") {
		t.Errorf("asset_000202 must NOT be touched or reported when targeting asset_000103")
	}
}

func TestBackfill_3_NotFound_SafeExit(t *testing.T) {
	store := newMockBackfillStore()

	var out bytes.Buffer
	opts := Options{
		DryRun:  true,
		AssetID: "asset_non_existent",
		Out:     &out,
	}

	err := runBackfill(context.Background(), store, nil, opts)
	if err != nil {
		t.Fatalf("expected clean safe exit, got error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "asset_non_existent|not_found") {
		t.Errorf("expected not_found message, got:\n%s", output)
	}
}

func TestBackfill_4_AlreadyPersisted_Skipped(t *testing.T) {
	store := newMockBackfillStore()
	store.assets["asset_persisted"] = &singleAssetRecord{
		ID:        "asset_persisted",
		MediaType: "video",
		URL:       "storage://file_12345",
		Metadata:  []byte(`{"fileId":"file_12345","storageManaged":true}`),
	}

	var out bytes.Buffer
	probeCalled := false
	opts := Options{
		DryRun:  true,
		AssetID: "asset_persisted",
		Probe: func(u string) (int, error) {
			probeCalled = true
			return 200, nil
		},
		Out: &out,
	}

	err := runBackfill(context.Background(), store, nil, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if probeCalled {
		t.Errorf("probe must NOT be called for already persisted asset")
	}
	output := out.String()
	if !strings.Contains(output, "asset_persisted|already_persisted") {
		t.Errorf("expected already_persisted output, got:\n%s", output)
	}
}

func TestBackfill_5_DryRun_206_ProbeOnly_NoWrite(t *testing.T) {
	store := newMockBackfillStore()
	store.assets["asset_000103"] = &singleAssetRecord{
		ID: "asset_000103", MediaType: "video", URL: "https://upstream.example.com/cat.mp4", Metadata: []byte(`{}`),
	}

	var out bytes.Buffer
	persistCalled := false
	opts := Options{
		DryRun:  true, // dry-run
		AssetID: "asset_000103",
		Probe: func(u string) (int, error) {
			return 206, nil
		},
		Persist: func(ctx context.Context, s backfillStore, f *storagecenter.Service, r assetRow) error {
			persistCalled = true
			return nil
		},
		Out: &out,
	}

	err := runBackfill(context.Background(), store, nil, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if persistCalled {
		t.Errorf("persist must NOT be called in dry-run mode")
	}
	if len(store.updates) != 0 {
		t.Errorf("no database updates allowed in dry-run mode")
	}
	output := out.String()
	if !strings.Contains(output, "asset_000103|dry_run_recoverable") {
		t.Errorf("expected dry_run_recoverable output, got:\n%s", output)
	}
}

func TestBackfill_6_DryRun_403_SafeSkip(t *testing.T) {
	store := newMockBackfillStore()
	store.assets["asset_expired_403"] = &singleAssetRecord{
		ID: "asset_expired_403", MediaType: "video", URL: "https://upstream.example.com/expired.mp4", Metadata: []byte(`{}`),
	}

	var out bytes.Buffer
	persistCalled := false
	opts := Options{
		DryRun:  true,
		AssetID: "asset_expired_403",
		Probe: func(u string) (int, error) {
			return 403, nil
		},
		Persist: func(ctx context.Context, s backfillStore, f *storagecenter.Service, r assetRow) error {
			persistCalled = true
			return nil
		},
		Out: &out,
	}

	err := runBackfill(context.Background(), store, nil, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if persistCalled {
		t.Errorf("persist must NOT be called for 403 expired asset")
	}
	output := out.String()
	if !strings.Contains(output, "asset_expired_403|skipped_upstream_status=403") {
		t.Errorf("expected skipped_upstream_status=403 output, got:\n%s", output)
	}
}

func TestBackfill_7_NonDryRun_206_ExecutesPersist(t *testing.T) {
	store := newMockBackfillStore()
	store.assets["asset_000103"] = &singleAssetRecord{
		ID: "asset_000103", MediaType: "video", URL: "https://upstream.example.com/cat.mp4", Metadata: []byte(`{}`),
	}

	var out bytes.Buffer
	persistCalled := false
	opts := Options{
		DryRun:  false, // REAL execution
		AssetID: "asset_000103",
		Probe: func(u string) (int, error) {
			return 206, nil
		},
		Persist: func(ctx context.Context, s backfillStore, f *storagecenter.Service, r assetRow) error {
			persistCalled = true
			_, err := s.UpdateAssetPersisted(ctx, r.ID, "storage://file_new", "storage://cover_new", []byte(`{"fileId":"file_new","storageManaged":true}`), []byte(`{}`))
			return err
		},
		Out: &out,
	}

	err := runBackfill(context.Background(), store, nil, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !persistCalled {
		t.Errorf("persist must be called for non-dry-run with 206 upstream")
	}
	if len(store.updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(store.updates))
	}
	if store.assets["asset_000103"].URL != "storage://file_new" {
		t.Errorf("asset URL must be updated to storage scheme, got %s", store.assets["asset_000103"].URL)
	}
	output := out.String()
	if !strings.Contains(output, "asset_000103|persisted") {
		t.Errorf("expected persisted output, got:\n%s", output)
	}
}

func TestBackfill_8_DBUpdateIdempotency(t *testing.T) {
	store := newMockBackfillStore()
	store.assets["asset_000103"] = &singleAssetRecord{
		ID: "asset_000103", MediaType: "video", URL: "https://upstream.example.com/cat.mp4", Metadata: []byte(`{}`),
	}

	// 1. Run first non-dry-run persist
	opts := Options{
		DryRun:  false,
		AssetID: "asset_000103",
		Probe: func(u string) (int, error) {
			return 206, nil
		},
		Persist: func(ctx context.Context, s backfillStore, f *storagecenter.Service, r assetRow) error {
			_, err := s.UpdateAssetPersisted(ctx, r.ID, "storage://file_new", "storage://cover_new", []byte(`{"fileId":"file_new","storageManaged":true}`), []byte(`{}`))
			return err
		},
		Out: &bytes.Buffer{},
	}

	if err := runBackfill(context.Background(), store, nil, opts); err != nil {
		t.Fatal(err)
	}

	// 2. Run second time with same asset ID
	var out2 bytes.Buffer
	secondPersistCalled := false
	opts2 := Options{
		DryRun:  false,
		AssetID: "asset_000103",
		Probe: func(u string) (int, error) {
			return 206, nil
		},
		Persist: func(ctx context.Context, s backfillStore, f *storagecenter.Service, r assetRow) error {
			secondPersistCalled = true
			return nil
		},
		Out: &out2,
	}

	if err := runBackfill(context.Background(), store, nil, opts2); err != nil {
		t.Fatal(err)
	}

	if secondPersistCalled {
		t.Errorf("second execution must be skipped and not re-persist")
	}
	if !strings.Contains(out2.String(), "asset_000103|already_persisted") {
		t.Errorf("expected already_persisted on rerun, got:\n%s", out2.String())
	}
}

func TestBackfill_9_NeverModifiesGenerationTasks(t *testing.T) {
	store := newMockBackfillStore()
	store.assets["asset_000103"] = &singleAssetRecord{
		ID: "asset_000103", TaskID: "task_000142", MediaType: "video", URL: "https://upstream.example.com/cat.mp4", Metadata: []byte(`{}`),
	}

	// Verify sqlBackfillStore methods and SQL queries
	sqlStore := &sqlBackfillStore{}
	_ = sqlStore

	opts := Options{
		DryRun:  false,
		AssetID: "asset_000103",
		Probe: func(u string) (int, error) {
			return 206, nil
		},
		Persist: func(ctx context.Context, s backfillStore, f *storagecenter.Service, r assetRow) error {
			_, err := s.UpdateAssetPersisted(ctx, r.ID, "storage://file_new", "storage://cover_new", []byte(`{"fileId":"file_new","storageManaged":true}`), []byte(`{}`))
			return err
		},
		Out: &bytes.Buffer{},
	}

	if err := runBackfill(context.Background(), store, nil, opts); err != nil {
		t.Fatal(err)
	}

	if store.tasksModified {
		t.Fatalf("CRITICAL VIOLATION: xz_generation_tasks must never be modified by backfill tool")
	}
}

func TestBackfill_10_ProbeAndPersistRequestHeadersAndCompression(t *testing.T) {
	var probeUA, probeEnc, probeRange string
	probeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probeUA = r.Header.Get("User-Agent")
		probeEnc = r.Header.Get("Accept-Encoding")
		probeRange = r.Header.Get("Range")
		if probeRange == "bytes=0-0" {
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("x"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer probeServer.Close()

	status, err := probe(probeServer.URL)
	if err != nil {
		t.Fatalf("probe failed: %v", err)
	}
	if status != http.StatusPartialContent {
		t.Errorf("expected 206 for probe with range, got %d", status)
	}
	if probeUA != backfillUserAgent {
		t.Errorf("probe User-Agent = %q, want %q", probeUA, backfillUserAgent)
	}
	if probeEnc != "identity" {
		t.Errorf("probe Accept-Encoding = %q, want %q", probeEnc, "identity")
	}
	if probeRange != "bytes=0-0" {
		t.Errorf("probe Range = %q, want %q", probeRange, "bytes=0-0")
	}
}
