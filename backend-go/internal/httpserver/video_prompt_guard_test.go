package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"reflect"
	"strings"
	"testing"

	"xianzhi-ai/backend-go/internal/app/generation"
)

type recordingVideoPromptProvider struct {
	request generation.CreateRequest
}

func (p *recordingVideoPromptProvider) DefaultModel() string { return "mock-video" }
func (p *recordingVideoPromptProvider) Create(_ context.Context, req generation.CreateRequest) (any, error) {
	p.request = cloneGenerationCreateRequest(req)
	return map[string]any{"providerTaskId": "provider-shadow-proof"}, nil
}

func guardCanonical(t *testing.T, prompt, mode string, duration int, refs []string) canonicalVideoRequest {
	t.Helper()
	request, err := buildCanonicalVideoRequest(canonicalVideoRequestInput{
		Prompt: prompt,
		Structured: canonicalVideoStructuredInput{
			Model: "grok-imagine-1.5-video", InputMode: mode, Duration: duration,
			AspectRatio: "9:16", Resolution: "720p", ReferenceImages: refs,
		},
		Capabilities: &canonicalVideoCapabilities{
			SupportedDurations: []int{6, 15, 30}, SupportedAspectRatios: []string{"9:16", "16:9"}, SupportedResolutions: []string{"720p"},
		},
	})
	if err != nil {
		t.Fatalf("build canonical: %v", err)
	}
	return request
}

func guardContainsCode(execution videoPromptExecution, code string) bool {
	for _, value := range execution.TransformationCodes {
		if value == code {
			return true
		}
	}
	return false
}

func TestVideoPromptGuardSimplePromptIsNoop(t *testing.T) {
	prompt := "未来科技城市夜景，镜头缓慢推进。"
	canonical := guardCanonical(t, prompt, "TEXT_TO_VIDEO", 15, nil)
	before := canonical

	execution := buildVideoPromptExecution(videoPromptGuardInput{UserPrompt: prompt, Canonical: canonical})
	if execution.ProviderPrompt != prompt || len(execution.TransformationCodes) != 0 {
		t.Fatalf("simple execution = %#v", execution)
	}
	if execution.UserPromptHash != videoPromptHash(prompt) || execution.ProviderPromptHash != videoPromptHash(prompt) {
		t.Fatalf("unexpected hashes: %#v", execution)
	}
	if !reflect.DeepEqual(canonical, before) {
		t.Fatal("guard mutated canonical request")
	}
}

func TestVideoPromptGuardStripsOnlyRequestLevelExecutionDeclarations(t *testing.T) {
	prompt := "生成30秒9:16 720p的商业动画。商务男性查看合规预警；30秒后出现倒计时。"
	canonical := guardCanonical(t, prompt, "TEXT_TO_VIDEO", 30, nil)
	beforeHash, err := canonicalVideoRequestHash(canonical)
	if err != nil {
		t.Fatal(err)
	}

	execution := buildVideoPromptExecution(videoPromptGuardInput{UserPrompt: prompt, Canonical: canonical})
	if !guardContainsCode(execution, videoPromptStrippedExecutionParamsCode) {
		t.Fatalf("codes = %#v", execution.TransformationCodes)
	}
	for _, removed := range []string{"9:16", "720p"} {
		if strings.Contains(execution.ProviderPrompt, removed) {
			t.Fatalf("provider prompt still contains %q: %q", removed, execution.ProviderPrompt)
		}
	}
	if !strings.Contains(execution.ProviderPrompt, "商务男性") || !strings.Contains(execution.ProviderPrompt, "30秒后") {
		t.Fatalf("semantic/narrative content lost: %q", execution.ProviderPrompt)
	}
	if canonical.Execution.DurationSeconds != 30 || canonical.Execution.AspectRatio != "9:16" || canonical.Execution.Resolution != "720p" {
		t.Fatalf("canonical execution changed: %#v", canonical.Execution)
	}
	afterHash, _ := canonicalVideoRequestHash(canonical)
	if beforeHash != afterHash {
		t.Fatal("canonical hash changed after pure guard")
	}
}

func TestVideoPromptGuardTimelinePreservesOrderAndDoesNotChangeDuration(t *testing.T) {
	prompt := "镜头 1（0-8s）：商务男性查看合规预警。镜头 2（8-18s）：业务流与资金流失衡。镜头 3（18-30s）：监管机构连接平台企业。"
	canonical := guardCanonical(t, prompt, "TEXT_TO_VIDEO", 30, nil)
	execution := buildVideoPromptExecution(videoPromptGuardInput{UserPrompt: prompt, Canonical: canonical})

	if !guardContainsCode(execution, videoPromptSimplifiedTimelineCode) || canonical.Execution.DurationSeconds != 30 {
		t.Fatalf("execution = %#v canonical = %#v", execution, canonical.Execution)
	}
	first, second, third := strings.Index(execution.ProviderPrompt, "商务男性"), strings.Index(execution.ProviderPrompt, "业务流"), strings.Index(execution.ProviderPrompt, "监管机构")
	if first < 0 || second <= first || third <= second || !strings.Contains(execution.ProviderPrompt, "依次表现") {
		t.Fatalf("timeline order not preserved: %q", execution.ProviderPrompt)
	}
}

func TestVideoPromptGuardConflictAndTimelineKeepCanonicalTruth(t *testing.T) {
	prompt := "生成30秒视频。镜头1（0-8s）：开场。镜头2（8-18s）：展示产品。"
	canonical := guardCanonical(t, prompt, "TEXT_TO_VIDEO", 15, nil)
	execution := buildVideoPromptExecution(videoPromptGuardInput{UserPrompt: prompt, Canonical: canonical})

	if canonical.Execution.DurationSeconds != 15 || !guardContainsCode(execution, videoPromptStrippedExecutionParamsCode) {
		t.Fatalf("canonical/guard result = %#v / %#v", canonical, execution)
	}
	if !strings.Contains(strings.Join(canonical.ConsistencyResult.WarningCodes, ","), videoPromptDurationMismatchCode) {
		t.Fatalf("missing original consistency warning: %#v", canonical.ConsistencyResult)
	}
}

func TestVideoPromptGuardMissingReferenceRemovesDependencyButNotWarning(t *testing.T) {
	prompt := "参考3张图片生成商业支付合规动画，商务男性查看合规预警。"
	canonical := guardCanonical(t, prompt, "TEXT_TO_VIDEO", 30, nil)
	execution := buildVideoPromptExecution(videoPromptGuardInput{UserPrompt: prompt, Canonical: canonical})

	if !guardContainsCode(execution, videoPromptRemovedMissingReferenceCode) || strings.Contains(execution.ProviderPrompt, "参考3张图片") {
		t.Fatalf("reference dependency not removed: %#v", execution)
	}
	if !strings.Contains(execution.ProviderPrompt, "商业支付合规") || !strings.Contains(execution.ProviderPrompt, "商务男性") {
		t.Fatalf("unrelated semantics lost: %q", execution.ProviderPrompt)
	}
	if len(canonical.Execution.ReferenceImages) != 0 || !strings.Contains(strings.Join(canonical.ConsistencyResult.WarningCodes, ","), videoPromptReferenceMissingCode) {
		t.Fatalf("canonical reference contract changed: %#v", canonical)
	}
}

func TestVideoPromptGuardRetainsReferenceIntentWhenReferencesExist(t *testing.T) {
	prompt := "参考这张图片生成商业支付合规动画，商务男性查看合规预警。"
	canonical := guardCanonical(t, prompt, "IMAGE_TO_VIDEO", 30, []string{"https://example.com/reference.png"})
	execution := buildVideoPromptExecution(videoPromptGuardInput{UserPrompt: prompt, Canonical: canonical})

	if guardContainsCode(execution, videoPromptRemovedMissingReferenceCode) || !strings.Contains(execution.ProviderPrompt, "参考这张图片") {
		t.Fatalf("valid reference intent changed: %#v", execution)
	}
}

func TestVideoPromptGuardSimplifiesTextSubtitleAndAudioWithoutLosingBusinessSemantics(t *testing.T) {
	prompt := "商务男性查看合规预警。屏幕上精确显示「资金流必须与业务流匹配」。字幕显示：「净额申报，优化税负」。人物精确说出：「请注意风险」，逐字口型同步。"
	canonical := guardCanonical(t, prompt, "TEXT_TO_VIDEO", 30, nil)
	execution := buildVideoPromptExecution(videoPromptGuardInput{UserPrompt: prompt, Canonical: canonical})

	for _, code := range []string{videoPromptSimplifiedTextRenderingCode, videoPromptSimplifiedSubtitleRequirement, videoPromptSimplifiedAudioRequirement} {
		if !guardContainsCode(execution, code) {
			t.Fatalf("missing %s in %#v", code, execution.TransformationCodes)
		}
	}
	for _, preserved := range []string{"商务男性", "合规预警", "业务流", "资金流", "净额申报", "税负"} {
		if !strings.Contains(execution.ProviderPrompt, preserved) {
			t.Fatalf("missing preserved semantic %q in %q", preserved, execution.ProviderPrompt)
		}
	}
	if strings.Contains(execution.ProviderPrompt, "资金流必须与业务流匹配") || strings.Contains(execution.ProviderPrompt, "逐字口型同步") {
		t.Fatalf("precise rendering requirement remains: %q", execution.ProviderPrompt)
	}
}

func TestVideoPromptGuardComplexPromptAndCoreSemanticPreservation(t *testing.T) {
	prompt := "金融合规商业动画，品牌星云支付。镜头1（0-8s）：商务男性查看合规预警。镜头2（8-18s）：业务流与资金流失衡，屏幕上精确显示「资金流必须与业务流匹配」，Logo中出现「星云支付」。镜头3（18-30s）：监管机构、监管专户和平台企业的数据关系。字幕显示：「净额申报，优化税负」。同步中文配音。"
	canonical := guardCanonical(t, prompt, "TEXT_TO_VIDEO", 30, nil)
	execution := buildVideoPromptExecution(videoPromptGuardInput{UserPrompt: prompt, Canonical: canonical})

	if !guardContainsCode(execution, videoPromptComplexityReducedCode) {
		t.Fatalf("complexity code missing: %#v", execution.TransformationCodes)
	}
	for _, semantic := range []string{"金融", "合规", "商业", "品牌星云支付", "商务男性", "业务流", "资金流", "监管机构", "监管专户", "平台企业"} {
		if !strings.Contains(execution.ProviderPrompt, semantic) {
			t.Fatalf("semantic %q missing: %q", semantic, execution.ProviderPrompt)
		}
	}
}

func TestVideoPromptGuardIsDeterministicAndVersioned(t *testing.T) {
	prompt := "生成30秒9:16视频，屏幕上精确显示「合规预警」。"
	canonical := guardCanonical(t, prompt, "TEXT_TO_VIDEO", 30, nil)
	want := buildVideoPromptExecution(videoPromptGuardInput{UserPrompt: prompt, Canonical: canonical})
	for i := 0; i < 100; i++ {
		got := buildVideoPromptExecution(videoPromptGuardInput{UserPrompt: prompt, Canonical: canonical})
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("iteration %d got %#v want %#v", i, got, want)
		}
	}
	v2 := buildVideoPromptExecutionForVersion(videoPromptGuardInput{UserPrompt: prompt, Canonical: canonical}, "video-prompt-guard-v2")
	if v2.GuardVersion == want.GuardVersion || v2.ProviderPrompt != want.ProviderPrompt || v2.ProviderPromptHash != want.ProviderPromptHash {
		t.Fatalf("version must be metadata-only for equal rules: v1=%#v v2=%#v", want, v2)
	}
}

func TestVideoPromptGuardHashesAndFingerprintRemainSeparated(t *testing.T) {
	firstPrompt := "未来科技城市夜景，镜头缓慢推进。"
	secondPrompt := "未来科技城市清晨，镜头缓慢推进。"
	canonical := guardCanonical(t, firstPrompt, "TEXT_TO_VIDEO", 15, nil)
	beforeCanonicalHash, _ := canonicalVideoRequestHash(canonical)

	first := buildVideoPromptExecution(videoPromptGuardInput{UserPrompt: firstPrompt, Canonical: canonical})
	second := buildVideoPromptExecution(videoPromptGuardInput{UserPrompt: secondPrompt, Canonical: guardCanonical(t, secondPrompt, "TEXT_TO_VIDEO", 15, nil)})
	if first.UserPromptHash == second.UserPromptHash || first.ProviderPromptHash == second.ProviderPromptHash {
		t.Fatalf("prompt hashes did not distinguish prompts: %#v / %#v", first, second)
	}

	req := generation.CreateRequest{Type: canonical.Execution.InputMode, Prompt: firstPrompt, Model: canonical.Execution.Model, Params: map[string]any{}}
	if err := persistCanonicalVideoRequest(&req, canonical); err != nil {
		t.Fatal(err)
	}
	fingerprintBefore, err := videoRequestFingerprint("task_guard", "configured", "video", req.Model, req.Params)
	if err != nil {
		t.Fatal(err)
	}
	req.Params["video_prompt_execution"] = first
	fingerprintAfter, err := videoRequestFingerprint("task_guard", "configured", "video", req.Model, req.Params)
	if err != nil {
		t.Fatal(err)
	}
	afterCanonicalHash, _ := canonicalVideoRequestHash(canonical)
	if fingerprintBefore != fingerprintAfter || beforeCanonicalHash != afterCanonicalHash {
		t.Fatalf("guard metadata affected fingerprint/hash: %s/%s %s/%s", fingerprintBefore, fingerprintAfter, beforeCanonicalHash, afterCanonicalHash)
	}

	changedCanonical := canonical
	changedCanonical.ConsistencyResult.WarningCodes = append(changedCanonical.ConsistencyResult.WarningCodes, "UI_ONLY_DIAGNOSTIC")
	changedHash, _ := canonicalVideoRequestHash(changedCanonical)
	if beforeCanonicalHash != changedHash {
		t.Fatal("warning diagnostics changed canonical hash")
	}
}

func TestVideoPromptExecutionSnapshotIsPersistedAndShadowDoesNotChangeProviderPrompt(t *testing.T) {
	_, data, _, prepared := canonicalDownstreamPreparedRequest(t)
	canonicalHash := stringValue(prepared.Params[canonicalVideoHashParam])
	fingerprintBefore, err := videoRequestFingerprint("task-shadow", "mock", "video", prepared.Model, prepared.Params)
	if err != nil {
		t.Fatal(err)
	}
	quoteBefore, err := generationQuoteForRequest(prepared, data)
	if err != nil {
		t.Fatal(err)
	}
	if !ensureVideoPromptExecutionSnapshot(&prepared) {
		t.Fatal("expected canonical task to receive a prompt execution snapshot")
	}
	snapshot, ok := videoPromptExecutionFromParams(prepared.Params, prepared.Prompt)
	if !ok {
		t.Fatalf("missing valid persisted snapshot: %#v", prepared.Params)
	}
	if snapshot.GuardVersion != videoPromptGuardVersion || snapshot.ProviderPrompt == "" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	fingerprintAfter, err := videoRequestFingerprint("task-shadow", "mock", "video", prepared.Model, prepared.Params)
	if err != nil {
		t.Fatal(err)
	}
	quoteAfter, err := generationQuoteForRequest(prepared, data)
	if err != nil {
		t.Fatal(err)
	}
	if fingerprintBefore != fingerprintAfter || quoteBefore.RequiredPoints != quoteAfter.RequiredPoints || canonicalHash != stringValue(prepared.Params[canonicalVideoHashParam]) {
		t.Fatalf("shadow snapshot changed request semantics fingerprint=%q/%q quote=%#v/%#v canonical=%q/%q", fingerprintBefore, fingerprintAfter, quoteBefore, quoteAfter, canonicalHash, stringValue(prepared.Params[canonicalVideoHashParam]))
	}
	providerRequest, canonicalPath := canonicalVideoDownstreamRequest(prepared)
	if !canonicalPath || providerRequest.Prompt != prepared.Prompt || providerRequest.Prompt == snapshot.ProviderPrompt && snapshot.ProviderPrompt != prepared.Prompt {
		t.Fatalf("shadow transport changed provider prompt request=%q task=%q snapshot=%q", providerRequest.Prompt, prepared.Prompt, snapshot.ProviderPrompt)
	}
	redacted := redactVideoPromptExecution(generationTask{Prompt: prepared.Prompt, Params: prepared.Params})
	if _, exposed := redacted.Params[videoPromptExecutionParam]; exposed {
		t.Fatal("provider prompt snapshot leaked to a public task projection")
	}
}

func TestVideoPromptExecutionShadowProviderReceivesOriginalCanonicalPrompt(t *testing.T) {
	_, _, _, prepared := canonicalDownstreamPreparedRequest(t)
	if !ensureVideoPromptExecutionSnapshot(&prepared) {
		t.Fatal("expected prompt snapshot")
	}
	snapshot, ok := videoPromptExecutionFromParams(prepared.Params, prepared.Prompt)
	if !ok || snapshot.ProviderPrompt == prepared.Prompt {
		t.Fatalf("fixture must deterministically transform the provider prompt: %#v", snapshot)
	}
	providerRequest, canonicalPath := canonicalVideoDownstreamRequest(prepared)
	if !canonicalPath {
		t.Fatal("expected canonical downstream request")
	}
	provider := &recordingVideoPromptProvider{}
	service := generation.NewServiceWithOptions(generation.ServiceOptions{VideoProvider: provider})
	if _, err := service.PrepareVideoTask(context.Background(), providerRequest); err != nil {
		t.Fatal(err)
	}
	if provider.request.Prompt != prepared.Prompt || provider.request.Prompt == snapshot.ProviderPrompt {
		t.Fatalf("shadow provider prompt=%q task=%q stored provider_prompt=%q", provider.request.Prompt, prepared.Prompt, snapshot.ProviderPrompt)
	}
}

func TestVideoPromptExecutionPublicProjectionsAndLogsNeverExposeProviderPrompt(t *testing.T) {
	_, _, _, prepared := canonicalDownstreamPreparedRequest(t)
	if !ensureVideoPromptExecutionSnapshot(&prepared) {
		t.Fatal("expected prompt snapshot")
	}
	snapshot, ok := videoPromptExecutionFromParams(prepared.Params, prepared.Prompt)
	if !ok {
		t.Fatal("missing snapshot")
	}
	// Use a recognisable value so every external serialization surface has an
	// unambiguous non-leak assertion, rather than only checking the field name.
	snapshot.ProviderPrompt = "PRIVATE_PROVIDER_PROMPT_MUST_NOT_LEAK"
	snapshot.ProviderPromptHash = videoPromptHash(snapshot.ProviderPrompt)
	prepared.Params[videoPromptExecutionParam] = snapshot
	task := generationTask{ID: "task-private", Prompt: prepared.Prompt, Params: prepared.Params}

	for name, value := range map[string]any{
		"create/detail/retry task": redactVideoPromptExecution(task),
		"task list":                redactVideoPromptExecutionTasks([]generationTask{task}),
		"workspace/history":        compactWorkspaceListTasks([]generationTask{task}),
		"dashboard":                limitGenerationTasks([]generationTask{task}, 30),
		"connector result":         redactVideoPromptExecutionRequest(prepared),
	} {
		payload, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		if bytes.Contains(payload, []byte(snapshot.ProviderPrompt)) || bytes.Contains(payload, []byte(videoPromptExecutionParam)) {
			t.Fatalf("%s leaked private snapshot: %s", name, payload)
		}
	}

	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previous)
	videoPromptExecutionTelemetry(task, prepared.Params)
	if strings.Contains(logs.String(), snapshot.ProviderPrompt) || strings.Contains(logs.String(), videoPromptExecutionParam) {
		t.Fatalf("shadow telemetry leaked provider prompt: %s", logs.String())
	}
	if !strings.Contains(logs.String(), snapshot.ProviderPromptHash) {
		t.Fatalf("shadow telemetry omitted safe hash: %s", logs.String())
	}
}

func TestVideoPromptExecutionStoredSnapshotSurvivesRecoveryRedeliveryConnectorAndRetry(t *testing.T) {
	_, _, _, prepared := canonicalDownstreamPreparedRequest(t)
	if !ensureVideoPromptExecutionSnapshot(&prepared) {
		t.Fatal("expected initial snapshot")
	}
	snapshot, ok := videoPromptExecutionFromParams(prepared.Params, prepared.Prompt)
	if !ok {
		t.Fatal("missing initial snapshot")
	}
	snapshot.GuardVersion = "video-prompt-guard-v1"
	snapshot.ProviderPrompt = "P1 stored provider prompt"
	snapshot.ProviderPromptHash = videoPromptHash(snapshot.ProviderPrompt)
	prepared.Params[videoPromptExecutionParam] = snapshot
	task := generationTask{ID: "task-v1", Prompt: prepared.Prompt, Params: cloneAnyMap(prepared.Params)}

	// Stale recovery and MQ redelivery both reconstruct their requests from the
	// durable task Params. Neither path calls the builder.
	for _, name := range []string{"stale recovery", "mq redelivery"} {
		request := generation.CreateRequest{Prompt: task.Prompt, Params: cloneAnyMap(task.Params)}
		got, ok := videoPromptExecutionFromParams(request.Params, request.Prompt)
		if !ok || !reflect.DeepEqual(got, snapshot) {
			t.Fatalf("%s recomputed or changed snapshot: %#v", name, got)
		}
	}

	// Connector idempotent retry explicitly replaces its transient new-request
	// data with the persisted task snapshot.
	connectorRetry := generation.CreateRequest{Prompt: task.Prompt, Params: map[string]any{}}
	if !reuseVideoPromptExecutionSnapshot(&connectorRetry, task.Params) {
		t.Fatal("connector retry did not reuse stored snapshot")
	}
	if got, _ := videoPromptExecutionFromParams(connectorRetry.Params, connectorRetry.Prompt); !reflect.DeepEqual(got, snapshot) {
		t.Fatalf("connector retry snapshot=%#v", got)
	}

	// Current terminal user retry creates a child task (retryOf), but its
	// request starts from the original task Params; startRetriedGenerationTask
	// therefore retains P1 rather than applying a future Guard implementation.
	childRetry := generation.CreateRequest{Prompt: task.Prompt, Params: cloneAnyMap(task.Params)}
	if !ensureVideoPromptExecutionSnapshot(&childRetry) {
		t.Fatal("retry child snapshot unavailable")
	}
	if got, _ := videoPromptExecutionFromParams(childRetry.Params, childRetry.Prompt); !reflect.DeepEqual(got, snapshot) {
		t.Fatalf("retry child changed snapshot: %#v", got)
	}
}

func TestVideoPromptExecutionInboundSnapshotIsDiscarded(t *testing.T) {
	_, _, _, prepared := canonicalDownstreamPreparedRequest(t)
	prepared.Params[videoPromptExecutionParam] = videoPromptExecution{
		SchemaVersion: videoPromptExecutionVersion, GuardVersion: "attacker-selected-v999",
		UserPromptHash: videoPromptHash(prepared.Prompt), ProviderPrompt: "attacker prompt", ProviderPromptHash: videoPromptHash("attacker prompt"),
	}
	removeUntrustedVideoPromptExecution(&prepared)
	if _, exists := prepared.Params[videoPromptExecutionParam]; exists {
		t.Fatal("inbound snapshot was not discarded")
	}
	if !ensureVideoPromptExecutionSnapshot(&prepared) {
		t.Fatal("expected server snapshot")
	}
	snapshot, ok := videoPromptExecutionFromParams(prepared.Params, prepared.Prompt)
	if !ok || snapshot.GuardVersion != videoPromptGuardVersion || snapshot.ProviderPrompt == "attacker prompt" {
		t.Fatalf("untrusted snapshot survived: %#v", snapshot)
	}
}

func TestVideoPromptExecutionSnapshotIsDeterministicAcrossConcurrentCreationPreparation(t *testing.T) {
	_, _, _, prepared := canonicalDownstreamPreparedRequest(t)
	delete(prepared.Params, videoPromptExecutionParam)
	const workers = 24
	results := make(chan videoPromptExecution, workers)
	for index := 0; index < workers; index++ {
		go func() {
			candidate := cloneGenerationCreateRequest(prepared)
			if !ensureVideoPromptExecutionSnapshot(&candidate) {
				results <- videoPromptExecution{}
				return
			}
			snapshot, _ := videoPromptExecutionFromParams(candidate.Params, candidate.Prompt)
			results <- snapshot
		}()
	}
	var expected videoPromptExecution
	for index := 0; index < workers; index++ {
		got := <-results
		if got.ProviderPrompt == "" {
			t.Fatal("concurrent preparation produced no snapshot")
		}
		if index == 0 {
			expected = got
		} else if !reflect.DeepEqual(got, expected) {
			t.Fatalf("concurrent preparations diverged: %#v / %#v", got, expected)
		}
	}
}

func TestVideoPromptExecutionSnapshotReusesStoredVersionAndLegacyFallsBack(t *testing.T) {
	_, _, _, prepared := canonicalDownstreamPreparedRequest(t)
	if !ensureVideoPromptExecutionSnapshot(&prepared) {
		t.Fatal("expected canonical task snapshot")
	}
	snapshot, ok := videoPromptExecutionFromParams(prepared.Params, prepared.Prompt)
	if !ok {
		t.Fatal("missing snapshot")
	}
	// Simulate a task created before a future Guard implementation. A retry,
	// restart or connector redelivery must keep this stored version verbatim.
	snapshot.GuardVersion = "video-prompt-guard-v2"
	prepared.Params[videoPromptExecutionParam] = snapshot
	if !ensureVideoPromptExecutionSnapshot(&prepared) {
		t.Fatal("expected stored snapshot to be reused")
	}
	reused, ok := videoPromptExecutionFromParams(prepared.Params, prepared.Prompt)
	if !ok || reused.GuardVersion != "video-prompt-guard-v2" || !reflect.DeepEqual(reused, snapshot) {
		t.Fatalf("stored snapshot was recomputed: %#v", reused)
	}

	legacy := generation.CreateRequest{Type: "TEXT_TO_VIDEO", Prompt: "legacy prompt", Model: "mock-video", Params: map[string]any{"duration": 5, "aspect_ratio": "16:9", "resolution": "480p"}}
	if ensureVideoPromptExecutionSnapshot(&legacy) {
		t.Fatal("legacy task unexpectedly received a canonical prompt snapshot")
	}
	if projected, canonicalPath := canonicalVideoDownstreamRequest(legacy); canonicalPath || projected.Prompt != legacy.Prompt {
		t.Fatalf("legacy fallback changed prompt/path: %#v canonical=%t", projected, canonicalPath)
	}
}

func TestVideoPromptGuardOddUnicodeAndMinimalInputAreSafe(t *testing.T) {
	prompt := "\r\n  ✨\t商务男性🙂\r\n屏幕上精确显示「合规」\r\n"
	canonical := guardCanonical(t, strings.TrimSpace(prompt), "TEXT_TO_VIDEO", 15, nil)
	first := buildVideoPromptExecution(videoPromptGuardInput{UserPrompt: prompt, Canonical: canonical})
	second := buildVideoPromptExecution(videoPromptGuardInput{UserPrompt: prompt, Canonical: canonical})
	if first.ProviderPrompt == "" || first.ProviderPrompt != second.ProviderPrompt || first.ProviderPromptHash != second.ProviderPromptHash {
		t.Fatalf("unicode result is not stable: %#v / %#v", first, second)
	}

	empty := buildVideoPromptExecution(videoPromptGuardInput{Canonical: canonicalVideoRequest{}})
	if empty.ProviderPrompt != "" || empty.UserPromptHash != videoPromptHash("") || empty.ProviderPromptHash != videoPromptHash("") {
		t.Fatalf("empty input result = %#v", empty)
	}
}
