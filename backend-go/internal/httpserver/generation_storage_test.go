package httpserver

import (
	"context"
	"encoding/base64"
	"io"
	"strings"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type generatedStorageTestFactory struct {
	provider storagecenter.Provider
}

func (f generatedStorageTestFactory) Build(storagecenter.Config) (storagecenter.Provider, error) {
	return f.provider, nil
}

type generatedStorageTestProvider struct {
	objects map[string]storagecenter.ObjectMetadata
}

func (p *generatedStorageTestProvider) PutObject(_ context.Context, key string, source io.Reader, size int64, contentType string) (storagecenter.ObjectMetadata, error) {
	_, _ = io.Copy(io.Discard, source)
	metadata := storagecenter.ObjectMetadata{Size: size, ContentType: contentType, ETag: "generated-etag"}
	p.objects[key] = metadata
	return metadata, nil
}

func (p *generatedStorageTestProvider) HeadObject(_ context.Context, key string) (storagecenter.ObjectMetadata, error) {
	item, ok := p.objects[key]
	if !ok {
		return storagecenter.ObjectMetadata{}, storagecenter.ErrFileNotFound
	}
	return item, nil
}

func (p *generatedStorageTestProvider) DeleteObject(_ context.Context, key string) error {
	delete(p.objects, key)
	return nil
}

func (p *generatedStorageTestProvider) CopyObject(_ context.Context, source string, target string) error {
	p.objects[target] = p.objects[source]
	return nil
}

func (p *generatedStorageTestProvider) CreatePresignedUploadURL(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://storage.example/upload/" + key, nil
}

func (p *generatedStorageTestProvider) CreatePresignedDownloadURL(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://storage.example/download/" + key, nil
}

func (p *generatedStorageTestProvider) TestConnection(context.Context) error { return nil }

func TestPersistGeneratedImagesBindsFileAndSignsAssetURL(t *testing.T) {
	repo := storagecenter.NewMemoryRepository()
	provider := &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}}
	service := storagecenter.NewService(repo, generatedStorageTestFactory{provider: provider}, storagecenter.Options{
		DefaultProvider:   "s3",
		Endpoint:          "https://storage.example",
		AccessKey:         "access",
		SecretKey:         "secret",
		Bucket:            "private-files",
		DefaultQuotaBytes: 1024,
		MaxUploadBytes:    1024,
		MasterKey:         "0123456789abcdef0123456789abcdef",
	})
	a := api{fileService: service}
	raw := []byte("generated")
	req := generation.CreateRequest{
		UserID: "user_1",
		Type:   "TEXT_TO_IMAGE",
		Prompt: "test prompt",
		Model:  "test-model",
		Params: map[string]any{"tenant_id": "tenant_default"},
		GeneratedImages: []generation.GeneratedImage{{
			URL:         "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw),
			ContentType: "image/png",
		}},
	}
	prepared, files, err := a.persistGeneratedImages(context.Background(), "task_1", req)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Status != storagecenter.StatusActive || files[0].BusinessID != "task_1" {
		t.Fatalf("unexpected persisted files: %+v", files)
	}
	if _, ok := provider.objects[files[0].ObjectKey]; !ok {
		t.Fatalf("provider object %q was not written", files[0].ObjectKey)
	}
	item := generatedAssetForRequest(prepared, req.UserID, "task_1", "asset_1", 0, time.Now().UTC().Format(time.RFC3339Nano))
	if stringValue(item.Metadata["fileId"]) != files[0].FileID || !boolValue(item.Metadata["storageManaged"]) {
		t.Fatalf("asset is not bound to stored file: %+v", item.Metadata)
	}
	signed := a.signStoredAssetURLs(context.Background(), req.UserID, []asset{item})
	if len(signed) != 1 || !strings.Contains(signed[0].URL, "/download/"+files[0].ObjectKey) || signed[0].ThumbnailURL != signed[0].URL {
		t.Fatalf("asset URL was not signed from storage: %+v", signed)
	}
}
