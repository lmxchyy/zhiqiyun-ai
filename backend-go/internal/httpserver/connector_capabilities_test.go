package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"xianzhi-ai/backend-go/internal/connector"
	feishuconnector "xianzhi-ai/backend-go/internal/connector/feishu"
	storagecenter "xianzhi-ai/backend-go/internal/storage"
)

func TestVideoHandlerPermissionsAndLimits(t *testing.T) {
	cfg := defaultConnectorConfig()
	cfg.AIVideoEnabled = true
	cfg.VideoPermissionMode = "allow"
	cfg.VideoMaxDuration = 15
	cfg.VideoMaxResolution = "1080p"
	runtime := &connectorCapabilityRuntime{
		item: enterpriseConnector{Config: cfg},
		binding: connectorUserBinding{Status: "active", Permission: map[string]any{"videoGenerate": true, "maxVideoDuration": 10}},
		message: connectorMessageRecord{ChatType: "single"},
	}
	handler := &videoGenerateHandler{connectorHandlerBase: connectorHandlerBase{runtime: runtime}}
	valid := connector.AICommand{Parameters: map[string]any{"duration": 10, "resolution": "1080p"}}
	if err := handler.Validate(context.Background(), valid); err != nil {
		t.Fatalf("valid video rejected: %v", err)
	}
	tooLong := connector.AICommand{Parameters: map[string]any{"duration": 11, "resolution": "720p"}}
	if err := handler.Validate(context.Background(), tooLong); err == nil || !strings.Contains(err.Error(), "duration") {
		t.Fatalf("duration limit err=%v", err)
	}
	runtime.binding.Permission["videoGenerate"] = false
	if err := handler.Validate(context.Background(), valid); err == nil {
		t.Fatal("member without video permission was allowed")
	}
}

func TestImageToVideoRequiresOwnedReference(t *testing.T) {
	cfg := defaultConnectorConfig()
	cfg.AIVideoEnabled, cfg.AllowImageToVideo = true, true
	runtime := &connectorCapabilityRuntime{
		item: enterpriseConnector{Config: cfg},
		binding: connectorUserBinding{Status: "active", Permission: map[string]any{"videoGenerate": true}},
		message: connectorMessageRecord{ChatType: "single"},
	}
	handler := &imageToVideoHandler{connectorHandlerBase: connectorHandlerBase{runtime: runtime}}
	command := connector.AICommand{Parameters: map[string]any{"duration": 5, "resolution": "720p"}}
	if err := handler.Validate(context.Background(), command); err == nil || !strings.Contains(err.Error(), "reference image") {
		t.Fatalf("missing reference err=%v", err)
	}
	command.ReferenceAssets = []connector.ReferenceAsset{{ID: "asset-owned", MediaType: "image"}}
	if err := handler.Validate(context.Background(), command); err != nil {
		t.Fatalf("owned reference rejected: %v", err)
	}
}

func TestPPTHandlerPermissionsAndPageLimit(t *testing.T) {
	cfg := defaultConnectorConfig()
	cfg.PPTEnabled, cfg.PPTPermissionMode, cfg.PPTMaxPageCount = true, "allow", 20
	runtime := &connectorCapabilityRuntime{
		item: enterpriseConnector{Config: cfg},
		binding: connectorUserBinding{Status: "active", Permission: map[string]any{"pptGenerate": true, "maxPptPages": 10}},
		message: connectorMessageRecord{ChatType: "single"},
	}
	handler := &pptGenerateHandler{connectorHandlerBase: connectorHandlerBase{runtime: runtime}}
	if err := handler.Validate(context.Background(), connector.AICommand{Parameters: map[string]any{"page_count": 10}}); err != nil {
		t.Fatalf("valid ppt rejected: %v", err)
	}
	if err := handler.Validate(context.Background(), connector.AICommand{Parameters: map[string]any{"page_count": 11}}); err == nil || !strings.Contains(err.Error(), "page") {
		t.Fatalf("page limit err=%v", err)
	}
	runtime.item.Config.PPTPermissionMode = "approval"
	if err := handler.Validate(context.Background(), connector.AICommand{Parameters: map[string]any{"page_count": 8}}); err == nil || !strings.Contains(err.Error(), "approval") {
		t.Fatalf("approval mode err=%v", err)
	}
}

func TestConnectorResultCardReservesSafeActions(t *testing.T) {
	message := connectorResultMessage("视频生成完成", connector.AICommand{Intent: connector.IntentVideoGenerate}, connector.CapabilityResult{
		InternalTaskID: "video-1", Status: "completed", Progress: 100,
		Data: map[string]any{"connectorTaskId": "connector-task-1", "downloadURL": "https://example.invalid/signed"},
	})
	encoded := string(connectorJSON(message.Card))
	for _, action := range []string{"task.view", "task.retry_delivery", "video.generate_similar", "asset.download", "connector-task-1"} {
		if !strings.Contains(encoded, action) {
			t.Fatalf("card missing %q: %s", action, encoded)
		}
	}
}

type connectorVideoReopenProvider struct {
	*generatedStorageTestProvider
	opened atomic.Bool
	closed atomic.Bool
}

func (p *connectorVideoReopenProvider) OpenObject(ctx context.Context, key string) (io.ReadCloser, error) {
	stream, err := p.generatedStorageTestProvider.OpenObject(ctx, key)
	if err != nil {
		return nil, err
	}
	p.opened.Store(true)
	return &connectorVideoTrackingReadCloser{ReadCloser: stream, closed: &p.closed}, nil
}

type connectorVideoTrackingReadCloser struct {
	io.ReadCloser
	closed *atomic.Bool
}

func (r *connectorVideoTrackingReadCloser) Close() error {
	r.closed.Store(true)
	return r.ReadCloser.Close()
}

func TestSendConnectorStoredVideoReopensPrivateObjectAndClosesStream(t *testing.T) {
	const (
		tenantID = "tenant_connector_video"
		userID   = "user_connector_video"
	)
	payload := []byte("private-video-object")
	provider := &connectorVideoReopenProvider{generatedStorageTestProvider: &generatedStorageTestProvider{objects: map[string]storagecenter.ObjectMetadata{}}}
	service := newVideoAdmissionStorageService(provider)
	file, err := service.StoreObject(t.Context(), storagecenter.UploadInitInput{
		TenantID: tenantID, UserID: userID, FileName: "private.mp4", FileSize: int64(len(payload)), MIMEType: "video/mp4",
		BusinessType: "generation_result", BusinessID: "task_private_delivery", Visibility: "PRIVATE",
	}, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}

	uploaded := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/auth/v3/tenant_access_token/internal":
			_, _ = w.Write([]byte(`{"code":0,"tenant_access_token":"token","expire":3600}`))
		case "/im/v1/files":
			if err := r.ParseMultipartForm(2 << 20); err != nil {
				t.Errorf("parse multipart: %v", err)
				return
			}
			part, _, err := r.FormFile("file")
			if err != nil {
				t.Errorf("open uploaded file: %v", err)
				return
			}
			defer part.Close()
			raw, err := io.ReadAll(part)
			if err != nil {
				t.Errorf("read uploaded file: %v", err)
				return
			}
			uploaded <- raw
			_, _ = w.Write([]byte(`{"code":0,"data":{"file_key":"private-file-key"}}`))
		case "/im/v1/messages":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode message: %v", err)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"message_id":"om-private-video"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := feishuconnector.New(feishuconnector.Config{AppID: "app", AppSecret: "secret", BaseURL: server.URL, HTTPClient: server.Client()})

	if _, err := sendConnectorStoredVideo(t.Context(), service, client, connector.MessageTarget{ChatID: "chat-private"}, "another-user", file); !errors.Is(err, storagecenter.ErrFileForbidden) {
		t.Fatalf("cross-user private reopen error=%v, want ErrFileForbidden", err)
	}
	if provider.opened.Load() {
		t.Fatal("cross-user private reopen reached object storage")
	}
	result, err := sendConnectorStoredVideo(t.Context(), service, client, connector.MessageTarget{ChatID: "chat-private"}, userID, file)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExternalMessageID != "om-private-video" {
		t.Fatalf("message id=%q, want om-private-video", result.ExternalMessageID)
	}
	select {
	case raw := <-uploaded:
		if !bytes.Equal(raw, payload) {
			t.Fatalf("uploaded bytes=%q, want private object %q", raw, payload)
		}
	default:
		t.Fatal("private video was not uploaded")
	}
	if !provider.opened.Load() || !provider.closed.Load() {
		t.Fatalf("private object lifecycle opened=%v closed=%v, want both true", provider.opened.Load(), provider.closed.Load())
	}
}
