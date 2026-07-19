package connector

import (
	"regexp"
	"strings"
)

const (
	IntentImageGenerate = "image.generate"
	IntentImageEdit     = "image.edit"
)

type Intent struct {
	Name    string `json:"intent"`
	Subject string `json:"subject"`
	Scene   string `json:"scene"`
	Count   int    `json:"count"`
	Size    string `json:"size"`
}

type IntentRouter interface {
	Route(text string, defaults IntentDefaults) Intent
}

type IntentDefaults struct {
	Size  string
	Count int
}

type RuleIntentRouter struct{}

var imageIntentKeywords = []string{"生成图片", "生成图", "生成照片", "画一张", "做一张图", "电商图", "商品图", "主图", "海报", "配图", "生图"}

var visualPromptStrongKeywords = []string{
	"镜头", "画面", "构图", "视角", "景深", "配色", "色调", "光影", "光线", "质感", "摄影", "渲染", "背景", "前景",
}

var visualPromptDetailKeywords = []string{
	"人物", "女孩", "少女", "男孩", "商品", "产品", "服装", "天空", "海面", "山谷", "公路", "瀑布", "建筑", "汽车", "自行车", "色彩", "霓虹", "特写", "近景", "远景", "全景",
}

var nonGenerationRequestKeywords = []string{
	"分析", "解释", "总结", "翻译", "是什么", "为什么", "怎么", "如何", "能否", "可以吗", "介绍",
}

var intentNoise = regexp.MustCompile(`(?i)(请|帮我|帮忙|给我|生成图片|生成图|生成|画一张|画|做一张图|做一张|生图|一张|的电商图|电商图|商品图|主图|海报|配图)[，,。.!！?？\s]*`)

func (RuleIntentRouter) Route(text string, defaults IntentDefaults) Intent {
	trimmed := strings.TrimSpace(text)
	result := Intent{Name: "unknown", Count: defaults.Count, Size: strings.TrimSpace(defaults.Size)}
	if result.Count <= 0 {
		result.Count = 1
	}
	if result.Size == "" {
		result.Size = "1024x1024"
	}
	for _, keyword := range imageIntentKeywords {
		if !strings.Contains(trimmed, keyword) {
			continue
		}
		result.Name = IntentImageGenerate
		result.Scene = detectScene(trimmed)
		result.Subject = strings.Trim(intentNoise.ReplaceAllString(trimmed, " "), " ，,。.!！?？")
		if result.Subject == "" {
			result.Subject = "商品"
		}
		return result
	}
	if looksLikeStandaloneVisualPrompt(trimmed) {
		result.Name = IntentImageGenerate
		result.Scene = detectScene(trimmed)
		result.Subject = trimmed
		return result
	}
	return result
}

func looksLikeStandaloneVisualPrompt(text string) bool {
	if len([]rune(strings.TrimSpace(text))) < 40 {
		return false
	}
	if strings.ContainsAny(text, "?？") || containsAny(text, nonGenerationRequestKeywords) {
		return false
	}
	return countContained(text, visualPromptStrongKeywords) >= 3 && countContained(text, visualPromptDetailKeywords) >= 2
}

func containsAny(text string, keywords []string) bool {
	return countContained(text, keywords) > 0
}

func countContained(text string, keywords []string) int {
	count := 0
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			count++
		}
	}
	return count
}

func detectScene(text string) string {
	for _, scene := range []string{"电商图", "商品图", "主图", "海报", "配图"} {
		if strings.Contains(text, scene) {
			return scene
		}
	}
	return "通用图片"
}
