package httpserver

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"path"
	"strings"

	"xianzhi-ai/backend-go/internal/app/generation"
)

type adminImageModelCapabilities struct {
	ImageGeneration  bool     `json:"image_generation"`
	ImageEdit        bool     `json:"image_edit"`
	ReferenceImages  bool     `json:"reference_images_supported"`
	MaxCount         int      `json:"max_count"`
	MaxFileSizeMB    int      `json:"max_file_size_mb"`
	Formats          []string `json:"formats,omitempty"`
	RequiredMinCount int      `json:"required_min_count,omitempty"`
}

type imageCaps = adminImageModelCapabilities

type imageCapabilityValidationError struct {
	code    string
	message string
	details map[string]any
}

func (e *imageCapabilityValidationError) Error() string { return e.message }
func (e *imageCapabilityValidationError) BusinessCode() string {
	return e.code
}
func (e *imageCapabilityValidationError) ErrorDetails() map[string]any {
	return e.details
}

func newImageCapabilityValidationError(code, message string, details map[string]any) error {
	return &imageCapabilityValidationError{code: code, message: message, details: details}
}

func defaultImageFormats() []string {
	return []string{"png", "jpg", "jpeg", "webp"}
}

func gptImage2ImageCapabilities() adminImageModelCapabilities {
	return adminImageModelCapabilities{
		ImageGeneration: true,
		ImageEdit:       true,
		ReferenceImages: true,
		MaxCount:        16,
		MaxFileSizeMB:   50,
		Formats:         defaultImageFormats(),
	}
}

func gptImage2ImageCapabilitiesPtr() *adminImageModelCapabilities {
	capabilities := gptImage2ImageCapabilities()
	return &capabilities
}

func mockStandardImageCapabilitiesPtr() *adminImageModelCapabilities {
	capabilities := adminImageModelCapabilities{
		ImageGeneration: true,
		ImageEdit:       true,
		ReferenceImages: true,
		MaxCount:        16,
		MaxFileSizeMB:   50,
		Formats:         defaultImageFormats(),
	}
	return &capabilities
}

func hyTextToImageCapabilitiesPtr() *adminImageModelCapabilities {
	capabilities := adminImageModelCapabilities{
		ImageGeneration: true,
		ImageEdit:       false,
		ReferenceImages: false,
		MaxCount:        0,
		MaxFileSizeMB:   50,
		Formats:         defaultImageFormats(),
	}
	return &capabilities
}

func hyImageToImageCapabilitiesPtr() *adminImageModelCapabilities {
	capabilities := adminImageModelCapabilities{
		ImageGeneration:  false,
		ImageEdit:        true,
		ReferenceImages:  true,
		MaxCount:         1,
		MaxFileSizeMB:    50,
		Formats:          defaultImageFormats(),
		RequiredMinCount: 1,
	}
	return &capabilities
}

func defaultImageCapabilitiesForModel(model adminAIModel) adminImageModelCapabilities {
	switch strings.ToLower(strings.TrimSpace(firstNonEmptyString(model.ModelName, model.ModelNameCamel))) {
	case "gpt-image-2":
		return gptImage2ImageCapabilities()
	case "mock-standard":
		return *mockStandardImageCapabilitiesPtr()
	case "hy-image-3.0-plus-4090-tob-v1.0":
		return *hyTextToImageCapabilitiesPtr()
	case "hy-image-v3.0-i2i-tob-v1.0.1":
		return *hyImageToImageCapabilitiesPtr()
	}
	capabilities := adminImageModelCapabilities{
		ImageGeneration: true,
		ImageEdit:       true,
		ReferenceImages: true,
		MaxCount:        16,
		MaxFileSizeMB:   50,
		Formats:         defaultImageFormats(),
	}
	for _, code := range model.CapabilityCode {
		switch strings.ToLower(strings.TrimSpace(code)) {
		case "image_to_image", "image_edit":
			capabilities.ImageEdit = true
			capabilities.ReferenceImages = true
		case "text_to_image":
			capabilities.ImageGeneration = true
		}
	}
	if !capabilities.ReferenceImages {
		capabilities.MaxCount = 0
	}
	return capabilities
}

func normalizeImageModelCapabilities(capabilities adminImageModelCapabilities) adminImageModelCapabilities {
	capabilities.Formats = uniqueTrimmedStrings(capabilities.Formats)
	if len(capabilities.Formats) == 0 {
		capabilities.Formats = defaultImageFormats()
	}
	if capabilities.MaxFileSizeMB <= 0 {
		capabilities.MaxFileSizeMB = 50
	}
	if !capabilities.ReferenceImages {
		capabilities.MaxCount = 0
		capabilities.RequiredMinCount = 0
		return capabilities
	}
	if capabilities.MaxCount <= 0 {
		capabilities.MaxCount = 16
	}
	if capabilities.RequiredMinCount < 0 {
		capabilities.RequiredMinCount = 0
	}
	if capabilities.RequiredMinCount > capabilities.MaxCount {
		capabilities.RequiredMinCount = capabilities.MaxCount
	}
	return capabilities
}

func isImageAIModel(model adminAIModel) bool {
	if strings.EqualFold(strings.TrimSpace(model.ModelType), "image") ||
		canonicalModuleCode(firstNonEmptyString(model.ModuleCode, model.ModuleCodeCamel)) == moduleImageGeneration {
		return true
	}
	for _, capability := range model.CapabilityCode {
		switch strings.ToLower(strings.TrimSpace(capability)) {
		case "text_to_image", "image_to_image", "image_edit":
			return true
		}
	}
	return false
}

func normalizeImageModelCapabilityData(data adminPlatformData) adminPlatformData {
	for index := range data.AIModels {
		model := &data.AIModels[index]
		if !isImageAIModel(*model) {
			continue
		}
		if model.ImageCapabilities == nil {
			defaults := defaultImageCapabilitiesForModel(*model)
			model.ImageCapabilities = &defaults
		} else {
			normalized := normalizeImageModelCapabilities(*model.ImageCapabilities)
			model.ImageCapabilities = &normalized
		}
	}
	return data
}

func resolveImageModelCapabilities(model adminAIModel) adminImageModelCapabilities {
	if model.ImageCapabilities != nil {
		return normalizeImageModelCapabilities(*model.ImageCapabilities)
	}
	return normalizeImageModelCapabilities(defaultImageCapabilitiesForModel(model))
}

func imageCapabilityPublicView(capabilities adminImageModelCapabilities) map[string]any {
	return map[string]any{
		"imageGeneration": capabilities.ImageGeneration,
		"imageEdit":       capabilities.ImageEdit,
		"referenceImages": map[string]any{
			"supported":     capabilities.ReferenceImages,
			"maxCount":      capabilities.MaxCount,
			"maxFileSizeMb": capabilities.MaxFileSizeMB,
			"formats":       append([]string(nil), capabilities.Formats...),
			"requiredMin":   capabilities.RequiredMinCount,
		},
	}
}

func findAIModelByName(items []adminAIModel, modelName string) adminAIModel {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return adminAIModel{}
	}
	for _, item := range items {
		if strings.EqualFold(firstNonEmptyString(item.ModelName, item.ModelNameCamel), modelName) {
			if item.ModelName == "" {
				item.ModelName = item.ModelNameCamel
			}
			return item
		}
	}
	return adminAIModel{}
}

func (a api) modelCapabilities(w http.ResponseWriter, r *http.Request) {
	modelName := strings.TrimSpace(firstNonEmptyString(r.PathValue("model"), r.URL.Query().Get("model")))
	if modelName == "" {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		for index, part := range parts {
			if part == "models" && index+1 < len(parts) && parts[index+1] != "capabilities" {
				modelName = parts[index+1]
				break
			}
		}
	}
	if modelName == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("model is required"))
		return
	}
	data, err := a.store.AdminData()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	data = normalizeAICapabilityDefaults(data)
	model := findAIModelByName(data.AIModels, modelName)
	if model.ID == "" || !isActiveLike(model.Status) {
		writeError(w, http.StatusNotFound, fmt.Errorf("ai model not found: %s", modelName))
		return
	}
	capabilities := resolveImageModelCapabilities(model)
	payload := imageCapabilityPublicView(capabilities)
	if isVideoAIModel(model) && model.VideoCapabilities != nil {
		payload["videoCapabilities"] = model.VideoCapabilities
	}
	writeJSON(w, map[string]any{
		"model":             firstNonEmptyString(model.ModelName, model.ModelNameCamel),
		"modelType":         firstNonEmptyString(model.ModelType, model.ModelTypeCamel),
		"capabilities":      payload,
		"referenceImages":   payload["referenceImages"],
		"imageCapabilities": capabilities,
	})
}

func collectImageReferenceItems(params map[string]any) []map[string]any {
	if params == nil {
		return nil
	}
	items := make([]map[string]any, 0)
	seen := map[string]bool{}
	appendItem := func(item map[string]any) {
		url := strings.TrimSpace(fmt.Sprint(item["url"]))
		name := strings.TrimSpace(fmt.Sprint(item["name"]))
		key := url + "\x00" + name
		if url == "" && name == "" {
			return
		}
		if seen[key] {
			return
		}
		seen[key] = true
		items = append(items, item)
	}
	for _, key := range []string{"referenceImages", "reference_images", "inputImagesSnapshot"} {
		raw, ok := params[key]
		if !ok || raw == nil {
			continue
		}
		switch typed := raw.(type) {
		case []any:
			for _, value := range typed {
				if item, ok := value.(map[string]any); ok {
					appendItem(item)
					continue
				}
				text := strings.TrimSpace(fmt.Sprint(value))
				if text != "" {
					appendItem(map[string]any{"url": text})
				}
			}
		case []string:
			for _, value := range typed {
				if strings.TrimSpace(value) != "" {
					appendItem(map[string]any{"url": value})
				}
			}
		}
	}
	for _, key := range []string{"image_urls", "imageUrls", "inputImageUrls"} {
		raw, ok := params[key]
		if !ok || raw == nil {
			continue
		}
		switch typed := raw.(type) {
		case []any:
			for _, value := range typed {
				text := strings.TrimSpace(fmt.Sprint(value))
				if text != "" {
					appendItem(map[string]any{"url": text})
				}
			}
		case []string:
			for _, value := range typed {
				if strings.TrimSpace(value) != "" {
					appendItem(map[string]any{"url": value})
				}
			}
		case string:
			if strings.TrimSpace(typed) != "" {
				appendItem(map[string]any{"url": typed})
			}
		}
	}
	return items
}

func imageReferenceFormat(item map[string]any) string {
	name := strings.ToLower(strings.TrimSpace(fmt.Sprint(item["name"])))
	url := strings.ToLower(strings.TrimSpace(fmt.Sprint(item["url"])))
	if strings.HasPrefix(url, "data:image/") {
		header := strings.TrimPrefix(url, "data:image/")
		if slash := strings.Index(header, ";"); slash >= 0 {
			header = header[:slash]
		}
		if slash := strings.Index(header, ","); slash >= 0 {
			header = header[:slash]
		}
		return normalizeImageFormat(header)
	}
	ext := strings.TrimPrefix(path.Ext(name), ".")
	if ext == "" {
		if parsed := strings.TrimPrefix(path.Ext(strings.Split(url, "?")[0]), "."); parsed != "" {
			ext = parsed
		}
	}
	return normalizeImageFormat(ext)
}

func normalizeImageFormat(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "jpeg":
		return "jpg"
	case "png", "jpg", "webp", "gif", "bmp":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func imageReferenceBytes(item map[string]any) int {
	if size, ok := anyToFloat(item["size"]); ok && size > 0 {
		return int(size)
	}
	if size, ok := anyToFloat(item["fileSize"]); ok && size > 0 {
		return int(size)
	}
	url := strings.TrimSpace(fmt.Sprint(item["url"]))
	if !strings.HasPrefix(url, "data:image/") {
		return 0
	}
	comma := strings.Index(url, ",")
	if comma < 0 {
		return 0
	}
	payload := url[comma+1:]
	if strings.Contains(strings.ToLower(url[:comma]), "base64") {
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return 0
		}
		return len(decoded)
	}
	return len(payload)
}

func formatAllowed(format string, allowed []string) bool {
	format = normalizeImageFormat(format)
	if format == "" {
		return true
	}
	for _, item := range allowed {
		if normalizeImageFormat(item) == format {
			return true
		}
	}
	return false
}

func validateImageReferenceCapabilities(req *generation.CreateRequest, resolved resolvedModuleSchema) error {
	if req == nil || canonicalModuleCode(resolved.Module.ModuleCode) != moduleImageGeneration {
		return nil
	}
	capabilities := resolveImageModelCapabilities(resolved.Model)
	items := collectImageReferenceItems(req.Params)
	actual := len(items)
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	req.Params["reference_image_count"] = actual
	req.Params["referenceImageCount"] = actual
	if actual == 0 {
		if capabilities.RequiredMinCount > 0 {
			return newImageCapabilityValidationError("REFERENCE_IMAGE_REQUIRED", "该模型需要上传参考图", map[string]any{
				"maxReferenceImages":    capabilities.MaxCount,
				"actualReferenceImages": actual,
				"requiredMinCount":      capabilities.RequiredMinCount,
			})
		}
		return nil
	}
	if !capabilities.ReferenceImages || capabilities.MaxCount <= 0 {
		return newImageCapabilityValidationError("REFERENCE_IMAGE_NOT_SUPPORTED", "当前模型不支持参考图", map[string]any{
			"maxReferenceImages":    0,
			"actualReferenceImages": actual,
		})
	}
	if actual > capabilities.MaxCount {
		return newImageCapabilityValidationError("REFERENCE_IMAGE_LIMIT_EXCEEDED", fmt.Sprintf("当前模型最多可上传 %d 张参考图", capabilities.MaxCount), map[string]any{
			"maxReferenceImages":    capabilities.MaxCount,
			"actualReferenceImages": actual,
		})
	}
	maxBytes := capabilities.MaxFileSizeMB * 1024 * 1024
	for index, item := range items {
		format := imageReferenceFormat(item)
		if format != "" && !formatAllowed(format, capabilities.Formats) {
			return newImageCapabilityValidationError("REFERENCE_IMAGE_FORMAT_UNSUPPORTED", fmt.Sprintf("第 %d 张参考图格式不受支持", index+1), map[string]any{
				"maxReferenceImages":    capabilities.MaxCount,
				"actualReferenceImages": actual,
				"format":                format,
			})
		}
		if size := imageReferenceBytes(item); size > 0 && maxBytes > 0 && size > maxBytes {
			return newImageCapabilityValidationError("REFERENCE_IMAGE_TOO_LARGE", fmt.Sprintf("第 %d 张参考图超过 %dMB", index+1, capabilities.MaxFileSizeMB), map[string]any{
				"maxReferenceImages":    capabilities.MaxCount,
				"actualReferenceImages": actual,
				"maxFileSizeMb":         capabilities.MaxFileSizeMB,
			})
		}
	}
	return nil
}

func imageQuotePricingMap(breakdown map[string]any, quantity float64) map[string]any {
	basePrice, _ := anyToFloat(breakdown["basePrice"])
	parameters, _ := breakdown["parameters"].(map[string]any)
	qualityMultiplier := 1.0
	sizeMultiplier := 1.0
	if parameters != nil {
		if value, ok := anyToFloat(parameters["quality"]); ok && value > 0 {
			qualityMultiplier = value
		}
		if value, ok := anyToFloat(parameters["size"]); ok && value > 0 {
			sizeMultiplier = value
		}
	}
	return map[string]any{
		"basePrice":         basePrice,
		"qualityMultiplier": qualityMultiplier,
		"sizeMultiplier":    sizeMultiplier,
		"quantity":          quantity,
	}
}

func dummyImageReferenceItems(count int) []any {
	items := make([]any, 0, count)
	for index := 0; index < count; index++ {
		items = append(items, map[string]any{
			"name": fmt.Sprintf("ref-%d.png", index+1),
			"url":  fmt.Sprintf("https://example.invalid/ref-%d.png", index+1),
		})
	}
	return items
}
