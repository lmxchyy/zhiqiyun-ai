package httpserver

import (
	"encoding/json"
	"fmt"
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
			{Key: "duration", Type: "select", Options: []any{float64(4), float64(5), float64(10), float64(15)}},
			{Key: "resolution", Type: "select", Options: []any{"480p", "720p", "1080p", "4k"}},
			{Key: "aspect_ratio", Type: "select", Options: []any{"16:9", "9:16", "1:1"}},
			{Key: "fps", Type: "select", Options: []any{float64(24), float64(30)}},
			{Key: "generate_audio", Type: "switch"},
			{Key: "motion_strength", Type: "select", Options: []any{"low", "medium", "high"}},
			{Key: "camera_movement", Type: "select", Options: []any{"static", "pan", "push", "pull"}},
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
		SupportedDurations:    []int{4, 5, 10, 15},
		SupportedResolutions:  []string{"480p", "720p", "1080p", "4k"},
		SupportedAspectRatios: []string{"16:9", "9:16", "1:1"},
		SupportedParameters:   []string{"duration", "resolution", "aspect_ratio"},
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

func TestNormalizeRequestParamAliasesCanonicalizesVideoRatio(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]any
		want   string
	}{
		{name: "canonical aspect ratio", params: map[string]any{"aspect_ratio": "9:16"}, want: "9:16"},
		{name: "legacy ratio", params: map[string]any{"ratio": "1:1"}, want: "1:1"},
		{name: "canonical wins", params: map[string]any{"aspect_ratio": "16:9", "ratio": "9:16"}, want: "16:9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := generation.CreateRequest{ModuleCode: moduleVideoGeneration, Params: tt.params}
			normalizeRequestParamAliases(&req)
			if got := req.Params["aspect_ratio"]; got != tt.want {
				t.Fatalf("aspect_ratio = %#v, want %q", got, tt.want)
			}
			if _, exists := req.Params["ratio"]; exists {
				t.Fatalf("legacy ratio leaked into normalized task params: %+v", req.Params)
			}
		})
	}
}

func TestVideoCapabilitiesExposeProviderSupportedParameters(t *testing.T) {
	raw, err := json.Marshal(adminVideoModelCapabilities{})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if _, exists := payload["supported_parameters"]; !exists {
		t.Fatalf("provider parameter capabilities missing from API payload: %s", raw)
	}
}

func TestVideoGenerationStripsProviderUnsupportedParameters(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value any
	}{
		{name: "fps", key: "fps", value: float64(30)},
		{name: "audio", key: "generate_audio", value: true},
		{name: "motion strength", key: "motion_strength", value: "high"},
		{name: "camera movement", key: "camera_movement", value: "pan"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := generation.CreateRequest{Type: "TEXT_TO_VIDEO", Params: map[string]any{
				"duration": float64(5), "resolution": "720p", "aspect_ratio": "16:9", tt.key: tt.value,
			}}
			if err := validateVideoGenerationRequest(&request, videoValidationTestResolved(videoValidationTestCapabilities())); err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
			if _, exists := request.Params[tt.key]; exists {
				t.Fatalf("unsupported parameter %s was not stripped: %+v", tt.key, request.Params)
			}
		})
	}
}

func TestVideoGenerationPreservesSupportedCanonicalParameters(t *testing.T) {
	capabilities := videoValidationTestCapabilities()
	capabilities.SupportedParameters = append(capabilities.SupportedParameters,
		"fps", "generate_audio", "motion_strength", "camera_movement")
	tests := []struct {
		name  string
		key   string
		value any
	}{
		{name: "ratio 16:9", key: "aspect_ratio", value: "16:9"},
		{name: "ratio 9:16", key: "aspect_ratio", value: "9:16"},
		{name: "ratio 1:1", key: "aspect_ratio", value: "1:1"},
		{name: "resolution 480p", key: "resolution", value: "480p"},
		{name: "resolution 720p", key: "resolution", value: "720p"},
		{name: "resolution 1080p", key: "resolution", value: "1080p"},
		{name: "resolution 4k", key: "resolution", value: "4k"},
		{name: "duration 4", key: "duration", value: float64(4)},
		{name: "duration 5", key: "duration", value: float64(5)},
		{name: "duration 10", key: "duration", value: float64(10)},
		{name: "duration 15", key: "duration", value: float64(15)},
		{name: "fps 24", key: "fps", value: float64(24)},
		{name: "fps 30", key: "fps", value: float64(30)},
		{name: "audio on", key: "generate_audio", value: true},
		{name: "audio off", key: "generate_audio", value: false},
		{name: "motion low", key: "motion_strength", value: "low"},
		{name: "motion medium", key: "motion_strength", value: "medium"},
		{name: "motion high", key: "motion_strength", value: "high"},
		{name: "camera static", key: "camera_movement", value: "static"},
		{name: "camera pan", key: "camera_movement", value: "pan"},
		{name: "camera push", key: "camera_movement", value: "push"},
		{name: "camera pull", key: "camera_movement", value: "pull"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := generation.CreateRequest{Type: "TEXT_TO_VIDEO", Params: map[string]any{tt.key: tt.value}}
			if err := validateVideoGenerationRequest(&request, videoValidationTestResolved(capabilities)); err != nil {
				t.Fatalf("supported parameter was rejected: %v", err)
			}
			if got := request.Params[tt.key]; got != tt.value {
				t.Fatalf("normalized %s = %#v, want %#v", tt.key, got, tt.value)
			}
		})
	}
}

func TestApplyVideoCapabilitiesToSchemaHidesProviderUnsupportedParameters(t *testing.T) {
	capabilities := videoValidationTestCapabilities()
	capabilities.SupportedParameters = append(capabilities.SupportedParameters, "generate_audio")
	schema := videoValidationTestResolved(capabilities).Schema.SchemaJSON
	filtered := applyVideoCapabilitiesToSchema(schema, capabilities)
	keys := map[string]bool{}
	for _, field := range filtered.Fields {
		keys[field.Key] = true
	}
	for _, key := range []string{"duration", "resolution", "aspect_ratio", "generate_audio"} {
		if !keys[key] {
			t.Fatalf("supported field %s was hidden: %+v", key, keys)
		}
	}
	for _, key := range []string{"fps", "motion_strength", "camera_movement"} {
		if keys[key] {
			t.Fatalf("unsupported field %s was advertised: %+v", key, keys)
		}
	}
}

func TestResolveModuleSchemaIntersectsSchemaWithRealProviderProtocol(t *testing.T) {
	data := seedAdminData()
	for index := range data.AIModels {
		if data.AIModels[index].ModelName == "doubao-seedance-2.0" {
			data.AIModels[index].ChannelID = "channel_cmecloud_seedance"
		}
	}
	for index := range data.APIChannels {
		if data.APIChannels[index].ID == "channel_cmecloud_seedance" {
			data.APIChannels[index].Status = "ACTIVE"
		}
	}
	data.APIKeys = append(data.APIKeys, adminAPIKey{
		ID: "video-contract-key", Customer: "channel_cmecloud_seedance", Secret: "sk-test", Status: "ACTIVE",
	})
	for _, schema := range data.AIParameterSchemas {
		if schema.ModuleCode != moduleVideoGeneration {
			continue
		}
		schema.ID = "schema_video_doubao_contract"
		schema.ModelName = "doubao-seedance-2.0"
		data.AIParameterSchemas = append(data.AIParameterSchemas, schema)
		break
	}

	resolved, err := resolveModuleSchema(data, adminUser{
		ID: "user_000002", Role: "MEMBER", PlanID: "plan_month",
	}, moduleVideoGeneration, "doubao-seedance-2.0")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Model.VideoCapabilities == nil {
		t.Fatal("video capabilities missing")
	}
	if !videoParameterSupported(resolved.Model.VideoCapabilities.SupportedParameters, "generate_audio") {
		t.Fatalf("Seedance content audio capability missing: %+v", resolved.Model.VideoCapabilities.SupportedParameters)
	}
	for _, key := range []string{"fps", "motion_strength", "camera_movement"} {
		if videoParameterSupported(resolved.Model.VideoCapabilities.SupportedParameters, key) {
			t.Fatalf("unsupported %s advertised by Seedance content protocol", key)
		}
	}
}

func TestLegacyVideoSchemaWritesCanonicalAspectRatioToTaskParams(t *testing.T) {
	resolved := videoValidationTestResolved(videoValidationTestCapabilities())
	resolved.Schema.SchemaJSON.Fields = []adminAIParameterField{
		{Key: "ratio", Type: "select", Default: "9:16", Options: []any{"16:9", "9:16"}},
	}
	resolved.FinalSchema = applyVideoCapabilitiesToSchema(resolved.Schema.SchemaJSON, videoValidationTestCapabilities())
	req := generation.CreateRequest{ModuleCode: moduleVideoGeneration, Params: map[string]any{}}
	if err := validateGenerationParams(req, resolved); err != nil {
		t.Fatalf("legacy video schema validation failed: %v", err)
	}
	if req.Params["aspect_ratio"] != "9:16" {
		t.Fatalf("canonical aspect_ratio default = %#v", req.Params["aspect_ratio"])
	}
	if _, exists := req.Params["ratio"]; exists {
		t.Fatalf("legacy ratio leaked from schema into task params: %+v", req.Params)
	}
}

func TestValidateGenerationParamsDoesNotWriteUnsupportedProviderDefaults(t *testing.T) {
	capabilities := videoValidationTestCapabilities()
	resolved := videoValidationTestResolved(capabilities)
	resolved.FinalSchema = applyVideoCapabilitiesToSchema(resolved.Schema.SchemaJSON, capabilities)
	req := generation.CreateRequest{ModuleCode: moduleVideoGeneration, Params: map[string]any{
		"duration": float64(5), "resolution": "720p", "aspect_ratio": "16:9",
	}}
	if err := validateGenerationParams(req, resolved); err != nil {
		t.Fatalf("video params validation failed: %v", err)
	}
	for _, key := range []string{"fps", "generate_audio", "motion_strength", "camera_movement"} {
		if _, exists := req.Params[key]; exists {
			t.Fatalf("unsupported default %s leaked into task params: %+v", key, req.Params)
		}
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

func TestGrokImagine15VideoLegacyDataResolvesExactCapabilities(t *testing.T) {
	model := adminAIModel{
		ModelName: "grok-imagine-1.5-video", ModelType: "video", ModuleCode: moduleVideoGeneration,
		CapabilityCode: []string{"text_to_video", "image_to_video"},
	}
	got := resolveVideoModelCapabilities(model, adminAIParameterSchemaJSON{})
	if !got.SupportsTextToVideo || !got.SupportsImageToVideo || !got.SupportsFirstFrame || got.SupportsLastFrame {
		t.Fatalf("unexpected Grok Imagine 1.5 modes: %+v", got)
	}
	if got.MaxReferenceImages != 7 {
		t.Fatalf("max references = %d, want 7", got.MaxReferenceImages)
	}
	if len(got.SupportedDurations) != 25 || got.SupportedDurations[0] != 6 || got.SupportedDurations[24] != 30 {
		t.Fatalf("supported durations = %#v", got.SupportedDurations)
	}
	wantResolutions := []string{"480p", "720p"}
	if !equalVideoStringSlices(got.SupportedResolutions, wantResolutions) {
		t.Fatalf("supported resolutions = %#v, want %#v", got.SupportedResolutions, wantResolutions)
	}
	wantRatios := []string{"16:9", "9:16", "1:1", "3:2", "2:3"}
	if !equalVideoStringSlices(got.SupportedAspectRatios, wantRatios) {
		t.Fatalf("supported ratios = %#v, want %#v", got.SupportedAspectRatios, wantRatios)
	}
}

func TestGrokImaginePreviewLegacyDataRemainsSingleImageOnly(t *testing.T) {
	model := adminAIModel{
		ModelName: "grok-imagine-video-1.5-preview", ModelType: "video", ModuleCode: moduleVideoGeneration,
		CapabilityCode: []string{"image_to_video"},
	}
	got := resolveVideoModelCapabilities(model, adminAIParameterSchemaJSON{})
	if got.SupportsTextToVideo || !got.SupportsImageToVideo || got.MaxReferenceImages != 1 {
		t.Fatalf("preview capabilities changed: %+v", got)
	}
	wantDurations := []int{10, 15}
	if len(got.SupportedDurations) != len(wantDurations) {
		t.Fatalf("supported durations = %#v, want %#v", got.SupportedDurations, wantDurations)
	}
	for index := range wantDurations {
		if got.SupportedDurations[index] != wantDurations[index] {
			t.Fatalf("supported durations = %#v, want %#v", got.SupportedDurations, wantDurations)
		}
	}
	wantResolutions := []string{"480p", "720p"}
	if !equalVideoStringSlices(got.SupportedResolutions, wantResolutions) {
		t.Fatalf("supported resolutions = %#v, want %#v", got.SupportedResolutions, wantResolutions)
	}
	wantRatios := []string{"16:9", "9:16"}
	if !equalVideoStringSlices(got.SupportedAspectRatios, wantRatios) {
		t.Fatalf("supported ratios = %#v, want %#v", got.SupportedAspectRatios, wantRatios)
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

func TestVideoGenerationPreservesSevenCanonicalReferenceImages(t *testing.T) {
	capabilities := videoValidationTestCapabilities()
	capabilities.MaxReferenceImages = 7
	images := []any{
		"https://example.test/1.png", "https://example.test/2.png", "https://example.test/3.png",
		"https://example.test/4.png", "https://example.test/5.png", "https://example.test/6.png",
		"https://example.test/7.png",
	}
	request := generation.CreateRequest{Type: "IMAGE_TO_VIDEO", Params: map[string]any{"image_urls": images}}
	if err := validateVideoGenerationRequest(&request, videoValidationTestResolved(capabilities)); err != nil {
		t.Fatalf("seven references were rejected: %v", err)
	}
	if request.Params["first_frame"] != images[0] {
		t.Fatalf("first_frame = %#v, want %#v", request.Params["first_frame"], images[0])
	}
	got, ok := request.Params["image_urls"].([]string)
	if !ok || len(got) != 7 {
		t.Fatalf("canonical image_urls = %#v", request.Params["image_urls"])
	}
}

func TestVideoGenerationRejectsEighthReferenceImage(t *testing.T) {
	capabilities := videoValidationTestCapabilities()
	capabilities.MaxReferenceImages = 7
	images := make([]any, 0, 8)
	for index := 1; index <= 8; index++ {
		images = append(images, fmt.Sprintf("https://example.test/%d.png", index))
	}
	request := generation.CreateRequest{Type: "IMAGE_TO_VIDEO", Params: map[string]any{"image_urls": images}}
	err := validateVideoGenerationRequest(&request, videoValidationTestResolved(capabilities))
	requireVideoValidationCode(t, err, "VIDEO_IMAGE_LIMIT_EXCEEDED")
}

func equalVideoStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
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
			"duration": float64(12),
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
