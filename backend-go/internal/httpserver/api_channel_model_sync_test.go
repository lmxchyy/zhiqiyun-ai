package httpserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"xianzhi-ai/backend-go/internal/config"
)

func TestSplitProviderModelsRecognizesImageAndVideoFamilies(t *testing.T) {
	imageModels, chatModels, videoModels := splitProviderModels([]string{
		"banana-pro-1k",
		"qwen-image-plus",
		"claude-opus-4-6",
		"seedance-2.0",
	})

	for _, model := range []string{"banana-pro-1k", "qwen-image-plus"} {
		if !containsString(imageModels, model) {
			t.Fatalf("image models missing %q: %#v", model, imageModels)
		}
	}
	if !containsString(chatModels, "claude-opus-4-6") {
		t.Fatalf("chat models missing claude-opus-4-6: %#v", chatModels)
	}
	if !containsString(videoModels, "seedance-2.0") {
		t.Fatalf("video models missing seedance-2.0: %#v", videoModels)
	}
}

func TestFetchProviderModelsCanMergeCandidatesWithoutPublishingModels(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"deepseek-v4-flash"},{"id":"banana-pro-1k"},{"id":"claude-opus-4-6"}]}`))
	}))
	defer upstream.Close()

	server := New(config.Config{Addr: ":0", DataPath: filepath.Join(t.TempDir(), "store.json"), StaticDir: t.TempDir()})
	adminToken := loginToken(t, server.Handler, "admin@xianzhi.ai", "Admin123!")
	createBody := fmt.Sprintf(`{"name":"candidate upstream","baseUrl":%q,"protocol":"openai","fetchModelsPath":"/models","status":"ACTIVE","models":["deepseek-v4-flash"]}`, upstream.URL)
	createdResponse := authedRequest(t, server.Handler, http.MethodPost, "/api/v1/admin/api/provider-channels", bytes.NewBufferString(createBody), adminToken)
	if createdResponse.Code != http.StatusOK {
		t.Fatalf("create channel status = %d, body = %s", createdResponse.Code, createdResponse.Body.String())
	}
	var created struct {
		Item adminAPIChannel `json:"item"`
	}
	if err := json.NewDecoder(createdResponse.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	syncResponse := authedRequest(t, server.Handler, http.MethodPost, "/api/v1/admin/api/provider-channels/"+created.Item.ID+"/fetch-models", bytes.NewBufferString(`{"syncModels":true}`), adminToken)
	if syncResponse.Code != http.StatusOK {
		t.Fatalf("sync channel status = %d, body = %s", syncResponse.Code, syncResponse.Body.String())
	}
	var synced struct {
		Synced          bool     `json:"synced"`
		AddedModels     []string `json:"addedModels"`
		CandidateModels []string `json:"candidateModels"`
		ImageModels     []string `json:"imageModels"`
	}
	if err := json.NewDecoder(syncResponse.Body).Decode(&synced); err != nil {
		t.Fatal(err)
	}
	if !synced.Synced {
		t.Fatalf("expected synced response: %#v", synced)
	}
	for _, model := range []string{"deepseek-v4-flash", "banana-pro-1k", "claude-opus-4-6"} {
		if !containsString(synced.CandidateModels, model) {
			t.Fatalf("candidate models missing %q: %#v", model, synced.CandidateModels)
		}
	}
	if containsString(synced.AddedModels, "deepseek-v4-flash") {
		t.Fatalf("existing model must not be reported as added: %#v", synced.AddedModels)
	}
	for _, model := range []string{"banana-pro-1k", "claude-opus-4-6"} {
		if !containsString(synced.AddedModels, model) {
			t.Fatalf("added models missing %q: %#v", model, synced.AddedModels)
		}
	}
	if !containsString(synced.ImageModels, "banana-pro-1k") {
		t.Fatalf("banana model was not classified as image: %#v", synced.ImageModels)
	}

	channelsResponse := authedRequest(t, server.Handler, http.MethodGet, "/api/v1/admin/api/provider-channels", nil, adminToken)
	var channels struct {
		Items []adminAPIChannel `json:"items"`
	}
	if err := json.NewDecoder(channelsResponse.Body).Decode(&channels); err != nil {
		t.Fatal(err)
	}
	for _, channel := range channels.Items {
		if channel.ID == created.Item.ID {
			if len(channel.Models) != 3 {
				t.Fatalf("persisted candidate models = %#v", channel.Models)
			}
			return
		}
	}
	t.Fatalf("synced channel %s was not persisted", created.Item.ID)
}
