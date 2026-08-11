package media

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/smartvideo"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type processorProvider struct {
	objects map[string][]byte
	types   map[string]string
}

func (p *processorProvider) PutObject(_ context.Context, key string, source io.Reader, size int64, contentType string) (storagecenter.ObjectMetadata, error) {
	raw, err := io.ReadAll(source)
	if err != nil {
		return storagecenter.ObjectMetadata{}, err
	}
	if int64(len(raw)) != size {
		return storagecenter.ObjectMetadata{}, errors.New("size mismatch")
	}
	p.objects[key], p.types[key] = raw, contentType
	return storagecenter.ObjectMetadata{Size: size, ContentType: contentType, ETag: "test-etag"}, nil
}

func (p *processorProvider) OpenObject(_ context.Context, key string) (io.ReadCloser, error) {
	raw, ok := p.objects[key]
	if !ok {
		return nil, storagecenter.ErrFileNotFound
	}
	return io.NopCloser(bytes.NewReader(raw)), nil
}

func (p *processorProvider) HeadObject(_ context.Context, key string) (storagecenter.ObjectMetadata, error) {
	raw, ok := p.objects[key]
	if !ok {
		return storagecenter.ObjectMetadata{}, storagecenter.ErrFileNotFound
	}
	return storagecenter.ObjectMetadata{Size: int64(len(raw)), ContentType: p.types[key], ETag: "test-etag"}, nil
}

func (p *processorProvider) DeleteObject(_ context.Context, key string) error {
	delete(p.objects, key)
	delete(p.types, key)
	return nil
}

func (p *processorProvider) CopyObject(_ context.Context, source, target string) error {
	p.objects[target] = append([]byte{}, p.objects[source]...)
	p.types[target] = p.types[source]
	return nil
}

func (*processorProvider) CreatePresignedUploadURL(context.Context, string, string, time.Duration) (string, error) {
	return "https://storage.invalid/upload", nil
}

func (*processorProvider) CreatePresignedDownloadURL(context.Context, string, time.Duration) (string, error) {
	return "https://storage.invalid/download", nil
}

func (*processorProvider) CreateMultipartUpload(_ context.Context, key, _ string) (string, error) {
	return "processor-mpu-" + key, nil
}

func (*processorProvider) PresignUploadPart(_ context.Context, key, uploadID string, partNumber int, _ time.Duration) (string, error) {
	return "https://storage.invalid/multipart/" + key + "/" + uploadID + "/" + string(rune('0'+partNumber%10)), nil
}

func (*processorProvider) CompleteMultipartUpload(_ context.Context, key, _ string, _ []storagecenter.CompletedPart) (storagecenter.ObjectMetadata, error) {
	return storagecenter.ObjectMetadata{Size: 0, ContentType: "application/octet-stream", ETag: "processor-multipart"}, nil
}

func (*processorProvider) AbortMultipartUpload(context.Context, string, string) error { return nil }

func (*processorProvider) TestConnection(context.Context) error { return nil }

type processorProviderFactory struct{ provider storagecenter.Provider }

func (f processorProviderFactory) Build(storagecenter.Config) (storagecenter.Provider, error) {
	return f.provider, nil
}

type processorProbe struct{ fail bool }

func (p processorProbe) ProbeVideo(context.Context, smartvideo.LocalMedia) (smartvideo.VideoMetadata, smartvideo.FilteredProbeResult, error) {
	if p.fail {
		return smartvideo.VideoMetadata{}, smartvideo.FilteredProbeResult{}, errors.New("probe failed at C:/private/source")
	}
	return smartvideo.VideoMetadata{
		Format: "mp4", MIMEType: "video/mp4", DurationMS: 1000,
		Width: 640, Height: 360, DisplayWidth: 640, DisplayHeight: 360,
		FPSNumerator: 30, FPSDenominator: 1, VideoCodec: "h264",
	}, smartvideo.FilteredProbeResult{FormatName: "mp4", StreamCodecs: []string{"video:h264"}}, nil
}

func (p processorProbe) ProbeImage(context.Context, smartvideo.LocalMedia) (smartvideo.ImageMetadata, smartvideo.FilteredProbeResult, error) {
	if p.fail {
		return smartvideo.ImageMetadata{}, smartvideo.FilteredProbeResult{}, &smartvideo.MediaError{
			Code: smartvideo.MediaErrorProbeFailed, Message: "图片探测失败",
		}
	}
	return smartvideo.ImageMetadata{
		Format: "png", MIMEType: "image/png", Width: 1, Height: 1,
		Orientation: 1, FrameCount: 1, ColorSpace: "rgba",
	}, smartvideo.FilteredProbeResult{FormatName: "png", StreamCodecs: []string{"video:png"}}, nil
}

func (processorProbe) GenerateThumbnail(_ context.Context, _ smartvideo.LocalMedia, output string, _ smartvideo.ThumbnailOptions) error {
	return os.WriteFile(output, []byte("thumbnail"), 0o600)
}

func (processorProbe) GenerateProxy(_ context.Context, _ smartvideo.LocalMedia, output string, _ smartvideo.ProxyOptions) error {
	return os.WriteFile(output, []byte("proxy"), 0o600)
}

func (processorProbe) GetToolVersion(context.Context) (string, string, error) {
	return "ffprobe version test", "ffmpeg version test", nil
}

func TestProcessorStoresDerivedFilesAndCleansTemporaryDirectory(t *testing.T) {
	tempRoot := t.TempDir()
	files, source := newProcessorStorage(t, tinyPNG())
	task, asset := processorTaskAndAsset(source, smartvideo.AssetTypeImage)
	processor := NewProcessor(files, processorProbe{}, ProcessorOptions{
		TempDir: tempRoot, MaxFileBytes: 1024, MaxImagePixels: 1_000_000, ProxyMaxWidth: 320,
	})
	result, err := processor.Process(context.Background(), task, asset)
	if err != nil {
		t.Fatal(err)
	}
	if result.ThumbnailFileID == "" || result.ProxyFileID != "" || result.Metadata.Image == nil {
		t.Fatalf("unexpected result: %+v", result)
	}
	assertTempRootEmpty(t, tempRoot)
}

func TestProcessorCleansTemporaryDirectoryAfterProbeFailure(t *testing.T) {
	tempRoot := t.TempDir()
	files, source := newProcessorStorage(t, tinyPNG())
	task, asset := processorTaskAndAsset(source, smartvideo.AssetTypeImage)
	processor := NewProcessor(files, processorProbe{fail: true}, ProcessorOptions{
		TempDir: tempRoot, MaxFileBytes: 1024, MaxImagePixels: 1_000_000,
	})
	_, err := processor.Process(context.Background(), task, asset)
	var mediaErr *smartvideo.MediaError
	if !errors.As(err, &mediaErr) || mediaErr.Code != smartvideo.MediaErrorProbeFailed {
		t.Fatalf("error = %v, want stable probe error", err)
	}
	assertTempRootEmpty(t, tempRoot)
}

func newProcessorStorage(t *testing.T, content []byte) (*storagecenter.Service, storagecenter.FileObject) {
	t.Helper()
	provider := &processorProvider{objects: map[string][]byte{}, types: map[string]string{}}
	repository := storagecenter.NewMemoryRepository()
	service := storagecenter.NewService(repository, processorProviderFactory{provider: provider}, storagecenter.Options{
		DefaultProvider: "minio", Endpoint: "http://storage.invalid", Bucket: "test",
		AccessKey: "test", SecretKey: "test", DefaultQuotaBytes: 1 << 20, MaxUploadBytes: 1 << 20,
		AllowedExtensions: []string{"png", "jpg", "mp4"}, AllowedMIMETypes: []string{"image/png", "image/jpeg", "video/mp4"},
	})
	source, err := service.StoreObject(context.Background(), storagecenter.UploadInitInput{
		TenantID: "tenant_a", UserID: "user_a", FileName: "source.png", FileSize: int64(len(content)),
		MIMEType: "image/png", BusinessType: "smart_video_source", BusinessID: "asset_1", Visibility: "PRIVATE",
	}, bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	return service, source
}

func processorTaskAndAsset(source storagecenter.FileObject, assetType string) (smartvideo.AnalysisTask, smartvideo.ProjectAsset) {
	task := smartvideo.AnalysisTask{
		ID: "task_1", ProjectID: "project_1", AssetID: "asset_1",
		TenantID: source.TenantID, UserID: source.UserID, SourceFileID: source.FileID,
	}
	asset := smartvideo.ProjectAsset{
		ID: "asset_1", ProjectID: task.ProjectID, TenantID: task.TenantID, UserID: task.UserID,
		FileID: source.FileID, StorageKey: source.ObjectKey, AssetType: assetType,
		Metadata: smartvideo.AssetMetadata{FileSize: source.FileSize, MIMEType: source.MIMEType},
	}
	return task, asset
}

func assertTempRootEmpty(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("temporary files were not cleaned: %s", strings.Join(names, ","))
	}
}

func tinyPNG() []byte {
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00,
	}
}
