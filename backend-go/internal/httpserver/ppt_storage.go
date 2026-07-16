package httpserver

import (
	"context"
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
	return task
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
