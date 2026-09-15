package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/config"
)

func TestAB_Diagnosis_NewTaskVsOldTask(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")

	// Task A: New Video (created 2026-09-15 10:53:00 UTC, with video persistence / storageManaged = true)
	timeA := "2026-09-15T10:53:00Z"
	taskA := fmt.Sprintf(`{
		"id": "task_20260915_1053",
		"userId": "user_demo",
		"type": "TEXT_TO_VIDEO",
		"status": "SUCCEEDED",
		"model": "grok-imagine-1.5-video",
		"prompt": "new video 10:53",
		"resultIds": ["asset_20260915_1053"],
		"params": {
			"duration": 6,
			"aspect_ratio": "16:9",
			"resolution": "480p",
			"providerTask": {
				"videoUrl": "storage://file_video_1053",
				"status": "SUCCEEDED"
			}
		},
		"createdAt": %q
	}`, timeA)

	assetA := fmt.Sprintf(`{
		"id": "asset_20260915_1053",
		"userId": "user_demo",
		"taskId": "task_20260915_1053",
		"name": "new video 10:53",
		"mediaType": "video",
		"url": "https://storage.example.com/download/file_video_1053.mp4",
		"thumbnailUrl": "https://storage.example.com/download/file_video_1053_cover.jpg",
		"metadata": {
			"fileId": "file_video_1053",
			"storageFileId": "file_video_1053",
			"storageManaged": true,
			"coverFileId": "file_video_1053_cover"
		},
		"createdAt": %q
	}`, timeA)

	// Task B: Old Video (e.g. negative routing test user1, created 72h ago, unmanaged provider temp URL)
	timeB := time.Now().UTC().Add(-72 * time.Hour).Format(time.RFC3339)
	taskB := fmt.Sprintf(`{
		"id": "task_000234",
		"userId": "user_demo",
		"type": "TEXT_TO_VIDEO",
		"status": "SUCCEEDED",
		"model": "grok-imagine-1.5-video",
		"prompt": "negative routing test user1",
		"resultIds": ["asset_task_000234"],
		"params": {
			"duration": 6,
			"aspect_ratio": "16:9",
			"resolution": "480p",
			"providerTask": {
				"videoUrl": "https://upstream-provider.example.com/temp-video-000234.mp4",
				"status": "SUCCEEDED"
			}
		},
		"createdAt": %q
	}`, timeB)

	assetB := fmt.Sprintf(`{
		"id": "asset_task_000234",
		"userId": "user_demo",
		"taskId": "task_000234",
		"name": "negative routing test user1",
		"mediaType": "video",
		"url": "https://upstream-provider.example.com/temp-video-000234.mp4",
		"thumbnailUrl": "",
		"metadata": {
			"model": "grok-imagine-1.5-video",
			"prompt": "negative routing test user1"
		},
		"createdAt": %q
	}`, timeB)

	user := `{"id":"user_demo","email":"demo@xianzhi.ai","name":"Demo User","role":"MEMBER","status":"ACTIVE","planId":"plan_free"}`

	raw := fmt.Sprintf(`{
		"users": [%s],
		"generationTasks": [%s, %s],
		"assets": [%s, %s]
	}`, user, taskA, taskB, assetA, assetB)

	if err := os.WriteFile(dataPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	server := New(config.Config{Addr: ":0", DataPath: dataPath, StaticDir: t.TempDir()})
	token := loginToken(t, server.Handler, "demo@xianzhi.ai", "Demo123!")

	// 1. Check GET /api/v1/generation-tasks/:id for A
	resA := authedRequest(t, server.Handler, http.MethodGet, "/api/v1/generation-tasks/task_20260915_1053", nil, token)
	if resA.Code != http.StatusOK {
		t.Fatalf("task A status = %d", resA.Code)
	}
	var taskViewA generationTask
	_ = json.Unmarshal(resA.Body.Bytes(), &taskViewA)

	// 2. Check GET /api/v1/generation-tasks/:id for B
	resB := authedRequest(t, server.Handler, http.MethodGet, "/api/v1/generation-tasks/task_000234", nil, token)
	if resB.Code != http.StatusOK {
		t.Fatalf("task B status = %d", resB.Code)
	}
	var taskViewB generationTask
	_ = json.Unmarshal(resB.Body.Bytes(), &taskViewB)

	// 3. Check GET /api/v1/assets/:id for A
	resAssetA := authedRequest(t, server.Handler, http.MethodGet, "/api/v1/assets/asset_20260915_1053", nil, token)
	var assetViewA struct {
		Item asset `json:"item"`
	}
	_ = json.Unmarshal(resAssetA.Body.Bytes(), &assetViewA)

	// 4. Check GET /api/v1/assets/:id for B
	resAssetB := authedRequest(t, server.Handler, http.MethodGet, "/api/v1/assets/asset_task_000234", nil, token)
	var assetViewB struct {
		Item asset `json:"item"`
	}
	_ = json.Unmarshal(resAssetB.Body.Bytes(), &assetViewB)

	t.Logf("=== TASK A (NEW 10:53) ===")
	t.Logf("Task A OutputURL: %q, ResultIDs: %v, Status: %s", taskViewA.OutputURL, taskViewA.ResultIDs, taskViewA.Status)
	t.Logf("Asset A URL: %q, Availability: %s, VideoStatus: %s", assetViewA.Item.URL, assetViewA.Item.Availability, assetViewA.Item.VideoStatus)

	t.Logf("=== TASK B (OLD task_000234) ===")
	t.Logf("Task B OutputURL: %q, ResultIDs: %v, Status: %s", taskViewB.OutputURL, taskViewB.ResultIDs, taskViewB.Status)
	t.Logf("Asset B URL: %q, Availability: %s, VideoStatus: %s, Reason: %s", assetViewB.Item.URL, assetViewB.Item.Availability, assetViewB.Item.VideoStatus, assetViewB.Item.AvailabilityReason)
}
