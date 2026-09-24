package httpserver

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const videoStoryboardComplexBrief = `竖屏9:16，30秒扁平矢量MG动画，深色科技网格背景，深蓝底色，青绿色、橙黄色高亮UI元素，商业支付合规主题。
镜头1（0-8s）：后台监控界面弹出橙色合规预警，卡通男性人物坐在电脑前神情严肃，界面显示高频进钱、高频发钱标签；
镜头2（8-18s）：天平动画，左侧业务流图标，右侧资金流图标，天平不平衡，红色不等号，依次弹出违规过账、面临重罚、梂停营业文字卡片，顶部大字资金流必须与业务流100%匹配；
镜头3（18-30s）：动态链路流程图，从上到下：政府监管机构、持牌机构监管专户、平台企业，虚线箭头流动，高亮文字「平台只传指令，完全不碰钱」「净额申报，优化税负」；
字幕同步配音，画面干净，无多余杂物，科技感，节奏快，抖音信息流短视频，高清，画面和参考3张插画保持统一画风。`

func storyboardCanonical(t *testing.T, prompt string, duration int) canonicalVideoRequest {
	t.Helper()
	return guardCanonical(t, prompt, "TEXT_TO_VIDEO", duration, nil)
}

func TestVideoStoryboardPlannerSimpleBriefIsOneShot(t *testing.T) {
	prompt := "未来科技城市夜景，镜头缓慢推进。"
	canonical := storyboardCanonical(t, prompt, 15)
	before := canonical
	beforeHash, err := canonicalVideoRequestHash(canonical)
	if err != nil {
		t.Fatal(err)
	}

	plan, err := buildVideoStoryboardPlan(canonical, prompt)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Shots) != 1 || plan.Shots[0].TargetDurationSeconds != 15 {
		t.Fatalf("simple plan shots = %#v", plan.Shots)
	}
	if strings.Contains(plan.Shots[0].ProviderPrompt, "15秒") || strings.Contains(plan.Shots[0].ProviderPrompt, "9:16") {
		t.Fatalf("simple prompt leaked execution: %q", plan.Shots[0].ProviderPrompt)
	}
	afterHash, err := canonicalVideoRequestHash(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if beforeHash != afterHash || !reflect.DeepEqual(canonical, before) {
		t.Fatal("planner mutated canonical")
	}
}

func TestVideoStoryboardPlannerTimedThreeShotsSumToParent(t *testing.T) {
	canonical := storyboardCanonical(t, videoStoryboardComplexBrief, 30)
	plan, err := buildVideoStoryboardPlan(canonical, videoStoryboardComplexBrief)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Shots) != 3 {
		t.Fatalf("shot count = %d", len(plan.Shots))
	}
	got := []int{plan.Shots[0].TargetDurationSeconds, plan.Shots[1].TargetDurationSeconds, plan.Shots[2].TargetDurationSeconds}
	if !reflect.DeepEqual(got, []int{8, 10, 12}) {
		t.Fatalf("durations = %#v", got)
	}
	sum := 0
	for _, shot := range plan.Shots {
		sum += shot.TargetDurationSeconds
	}
	if sum != 30 || plan.ParentDurationSeconds != 30 {
		t.Fatalf("sum=%d parent=%d", sum, plan.ParentDurationSeconds)
	}
}

func TestVideoStoryboardPlannerMultiSceneWithoutTimelineStaysConservative(t *testing.T) {
	prompt := "办公室开会讨论方案，然后工厂流水线开始运转。"
	canonical := storyboardCanonical(t, prompt, 15)
	plan, err := buildVideoStoryboardPlan(canonical, prompt)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Shots) != 1 || plan.Shots[0].TargetDurationSeconds != 15 {
		t.Fatalf("conservative split = %#v", plan.Shots)
	}
}

func TestVideoStoryboardPlannerMovesSubtitleVoiceoverOverlayAndInfographic(t *testing.T) {
	canonical := storyboardCanonical(t, videoStoryboardComplexBrief, 30)
	plan, err := buildVideoStoryboardPlan(canonical, videoStoryboardComplexBrief)
	if err != nil {
		t.Fatal(err)
	}
	joinedPrompts := plan.Shots[0].ProviderPrompt + plan.Shots[1].ProviderPrompt + plan.Shots[2].ProviderPrompt
	if strings.Contains(joinedPrompts, "字幕") || strings.Contains(joinedPrompts, "配音") {
		t.Fatalf("av leaked into shots: %q", joinedPrompts)
	}
	if len(plan.PostProcess.Subtitles) == 0 || len(plan.PostProcess.Voiceover) == 0 {
		t.Fatalf("missing av tracks: %#v", plan.PostProcess)
	}
	if len(plan.PostProcess.Overlays) == 0 {
		t.Fatalf("missing overlays: %#v", plan.PostProcess)
	}
	if len(plan.PostProcess.InfographicOverlays) == 0 {
		t.Fatalf("missing infographic: %#v", plan.PostProcess)
	}
	overlayText := ""
	for _, item := range plan.PostProcess.Overlays {
		overlayText += item.Text
	}
	if !strings.Contains(overlayText, "高频进钱") && !strings.Contains(overlayText, "100%匹配") && !strings.Contains(overlayText, "平台只传指令") {
		t.Fatalf("overlay text = %q", overlayText)
	}
}

func TestVideoStoryboardPlannerMovesLogoToBranding(t *testing.T) {
	prompt := "品牌Logo出现在左上角，卡通男性走进办公室。"
	canonical := storyboardCanonical(t, prompt, 6)
	plan, err := buildVideoStoryboardPlan(canonical, prompt)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(plan.Shots[0].ProviderPrompt), "logo") {
		t.Fatalf("logo remained in prompt: %q", plan.Shots[0].ProviderPrompt)
	}
	if len(plan.PostProcess.Branding) == 0 {
		t.Fatalf("branding empty: %#v", plan.PostProcess)
	}
}

func TestVideoStoryboardPlannerStripsCanonicalExecutionTokens(t *testing.T) {
	prompt := "生成30秒9:16 720p TEXT_TO_VIDEO 科技城市夜景，镜头缓慢推进。"
	canonical := storyboardCanonical(t, prompt, 30)
	plan, err := buildVideoStoryboardPlan(canonical, prompt)
	if err != nil {
		t.Fatal(err)
	}
	got := plan.Shots[0].ProviderPrompt
	for _, token := range []string{"30秒", "9:16", "720p", "TEXT_TO_VIDEO"} {
		if strings.Contains(got, token) {
			t.Fatalf("execution token %q in %q", token, got)
		}
	}
}

func TestVideoStoryboardPlannerPreservesComplianceSemantics(t *testing.T) {
	canonical := storyboardCanonical(t, videoStoryboardComplexBrief, 30)
	plan, err := buildVideoStoryboardPlan(canonical, videoStoryboardComplexBrief)
	if err != nil {
		t.Fatal(err)
	}
	blob := plan.Shots[0].ProviderPrompt + " " + plan.Shots[1].ProviderPrompt + " " + plan.Shots[2].ProviderPrompt
	if !strings.Contains(blob, "合规") {
		t.Fatalf("compliance semantics dropped: %q", blob)
	}
	if !strings.Contains(plan.Shots[0].ProviderPrompt, "男性") {
		t.Fatalf("subject dropped: %q", plan.Shots[0].ProviderPrompt)
	}
	if !strings.Contains(plan.Shots[1].ProviderPrompt, "天平") {
		t.Fatalf("balance scene dropped: %q", plan.Shots[1].ProviderPrompt)
	}
}

func TestVideoStoryboardPlannerDurationConflictDoesNotOverrideParent(t *testing.T) {
	canonical := storyboardCanonical(t, videoStoryboardComplexBrief, 15)
	_, err := buildVideoStoryboardPlan(canonical, videoStoryboardComplexBrief)
	if err == nil {
		t.Fatal("expected duration mismatch")
	}
	planErr, ok := err.(*videoStoryboardPlanError)
	if !ok || planErr.Code != videoStoryboardDurationMismatchCode {
		t.Fatalf("err = %v", err)
	}
	if canonical.Execution.DurationSeconds != 15 {
		t.Fatalf("parent duration mutated: %d", canonical.Execution.DurationSeconds)
	}
}

func TestVideoStoryboardPlannerIsDeterministic(t *testing.T) {
	canonical := storyboardCanonical(t, videoStoryboardComplexBrief, 30)
	first, err := buildVideoStoryboardPlan(canonical, videoStoryboardComplexBrief)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		next, err := buildVideoStoryboardPlan(canonical, videoStoryboardComplexBrief)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(first, next) {
			t.Fatalf("iteration %d produced a different plan", i)
		}
	}
}

func TestVideoStoryboardPlannerUnicodeAndMinimalAreSafe(t *testing.T) {
	prompt := "🚀中英 mixed OK，😊 夜景。"
	canonical := storyboardCanonical(t, prompt, 6)
	plan, err := buildVideoStoryboardPlan(canonical, prompt)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Shots) != 1 || plan.Shots[0].TargetDurationSeconds != 6 {
		t.Fatalf("unicode plan = %#v", plan.Shots)
	}

	emptyCanonical := storyboardCanonical(t, "夜景", 6)
	emptyCanonical.Prompt = ""
	empty, err := buildVideoStoryboardPlan(emptyCanonical, "   ")
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.Shots) != 1 || empty.Shots[0].TargetDurationSeconds != 6 {
		t.Fatalf("empty plan = %#v", empty.Shots)
	}
}

func TestVideoStoryboardPlannerUntimedLabeledShotsInferDurations(t *testing.T) {
	prompt := "镜头1：办公室里的商务男性点头。\n镜头2：窗外城市夜景推进。\n镜头3：产品特写旋转。"
	canonical := storyboardCanonical(t, prompt, 30)
	plan, err := buildVideoStoryboardPlan(canonical, prompt)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Shots) != 3 {
		t.Fatalf("shots = %#v", plan.Shots)
	}
	sum := 0
	for _, shot := range plan.Shots {
		sum += shot.TargetDurationSeconds
	}
	if sum != 30 {
		t.Fatalf("inferred sum = %d", sum)
	}
	if !storyboardContainsCode(plan.Consistency.WarningCodes, videoStoryboardDurationInferredCode) {
		t.Fatalf("expected inferred warning, got %#v", plan.Consistency)
	}
}

func TestVideoStoryboardPlannerIsNotWiredIntoGeneration(t *testing.T) {
	roots := []string{
		filepath.Join("api.go"),
		filepath.Join("generation_worker.go"),
		filepath.Join("connector_generation.go"),
	}
	for _, name := range roots {
		payload, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(payload), "buildVideoStoryboardPlan(") {
			t.Fatalf("%s wires the storyboard planner", name)
		}
	}
}

func storyboardContainsCode(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
