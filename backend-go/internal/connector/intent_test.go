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
