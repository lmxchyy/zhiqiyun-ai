package image

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"

	"xianzhi-ai/backend-go/internal/config"
)

type OpenAICompatible struct {
	code    string
	baseURL string
	apiKey  string
	model   string
	models  []string
	client  *http.Client
}

func NewOpenAICompatible(cfg config.Config) OpenAICompatible {
	timeoutMS, _ := strconv.Atoi(cfg.ModelTimeoutMS)
	if timeoutMS <= 0 {
		timeoutMS = 30000
	}
	model := strings.TrimSpace(cfg.ImageModel)
	return OpenAICompatible{
		code:    "openai-compatible",
		baseURL: strings.TrimSpace(cfg.ModelProviderURL),
		apiKey:  strings.TrimSpace(cfg.ModelProviderAPIKey),
		model:   model,
		models:  nonEmptyStrings(model),
		client:  &http.Client{Timeout: time.Duration(timeoutMS) * time.Millisecond},
	}
}
func NewOpenAICompatibleWithOptions(opts OpenAICompatibleOptions) OpenAICompatible {
	timeoutMS := opts.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = 30000
	}
	model := strings.TrimSpace(opts.ImageModel)
	models := nonEmptyStrings(opts.Models...)
	if model != "" {
		models = appendIfMissing(models, model)
	}
	return OpenAICompatible{
		code:    strings.TrimSpace(opts.Code),
		baseURL: strings.TrimSpace(opts.BaseURL),
		apiKey:  strings.TrimSpace(opts.APIKey),
		model:   model,
		models:  models,
		client:  &http.Client{Timeout: time.Duration(timeoutMS) * time.Millisecond},
	}
}

type OpenAICompatibleOptions struct {
	Code       string
	BaseURL    string
	APIKey     string
	ImageModel string
	Models     []string
	TimeoutMS  int
}

func (p OpenAICompatible) enabled() bool {
	return p.baseURL != "" && p.apiKey != ""
}

func (p OpenAICompatible) Code() string {
	if strings.TrimSpace(p.code) == "" {
		return "openai-compatible"
	}
	return p.code
}

func (p OpenAICompatible) Supports(req generation.CreateRequest) bool {
	if !p.enabled() {
		return false
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		return true
	}
	if p.model == "" && len(p.models) == 0 {
		return true
	}
	if model == p.model {
		return true
	}
	for _, item := range p.models {
		if model == item {
			return true
		}
	}
	return false
}

func (p OpenAICompatible) DefaultModel() string {
	return p.model
}

func (p OpenAICompatible) Generate(ctx context.Context, req generation.CreateRequest) ([]generation.GeneratedImage, error) {
	return p.generate(ctx, req)
}

func (p OpenAICompatible) generate(ctx context.Context, req generation.CreateRequest) ([]generation.GeneratedImage, error) {
	if !p.enabled() {
		return nil, nil
	}
	if strings.EqualFold(req.Type, "IMAGE_TO_IMAGE") {
		references := referenceImages(req.Params)
		if len(references) == 0 {
			return nil, errors.New("reference image is required for image-to-image generation")
		}
		editCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		images, err := p.edit(editCtx, req, references)
		if err != nil && isTimeoutError(err) {
			return nil, fmt.Errorf("当前图片上游未响应参考图生图接口，请切换支持 image edits 的上游或模型后重试: %w", err)
		}
		return images, err
	}
	count := imageCount(req.Params)
	size, width, height := imageSize(req.Params)
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = p.model
	}
	if model == "" {
		model = "gpt-image-2"
	}
	body := map[string]any{
		"model":           model,
		"prompt":          req.Prompt,
		"n":               count,
		"size":            size,
		"response_format": "b64_json",
	}
	images, err := p.generateWithBody(ctx, body, width, height)
	if err == nil {
		return images, nil
	}
	if isTimeoutError(err) {
		return nil, err
	}
	delete(body, "response_format")
	images, retryErr := p.generateWithBody(ctx, body, width, height)
	if retryErr == nil {
		return images, nil
	}
	if isTimeoutError(retryErr) {
		return nil, retryErr
	}
	body["response_format"] = "url"
	images, urlErr := p.generateWithBody(ctx, body, width, height)
	if urlErr == nil {
		return images, nil
	}
	return nil, err
}

func (p OpenAICompatible) edit(ctx context.Context, req generation.CreateRequest, references []referenceImage) ([]generation.GeneratedImage, error) {
	count := imageCount(req.Params)
	size, width, height := imageSize(req.Params)
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = p.model
	}
	if model == "" {
		model = "gpt-image-2"
	}
	fields := map[string]string{
		"model":           model,
		"prompt":          req.Prompt,
		"n":               strconv.Itoa(count),
		"size":            size,
		"response_format": "b64_json",
	}
	images, err := p.editWithFields(ctx, fields, references, width, height)
	if err == nil {
		return images, nil
	}
	if isTimeoutError(err) {
		return nil, err
	}
	delete(fields, "response_format")
	images, retryErr := p.editWithFields(ctx, fields, references, width, height)
	if retryErr == nil {
		return images, nil
	}
	if isTimeoutError(retryErr) {
		return nil, retryErr
	}
	fields["response_format"] = "url"
	images, urlErr := p.editWithFields(ctx, fields, references, width, height)
	if urlErr == nil {
		return images, nil
	}
	images, fallbackErr := p.generateWithReferences(ctx, req, references, width, height)
	if fallbackErr == nil {
		return images, nil
	}
	return nil, fmt.Errorf("image-to-image failed; edits error: %w; compatible generation fallback error: %v", err, fallbackErr)
}

func (p OpenAICompatible) generateWithReferences(ctx context.Context, req generation.CreateRequest, references []referenceImage, width int, height int) ([]generation.GeneratedImage, error) {
	count := imageCount(req.Params)
	size, _, _ := imageSize(req.Params)
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = p.model
	}
	if model == "" {
		model = "gpt-image-2"
	}
	referenceURLs, err := p.referenceDataURLs(ctx, references)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"model":            model,
		"prompt":           req.Prompt,
		"n":                count,
		"size":             size,
		"response_format":  "b64_json",
		"reference_images": referenceURLs,
		"referenceImages":  referenceURLs,
		"images":           referenceURLs,
	}
	if len(referenceURLs) > 0 {
		body["image"] = referenceURLs[0]
	}
	images, err := p.generateWithBody(ctx, body, width, height)
	if err == nil {
		return images, nil
	}
	if isTimeoutError(err) {
		return nil, err
	}
	delete(body, "response_format")
	images, retryErr := p.generateWithBody(ctx, body, width, height)
	if retryErr == nil {
		return images, nil
	}
	if isTimeoutError(retryErr) {
		return nil, retryErr
	}
	body["response_format"] = "url"
	images, urlErr := p.generateWithBody(ctx, body, width, height)
	if urlErr == nil {
		return images, nil
	}
	return nil, err
}

func (p OpenAICompatible) editWithFields(ctx context.Context, fields map[string]string, references []referenceImage, width int, height int) ([]generation.GeneratedImage, error) {
	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, err
		}
	}
	for index, ref := range references {
		raw, filename, err := p.referenceBytes(ctx, ref, index)
		if err != nil {
			return nil, err
		}
		part, err := writer.CreateFormFile("image[]", filename)
		if err != nil {
			return nil, err
		}
		if _, err := part.Write(raw); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.editsEndpoint(), &payload)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	httpReq.Header.Set("Accept", "application/json")

	res, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("image provider edit returned %d: %s", res.StatusCode, string(raw))
	}
	return decodeGeneratedImages(raw, width, height)
}

func (p OpenAICompatible) generateWithBody(ctx context.Context, body map[string]any, width int, height int) ([]generation.GeneratedImage, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.generationsEndpoint(), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	res, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("image provider returned %d: %s", res.StatusCode, string(raw))
	}
	return decodeGeneratedImages(raw, width, height)
}

func decodeGeneratedImages(raw []byte, width int, height int) ([]generation.GeneratedImage, error) {
	var decoded struct {
		Data []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	if len(decoded.Data) == 0 {
		return nil, errors.New("image provider returned no images")
	}
	images := make([]generation.GeneratedImage, 0, len(decoded.Data))
	for _, item := range decoded.Data {
		imageURL := strings.TrimSpace(item.URL)
		contentType := "image/png"
		if imageURL == "" && item.B64JSON != "" {
			imageURL = "data:image/png;base64," + item.B64JSON
		}
		if imageURL == "" {
			continue
		}
		images = append(images, generation.GeneratedImage{
			URL:         imageURL,
			ContentType: contentType,
			Width:       width,
			Height:      height,
			Source:      "model-provider",
		})
	}
	if len(images) == 0 {
		return nil, errors.New("image provider returned empty image payloads")
	}
	return images, nil
}

type referenceImage struct {
	AssetID string
	Name    string
	URL     string
}

func referenceImages(params map[string]any) []referenceImage {
	rawItems, ok := params["referenceImages"].([]any)
	if !ok {
		return nil
	}
	items := make([]referenceImage, 0, len(rawItems))
	for _, raw := range rawItems {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		ref := referenceImage{
			AssetID: strings.TrimSpace(fmt.Sprint(item["assetId"])),
			Name:    strings.TrimSpace(fmt.Sprint(item["name"])),
			URL:     strings.TrimSpace(fmt.Sprint(item["url"])),
		}
		if ref.URL != "" {
			items = append(items, ref)
		}
	}
	return items
}

func (p OpenAICompatible) referenceBytes(ctx context.Context, ref referenceImage, index int) ([]byte, string, error) {
	name := strings.TrimSpace(ref.Name)
	if name == "" {
		name = fmt.Sprintf("reference-%d.png", index+1)
	}
	if strings.HasPrefix(ref.URL, "data:image/") {
		comma := strings.Index(ref.URL, ",")
		if comma < 0 || !strings.Contains(ref.URL[:comma], ";base64") {
			return nil, "", errors.New("unsupported reference image data URL")
		}
		raw, err := base64.StdEncoding.DecodeString(ref.URL[comma+1:])
		if err != nil {
			return nil, "", err
		}
		return raw, imageFilename(name, ref.URL[:comma]), nil
	}
	if strings.HasPrefix(ref.URL, "http://") || strings.HasPrefix(ref.URL, "https://") {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, ref.URL, nil)
		if err != nil {
			return nil, "", err
		}
		res, err := p.client.Do(httpReq)
		if err != nil {
			return nil, "", err
		}
		defer res.Body.Close()
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return nil, "", fmt.Errorf("reference image returned %d", res.StatusCode)
		}
		raw, err := io.ReadAll(io.LimitReader(res.Body, 10<<20))
		if err != nil {
			return nil, "", err
		}
		return raw, imageFilename(name, res.Header.Get("Content-Type")), nil
	}
	return nil, "", fmt.Errorf("reference image must be data URL or HTTP URL")
}

func (p OpenAICompatible) referenceDataURLs(ctx context.Context, references []referenceImage) ([]string, error) {
	items := make([]string, 0, len(references))
	for index, ref := range references {
		if strings.HasPrefix(ref.URL, "data:image/") {
			items = append(items, ref.URL)
			continue
		}
		raw, filename, err := p.referenceBytes(ctx, ref, index)
		if err != nil {
			return nil, err
		}
		contentType := "image/png"
		lower := strings.ToLower(filename)
		switch {
		case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
			contentType = "image/jpeg"
		case strings.HasSuffix(lower, ".webp"):
			contentType = "image/webp"
		}
		items = append(items, "data:"+contentType+";base64,"+base64.StdEncoding.EncodeToString(raw))
	}
	return items, nil
}

func imageFilename(name string, contentType string) string {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg") || strings.HasSuffix(lower, ".webp") {
		return name
	}
	contentType = strings.ToLower(contentType)
	switch {
	case strings.Contains(contentType, "jpeg"), strings.Contains(contentType, "jpg"):
		return name + ".jpg"
	case strings.Contains(contentType, "webp"):
		return name + ".webp"
	default:
		return name + ".png"
	}
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr interface{ Timeout() bool }
	return errors.As(err, &netErr) && netErr.Timeout()
}

func (p OpenAICompatible) generationsEndpoint() string {
	base := strings.TrimRight(p.baseURL, "/")
	parsed, err := url.Parse(base)
	if err == nil && strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), "/v1") {
		return base + "/images/generations"
	}
	return base + "/v1/images/generations"
}

func (p OpenAICompatible) editsEndpoint() string {
	base := strings.TrimRight(p.baseURL, "/")
	parsed, err := url.Parse(base)
	if err == nil && strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), "/v1") {
		return base + "/images/edits"
	}
	return base + "/v1/images/edits"
}

func imageSize(params map[string]any) (string, int, int) {
	ratio := strings.TrimSpace(fmt.Sprint(params["imageRatio"]))
	switch ratio {
	case "3:4", "9:16":
		return "1024x1536", 1024, 1536
	case "4:3", "16:9":
		return "1536x1024", 1536, 1024
	default:
		return "1024x1024", 1024, 1024
	}
}

func imageCount(params map[string]any) int {
	value, ok := params["count"]
	if !ok {
		return 1
	}
	var count int
	switch typed := value.(type) {
	case float64:
		count = int(math.Floor(typed))
	case int:
		count = typed
	case string:
		_, _ = fmt.Sscanf(typed, "%d", &count)
	}
	if count < 1 {
		return 1
	}
	if count > 8 {
		return 8
	}
	return count
}

func nonEmptyStrings(values ...string) []string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			items = appendIfMissing(items, trimmed)
		}
	}
	return items
}

func appendIfMissing(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}
