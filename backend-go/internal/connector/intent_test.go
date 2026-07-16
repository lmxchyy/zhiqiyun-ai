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
	if got := router.Route("你好，你能做什么？", IntentDefaults{}); got.Name != "unknown" {
		t.Fatalf("non-image intent = %#v", got)
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
