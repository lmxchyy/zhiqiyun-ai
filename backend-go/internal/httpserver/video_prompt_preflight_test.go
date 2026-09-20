package httpserver

import (
	"strings"
	"testing"
)

func TestVideoPromptPreflightDurationConsistency(t *testing.T) {
	mismatch := inspectVideoPromptPreflight("生成30秒宣传片", map[string]any{"duration": 15}, videoModeText)
	if !videoPromptContainsString(mismatch.WarningCodes, videoPromptDurationMismatchCode) {
		t.Fatalf("warning codes = %v, want duration mismatch", mismatch.WarningCodes)
	}
	match := inspectVideoPromptPreflight("生成15秒宣传片", map[string]any{"duration": 15}, videoModeText)
	if videoPromptContainsString(match.WarningCodes, videoPromptDurationMismatchCode) {
		t.Fatalf("warning codes = %v, did not want duration mismatch", match.WarningCodes)
	}
}

func TestVideoPromptPreflightReferenceConsistency(t *testing.T) {
	missing := inspectVideoPromptPreflight("根据3张参考图片生成视频", map[string]any{"duration": 15}, videoModeImage)
	if !videoPromptContainsString(missing.WarningCodes, videoPromptReferenceMissingCode) {
		t.Fatalf("warning codes = %v, want reference missing", missing.WarningCodes)
	}
	present := inspectVideoPromptPreflight("根据3张参考图片生成视频", map[string]any{
		"duration":   15,
		"image_urls": []any{"https://example.test/1.png", "https://example.test/2.png", "https://example.test/3.png"},
	}, videoModeImage)
	if videoPromptContainsString(present.WarningCodes, videoPromptReferenceMissingCode) {
		t.Fatalf("warning codes = %v, did not want reference missing", present.WarningCodes)
	}
}

func TestVideoPromptPreflightModeAndComplexityAreWarningsOnly(t *testing.T) {
	result := inspectVideoPromptPreflight("多场景多镜头，加入字幕、配音并保持音画同步", map[string]any{"duration": 15}, videoModeText)
	if !videoPromptContainsString(result.WarningCodes, videoPromptComplexityCode) {
		t.Fatalf("warning codes = %v, want complexity warning", result.WarningCodes)
	}
	mode := inspectVideoPromptPreflight("根据上传图片生成一个镜头", map[string]any{"duration": 15}, videoModeText)
	if !videoPromptContainsString(mode.WarningCodes, videoPromptModeMismatchCode) {
		t.Fatalf("warning codes = %v, want mode mismatch", mode.WarningCodes)
	}
	if len(result.WarningCodes) == 0 || strings.Contains(strings.Join(result.WarningCodes, ","), "REJECT") {
		t.Fatalf("preflight must only return soft warning codes: %v", result.WarningCodes)
	}
}

func TestVideoPromptPreflightDoesNotRejectFinancialComplianceText(t *testing.T) {
	result := inspectVideoPromptPreflight("制作金融合规宣传片，强调风险提示与合规经营", map[string]any{"duration": 15}, videoModeText)
	if len(result.WarningCodes) != 0 {
		t.Fatalf("warning codes = %v, financial/compliance wording alone should be allowed", result.WarningCodes)
	}
}

func TestVideoPromptHashDoesNotExposePrompt(t *testing.T) {
	prompt := "生成30秒宣传片"
	hash := videoPromptHash(prompt)
	if hash == prompt || len(hash) != 64 {
		t.Fatalf("hash = %q, want a 64-character digest without prompt text", hash)
	}
}

func videoPromptContainsString(items []string, wanted string) bool {
	for _, item := range items {
		if item == wanted {
			return true
		}
	}
	return false
}
