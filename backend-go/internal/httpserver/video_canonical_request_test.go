package httpserver

import (
	"bytes"
	"strings"
	"testing"
)

func canonicalVideoTestCapabilities() *canonicalVideoCapabilities {
	return &canonicalVideoCapabilities{
		SupportedDurations:    []int{5, 10, 15, 30},
		SupportedResolutions:  []string{"480p", "720p"},
		SupportedAspectRatios: []string{"16:9", "9:16", "1:1"},
	}
}

func canonicalVideoTestInput(prompt string, structured canonicalVideoStructuredInput) canonicalVideoRequestInput {
	return canonicalVideoRequestInput{Prompt: prompt, Structured: structured, Capabilities: canonicalVideoTestCapabilities()}
}

func canonicalVideoTestStructured() canonicalVideoStructuredInput {
	return canonicalVideoStructuredInput{
		Model:       "grok-imagine-1.5-video",
		InputMode:   "TEXT_TO_VIDEO",
		Duration:    15,
		AspectRatio: "16:9",
		Resolution:  "720p",
	}
}

func TestCanonicalVideoRequestStructuredInput(t *testing.T) {
	request, err := buildCanonicalVideoRequest(canonicalVideoTestInput("a cinematic product reveal", canonicalVideoTestStructured()))
	if err != nil {
		t.Fatalf("build canonical request: %v", err)
	}
	if request.SchemaVersion != canonicalVideoRequestVersion {
		t.Fatalf("schema version = %d", request.SchemaVersion)
	}
	if request.Execution.DurationSeconds != 15 || request.Execution.AspectRatio != "16:9" || request.Execution.Resolution != "720p" {
		t.Fatalf("execution = %#v", request.Execution)
	}
	if request.ConsistencyResult.Status != "ok" {
		t.Fatalf("consistency status = %q", request.ConsistencyResult.Status)
	}
}

func TestCanonicalVideoRequestQualityAliasNormalizesToResolution(t *testing.T) {
	structured := canonicalVideoTestStructured()
	structured.Resolution = nil
	structured.Quality = "720P"
	request, err := buildCanonicalVideoRequest(canonicalVideoTestInput("product reveal", structured))
	if err != nil {
		t.Fatalf("build canonical request: %v", err)
	}
	if request.Execution.Resolution != "720p" {
		t.Fatalf("resolution = %q", request.Execution.Resolution)
	}
}

func TestCanonicalVideoRequestPromptConflictDoesNotOverwriteExecution(t *testing.T) {
	structured := canonicalVideoTestStructured()
	request, err := buildCanonicalVideoRequest(canonicalVideoTestInput("生成30秒视频，9:16，1080p", structured))
	if err != nil {
		t.Fatalf("build canonical request: %v", err)
	}
	if request.Execution.DurationSeconds != 15 || request.Execution.AspectRatio != "16:9" || request.Execution.Resolution != "720p" {
		t.Fatalf("prompt changed execution = %#v", request.Execution)
	}
	if len(request.ConsistencyResult.Warnings) != 3 {
		t.Fatalf("warnings = %#v, want duration/aspect/resolution conflicts", request.ConsistencyResult.Warnings)
	}
}

func TestCanonicalVideoRequestTimelineDoesNotCreateDurationHint(t *testing.T) {
	request, err := buildCanonicalVideoRequest(canonicalVideoTestInput("0-8s 开场，8-18s 展示，18-30s 结尾", canonicalVideoTestStructured()))
	if err != nil {
		t.Fatalf("build canonical request: %v", err)
	}
	if request.PromptIntentHints.RequestedDurationSeconds != nil {
		t.Fatalf("timeline created duration hint: %#v", request.PromptIntentHints.RequestedDurationSeconds)
	}
}

func TestCanonicalVideoRequestReferenceIntentDoesNotInjectReferences(t *testing.T) {
	request, err := buildCanonicalVideoRequest(canonicalVideoTestInput("根据3张参考图片生成视频", canonicalVideoTestStructured()))
	if err != nil {
		t.Fatalf("build canonical request: %v", err)
	}
	if len(request.Execution.ReferenceImages) != 0 {
		t.Fatalf("references were injected: %#v", request.Execution.ReferenceImages)
	}
	if !request.PromptIntentHints.ReferenceImageRequested {
		t.Fatal("reference image intent was not detected")
	}
	foundReferenceWarning := false
	for _, item := range request.ConsistencyResult.Warnings {
		if item.Code == "VIDEO_PROMPT_REFERENCE_MISSING" {
			foundReferenceWarning = true
			break
		}
	}
	if !foundReferenceWarning {
		t.Fatalf("warnings = %#v", request.ConsistencyResult.Warnings)
	}
}

func TestCanonicalVideoRequestRepresentationAndHashAreDeterministic(t *testing.T) {
	requestA, err := buildCanonicalVideoRequest(canonicalVideoTestInput("product reveal", canonicalVideoTestStructured()))
	if err != nil {
		t.Fatalf("build request A: %v", err)
	}
	requestB, err := buildCanonicalVideoRequest(canonicalVideoTestInput("product reveal", canonicalVideoTestStructured()))
	if err != nil {
		t.Fatalf("build request B: %v", err)
	}
	representationA, err := canonicalVideoRequestRepresentation(requestA)
	if err != nil {
		t.Fatalf("representation A: %v", err)
	}
	representationB, err := canonicalVideoRequestRepresentation(requestB)
	if err != nil {
		t.Fatalf("representation B: %v", err)
	}
	if !bytes.Equal(representationA, representationB) {
		t.Fatalf("representations differ:\n%s\n%s", representationA, representationB)
	}
	hashA, err := canonicalVideoRequestHash(requestA)
	if err != nil {
		t.Fatalf("hash A: %v", err)
	}
	hashB, err := canonicalVideoRequestHash(requestB)
	if err != nil {
		t.Fatalf("hash B: %v", err)
	}
	if hashA == "" || hashA != hashB {
		t.Fatalf("hashes = %q / %q", hashA, hashB)
	}
	requestB.PromptIntentHints.RequestedDurationSeconds = nil
	requestB.ConsistencyResult.Warnings = append(requestB.ConsistencyResult.Warnings, canonicalConsistencyWarning{
		Code: "UI_ONLY_DIAGNOSTIC", Field: "prompt", Severity: "warning",
	})
	hashDiagnosticsChanged, err := canonicalVideoRequestHash(requestB)
	if err != nil {
		t.Fatalf("hash with changed diagnostics: %v", err)
	}
	if hashA != hashDiagnosticsChanged {
		t.Fatalf("diagnostics changed execution hash: %q / %q", hashA, hashDiagnosticsChanged)
	}
}

func TestCanonicalVideoRequestRejectsMissingAndInvalidFields(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*canonicalVideoStructuredInput)
		want   string
	}{
		{name: "missing model", mutate: func(value *canonicalVideoStructuredInput) { value.Model = nil }, want: "model is required"},
		{name: "invalid mode", mutate: func(value *canonicalVideoStructuredInput) { value.InputMode = "VIDEO_TO_VIDEO" }, want: "input_mode is invalid"},
		{name: "invalid duration", mutate: func(value *canonicalVideoStructuredInput) { value.Duration = 0 }, want: "duration is invalid"},
		{name: "unsupported resolution", mutate: func(value *canonicalVideoStructuredInput) { value.Resolution = "4k" }, want: "resolution is not supported"},
		{name: "unsupported ratio", mutate: func(value *canonicalVideoStructuredInput) { value.AspectRatio = "4:3" }, want: "aspect_ratio is not supported"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			structured := canonicalVideoTestStructured()
			tc.mutate(&structured)
			_, err := buildCanonicalVideoRequest(canonicalVideoTestInput("product reveal", structured))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestCanonicalVideoRequestIntentAndConsistencyAreNotExecution(t *testing.T) {
	request, err := buildCanonicalVideoRequest(canonicalVideoTestInput("30秒 9:16 1080p，根据参考图片", canonicalVideoTestStructured()))
	if err != nil {
		t.Fatalf("build canonical request: %v", err)
	}
	if request.Execution.DurationSeconds != 15 || request.Execution.AspectRatio != "16:9" || request.Execution.Resolution != "720p" || len(request.Execution.ReferenceImages) != 0 {
		t.Fatalf("execution was changed by hints: %#v", request.Execution)
	}
	if request.PromptIntentHints.RequestedDurationSeconds == nil || *request.PromptIntentHints.RequestedDurationSeconds != 30 {
		t.Fatalf("duration hint = %#v", request.PromptIntentHints.RequestedDurationSeconds)
	}
	if request.ConsistencyResult.Status != "warning" {
		t.Fatalf("consistency = %#v", request.ConsistencyResult)
	}
}
