package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGenerationTaskSummarySelectOmitsListBloatKeys(t *testing.T) {
	sql := generationTaskSummarySelect
	for _, key := range []string{
		"first_frame", "final_schema_snapshot", "limit_snapshot",
		"organization_id", "tenant_id", "billing_account_id", "billing_scope", "billing_type",
		"module_code", "model_name", "billingReservedAt", "billingReserved",
		"billingReservationBalanceBefore", "billingReservationBalanceAfter", "billingReservationPointCost",
	} {
		if !strings.Contains(sql, "'"+key+"'") {
			t.Fatalf("list summary select must subtract %s at SQL layer: %s", key, sql)
		}
	}
	detail := generationTaskDetailSelect
	for _, key := range []string{"first_frame", "final_schema_snapshot", "limit_snapshot"} {
		token := "- '" + key + "'"
		if strings.Contains(detail, token) {
			t.Fatalf("detail select must keep %s for single-task fetch: %s", key, detail)
		}
	}
	if !strings.Contains(detail, generationTaskParamsDetailExpr) {
		t.Fatal("detail select lost params projection")
	}
}

func TestAssetWorkspaceListSelectOmitsMetadataBloatKeys(t *testing.T) {
	for _, key := range []string{"thumbnailUrl", "storageObjectKey", "sourceUrl"} {
		if !strings.Contains(assetWorkspaceListSelect, "'"+key+"'") {
			t.Fatalf("workspace list select must subtract metadata.%s at SQL layer: %s", key, assetWorkspaceListSelect)
		}
	}
	if strings.Contains(assetWorkspaceListSelect, "coalesce(metadata::text") {
		t.Fatal("workspace list still reads full metadata::text without key subtraction")
	}
	summary := assetSummarySelect
	if strings.Contains(summary, "'thumbnailUrl'") || strings.Contains(summary, "'storageObjectKey'") {
		t.Fatal("detail/summary asset select must not strip metadata keys needed by download/sign")
	}
	if !strings.Contains(strings.ToLower(summary), "metadata::text") {
		t.Fatal("detail/summary asset select lost full metadata")
	}
}

func TestCompactWorkspaceListTasksDropsSchemaLimitAndFirstFrame(t *testing.T) {
	huge := "data:image/png;base64," + strings.Repeat("A", 64*1024)
	items := compactWorkspaceListTasks([]generationTask{{
		ID:     "task_1",
		Prompt: "a cat",
		Model:  "gpt-image-1",
		Params: map[string]any{
			"first_frame":            huge,
			"final_schema_snapshot":  map[string]any{"fields": []any{map[string]any{"key": "size"}}},
			"limit_snapshot":         map[string]any{"models": map[string]any{"allowed": []any{"a"}}},
			"provider":               "openai",
			"size":                   "1024x1024",
			"quality":                "high",
			"n":                      1,
			"ratio":                  "1:1",
			"resolution":             "1k",
			"width":                  1024,
			"height":                 1024,
			"outputFormat":           "png",
			"moderation":             "auto",
			"imageQuality":           "high",
			"count":                  2,
			"output_format":          "jpeg",
			"transparent_output":     false,
			"transparentOutput":      true,
			"imageRatio":             "16:9",
		},
	}})
	if len(items) != 1 {
		t.Fatalf("items=%d", len(items))
	}
	params := items[0].Params
	for _, key := range []string{"first_frame", "final_schema_snapshot", "limit_snapshot"} {
		if _, ok := params[key]; ok {
			t.Fatalf("compact still kept %s: %+v", key, params[key])
		}
	}
	for _, key := range []string{
		"provider", "size", "quality", "n", "ratio", "resolution", "width", "height",
		"outputFormat", "moderation", "imageQuality", "count", "output_format",
		"transparent_output", "transparentOutput", "imageRatio",
	} {
		if _, ok := params[key]; !ok {
			t.Fatalf("reuse field %s missing after compact: %+v", key, params)
		}
	}
	if items[0].Prompt != "a cat" || items[0].Model != "gpt-image-1" {
		t.Fatalf("prompt/model dropped: %+v", items[0])
	}
}

func TestUserOnlineImageListProjectionDropsBloatKeepsReuseAndTickets(t *testing.T) {
	const itemCount = 40
	hugeFrame := "data:image/png;base64," + strings.Repeat("F", 48*1024)
	hugeThumb := "data:image/jpeg;base64," + strings.Repeat("T", 24*1024)
	handler, token, provider, stored := newWorkspaceListPerfFixture(t, itemCount, 0)
	store := handler.store.(*jsonStore)
	if err := store.updateAdmin(func(data *adminPlatformData) error {
		for index := range data.GenerationTasks {
			if data.GenerationTasks[index].UserID != stored[0].UserID {
				continue
			}
			data.GenerationTasks[index].Prompt = "reuse me " + data.GenerationTasks[index].ID
			data.GenerationTasks[index].Model = "gpt-image-1"
			data.GenerationTasks[index].Params = map[string]any{
				"first_frame":           hugeFrame,
				"final_schema_snapshot": map[string]any{"fields": []any{map[string]any{"key": "size", "options": []any{"1k", "2k", "4k"}}}},
				"limit_snapshot":        map[string]any{"models": map[string]any{"allowed": []any{"gpt-image-1", "seedream"}}},
				"provider":              "openai",
				"size":                  "1024x1024",
				"quality":               "high",
				"n":                     1,
				"ratio":                 "1:1",
				"resolution":            "1k",
				"width":                 1024,
				"height":                1024,
				"outputFormat":          "png",
				"moderation":            "auto",
			}
		}
		for index := range data.Assets {
			if data.Assets[index].UserID != stored[0].UserID {
				continue
			}
			if data.Assets[index].Metadata == nil {
				data.Assets[index].Metadata = map[string]any{}
			}
			data.Assets[index].Metadata["thumbnailUrl"] = hugeThumb
			data.Assets[index].Metadata["storageObjectKey"] = "tenants/t1/" + data.Assets[index].ID + ".png"
			data.Assets[index].Metadata["sourceUrl"] = hugeFrame
			data.Assets[index].Metadata["fileId"] = data.Assets[index].Metadata["fileId"]
			data.Assets[index].ThumbnailURL = hugeThumb
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

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
	body := response.Body.Bytes()
	got := len(body)
	if got > 100*1024 {
		t.Fatalf("P1.5 wire budget exceeded: got=%d want<=100KB", got)
	}
	for _, banned := range []string{
		`"first_frame"`,
		`"final_schema_snapshot"`,
		`"limit_snapshot"`,
		`"storageObjectKey"`,
		"data:image/",
	} {
		if bytes.Contains(body, []byte(banned)) {
			t.Fatalf("payload still contains %s (bytes=%d)", banned, got)
		}
	}

	var payload struct {
		RecentTasks []generationTask `json:"recentTasks"`
		Assets      []asset          `json:"assets"`
		Summary     map[string]any   `json:"summary"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.RecentTasks) != itemCount || len(payload.Assets) != itemCount {
		t.Fatalf("tasks=%d assets=%d", len(payload.RecentTasks), len(payload.Assets))
	}
	if _, ok := payload.Summary["totalPoints"]; !ok {
		t.Fatalf("summary lost totalPoints: %+v", payload.Summary)
	}
	for _, task := range payload.RecentTasks {
		if task.ThumbnailURL != "" {
			t.Fatalf("recentTasks thumbnailUrl must stay empty: %s", task.ThumbnailURL)
		}
		if task.Prompt == "" || task.Model == "" {
			t.Fatalf("reuse prompt/model missing: %+v", task)
		}
		for _, key := range []string{"provider", "size", "quality", "n", "ratio", "resolution", "width", "height", "outputFormat", "moderation"} {
			if _, ok := task.Params[key]; !ok {
				t.Fatalf("reuse param %s missing on %s: %+v", key, task.ID, task.Params)
			}
		}
		for _, key := range []string{"first_frame", "final_schema_snapshot", "limit_snapshot"} {
			if _, ok := task.Params[key]; ok {
				t.Fatalf("task %s still has %s", task.ID, key)
			}
		}
	}
	for _, item := range payload.Assets {
		assertWorkspaceListTicketThumbnailURL(t, item)
		if item.Metadata["thumbnailUrl"] != nil || item.Metadata["storageObjectKey"] != nil || item.Metadata["sourceUrl"] != nil {
			t.Fatalf("asset metadata still has list bloat: %+v", item.Metadata)
		}
		if item.Metadata["fileId"] == nil || item.Metadata["fileId"] == "" {
			t.Fatalf("asset dropped fileId: %+v", item.Metadata)
		}
	}
	t.Logf("P1.5 online-image wire=%d bytes tasks=%d assets=%d", got, len(payload.RecentTasks), len(payload.Assets))
}

func TestUserDashboardListProjectionSharesTaskCompact(t *testing.T) {
	hugeFrame := "data:image/png;base64," + strings.Repeat("D", 32*1024)
	handler, token, _, stored := newWorkspaceListPerfFixture(t, 8, 0)
	store := handler.store.(*jsonStore)
	if err := store.updateAdmin(func(data *adminPlatformData) error {
		for index := range data.GenerationTasks {
			if data.GenerationTasks[index].UserID != stored[0].UserID {
				continue
			}
			data.GenerationTasks[index].Params = map[string]any{
				"first_frame":           hugeFrame,
				"final_schema_snapshot": map[string]any{"fields": []any{}},
				"limit_snapshot":        map[string]any{"ok": true},
				"size":                  "1024x1024",
				"quality":               "medium",
				"n":                     1,
			}
		}
		for index := range data.Assets {
			if data.Assets[index].UserID != stored[0].UserID {
				continue
			}
			if data.Assets[index].Metadata == nil {
				data.Assets[index].Metadata = map[string]any{}
			}
			data.Assets[index].Metadata["storageObjectKey"] = "leak/" + data.Assets[index].ID
			data.Assets[index].Metadata["thumbnailUrl"] = hugeFrame
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/dashboard?taskLimit=30&assetLimit=30", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Host = "ai.zs-kjhn.cn"
	response := httptest.NewRecorder()
	handler.userDashboard(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.Bytes()
	for _, banned := range []string{`"first_frame"`, `"final_schema_snapshot"`, `"limit_snapshot"`, `"storageObjectKey"`, "data:image/"} {
		if bytes.Contains(body, []byte(banned)) {
			t.Fatalf("dashboard still contains %s", banned)
		}
	}
	var payload struct {
		Summary      map[string]any   `json:"summary"`
		RecentTasks  []generationTask `json:"recentTasks"`
		RecentAssets []asset          `json:"recentAssets"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload.Summary["totalPoints"]; !ok {
		t.Fatalf("dashboard summary lost totalPoints: %+v", payload.Summary)
	}
	if len(payload.RecentTasks) == 0 || len(payload.RecentAssets) == 0 {
		t.Fatalf("dashboard emptied lists: tasks=%d assets=%d", len(payload.RecentTasks), len(payload.RecentAssets))
	}
	for _, item := range payload.RecentAssets {
		assertWorkspaceListTicketThumbnailURL(t, item)
	}
}
