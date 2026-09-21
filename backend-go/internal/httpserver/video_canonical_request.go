package httpserver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const canonicalVideoRequestVersion = 1

type canonicalVideoRequestError struct {
	Code    string
	Message string
}

func (e *canonicalVideoRequestError) Error() string { return e.Message }

// canonicalVideoCapabilities is deliberately a pure input snapshot. The
// builder does not load capabilities or consult a provider.
type canonicalVideoCapabilities struct {
	SupportedDurations    []int
	SupportedResolutions  []string
	SupportedAspectRatios []string
}

// canonicalVideoStructuredInput is the normalized boundary input accepted by
// the PR-A builder. It is intentionally explicit rather than a Params mirror.
type canonicalVideoStructuredInput struct {
	Model           any
	InputMode       any
	Duration        any
	AspectRatio     any
	Ratio           any
	Resolution      any
	Quality         any
	ReferenceImages any
	FirstFrame      any
	LastFrame       any
	Parameters      map[string]any
}

type canonicalVideoRequestInput struct {
	Prompt       string
	Structured   canonicalVideoStructuredInput
	Capabilities *canonicalVideoCapabilities
}

type canonicalPromptIntentEvidence struct {
	Field           string `json:"field"`
	NormalizedValue any    `json:"normalized_value"`
	SourceKind      string `json:"source_kind"`
	Confidence      string `json:"confidence"`
}

type canonicalPromptIntentHints struct {
	ParserVersion            int                             `json:"parser_version"`
	RequestedDurationSeconds *int                            `json:"requested_duration_seconds,omitempty"`
	RequestedAspectRatio     string                          `json:"requested_aspect_ratio,omitempty"`
	RequestedResolution      string                          `json:"requested_resolution,omitempty"`
	RequestedInputMode       string                          `json:"requested_input_mode,omitempty"`
	ReferenceImageRequested  bool                            `json:"reference_image_requested"`
	RequestedReferenceCount  *int                            `json:"requested_reference_count,omitempty"`
	Evidence                 []canonicalPromptIntentEvidence `json:"evidence"`
}

type canonicalConsistencyWarning struct {
	Code            string   `json:"code"`
	Field           string   `json:"field"`
	StructuredValue any      `json:"structured_value,omitempty"`
	HintedValue     any      `json:"hinted_value,omitempty"`
	Severity        string   `json:"severity"`
	Actions         []string `json:"actions"`
}

type canonicalConsistencyResult struct {
	Status   string                        `json:"status"`
	Warnings []canonicalConsistencyWarning `json:"warnings"`
}

type canonicalVideoOptionalParameters struct {
	FPS            *int     `json:"fps,omitempty"`
	GenerateAudio  *bool    `json:"generate_audio,omitempty"`
	MotionStrength *float64 `json:"motion_strength,omitempty"`
	CameraMovement string   `json:"camera_movement,omitempty"`
}

type canonicalVideoExecution struct {
	Model           string                           `json:"model"`
	InputMode       string                           `json:"input_mode"`
	DurationSeconds int                              `json:"duration_seconds"`
	AspectRatio     string                           `json:"aspect_ratio"`
	Resolution      string                           `json:"resolution"`
	ReferenceImages []string                         `json:"reference_images"`
	FirstFrame      string                           `json:"first_frame,omitempty"`
	LastFrame       string                           `json:"last_frame,omitempty"`
	Optional        canonicalVideoOptionalParameters `json:"optional_parameters"`
}

type canonicalVideoRequest struct {
	SchemaVersion     int                        `json:"schema_version"`
	Prompt            string                     `json:"prompt"`
	Execution         canonicalVideoExecution    `json:"execution"`
	PromptIntentHints canonicalPromptIntentHints `json:"prompt_intent_hints"`
	ConsistencyResult canonicalConsistencyResult `json:"consistency_result"`
}

var (
	canonicalDurationPattern       = regexp.MustCompile(`(?i)(?:时长|持续|duration|length|生成|视频)?\s*(\d{1,3})\s*(?:秒|s|seconds?)`)
	canonicalTimelineRangePattern  = regexp.MustCompile(`(?i)(?:第\s*)?\d{1,3}\s*(?:秒|s)?\s*[-–—~～至到]\s*\d{1,3}\s*(?:秒|s)`)
	canonicalClockRangePattern     = regexp.MustCompile(`(?i)\b\d{1,2}:\d{2}(?::\d{2})?\s*[-–—~～至到]\s*\d{1,2}:\d{2}(?::\d{2})?\b`)
	canonicalAspectPattern         = regexp.MustCompile(`(?i)(?:比例|画幅|aspect\s*ratio|ratio)?\s*(\d{1,2})\s*:\s*(\d{1,2})`)
	canonicalResolutionPattern     = regexp.MustCompile(`(?i)\b(\d{3,4}\s*p|[248]\s*k)\b`)
	canonicalReferencePattern      = regexp.MustCompile(`(?i)(参考(?:图|图片|素材)|根据(?:我?上传|提供|这|该)?(?:的)?(?:图片|图像|照片)|(?:第\s*)?(?:一|二|三|1|2|3)\s*(?:张)?\s*(?:参考图|图片)|\breference\s+images?\b|\breference\s+photos?\b|\binput\s+images?\b|\buploaded\s+images?\b)`)
	canonicalReferenceCountPattern = regexp.MustCompile(`(?i)(\d{1,2})\s*(?:张|个)?\s*(?:参考图|参考图片|图片|图像|reference\s+images?)`)
)

func canonicalText(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func canonicalDuration(value any) (int, bool) {
	raw := strings.TrimSuffix(strings.ToLower(canonicalText(value)), "s")
	if raw == "" {
		return 0, false
	}
	parsed, err := strconv.Atoi(raw)
	return parsed, err == nil && parsed > 0
}

func canonicalAspectRatio(value any) (string, bool) {
	match := regexp.MustCompile(`^(\d{1,2})\s*:\s*(\d{1,2})$`).FindStringSubmatch(canonicalText(value))
	if len(match) != 3 {
		return "", false
	}
	left, _ := strconv.Atoi(match[1])
	right, _ := strconv.Atoi(match[2])
	if left <= 0 || right <= 0 {
		return "", false
	}
	return fmt.Sprintf("%d:%d", left, right), true
}

func canonicalResolution(value any) (string, bool) {
	normalized := strings.ToLower(strings.ReplaceAll(canonicalText(value), " ", ""))
	if !canonicalResolutionPattern.MatchString(normalized) || !regexp.MustCompile(`^(\d{3,4}p|[248]k)$`).MatchString(normalized) {
		return "", false
	}
	return normalized, true
}

func canonicalInputMode(value any) (string, bool) {
	mode := strings.ToUpper(strings.ReplaceAll(canonicalText(value), "-", "_"))
	switch mode {
	case "TEXT", "TEXT_TO_VIDEO":
		return "TEXT_TO_VIDEO", true
	case "IMAGE", "IMAGE_TO_VIDEO":
		return "IMAGE_TO_VIDEO", true
	default:
		return "", false
	}
}

func canonicalStringList(value any) []string {
	values := []string{}
	switch typed := value.(type) {
	case string:
		values = append(values, typed)
	case []string:
		values = append(values, typed...)
	case []any:
		for _, item := range typed {
			values = append(values, canonicalText(item))
		}
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = canonicalText(value)
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

func canonicalDurationHints(prompt string) []int {
	withoutTimeline := canonicalTimelineRangePattern.ReplaceAllString(prompt, " ")
	withoutTimeline = canonicalClockRangePattern.ReplaceAllString(withoutTimeline, " ")
	values := []int{}
	seen := map[int]struct{}{}
	for _, match := range canonicalDurationPattern.FindAllStringSubmatch(withoutTimeline, -1) {
		if len(match) < 2 {
			continue
		}
		value, err := strconv.Atoi(match[1])
		if err != nil || value <= 0 || value > 180 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	return values
}

func canonicalPromptIntent(prompt string) canonicalPromptIntentHints {
	hints := canonicalPromptIntentHints{ParserVersion: 1, Evidence: []canonicalPromptIntentEvidence{}}
	durations := canonicalDurationHints(prompt)
	if len(durations) > 0 {
		hints.RequestedDurationSeconds = &durations[0]
		hints.Evidence = append(hints.Evidence, canonicalPromptIntentEvidence{Field: "duration", NormalizedValue: durations[0], SourceKind: "explicit_text", Confidence: "high"})
	}
	for _, match := range canonicalAspectPattern.FindAllStringSubmatch(prompt, -1) {
		if len(match) < 3 {
			continue
		}
		if ratio, ok := canonicalAspectRatio(match[1] + ":" + match[2]); ok {
			hints.RequestedAspectRatio = ratio
			hints.Evidence = append(hints.Evidence, canonicalPromptIntentEvidence{Field: "aspect_ratio", NormalizedValue: ratio, SourceKind: "explicit_text", Confidence: "high"})
			break
		}
	}
	for _, match := range canonicalResolutionPattern.FindAllStringSubmatch(prompt, -1) {
		if len(match) < 2 {
			continue
		}
		if resolution, ok := canonicalResolution(match[1]); ok {
			hints.RequestedResolution = resolution
			hints.Evidence = append(hints.Evidence, canonicalPromptIntentEvidence{Field: "resolution", NormalizedValue: resolution, SourceKind: "explicit_text", Confidence: "high"})
			break
		}
	}
	hints.ReferenceImageRequested = canonicalReferencePattern.MatchString(prompt) || canonicalReferenceCountPattern.MatchString(prompt)
	if hints.ReferenceImageRequested {
		hints.RequestedInputMode = "IMAGE_TO_VIDEO"
		value := any(true)
		if match := canonicalReferenceCountPattern.FindStringSubmatch(prompt); len(match) >= 2 {
			if count, err := strconv.Atoi(match[1]); err == nil && count > 0 {
				hints.RequestedReferenceCount = &count
				value = count
			}
		}
		hints.Evidence = append(hints.Evidence,
			canonicalPromptIntentEvidence{Field: "reference_images", NormalizedValue: value, SourceKind: "explicit_text", Confidence: "high"},
			canonicalPromptIntentEvidence{Field: "input_mode", NormalizedValue: "IMAGE_TO_VIDEO", SourceKind: "explicit_text", Confidence: "high"},
		)
	}
	return hints
}

func canonicalWarning(code, field string, structuredValue, hintedValue any) canonicalConsistencyWarning {
	return canonicalConsistencyWarning{Code: code, Field: field, StructuredValue: structuredValue, HintedValue: hintedValue, Severity: "warning", Actions: []string{"use_structured", "apply_hint", "edit_prompt"}}
}

func canonicalConsistency(execution canonicalVideoExecution, hints canonicalPromptIntentHints) canonicalConsistencyResult {
	warnings := []canonicalConsistencyWarning{}
	if hints.RequestedDurationSeconds != nil && *hints.RequestedDurationSeconds != execution.DurationSeconds {
		warnings = append(warnings, canonicalWarning("VIDEO_PROMPT_DURATION_MISMATCH", "duration", execution.DurationSeconds, *hints.RequestedDurationSeconds))
	}
	if hints.RequestedAspectRatio != "" && hints.RequestedAspectRatio != execution.AspectRatio {
		warnings = append(warnings, canonicalWarning("VIDEO_PROMPT_ASPECT_RATIO_MISMATCH", "aspect_ratio", execution.AspectRatio, hints.RequestedAspectRatio))
	}
	if hints.RequestedResolution != "" && hints.RequestedResolution != execution.Resolution {
		warnings = append(warnings, canonicalWarning("VIDEO_PROMPT_RESOLUTION_MISMATCH", "resolution", execution.Resolution, hints.RequestedResolution))
	}
	if hints.RequestedInputMode != "" && hints.RequestedInputMode != execution.InputMode {
		warnings = append(warnings, canonicalWarning("VIDEO_PROMPT_MODE_MISMATCH", "input_mode", execution.InputMode, hints.RequestedInputMode))
	}
	if hints.ReferenceImageRequested && len(execution.ReferenceImages) == 0 && execution.FirstFrame == "" {
		requested := any(true)
		if hints.RequestedReferenceCount != nil {
			requested = *hints.RequestedReferenceCount
		}
		warnings = append(warnings, canonicalWarning("VIDEO_PROMPT_REFERENCE_MISSING", "reference_images", []string{}, requested))
	}
	status := "ok"
	if len(warnings) > 0 {
		status = "warning"
	}
	return canonicalConsistencyResult{Status: status, Warnings: warnings}
}

func canonicalOptionalParameters(parameters map[string]any) (canonicalVideoOptionalParameters, error) {
	result := canonicalVideoOptionalParameters{}
	if parameters == nil {
		return result, nil
	}
	if value, exists := parameters["fps"]; exists && canonicalText(value) != "" {
		parsed, err := strconv.Atoi(canonicalText(value))
		if err != nil || parsed <= 0 {
			return result, &canonicalVideoRequestError{Code: "VIDEO_CANONICAL_INVALID_PARAMETER", Message: "fps is invalid"}
		}
		result.FPS = &parsed
	}
	if value, exists := parameters["generate_audio"]; exists {
		parsed, ok := value.(bool)
		if !ok {
			return result, &canonicalVideoRequestError{Code: "VIDEO_CANONICAL_INVALID_PARAMETER", Message: "generate_audio is invalid"}
		}
		result.GenerateAudio = &parsed
	} else if value, exists := parameters["generateAudio"]; exists {
		parsed, ok := value.(bool)
		if !ok {
			return result, &canonicalVideoRequestError{Code: "VIDEO_CANONICAL_INVALID_PARAMETER", Message: "generate_audio is invalid"}
		}
		result.GenerateAudio = &parsed
	}
	if value, exists := parameters["motion_strength"]; exists && canonicalText(value) != "" {
		parsed, err := strconv.ParseFloat(canonicalText(value), 64)
		if err != nil {
			return result, &canonicalVideoRequestError{Code: "VIDEO_CANONICAL_INVALID_PARAMETER", Message: "motion_strength is invalid"}
		}
		result.MotionStrength = &parsed
	}
	if value, exists := parameters["camera_movement"]; exists {
		result.CameraMovement = canonicalText(value)
	}
	return result, nil
}

func canonicalSupportedDuration(value int, values []int) bool {
	if len(values) == 0 {
		return true
	}
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func canonicalSupportedString(value string, values []string, normalize func(any) (string, bool)) bool {
	if len(values) == 0 {
		return true
	}
	for _, candidate := range values {
		if normalized, ok := normalize(candidate); ok && normalized == value {
			return true
		}
	}
	return false
}

func buildCanonicalVideoRequest(input canonicalVideoRequestInput) (canonicalVideoRequest, error) {
	prompt := strings.TrimSpace(input.Prompt)
	if prompt == "" {
		return canonicalVideoRequest{}, &canonicalVideoRequestError{Code: "VIDEO_CANONICAL_REQUIRED", Message: "prompt is required"}
	}
	model := canonicalText(input.Structured.Model)
	if model == "" {
		return canonicalVideoRequest{}, &canonicalVideoRequestError{Code: "VIDEO_CANONICAL_REQUIRED", Message: "model is required"}
	}
	inputMode, ok := canonicalInputMode(input.Structured.InputMode)
	if !ok {
		return canonicalVideoRequest{}, &canonicalVideoRequestError{Code: "VIDEO_CANONICAL_INVALID_MODE", Message: "input_mode is invalid"}
	}
	duration, ok := canonicalDuration(input.Structured.Duration)
	if !ok {
		return canonicalVideoRequest{}, &canonicalVideoRequestError{Code: "VIDEO_CANONICAL_INVALID_DURATION", Message: "duration is invalid"}
	}
	aspectRatio, ok := canonicalAspectRatio(input.Structured.AspectRatio)
	if !ok {
		aspectRatio, ok = canonicalAspectRatio(input.Structured.Ratio)
	}
	if !ok {
		return canonicalVideoRequest{}, &canonicalVideoRequestError{Code: "VIDEO_CANONICAL_INVALID_ASPECT_RATIO", Message: "aspect_ratio is invalid"}
	}
	resolutionValue := input.Structured.Resolution
	if canonicalText(resolutionValue) == "" {
		resolutionValue = input.Structured.Quality
	}
	resolution, ok := canonicalResolution(resolutionValue)
	if !ok {
		return canonicalVideoRequest{}, &canonicalVideoRequestError{Code: "VIDEO_CANONICAL_INVALID_RESOLUTION", Message: "resolution is invalid"}
	}
	capabilities := input.Capabilities
	if capabilities != nil {
		if !canonicalSupportedDuration(duration, capabilities.SupportedDurations) {
			return canonicalVideoRequest{}, &canonicalVideoRequestError{Code: "VIDEO_CANONICAL_UNSUPPORTED_PARAMETER", Message: "duration is not supported"}
		}
		if !canonicalSupportedString(resolution, capabilities.SupportedResolutions, canonicalResolution) {
			return canonicalVideoRequest{}, &canonicalVideoRequestError{Code: "VIDEO_CANONICAL_UNSUPPORTED_PARAMETER", Message: "resolution is not supported"}
		}
		if !canonicalSupportedString(aspectRatio, capabilities.SupportedAspectRatios, canonicalAspectRatio) {
			return canonicalVideoRequest{}, &canonicalVideoRequestError{Code: "VIDEO_CANONICAL_UNSUPPORTED_PARAMETER", Message: "aspect_ratio is not supported"}
		}
	}
	references := canonicalStringList(input.Structured.ReferenceImages)
	firstFrame := canonicalText(input.Structured.FirstFrame)
	lastFrame := canonicalText(input.Structured.LastFrame)
	if inputMode == "TEXT_TO_VIDEO" && (len(references) > 0 || firstFrame != "" || lastFrame != "") {
		return canonicalVideoRequest{}, &canonicalVideoRequestError{Code: "VIDEO_CANONICAL_TEXT_MODE_REFERENCES", Message: "text-to-video cannot contain references"}
	}
	optional, err := canonicalOptionalParameters(input.Structured.Parameters)
	if err != nil {
		return canonicalVideoRequest{}, err
	}
	execution := canonicalVideoExecution{Model: model, InputMode: inputMode, DurationSeconds: duration, AspectRatio: aspectRatio, Resolution: resolution, ReferenceImages: references, FirstFrame: firstFrame, LastFrame: lastFrame, Optional: optional}
	hints := canonicalPromptIntent(input.Prompt)
	return canonicalVideoRequest{SchemaVersion: canonicalVideoRequestVersion, Prompt: prompt, Execution: execution, PromptIntentHints: hints, ConsistencyResult: canonicalConsistency(execution, hints)}, nil
}

func canonicalVideoRequestRepresentation(request canonicalVideoRequest) ([]byte, error) {
	return json.Marshal(request)
}

// canonicalVideoRequestHash covers provider-relevant content plus execution
// fields only. Prompt intent hints and consistency diagnostics are deliberately
// excluded so warning/UI changes cannot alter the execution identity.
func canonicalVideoRequestHash(request canonicalVideoRequest) (string, error) {
	executionIdentity := struct {
		Prompt    string                  `json:"prompt"`
		Execution canonicalVideoExecution `json:"execution"`
	}{Prompt: request.Prompt, Execution: request.Execution}
	representation, err := json.Marshal(executionIdentity)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(representation)
	return hex.EncodeToString(digest[:]), nil
}
