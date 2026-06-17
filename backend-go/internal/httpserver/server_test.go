package httpserver

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"xianzhi-ai/backend-go/internal/config"
)

func TestGenerationTaskLifecycle(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
	handler := server.Handler

	assertStatus(t, handler, http.MethodGet, "/api/v1/health", nil, http.StatusOK)
	assertStatus(t, handler, http.MethodGet, "/api/v1/models", nil, http.StatusOK)

	createBody := bytes.NewBufferString(`{"type":"TEXT_TO_IMAGE","prompt":"画一只小猫","model":"mock-standard","params":{"count":1}}`)
	createRes := request(t, handler, http.MethodPost, "/api/v1/generation-tasks", createBody)
	if createRes.Code != http.StatusOK {
		t.Fatalf("create task status = %d, body = %s", createRes.Code, createRes.Body.String())
	}
	var task generationTask
	if err := json.NewDecoder(createRes.Body).Decode(&task); err != nil {
		t.Fatal(err)
	}
	if task.Status != "SUCCEEDED" || len(task.ResultIDs) != 1 {
		t.Fatalf("unexpected task: %+v", task)
	}

	assertStatus(t, handler, http.MethodGet, "/api/v1/generation-tasks", nil, http.StatusOK)
	assetsRes := request(t, handler, http.MethodGet, "/api/v1/assets", nil)
	if assetsRes.Code != http.StatusOK {
		t.Fatalf("list assets status = %d, body = %s", assetsRes.Code, assetsRes.Body.String())
	}
	var assets []asset
	if err := json.NewDecoder(assetsRes.Body).Decode(&assets); err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 {
		t.Fatalf("assets length = %d, want 1", len(assets))
	}
	if strings.Contains(assets[0].URL, "picsum.photos") {
		t.Fatalf("asset URL still uses random placeholder: %s", assets[0].URL)
	}
	const prefix = "data:image/svg+xml;base64,"
	if !strings.HasPrefix(assets[0].URL, prefix) {
		t.Fatalf("asset URL = %q, want SVG data URL", assets[0].URL)
	}
	rawSVG, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(assets[0].URL, prefix))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rawSVG), `id="cat-subject"`) {
		t.Fatalf("cat prompt did not render cat SVG: %s", string(rawSVG))
	}
	assertStatus(t, handler, http.MethodDelete, "/api/v1/assets/"+task.ResultIDs[0], nil, http.StatusOK)
}

func TestDeleteMissingAssetReturnsNotFound(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})

	assertStatus(t, server.Handler, http.MethodDelete, "/api/v1/assets/missing", nil, http.StatusNotFound)
}

func TestConcurrentGenerationTaskCreatesKeepUniqueIDs(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	server := New(config.Config{
		Addr:      ":0",
		DataPath:  dataPath,
		StaticDir: t.TempDir(),
	})
	handler := server.Handler

	const requestCount = 20
	errs := make(chan string, requestCount)
	var wg sync.WaitGroup
	for i := 0; i < requestCount; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := bytes.NewBufferString(`{"type":"TEXT_TO_IMAGE","prompt":"cat ` + string(rune('a'+i)) + `","model":"mock-standard","params":{"count":1}}`)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/generation-tasks", body)
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			if res.Code != http.StatusOK {
				errs <- res.Body.String()
				return
			}
			var task generationTask
			if err := json.NewDecoder(res.Body).Decode(&task); err != nil {
				errs <- err.Error()
				return
			}
			if task.ID == "" || len(task.ResultIDs) != 1 {
				errs <- "created task is missing ID or result"
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	tasksRes := request(t, handler, http.MethodGet, "/api/v1/generation-tasks", nil)
	if tasksRes.Code != http.StatusOK {
		t.Fatalf("list tasks status = %d, body = %s", tasksRes.Code, tasksRes.Body.String())
	}
	var tasks []generationTask
	if err := json.NewDecoder(tasksRes.Body).Decode(&tasks); err != nil {
		t.Fatal(err)
	}
	if len(tasks) != requestCount {
		t.Fatalf("tasks length = %d, want %d", len(tasks), requestCount)
	}
	seenTasks := map[string]bool{}
	seenAssets := map[string]bool{}
	for _, task := range tasks {
		if seenTasks[task.ID] {
			t.Fatalf("duplicate task ID %q", task.ID)
		}
		seenTasks[task.ID] = true
		for _, assetID := range task.ResultIDs {
			if seenAssets[assetID] {
				t.Fatalf("duplicate asset ID %q", assetID)
			}
			seenAssets[assetID] = true
		}
	}
}

func TestWriteFileAtomicallyReplacesTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "store.json")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := writeFileAtomically(path, []byte("new\n")); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "new\n" {
		t.Fatalf("store content = %q, want %q", string(raw), "new\n")
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".store.json.*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files were not cleaned up: %v", matches)
	}
}

func assertStatus(t *testing.T, handler http.Handler, method string, path string, body *bytes.Buffer, want int) {
	t.Helper()
	res := request(t, handler, method, path, body)
	if res.Code != want {
		t.Fatalf("%s %s status = %d, want %d, body = %s", method, path, res.Code, want, res.Body.String())
	}
}

func request(t *testing.T, handler http.Handler, method string, path string, body *bytes.Buffer) *httptest.ResponseRecorder {
	t.Helper()
	if body == nil {
		body = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequest(method, path, body)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}
