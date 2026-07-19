package connector

import (
	"strings"
	"testing"
)

func TestRuleIntentRouter(t *testing.T) {
	router := RuleIntentRouter{}
	intent := router.Route("生成 iPhone 17 的电商图", IntentDefaults{Size: "1024x1024", Count: 1})
	if intent.Name != IntentImageGenerate || intent.Subject != "iPhone 17" || intent.Scene != "电商图" || intent.Count != 1 {
		t.Fatalf("unexpected intent: %#v", intent)
	}
	if got := router.Route("你好，今天天气怎么样？", IntentDefaults{}); got.Name != "unknown" {
		t.Fatalf("non-image intent = %#v", got)
	}
}

func TestRuleIntentRouterRecognizesModelInfoQuery(t *testing.T) {
	router := RuleIntentRouter{}
	for _, query := range []string{"使用的是什么模型", "你用什么模型？", "生图模型是什么"} {
		if got := router.Route(query, IntentDefaults{}); got.Name != IntentModelInfo {
			t.Fatalf("model query %q intent = %#v", query, got)
		}
	}
	if got := router.Route("生成图片：一个桌面上的汽车模型", IntentDefaults{}); got.Name != IntentImageGenerate {
		t.Fatalf("image prompt containing model = %#v", got)
	}
}

func TestRuleIntentRouterRecognizesCapabilityInfoQuery(t *testing.T) {
	router := RuleIntentRouter{}
	for _, query := range []string{"你都有什么功能", "你能做什么？", "如何使用"} {
		if got := router.Route(query, IntentDefaults{}); got.Name != IntentCapabilityInfo {
			t.Fatalf("capability query %q intent = %#v", query, got)
		}
	}
}

func TestRuleIntentRouterRecognizesStandaloneVisualPrompt(t *testing.T) {
	router := RuleIntentRouter{}
	prompt := "天蓝色连衣裙背影的少女，在沿海公路疾驰，裙角系着七彩风车。背影正对着镜头，背景渐变珊瑚粉晚霞，近景柏油路面反光呈液态金属质感。采用孟菲斯风格配色，强调霓虹紫晚霞和近景视角。"
	intent := router.Route(prompt, IntentDefaults{})
	if intent.Name != IntentImageGenerate || intent.Subject != prompt || intent.Scene != "通用图片" {
		t.Fatalf("standalone visual prompt = %#v", intent)
	}
}

func TestRuleIntentRouterRejectsLongVisualQuestion(t *testing.T) {
	router := RuleIntentRouter{}
	question := "请分析这个画面的构图、镜头、背景、光影、配色和人物质感，为什么近景视角会让少女和自行车显得更有动感？"
	if got := router.Route(question, IntentDefaults{}); got.Name != "unknown" {
		t.Fatalf("visual question intent = %#v", got)
	}
}

func TestEcommerceImagePromptBuilder(t *testing.T) {
	prompt := (EcommerceImagePromptBuilder{}).Build(Intent{Subject: "咖啡机", Scene: "商品图"})
	for _, expected := range []string{"咖啡机", "商业摄影布光", "1:1", "不虚构具体促销价格", "概念效果图"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt missing %q: %s", expected, prompt)
		}
	}
}

func TestPromptBuilderPreservesDetailedGeneralPrompt(t *testing.T) {
	subject := "天蓝色连衣裙背影的少女，在沿海公路疾驰，背景渐变珊瑚粉晚霞，近景柏油路面反光呈液态金属质感，采用孟菲斯风格配色和霓虹紫光影。"
	prompt := (EcommerceImagePromptBuilder{}).Build(Intent{Subject: subject, Scene: "通用图片"})
	if !strings.Contains(prompt, subject) || strings.Contains(prompt, "电商主图构图") || !strings.Contains(prompt, "不要擅自改成电商主图") {
		t.Fatalf("general visual prompt was not preserved: %s", prompt)
	}
}

func TestRuleIntentRouterRecognizesVideoParameters(t *testing.T) {
	router := RuleIntentRouter{}
	defaults := IntentDefaults{VideoDuration: 5, VideoAspectRatio: "16:9", VideoResolution: "720p", VideoModelID: "mock-video"}
	tests := []struct {
		text       string
		intent     string
		duration   int
		ratio      string
		resolution string
	}{
		{"生成一条15秒的产品宣传视频", IntentVideoGenerate, 15, "16:9", "720p"},
		{"做一个9:16竖屏宣传片", IntentVideoGenerate, 5, "9:16", "720p"},
		{"生成一个720P横屏视频", IntentVideoGenerate, 5, "16:9", "720p"},
		{"用刚才的图片生成10秒视频", IntentVideoImageToVideo, 10, "16:9", "720p"},
	}
	for _, test := range tests {
		got := router.Route(test.text, defaults)
		if got.Name != test.intent || got.Duration != test.duration || got.AspectRatio != test.ratio || got.Resolution != test.resolution {
			t.Fatalf("video intent %q = %#v", test.text, got)
		}
	}
}

func TestRuleIntentRouterRecognizesPPTParameters(t *testing.T) {
	router := RuleIntentRouter{}
	defaults := IntentDefaults{PPTPageCount: 8, PPTTemplateID: "business", PPTTheme: "business", PPTLanguage: "zh"}
	for _, test := range []struct {
		text     string
		pages    int
		audience string
		purpose  string
	}{
		{"生成一份知启云AI招商PPT", 8, "auto", "招商"},
		{"做一个10页的产品介绍PPT", 10, "auto", "产品介绍"},
		{"帮我做8页老板汇报PPT", 8, "老板", "汇报"},
		{"生成一份面向代理商的招商方案", 8, "代理商", "招商"},
	} {
		got := router.Route(test.text, defaults)
		if got.Name != IntentPPTGenerate || got.PageCount != test.pages || got.Audience != test.audience || got.Purpose != test.purpose {
			t.Fatalf("ppt intent %q = %#v", test.text, got)
		}
	}
}

func TestRuleIntentRouterRecognizesImageEditAndTaskQuery(t *testing.T) {
	router := RuleIntentRouter{}
	if got := router.Route("在刚才图片上加上京东logo", IntentDefaults{}); got.Name != IntentImageEdit || !got.ReferenceAssetRequested {
		t.Fatalf("image edit = %#v", got)
	}
	if got := router.Route("查询刚才的任务进度", IntentDefaults{}); got.Name != IntentTaskQuery {
		t.Fatalf("task query = %#v", got)
	}
}
