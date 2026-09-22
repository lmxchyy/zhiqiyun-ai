package httpserver

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	videoPromptExecutionVersion = 1
	videoPromptGuardVersion     = "video-prompt-guard-v1"

	videoPromptStrippedExecutionParamsCode   = "VIDEO_PROMPT_STRIPPED_EXECUTION_PARAMS"
	videoPromptRemovedMissingReferenceCode   = "VIDEO_PROMPT_REMOVED_MISSING_REFERENCE"
	videoPromptSimplifiedTextRenderingCode   = "VIDEO_PROMPT_SIMPLIFIED_TEXT_RENDERING"
	videoPromptSimplifiedSubtitleRequirement = "VIDEO_PROMPT_SIMPLIFIED_SUBTITLE_REQUIREMENT"
	videoPromptSimplifiedAudioRequirement    = "VIDEO_PROMPT_SIMPLIFIED_AUDIO_REQUIREMENT"
	videoPromptSimplifiedTimelineCode        = "VIDEO_PROMPT_SIMPLIFIED_TIMELINE"
	videoPromptComplexityReducedCode         = "VIDEO_PROMPT_COMPLEXITY_REDUCED"
)

// videoPromptExecution is a provider-transport prompt snapshot. It deliberately
// does not contain another copy of the user prompt: task.Prompt and
// canonical_video_request.prompt remain the immutable user-request sources.
//
// It is intentionally a pure value in this phase. No task, provider, billing,
// canonical, fingerprint, or persistence path calls this builder yet.
type videoPromptExecution struct {
	SchemaVersion       int      `json:"schema_version"`
	GuardVersion        string   `json:"guard_version"`
	UserPromptHash      string   `json:"user_prompt_hash"`
	ProviderPrompt      string   `json:"provider_prompt"`
	ProviderPromptHash  string   `json:"provider_prompt_hash"`
	TransformationCodes []string `json:"transformation_codes"`
}

type videoPromptGuardInput struct {
	UserPrompt string
	Canonical  canonicalVideoRequest
}

var (
	videoPromptInlineWhitespacePattern = regexp.MustCompile(`[\t \f\v]+`)
	videoPromptTimelineLabelPattern    = regexp.MustCompile(`(?m)(镜头|场景)\s*([0-9一二三四五六七八九十]+)\s*[（(]\s*(?:第\s*)?\d{1,3}\s*(?:秒|s)?\s*[-–—~～至到]\s*\d{1,3}\s*(?:秒|s)?\s*[）)]\s*[:：]`)
	videoPromptExactTextPattern        = regexp.MustCompile(`(?i)(?:屏幕(?:上|中)?(?:精确)?显示|界面(?:中)?(?:精确)?显示|顶部大字|高亮文字|文字卡片|UI\s*中显示|logo\s*中出现)\s*[「“\"']([^」”\"'\n]{1,120})[」”\"']`)
	videoPromptTextLabelPattern        = regexp.MustCompile(`(?i)(?:界面|屏幕|UI)\s*(?:显示|出现)\s*[^，。；;\n]{1,60}(?:标签|文字|文案|表格)`)
	videoPromptSubtitlePattern         = regexp.MustCompile(`(?i)(?:精确)?字幕\s*(?:显示|内容为|文字为)?\s*[:：]?\s*(?:[「“\"'][^」”\"'\n]{1,160}[」”\"']|[^，。；;\n]{1,100})`)
	videoPromptAudioPattern            = regexp.MustCompile(`(?i)(?:人物)?\s*(?:精确)?(?:说出|口播|旁白|同步中文配音|同步配音|精确中文语音|逐字口型同步|lip[-\s]?sync)\s*[:：]?\s*(?:[「“\"'][^」”\"'\n]{1,160}[」”\"']|[^，。；;\n]{0,100})`)
	videoPromptMissingReferencePattern = regexp.MustCompile(`(?i)(?:请)?(?:参考|依据|根据)(?:我?上传|提供|这|该)?(?:的)?\s*(?:[一二三四五六七\d]+\s*张?)?\s*(?:参考)?(?:图|图片|图像|照片|插画|素材)(?:来|进行)?(?:生成|制作)?`)
)

// buildVideoPromptExecution is deterministic and side-effect free. It never
// mutates input.Canonical; its output is only a future provider transport
// payload candidate.
func buildVideoPromptExecution(input videoPromptGuardInput) videoPromptExecution {
	return buildVideoPromptExecutionForVersion(input, videoPromptGuardVersion)
}

func buildVideoPromptExecutionForVersion(input videoPromptGuardInput, guardVersion string) videoPromptExecution {
	userPrompt := strings.TrimSpace(input.UserPrompt)
	if userPrompt == "" {
		userPrompt = strings.TrimSpace(input.Canonical.Prompt)
	}
	providerPrompt := normalizeVideoProviderPrompt(userPrompt)
	codes := map[string]struct{}{}

	if next, changed := stripVideoExecutionPromptDeclarations(providerPrompt, input.Canonical); changed {
		providerPrompt = next
		codes[videoPromptStrippedExecutionParamsCode] = struct{}{}
	}
	if len(input.Canonical.Execution.ReferenceImages) == 0 && input.Canonical.PromptIntentHints.ReferenceImageRequested {
		if next, changed := removeMissingVideoReferenceInstruction(providerPrompt); changed {
			providerPrompt = next
			codes[videoPromptRemovedMissingReferenceCode] = struct{}{}
		}
	}
	if next, changed := simplifyVideoTextRendering(providerPrompt); changed {
		providerPrompt = next
		codes[videoPromptSimplifiedTextRenderingCode] = struct{}{}
	}
	if next, changed := simplifyVideoSubtitleRequirement(providerPrompt); changed {
		providerPrompt = next
		codes[videoPromptSimplifiedSubtitleRequirement] = struct{}{}
	}
	if next, changed := simplifyVideoAudioRequirement(providerPrompt); changed {
		providerPrompt = next
		codes[videoPromptSimplifiedAudioRequirement] = struct{}{}
	}
	if next, changed := simplifyVideoTimeline(providerPrompt); changed {
		providerPrompt = next
		codes[videoPromptSimplifiedTimelineCode] = struct{}{}
	}

	providerPrompt = normalizeVideoProviderPrompt(providerPrompt)
	// Never turn a minimally specified request into an empty provider prompt.
	// A no-op is safer than dropping the user's only expressive content.
	if providerPrompt == "" && userPrompt != "" {
		providerPrompt = normalizeVideoProviderPrompt(userPrompt)
		codes = map[string]struct{}{}
	}
	orderedCodes := sortedVideoPromptTransformationCodes(codes)
	if len(orderedCodes) > 0 || len(input.Canonical.PromptIntentHints.ComplexitySignals) >= 3 {
		orderedCodes = appendUniqueVideoPromptCode(orderedCodes, videoPromptComplexityReducedCode)
		sort.Strings(orderedCodes)
	}
	return videoPromptExecution{
		SchemaVersion:       videoPromptExecutionVersion,
		GuardVersion:        stableVideoPromptGuardVersion(guardVersion),
		UserPromptHash:      videoPromptHash(userPrompt),
		ProviderPrompt:      providerPrompt,
		ProviderPromptHash:  videoPromptHash(providerPrompt),
		TransformationCodes: orderedCodes,
	}
}

func stableVideoPromptGuardVersion(value string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return videoPromptGuardVersion
}

func normalizeVideoProviderPrompt(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	lines := strings.Split(value, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = videoPromptInlineWhitespacePattern.ReplaceAllString(strings.TrimSpace(line), " ")
		if line != "" {
			result = append(result, line)
		}
	}
	return strings.TrimSpace(strings.Join(result, "\n"))
}

func stripVideoExecutionPromptDeclarations(prompt string, canonical canonicalVideoRequest) (string, bool) {
	// Restrict removal to the request-style prefix. This excludes later
	// narrative phrases such as "30秒后", "30秒倒计时", and UI data values.
	prefix, suffix := videoPromptGuardPrefix(prompt)
	changed := false
	durations := []int{canonical.Execution.DurationSeconds}
	if canonical.PromptIntentHints.RequestedDurationSeconds != nil {
		durations = append(durations, *canonical.PromptIntentHints.RequestedDurationSeconds)
	}
	for _, duration := range uniqueVideoPromptInts(durations) {
		if duration <= 0 {
			continue
		}
		pattern := regexp.MustCompile(`(?:生成|制作|创建|做)?\s*(?:一个|一条)?\s*(?:约|时长(?:为)?|总时长(?:为)?)?\s*` + regexp.QuoteMeta(strconvItoa(duration)) + `\s*(?:秒|s|seconds?)`)
		prefix, changed = replaceVideoPromptDeclaration(prefix, pattern, changed)
	}
	aspectRatios := []string{canonical.Execution.AspectRatio, canonical.PromptIntentHints.RequestedAspectRatio}
	for _, aspectRatio := range uniqueVideoPromptStrings(aspectRatios) {
		pattern := regexp.MustCompile(`(?i)(?:竖屏|横屏|画幅(?:比例)?|比例|aspect\s*ratio)?\s*` + regexp.QuoteMeta(aspectRatio))
		prefix, changed = replaceVideoPromptDeclaration(prefix, pattern, changed)
	}
	resolutions := []string{canonical.Execution.Resolution, canonical.PromptIntentHints.RequestedResolution}
	for _, resolution := range uniqueVideoPromptStrings(resolutions) {
		pattern := regexp.MustCompile(`(?i)(?:分辨率|清晰度|quality)?\s*` + regexp.QuoteMeta(resolution))
		prefix, changed = replaceVideoPromptDeclaration(prefix, pattern, changed)
	}
	if !changed {
		return prompt, false
	}
	return strings.TrimSpace(prefix + suffix), true
}

func uniqueVideoPromptInts(values []int) []int {
	seen := map[int]struct{}{}
	result := make([]int, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; value <= 0 || ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func uniqueVideoPromptStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if _, ok := seen[value]; value == "" || ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func videoPromptGuardPrefix(prompt string) (string, string) {
	const maxPrefixRunes = 120
	cut := len(prompt)
	// A timeline can be introduced as "镜头 1" or without a scene label, so
	// its range is the authoritative prefix boundary. Request-level execution
	// cleanup must never inspect a timeline range.
	if location := videoPromptTimelineRangePattern.FindStringIndex(prompt); location != nil {
		cut = location[0]
	}
	for _, marker := range []string{"\n镜头", "\n场景", "镜头1", "镜头一", "场景1", "场景一"} {
		if index := strings.Index(prompt, marker); index >= 0 && index < cut {
			cut = index
		}
	}
	if len([]rune(prompt[:cut])) > maxPrefixRunes {
		for index := range prompt {
			if len([]rune(prompt[:index])) >= maxPrefixRunes {
				cut = index
				break
			}
		}
	}
	return prompt[:cut], prompt[cut:]
}

func replaceVideoPromptDeclaration(value string, pattern *regexp.Regexp, changed bool) (string, bool) {
	locations := pattern.FindAllStringIndex(value, -1)
	if len(locations) == 0 {
		return value, changed
	}
	var builder strings.Builder
	cursor := 0
	for _, location := range locations {
		matched := value[location[0]:location[1]]
		// Preserve a possible narrative duration/countdown construction. The
		// prefix limiter above already keeps this conservative; this extra check
		// prevents a bare "30秒后" from becoming malformed if it appears there.
		after := value[location[1]:]
		if strings.HasPrefix(after, "后") || strings.HasPrefix(after, "倒计时") || strings.HasPrefix(after, "内") || strings.HasPrefix(strings.TrimSpace(matched), "第") {
			continue
		}
		builder.WriteString(value[cursor:location[0]])
		builder.WriteString(" ")
		cursor = location[1]
		changed = true
	}
	if !changed {
		return value, false
	}
	builder.WriteString(value[cursor:])
	return builder.String(), true
}

func removeMissingVideoReferenceInstruction(prompt string) (string, bool) {
	if !videoPromptMissingReferencePattern.MatchString(prompt) {
		return prompt, false
	}
	return videoPromptMissingReferencePattern.ReplaceAllString(prompt, ""), true
}

func simplifyVideoTextRendering(prompt string) (string, bool) {
	changed := false
	prompt = videoPromptExactTextPattern.ReplaceAllStringFunc(prompt, func(match string) string {
		changed = true
		return "展示" + videoPromptSemanticTopic(match) + "主题的简洁视觉 UI 元素，无需可读文字"
	})
	prompt = videoPromptTextLabelPattern.ReplaceAllStringFunc(prompt, func(match string) string {
		changed = true
		return "展示" + videoPromptSemanticTopic(match) + "主题的简洁视觉 UI 元素，无需可读文字"
	})
	return prompt, changed
}

func simplifyVideoSubtitleRequirement(prompt string) (string, bool) {
	if !videoPromptSubtitlePattern.MatchString(prompt) {
		return prompt, false
	}
	return videoPromptSubtitlePattern.ReplaceAllStringFunc(prompt, func(match string) string {
		return "画面下方保留清晰字幕区域，表达" + videoPromptSemanticTopic(match) + "主题，无需可读文字"
	}), true
}

func videoPromptSemanticTopic(value string) string {
	// Preserve only high-level business/story concepts, not a demand to render
	// the original sentence verbatim. This intentionally remains a small,
	// explicit vocabulary; unknown copy becomes the neutral "原有内容".
	terms := []string{"商务男性", "商务女性", "合规预警", "合规", "资金流", "业务流", "监管机构", "监管专户", "平台企业", "净额申报", "优化税负", "税负", "支付", "风险"}
	kept := make([]string, 0, len(terms))
	for _, term := range terms {
		if strings.Contains(value, term) {
			kept = append(kept, term)
		}
	}
	if len(kept) > 0 {
		return strings.Join(kept, "、")
	}
	// A brand or product may appear only in quoted on-screen copy. Preserve
	// that semantic identity while explicitly removing the readable-text
	// requirement; do not silently turn an unknown brand into generic UI.
	if quoted := videoPromptQuotedContent(value); quoted != "" {
		return quoted
	}
	return "原有内容"
}

func videoPromptQuotedContent(value string) string {
	for _, delimiter := range [][2]string{{"「", "」"}, {"“", "”"}, {"\"", "\""}, {"'", "'"}} {
		start := strings.Index(value, delimiter[0])
		if start < 0 {
			continue
		}
		rest := value[start+len(delimiter[0]):]
		if end := strings.Index(rest, delimiter[1]); end >= 0 {
			if text := strings.TrimSpace(rest[:end]); text != "" {
				return text
			}
		}
	}
	return ""
}

func simplifyVideoAudioRequirement(prompt string) (string, bool) {
	if !videoPromptAudioPattern.MatchString(prompt) {
		return prompt, false
	}
	return videoPromptAudioPattern.ReplaceAllString(prompt, "人物进行自然表达动作"), true
}

func simplifyVideoTimeline(prompt string) (string, bool) {
	matches := videoPromptTimelineLabelPattern.FindAllStringSubmatchIndex(prompt, -1)
	if len(matches) < 2 {
		return prompt, false
	}
	prompt = videoPromptTimelineLabelPattern.ReplaceAllString(prompt, "场景$2：")
	first := strings.Index(prompt, "场景")
	if first >= 0 && !strings.Contains(prompt[:first], "依次表现") {
		prompt = prompt[:first] + "依次表现：" + prompt[first:]
	}
	return prompt, true
}

func sortedVideoPromptTransformationCodes(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func appendUniqueVideoPromptCode(values []string, code string) []string {
	for _, value := range values {
		if value == code {
			return values
		}
	}
	return append(values, code)
}

func strconvItoa(value int) string {
	return strconv.Itoa(value)
}
