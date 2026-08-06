package httpserver

import (
	"context"
	"io"
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
	if value == "" || value != strings.TrimSpace(value) {
		return "", "", false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != pptStorageReferenceScheme || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
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
	tenantID, fileID = strings.TrimSpace(tenantID), strings.TrimSpace(fileID)
	if pptStorageReference(storagecenter.FileObject{TenantID: tenantID, FileID: fileID}) != value {
		return "", "", false
	}
	return tenantID, fileID, true
}

func (a api) pptxStorageImageResolver(user adminUser, expectedTenantID string) pptxImageResolver {
	expectedTenantID = strings.TrimSpace(expectedTenantID)
	return func(ctx context.Context, referencedTenantID, fileID string) (string, []byte, bool) {
		if a.fileService == nil || expectedTenantID == "" || strings.TrimSpace(referencedTenantID) != expectedTenantID {
			return "", nil, false
		}
		role := strings.ToUpper(strings.TrimSpace(user.Role))
		file, stream, err := a.fileService.OpenObject(ctx, storagecenter.AccessContext{
			TenantID: expectedTenantID,
			UserID:   strings.TrimSpace(user.ID),
			IsAdmin:  role == "SUPER_ADMIN" || role == "PLATFORM_ADMIN" || role == "ADMIN",
		}, strings.TrimSpace(fileID))
		if err != nil || stream == nil {
			if stream != nil {
				_ = stream.Close()
			}
			return "", nil, false
		}
		defer stream.Close()
		if strings.TrimSpace(file.TenantID) != expectedTenantID || strings.TrimSpace(file.FileID) != strings.TrimSpace(fileID) || file.FileSize > 8<<20 {
			return "", nil, false
		}
		data, err := io.ReadAll(io.LimitReader(stream, (8<<20)+1))
		if err != nil || len(data) == 0 || len(data) > 8<<20 {
			return "", nil, false
		}
		return strings.TrimSpace(file.MIMEType), data, true
	}
}

func (a api) materializePPTTaskVisualURLs(ctx context.Context, user adminUser, task pptapp.Task) pptapp.Task {
	expectedTenantID := strings.TrimSpace(task.TenantID)
	task = projectPPTTaskForHTTP(task)
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
		if signed, ok := a.resolvePPTStorageReference(ctx, user, expectedTenantID, slide.ImageURL); ok {
			slide.VisualStorageRef = slide.ImageURL
			slide.ImageURL = signed
		}
		for historyIndex := range slide.VisualHistory {
			asset := &slide.VisualHistory[historyIndex]
			if signed, ok := a.resolvePPTStorageReference(ctx, user, expectedTenantID, asset.URL); ok {
				asset.StorageRef = asset.URL
				asset.URL = signed
			}
		}
	}
	return task
}

func (a api) resolvePPTStorageReference(ctx context.Context, user adminUser, expectedTenantID string, value string) (string, bool) {
	tenantID, fileID, ok := parsePPTStorageReference(value)
	if !ok || a.fileService == nil || tenantID != strings.TrimSpace(expectedTenantID) {
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
