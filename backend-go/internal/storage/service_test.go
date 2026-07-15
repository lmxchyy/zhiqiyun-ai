package storage

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

type fakeProviderFactory struct {
	provider Provider
	last     Config
}

func (f *fakeProviderFactory) Build(config Config) (Provider, error) {
	f.last = config
	return f.provider, nil
}

type fakeProvider struct {
	objects map[string]ObjectMetadata
	deleted []string
}

func newFakeProvider() *fakeProvider {
	return &fakeProvider{objects: map[string]ObjectMetadata{}}
}

func (p *fakeProvider) PutObject(_ context.Context, key string, source io.Reader, size int64, contentType string) (ObjectMetadata, error) {
	_, _ = io.Copy(io.Discard, source)
	object := ObjectMetadata{Size: size, ContentType: contentType, ETag: "etag-test"}
	p.objects[key] = object
	return object, nil
}

func (p *fakeProvider) HeadObject(_ context.Context, key string) (ObjectMetadata, error) {
	object, ok := p.objects[key]
	if !ok {
		return ObjectMetadata{}, ErrFileNotFound
	}
	return object, nil
}

func (p *fakeProvider) DeleteObject(_ context.Context, key string) error {
	delete(p.objects, key)
	p.deleted = append(p.deleted, key)
	return nil
}

func (p *fakeProvider) CopyObject(_ context.Context, source string, target string) error {
	p.objects[target] = p.objects[source]
	return nil
}

func (p *fakeProvider) CreatePresignedUploadURL(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://storage.example/upload/" + key, nil
}

func (p *fakeProvider) CreatePresignedDownloadURL(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://storage.example/download/" + key, nil
}

func (p *fakeProvider) TestConnection(context.Context) error { return nil }

func testService(quota int64) (*Service, *MemoryRepository, *fakeProvider, *fakeProviderFactory) {
	repo := NewMemoryRepository()
	provider := newFakeProvider()
	factory := &fakeProviderFactory{provider: provider}
	service := NewService(repo, factory, Options{
		DefaultProvider: "minio", Endpoint: "http://minio:9000", AccessKey: "access", SecretKey: "secret", Bucket: "files",
		DefaultQuotaBytes: quota, MaxUploadBytes: 1024, UploadURLTTL: time.Minute, AccessURLTTL: time.Minute, RecycleRetention: time.Hour,
		MasterKey: "0123456789abcdef0123456789abcdef",
	})
	return service, repo, provider, factory
}

func TestUploadCompleteAccessAndRecycleLifecycle(t *testing.T) {
	service, _, provider, _ := testService(100)
	ctx := context.Background()
	ticket, err := service.InitUpload(ctx, UploadInitInput{
		TenantID: "tenant_a", UserID: "user_a", FileName: "产品资料.pdf", FileSize: 5,
		MIMEType: "application/pdf", BusinessType: "knowledge_base", Visibility: "private",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ticket.File.ObjectKey, "tenants/tenant_a/knowledge_base/") {
		t.Fatalf("unexpected object key %q", ticket.File.ObjectKey)
	}
	if ticket.UploadURL == "" || ticket.Headers["Content-Type"] != "application/pdf" {
		t.Fatalf("invalid upload ticket: %+v", ticket)
	}
	provider.objects[ticket.File.ObjectKey] = ObjectMetadata{Size: 5, ContentType: "application/pdf", ETag: "etag-1"}
	access := AccessContext{TenantID: "tenant_a", UserID: "user_a"}
	file, err := service.CompleteUpload(ctx, access, ticket.File.FileID)
	if err != nil {
		t.Fatal(err)
	}
	if file.Status != StatusActive || file.FileSize != 5 || file.ETag != "etag-1" {
		t.Fatalf("unexpected completed file: %+v", file)
	}
	quota, err := service.Quota(ctx, "tenant_a")
	if err != nil {
		t.Fatal(err)
	}
	if quota.UsedBytes != 5 || quota.ReservedBytes != 0 || quota.FileCount != 1 {
		t.Fatalf("unexpected quota after complete: %+v", quota)
	}
	if _, err = service.GetFile(ctx, AccessContext{TenantID: "tenant_b", UserID: "user_a"}, file.FileID); err == nil {
		t.Fatal("cross-tenant file access must be rejected")
	}
	if _, err = service.GetFile(ctx, AccessContext{TenantID: "tenant_a", UserID: "user_b"}, file.FileID); err != ErrFileForbidden {
		t.Fatalf("private file access error = %v", err)
	}
	accessTicket, err := service.AccessURL(ctx, access, file.FileID, true)
	if err != nil || !strings.Contains(accessTicket.URL, file.ObjectKey) {
		t.Fatalf("access ticket = %+v err=%v", accessTicket, err)
	}
	deleted, err := service.Delete(ctx, access, file.FileID)
	if err != nil || deleted.Status != StatusDeletePending || deleted.RecycleExpiresAt == nil {
		t.Fatalf("delete result = %+v err=%v", deleted, err)
	}
	restored, err := service.Restore(ctx, access, file.FileID)
	if err != nil || restored.Status != StatusActive {
		t.Fatalf("restore result = %+v err=%v", restored, err)
	}
	if _, err = service.Delete(ctx, access, file.FileID); err != nil {
		t.Fatal(err)
	}
	if err = service.PermanentDelete(ctx, access, file.FileID); err != nil {
		t.Fatal(err)
	}
	quota, _ = service.Quota(ctx, "tenant_a")
	if quota.UsedBytes != 0 || quota.FileCount != 0 || len(provider.deleted) != 1 {
		t.Fatalf("quota/provider after permanent delete: quota=%+v deleted=%v", quota, provider.deleted)
	}
}

func TestQuotaReservationPreventsConcurrentOvercommit(t *testing.T) {
	service, _, _, _ := testService(10)
	ctx := context.Background()
	_, err := service.InitUpload(ctx, UploadInitInput{TenantID: "tenant_a", UserID: "user_a", FileName: "a.pdf", FileSize: 8, MIMEType: "application/pdf"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.InitUpload(ctx, UploadInitInput{TenantID: "tenant_a", UserID: "user_a", FileName: "b.pdf", FileSize: 3, MIMEType: "application/pdf"})
	if err != ErrQuotaExceeded {
		t.Fatalf("second reservation error = %v", err)
	}
}

func TestStoreObjectCompletesServerSideUpload(t *testing.T) {
	service, _, provider, _ := testService(100)
	ctx := context.Background()
	file, err := service.StoreObject(ctx, UploadInitInput{
		TenantID: "tenant_a", UserID: "user_a", FileName: "generated.png", FileSize: 9,
		MIMEType: "image/png", BusinessType: "generation_result", BusinessID: "task_123", Visibility: "PRIVATE",
	}, strings.NewReader("generated"))
	if err != nil {
		t.Fatal(err)
	}
	if file.Status != StatusActive || file.FileSize != 9 || file.BusinessID != "task_123" {
		t.Fatalf("unexpected stored file: %+v", file)
	}
	if _, ok := provider.objects[file.ObjectKey]; !ok {
		t.Fatalf("provider object %q was not written", file.ObjectKey)
	}
	quota, err := service.Quota(ctx, "tenant_a")
	if err != nil {
		t.Fatal(err)
	}
	if quota.UsedBytes != 9 || quota.ReservedBytes != 0 || quota.FileCount != 1 {
		t.Fatalf("unexpected quota after server-side upload: %+v", quota)
	}
}

func TestStorageConfigCredentialsAreEncryptedAndHydratedOnlyForProvider(t *testing.T) {
	service, repo, provider, factory := testService(100)
	provider.objects["health"] = ObjectMetadata{Size: 1}
	ctx := context.Background()
	saved, err := service.SaveConfig(ctx, Config{
		TenantID: "tenant_a", Name: "Tenant MinIO", Provider: "minio", Endpoint: "http://tenant-minio:9000", Bucket: "tenant-files",
		ForcePathStyle: true, IsDefault: true, Status: "ENABLED", CreatedBy: "admin", UpdatedBy: "admin",
	}, "tenant-access", "tenant-secret", "")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := repo.GetConfig(ctx, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if raw.AccessKeyEncrypted == "" || raw.SecretKeyEncrypted == "" || strings.Contains(raw.AccessKeyEncrypted, "tenant-access") || strings.Contains(raw.SecretKeyEncrypted, "tenant-secret") {
		t.Fatalf("credentials were not encrypted: %+v", raw)
	}
	ticket, err := service.InitUpload(ctx, UploadInitInput{TenantID: "tenant_a", UserID: "user_a", FileName: "a.pdf", FileSize: 1, MIMEType: "application/pdf"})
	if err != nil {
		t.Fatal(err)
	}
	if ticket.File.StorageConfigID != saved.ID || factory.last.AccessKey != "tenant-access" || factory.last.SecretKey != "tenant-secret" {
		t.Fatalf("tenant config was not resolved/decrypted: file=%+v providerConfig=%+v", ticket.File, factory.last)
	}
	encoded, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "tenant-secret") {
		t.Fatal("secret leaked through config JSON")
	}
}
