package httpserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

const (
	generatedStorageFilesParam = "generated_storage_files"
	maxGeneratedArtifactBytes  = 64 << 20
	maxGeneratedVideoBytes     = 100 << 20
)

func (a api) persistGeneratedImages(ctx context.Context, taskID string, req generation.CreateRequest) (generation.CreateRequest, []storagecenter.FileObject, error) {
	req = applyGeneratedImageProviderMetadata(req)
	if a.fileService == nil || len(req.GeneratedImages) == 0 {
		return req, nil, nil
	}
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	tenantID := firstNonEmptyString(stringValue(req.Params["tenant_id"]), "tenant_default")
	available, err := a.fileService.StorageAvailable(ctx, tenantID)
	if err != nil {
		return req, nil, fmt.Errorf("resolve generated artifact storage: %w", err)
	}
	if !available {
		return req, nil, nil
	}

	stored := make([]storagecenter.FileObject, 0, len(req.GeneratedImages))
	records := make([]map[string]any, 0, len(req.GeneratedImages))
	for index, image := range req.GeneratedImages {
		raw, contentType, extension, err := readGeneratedArtifact(ctx, image.URL, image.ContentType)
		if err != nil {
			a.cleanupGeneratedFiles(stored)
			return req, nil, fmt.Errorf("download generated image %d: %w", index+1, err)
		}
		file, err := a.fileService.StoreObject(ctx, storagecenter.UploadInitInput{
			TenantID:     tenantID,
			UserID:       req.UserID,
			FileName:     fmt.Sprintf("%s-%02d.%s", taskID, index+1, extension),
			FileSize:     int64(len(raw)),
			MIMEType:     contentType,
			BusinessType: "generation_result",
			BusinessID:   taskID,
			Visibility:   "PRIVATE",
		}, bytes.NewReader(raw))
		if err != nil {
			a.cleanupGeneratedFiles(stored)
			return req, nil, fmt.Errorf("store generated image %d: %w", index+1, err)
		}
		// The asset grid must not fetch multi-megabyte originals as thumbnails.
		// Keep a compact 420px JPEG inline so the first four cards render with the
		// list response and do not incur four extra object-storage round trips.
		if thumbnailURL, width, height, ok := thumbnailAndDimensionsFromBytes(raw); ok && thumbnailURL != "" {
			req.GeneratedImages[index].ThumbnailURL = thumbnailURL
			if req.GeneratedImages[index].Width <= 0 {
				req.GeneratedImages[index].Width = width
			}
			if req.GeneratedImages[index].Height <= 0 {
				req.GeneratedImages[index].Height = height
			}
		} else if strings.TrimSpace(image.ThumbnailURL) == "" {
			req.GeneratedImages[index].ThumbnailURL = image.URL
		}
		stored = append(stored, file)
		record := map[string]any{
			"fileId":         file.FileID,
			"tenantId":       file.TenantID,
			"provider":       file.Provider,
			"bucket":         file.Bucket,
			"objectKey":      file.ObjectKey,
			"fileSize":       file.FileSize,
			"contentType":    file.MIMEType,
			"source":         image.Source,
			"providerTaskId": image.ProviderTaskID,
		}
		if sourceURL := compactPersistedSourceURL(image.URL); sourceURL != "" {
			record["sourceUrl"] = sourceURL
		}
		records = append(records, record)
	}
	req.Params[generatedStorageFilesParam] = records
	return req, stored, nil
}

func compactPersistedSourceURL(value string) string {
	text := strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(text), "data:") || len(text) > 4096 {
		return ""
	}
	return text
}

func applyGeneratedImageProviderMetadata(req generation.CreateRequest) generation.CreateRequest {
	if len(req.GeneratedImages) == 0 {
		return req
	}
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	image := req.GeneratedImages[0]
	if value := strings.TrimSpace(image.ProviderTaskID); value != "" {
		req.Params["provider_task_id"] = value
	}
	if value := strings.TrimSpace(image.RevisedPrompt); value != "" {
		req.Params["provider_revised_prompt"] = value
	}
	if len(image.ProviderMetadata) > 0 {
		req.Params["provider_metadata"] = cloneAnyMap(image.ProviderMetadata)
	}
	return req
}

func (a api) cleanupGeneratedFiles(files []storagecenter.FileObject) {
	if a.fileService == nil || len(files) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, file := range files {
		_ = a.fileService.PermanentDelete(ctx, storagecenter.AccessContext{
			TenantID: file.TenantID,
			UserID:   file.UserID,
			IsAdmin:  true,
		}, file.FileID)
	}
}

func (a api) signStoredAssetURLs(ctx context.Context, userID string, items []asset) []asset {
	if a.fileService == nil || len(items) == 0 {
		return items
	}
	result := make([]asset, len(items))
	copy(result, items)
	for index := range result {
		originalURL := result[index].URL
		originalThumbnailURL := result[index].ThumbnailURL
		fileID := firstNonEmptyString(stringValue(result[index].Metadata["fileId"]), stringValue(result[index].Metadata["storageFileId"]))
		if fileID == "" {
			continue
		}
		tenantID := firstNonEmptyString(result[index].TenantID, stringValue(result[index].Metadata["storageTenantId"]), "tenant_default")
		ticket, err := a.fileService.AccessURL(ctx, storagecenter.AccessContext{
			TenantID: tenantID,
			UserID:   userID,
		}, fileID, false)
		if err != nil {
			continue
		}
		result[index].URL = ticket.URL
		if originalThumbnailURL == "" || originalThumbnailURL == originalURL {
			result[index].ThumbnailURL = ticket.URL
		} else {
			result[index].ThumbnailURL = originalThumbnailURL
		}
	}
	return result
}

func generatedStorageRecord(params map[string]any, index int) (map[string]any, bool) {
	if params == nil || index < 0 {
		return nil, false
	}
	switch records := params[generatedStorageFilesParam].(type) {
	case []map[string]any:
		if index < len(records) && records[index] != nil {
			return records[index], true
		}
	case []any:
		if index < len(records) {
			record, ok := mapValue(records[index])
			return record, ok
		}
	}
	return nil, false
}

func readGeneratedArtifact(ctx context.Context, rawURL string, preferredContentType string) ([]byte, string, string, error) {
	value := strings.TrimSpace(rawURL)
	if strings.HasPrefix(strings.ToLower(value), "data:") {
		return readGeneratedDataURL(value, preferredContentType)
	}
	remoteURL, err := validateRemoteDownloadURL(value)
	if err != nil {
		return nil, "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, remoteURL.String(), nil)
	if err != nil {
		return nil, "", "", err
	}
	res, err := remoteDownloadHTTPClient().Do(req)
	if err != nil {
		return nil, "", "", err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, "", "", fmt.Errorf("upstream returned %d", res.StatusCode)
	}
	if res.ContentLength > maxGeneratedArtifactBytes {
		return nil, "", "", fmt.Errorf("generated image exceeds %d bytes", maxGeneratedArtifactBytes)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxGeneratedArtifactBytes+1))
	if err != nil {
		return nil, "", "", err
	}
	if len(raw) == 0 || len(raw) > maxGeneratedArtifactBytes {
		return nil, "", "", fmt.Errorf("generated image has invalid size %d", len(raw))
	}
	contentType, extension, err := generatedImageType(firstNonEmptyString(res.Header.Get("Content-Type"), preferredContentType), raw)
	if err != nil {
		return nil, "", "", err
	}
	return raw, contentType, extension, nil
}

func readGeneratedVideoArtifact(ctx context.Context, rawURL string) ([]byte, string, string, error) {
	value := strings.TrimSpace(rawURL)
	remoteURL, err := validateRemoteDownloadURL(value)
	if err != nil {
		return nil, "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, remoteURL.String(), nil)
	if err != nil {
		return nil, "", "", err
	}
	res, err := remoteDownloadHTTPClient().Do(req)
	if err != nil {
		return nil, "", "", err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, "", "", fmt.Errorf("upstream returned %d", res.StatusCode)
	}
	if res.ContentLength > maxGeneratedVideoBytes {
		return nil, "", "", fmt.Errorf("generated video exceeds %d bytes", maxGeneratedVideoBytes)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxGeneratedVideoBytes+1))
	if err != nil {
		return nil, "", "", err
	}
	if len(raw) == 0 || len(raw) > maxGeneratedVideoBytes {
		return nil, "", "", fmt.Errorf("generated video has invalid size %d", len(raw))
	}
	contentType := strings.ToLower(strings.TrimSpace(res.Header.Get("Content-Type")))
	if parsed, _, parseErr := mime.ParseMediaType(contentType); parseErr == nil {
		contentType = strings.ToLower(parsed)
	}
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = strings.ToLower(http.DetectContentType(raw))
	}
	pathHint := strings.ToLower(remoteURL.Path)
	extensions := map[string]string{
		"video/mp4":       "mp4",
		"video/x-m4v":     "mp4",
		"video/quicktime": "mp4",
		"video/webm":      "webm",
	}
	extension, ok := extensions[contentType]
	if !ok {
		switch {
		case strings.HasSuffix(pathHint, ".m4v"), strings.HasSuffix(pathHint, ".mp4"), strings.HasSuffix(pathHint, ".mov"):
			contentType = "video/mp4"
			extension = "mp4"
		case strings.HasSuffix(pathHint, ".webm"):
			contentType = "video/webm"
			extension = "webm"
		default:
			return nil, "", "", fmt.Errorf("unsupported generated video content type %q", contentType)
		}
	}
	return raw, contentType, extension, nil
}

func readGeneratedDataURL(value string, preferredContentType string) ([]byte, string, string, error) {
	comma := strings.IndexByte(value, ',')
	if comma <= len("data:") || !strings.Contains(strings.ToLower(value[:comma]), ";base64") {
		return nil, "", "", errors.New("generated data URL must be base64 encoded")
	}
	encoded := value[comma+1:]
	if len(encoded) > (maxGeneratedArtifactBytes*4/3)+8 {
		return nil, "", "", fmt.Errorf("generated image exceeds %d bytes", maxGeneratedArtifactBytes)
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, "", "", fmt.Errorf("decode generated data URL: %w", err)
	}
	if len(raw) == 0 || len(raw) > maxGeneratedArtifactBytes {
		return nil, "", "", fmt.Errorf("generated image has invalid size %d", len(raw))
	}
	declared := strings.TrimSpace(strings.Split(strings.TrimPrefix(value[:comma], "data:"), ";")[0])
	contentType, extension, err := generatedImageType(firstNonEmptyString(declared, preferredContentType), raw)
	if err != nil {
		return nil, "", "", err
	}
	return raw, contentType, extension, nil
}

func generatedImageType(value string, raw []byte) (string, string, error) {
	contentType := strings.ToLower(strings.TrimSpace(value))
	if parsed, _, err := mime.ParseMediaType(contentType); err == nil {
		contentType = strings.ToLower(parsed)
	}
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = strings.ToLower(http.DetectContentType(raw))
	}
	extensions := map[string]string{
		"image/jpeg": "jpg",
		"image/png":  "png",
		"image/webp": "webp",
		"image/avif": "avif",
		"image/gif":  "gif",
	}
	extension, ok := extensions[contentType]
	if !ok {
		return "", "", fmt.Errorf("unsupported generated image content type %q", contentType)
	}
	return contentType, extension, nil
}
