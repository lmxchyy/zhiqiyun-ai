package httpserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

type pptV2AgentImageAssets struct {
	generation generation.Service
	files      *storagecenter.Service
	store      platformStore
	client     *http.Client
}

func (a pptV2AgentImageAssets) ResolveImage(ctx context.Context, scope pptapp.GenerationJobScope, jobID, slideID string, intent pptapp.SlideAssetIntent) (pptapp.ResolvedDeckAsset, error) {
	if a.files == nil {
		return pptapp.ResolvedDeckAsset{}, pptapp.NewAgentWorkflowError(pptapp.ImageStorageFailed, "图片存储暂时不可用，请重试。", true, errors.New("private image storage is unavailable"))
	}
	businessID := strings.TrimSpace(jobID) + ":" + strings.TrimSpace(intent.StableID)
	if existing, found, err := a.find(ctx, scope, businessID); err != nil {
		return pptapp.ResolvedDeckAsset{}, imageStorageWorkflowError(err)
	} else if found {
		resolved, materializeErr := a.materialize(ctx, scope, slideID, intent, existing)
		if materializeErr != nil {
			return pptapp.ResolvedDeckAsset{}, imageStorageWorkflowError(materializeErr)
		}
		return resolved, nil
	}
	clientRequestID := "ppt-v2:" + businessID
	generated, found, err := a.findGeneratedImage(scope, clientRequestID)
	if err != nil {
		return pptapp.ResolvedDeckAsset{}, err
	}
	if !found {
		created, createErr := a.generation.Create(ctx, generation.CreateRequest{
			Type: "TEXT_TO_IMAGE", UserID: scope.UserID, ClientRequestID: "ppt-v2:" + businessID,
			Prompt: intent.Prompt, Params: map[string]any{"width": 1024, "height": 1024, "n": 1},
		})
		if createErr != nil {
			return pptapp.ResolvedDeckAsset{}, createErr
		}
		task, ok := created.(generationTask)
		if !ok {
			return pptapp.ResolvedDeckAsset{}, errors.New("image generation task result is invalid")
		}
		generated, err = a.generatedImageForTask(scope, task)
		if err != nil {
			return pptapp.ResolvedDeckAsset{}, err
		}
	}
	data, mimeType, err := a.download(ctx, generated)
	if err != nil {
		return pptapp.ResolvedDeckAsset{}, pptapp.NewAgentWorkflowError(pptapp.ImageInvalidResult, "图片结果无效，请重试。", true, err)
	}
	fileName := intent.StableID + imageExtension(mimeType)
	stored, err := a.files.StoreObject(ctx, storagecenter.UploadInitInput{TenantID: scope.TenantID, UserID: scope.UserID, FileName: fileName, FileSize: int64(len(data)), MIMEType: mimeType, BusinessType: "ppt_v2_image_asset", BusinessID: businessID, Visibility: "PRIVATE"}, bytes.NewReader(data))
	if err != nil {
		if existing, found, findErr := a.find(ctx, scope, businessID); findErr == nil && found {
			resolved, materializeErr := a.materialize(ctx, scope, slideID, intent, existing)
			if materializeErr == nil {
				return resolved, nil
			}
		}
		return pptapp.ResolvedDeckAsset{}, imageStorageWorkflowError(err)
	}
	return a.materializeBytes(scope, slideID, intent, stored, data), nil
}

func imageStorageWorkflowError(err error) error {
	return pptapp.NewAgentWorkflowError(pptapp.ImageStorageFailed, "图片存储暂时不可用，请重试。", true, err)
}

func (a pptV2AgentImageAssets) findGeneratedImage(scope pptapp.GenerationJobScope, clientRequestID string) (generation.GeneratedImage, bool, error) {
	if a.store == nil {
		return generation.GeneratedImage{}, false, errors.New("image generation task store is unavailable")
	}
	tasks, err := a.store.ListGenerationTasks()
	if err != nil {
		return generation.GeneratedImage{}, false, err
	}
	for _, task := range tasks {
		if task.UserID != scope.UserID || task.ClientRequestID != clientRequestID {
			continue
		}
		image, imageErr := a.generatedImageForTask(scope, task)
		return image, imageErr == nil, imageErr
	}
	return generation.GeneratedImage{}, false, nil
}

func (a pptV2AgentImageAssets) generatedImageForTask(scope pptapp.GenerationJobScope, task generationTask) (generation.GeneratedImage, error) {
	if task.UserID != scope.UserID || !strings.EqualFold(task.Status, "SUCCEEDED") || len(task.ResultIDs) == 0 {
		return generation.GeneratedImage{}, errors.New("image generation task is not complete")
	}
	assets, err := a.store.ListAssets()
	if err != nil {
		return generation.GeneratedImage{}, err
	}
	resultID := task.ResultIDs[0]
	for _, item := range assets {
		if item.ID != resultID || item.TaskID != task.ID || item.UserID != scope.UserID || item.MediaType != "image" {
			continue
		}
		if strings.TrimSpace(item.URL) == "" {
			return generation.GeneratedImage{}, errors.New("image generation result has no locator")
		}
		return generation.GeneratedImage{
			URL: item.URL, ThumbnailURL: item.ThumbnailURL,
			ContentType: strings.TrimSpace(stringValue(item.Metadata["contentType"])),
			Source:      strings.TrimSpace(stringValue(item.Metadata["source"])),
			Width:       intValue(item.Metadata["width"]), Height: intValue(item.Metadata["height"]),
		}, nil
	}
	return generation.GeneratedImage{}, errors.New("image generation result asset is missing")
}

func (a pptV2AgentImageAssets) find(ctx context.Context, scope pptapp.GenerationJobScope, businessID string) (storagecenter.FileObject, bool, error) {
	for offset := 0; ; offset += 200 {
		items, total, err := a.files.ListFiles(ctx, storagecenter.FileFilter{TenantID: scope.TenantID, UserID: scope.UserID, BusinessType: "ppt_v2_image_asset", Status: storagecenter.StatusActive, Limit: 200, Offset: offset})
		if err != nil {
			return storagecenter.FileObject{}, false, err
		}
		for _, item := range items {
			if item.BusinessID == businessID {
				return item, true, nil
			}
		}
		if int64(offset+len(items)) >= total || len(items) == 0 {
			return storagecenter.FileObject{}, false, nil
		}
	}
}

func (a pptV2AgentImageAssets) materialize(ctx context.Context, scope pptapp.GenerationJobScope, slideID string, intent pptapp.SlideAssetIntent, file storagecenter.FileObject) (pptapp.ResolvedDeckAsset, error) {
	_, stream, err := a.files.OpenObject(ctx, storagecenter.AccessContext{TenantID: scope.TenantID, UserID: scope.UserID}, file.FileID)
	if err != nil {
		return pptapp.ResolvedDeckAsset{}, err
	}
	data, readErr := io.ReadAll(io.LimitReader(stream, 25<<20))
	closeErr := stream.Close()
	if readErr != nil {
		return pptapp.ResolvedDeckAsset{}, readErr
	}
	if closeErr != nil {
		return pptapp.ResolvedDeckAsset{}, closeErr
	}
	return a.materializeBytes(scope, slideID, intent, file, data), nil
}

func (a pptV2AgentImageAssets) materializeBytes(scope pptapp.GenerationJobScope, slideID string, intent pptapp.SlideAssetIntent, file storagecenter.FileObject, data []byte) pptapp.ResolvedDeckAsset {
	digest := sha256.Sum256(data)
	stable := hex.EncodeToString(digest[:])
	return pptapp.ResolvedDeckAsset{ID: "asset_" + stable[:16], IntentID: intent.StableID, SlideID: slideID, MIMEType: file.MIMEType, URI: "asset://ppt-v2/" + stable[:24], SHA256: stable, FileID: file.FileID, AltText: intent.AltText, TenantID: scope.TenantID, UserID: scope.UserID}
}

func (a pptV2AgentImageAssets) download(ctx context.Context, image generation.GeneratedImage) ([]byte, string, error) {
	locator := strings.TrimSpace(image.URL)
	if strings.HasPrefix(locator, "data:") {
		parts := strings.SplitN(locator, ",", 2)
		if len(parts) != 2 || !strings.Contains(parts[0], ";base64") {
			return nil, "", errors.New("invalid image data URI")
		}
		if base64.StdEncoding.DecodedLen(len(parts[1])) > 25<<20 {
			return nil, "", errors.New("image data URI exceeds size limit")
		}
		data, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, "", err
		}
		mime := strings.TrimPrefix(strings.Split(parts[0], ";")[0], "data:")
		if !allowedPPTImageMIME(mime) {
			return nil, "", errors.New("unsupported image MIME type")
		}
		if err := validatePPTImageBytes(data, mime); err != nil {
			return nil, "", err
		}
		return data, mime, nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, locator, nil)
	if err != nil {
		return nil, "", err
	}
	client := a.client
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, "", fmt.Errorf("image download returned %s", response.Status)
	}
	mime := strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0])
	if mime == "" {
		mime = strings.TrimSpace(image.ContentType)
	}
	if !allowedPPTImageMIME(mime) {
		return nil, "", errors.New("unsupported image MIME type")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 25<<20))
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 {
		return nil, "", errors.New("empty image response")
	}
	if err := validatePPTImageBytes(data, mime); err != nil {
		return nil, "", err
	}
	return data, mime, nil
}

func validatePPTImageBytes(data []byte, mime string) error {
	_, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return errors.New("image bytes are invalid")
	}
	if (mime == "image/png" && format != "png") || (mime == "image/jpeg" && format != "jpeg") {
		return errors.New("image bytes do not match MIME type")
	}
	return nil
}

func allowedPPTImageMIME(value string) bool { return value == "image/png" || value == "image/jpeg" }
func imageExtension(mime string) string {
	if mime == "image/jpeg" {
		return ".jpg"
	}
	return ".png"
}

var _ pptapp.AgentDeckAssetPort = pptV2AgentImageAssets{}
