package connector

import (
	"regexp"
	"strconv"
	"strings"
)

const (
	IntentImageGenerate     = "image.generate"
	IntentImageEdit         = "image.edit"
	IntentVideoGenerate     = "video.generate"
	IntentVideoImageToVideo = "video.image_to_video"
	IntentPPTGenerate       = "ppt.generate"
	IntentTaskQuery         = "task.query"
	IntentHelp              = "help"
	IntentUnknown           = "unknown"
	IntentModelInfo         = "model.info"
	IntentCapabilityInfo    = "capability.info"
)

type Intent struct {
	Name                    string `json:"intent"`
	Subject                 string `json:"subject"`
	Topic                   string `json:"topic"`
	Scene                   string `json:"scene"`
	Count                   int    `json:"count"`
	Size                    string `json:"size"`
	Duration                int    `json:"duration"`
	AspectRatio             string `json:"aspect_ratio"`
	Resolution              string `json:"resolution"`
	ModelID                 string `json:"model_id"`
	Style                   string `json:"style"`
	GenerationMode          string `json:"generation_mode"`
	ReferenceAssetRequested bool   `json:"reference_asset_requested"`
	PageCount               int    `json:"page_count"`
	Audience                string `json:"audience"`
	Purpose                 string `json:"purpose"`
	TemplateID              string `json:"template_id"`
	Theme                   string `json:"theme"`
	Language                string `json:"language"`
	UseEnterpriseLogo       bool   `json:"use_enterprise_logo"`
	UseEnterpriseKnowledge  bool   `json:"use_enterprise_knowledge"`
}

type IntentRouter interface {
	Route(string, IntentDefaults) Intent
}

type IntentDefaults struct {
	Size                   string
	Count                  int
	VideoDuration          int
	VideoAspectRatio       string
	VideoResolution        string
	VideoModelID           string
	PPTPageCount           int
	PPTTemplateID          string
	PPTTheme               string
	PPTLanguage            string
	UseEnterpriseLogo      bool
	UseEnterpriseKnowledge bool
}

type RuleIntentRouter struct{}

var (
	imageIntentKeywords          = []string{"生成图片", "生成图", "生成照片", "画一张", "做一张图", "电商图", "商品图", "主图", "海报", "配图", "生图"}
	videoIntentKeywords          = []string{"生成视频", "做视频", "制作视频", "宣传视频", "宣传片", "短视频", "做成视频", "生成一条", "制作一条", "旋转视频"}
	pptIntentKeywords            = []string{"ppt", "演示文稿", "招商方案", "汇报材料", "幻灯片"}
	taskQueryKeywords            = []string{"任务进度", "任务状态", "进度如何", "进度怎么样", "完成了吗", "做好了吗", "刚才的任务", "上一任务", "最近任务"}
	referenceWords               = []string{"刚才", "上一张", "上张", "上图", "上面", "这张图", "这张图片", "刚生成", "之前生成", "前一张"}
	editWords                    = []string{"加上", "添加", "加个", "加一个", "修改", "改成", "换成", "替换", "去掉", "删除", "移除", "调整", "编辑", "logo", "水印", "文字"}
	visualPromptStrongKeywords   = []string{"镜头", "画面", "构图", "视角", "景深", "配色", "色调", "光影", "光线", "质感", "摄影", "渲染", "背景", "前景"}
	visualPromptDetailKeywords   = []string{"人物", "女孩", "少女", "男孩", "商品", "产品", "服装", "天空", "海面", "山谷", "公路", "瀑布", "建筑", "汽车", "自行车", "色彩", "霓虹", "特写", "近景", "远景", "全景"}
	nonGenerationRequestKeywords = []string{"分析", "解释", "总结", "翻译", "是什么", "为什么", "怎么", "如何", "能否", "可以吗", "介绍"}
	modelInfoQueryKeywords       = []string{"使用的是什么模型", "用的是什么模型", "当前使用什么模型", "当前是什么模型", "现在使用什么模型", "现在是什么模型", "生图使用什么模型", "生图用什么模型", "生图模型是什么", "模型名称是什么", "你是什么模型", "你用什么模型"}
	capabilityInfoQueryKeywords  = []string{"你都有什么功能", "你有什么功能", "都有什么功能", "有什么功能", "你会什么", "你能做什么", "能做什么", "支持哪些功能", "支持什么功能", "怎么使用", "如何使用", "使用帮助", "功能介绍"}
	intentNoise                  = regexp.MustCompile(`(?i)(请|帮我|帮忙|给我|生成图片|生成图|生成|画一张|画|做一张图|做一张|生图|一张|的电商图|电商图|商品图|主图|海报|配图)[，,。.!！?？\s]*`)
	durationPattern              = regexp.MustCompile(`(?i)(\d{1,3})\s*秒`)
	pagePattern                  = regexp.MustCompile(`(?i)(\d{1,3})\s*页`)
	ratioPattern                 = regexp.MustCompile(`(?i)(9\s*[:：]\s*16|16\s*[:：]\s*9|1\s*[:：]\s*1|4\s*[:：]\s*3|3\s*[:：]\s*4)`)
	resolutionPattern            = regexp.MustCompile(`(?i)(4k|2160p|1080p|720p|480p)`)
)

func (RuleIntentRouter) Route(text string, defaults IntentDefaults) Intent {
	trimmed := strings.TrimSpace(text)
	result := defaultsIntent(defaults)
	if isModelInfoQuery(trimmed) {
		result.Name = IntentModelInfo
		return result
	}
	if isCapabilityInfoQuery(trimmed) {
		result.Name = IntentCapabilityInfo
		return result
	}
	if containsAny(strings.ToLower(trimmed), taskQueryKeywords) {
		result.Name = IntentTaskQuery
		return result
	}
	ref := containsAny(strings.ToLower(trimmed), referenceWords)
	videoText := strings.ToLower(trimmed)
	if containsAny(videoText, videoIntentKeywords) || (strings.Contains(videoText, "视频") && containsAny(videoText, []string{"生成", "制作", "做成", "转成"})) {
		result.Name = IntentVideoGenerate
		result.GenerationMode = "text_to_video"
		result.ReferenceAssetRequested = ref && containsAny(strings.ToLower(trimmed), []string{"图", "图片", "照片", "画面"})
		if result.ReferenceAssetRequested {
			result.Name = IntentVideoImageToVideo
			result.GenerationMode = "image_to_video"
		}
		result.Topic, result.Subject = cleanTopic(trimmed), cleanTopic(trimmed)
		result.Duration = parsedInt(durationPattern, trimmed, result.Duration)
		result.AspectRatio = normalizedMatch(ratioPattern, trimmed, result.AspectRatio)
		result.Resolution = strings.ToLower(normalizedMatch(resolutionPattern, trimmed, result.Resolution))
		result.Style = detectVideoStyle(trimmed)
		return result
	}
	if containsAny(strings.ToLower(trimmed), pptIntentKeywords) || (strings.Contains(trimmed, "整理成") && strings.Contains(strings.ToLower(trimmed), "ppt")) {
		result.Name = IntentPPTGenerate
		result.Topic, result.Subject = cleanTopic(trimmed), cleanTopic(trimmed)
		result.PageCount = parsedInt(pagePattern, trimmed, result.PageCount)
		result.Audience = detectAudience(trimmed)
		result.Purpose = detectPPTPurpose(trimmed)
		if strings.Contains(trimmed, "科技") {
			result.Theme = "technology"
		} else if strings.Contains(trimmed, "商务") {
			result.Theme = "business"
		}
		return result
	}
	if ref && containsAny(strings.ToLower(trimmed), editWords) {
		result.Name = IntentImageEdit
		result.ReferenceAssetRequested = true
		result.Topic, result.Subject = trimmed, trimmed
		return result
	}
	for _, keyword := range imageIntentKeywords {
		if strings.Contains(trimmed, keyword) {
			result.Name = IntentImageGenerate
			result.Scene = detectScene(trimmed)
			result.Subject = strings.Trim(intentNoise.ReplaceAllString(trimmed, " "), " ，,。.!！?？")
			if result.Subject == "" {
				result.Subject = "商品"
			}
			result.Topic = result.Subject
			return result
		}
	}
	if looksLikeStandaloneVisualPrompt(trimmed) {
		result.Name, result.Scene, result.Subject, result.Topic = IntentImageGenerate, detectScene(trimmed), trimmed, trimmed
		return result
	}
	result.Name = IntentUnknown
	return result
}

func defaultsIntent(d IntentDefaults) Intent {
	r := Intent{Name: IntentUnknown, Count: d.Count, Size: strings.TrimSpace(d.Size), Duration: d.VideoDuration, AspectRatio: d.VideoAspectRatio, Resolution: d.VideoResolution, ModelID: d.VideoModelID, PageCount: d.PPTPageCount, TemplateID: d.PPTTemplateID, Theme: d.PPTTheme, Language: d.PPTLanguage, UseEnterpriseLogo: d.UseEnterpriseLogo, UseEnterpriseKnowledge: d.UseEnterpriseKnowledge}
	if r.Count <= 0 {
		r.Count = 1
	}
	if r.Size == "" {
		r.Size = "1024x1024"
	}
	if r.Duration <= 0 {
		r.Duration = 5
	}
	if r.AspectRatio == "" {
		r.AspectRatio = "16:9"
	}
	if r.Resolution == "" {
		r.Resolution = "720p"
	}
	if r.PageCount <= 0 {
		r.PageCount = 8
	}
	if r.Theme == "" {
		r.Theme = "business"
	}
	if r.Language == "" {
		r.Language = "zh"
	}
	return r
}

func parsedInt(pattern *regexp.Regexp, text string, fallback int) int {
	m := pattern.FindStringSubmatch(text)
	if len(m) > 1 {
		if v, e := strconv.Atoi(m[1]); e == nil {
			return v
		}
	}
	return fallback
}
func normalizedMatch(pattern *regexp.Regexp, text, fallback string) string {
	m := pattern.FindString(text)
	if m == "" {
		return fallback
	}
	return strings.ReplaceAll(strings.ReplaceAll(strings.ToLower(m), "：", ":"), " ", "")
}
func cleanTopic(text string) string {
	return strings.Trim(strings.Join(strings.Fields(text), " "), " ，,。.!！?？")
}
func detectVideoStyle(text string) string {
	for _, v := range []string{"电商", "科技", "电影", "写实", "动漫", "产品", "宣传"} {
		if strings.Contains(text, v) {
			return v
		}
	}
	return ""
}
func detectAudience(text string) string {
	for _, v := range []string{"代理商", "老板", "投资人", "客户", "员工", "学生"} {
		if strings.Contains(text, v) {
			return v
		}
	}
	return "auto"
}
func detectPPTPurpose(text string) string {
	for _, v := range []string{"招商", "汇报", "产品介绍", "培训", "路演"} {
		if strings.Contains(text, v) {
			return v
		}
	}
	return "general"
}
func isModelInfoQuery(text string) bool {
	n := strings.ToLower(strings.TrimSpace(text))
	return len([]rune(n)) <= 40 && containsAny(n, modelInfoQueryKeywords)
}
func isCapabilityInfoQuery(text string) bool {
	n := strings.ToLower(strings.TrimSpace(text))
	return len([]rune(n)) <= 40 && containsAny(n, capabilityInfoQueryKeywords)
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
func containsAny(text string, keywords []string) bool { return countContained(text, keywords) > 0 }
func countContained(text string, keywords []string) int {
	c := 0
	for _, k := range keywords {
		if strings.Contains(text, k) {
			c++
		}
	}
	return c
}
func detectScene(text string) string {
	for _, s := range []string{"电商图", "商品图", "主图", "海报", "配图"} {
		if strings.Contains(text, s) {
			return s
		}
	}
	return "通用图片"
}
