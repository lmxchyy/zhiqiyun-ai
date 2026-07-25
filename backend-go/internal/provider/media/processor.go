package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"xianzhi-ai/backend-go/internal/app/smartvideo"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type ProcessorOptions struct {
	TempDir            string
	MaxFileBytes       int64
	MaxVideoDurationMS int64
	MaxVideoPixels     int64
	MaxImagePixels     int64
	ProxyMaxWidth      int
	ProxyVideoBitrate  string
	ProxyAudioBitrate  string
}

type Processor struct {
	files   *storagecenter.Service
	probe   smartvideo.MediaProbe
	options ProcessorOptions
}

func NewProcessor(files *storagecenter.Service, probe smartvideo.MediaProbe, options ProcessorOptions) *Processor {
	if options.MaxFileBytes <= 0 {
		options.MaxFileBytes = 2 << 30
	}
	if options.MaxVideoDurationMS <= 0 {
		options.MaxVideoDurationMS = int64((30 * 60 * 1000))
	}
	if options.MaxVideoPixels <= 0 {
		options.MaxVideoPixels = 3840 * 2160
	}
	if options.MaxImagePixels <= 0 {
		options.MaxImagePixels = 80_000_000
	}
	if options.ProxyMaxWidth <= 0 {
		options.ProxyMaxWidth = 960
	}
	if strings.TrimSpace(options.ProxyVideoBitrate) == "" {
		options.ProxyVideoBitrate = "1200k"
	}
	if strings.TrimSpace(options.ProxyAudioBitrate) == "" {
		options.ProxyAudioBitrate = "96k"
	}
	return &Processor{files: files, probe: probe, options: options}
}

func (p *Processor) Process(ctx context.Context, task smartvideo.AnalysisTask, asset smartvideo.ProjectAsset) (smartvideo.AnalysisResult, error) {
	if p.files == nil || p.probe == nil {
		return smartvideo.AnalysisResult{}, &smartvideo.MediaError{Code: smartvideo.MediaErrorProbeFailed, Message: "媒体分析服务不可用"}
	}
	if task.SourceFileID != asset.FileID || task.TenantID != asset.TenantID || task.UserID != asset.UserID {
		return smartvideo.AnalysisResult{}, &smartvideo.MediaError{Code: smartvideo.MediaErrorDownloadFailed, Message: "素材访问范围无效"}
	}
	if asset.Metadata.FileSize > p.options.MaxFileBytes {
		return smartvideo.AnalysisResult{}, &smartvideo.MediaError{Code: smartvideo.MediaErrorFileTooLarge, Message: "素材文件超过大小限制"}
	}
	tempDir, err := os.MkdirTemp(p.options.TempDir, "smartvideo-")
	if err != nil {
		return smartvideo.AnalysisResult{}, &smartvideo.MediaError{Code: smartvideo.MediaErrorDownloadFailed, Message: "无法创建受控临时目录", Cause: err}
	}
	defer func() { _ = os.Remove(tempDir) }()
	sourcePath, err := p.downloadSource(ctx, task, tempDir)
	if err != nil {
		return smartvideo.AnalysisResult{}, err
	}
	defer func() { _ = os.Remove(sourcePath) }()
	if err := validateDetectedMediaType(sourcePath, asset.AssetType); err != nil {
		return smartvideo.AnalysisResult{}, err
	}
	ffprobeVersion, ffmpegVersion, err := p.probe.GetToolVersion(ctx)
	if err != nil {
		return smartvideo.AnalysisResult{}, err
	}
	analyzerVersion := boundedVersion(ffprobeVersion + "; " + ffmpegVersion)
	result := smartvideo.AnalysisResult{AnalyzerVersion: analyzerVersion}
	local := smartvideo.LocalMedia{Path: sourcePath, AssetType: asset.AssetType}
	switch asset.AssetType {
	case smartvideo.AssetTypeVideo:
		metadata, filtered, err := p.probe.ProbeVideo(ctx, local)
		if err != nil {
			return smartvideo.AnalysisResult{}, err
		}
		if metadata.DurationMS > p.options.MaxVideoDurationMS {
			return smartvideo.AnalysisResult{}, &smartvideo.MediaError{Code: smartvideo.MediaErrorDurationExceeded, Message: "视频时长超过限制"}
		}
		if pixels(metadata.DisplayWidth, metadata.DisplayHeight) > p.options.MaxVideoPixels {
			return smartvideo.AnalysisResult{}, &smartvideo.MediaError{Code: smartvideo.MediaErrorPixelsExceeded, Message: "视频分辨率超过限制"}
		}
		result.Metadata = smartvideo.NormalizedMediaMetadata{Kind: "VIDEO", Video: &metadata}
		result.FilteredProbeResult = filtered
		thumbnailPath, err := createOutputPath(tempDir, "thumbnail-*.jpg")
		if err != nil {
			return smartvideo.AnalysisResult{}, preprocessError(err)
		}
		defer func() { _ = os.Remove(thumbnailPath) }()
		if err := p.probe.GenerateThumbnail(ctx, local, thumbnailPath, smartvideo.ThumbnailOptions{
			MaxWidth: p.options.ProxyMaxWidth, MaxHeight: p.options.ProxyMaxWidth, Quality: 4,
		}); err != nil {
			return smartvideo.AnalysisResult{}, err
		}
		thumbnail, err := p.storeDerived(ctx, task, asset, thumbnailPath, "smart_video_thumbnail", "image/jpeg")
		if err != nil {
			return smartvideo.AnalysisResult{}, err
		}
		result.ThumbnailFileID = thumbnail.FileID

		proxyPath, err := createOutputPath(tempDir, "proxy-*.mp4")
		if err != nil {
			return smartvideo.AnalysisResult{}, preprocessError(err)
		}
		defer func() { _ = os.Remove(proxyPath) }()
		if err := p.probe.GenerateProxy(ctx, local, proxyPath, smartvideo.ProxyOptions{
			MaxWidth: p.options.ProxyMaxWidth, VideoBitrate: p.options.ProxyVideoBitrate,
			AudioBitrate: p.options.ProxyAudioBitrate,
		}); err != nil {
			return smartvideo.AnalysisResult{}, err
		}
		proxy, err := p.storeDerived(ctx, task, asset, proxyPath, "smart_video_proxy", "video/mp4")
		if err != nil {
			return smartvideo.AnalysisResult{}, err
		}
		result.ProxyFileID = proxy.FileID
	case smartvideo.AssetTypeImage:
		metadata, filtered, err := p.probe.ProbeImage(ctx, local)
		if err != nil {
			return smartvideo.AnalysisResult{}, err
		}
		if pixels(metadata.Width, metadata.Height) > p.options.MaxImagePixels {
			return smartvideo.AnalysisResult{}, &smartvideo.MediaError{Code: smartvideo.MediaErrorPixelsExceeded, Message: "图片像素总量超过限制"}
		}
		result.Metadata = smartvideo.NormalizedMediaMetadata{Kind: "IMAGE", Image: &metadata}
		result.FilteredProbeResult = filtered
		thumbnailPath, err := createOutputPath(tempDir, "thumbnail-*.jpg")
		if err != nil {
			return smartvideo.AnalysisResult{}, preprocessError(err)
		}
		defer func() { _ = os.Remove(thumbnailPath) }()
		if err := p.probe.GenerateThumbnail(ctx, local, thumbnailPath, smartvideo.ThumbnailOptions{
			MaxWidth: p.options.ProxyMaxWidth, MaxHeight: p.options.ProxyMaxWidth, Quality: 4,
		}); err != nil {
			return smartvideo.AnalysisResult{}, err
		}
		thumbnail, err := p.storeDerived(ctx, task, asset, thumbnailPath, "smart_video_thumbnail", "image/jpeg")
		if err != nil {
			return smartvideo.AnalysisResult{}, err
		}
		result.ThumbnailFileID = thumbnail.FileID
	default:
		return smartvideo.AnalysisResult{}, &smartvideo.MediaError{Code: smartvideo.MediaErrorUnsupported, Message: "不支持的素材类型"}
	}
	return result, nil
}

func (p *Processor) downloadSource(ctx context.Context, task smartvideo.AnalysisTask, tempDir string) (string, error) {
	access := storagecenter.AccessContext{TenantID: task.TenantID, UserID: task.UserID}
	file, source, err := p.files.OpenObject(ctx, access, task.SourceFileID)
	if err != nil {
		return "", &smartvideo.MediaError{Code: smartvideo.MediaErrorDownloadFailed, Message: "读取素材失败", Cause: err}
	}
	defer source.Close()
	if file.FileSize > p.options.MaxFileBytes {
		return "", &smartvideo.MediaError{Code: smartvideo.MediaErrorFileTooLarge, Message: "素材文件超过大小限制"}
	}
	target, err := os.CreateTemp(tempDir, "source-*")
	if err != nil {
		return "", &smartvideo.MediaError{Code: smartvideo.MediaErrorDownloadFailed, Message: "创建临时文件失败", Cause: err}
	}
	targetPath := target.Name()
	copied, copyErr := io.Copy(target, io.LimitReader(source, p.options.MaxFileBytes+1))
	closeErr := target.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(targetPath)
		return "", &smartvideo.MediaError{Code: smartvideo.MediaErrorDownloadFailed, Message: "下载素材失败", Cause: errors.Join(copyErr, closeErr)}
	}
	if copied > p.options.MaxFileBytes {
		_ = os.Remove(targetPath)
		return "", &smartvideo.MediaError{Code: smartvideo.MediaErrorFileTooLarge, Message: "素材文件超过大小限制"}
	}
	if file.FileSize > 0 && copied != file.FileSize {
		_ = os.Remove(targetPath)
		return "", &smartvideo.MediaError{Code: smartvideo.MediaErrorDownloadFailed, Message: "素材下载不完整"}
	}
	return targetPath, nil
}

func (p *Processor) storeDerived(ctx context.Context, task smartvideo.AnalysisTask, asset smartvideo.ProjectAsset, path, businessType, mimeType string) (storagecenter.FileObject, error) {
	info, err := os.Stat(path)
	if err != nil || info.Size() <= 0 {
		return storagecenter.FileObject{}, &smartvideo.MediaError{Code: smartvideo.MediaErrorPreprocessFailed, Message: "派生文件无效", Cause: err}
	}
	source, err := os.Open(path)
	if err != nil {
		return storagecenter.FileObject{}, &smartvideo.MediaError{Code: smartvideo.MediaErrorStorageFailed, Message: "无法读取派生文件", Cause: err}
	}
	defer source.Close()
	file, err := p.files.StoreObject(ctx, storagecenter.UploadInitInput{
		TenantID: task.TenantID, UserID: task.UserID, FileName: filepath.Base(path), FileSize: info.Size(),
		MIMEType: mimeType, BusinessType: businessType, BusinessID: asset.ID, Visibility: "PRIVATE",
	}, source)
	if err != nil {
		return storagecenter.FileObject{}, &smartvideo.MediaError{Code: smartvideo.MediaErrorStorageFailed, Message: "保存派生文件失败", Cause: err}
	}
	return file, nil
}

func createOutputPath(tempDir, pattern string) (string, error) {
	file, err := os.CreateTemp(tempDir, pattern)
	if err != nil {
		return "", err
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	return path, nil
}

func validateDetectedMediaType(path, assetType string) error {
	file, err := os.Open(path)
	if err != nil {
		return &smartvideo.MediaError{Code: smartvideo.MediaErrorUnsupported, Message: "无法读取素材文件头", Cause: err}
	}
	defer file.Close()
	header := make([]byte, 512)
	count, err := io.ReadFull(file, header)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return &smartvideo.MediaError{Code: smartvideo.MediaErrorUnsupported, Message: "无法读取素材文件头", Cause: err}
	}
	detected := http.DetectContentType(header[:count])
	switch assetType {
	case smartvideo.AssetTypeImage:
		if !strings.HasPrefix(detected, "image/") {
			return &smartvideo.MediaError{Code: smartvideo.MediaErrorUnsupported, Message: "文件内容不是支持的图片"}
		}
	case smartvideo.AssetTypeVideo:
		if !strings.HasPrefix(detected, "video/") && detected != "application/octet-stream" {
			return &smartvideo.MediaError{Code: smartvideo.MediaErrorUnsupported, Message: "文件内容不是支持的视频"}
		}
	default:
		return &smartvideo.MediaError{Code: smartvideo.MediaErrorUnsupported, Message: "不支持的素材类型"}
	}
	return nil
}

func pixels(width, height int) int64 {
	if width <= 0 || height <= 0 {
		return 0
	}
	return int64(width) * int64(height)
}

func boundedVersion(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 512 {
		return value[:512]
	}
	return value
}

func preprocessError(err error) error {
	return &smartvideo.MediaError{Code: smartvideo.MediaErrorPreprocessFailed, Message: "创建派生文件失败", Cause: err}
}

func sanitizeInternalError(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%T", err)
}
