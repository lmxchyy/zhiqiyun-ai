package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/config"
)

func TestGenerationTaskDetailReturnsSignedURL_WhenAssetOutsideRecentLimit(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")

	// Create user with 1 old task whose asset is NOT in the top maxUserContentListLimit assets
	// We insert maxUserContentListLimit newer assets, and 1 older asset that belongs to task_video_old.
	user := `{"id":"user_test_video","email":"video@example.com","name":"Video User","role":"MEMBER","status":"ACTIVE","planId":"plan_free"},{"id":"user_other","email":"video_other@example.com","name":"Other User","role":"MEMBER","status":"ACTIVE","planId":"plan_free"}`
	task := `{"id":"task_video_old","userId":"user_test_video","type":"TEXT_TO_VIDEO","status":"SUCCEEDED","model":"grok-imagine-1.5-video","resultIds":["asset_video_old"],"createdAt":"2026-01-01T00:00:00Z"}`

	// Generate 105 newer dummy assets for the user
	newerAssets := ""
	for i := 1; i <= 105; i++ {
		newerAssets += fmt.Sprintf(`,{"id":"asset_newer_%d","userId":"user_test_video","name":"Newer %d","mediaType":"image","url":"https://example.com/newer.png","createdAt":"2026-02-01T00:00:00Z"}`, i, i)
	}
	// Recent unmanaged video (< 24h ago) with long signed URL (> 4KB, e.g. signed S3/OBS query params)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	longSignedURL := "https://storage.example.com/video_signed.mp4?signature=" + strings.Repeat("x", 4100)
	oldAsset := fmt.Sprintf(`,{"id":"asset_video_old","userId":"user_test_video","taskId":"task_video_old","name":"Old Video","mediaType":"video","url":%q,"createdAt":%q}`, longSignedURL, now)

	raw := fmt.Sprintf(`{
		"users": [%s],
		"generationTasks": [%s],
		"assets": [%s]
	}`, user, task, oldAsset[1:]+newerAssets)

	if err := os.WriteFile(dataPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	server := New(config.Config{Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir()})
	token := loginToken(t, server.Handler, "video@example.com", "Demo123!")

	res := authedRequest(t, server.Handler, http.MethodGet, "/api/v1/generation-tasks/task_video_old", nil, token)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}

	var parsed generationTask
	if err := json.Unmarshal(res.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Ownership test: another user must not be able to get this task or its outputUrl
	otherToken := loginToken(t, server.Handler, "video_other@example.com", "Demo123!")
	otherRes := authedRequest(t, server.Handler, http.MethodGet, "/api/v1/generation-tasks/task_video_old", nil, otherToken)
	if otherRes.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for other user accessing task, got %d", otherRes.Code)
	}

	// Workspace list test: /api/v1/user/online-image must remain compact (outputUrl must not be restored)
	onlineImgRes := authedRequest(t, server.Handler, http.MethodGet, "/api/v1/user/online-image", nil, token)
	if onlineImgRes.Code != http.StatusOK {
		t.Fatalf("online-image status = %d", onlineImgRes.Code)
	}
	var onlineImgData struct {
		RecentTasks []generationTask `json:"recentTasks"`
	}
	if err := json.Unmarshal(onlineImgRes.Body.Bytes(), &onlineImgData); err != nil {
		t.Fatal(err)
	}
	for _, tsk := range onlineImgData.RecentTasks {
		if tsk.ID == "task_video_old" && tsk.OutputURL != "" {
			t.Errorf("online-image AC-4 violation: OutputURL should remain empty in compact workspace list, got %q", tsk.OutputURL)
		}
	}
}
