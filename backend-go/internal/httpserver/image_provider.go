package httpserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/config"
)

type imageProvider struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func newImageProvider(cfg config.Config) imageProvider {
	timeoutMS, _ := strconv.Atoi(cfg.ModelTimeoutMS)
	if timeoutMS <= 0 {
		timeoutMS = 30000
	}
	return imageProvider{
		baseURL: strings.TrimSpace(cfg.ModelProviderURL),
		apiKey:  strings.TrimSpace(cfg.ModelProviderAPIKey),
		model:   strings.TrimSpace(cfg.ImageModel),
		client: &http.Client{
			Timeout:       time.Duration(timeoutMS) * time.Millisecond,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (p imageProvider) enabled() bool {
	return p.baseURL != "" && p.apiKey != ""
}

func (p imageProvider) generate(ctx context.Context, req createGenerationTaskRequest) ([]generatedImage, error) {
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
		return p.edit(editCtx, req, references)
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
	return p.generateWithBody(ctx, body, width, height)
}

func (p imageProvider) edit(ctx context.Context, req createGenerationTaskRequest, references []referenceImage) ([]generatedImage, error) {
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
		"model":  model,
		"prompt": imageEditPrompt(req, references),
		"size":   size,
	}
	addOptionalImageEditFields(fields, req.Params, count)
	if !p.usesOfficialOpenAIEndpoint() {
		if err := p.addCompatibleReferenceFields(ctx, fields, references); err != nil {
			return nil, err
		}
	}
	if !isGPTImage2Model(model) {
		fields["response_format"] = "b64_json"
	}
	images, err := p.editWithCompatibleFields(ctx, fields, references, width, height)
	if err != nil && isGPTImage2Model(model) {
		return nil, fmt.Errorf("GPT-Image-2 image edit failed: %w", err)
	}
	return images, err
}

func (p imageProvider) generateWithReferences(ctx context.Context, req createGenerationTaskRequest, references []referenceImage, width int, height int) ([]generatedImage, error) {
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
		"prompt":           imageEditPrompt(req, references),
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
	return p.generateWithBody(ctx, body, width, height)
}

func (p imageProvider) editWithCompatibleFields(ctx context.Context, fields map[string]string, references []referenceImage, width int, height int) ([]generatedImage, error) {
	return p.editWithFields(ctx, fields, references, width, height, "image[]")
}

func addOptionalImageEditFields(fields map[string]string, params map[string]any, count int) {
	if count > 1 {
		fields["n"] = strconv.Itoa(count)
	}
	for _, item := range []struct {
		field string
		keys  []string
	}{
		{field: "quality", keys: []string{"quality", "imageQuality"}},
		{field: "output_format", keys: []string{"output_format", "outputFormat"}},
		{field: "moderation", keys: []string{"moderation"}},
	} {
		if value := firstStringParam(params, item.keys...); value != "" {
			if item.field == "quality" {
				value = normalizedLegacyImageQuality(value)
				if value == "" {
					continue
				}
			}
			fields[item.field] = value
		}
	}
	if value, ok := params["output_compression"]; ok && value != nil {
		fields["output_compression"] = fmt.Sprint(value)
	} else if value, ok := params["outputCompression"]; ok && value != nil {
		fields["output_compression"] = fmt.Sprint(value)
	}
}

func normalizedLegacyImageQuality(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "<nil>", "auto", "standard", "medium", "low", "draft":
		return ""
	case "high":
		return "high"
	default:
		return ""
	}
}

func firstStringParam(params map[string]any, keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(fmt.Sprint(params[key]))
		if value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func (p imageProvider) usesOfficialOpenAIEndpoint() bool {
	parsed, err := url.Parse(strings.TrimSpace(p.baseURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "api.openai.com" || strings.HasSuffix(host, ".openai.com")
}

func (p imageProvider) addCompatibleReferenceFields(ctx context.Context, fields map[string]string, references []referenceImage) error {
	referenceURLs, err := p.referenceDataURLs(ctx, references)
	if err != nil {
		return err
	}
	if len(referenceURLs) == 0 {
		return nil
	}
	raw, err := json.Marshal(referenceURLs)
	if err != nil {
		return err
	}
	encoded := string(raw)
	fields["image_url"] = referenceURLs[0]
	fields["image_urls"] = encoded
	fields["reference_images"] = encoded
	fields["referenceImages"] = encoded
	fields["images"] = encoded
	return nil
}

func (p imageProvider) editWithFields(ctx context.Context, fields map[string]string, references []referenceImage, width int, height int, imageField string) ([]generatedImage, error) {
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
		part, err := writer.CreateFormFile(imageField, filename)
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

func (p imageProvider) generateWithBody(ctx context.Context, body map[string]any, width int, height int) ([]generatedImage, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	raw, err := p.doJSONWithRetry(ctx, p.generationsEndpoint(), payload)
	if err != nil {
		return nil, err
	}
	return decodeGeneratedImages(raw, width, height)
}

func (p imageProvider) doJSONWithRetry(ctx context.Context, endpoint string, payload []byte) ([]byte, error) {
	raw, _, err := p.doJSON(ctx, endpoint, payload)
	return raw, err
}

func (p imageProvider) doJSON(ctx context.Context, endpoint string, payload []byte) ([]byte, int, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	res, err := p.client.Do(httpReq)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 10<<20))
	if err != nil {
		return nil, res.StatusCode, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, res.StatusCode, fmt.Errorf("image provider returned %d: %s", res.StatusCode, providerErrorText(raw))
	}
	return raw, res.StatusCode, nil
}

func decodeGeneratedImages(raw []byte, width int, height int) ([]generatedImage, error) {
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
	images := make([]generatedImage, 0, len(decoded.Data))
	for _, item := range decoded.Data {
		imageURL := strings.TrimSpace(item.URL)
		contentType := "image/png"
		if imageURL == "" && item.B64JSON != "" {
			imageURL = "data:image/png;base64," + item.B64JSON
		}
		if imageURL == "" {
			continue
		}
		images = append(images, generatedImage{
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

func imageEditPrompt(req createGenerationTaskRequest, references []referenceImage) string {
	prompt := strings.TrimSpace(firstStringParam(req.Params, "effectivePrompt", "promptForApi"))
	if prompt == "" {
		prompt = strings.TrimSpace(req.Prompt)
	}
	return prompt
}

func (p imageProvider) referenceBytes(ctx context.Context, ref referenceImage, index int) ([]byte, string, error) {
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

func (p imageProvider) referenceDataURLs(ctx context.Context, references []referenceImage) ([]string, error) {
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

func isGPTImage2Model(model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(model))
	normalized = strings.ReplaceAll(normalized, "_", "-")
	return strings.Contains(normalized, "gpt-image-2")
}

func isTransientProviderStatus(status int) bool {
	return status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

func sleepWithContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func providerErrorText(raw []byte) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "empty response"
	}
	var decoded struct {
		Error any `json:"error"`
	}
	if err := json.Unmarshal(raw, &decoded); err == nil && decoded.Error != nil {
		switch value := decoded.Error.(type) {
		case string:
			if message := strings.TrimSpace(value); message != "" {
				return message
			}
		case map[string]any:
			for _, key := range []string{"message", "msg", "detail", "code"} {
				message := strings.TrimSpace(fmt.Sprint(value[key]))
				if message != "" && message != "<nil>" {
					return message
				}
			}
		}
	}
	if len(trimmed) > 600 {
		return trimmed[:600] + "...[truncated]"
	}
	return trimmed
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

func (p imageProvider) generationsEndpoint() string {
	base := strings.TrimRight(p.baseURL, "/")
	parsed, err := url.Parse(base)
	if err == nil && strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), "/v1") {
		return base + "/images/generations"
	}
	return base + "/v1/images/generations"
}

func (p imageProvider) editsEndpoint() string {
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
