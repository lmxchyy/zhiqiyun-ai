package httpserver

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"xianzhi-ai/backend-go/internal/config"
)

func testMediaFileHeader(t *testing.T, name string, body []byte) *multipart.FileHeader {
	t.Helper()
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(body); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	request := httptest.NewRequest("POST", "/", &buffer)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if err = request.ParseMultipartForm(1 << 20); err != nil {
		t.Fatal(err)
	}
	return request.MultipartForm.File["file"][0]
}

func TestMediaStorageFactorySupportsS3CompatibleProviders(t *testing.T) {
	for _, provider := range []string{"s3", "aliyun_oss", "tencent_cos", "qiniu"} {
		storage, err := newMediaStorage(config.Config{MediaStorageProvider: provider, S3Endpoint: "http://minio:9000", S3AccessKey: "access", S3SecretKey: "secret", S3Bucket: "assets"})
		if err != nil {
			t.Fatalf("provider %s: %v", provider, err)
		}
		if _, ok := storage.(*s3CompatibleMediaStorage); !ok {
			t.Fatalf("provider %s did not use S3-compatible storage", provider)
		}
	}
}

func TestMemorySlotsPreferTenantOverride(t *testing.T) {
	repo := newMemoryMediaRepository()
	ctx := context.Background()
	_, _ = repo.SaveSlot(ctx, pageAssetSlot{ID: "tenant_slot", TenantID: "tenant_a", PageCode: "home", ModuleCode: "hero", SlotKey: "home.hero.background", SlotName: "Tenant Hero", MaterialURL: "https://cdn.example/tenant.webp", IsEnabled: true})
	items, err := repo.ListSlots(ctx, "tenant_a", "home", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.SlotKey == "home.hero.background" {
			if item.TenantID != "tenant_a" || item.MaterialURL != "https://cdn.example/tenant.webp" {
				t.Fatalf("tenant override not selected: %#v", item)
			}
			return
		}
	}
	t.Fatal("home hero slot missing")
}

func TestPostgresPageSlotListQueryAliasesInheritedColumns(t *testing.T) {
	query := pageSlotListQuery("tenant_a", true)
	for _, fragment := range []string{
		"as asset_id",
		"as fallback_asset_id",
		"as material_url",
		"as fallback_url",
		"as alt_text",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("inherited page slot query is missing %q", fragment)
		}
	}
}
func TestValidateMediaUploadRejectsUnsafeSVG(t *testing.T) {
	header := testMediaFileHeader(t, "unsafe.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))
	if _, err := validateMediaUpload(header, 1<<20); err == nil {
		t.Fatal("expected unsafe SVG to be rejected")
	}
}
func TestMemoryMediaRepositoryTenantIsolationAndInUseProtection(t *testing.T) {
	repo := newMemoryMediaRepository()
	ctx := context.Background()
	asset, _ := repo.SaveAsset(ctx, mediaAsset{ID: "asset_a", TenantID: "tenant_a", Name: "A", FileHash: "hash-a", Status: "ACTIVE"})
	if _, err := repo.GetAsset(ctx, "tenant_b", asset.ID); err == nil {
		t.Fatal("cross-tenant asset read must fail")
	}
	slot := pageAssetSlot{ID: "slot_tenant_a", TenantID: "tenant_a", PageCode: "home", ModuleCode: "hero", SlotKey: "home.hero.background", SlotName: "Hero", AssetID: asset.ID, IsEnabled: true}
	if _, err := repo.SaveSlot(ctx, slot); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteAsset(ctx, "tenant_a", asset.ID); err != errMediaInUse {
		t.Fatalf("expected in-use error, got %v", err)
	}
}
func TestMemoryPagePublishAndRollback(t *testing.T) {
	repo := newMemoryMediaRepository()
	ctx := context.Background()
	if _, err := repo.SavePageDraft(ctx, pageConfig{TenantID: "tenant_a", PageCode: "home", ConfigJSON: map[string]any{"title": "v1"}}); err != nil {
		t.Fatal(err)
	}
	first, err := repo.PublishPage(ctx, "tenant_a", "home", "first", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if first.Version != 1 {
		t.Fatalf("expected version 1, got %d", first.Version)
	}
	_, _ = repo.SavePageDraft(ctx, pageConfig{TenantID: "tenant_a", PageCode: "home", ConfigJSON: map[string]any{"title": "v2"}})
	_, _ = repo.PublishPage(ctx, "tenant_a", "home", "second", "admin")
	rolled, err := repo.RollbackPage(ctx, "tenant_a", "home", 1, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if rolled.Version != 3 || rolled.ConfigJSON["title"] != "v1" {
		t.Fatalf("unexpected rollback result: %#v", rolled)
	}
}
