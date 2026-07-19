package httpserver

import (
	"context"
	"strings"
	"testing"

	"xianzhi-ai/backend-go/internal/connector"
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
