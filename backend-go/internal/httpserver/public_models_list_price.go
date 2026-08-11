package httpserver

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

func videoListBaselineParams(caps adminVideoModelCapabilities) (duration int, resolution string) {
	duration = 5
	if len(caps.SupportedDurations) > 0 {
		duration = caps.SupportedDurations[0]
		for _, value := range caps.SupportedDurations {
			if value > 0 && value < duration {
				duration = value
			}
		}
	}
	resolution = "720p"
	if len(caps.SupportedResolutions) == 0 {
		return duration, resolution
	}
	for _, value := range caps.SupportedResolutions {
		if strings.EqualFold(strings.TrimSpace(value), "720p") {
			return duration, "720p"
		}
	}
	return duration, strings.TrimSpace(caps.SupportedResolutions[0])
}

func videoModelPublicDisplayName(modelName string) string {
	switch strings.ToLower(strings.TrimSpace(modelName)) {
	case "mock-video":
		return "本地演示视频"
	case "grok-image-video", "grok-video-image":
		return "Grok Image Video"
	case "grok-imagine-video-1.5-preview":
		return "Grok Imagine Video 1.5 Preview"
	case "grok-imagine-1.5-video":
		return "Grok Imagine Video 1.5"
	case "seedance-fast-2.0", "seedance-2.0":
		return "Seedance 2.0"
	case "doubao-seedance-2.0":
		return "Doubao Seedance 2.0"
	default:
		return strings.TrimSpace(modelName)
	}
}

func videoModelPriceHint(data adminPlatformData, modelName string) string {
	req := createGenerationTaskRequest{
		Type:       videoModeText,
		Model:      strings.TrimSpace(modelName),
		ModuleCode: moduleVideoGeneration,
		Params:     map[string]any{},
	}
	rule := billingRuleForRequest(req, data)
	base := int(math.Ceil(rule.BasePrice))
	if minimum := int(math.Ceil(rule.MinimumCharge)); minimum > base {
		base = minimum
	}
	if base < 1 {
		base = 1
	}
	switch strings.ToLower(strings.TrimSpace(rule.BillingType)) {
	case "per_request":
		return fmt.Sprintf("%d 积分/次", base)
	case "per_second":
		return fmt.Sprintf("%d 积分/秒", base)
	default:
		return fmt.Sprintf("%d 积分起", base)
	}
}

func videoModelCapabilityHint(caps adminVideoModelCapabilities) string {
	parts := make([]string, 0, 3)
	switch {
	case caps.SupportsTextToVideo && caps.SupportsImageToVideo:
		parts = append(parts, "文生/图生")
	case caps.SupportsImageToVideo:
		parts = append(parts, "仅图生")
	case caps.SupportsTextToVideo:
		parts = append(parts, "仅文生")
	}
	if len(caps.SupportedDurations) > 0 {
		minDuration := caps.SupportedDurations[0]
		maxDuration := caps.SupportedDurations[0]
		for _, value := range caps.SupportedDurations {
			if value > 0 && value < minDuration {
				minDuration = value
			}
			if value > maxDuration {
				maxDuration = value
			}
		}
		if minDuration == maxDuration {
			parts = append(parts, fmt.Sprintf("%ds", minDuration))
		} else if len(caps.SupportedDurations) <= 3 {
			labels := make([]string, 0, len(caps.SupportedDurations))
			for _, value := range caps.SupportedDurations {
				labels = append(labels, fmt.Sprintf("%d", value))
			}
			parts = append(parts, strings.Join(labels, "/")+"s")
		} else {
			parts = append(parts, fmt.Sprintf("%d–%ds", minDuration, maxDuration))
		}
	}
	if caps.SupportsImageToVideo && caps.MaxReferenceImages > 1 {
		parts = append(parts, fmt.Sprintf("最多%d图", caps.MaxReferenceImages))
	} else if caps.SupportsImageToVideo && !caps.SupportsTextToVideo && caps.MaxReferenceImages == 1 {
		parts = append(parts, "需1张参考图")
	}
	return strings.Join(parts, " · ")
}

func videoModelListPrice(data adminPlatformData, modelName string, caps adminVideoModelCapabilities) (points int, label string) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return 1, "约 1 积分起"
	}
	duration, resolution := videoListBaselineParams(caps)
	req := createGenerationTaskRequest{
		Type:       videoModeText,
		Model:      modelName,
		ModuleCode: moduleVideoGeneration,
		Params: map[string]any{
			"duration":     duration,
			"resolution":   resolution,
			"aspect_ratio": "16:9",
		},
	}
	points = generationPointCostForRequest(req, data)
	rule := billingRuleForRequest(req, data)
	switch strings.ToLower(strings.TrimSpace(rule.BillingType)) {
	case "per_request":
		label = fmt.Sprintf("%d 积分/次", points)
	case "per_second":
		label = fmt.Sprintf("约 %d 积分起（%ds·%s）", points, duration, resolution)
	default:
		label = fmt.Sprintf("约 %d 积分起", points)
	}
	return points, label
}

func videoModelPublicSubtitle(priceHint, capabilityHint string) string {
	parts := make([]string, 0, 2)
	if hint := strings.TrimSpace(priceHint); hint != "" {
		parts = append(parts, hint)
	}
	if hint := strings.TrimSpace(capabilityHint); hint != "" {
		parts = append(parts, hint)
	}
	return strings.Join(parts, " · ")
}

func attachVideoModelPublicPricing(item map[string]any, data adminPlatformData, code string, caps adminVideoModelCapabilities) {
	listPricePoints, _ := videoModelListPrice(data, code, caps)
	priceHint := videoModelPriceHint(data, code)
	capabilityHint := videoModelCapabilityHint(caps)
	displayName := videoModelPublicDisplayName(code)
	subtitle := videoModelPublicSubtitle(priceHint, capabilityHint)
	item["name"] = displayName
	item["displayName"] = displayName
	item["listPricePoints"] = listPricePoints
	item["priceHint"] = priceHint
	item["capabilityHint"] = capabilityHint
	item["priceLabel"] = subtitle
	item["pointCost"] = listPricePoints
	item["description"] = subtitle
}

func isPublicVideoModelItem(item map[string]any) bool {
	if item == nil {
		return false
	}
	if _, ok := item["videoCapabilities"]; ok {
		return true
	}
	if _, ok := item["video_capabilities"]; ok {
		return true
	}
	capabilities, ok := item["capabilities"].([]string)
	if !ok {
		return false
	}
	for _, capability := range capabilities {
		switch strings.ToUpper(strings.TrimSpace(capability)) {
		case "TEXT_TO_VIDEO", "IMAGE_TO_VIDEO", "VIDEO_TO_VIDEO":
			return true
		}
	}
	return false
}

func publicModelListPricePoints(item map[string]any) int {
	if item == nil {
		return 0
	}
	switch value := item["listPricePoints"].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func publicModelCode(item map[string]any) string {
	if item == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(item["code"]))
}

func sortPublicModelsVideoByListPrice(items []map[string]any) []map[string]any {
	if len(items) < 2 {
		return items
	}
	nonVideo := make([]map[string]any, 0, len(items))
	video := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if isPublicVideoModelItem(item) {
			video = append(video, item)
			continue
		}
		nonVideo = append(nonVideo, item)
	}
	sort.SliceStable(video, func(i, j int) bool {
		pi := publicModelListPricePoints(video[i])
		pj := publicModelListPricePoints(video[j])
		if pi != pj {
			return pi < pj
		}
		return publicModelCode(video[i]) < publicModelCode(video[j])
	})
	return append(nonVideo, video...)
}
