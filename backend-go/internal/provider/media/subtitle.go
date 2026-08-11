package media

import (
	"fmt"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	SubtitlePresetClean    = "clean"
	SubtitlePresetEmphasis = "emphasis"

	SubtitlePositionBottom = "bottom"
	SubtitlePositionCenter = "center"

	DefaultSubtitleFont = "Source Han Sans SC"
	maxSubtitleLineRunes = 18
)

var allowedSubtitleFonts = map[string]bool{
	"Source Han Sans SC": true,
	"Noto Sans CJK SC":   true,
}

// NarrationSegment is one timed narration unit used for TTS and captions.
type NarrationSegment struct {
	SceneIndex int
	Text       string
	StartMs    int64
	DurationMs int64
}

type SubtitleCue struct {
	Index   int
	StartMs int64
	EndMs   int64
	Lines   []string
}

type SubtitleStyle struct {
	Preset   string
	Position string
	FontName string
}

// NarrationSceneInput is the minimal scene payload needed for caption/TTS timing.
type NarrationSceneInput struct {
	Index      int
	Narration  string
	DurationMs int64
}

// SegmentNarration splits scene narrations into speakable chunks, skipping blanks.
func SegmentNarration(scenes []NarrationSceneInput, speed float64) []NarrationSegment {
	if speed <= 0 {
		speed = 1
	}
	out := make([]NarrationSegment, 0, len(scenes))
	var cursor int64
	for _, scene := range scenes {
		parts := splitNarrationParts(scene.Narration)
		if len(parts) == 0 {
			cursor += scene.DurationMs
			continue
		}
		weights := make([]float64, len(parts))
		var totalWeight float64
		for i, part := range parts {
			weights[i] = math.Max(float64(utf8.RuneCountInString(part)), 1)
			totalWeight += weights[i]
		}
		sceneStart := cursor
		used := int64(0)
		for i, part := range parts {
			dur := int64(math.Round(float64(scene.DurationMs) * (weights[i] / totalWeight)))
			if i == len(parts)-1 {
				dur = scene.DurationMs - used
			}
			if dur < 400 {
				dur = 400
			}
			// Prefer punctuation-aware speech estimate when scene budget is generous.
			estimate := EstimateNarrationDurationMs(part, speed)
			if estimate > 0 && estimate < dur {
				dur = estimate
			}
			out = append(out, NarrationSegment{
				SceneIndex: scene.Index,
				Text:       part,
				StartMs:    sceneStart + used,
				DurationMs: dur,
			})
			used += dur
		}
		cursor += scene.DurationMs
	}
	return out
}

func splitNarrationParts(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	replacer := strings.NewReplacer("\r\n", "\n", "\r", "\n")
	text = replacer.Replace(text)
	var parts []string
	var b strings.Builder
	flush := func() {
		part := strings.TrimSpace(b.String())
		b.Reset()
		if part != "" {
			parts = append(parts, part)
		}
	}
	for _, r := range text {
		b.WriteRune(r)
		if r == '\n' || r == '。' || r == '！' || r == '？' || r == '；' || r == '.' || r == '!' || r == '?' || r == ';' {
			flush()
		}
	}
	flush()
	if len(parts) == 0 {
		return nil
	}
	return parts
}

// EstimateNarrationDurationMs estimates spoken duration from text and speed.
func EstimateNarrationDurationMs(text string, speed float64) int64 {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	if speed <= 0 {
		speed = 1
	}
	chars := 0
	punctBonus := 0.0
	for _, r := range text {
		if unicode.IsSpace(r) {
			continue
		}
		chars++
		switch r {
		case '，', ',', '、', '：', ':':
			punctBonus += 0.12
		case '。', '.', '！', '!', '？', '?', '；', ';':
			punctBonus += 0.22
		}
	}
	if chars == 0 {
		return 0
	}
	seconds := (float64(chars)/4.0 + punctBonus) / speed
	if seconds < 0.4 {
		seconds = 0.4
	}
	return int64(math.Round(seconds * 1000))
}

func WrapSubtitleLines(text string, maxRunes int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if maxRunes <= 0 {
		maxRunes = maxSubtitleLineRunes
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return []string{text}
	}
	lines := make([]string, 0, (len(runes)/maxRunes)+1)
	for len(runes) > 0 {
		n := maxRunes
		if n > len(runes) {
			n = len(runes)
		}
		// Prefer breaking after punctuation/space inside the window.
		cut := n
		for i := n - 1; i >= n/2; i-- {
			r := runes[i]
			if unicode.IsSpace(r) || strings.ContainsRune("，。！？；、,.!?;:", r) {
				cut = i + 1
				break
			}
		}
		lines = append(lines, strings.TrimSpace(string(runes[:cut])))
		runes = runes[cut:]
	}
	return lines
}

func BuildSubtitleCues(segments []NarrationSegment) ([]SubtitleCue, error) {
	cues := make([]SubtitleCue, 0, len(segments))
	var lastEnd int64
	for i, seg := range segments {
		text := strings.TrimSpace(seg.Text)
		if text == "" {
			continue
		}
		start := seg.StartMs
		if start < lastEnd {
			return nil, fmt.Errorf("subtitle timing is not monotonic at segment %d", i)
		}
		end := start + seg.DurationMs
		if end <= start {
			end = start + 400
		}
		cues = append(cues, SubtitleCue{
			Index:   len(cues) + 1,
			StartMs: start,
			EndMs:   end,
			Lines:   WrapSubtitleLines(text, maxSubtitleLineRunes),
		})
		lastEnd = end
	}
	return cues, nil
}

func ResolveSubtitleFont(fontName string) (string, error) {
	fontName = strings.TrimSpace(fontName)
	if fontName == "" {
		return DefaultSubtitleFont, nil
	}
	if !allowedSubtitleFonts[fontName] {
		return "", fmt.Errorf("subtitle font %q is not allowed", fontName)
	}
	return fontName, nil
}

func RenderSRT(cues []SubtitleCue) string {
	var b strings.Builder
	for _, cue := range cues {
		b.WriteString(fmt.Sprintf("%d\n", cue.Index))
		b.WriteString(formatSRTTimestamp(cue.StartMs))
		b.WriteString(" --> ")
		b.WriteString(formatSRTTimestamp(cue.EndMs))
		b.WriteByte('\n')
		b.WriteString(strings.Join(cue.Lines, "\n"))
		b.WriteString("\n\n")
	}
	return b.String()
}

func RenderASS(cues []SubtitleCue, style SubtitleStyle) (string, error) {
	font, err := ResolveSubtitleFont(style.FontName)
	if err != nil {
		return "", err
	}
	preset := strings.ToLower(strings.TrimSpace(style.Preset))
	if preset == "" {
		preset = SubtitlePresetClean
	}
	if preset != SubtitlePresetClean && preset != SubtitlePresetEmphasis {
		return "", fmt.Errorf("unsupported subtitle preset %s", preset)
	}
	position := strings.ToLower(strings.TrimSpace(style.Position))
	if position == "" {
		position = SubtitlePositionBottom
	}
	alignment := 2
	if position == SubtitlePositionCenter {
		alignment = 5
	} else if position != SubtitlePositionBottom {
		return "", fmt.Errorf("unsupported subtitle position %s", position)
	}
	fontSize := 48
	primaryColour := "&H00FFFFFF"
	outline := 2
	if preset == SubtitlePresetEmphasis {
		fontSize = 56
		primaryColour = "&H0000E5FF"
		outline = 3
	}
	var b strings.Builder
	b.WriteString("[Script Info]\n")
	b.WriteString("ScriptType: v4.00+\n")
	b.WriteString("PlayResX: 1080\n")
	b.WriteString("PlayResY: 1920\n")
	b.WriteString("WrapStyle: 2\n\n")
	b.WriteString("[V4+ Styles]\n")
	b.WriteString("Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n")
	b.WriteString(fmt.Sprintf(
		"Style: Default,%s,%d,%s,&H000000FF,&H00000000,&H64000000,0,0,0,0,100,100,0,0,1,%d,0,%d,40,40,80,1\n\n",
		font, fontSize, primaryColour, outline, alignment,
	))
	b.WriteString("[Events]\n")
	b.WriteString("Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")
	for _, cue := range cues {
		text := strings.Join(cue.Lines, "\\N")
		text = strings.ReplaceAll(text, "{", "(")
		text = strings.ReplaceAll(text, "}", ")")
		b.WriteString(fmt.Sprintf(
			"Dialogue: 0,%s,%s,Default,,0,0,0,,%s\n",
			formatASSTimestamp(cue.StartMs),
			formatASSTimestamp(cue.EndMs),
			text,
		))
	}
	return b.String(), nil
}

func formatSRTTimestamp(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	hours := ms / 3_600_000
	ms %= 3_600_000
	minutes := ms / 60_000
	ms %= 60_000
	seconds := ms / 1000
	millis := ms % 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, seconds, millis)
}

func formatASSTimestamp(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	hours := ms / 3_600_000
	ms %= 3_600_000
	minutes := ms / 60_000
	ms %= 60_000
	seconds := ms / 1000
	centis := (ms % 1000) / 10
	return fmt.Sprintf("%d:%02d:%02d.%02d", hours, minutes, seconds, centis)
}
