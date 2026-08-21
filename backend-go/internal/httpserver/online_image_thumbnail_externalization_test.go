package httpserver

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAssetWorkspaceListSelectOmitsThumbnailURLColumn(t *testing.T) {
	sql := strings.ToLower(assetWorkspaceListSelect)
	if !strings.Contains(sql, "'' as thumbnail_url") {
		t.Fatalf("workspace list select must keep scan shape with empty thumbnail alias: %s", assetWorkspaceListSelect)
	}
	if strings.Contains(sql, "coalesce(thumbnail_url") {
		t.Fatal("workspace list select still reads thumbnail_url TEXT")
	}
	if strings.Contains(strings.ToLower(assetSummarySelect), "'' as thumbnail_url") {
		t.Fatal("detail/summary select must keep reading thumbnail_url")
	}
	if !strings.Contains(strings.ToLower(assetSummarySelect), "coalesce(thumbnail_url") {
		t.Fatal("detail/summary select lost thumbnail_url")
	}
}

func TestUserOnlineImageWorkspaceListUsesThumbnailTickets(t *testing.T) {
	handler, token, provider, stored := newWorkspaceListPerfFixture(t, 3, 0)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/online-image?taskLimit=40&assetLimit=40", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Host = "ai.zs-kjhn.cn"
	response := httptest.NewRecorder()
	handler.userOnlineImage(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if provider.count.Load() != 0 {
		t.Fatalf("workspace list signed originals: %d", provider.count.Load())
	}
	if bytes.Contains(response.Body.Bytes(), []byte("data:image/")) {
		t.Fatal("workspace list still returned data thumbnails")
	}
	if bytes.Contains(response.Body.Bytes(), []byte("storage://")) {
		t.Fatal("workspace list leaked storage://")
	}
	if bytes.Contains(response.Body.Bytes(), []byte("storageObjectKey")) {
		t.Fatal("workspace list leaked storageObjectKey")
	}
	var payload struct {
		Assets []asset `json:"assets"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Assets) != 3 {
		t.Fatalf("assets=%d", len(payload.Assets))
	}
	for _, item := range payload.Assets {
		assertWorkspaceListTicketThumbnailURL(t, item)
		if !strings.HasPrefix(item.ThumbnailURL, "https://ai.zs-kjhn.cn/api/v1/assets/"+item.ID+"/thumbnail") {
			t.Fatalf("thumbnailUrl is not an absolute ticket URL: %s", item.ThumbnailURL)
		}
	}
	_ = stored
}

func TestAssetThumbnailEndpointReturnsJPEGBytes(t *testing.T) {
	handler, token, _, stored := newWorkspaceListPerfFixture(t, 1, 0)
	thumbURL := workspaceListThumbnailURL(t, handler, token, stored[0].ID)
	status, header, body := fetchAssetThumbnail(t, handler, stored[0].ID, thumbURL)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	if got := header.Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("Content-Type=%q", got)
	}
	if header.Get("Content-Length") != strconv.Itoa(len(body)) {
		t.Fatalf("Content-Length=%s want %d", header.Get("Content-Length"), len(body))
	}
	if !strings.Contains(header.Get("Cache-Control"), "private") {
		t.Fatalf("Cache-Control=%q must stay private", header.Get("Cache-Control"))
	}
	if strings.Contains(header.Get("Cache-Control"), "public") {
		t.Fatalf("Cache-Control leaked public cache: %q", header.Get("Cache-Control"))
	}
	if len(body) < 16 || body[0] != 0xff || body[1] != 0xd8 {
		t.Fatalf("body is not JPEG bytes: len=%d prefix=%x", len(body), body[:min(8, len(body))])
	}
	if bytes.Contains(body, []byte("data:image")) || bytes.HasPrefix(bytes.TrimSpace(body), []byte("{")) {
		t.Fatal("thumbnail endpoint returned encoded JSON/data URL instead of JPEG")
	}
	if bytes.Equal(body, []byte("png0")) {
		t.Fatal("thumbnail endpoint returned the original object bytes")
	}
}

func TestAssetThumbnailTicketExpiryIsRejected(t *testing.T) {
	handler, token, _, stored := newWorkspaceListPerfFixture(t, 1, 0)
	handler.thumbnailNow = func() time.Time { return time.Now().UTC().Add(-20 * time.Minute) }
	thumbURL := workspaceListThumbnailURL(t, handler, token, stored[0].ID)
	handler.thumbnailNow = time.Now
	status, _, body := fetchAssetThumbnail(t, handler, stored[0].ID, thumbURL)
	if status != http.StatusNotFound {
		t.Fatalf("expired ticket status=%d body=%s want 404", status, body)
	}
}

func TestAssetThumbnailTicketTamperingIsRejected(t *testing.T) {
	handler, token, _, stored := newWorkspaceListPerfFixture(t, 2, 0)
	thumbURL := workspaceListThumbnailURL(t, handler, token, stored[0].ID)
	parsed, err := url.Parse(thumbURL)
	if err != nil {
		t.Fatal(err)
	}
	exp := parsed.Query().Get("exp")
	sig := parsed.Query().Get("sig")

	cases := []struct {
		name    string
		assetID string
		query   url.Values
	}{
		{name: "assetId", assetID: stored[1].ID, query: parsed.Query()},
		{name: "exp", assetID: stored[0].ID, query: url.Values{"exp": {strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)}, "sig": {sig}}},
		{name: "sig", assetID: stored[0].ID, query: url.Values{"exp": {exp}, "sig": {sig + "aa"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := "/api/v1/assets/" + tc.assetID + "/thumbnail?" + tc.query.Encode()
			status, _, body := fetchAssetThumbnail(t, handler, tc.assetID, raw)
			if status != http.StatusNotFound {
				t.Fatalf("tampered %s status=%d body=%s want 404", tc.name, status, body)
			}
		})
	}
}

func TestAssetThumbnailTicketOwnerIsolation(t *testing.T) {
	handler, token, _, stored := newWorkspaceListPerfFixture(t, 1, 0)
	ownerThumb := workspaceListThumbnailURL(t, handler, token, stored[0].ID)

	store := handler.store.(*jsonStore)
	other, err := store.CreateAdminCustomer(adminCustomerMutation{Name: "Other Thumb User", Email: "other-thumb@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	thumb, _ := testJPEGDataURL(t, 8)
	otherAsset := asset{ID: "asset_other_thumb", UserID: other.ID, MediaType: "IMAGE", ThumbnailURL: thumb}
	if err := store.updateAdmin(func(data *adminPlatformData) error {
		data.Assets = append(data.Assets, otherAsset)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	status, _, body := fetchAssetThumbnail(t, handler, otherAsset.ID, strings.Replace(ownerThumb, stored[0].ID, otherAsset.ID, 1))
	if status != http.StatusNotFound {
		t.Fatalf("cross-owner ticket status=%d body=%s want 404", status, body)
	}
}

func TestAssetThumbnailMalformedAndMissingAreRejected(t *testing.T) {
	handler, token, _, stored := newWorkspaceListPerfFixture(t, 2, 0)
	store := handler.store.(*jsonStore)
	if err := store.updateAdmin(func(data *adminPlatformData) error {
		for index := range data.Assets {
			if data.Assets[index].ID == stored[0].ID {
				data.Assets[index].ThumbnailURL = "data:image/jpeg;base64,$$$$"
			}
			if data.Assets[index].ID == stored[1].ID {
				data.Assets[index].ThumbnailURL = ""
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	malformed := workspaceListThumbnailURL(t, handler, token, stored[0].ID)
	missing := workspaceListThumbnailURL(t, handler, token, stored[1].ID)
	malformedStatus, _, _ := fetchAssetThumbnail(t, handler, stored[0].ID, malformed)
	missingStatus, _, _ := fetchAssetThumbnail(t, handler, stored[1].ID, missing)
	if malformedStatus != http.StatusNotFound {
		t.Fatalf("malformed thumbnail status=%d want 404", malformedStatus)
	}
	if missingStatus != http.StatusNotFound {
		t.Fatalf("missing thumbnail status=%d want 404", missingStatus)
	}
}

func TestUserOnlineImageThumbnailTicketsStayUnderPayloadBudget(t *testing.T) {
	const assetCount = 40
	thumb, raw := testJPEGDataURLAtLeast(t, 32*1024)
	if len(thumb) > 50*1024 {
		t.Logf("fixture data URL larger than 50KB: %d jpeg=%d", len(thumb), len(raw))
	}
	handler, token, provider, stored := newWorkspaceListPerfFixture(t, assetCount, 0)
	store := handler.store.(*jsonStore)
	if err := store.updateAdmin(func(data *adminPlatformData) error {
		for index := range data.Assets {
			if data.Assets[index].UserID != stored[0].UserID {
				continue
			}
			data.Assets[index].ThumbnailURL = thumb
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	oldEstimate := assetCount * len(thumb)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/online-image?taskLimit=40&assetLimit=40", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Host = "ai.zs-kjhn.cn"
	response := httptest.NewRecorder()
	handler.userOnlineImage(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if provider.count.Load() != 0 {
		t.Fatalf("workspace list signed originals: %d", provider.count.Load())
	}
	got := response.Body.Len()
	if got >= 100*1024 {
		t.Fatalf("bootstrap JSON still too large: %d bytes (old estimate %d)", got, oldEstimate)
	}
	if bytes.Contains(response.Body.Bytes(), []byte("data:image/")) {
		t.Fatal("bootstrap still contains data:image thumbnails")
	}
	var payload struct {
		Assets []asset `json:"assets"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Assets) != assetCount {
		t.Fatalf("assets=%d want %d", len(payload.Assets), assetCount)
	}
	firstPaintJPEG := 0
	var sampleJPEG int
	for i := 0; i < 8; i++ {
		status, _, body := fetchAssetThumbnail(t, handler, payload.Assets[i].ID, payload.Assets[i].ThumbnailURL)
		if status != http.StatusOK {
			t.Fatalf("cover %d status=%d", i, status)
		}
		firstPaintJPEG += len(body)
		if i == 0 {
			sampleJPEG = len(body)
		}
	}
	t.Logf("P1-A payload: old-inline-estimate=%d bytes new-json=%d bytes jpeg-each=%d first-paint-8-covers=%d json+8covers=%d", oldEstimate, got, sampleJPEG, firstPaintJPEG, got+firstPaintJPEG)
}

func TestAssetThumbnailDoesNotRequireSessionCookie(t *testing.T) {
	handler, token, _, stored := newWorkspaceListPerfFixture(t, 1, 0)
	thumbURL := workspaceListThumbnailURL(t, handler, token, stored[0].ID)
	req := httptest.NewRequest(http.MethodGet, thumbURL, nil)
	req.SetPathValue("id", stored[0].ID)
	response := httptest.NewRecorder()
	handler.assetThumbnail(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("img/src ticket must work without Authorization: status=%d body=%s", response.Code, response.Body.String())
	}
}

func assertWorkspaceListTicketThumbnailURL(t *testing.T, item asset) {
	t.Helper()
	raw := strings.TrimSpace(item.ThumbnailURL)
	if raw == "" {
		t.Fatalf("asset %s dropped thumbnailUrl", item.ID)
	}
	if strings.HasPrefix(raw, "data:image/") {
		t.Fatalf("asset %s still has data thumbnail: %s", item.ID, raw)
	}
	if strings.Contains(raw, "storage://") || strings.Contains(raw, "storageObjectKey") {
		t.Fatalf("asset %s leaked storage ref: %s", item.ID, raw)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("asset %s thumbnailUrl parse: %v", item.ID, err)
	}
	if !strings.Contains(parsed.Path, "/api/v1/assets/"+item.ID+"/thumbnail") {
		t.Fatalf("asset %s thumbnailUrl is not the ticket endpoint: %s", item.ID, raw)
	}
	query := parsed.Query()
	if query.Get("exp") == "" || query.Get("sig") == "" {
		t.Fatalf("asset %s thumbnailUrl missing ticket query: %s", item.ID, raw)
	}
	if query.Get("uid") != "" {
		t.Fatalf("asset %s thumbnailUrl exposed user id: %s", item.ID, raw)
	}
}

func workspaceListThumbnailURL(t *testing.T, handler api, token, assetID string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/online-image?taskLimit=40&assetLimit=40", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.userOnlineImage(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Assets []asset `json:"assets"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	for _, item := range payload.Assets {
		if item.ID == assetID {
			assertWorkspaceListTicketThumbnailURL(t, item)
			return item.ThumbnailURL
		}
	}
	t.Fatalf("asset %s missing from workspace list", assetID)
	return ""
}

func fetchAssetThumbnail(t *testing.T, handler api, assetID, rawURL string) (int, http.Header, []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, rawURL, nil)
	req.SetPathValue("id", assetID)
	response := httptest.NewRecorder()
	handler.assetThumbnail(response, req)
	return response.Code, response.Header(), append([]byte(nil), response.Body.Bytes()...)
}

func testJPEGDataURL(t *testing.T, size int) (string, []byte) {
	t.Helper()
	if size < 1 {
		size = 8
	}
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 3), G: uint8(y * 5), B: uint8(x + y), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(raw), raw
}

func testJPEGDataURLAtLeast(t *testing.T, minBytes int) (string, []byte) {
	t.Helper()
	for size := 64; size <= 900; size += 32 {
		dataURL, raw := testJPEGDataURL(t, size)
		if len(dataURL) >= minBytes {
			return dataURL, raw
		}
	}
	t.Fatalf("could not build jpeg data URL of at least %d bytes", minBytes)
	return "", nil
}

func TestJSONStoreWorkspaceListProjectionClearsThumbnailURL(t *testing.T) {
	handler, _, _, stored := newWorkspaceListPerfFixture(t, 1, 0)
	store := handler.store.(*jsonStore)
	items, err := store.ListAssetsForWorkspaceList(stored[0].UserID, 40)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 || items[0].ThumbnailURL != "" {
		t.Fatalf("workspace list projection still loaded thumbnail_url: %+v", items)
	}
	full, found, err := store.GetAssetByID(stored[0].ID)
	if err != nil || !found || !strings.HasPrefix(full.ThumbnailURL, "data:image/jpeg") {
		t.Fatalf("thumbnail endpoint source lost data thumbnail: found=%v err=%v item=%+v", found, err, full)
	}
}
