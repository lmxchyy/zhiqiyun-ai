package httpserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	"xianzhi-ai/backend-go/internal/app/smartvideo"
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
	payload map[string][]byte
}

func (p *generatedStorageTestProvider) PutObject(_ context.Context, key string, source io.Reader, size int64, contentType string) (storagecenter.ObjectMetadata, error) {
	raw, err := io.ReadAll(io.LimitReader(source, size+1))
	if err != nil {
		return storagecenter.ObjectMetadata{}, err
	}
	if p.payload == nil {
		p.payload = map[string][]byte{}
	}
	p.payload[key] = append([]byte(nil), raw...)
	metadata := storagecenter.ObjectMetadata{Size: size, ContentType: contentType, ETag: "generated-etag"}
	p.objects[key] = metadata
	return metadata, nil
}

func (p *generatedStorageTestProvider) OpenObject(_ context.Context, key string) (io.ReadCloser, error) {
	item, ok := p.objects[key]
	if !ok {
		return nil, storagecenter.ErrFileNotFound
	}
	if raw, exists := p.payload[key]; exists {
		return io.NopCloser(bytes.NewReader(raw)), nil
	}
	return io.NopCloser(strings.NewReader(strings.Repeat("\x00", int(item.Size)))), nil
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

func (p *generatedStorageTestProvider) CreatePresignedUploadURL(_ context.Context, key, _ string, _ time.Duration) (string, error) {
	return "https://storage.example/upload/" + key, nil
}

func (p *generatedStorageTestProvider) CreatePresignedDownloadURL(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://storage.example/download/" + key, nil
}

func (p *generatedStorageTestProvider) TestConnection(context.Context) error { return nil }

type generatedStorageVideoProvider struct {
	url string
}

func (p generatedStorageVideoProvider) DefaultModel() string { return "mock-video" }
func (p generatedStorageVideoProvider) Create(context.Context, generation.CreateRequest) (any, error) {
	return map[string]any{"videoUrl": p.url, "provider": "test-video-provider"}, nil
}

func TestRunVideoGenerationTaskPersistsPrivateVideoWithJPEGThumbnail(t *testing.T) {
	previousReader := generatedVideoArtifactReader
	generatedVideoArtifactReader = func(context.Context, string) ([]byte, string, string, error) {
		return []byte("test-mp4-payload"), "video/mp4", "mp4", nil
	}
	defer func() { generatedVideoArtifactReader = previousReader }()
	previousProbe := generatedVideoMetadataProbe
	generatedVideoMetadataProbe = func(context.Context, []byte, string) (smartvideo.VideoMetadata, error) {
		return smartvideo.VideoMetadata{DurationMS: 5000, Width: 640, Height: 360}, nil
	}
	defer func() { generatedVideoMetadataProbe = previousProbe }()
	previousThumbnailExtractor := generatedVideoThumbnailExtractor
	generatedVideoThumbnailExtractor = func(context.Context, []byte) (string, error) {
		return jpegDataURL(t, 4, 3), nil
	}
	defer func() { generatedVideoThumbnailExtractor = previousThumbnailExtractor }()

	repo := storagecenter.NewMemoryRepository()
	provider := &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}}
	fileService := storagecenter.NewService(repo, generatedStorageTestFactory{provider: provider}, storagecenter.Options{
		DefaultProvider: "s3", Endpoint: "https://storage.example", AccessKey: "access", SecretKey: "secret",
		Bucket: "private-files", DefaultQuotaBytes: 1024, MaxUploadBytes: 1024,
		MasterKey: "0123456789abcdef0123456789abcdef",
	})
	store := newBillingAcceptanceStore(t)
	req := generation.CreateRequest{
		UserID: billingAcceptanceUserID, Type: "TEXT_TO_VIDEO", Prompt: "persist generated video", Model: "mock-video",
		Params: map[string]any{"tenant_id": "tenant_default", "duration": 5},
	}
	task, err := store.CreatePendingGenerationTask(req)
	if err != nil {
		t.Fatal(err)
	}
	service := generation.NewServiceWithOptions(generation.ServiceOptions{
		VideoProvider: generatedStorageVideoProvider{url: "https://video.example/result.mp4"},
	})
	api{store: store, fileService: fileService}.runVideoGenerationTask(task.ID, service, req)

	items, err := store.ListAssets()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("asset count=%d, want 1", len(items))
	}
	if stringValue(items[0].Metadata["fileId"]) == "" || !boolValue(items[0].Metadata["storageManaged"]) {
		t.Fatalf("video asset was not persisted to private storage: url=%q metadata=%+v", items[0].URL, items[0].Metadata)
	}
	if items[0].MediaType != "video" || items[0].URL != "" || stringValue(items[0].Metadata["sourceUrl"]) != "" {
		t.Fatalf("video asset retained upstream URL: mediaType=%q url=%q metadata=%+v", items[0].MediaType, items[0].URL, items[0].Metadata)
	}
	if stringValue(items[0].Metadata["source"]) != "test-video-provider" {
		t.Fatalf("video asset provider=%q, want test-video-provider", stringValue(items[0].Metadata["source"]))
	}
	if !strings.HasPrefix(items[0].ThumbnailURL, "data:image/jpeg;base64,") {
		t.Fatalf("video asset thumbnail=%q, want generated JPEG data URL", items[0].ThumbnailURL)
	}
	if duration, ok := items[0].Metadata["duration"].(float64); !ok || duration != 5 ||
		intValue(items[0].Metadata["width"]) != 640 || intValue(items[0].Metadata["height"]) != 360 {
		t.Fatalf("video asset metadata=%+v, want duration 5 and dimensions 640x360", items[0].Metadata)
	}
	if len(provider.objects) != 1 {
		t.Fatalf("stored object count=%d, want 1", len(provider.objects))
	}
	signed := (api{fileService: fileService}).signStoredAssetURLs(t.Context(), req.UserID, items)
	if len(signed) != 1 || !strings.Contains(signed[0].URL, "/download/") {
		t.Fatalf("managed video URL was not signed from file ID: %+v", signed)
	}
	tasks, err := store.ListGenerationTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("task count=%d, want 1", len(tasks))
	}
	if providerTask := providerTaskPayload(createGenerationTaskRequest{Params: tasks[0].Params}); providerTask != nil &&
		(stringValue(providerTask["videoUrl"]) != "" || stringValue(providerTask["sourceUrl"]) != "") {
		t.Fatalf("generation task retained upstream URL: %+v", providerTask)
	}
}

func TestGeneratedVideoThumbnailHandlesMP4ThatRequiresSeeking(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is unavailable")
	}
	videoPath := filepath.Join(t.TempDir(), "seek-required.mp4")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-f", "lavfi", "-i", "color=c=blue:s=640x360:d=1", "-c:v", "libx264", "-pix_fmt", "yuv420p", videoPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create MP4 fixture: %v: %s", err, output)
	}
	raw, err := os.ReadFile(videoPath)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := probeGeneratedVideoArtifact(t.Context(), raw, "mp4")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.DurationMS <= 0 || metadata.Width != 640 || metadata.Height != 360 {
		t.Fatalf("probed video metadata=%+v, want positive duration and 640x360", metadata)
	}
	thumbnailURL, err := generatedVideoThumbnailDataURL(t.Context(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(thumbnailURL, "data:image/jpeg;base64,") {
		t.Fatalf("thumbnail=%q, want JPEG data URL", thumbnailURL)
	}
	if err := validateGeneratedVideoThumbnailDataURL(thumbnailURL); err != nil {
		t.Fatalf("generated thumbnail is not a real JPEG: %v", err)
	}
}

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
	if stringValue(item.Metadata["sourceUrl"]) != "" || strings.HasPrefix(item.URL, "data:") {
		t.Fatalf("stored asset duplicated an inline original: url=%q metadata=%+v", item.URL, item.Metadata)
	}
	signed := a.signStoredAssetURLs(context.Background(), req.UserID, []asset{item})
	if len(signed) != 1 || !strings.Contains(signed[0].URL, "/download/"+files[0].ObjectKey) || signed[0].ThumbnailURL != signed[0].URL {
		t.Fatalf("asset URL was not signed from storage: %+v", signed)
	}
}

func TestWriteAssetDownloadStreamsPrivateObjectStorage(t *testing.T) {
	repo := storagecenter.NewMemoryRepository()
	provider := &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}}
	service := storagecenter.NewService(repo, generatedStorageTestFactory{provider: provider}, storagecenter.Options{
		DefaultProvider: "s3", Endpoint: "http://minio:9000", PublicEndpoint: "http://minio:9000",
		AccessKey: "access", SecretKey: "secret", Bucket: "private-files",
		DefaultQuotaBytes: 1024, MaxUploadBytes: 1024, MasterKey: "0123456789abcdef0123456789abcdef",
	})
	a := api{fileService: service}
	raw := []byte("private-original-bytes")
	prepared, files, err := a.persistGeneratedImages(context.Background(), "task_private", generation.CreateRequest{
		UserID: "user_1", Type: "TEXT_TO_IMAGE", Prompt: "prompt", Model: "model",
		Params:          map[string]any{"tenant_id": "tenant_default"},
		GeneratedImages: []generation.GeneratedImage{{URL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw), ContentType: "image/png"}},
	})
	if err != nil || len(files) != 1 {
		t.Fatalf("persist failed: files=%#v err=%v", files, err)
	}
	item := generatedAssetForRequest(prepared, "user_1", "task_private", "asset_private", 0, time.Now().UTC().Format(time.RFC3339Nano))
	item.URL = "http://minio:9000/private-files/" + files[0].ObjectKey + "?X-Amz-Signature=deadbeef"
	item.Metadata["ai_generated"] = false

	response := httptest.NewRecorder()
	a.writeAssetDownload(response, httptest.NewRequest(http.MethodGet, "/assets/asset_private/download", nil), item)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Body.String() != string(raw) {
		t.Fatalf("body=%q want=%q", response.Body.String(), raw)
	}
}

func TestPPTStorageReferenceMaterializesFreshSignedURLs(t *testing.T) {
	repo := storagecenter.NewMemoryRepository()
	provider := &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}}
	service := storagecenter.NewService(repo, generatedStorageTestFactory{provider: provider}, storagecenter.Options{
		DefaultProvider: "s3", Endpoint: "https://storage.example", AccessKey: "access", SecretKey: "secret",
		Bucket: "private-files", DefaultQuotaBytes: 1024, MaxUploadBytes: 1024,
		MasterKey: "0123456789abcdef0123456789abcdef",
	})
	a := api{fileService: service}
	raw := []byte("generated")
	prepared, files, err := a.persistGeneratedImages(t.Context(), "task_1", generation.CreateRequest{
		UserID: "user_1", Type: "TEXT_TO_IMAGE", Prompt: "prompt", Model: "model",
		Params:          map[string]any{"tenant_id": "tenant_default"},
		GeneratedImages: []generation.GeneratedImage{{URL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw), ContentType: "image/png"}},
	})
	if err != nil || len(files) != 1 || len(prepared.GeneratedImages) != 1 {
		t.Fatalf("persist generated image failed: files=%#v err=%v", files, err)
	}
	ref := pptStorageReference(files[0])
	tenantID, fileID, ok := parsePPTStorageReference(ref)
	if !ok || tenantID != files[0].TenantID || fileID != files[0].FileID {
		t.Fatalf("storage reference did not round trip: ref=%q tenant=%q file=%q", ref, tenantID, fileID)
	}
	task := pptapp.Task{Slides: []pptapp.Slide{{
		ID: "slide_1", ImageURL: ref,
		VisualHistory: []pptapp.VisualAsset{{URL: ref, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}},
	}}}
	materialized := a.materializePPTTaskVisualURLs(t.Context(), adminUser{ID: "user_1"}, task)
	slide := materialized.Slides[0]
	if slide.VisualStorageRef != ref || !strings.Contains(slide.ImageURL, "/download/"+files[0].ObjectKey) {
		t.Fatalf("current visual was not materialized: %#v", slide)
	}
	if len(slide.VisualHistory) != 1 || slide.VisualHistory[0].StorageRef != ref || !strings.Contains(slide.VisualHistory[0].URL, "/download/"+files[0].ObjectKey) {
		t.Fatalf("historical visual was not materialized: %#v", slide.VisualHistory)
	}
	unauthorized := a.materializePPTTaskVisualURLs(t.Context(), adminUser{ID: "user_2"}, task)
	if unauthorized.Slides[0].ImageURL != ref || unauthorized.Slides[0].VisualStorageRef != "" {
		t.Fatalf("another user received a signed URL for a private PPT visual: %#v", unauthorized.Slides[0])
	}
}
