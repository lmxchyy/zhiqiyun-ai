package httpserver

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"xianzhi-ai/backend-go/internal/app/generation"
)

const (
	videoModeText           = "TEXT_TO_VIDEO"
	videoModeImage          = "IMAGE_TO_VIDEO"
	maxVideoReferenceImages = 7
)

var videoCoreParameters = []string{"duration", "resolution", "aspect_ratio"}
var videoOptionalProviderParameters = []string{"fps", "generate_audio", "motion_strength", "camera_movement"}

type videoGenerationValidationError struct {
	code    string
	message string
}

func (e *videoGenerationValidationError) Error() string {
	return e.message
}

func (e *videoGenerationValidationError) BusinessCode() string {
	return e.code
}

func newVideoGenerationValidationError(code, message string) error {
	return &videoGenerationValidationError{code: code, message: message}
}

func safeVideoModelCapabilities() adminVideoModelCapabilities {
	return adminVideoModelCapabilities{
		SupportsTextToVideo: true,
		SupportedParameters: append([]string(nil), videoCoreParameters...),
	}
}

func legacyVideoModelCapabilities(model adminAIModel) adminVideoModelCapabilities {
	switch strings.ToLower(strings.TrimSpace(model.ModelName)) {
	case "grok-imagine-1.5-video":
		return grokImagine15VideoCapabilities()
	case "grok-imagine-video-1.5-preview":
		return grokImagine15VideoPreviewCapabilities()
	}
	capabilities := safeVideoModelCapabilities()
	hasExplicitVideoCode := false
	for _, capability := range model.CapabilityCode {
		switch strings.ToLower(strings.TrimSpace(capability)) {
		case "text_to_video":
			hasExplicitVideoCode = true
		case "image_to_video":
			hasExplicitVideoCode = true
			capabilities.SupportsImageToVideo = true
			capabilities.SupportsFirstFrame = true
			capabilities.MaxReferenceImages = 1
		}
	}
	if !hasExplicitVideoCode {
		return safeVideoModelCapabilities()
	}
	return capabilities
}

func grokImagine15VideoCapabilities() adminVideoModelCapabilities {
	durations := make([]int, 0, 25)
	for seconds := 6; seconds <= 30; seconds++ {
		durations = append(durations, seconds)
	}
	return adminVideoModelCapabilities{
		SupportsTextToVideo:   true,
		SupportsImageToVideo:  true,
		SupportsFirstFrame:    true,
		MaxReferenceImages:    7,
		SupportedDurations:    durations,
		SupportedResolutions:  []string{"480p", "720p"},
		SupportedAspectRatios: []string{"16:9", "9:16", "1:1", "3:2", "2:3"},
		SupportedParameters:   append([]string(nil), videoCoreParameters...),
	}
}

func grokImagine15VideoCapabilitiesPtr() *adminVideoModelCapabilities {
	capabilities := grokImagine15VideoCapabilities()
	return &capabilities
}

func grokImagine15VideoPreviewCapabilities() adminVideoModelCapabilities {
	return adminVideoModelCapabilities{
		SupportsTextToVideo:   false,
		SupportsImageToVideo:  true,
		SupportsFirstFrame:    true,
		MaxReferenceImages:    1,
		SupportedDurations:    []int{10, 15},
		SupportedResolutions:  []string{"480p", "720p"},
		SupportedAspectRatios: []string{"16:9", "9:16"},
		SupportedParameters:   append([]string(nil), videoCoreParameters...),
	}
}

func grokImagine15VideoPreviewCapabilitiesPtr() *adminVideoModelCapabilities {
	capabilities := grokImagine15VideoPreviewCapabilities()
	return &capabilities
}

func normalizeVideoModelCapabilities(capabilities adminVideoModelCapabilities) adminVideoModelCapabilities {
	capabilities.SupportedDurations = uniquePositiveInts(capabilities.SupportedDurations)
	capabilities.SupportedResolutions = uniqueTrimmedStrings(capabilities.SupportedResolutions)
	capabilities.SupportedAspectRatios = uniqueTrimmedStrings(capabilities.SupportedAspectRatios)
	capabilities.SupportedParameters = uniqueTrimmedStrings(capabilities.SupportedParameters)
	if len(capabilities.SupportedParameters) == 0 {
		capabilities.SupportedParameters = append([]string(nil), videoCoreParameters...)
	}
	if !capabilities.SupportsImageToVideo {
		capabilities.SupportsFirstFrame = false
		capabilities.SupportsLastFrame = false
		capabilities.MaxReferenceImages = 0
		return capabilities
	}
	capabilities.SupportsFirstFrame = true
	if capabilities.MaxReferenceImages < 1 {
		capabilities.MaxReferenceImages = 1
	}
	if capabilities.SupportsLastFrame && capabilities.MaxReferenceImages < 2 {
		capabilities.MaxReferenceImages = 2
	}
	if capabilities.MaxReferenceImages > maxVideoReferenceImages {
		capabilities.MaxReferenceImages = maxVideoReferenceImages
	}
	return capabilities
}

func resolveVideoModelCapabilities(model adminAIModel, schema adminAIParameterSchemaJSON) adminVideoModelCapabilities {
	var capabilities adminVideoModelCapabilities
	if model.VideoCapabilities == nil {
		capabilities = legacyVideoModelCapabilities(model)
	} else {
		capabilities = normalizeVideoModelCapabilities(*model.VideoCapabilities)
	}
	if len(capabilities.SupportedDurations) == 0 {
		capabilities.SupportedDurations = videoSchemaDurationOptions(schema)
	}
	if len(capabilities.SupportedResolutions) == 0 {
		capabilities.SupportedResolutions = videoSchemaStringOptions(schema, "resolution")
	}
	if len(capabilities.SupportedAspectRatios) == 0 {
		capabilities.SupportedAspectRatios = videoSchemaStringOptions(schema, "ratio", "aspect_ratio")
	}
	return capabilities
}

func uniqueTrimmedStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func uniquePositiveInts(values []int) []int {
	result := make([]int, 0, len(values))
	seen := map[int]struct{}{}
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func videoSchemaDurationOptions(schema adminAIParameterSchemaJSON) []int {
	values := videoSchemaStringOptions(schema, "duration")
	result := make([]int, 0, len(values))
	for _, value := range values {
		seconds, err := strconv.Atoi(strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), "s"))
		if err == nil && seconds > 0 {
			result = append(result, seconds)
		}
	}
	return uniquePositiveInts(result)
}

func videoSchemaStringOptions(schema adminAIParameterSchemaJSON, keys ...string) []string {
	wanted := map[string]struct{}{}
	for _, key := range keys {
		wanted[strings.ToLower(strings.TrimSpace(key))] = struct{}{}
	}
	for _, field := range schema.Fields {
		if _, ok := wanted[strings.ToLower(strings.TrimSpace(field.Key))]; !ok {
			continue
		}
		values := make([]string, 0, len(field.Options))
		for _, option := range field.Options {
			value := strings.TrimSpace(fmt.Sprint(option))
			if value != "" && value != "<nil>" {
				values = append(values, value)
			}
		}
		return uniqueTrimmedStrings(values)
	}
	return nil
}

func normalizeVideoModelCapabilityData(data adminPlatformData) adminPlatformData {
	for index := range data.AIModels {
		model := &data.AIModels[index]
		if !isVideoAIModel(*model) {
			continue
		}
		if model.VideoCapabilities == nil {
			defaults := legacyVideoModelCapabilities(*model)
			model.VideoCapabilities = &defaults
		} else {
			normalized := normalizeVideoModelCapabilities(*model.VideoCapabilities)
			model.VideoCapabilities = &normalized
		}
		model.CapabilityCode = syncVideoCapabilityCodes(model.CapabilityCode, *model.VideoCapabilities)
		model.CapabilityCodeCamel = append([]string(nil), model.CapabilityCode...)
	}
	return data
}

func isVideoAIModel(model adminAIModel) bool {
	if strings.EqualFold(strings.TrimSpace(model.ModuleCode), "video") ||
		strings.EqualFold(strings.TrimSpace(model.ModelType), "video") {
		return true
	}
	for _, capability := range model.CapabilityCode {
		switch strings.ToLower(strings.TrimSpace(capability)) {
		case "text_to_video", "image_to_video":
			return true
		}
	}
	return false
}

func syncVideoCapabilityCodes(codes []string, capabilities adminVideoModelCapabilities) []string {
	result := make([]string, 0, len(codes)+2)
	for _, code := range codes {
		switch strings.ToLower(strings.TrimSpace(code)) {
		case "text_to_video", "image_to_video":
			continue
		}
		if strings.TrimSpace(code) != "" {
			result = append(result, code)
		}
	}
	if capabilities.SupportsTextToVideo {
		result = append(result, "text_to_video")
	}
	if capabilities.SupportsImageToVideo {
		result = append(result, "image_to_video")
	}
	return uniqueTrimmedStrings(result)
}

func applyAIModelVideoCapabilitiesMutation(model *adminAIModel, mutation adminAIModelMutation) error {
	if model == nil {
		return errors.New("模型不能为空")
	}
	if !isVideoAIModel(*model) && mutation.VideoCapabilities == nil {
		model.VideoCapabilities = nil
		return nil
	}
	capabilities := legacyVideoModelCapabilities(*model)
	if model.VideoCapabilities != nil {
		capabilities = *model.VideoCapabilities
	}
	if mutation.VideoCapabilities != nil {
		capabilities = *mutation.VideoCapabilities
	}
	capabilities = normalizeVideoModelCapabilities(capabilities)
	if !capabilities.SupportsTextToVideo && !capabilities.SupportsImageToVideo {
		return newVideoGenerationValidationError("VIDEO_CAPABILITY_INVALID", "视频模型至少需要支持文生视频或图生视频")
	}
	if capabilities.SupportsLastFrame && !capabilities.SupportsImageToVideo {
		return newVideoGenerationValidationError("VIDEO_CAPABILITY_INVALID", "支持尾帧的模型必须同时支持图生视频")
	}
	model.VideoCapabilities = &capabilities
	model.CapabilityCode = syncVideoCapabilityCodes(model.CapabilityCode, capabilities)
	model.CapabilityCodeCamel = append([]string(nil), model.CapabilityCode...)
	return nil
}

func applyVideoCapabilitiesToSchema(schema adminAIParameterSchemaJSON, capabilities adminVideoModelCapabilities) adminAIParameterSchemaJSON {
	filtered := make([]adminAIParameterField, 0, len(schema.Fields))
	for _, field := range schema.Fields {
		switch strings.ToLower(strings.TrimSpace(field.Key)) {
		case "reference_image", "reference_images", "image", "image_url", "image_urls":
			continue
		case "first_frame":
			if !capabilities.SupportsImageToVideo || !capabilities.SupportsFirstFrame {
				continue
			}
			field.Label = "首帧图"
			field.Required = true
		case "last_frame":
			if !capabilities.SupportsLastFrame {
				continue
			}
			field.Label = "尾帧图"
		case "duration":
			if len(capabilities.SupportedDurations) > 0 {
				field.Options = intOptionsToAny(capabilities.SupportedDurations)
			}
		case "resolution":
			if len(capabilities.SupportedResolutions) > 0 {
				field.Options = stringOptionsToAny(capabilities.SupportedResolutions)
			}
		case "ratio", "aspect_ratio":
			field.Key = "aspect_ratio"
			if len(capabilities.SupportedAspectRatios) > 0 {
				field.Options = stringOptionsToAny(capabilities.SupportedAspectRatios)
			}
		case "fps", "generate_audio", "motion_strength", "camera_movement":
			if !videoParameterSupported(capabilities.SupportedParameters, field.Key) {
				continue
			}
		}
		filtered = append(filtered, field)
	}
	schema.Fields = filtered
	return schema
}

func videoParameterSupported(supported []string, key string) bool {
	for _, item := range supported {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(key)) {
			return true
		}
	}
	return false
}

func stringOptionsToAny(values []string) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func intOptionsToAny(values []int) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

var videoImageParameterKeys = []string{
	"first_frame",
	"last_frame",
	"reference_image",
	"reference_images",
	"referenceImage",
	"referenceImages",
	"image",
	"image_url",
	"image_urls",
	"imageUrl",
	"imageUrls",
	"images",
	"input_image",
	"input_images",
	"inputImageUrl",
	"inputImageUrls",
	"inputImages",
	"inputImagesSnapshot",
}

func validateVideoGenerationRequest(req *generation.CreateRequest, resolved resolvedModuleSchema) error {
	if req == nil {
		return newVideoGenerationValidationError("VIDEO_REQUEST_INVALID", "视频生成请求不能为空")
	}
	mode := strings.ToUpper(strings.TrimSpace(req.Type))
	if mode != videoModeText && mode != videoModeImage {
		return newVideoGenerationValidationError("VIDEO_MODE_INVALID", "视频生成模式无效，请选择文生视频或图生视频")
	}
	capabilities := resolveVideoModelCapabilities(resolved.Model, resolved.Schema.SchemaJSON)
	images := collectVideoImageParameters(req.Params)
	firstFrame := strings.TrimSpace(parameterString(req.Params, "first_frame"))
	lastFrame := strings.TrimSpace(parameterString(req.Params, "last_frame"))

	if mode == videoModeText {
		if len(images) > 0 {
			return newVideoGenerationValidationError("VIDEO_TEXT_MODE_IMAGE_FORBIDDEN", "文生视频模式不得携带首帧图、尾帧图或其他图片字段")
		}
		if !capabilities.SupportsTextToVideo {
			return newVideoGenerationValidationError("VIDEO_MODE_NOT_SUPPORTED", "所选模型不支持文生视频")
		}
	} else {
		if !capabilities.SupportsImageToVideo {
			return newVideoGenerationValidationError("VIDEO_MODE_NOT_SUPPORTED", "所选模型不支持图生视频")
		}
		if lastFrame != "" && !capabilities.SupportsLastFrame {
			return newVideoGenerationValidationError("VIDEO_LAST_FRAME_NOT_SUPPORTED", "所选模型不支持尾帧图")
		}
		legacyImages := legacyVideoImageValues(req.Params)
		if firstFrame == "" && len(legacyImages) > 0 {
			firstFrame = legacyImages[0]
			if req.Params == nil {
				req.Params = map[string]any{}
			}
			req.Params["first_frame"] = firstFrame
		}
		if firstFrame == "" {
			return newVideoGenerationValidationError("VIDEO_FIRST_FRAME_REQUIRED", "图生视频必须上传首帧图")
		}
		if capabilities.MaxReferenceImages <= 1 {
			for _, legacyImage := range legacyImages {
				if legacyImage != firstFrame && legacyImage != lastFrame {
					return newVideoGenerationValidationError("VIDEO_IMAGE_LIMIT_EXCEEDED", "所选模型最多支持 1 张视频输入图片")
				}
			}
		}
		if len(images) > capabilities.MaxReferenceImages {
			return newVideoGenerationValidationError("VIDEO_IMAGE_LIMIT_EXCEEDED", fmt.Sprintf("所选模型最多支持 %d 张视频输入图片", capabilities.MaxReferenceImages))
		}
		clearLegacyVideoImageParameters(req.Params)
		if capabilities.MaxReferenceImages > 1 {
			req.Params["image_urls"] = uniqueImageValues(append([]string{firstFrame}, legacyImages...))
		}
	}

	if err := validateVideoDurationOption(req.Params, capabilities.SupportedDurations); err != nil {
		return err
	}
	if err := validateVideoParameterOption(req.Params, "resolution", capabilities.SupportedResolutions, "VIDEO_RESOLUTION_NOT_SUPPORTED", "所选模型不支持该视频分辨率"); err != nil {
		return err
	}
	if err := validateVideoParameterOption(req.Params, "ratio", capabilities.SupportedAspectRatios, "VIDEO_ASPECT_RATIO_NOT_SUPPORTED", "所选模型不支持该视频比例"); err != nil {
		return err
	}
	if err := validateVideoParameterOption(req.Params, "aspect_ratio", capabilities.SupportedAspectRatios, "VIDEO_ASPECT_RATIO_NOT_SUPPORTED", "所选模型不支持该视频比例"); err != nil {
		return err
	}
	// Drop optional provider params the selected upstream protocol cannot forward.
	// Clients may still send stale toggles (for example generate_audio) after a
	// channel/protocol change; rejecting the whole task after the user clicked
	// generate is worse than silently ignoring unsupported extras.
	for _, key := range videoOptionalProviderParameters {
		if _, exists := req.Params[key]; exists && !videoParameterSupported(capabilities.SupportedParameters, key) {
			delete(req.Params, key)
		}
	}
	delete(req.Params, "generateAudio")
	return nil
}

func clearLegacyVideoImageParameters(parameters map[string]any) {
	for _, key := range videoImageParameterKeys {
		if key == "first_frame" || key == "last_frame" {
			continue
		}
		delete(parameters, key)
	}
}

func parameterString(parameters map[string]any, key string) string {
	if parameters == nil {
		return ""
	}
	value, ok := parameters[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func collectVideoImageParameters(parameters map[string]any) []string {
	values := make([]string, 0, 2)
	for _, key := range videoImageParameterKeys {
		values = append(values, videoParameterValues(parameters, key)...)
	}
	return uniqueImageValues(values)
}

func legacyVideoImageValues(parameters map[string]any) []string {
	values := make([]string, 0, 2)
	for _, key := range videoImageParameterKeys {
		if key == "first_frame" || key == "last_frame" {
			continue
		}
		values = append(values, videoParameterValues(parameters, key)...)
	}
	return uniqueImageValues(values)
}

func videoParameterValues(parameters map[string]any, key string) []string {
	if parameters == nil {
		return nil
	}
	value, ok := parameters[key]
	if !ok || value == nil {
		return nil
	}
	result := []string{}
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) != "" {
			result = append(result, typed)
		}
	case []string:
		result = append(result, typed...)
	case []any:
		for _, item := range typed {
			result = append(result, videoImageValuesFromAny(item)...)
		}
	case map[string]any:
		result = append(result, videoImageValuesFromAny(typed)...)
	default:
		result = append(result, fmt.Sprint(value))
	}
	return result
}

func videoImageValuesFromAny(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case map[string]any:
		for _, key := range []string{"url", "image_url", "imageUrl", "src", "sourceUrl", "remoteUrl", "fileUrl"} {
			if result := strings.TrimSpace(fmt.Sprint(typed[key])); result != "" && result != "<nil>" {
				return []string{result}
			}
		}
	case []any:
		result := []string{}
		for _, item := range typed {
			result = append(result, videoImageValuesFromAny(item)...)
		}
		return result
	}
	return nil
}

func uniqueImageValues(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := value
		if parsed, err := url.Parse(value); err == nil {
			key = parsed.String()
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func validateVideoParameterOption(parameters map[string]any, key string, supported []string, code, message string) error {
	value := parameterString(parameters, key)
	if value == "" || len(supported) == 0 {
		return nil
	}
	for _, candidate := range supported {
		if strings.EqualFold(strings.TrimSpace(candidate), value) {
			return nil
		}
	}
	allowed := append([]string(nil), supported...)
	sort.Strings(allowed)
	return newVideoGenerationValidationError(code, fmt.Sprintf("%s，可选值：%s", message, strings.Join(allowed, "、")))
}

func validateVideoDurationOption(parameters map[string]any, supported []int) error {
	value := parameterString(parameters, "duration")
	if value == "" || len(supported) == 0 {
		return nil
	}
	seconds, err := strconv.Atoi(strings.TrimSuffix(strings.ToLower(value), "s"))
	if err == nil {
		for _, candidate := range supported {
			if candidate == seconds {
				return nil
			}
		}
	}
	allowed := make([]string, 0, len(supported))
	for _, candidate := range supported {
		allowed = append(allowed, strconv.Itoa(candidate))
	}
	return newVideoGenerationValidationError("VIDEO_DURATION_NOT_SUPPORTED", fmt.Sprintf("所选模型不支持该视频时长，可选值：%s", strings.Join(allowed, "、")))
}

func publicModelCapabilities(model adminAIModel, schema adminAIParameterSchemaJSON) []string {
	if isVideoAIModel(model) {
		capabilities := resolveVideoModelCapabilities(model, schema)
		result := []string{}
		if capabilities.SupportsTextToVideo {
			result = append(result, videoModeText)
		}
		if capabilities.SupportsImageToVideo {
			result = append(result, videoModeImage)
		}
		return result
	}
	result := make([]string, 0, len(model.CapabilityCode))
	for _, capability := range model.CapabilityCode {
		capability = strings.ToUpper(strings.TrimSpace(capability))
		if capability != "" {
			result = append(result, capability)
		}
	}
	return result
}
