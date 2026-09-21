package httpserver

import (
	"reflect"
	"testing"
)

func TestCanonicalVideoPromptIntentDurationParserExcludesSegmentDurations(t *testing.T) {
	cases := []struct {
		prompt string
		want   *int
	}{
		{prompt: "生成30秒宣传片", want: intPtr(30)},
		{prompt: "0-8s 开场，8-18s 展示，18-30s 结尾", want: nil},
		{prompt: "前5秒开场，最后3秒收尾", want: nil},
		{prompt: "生成30秒宣传片：0-8s 开场，8-18s 产品，18-30s 结尾", want: intPtr(30)},
	}
	for _, tc := range cases {
		t.Run(tc.prompt, func(t *testing.T) {
			hints := canonicalPromptIntent(tc.prompt)
			if !reflect.DeepEqual(hints.RequestedDurationSeconds, tc.want) {
				t.Fatalf("duration hint = %#v, want %#v", hints.RequestedDurationSeconds, tc.want)
			}
		})
	}
}

func TestCanonicalVideoPromptIntentAspectResolutionAndConservativeMobileHint(t *testing.T) {
	cases := []struct {
		prompt string
		field  string
		want   string
	}{
		{prompt: "竖屏风格", field: "aspect", want: "9:16"},
		{prompt: "横屏广告", field: "aspect", want: "16:9"},
		{prompt: "方形封面", field: "aspect", want: "1:1"},
		{prompt: "1K 清晰度", field: "resolution", want: "1k"},
		{prompt: "4K 画质", field: "resolution", want: "4k"},
	}
	for _, tc := range cases {
		t.Run(tc.prompt, func(t *testing.T) {
			hints := canonicalPromptIntent(tc.prompt)
			if tc.field == "aspect" && hints.RequestedAspectRatio != tc.want {
				t.Fatalf("aspect hint = %q, want %q", hints.RequestedAspectRatio, tc.want)
			}
			if tc.field == "resolution" && hints.RequestedResolution != tc.want {
				t.Fatalf("resolution hint = %q, want %q", hints.RequestedResolution, tc.want)
			}
		})
	}
	if hints := canonicalPromptIntent("适合手机看的视频"); hints.RequestedAspectRatio != "" {
		t.Fatalf("mobile wording guessed aspect ratio %q", hints.RequestedAspectRatio)
	}
}

func TestCanonicalVideoPromptIntentModeAndReferenceCount(t *testing.T) {
	if got := canonicalPromptIntent("文生视频").RequestedInputMode; got != "TEXT_TO_VIDEO" {
		t.Fatalf("text mode hint = %q", got)
	}
	if got := canonicalPromptIntent("图生视频").RequestedInputMode; got != "IMAGE_TO_VIDEO" {
		t.Fatalf("image mode hint = %q", got)
	}
	if got := canonicalPromptIntent("视频转视频").RequestedInputMode; got != "VIDEO_TO_VIDEO" {
		t.Fatalf("video mode hint = %q", got)
	}
	hints := canonicalPromptIntent("根据3张图片，并参考第一张图")
	if !hints.ReferenceImageRequested || hints.RequestedReferenceCount == nil || *hints.RequestedReferenceCount != 3 {
		t.Fatalf("reference hints = %#v", hints)
	}
}

func TestCanonicalVideoPromptComplexityUsesCombinedSignals(t *testing.T) {
	complex := canonicalPromptIntent("0-8s 开场，多镜头，添加字幕和旁白")
	if len(complex.ComplexitySignals) < 3 {
		t.Fatalf("complexity signals = %#v", complex.ComplexitySignals)
	}
	structured := canonicalVideoTestStructured()
	request, err := buildCanonicalVideoRequest(canonicalVideoTestInput("0-8s 开场，多镜头，添加字幕和旁白", structured))
	if err != nil {
		t.Fatalf("build complex request: %v", err)
	}
	if !containsCanonicalWarningCode(request.ConsistencyResult.WarningCodes, "VIDEO_PROMPT_COMPLEXITY_WARNING") {
		t.Fatalf("warning codes = %#v", request.ConsistencyResult.WarningCodes)
	}
	compliance, err := buildCanonicalVideoRequest(canonicalVideoTestInput("金融合规商业宣传片", structured))
	if err != nil {
		t.Fatalf("build compliance request: %v", err)
	}
	if containsCanonicalWarningCode(compliance.ConsistencyResult.WarningCodes, "VIDEO_PROMPT_COMPLEXITY_WARNING") {
		t.Fatalf("compliance wording created complexity warning: %#v", compliance.ConsistencyResult.WarningCodes)
	}
}

func TestCanonicalVideoPromptConsistencyWarningCodesAreStable(t *testing.T) {
	prompt := "0-8s 开场，生成30秒视频，9:16，720p，根据参考3张图片，多镜头加字幕和旁白"
	structured := canonicalVideoTestStructured()
	structured.Resolution = "480p"
	request, err := buildCanonicalVideoRequest(canonicalVideoTestInput(prompt, structured))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	want := []string{
		"VIDEO_PROMPT_DURATION_MISMATCH",
		"VIDEO_PROMPT_ASPECT_RATIO_MISMATCH",
		"VIDEO_PROMPT_RESOLUTION_MISMATCH",
		"VIDEO_PROMPT_MODE_MISMATCH",
		"VIDEO_PROMPT_REFERENCE_MISSING",
		"VIDEO_PROMPT_COMPLEXITY_WARNING",
	}
	if !reflect.DeepEqual(request.ConsistencyResult.WarningCodes, want) {
		t.Fatalf("warning codes = %#v, want %#v", request.ConsistencyResult.WarningCodes, want)
	}
	if request.Execution.DurationSeconds != 15 || request.Execution.AspectRatio != "16:9" || request.Execution.Resolution != "480p" || len(request.Execution.ReferenceImages) != 0 {
		t.Fatalf("prompt changed execution = %#v", request.Execution)
	}
}

func intPtr(value int) *int { return &value }

func containsCanonicalWarningCode(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
