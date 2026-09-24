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

const (
	videoStoryboardPlanVersion          = 1
	videoStoryboardPlannerVersion       = "video-storyboard-planner-v1"
	videoStoryboardMaxShots             = 6
	videoStoryboardDurationMismatchCode = "VIDEO_STORYBOARD_DURATION_MISMATCH"
	videoStoryboardTooManyShotsCode     = "VIDEO_STORYBOARD_TOO_MANY_SHOTS"
	videoStoryboardInvalidTimelineCode  = "VIDEO_STORYBOARD_TIMELINE_INVALID"
	videoStoryboardEmptyBriefCode       = "VIDEO_STORYBOARD_EMPTY_BRIEF"
	videoStoryboardDurationInferredCode = "VIDEO_STORYBOARD_DURATION_INFERRED"
)

type videoStoryboardPlanError struct {
	Code    string
	Message string
}

func (e *videoStoryboardPlanError) Error() string { return e.Message }

type videoStoryboardPlan struct {
	SchemaVersion         int                        `json:"schema_version"`
	PlannerVersion        string                     `json:"planner_version"`
	ParentDurationSeconds int                        `json:"parent_duration_seconds"`
	ParentCanonicalHash   string                     `json:"parent_canonical_hash,omitempty"`
	BriefHash             string                     `json:"brief_hash"`
	StoryboardHash        string                     `json:"storyboard_hash"`
	Shots                 []videoStoryboardShot      `json:"shots"`
	PostProcess           videoStoryboardPostProcess `json:"post_process"`
	Continuity            videoStoryboardContinuity  `json:"continuity"`
	Consistency           videoStoryboardConsistency `json:"consistency"`
}

type videoStoryboardShot struct {
	ShotID                string `json:"shot_id"`
	Order                 int    `json:"order"`
	TargetDurationSeconds int    `json:"target_duration_seconds"`
	Scene                 string `json:"scene"`
	Subject               string `json:"subject"`
	Action                string `json:"action"`
	Camera                string `json:"camera"`
	VisualStyle           string `json:"visual_style"`
	ContinuityNotes       string `json:"continuity_notes"`
	ProviderPrompt        string `json:"provider_prompt"`
}

type videoStoryboardPostProcess struct {
	Subtitles           []videoStoryboardTrackItem `json:"subtitles"`
	Voiceover           []videoStoryboardTrackItem `json:"voiceover"`
	Overlays            []videoStoryboardTrackItem `json:"overlays"`
	Branding            []videoStoryboardTrackItem `json:"branding"`
	InfographicOverlays []videoStoryboardTrackItem `json:"infographic_overlays"`
}

type videoStoryboardTrackItem struct {
	ShotID string `json:"shot_id,omitempty"`
	Kind   string `json:"kind"`
	Text   string `json:"text"`
}

type videoStoryboardContinuity struct {
	SubjectIdentity  string `json:"subject_identity"`
	ClothingStyle    string `json:"clothing_style"`
	Palette          string `json:"palette"`
	Environment      string `json:"environment"`
	PreviousShotHint string `json:"previous_shot_hint"`
}

type videoStoryboardConsistency struct {
	Status       string   `json:"status"`
	WarningCodes []string `json:"warning_codes,omitempty"`
	Message      string   `json:"message,omitempty"`
}

type videoStoryboardSegment struct {
	Label string
	Start *int
	End   *int
	Body  string
}

var (
	videoStoryboardShotHeaderPattern   = regexp.MustCompile(`(?m)^\s*(镜头|场景)\s*([0-9一二三四五六七八九十]+)\s*(?:[（(]\s*(?:第\s*)?(\d{1,3})\s*(?:秒|s)?\s*[-–—~～至到]\s*(\d{1,3})\s*(?:秒|s)?\s*[）)])?\s*[:：]`)
	videoStoryboardOverlayQuotePattern = regexp.MustCompile(`[「“"]([^」”"\n]{1,160})[」”"]`)
	videoStoryboardUILabelPattern      = regexp.MustCompile(`界面显示([^，。；;\n]{1,80})标签`)
	videoStoryboardHeadlinePattern     = regexp.MustCompile(`顶部大字([^，。；;\n]{1,80})`)
	videoStoryboardCardPattern         = regexp.MustCompile(`(?:依次弹出)?([^，。；;\n]{1,80})文字卡片`)
	videoStoryboardHighlightPattern    = regexp.MustCompile(`高亮文字\s*[「“"]([^」”"\n]{1,160})[」”"]`)
	videoStoryboardLogoPattern         = regexp.MustCompile(`(?i)(?:\blogo\b|品牌标|品牌logo)`)
	videoStoryboardFlowNodesPattern    = regexp.MustCompile(`从上到下[:：]\s*([^。\n]+)`)
	videoStoryboardIconPattern         = regexp.MustCompile(`(?:左侧|右侧)([^，。；;\n]{1,40})图标`)
	videoStoryboardSyncAVPattern       = regexp.MustCompile(`字幕同步配音`)
	videoStoryboardHDPattern           = regexp.MustCompile(`高清`)
	videoStoryboardCameraPattern       = regexp.MustCompile(`镜头(?:缓慢)?(?:推进|拉开|环绕|下移|上移|跟随)`)
	videoStoryboardSubjectPattern      = regexp.MustCompile(`(?:卡通)?(?:商务)?(?:男性|女性|人物)|天平|流程图`)
)

func buildVideoStoryboardPlan(canonical canonicalVideoRequest, userPrompt string) (videoStoryboardPlan, error) {
	parentDuration := canonical.Execution.DurationSeconds
	if parentDuration <= 0 {
		return videoStoryboardPlan{}, &videoStoryboardPlanError{Code: videoStoryboardInvalidTimelineCode, Message: "parent duration is invalid"}
	}
	brief := strings.TrimSpace(userPrompt)
	if brief == "" {
		brief = strings.TrimSpace(canonical.Prompt)
	}
	brief = normalizeVideoProviderPrompt(brief)
	parentHash, err := canonicalVideoRequestHash(canonical)
	if err != nil {
		return videoStoryboardPlan{}, err
	}

	preamble, segments := splitVideoStoryboardSegments(brief)
	if len(segments) > videoStoryboardMaxShots {
		return videoStoryboardPlan{}, &videoStoryboardPlanError{Code: videoStoryboardTooManyShotsCode, Message: "too many shots"}
	}
	if len(segments) == 0 {
		segments = []videoStoryboardSegment{{Label: "scene", Body: brief}}
		preamble = ""
	}

	durations, inferred, err := assignVideoStoryboardDurations(segments, parentDuration)
	if err != nil {
		return videoStoryboardPlan{}, err
	}

	styleSeed := sanitizeVideoStoryboardVisualText(preamble, canonical, "")
	plan := videoStoryboardPlan{
		SchemaVersion:         videoStoryboardPlanVersion,
		PlannerVersion:        videoStoryboardPlannerVersion,
		ParentDurationSeconds: parentDuration,
		ParentCanonicalHash:   parentHash,
		BriefHash:             videoPromptHash(brief),
		Shots:                 make([]videoStoryboardShot, 0, len(segments)),
		PostProcess: videoStoryboardPostProcess{
			Subtitles:           []videoStoryboardTrackItem{},
			Voiceover:           []videoStoryboardTrackItem{},
			Overlays:            []videoStoryboardTrackItem{},
			Branding:            []videoStoryboardTrackItem{},
			InfographicOverlays: []videoStoryboardTrackItem{},
		},
		Consistency: videoStoryboardConsistency{Status: "ok"},
	}
	if strings.TrimSpace(brief) == "" {
		plan.Consistency.Status = "warning"
		plan.Consistency.WarningCodes = []string{videoStoryboardEmptyBriefCode}
		plan.Consistency.Message = "empty brief"
	}
	if inferred {
		plan.Consistency.WarningCodes = appendUniqueVideoPromptCode(plan.Consistency.WarningCodes, videoStoryboardDurationInferredCode)
		if plan.Consistency.Status == "ok" {
			plan.Consistency.Status = "warning"
		}
	}

	globalBody := preamble
	if globalBody == "" {
		globalBody = brief
	}
	plan.PostProcess, globalBody = extractVideoStoryboardTracks(plan.PostProcess, "", globalBody)
	styleSeed = sanitizeVideoStoryboardVisualText(globalBody, canonical, styleSeed)
	plan.Continuity = buildVideoStoryboardContinuity(styleSeed, segments)

	for index, segment := range segments {
		shotID := fmt.Sprintf("shot_%02d", index+1)
		tracks, visual := extractVideoStoryboardTracks(videoStoryboardPostProcess{
			Subtitles:           []videoStoryboardTrackItem{},
			Voiceover:           []videoStoryboardTrackItem{},
			Overlays:            []videoStoryboardTrackItem{},
			Branding:            []videoStoryboardTrackItem{},
			InfographicOverlays: []videoStoryboardTrackItem{},
		}, shotID, segment.Body)
		plan.PostProcess = mergeVideoStoryboardTracks(plan.PostProcess, tracks)
		providerPrompt := composeVideoStoryboardShotPrompt(styleSeed, visual, canonical)
		shot := videoStoryboardShot{
			ShotID:                shotID,
			Order:                 index + 1,
			TargetDurationSeconds: durations[index],
			Scene:                 firstVideoStoryboardClause(visual),
			Subject:               matchVideoStoryboardPattern(visual, videoStoryboardSubjectPattern),
			Action:                visual,
			Camera:                matchVideoStoryboardPattern(visual+" "+styleSeed, videoStoryboardCameraPattern),
			VisualStyle:           styleSeed,
			ContinuityNotes:       videoStoryboardContinuityNote(plan.Continuity, index),
			ProviderPrompt:        providerPrompt,
		}
		if shot.Subject == "" {
			shot.Subject = plan.Continuity.SubjectIdentity
		}
		plan.Shots = append(plan.Shots, shot)
	}

	sum := 0
	for _, shot := range plan.Shots {
		sum += shot.TargetDurationSeconds
		if forbiddenVideoStoryboardPromptContent(shot.ProviderPrompt, canonical) {
			return videoStoryboardPlan{}, &videoStoryboardPlanError{Code: "VIDEO_STORYBOARD_PROMPT_SANITIZE_FAILED", Message: "shot provider_prompt still contains forbidden content"}
		}
	}
	if sum != parentDuration {
		return videoStoryboardPlan{}, &videoStoryboardPlanError{Code: videoStoryboardDurationMismatchCode, Message: "shot durations do not sum to parent duration"}
	}
	hash, err := videoStoryboardPlanHash(plan)
	if err != nil {
		return videoStoryboardPlan{}, err
	}
	plan.StoryboardHash = hash
	return plan, nil
}

func splitVideoStoryboardSegments(brief string) (string, []videoStoryboardSegment) {
	indexes := videoStoryboardShotHeaderPattern.FindAllStringSubmatchIndex(brief, -1)
	if len(indexes) == 0 {
		return "", nil
	}
	preamble := strings.TrimSpace(brief[:indexes[0][0]])
	segments := make([]videoStoryboardSegment, 0, len(indexes))
	for i, loc := range indexes {
		end := len(brief)
		if i+1 < len(indexes) {
			end = indexes[i+1][0]
		}
		body := strings.TrimSpace(brief[loc[1]:end])
		segment := videoStoryboardSegment{Label: brief[loc[0]:loc[1]], Body: body}
		if loc[6] >= 0 && loc[8] >= 0 {
			start, errStart := strconv.Atoi(brief[loc[6]:loc[7]])
			finish, errEnd := strconv.Atoi(brief[loc[8]:loc[9]])
			if errStart == nil && errEnd == nil {
				segment.Start = &start
				segment.End = &finish
			}
		}
		segments = append(segments, segment)
	}
	return preamble, segments
}

func assignVideoStoryboardDurations(segments []videoStoryboardSegment, parent int) ([]int, bool, error) {
	durations := make([]int, len(segments))
	timed := 0
	for i, segment := range segments {
		if segment.Start == nil || segment.End == nil {
			continue
		}
		timed++
		if *segment.Start < 0 || *segment.End <= *segment.Start {
			return nil, false, &videoStoryboardPlanError{Code: videoStoryboardInvalidTimelineCode, Message: "shot timeline is invalid"}
		}
		durations[i] = *segment.End - *segment.Start
	}
	if timed == 0 {
		base := parent / len(segments)
		remainder := parent % len(segments)
		for i := range durations {
			durations[i] = base
		}
		durations[len(durations)-1] += remainder
		return durations, len(segments) > 1, nil
	}
	if timed != len(segments) {
		return nil, false, &videoStoryboardPlanError{Code: videoStoryboardInvalidTimelineCode, Message: "mixed timed and untimed shots"}
	}
	sum := 0
	for _, duration := range durations {
		sum += duration
	}
	if sum != parent {
		return nil, false, &videoStoryboardPlanError{Code: videoStoryboardDurationMismatchCode, Message: "timeline does not match parent duration"}
	}
	return durations, false, nil
}

func extractVideoStoryboardTracks(tracks videoStoryboardPostProcess, shotID, text string) (videoStoryboardPostProcess, string) {
	if videoStoryboardSyncAVPattern.MatchString(text) {
		tracks.Subtitles = append(tracks.Subtitles, videoStoryboardTrackItem{ShotID: shotID, Kind: "sync_subtitle", Text: "字幕同步"})
		tracks.Voiceover = append(tracks.Voiceover, videoStoryboardTrackItem{ShotID: shotID, Kind: "sync_voiceover", Text: "同步配音"})
		text = videoStoryboardSyncAVPattern.ReplaceAllString(text, " ")
	}
	text = videoStoryboardHighlightPattern.ReplaceAllStringFunc(text, func(match string) string {
		sub := videoStoryboardHighlightPattern.FindStringSubmatch(match)
		if len(sub) == 2 {
			tracks.Overlays = append(tracks.Overlays, videoStoryboardTrackItem{ShotID: shotID, Kind: "highlight_text", Text: strings.TrimSpace(sub[1])})
		}
		return " "
	})
	text = videoStoryboardOverlayQuotePattern.ReplaceAllStringFunc(text, func(match string) string {
		sub := videoStoryboardOverlayQuotePattern.FindStringSubmatch(match)
		if len(sub) == 2 {
			tracks.Overlays = append(tracks.Overlays, videoStoryboardTrackItem{ShotID: shotID, Kind: "quoted_text", Text: strings.TrimSpace(sub[1])})
		}
		return " "
	})
	text = videoPromptSubtitlePattern.ReplaceAllStringFunc(text, func(match string) string {
		tracks.Subtitles = append(tracks.Subtitles, videoStoryboardTrackItem{ShotID: shotID, Kind: "subtitle", Text: strings.TrimSpace(match)})
		return " "
	})
	text = videoPromptAudioPattern.ReplaceAllStringFunc(text, func(match string) string {
		tracks.Voiceover = append(tracks.Voiceover, videoStoryboardTrackItem{ShotID: shotID, Kind: "voiceover", Text: strings.TrimSpace(match)})
		return " "
	})
	text = videoStoryboardUILabelPattern.ReplaceAllStringFunc(text, func(match string) string {
		sub := videoStoryboardUILabelPattern.FindStringSubmatch(match)
		if len(sub) == 2 {
			tracks.Overlays = append(tracks.Overlays, videoStoryboardTrackItem{ShotID: shotID, Kind: "ui_label", Text: strings.TrimSpace(sub[1])})
		}
		return " "
	})
	text = videoStoryboardHeadlinePattern.ReplaceAllStringFunc(text, func(match string) string {
		sub := videoStoryboardHeadlinePattern.FindStringSubmatch(match)
		if len(sub) == 2 {
			tracks.Overlays = append(tracks.Overlays, videoStoryboardTrackItem{ShotID: shotID, Kind: "headline", Text: strings.TrimSpace(sub[1])})
		}
		return " "
	})
	text = videoStoryboardCardPattern.ReplaceAllStringFunc(text, func(match string) string {
		sub := videoStoryboardCardPattern.FindStringSubmatch(match)
		if len(sub) == 2 {
			tracks.Overlays = append(tracks.Overlays, videoStoryboardTrackItem{ShotID: shotID, Kind: "text_card", Text: strings.TrimSpace(sub[1])})
		}
		return " "
	})
	text = videoStoryboardFlowNodesPattern.ReplaceAllStringFunc(text, func(match string) string {
		sub := videoStoryboardFlowNodesPattern.FindStringSubmatch(match)
		if len(sub) == 2 {
			tracks.InfographicOverlays = append(tracks.InfographicOverlays, videoStoryboardTrackItem{ShotID: shotID, Kind: "flowchart_nodes", Text: strings.TrimSpace(sub[1])})
		}
		return "流程图"
	})
	text = videoStoryboardIconPattern.ReplaceAllStringFunc(text, func(match string) string {
		tracks.InfographicOverlays = append(tracks.InfographicOverlays, videoStoryboardTrackItem{ShotID: shotID, Kind: "icon_label", Text: strings.TrimSpace(match)})
		return "图标"
	})
	if videoStoryboardLogoPattern.MatchString(text) {
		tracks.Branding = append(tracks.Branding, videoStoryboardTrackItem{ShotID: shotID, Kind: "logo", Text: "Logo"})
		text = videoStoryboardLogoPattern.ReplaceAllString(text, " ")
	}
	return tracks, strings.TrimSpace(text)
}

func mergeVideoStoryboardTracks(dst, src videoStoryboardPostProcess) videoStoryboardPostProcess {
	dst.Subtitles = append(dst.Subtitles, src.Subtitles...)
	dst.Voiceover = append(dst.Voiceover, src.Voiceover...)
	dst.Overlays = append(dst.Overlays, src.Overlays...)
	dst.Branding = append(dst.Branding, src.Branding...)
	dst.InfographicOverlays = append(dst.InfographicOverlays, src.InfographicOverlays...)
	return dst
}

func sanitizeVideoStoryboardVisualText(text string, canonical canonicalVideoRequest, fallback string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return fallback
	}
	text = stripVideoStoryboardExecutionTokens(text, canonical)
	if next, changed := removeMissingVideoReferenceInstruction(text); changed {
		text = next
	}
	text = videoStoryboardHDPattern.ReplaceAllString(text, " ")
	text = videoPromptTextLabelPattern.ReplaceAllString(text, " ")
	text = normalizeVideoProviderPrompt(text)
	if text == "" {
		return fallback
	}
	return text
}

func stripVideoStoryboardExecutionTokens(prompt string, canonical canonicalVideoRequest) string {
	changed := false
	durations := uniqueVideoPromptInts([]int{canonical.Execution.DurationSeconds})
	for _, duration := range durations {
		pattern := regexp.MustCompile(`(?:生成|制作|创建|做)?\s*(?:一个|一条)?\s*(?:约|时长(?:为)?|总时长(?:为)?)?\s*` + regexp.QuoteMeta(strconv.Itoa(duration)) + `\s*(?:秒|s|seconds?)`)
		prompt, changed = replaceVideoPromptDeclaration(prompt, pattern, changed)
	}
	for _, aspectRatio := range uniqueVideoPromptStrings([]string{canonical.Execution.AspectRatio, "竖屏" + canonical.Execution.AspectRatio}) {
		pattern := regexp.MustCompile(`(?i)(?:竖屏|横屏|画幅(?:比例)?|比例|aspect\s*ratio)?\s*` + regexp.QuoteMeta(strings.TrimPrefix(aspectRatio, "竖屏")))
		prompt, changed = replaceVideoPromptDeclaration(prompt, pattern, changed)
	}
	for _, resolution := range uniqueVideoPromptStrings([]string{canonical.Execution.Resolution}) {
		pattern := regexp.MustCompile(`(?i)(?:分辨率|清晰度|quality)?\s*` + regexp.QuoteMeta(resolution))
		prompt, changed = replaceVideoPromptDeclaration(prompt, pattern, changed)
	}
	mode := strings.TrimSpace(canonical.Execution.InputMode)
	if mode != "" {
		pattern := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(mode) + `|文生视频|text-to-video`)
		prompt, changed = replaceVideoPromptDeclaration(prompt, pattern, changed)
	}
	_ = changed
	return prompt
}

func composeVideoStoryboardShotPrompt(style, visual string, canonical canonicalVideoRequest) string {
	parts := make([]string, 0, 2)
	if visual = sanitizeVideoStoryboardVisualText(visual, canonical, ""); visual != "" {
		parts = append(parts, visual)
	}
	if style = sanitizeVideoStoryboardVisualText(style, canonical, ""); style != "" && style != visual {
		parts = append(parts, style)
	}
	return normalizeVideoProviderPrompt(strings.Join(parts, "，"))
}

func buildVideoStoryboardContinuity(style string, segments []videoStoryboardSegment) videoStoryboardContinuity {
	joined := style
	if len(segments) > 0 {
		joined = strings.TrimSpace(style + " " + segments[0].Body)
	}
	continuity := videoStoryboardContinuity{
		SubjectIdentity:  matchVideoStoryboardPattern(joined, videoStoryboardSubjectPattern),
		Environment:      firstVideoStoryboardClause(style),
		Palette:          style,
		PreviousShotHint: "keep subject, wardrobe, palette and environment continuous",
	}
	if strings.Contains(style, "MG") || strings.Contains(style, "矢量") {
		continuity.ClothingStyle = "flat vector MG"
	}
	return continuity
}

func videoStoryboardContinuityNote(continuity videoStoryboardContinuity, index int) string {
	if index == 0 {
		return strings.TrimSpace(strings.Join([]string{continuity.SubjectIdentity, continuity.Environment, continuity.ClothingStyle}, "，"))
	}
	return continuity.PreviousShotHint
}

func firstVideoStoryboardClause(value string) string {
	value = strings.TrimSpace(value)
	for _, sep := range []string{"，", "。", ",", ";"} {
		if index := strings.Index(value, sep); index > 0 {
			return strings.TrimSpace(value[:index])
		}
	}
	runes := []rune(value)
	if len(runes) > 40 {
		return string(runes[:40])
	}
	return value
}

func matchVideoStoryboardPattern(value string, pattern *regexp.Regexp) string {
	return strings.TrimSpace(pattern.FindString(value))
}

func forbiddenVideoStoryboardPromptContent(prompt string, canonical canonicalVideoRequest) bool {
	if videoPromptSubtitlePattern.MatchString(prompt) || videoPromptAudioPattern.MatchString(prompt) || videoStoryboardSyncAVPattern.MatchString(prompt) {
		return true
	}
	if videoStoryboardHeadlinePattern.MatchString(prompt) || videoStoryboardUILabelPattern.MatchString(prompt) || videoStoryboardHighlightPattern.MatchString(prompt) {
		return true
	}
	if videoStoryboardLogoPattern.MatchString(prompt) {
		return true
	}
	if canonical.Execution.DurationSeconds > 0 {
		token := strconv.Itoa(canonical.Execution.DurationSeconds) + "秒"
		if strings.Contains(prompt, token) || strings.Contains(strings.ToLower(prompt), strings.ToLower(strconv.Itoa(canonical.Execution.DurationSeconds)+"s")) {
			return true
		}
	}
	if ratio := strings.TrimSpace(canonical.Execution.AspectRatio); ratio != "" && strings.Contains(prompt, ratio) {
		return true
	}
	if resolution := strings.TrimSpace(canonical.Execution.Resolution); resolution != "" && strings.Contains(strings.ToLower(prompt), strings.ToLower(resolution)) {
		return true
	}
	if mode := strings.TrimSpace(canonical.Execution.InputMode); mode != "" && strings.Contains(strings.ToUpper(prompt), strings.ToUpper(mode)) {
		return true
	}
	return false
}

func videoStoryboardPlanHash(plan videoStoryboardPlan) (string, error) {
	identity := struct {
		SchemaVersion         int                        `json:"schema_version"`
		PlannerVersion        string                     `json:"planner_version"`
		ParentDurationSeconds int                        `json:"parent_duration_seconds"`
		ParentCanonicalHash   string                     `json:"parent_canonical_hash"`
		BriefHash             string                     `json:"brief_hash"`
		Shots                 []videoStoryboardShot      `json:"shots"`
		PostProcess           videoStoryboardPostProcess `json:"post_process"`
		Continuity            videoStoryboardContinuity  `json:"continuity"`
		Consistency           videoStoryboardConsistency `json:"consistency"`
	}{
		SchemaVersion: plan.SchemaVersion, PlannerVersion: plan.PlannerVersion,
		ParentDurationSeconds: plan.ParentDurationSeconds, ParentCanonicalHash: plan.ParentCanonicalHash,
		BriefHash: plan.BriefHash, Shots: plan.Shots, PostProcess: plan.PostProcess,
		Continuity: plan.Continuity, Consistency: plan.Consistency,
	}
	payload, err := json.Marshal(identity)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}
