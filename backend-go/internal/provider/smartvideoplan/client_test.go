package smartvideoplan_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/app/smartvideo"
	"xianzhi-ai/backend-go/internal/provider/chat"
	"xianzhi-ai/backend-go/internal/provider/smartvideoplan"
)

type stubChat struct {
	response chat.Response
	err      error
	lastReq  generation.CreateRequest
}

func (s *stubChat) Chat(_ context.Context, req generation.CreateRequest) (chat.Response, error) {
	s.lastReq = req
	return s.response, s.err
}

func validPlanJSON() string {
	return `{
  "schemaVersion": 1,
  "title": "开业海报混剪",
  "summary": "15秒竖版宣传",
  "language": "zh-CN",
  "target": {"aspectRatio": "9:16", "resolution": "720p", "durationMs": 15000},
  "voice": {"enabled": true, "modelKey": "smart-video-speech", "voiceKey": "alloy", "speed": 1},
  "subtitles": {"enabled": true, "preset": "clean", "position": "bottom"},
  "audio": {"sourceGain": 0.2, "voiceGain": 1},
  "scenes": [{
    "id": "scene-1",
    "index": 0,
    "title": "开场",
    "durationMs": 15000,
    "narration": "欢迎光临",
    "clips": [{
      "assetId": "asset_1",
      "assetType": "image",
      "sourceInMs": 0,
      "sourceOutMs": 0,
      "displayDurationMs": 15000,
      "fitMode": "cover",
      "motion": "static",
      "originalAudioGain": 0
    }],
    "transition": {"type": "cut", "durationMs": 0}
  }]
}`
}

func TestPlanClientForcesJSONObjectAndParsesPlan(t *testing.T) {
	stub := &stubChat{response: chat.Response{
		Model: "planner-x",
		Message: chat.Message{Role: "assistant", Content: validPlanJSON()},
		Metadata: map[string]any{"id": "req_1"},
		Usage:    map[string]any{"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30},
	}}
	client := smartvideoplan.NewClient(stub, smartvideoplan.Options{ModelKey: "smart-video-standard"})
	plan, usage, err := client.Plan(context.Background(), smartvideo.PlanRequest{
		Requirement: "开业促销",
		TargetSpec:  smartvideo.TargetSpec{AspectRatio: "9:16", Resolution: "720p", DurationMs: 15000},
		Assets: []smartvideo.ProjectAsset{{
			ID: "asset_1", AssetType: "image", DurationMS: 0,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Title != "开业海报混剪" {
		t.Fatalf("title = %q", plan.Title)
	}
	if usage.ProviderRequestID != "req_1" || usage.TotalTokens != 30 {
		t.Fatalf("usage = %+v", usage)
	}
	format, _ := stub.lastReq.Params["response_format"].(map[string]any)
	if format["type"] != "json_object" {
		t.Fatalf("response_format = %+v", stub.lastReq.Params["response_format"])
	}
}

func TestPlanClientMapsRateLimitAndInvalidJSON(t *testing.T) {
	stub := &stubChat{err: errors.New("chat provider returned HTTP 429: rate limit")}
	client := smartvideoplan.NewClient(stub, smartvideoplan.Options{})
	_, _, err := client.Plan(context.Background(), smartvideo.PlanRequest{
		Requirement: "x",
		TargetSpec:  smartvideo.TargetSpec{AspectRatio: "9:16", Resolution: "720p", DurationMs: 15000},
		Assets:      []smartvideo.ProjectAsset{{ID: "asset_1", AssetType: "image"}},
	})
	if !errors.Is(err, smartvideoplan.ErrRateLimited) {
		t.Fatalf("error = %v", err)
	}

	stub = &stubChat{response: chat.Response{Message: chat.Message{Content: "{not-json"}}}
	client = smartvideoplan.NewClient(stub, smartvideoplan.Options{})
	_, _, err = client.Plan(context.Background(), smartvideo.PlanRequest{
		Requirement: "x",
		TargetSpec:  smartvideo.TargetSpec{AspectRatio: "9:16", Resolution: "720p", DurationMs: 15000},
		Assets:      []smartvideo.ProjectAsset{{ID: "asset_1", AssetType: "image"}},
	})
	if !errors.Is(err, smartvideoplan.ErrInvalidJSON) {
		t.Fatalf("invalid json error = %v", err)
	}
}

func TestPlanClientRejectsUnknownAsset(t *testing.T) {
	bad := strings.Replace(validPlanJSON(), "asset_1", "asset_unknown", 1)
	stub := &stubChat{response: chat.Response{Message: chat.Message{Content: bad}}}
	client := smartvideoplan.NewClient(stub, smartvideoplan.Options{})
	_, _, err := client.Plan(context.Background(), smartvideo.PlanRequest{
		Requirement: "x",
		TargetSpec:  smartvideo.TargetSpec{AspectRatio: "9:16", Resolution: "720p", DurationMs: 15000},
		Assets:      []smartvideo.ProjectAsset{{ID: "asset_1", AssetType: "image"}},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
