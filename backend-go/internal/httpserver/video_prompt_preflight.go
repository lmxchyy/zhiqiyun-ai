package httpserver

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
)

const (
	videoPromptDurationMismatchCode = "VIDEO_PROMPT_DURATION_MISMATCH"
	videoPromptReferenceMissingCode = "VIDEO_PROMPT_REFERENCE_MISSING"
	videoPromptModeMismatchCode     = "VIDEO_PROMPT_MODE_MISMATCH"
	videoPromptComplexityCode       = "VIDEO_PROMPT_COMPLEXITY_WARNING"
)

var (
	videoPromptDurationPattern       = regexp.MustCompile(`(?i)(?:时长|持续|duration|length|生成|视频)?\s*(\d{1,3})\s*(?:秒|s|seconds?)`)
	videoPromptReferencePattern      = regexp.MustCompile(`(?i)(参考(?:图|图片|素材)|根据(?:我?上传|提供|这|该)?(?:的)?(?:图片|图像|照片)|(?:第\s*)?(?:一|二|三|1|2|3)\s*(?:张)?\s*(?:参考图|图片)|\breference\s+images?\b|\breference\s+photos?\b|\binput\s+images?\b|\buploaded\s+images?\b)`)
	videoPromptReferenceCountPattern = regexp.MustCompile(`(?i)(?:\d{1,2}|一|二|三|四|五|六|七)\s*(?:张|个)?\s*(?:参考图|参考图片|图片|图像|reference\s+images?)`)
)

type videoPromptPreflightResult struct {
	WarningCodes       []string
	RequestedDurations []int
	ComplexitySignals  []string
}

func inspectVideoPromptPreflight(prompt string, params map[string]any, taskType string) videoPromptPreflightResult {
	prompt = strings.TrimSpace(prompt)
	result := videoPromptPreflightResult{}
	for _, match := range videoPromptDurationPattern.FindAllStringSubmatch(prompt, -1) {
		if len(match) < 2 {
			continue
		}
		var seconds int
		if _, err := fmt.Sscanf(match[1], "%d", &seconds); err == nil && seconds > 0 && seconds <= 180 {
			result.RequestedDurations = append(result.RequestedDurations, seconds)
		}
	}
	result.RequestedDurations = uniquePositiveInts(result.RequestedDurations)

	selectedDuration := parameterInt(params, "duration")
	if selectedDuration > 0 {
		for _, requested := range result.RequestedDurations {
			if requested != selectedDuration {
				result.WarningCodes = append(result.WarningCodes, videoPromptDurationMismatchCode)
				break
			}
		}
	}

	hasReferenceInstruction := videoPromptReferencePattern.MatchString(prompt) || videoPromptReferenceCountPattern.MatchString(prompt)
	referenceCount := len(collectVideoImageParameters(params))
	if hasReferenceInstruction && referenceCount == 0 {
		result.WarningCodes = append(result.WarningCodes, videoPromptReferenceMissingCode)
	}
	if normalizeVideoPromptMode(taskType, params) == videoModeText && hasReferenceInstruction {
		result.WarningCodes = append(result.WarningCodes, videoPromptModeMismatchCode)
	}

	complexityPatterns := []struct {
		signal  string
		pattern *regexp.Regexp
	}{
		{"multi_scene", regexp.MustCompile(`(?i)多场景|多个场景|分场景|multi[-\s]?scene|multiple\s+scenes`)},
		{"multi_shot", regexp.MustCompile(`(?i)多镜头|多个镜头|镜头切换|multi[-\s]?shot|multiple\s+shots|shot\s+list`)},
		{"subtitles", regexp.MustCompile(`(?i)字幕|屏幕文字|标题字卡|subtitles?|on[-\s]?screen\s+text`)},
		{"voiceover", regexp.MustCompile(`(?i)配音|旁白|口播|voice[-\s]?over|narration|voice\s+acting`)},
		{"synchronized_audio", regexp.MustCompile(`(?i)同步音频|同步声音|音画同步|同步配乐|sync(?:hronized)?\s+(?:audio|sound)|lip[-\s]?sync`)},
	}
	for _, item := range complexityPatterns {
		if item.pattern.MatchString(prompt) {
			result.ComplexitySignals = append(result.ComplexitySignals, item.signal)
		}
	}
	if len(result.ComplexitySignals) >= 3 {
		result.WarningCodes = append(result.WarningCodes, videoPromptComplexityCode)
	}
	return videoPromptPreflightResult{
		WarningCodes:       videoPromptUniqueStrings(result.WarningCodes),
		RequestedDurations: result.RequestedDurations,
		ComplexitySignals:  result.ComplexitySignals,
	}
}

func normalizeVideoPromptMode(taskType string, params map[string]any) string {
	mode := strings.ToUpper(strings.TrimSpace(firstNonEmptyString(stringValue(params["inputMode"]), stringValue(params["input_mode"]), taskType)))
	switch mode {
	case "TEXT", "TEXT-TO-VIDEO", "TEXT_TO_VIDEO":
		return videoModeText
	case "IMAGE", "IMAGE-TO-VIDEO", "IMAGE_TO_VIDEO":
		return videoModeImage
	default:
		return mode
	}
}

func parameterInt(params map[string]any, key string) int {
	value := strings.TrimSuffix(strings.ToLower(parameterString(params, key)), "s")
	var result int
	if _, err := fmt.Sscanf(value, "%d", &result); err != nil {
		return 0
	}
	return result
}

func videoPromptHash(prompt string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(prompt)))
	return hex.EncodeToString(digest[:])
}

func videoPromptPreflightTelemetry(task generationTask, reqType, model string, params map[string]any) {
	if !isVideoGenerationType(reqType) {
		return
	}
	codes := videoPromptStringSlice(params["preflight_warning_codes"])
	sort.Strings(codes)
	log.Printf("video_prompt_preflight task_id=%s model=%s duration=%s aspect_ratio=%s input_mode=%s prompt_hash=%s prompt_length=%d preflight_warning_codes=%s", task.ID, model, parameterString(params, "duration"), firstNonEmptyString(parameterString(params, "aspect_ratio"), parameterString(params, "ratio")), firstNonEmptyString(parameterString(params, "inputMode"), parameterString(params, "input_mode"), reqType), stringValue(params["prompt_hash"]), len([]rune(task.Prompt)), strings.Join(codes, ","))
}

func videoPromptStringSlice(value any) []string {
	result := []string{}
	switch typed := value.(type) {
	case []string:
		result = append(result, typed...)
	case []any:
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" && text != "<nil>" {
				result = append(result, text)
			}
		}
	case string:
		if strings.TrimSpace(typed) != "" {
			result = append(result, typed)
		}
	}
	return result
}

func videoPromptUniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
