package httpserver

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
)

type workspaceAssetListStore interface {
	ListAssetsForWorkspaceList(userID string, limit int) ([]asset, error)
}

type assetByIDStore interface {
	GetAssetByID(id string) (asset, bool, error)
}

func (a api) thumbnailSigner() assetThumbnailSigner {
	return newAssetThumbnailSigner(a.cfg, a.thumbnailNow)
}

func (a api) attachWorkspaceListThumbnailTickets(r *http.Request, userID string, items []asset) []asset {
	if len(items) == 0 {
		return items
	}
	signer := a.thumbnailSigner()
	base := publicAPIBaseURL(r)
	for index := range items {
		assetID := strings.TrimSpace(items[index].ID)
		if assetID == "" {
			items[index].ThumbnailURL = ""
			continue
		}
		exp, sig := signer.issue(assetID, userID)
		items[index].ThumbnailURL = assetThumbnailTicketURL(base, assetID, exp, sig)
	}
	return items
}

func (a api) assetThumbnail(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	exp, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("exp")), 10, 64)
	sig := strings.TrimSpace(r.URL.Query().Get("sig"))
	if id == "" || exp <= 0 || sig == "" {
		writeError(w, http.StatusNotFound, errAssetNotFound)
		return
	}
	item, found, err := a.lookupAssetByID(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if !found || strings.TrimSpace(item.ID) != id {
		writeError(w, http.StatusNotFound, errAssetNotFound)
		return
	}
	if !a.thumbnailSigner().verify(id, item.UserID, exp, sig) {
		writeError(w, http.StatusNotFound, errAssetNotFound)
		return
	}
	contentType, raw, ok := decodeStoredAssetThumbnail(item.ThumbnailURL)
	if !ok {
		writeError(w, http.StatusNotFound, errAssetNotFound)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
	w.Header().Set("Cache-Control", assetThumbnailCacheControl)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

func (a api) lookupAssetByID(id string) (asset, bool, error) {
	if lookup, ok := a.store.(assetByIDStore); ok {
		return lookup.GetAssetByID(id)
	}
	assets, err := a.store.ListAssets()
	if err != nil {
		return asset{}, false, err
	}
	for _, item := range assets {
		if item.ID == id {
			return item, true, nil
		}
	}
	return asset{}, false, nil
}

func decodeStoredAssetThumbnail(thumbnailURL string) (string, []byte, bool) {
	text := strings.TrimSpace(thumbnailURL)
	lower := strings.ToLower(text)
	if !strings.HasPrefix(lower, "data:image/") {
		return "", nil, false
	}
	comma := strings.IndexByte(text, ',')
	if comma < 0 {
		return "", nil, false
	}
	header := strings.ToLower(text[:comma])
	if !strings.Contains(header, ";base64") || strings.Contains(header, "svg") {
		return "", nil, false
	}
	raw, err := base64.StdEncoding.DecodeString(text[comma+1:])
	if err != nil || len(raw) == 0 {
		return "", nil, false
	}
	contentType := "image/jpeg"
	switch {
	case strings.HasPrefix(header, "data:image/png"):
		contentType = "image/png"
	case strings.HasPrefix(header, "data:image/webp"):
		contentType = "image/webp"
	case strings.HasPrefix(header, "data:image/gif"):
		contentType = "image/gif"
	case strings.HasPrefix(header, "data:image/jpeg"), strings.HasPrefix(header, "data:image/jpg"):
		contentType = "image/jpeg"
	default:
		return "", nil, false
	}
	return contentType, raw, true
}
