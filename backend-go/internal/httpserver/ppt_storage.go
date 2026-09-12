package httpserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

const pptStorageReferenceScheme = "storage"

func pptStorageReference(file storagecenter.FileObject) string {
	tenantID, fileID := strings.TrimSpace(file.TenantID), strings.TrimSpace(file.FileID)
	if tenantID == "" || fileID == "" {
		return ""
	}
	return pptStorageReferenceScheme + "://" + url.PathEscape(tenantID) + "/" + url.PathEscape(fileID)
}

func parsePPTStorageReference(value string) (tenantID, fileID string, ok bool) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || !strings.EqualFold(parsed.Scheme, pptStorageReferenceScheme) {
		return "", "", false
	}
	tenantID, err = url.PathUnescape(parsed.Host)
	if err != nil {
		return "", "", false
	}
	fileID, err = url.PathUnescape(strings.TrimPrefix(parsed.EscapedPath(), "/"))
	if err != nil || strings.TrimSpace(tenantID) == "" || strings.TrimSpace(fileID) == "" || strings.Contains(fileID, "/") {
		return "", "", false
	}
	return strings.TrimSpace(tenantID), strings.TrimSpace(fileID), true
}

func (a api) materializePPTTaskVisualURLs(ctx context.Context, user adminUser, task pptapp.Task) pptapp.Task {
	if a.fileService == nil {
		return task
	}
	if signed, ok := a.resolvePPTStorageReference(ctx, user, task.PPTURL); ok {
		task.PPTURL = signed
	}
	// Task is passed by value, but its slices still share backing arrays with the
	// persisted model. Materializing signed URLs must only affect the response
	// copy; otherwise a later caller can inherit another user's signed URL.
	task.Slides = append([]pptapp.Slide(nil), task.Slides...)
	for slideIndex := range task.Slides {
		task.Slides[slideIndex].VisualHistory = append([]pptapp.VisualAsset(nil), task.Slides[slideIndex].VisualHistory...)
	}
	for slideIndex := range task.Slides {
		slide := &task.Slides[slideIndex]
		if signed, ok := a.resolvePPTStorageReference(ctx, user, slide.ImageURL); ok {
			slide.VisualStorageRef = slide.ImageURL
			slide.ImageURL = signed
		}
		for historyIndex := range slide.VisualHistory {
			asset := &slide.VisualHistory[historyIndex]
			if signed, ok := a.resolvePPTStorageReference(ctx, user, asset.URL); ok {
				asset.StorageRef = asset.URL
				asset.URL = signed
			}
		}
	}
	if task.StorageRef != "" {
		if signed, ok := a.resolvePPTStorageReference(ctx, user, task.StorageRef); ok {
			task.PPTURL = signed
		}
	} else if task.PPTURL != "" && strings.HasPrefix(task.PPTURL, pptStorageReferenceScheme+"://") {
		task.StorageRef = task.PPTURL
		if signed, ok := a.resolvePPTStorageReference(ctx, user, task.PPTURL); ok {
			task.PPTURL = signed
		}
	}
	return task
}

func (a api) persistPPTXArtifact(ctx context.Context, user adminUser, task pptapp.Task, payload []byte) (string, error) {
	if a.fileService == nil {
		return "", errors.New("private file storage is unavailable")
	}
	tenantID := strings.TrimSpace(user.TenantID)
	if tenantID == "" {
		tenantID = "personal:" + strings.TrimSpace(user.ID)
	}
	available, err := a.fileService.StorageAvailable(ctx, tenantID)
	if err != nil {
		return "", err
	}
	if !available {
		return "", errors.New("private file storage is not configured")
	}
	digest := sha256.Sum256(payload)
	fileName := fmt.Sprintf("%s-%s.pptx", strings.TrimSpace(task.TaskID), hex.EncodeToString(digest[:])[:16])
	file, err := a.fileService.StoreObjectIdempotent(ctx, storagecenter.UploadInitInput{
		TenantID: tenantID, UserID: user.ID, FileName: fileName, FileSize: int64(len(payload)),
		MIMEType:     "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		BusinessType: "pptx_export", BusinessID: task.TaskID, Visibility: "PRIVATE",
	}, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("store pptx artifact: %w", err)
	}
	ref := pptStorageReference(file)
	if ref == "" {
		return "", errors.New("stored pptx artifact has no storage reference")
	}
	if _, err := a.pptService.SetPPTURL(user.ID, task.TaskID, ref); err != nil {
		return "", fmt.Errorf("persist pptx artifact reference: %w", err)
	}
	return ref, nil
}

func (a api) resolvePPTStorageReference(ctx context.Context, user adminUser, value string) (string, bool) {
	tenantID, fileID, ok := parsePPTStorageReference(value)
	if !ok || a.fileService == nil {
		return "", false
	}
	role := strings.ToUpper(strings.TrimSpace(user.Role))
	ticket, err := a.fileService.AccessURL(ctx, storagecenter.AccessContext{
		TenantID: tenantID, UserID: user.ID,
		IsAdmin: role == "SUPER_ADMIN" || role == "PLATFORM_ADMIN" || role == "ADMIN",
	}, fileID, false)
	if err != nil || strings.TrimSpace(ticket.URL) == "" {
		return "", false
	}
	return strings.TrimSpace(ticket.URL), true
}
