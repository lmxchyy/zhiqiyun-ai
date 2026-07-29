package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"xianzhi-ai/backend-go/internal/app/generation"
)

type businessCodeError interface {
	BusinessCode() string
}

func videoValidationTestResolved(capabilities adminVideoModelCapabilities) resolvedModuleSchema {
	return resolvedModuleSchema{
		Module: adminAIModule{ModuleCode: moduleVideoGeneration},
		Model: adminAIModel{
			ID:                "video-model-id",
			ModelName:         "video-model",
			ModuleCode:        moduleVideoGeneration,
			VideoCapabilities: &capabilities,
		},
		Schema: adminAIParameterSchema{SchemaJSON: adminAIParameterSchemaJSON{Fields: []adminAIParameterField{
			{Key: "duration", Type: "select", Options: []any{float64(5), float64(10)}},
			{Key: "resolution", Type: "select", Options: []any{"720p", "1080p"}},
			{Key: "aspect_ratio", Type: "select", Options: []any{"16:9", "9:16"}},
			{Key: "first_frame", Type: "image_upload"},
			{Key: "last_frame", Type: "image_upload"},
		}}},
	}
}

func videoValidationTestCapabilities() adminVideoModelCapabilities {
	return adminVideoModelCapabilities{
		SupportsTextToVideo:   true,
		SupportsImageToVideo:  true,
		SupportsFirstFrame:    true,
		SupportsLastFrame:     false,
		MaxReferenceImages:    1,
		SupportedDurations:    []int{5, 10},
		SupportedResolutions:  []string{"720p", "1080p"},
		SupportedAspectRatios: []string{"16:9", "9:16"},
	}
}

func requireVideoValidationCode(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected validation code %s, got nil", want)
	}
	coded, ok := err.(businessCodeError)
	if !ok || coded.BusinessCode() != want {
		t.Fatalf("validation error = %T %v, want code %s", err, err, want)
	}
}

func TestVideoModelCapabilitiesLegacyDefaultsAreSafe(t *testing.T) {
	model := adminAIModel{ModelType: "video", ModuleCode: moduleVideoGeneration}
	schema := adminAIParameterSchemaJSON{Fields: []adminAIParameterField{
		{Key: "duration", Options: []any{float64(5), float64(10)}},
		{Key: "resolution", Options: []any{"720p", "1080p"}},
		{Key: "aspect_ratio", Options: []any{"16:9", "9:16"}},
	}}

	got := resolveVideoModelCapabilities(model, schema)
	if !got.SupportsTextToVideo || got.SupportsImageToVideo || got.SupportsFirstFrame || got.SupportsLastFrame || got.MaxReferenceImages != 0 {
		t.Fatalf("unsafe legacy capability defaults: %+v", got)
	}
	if len(got.SupportedDurations) != 2 || len(got.SupportedResolutions) != 2 || len(got.SupportedAspectRatios) != 2 {
		t.Fatalf("schema options were not exposed as supported values: %+v", got)
	}
}

func TestVideoGenerationTextModeRejectsImageFields(t *testing.T) {
	request := generation.CreateRequest{
		Type: "TEXT_TO_VIDEO",
		Params: map[string]any{
			"duration":     float64(5),
			"resolution":   "720p",
			"aspect_ratio": "16:9",
			"first_frame":  "https://example.test/first.png",
		},
	}
	err := validateVideoGenerationRequest(&request, videoValidationTestResolved(videoValidationTestCapabilities()))
	requireVideoValidationCode(t, err, "VIDEO_TEXT_MODE_IMAGE_FORBIDDEN")
	if err == nil || err.Error() != "文生视频模式不得携带首帧图、尾帧图或其他图片字段" {
		t.Fatalf("unexpected Chinese message: %v", err)
	}
}

func TestVideoGenerationImageModeRequiresFirstFrame(t *testing.T) {
	request := generation.CreateRequest{Type: "IMAGE_TO_VIDEO", Params: map[string]any{"duration": float64(5)}}
	err := validateVideoGenerationRequest(&request, videoValidationTestResolved(videoValidationTestCapabilities()))
	requireVideoValidationCode(t, err, "VIDEO_FIRST_FRAME_REQUIRED")
}

func TestVideoGenerationTextOnlyModelRejectsImageMode(t *testing.T) {
	capabilities := videoValidationTestCapabilities()
	capabilities.SupportsImageToVideo = false
	capabilities.SupportsFirstFrame = false
	capabilities.MaxReferenceImages = 0
	request := generation.CreateRequest{Type: "IMAGE_TO_VIDEO", Params: map[string]any{"first_frame": "https://example.test/first.png"}}
	err := validateVideoGenerationRequest(&request, videoValidationTestResolved(capabilities))
	requireVideoValidationCode(t, err, "VIDEO_MODE_NOT_SUPPORTED")
}

func TestVideoGenerationNormalizesOneLegacyFirstFrame(t *testing.T) {
	firstFrame := "https://example.test/first.png"
	request := generation.CreateRequest{
		Type: "IMAGE_TO_VIDEO",
		Params: map[string]any{
			"duration":        float64(5),
			"resolution":      "720p",
			"aspect_ratio":    "16:9",
			"image_url":       firstFrame,
			"image_urls":      []any{firstFrame},
			"referenceImages": []any{map[string]any{"url": firstFrame}},
		},
	}
	if err := validateVideoGenerationRequest(&request, videoValidationTestResolved(videoValidationTestCapabilities())); err != nil {
		t.Fatalf("one legacy first frame was rejected: %v", err)
	}
	if request.Params["first_frame"] != firstFrame {
		t.Fatalf("canonical first_frame = %#v", request.Params["first_frame"])
	}
	for _, key := range []string{"image_url", "image_urls", "referenceImages", "reference_images", "reference_image"} {
		if _, exists := request.Params[key]; exists {
			t.Fatalf("legacy key %s leaked after normalization: %+v", key, request.Params)
		}
	}
}

func TestVideoGenerationRejectsMultipleDistinctImages(t *testing.T) {
	request := generation.CreateRequest{
		Type: "IMAGE_TO_VIDEO",
		Params: map[string]any{
			"image_urls": []any{"https://example.test/first.png", "https://example.test/second.png"},
		},
	}
	err := validateVideoGenerationRequest(&request, videoValidationTestResolved(videoValidationTestCapabilities()))
	requireVideoValidationCode(t, err, "VIDEO_IMAGE_LIMIT_EXCEEDED")
}

func TestVideoGenerationRejectsUnsupportedLastFrame(t *testing.T) {
	request := generation.CreateRequest{
		Type: "IMAGE_TO_VIDEO",
		Params: map[string]any{
			"first_frame": "https://example.test/first.png",
			"last_frame":  "https://example.test/last.png",
		},
	}
	err := validateVideoGenerationRequest(&request, videoValidationTestResolved(videoValidationTestCapabilities()))
	requireVideoValidationCode(t, err, "VIDEO_LAST_FRAME_NOT_SUPPORTED")
}

func TestVideoGenerationAcceptsSupportedLastFrame(t *testing.T) {
	capabilities := videoValidationTestCapabilities()
	capabilities.SupportsLastFrame = true
	capabilities.MaxReferenceImages = 2
	request := generation.CreateRequest{
		Type: "IMAGE_TO_VIDEO",
		Params: map[string]any{
			"first_frame": "https://example.test/first.png",
			"last_frame":  "https://example.test/last.png",
		},
	}
	if err := validateVideoGenerationRequest(&request, videoValidationTestResolved(capabilities)); err != nil {
		t.Fatalf("supported last frame was rejected: %v", err)
	}
}

func TestVideoGenerationRejectsUnsupportedDuration(t *testing.T) {
	request := generation.CreateRequest{
		Type: "TEXT_TO_VIDEO",
		Params: map[string]any{
			"duration": float64(15),
		},
	}
	err := validateVideoGenerationRequest(&request, videoValidationTestResolved(videoValidationTestCapabilities()))
	requireVideoValidationCode(t, err, "VIDEO_DURATION_NOT_SUPPORTED")
}

func TestVideoGenerationValidationErrorResponseHasStableCode(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeError(recorder, http.StatusBadRequest, newVideoGenerationValidationError("VIDEO_FIRST_FRAME_REQUIRED", "图生视频模式必须上传首帧图"))

	var payload map[string]string
	if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusBadRequest || payload["code"] != "VIDEO_FIRST_FRAME_REQUIRED" || payload["message"] != "图生视频模式必须上传首帧图" || payload["error"] != payload["message"] {
		t.Fatalf("unexpected validation response: status=%d payload=%+v", recorder.Code, payload)
	}
}
