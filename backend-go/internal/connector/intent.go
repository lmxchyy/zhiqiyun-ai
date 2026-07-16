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

var imageIntentKeywords = []string{"生成图片", "生成图", "画一张", "做一张图", "电商图", "商品图", "主图", "海报", "配图", "生图"}

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
	return result
}

func detectScene(text string) string {
	for _, scene := range []string{"电商图", "商品图", "主图", "海报", "配图"} {
		if strings.Contains(text, scene) {
			return scene
		}
	}
	return "通用图片"
}
