package image

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"xianzhi-ai/backend-go/internal/app/generation"
	"xianzhi-ai/backend-go/internal/providerexecution"

	"xianzhi-ai/backend-go/internal/config"

	_ "image/gif"
)

const (
	maxCompatibleReferencePartBytes  = 900 << 10
	maxCompatibleReferenceFieldBytes = 800 << 10
	maxReferenceImageBytes           = 10 << 20
)

type providerOperationKeyContextKey struct{}

type OpenAICompatible struct {
	code               string
	baseURL            string
	apiKey             string
	model              string
	models             []string
	generationEndpoint string
	editEndpoint       string
	responseEndpoint   string
	referenceImageDir  string
	client             *http.Client
}

func NewOpenAICompatible(cfg config.Config) OpenAICompatible {
	model := strings.TrimSpace(cfg.ImageModel)
	return OpenAICompatible{
		code:              "openai-compatible",
		baseURL:           strings.TrimSpace(cfg.ModelProviderURL),
		apiKey:            strings.TrimSpace(cfg.ModelProviderAPIKey),
		model:             model,
		models:            nonEmptyStrings(model),
		referenceImageDir: referenceImageDirFromDataPath(cfg.DataPath),
		client:            generationHTTPClient(cfg.ImageProviderTimeout()),
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
		code:               strings.TrimSpace(opts.Code),
		baseURL:            strings.TrimSpace(opts.BaseURL),
		apiKey:             strings.TrimSpace(opts.APIKey),
		model:              model,
		models:             models,
		generationEndpoint: strings.TrimSpace(opts.ImageGenerationEndpoint),
		editEndpoint:       strings.TrimSpace(opts.ImageEditEndpoint),
		responseEndpoint:   strings.TrimSpace(opts.ResponseEndpoint),
		referenceImageDir:  strings.TrimSpace(opts.ReferenceImageDir),
		client:             generationHTTPClient(time.Duration(timeoutMS) * time.Millisecond),
	}
}

type OpenAICompatibleOptions struct {
	Code                    string
	BaseURL                 string
	APIKey                  string
	ImageModel              string
	Models                  []string
	ImageGenerationEndpoint string
	ImageEditEndpoint       string
	ResponseEndpoint        string
	ReferenceImageDir       string
	TimeoutMS               int
}

func generationHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			// 307/308 can replay a non-idempotent POST without the execution guard
			// observing the intermediate response.
			return http.ErrUseLastResponse
		},
	}
}

func referenceImageDirFromDataPath(dataPath string) string {
	base := filepath.Dir(strings.TrimSpace(dataPath))
	if base == "." || base == "" {
		base = "data"
	}
	return filepath.Join(base, "reference-images")
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
	if key := strings.TrimSpace(req.ClientRequestID); key != "" && p.usesOfficialOpenAIEndpoint() {
		// OpenAI supports Idempotency-Key. Other compatible gateways are
		// deliberately treated as having no verified native idempotency.
		ctx = context.WithValue(ctx, providerOperationKeyContextKey{}, key)
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = p.model
	}
	if model == "" || isGPTImage2Model(model) {
		if err := validateOpenAIImageParameters(req.Params); err != nil {
			return nil, err
		}
	}
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
		preparedReferences, prepareErr := p.prepareReferenceImages(ctx, references)
		if prepareErr != nil {
			return nil, prepareErr
		}
		references = preparedReferences
		return p.edit(ctx, req, references)
	}
	count, err := openAIImageCount(req.Params)
	if err != nil {
		return nil, err
	}
	size, width, height := imageSize(req.Params)
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = p.model
	}
	if model == "" {
		model = "gpt-image-2"
	}
	if shouldTryResponsesFirst(req.Params) {
		return p.generateWithResponses(ctx, req, width, height)
	}
	body := map[string]any{
		"model":  model,
		"prompt": req.Prompt,
		"n":      count,
		"size":   size,
	}
	addOptionalImageGenerationFields(body, req.Params)
	if !isGPTImage2Model(model) {
		body["response_format"] = "b64_json"
	}
	return p.generateWithBody(ctx, body, width, height)
}

func (p OpenAICompatible) generateWithResponses(ctx context.Context, req generation.CreateRequest, width int, height int) ([]generation.GeneratedImage, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = p.model
	}
	if model == "" {
		model = "gpt-image-2"
	}
	tool := responsesImageTool(req.Params, false)
	body := map[string]any{
		"model":       model,
		"input":       responsesInput(strings.TrimSpace(req.Prompt), nil),
		"tools":       []map[string]any{tool},
		"tool_choice": "required",
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	raw, err := p.doJSONWithRetry(ctx, p.responsesEndpoint(), payload)
	if err != nil {
		return nil, err
	}
	return decodeResponsesGeneratedImages(raw, width, height)
}

func addOptionalImageGenerationFields(body map[string]any, params map[string]any) {
	if quality := gptImageProviderQuality(params); quality != "" {
		body["quality"] = quality
	}
	for _, item := range []struct {
		field string
		keys  []string
	}{
		{field: "background", keys: []string{"background"}},
		{field: "output_format", keys: []string{"output_format", "outputFormat"}},
		{field: "output_compression", keys: []string{"output_compression", "outputCompression"}},
		{field: "moderation", keys: []string{"moderation"}},
	} {
		if value := firstStringParam(params, item.keys...); value != "" {
			body[item.field] = value
		}
	}
}

func (p OpenAICompatible) edit(ctx context.Context, req generation.CreateRequest, references []referenceImage) ([]generation.GeneratedImage, error) {
	count, err := openAIImageCount(req.Params)
	if err != nil {
		return nil, err
	}
	size, width, height := imageSize(req.Params)
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = p.model
	}
	if model == "" {
		model = "gpt-image-2"
	}
	if shouldTryResponsesFirst(req.Params) {
		return p.editWithResponses(ctx, req, references, width, height)
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
	return p.editWithCompatibleFields(ctx, fields, references, width, height)
}

func (p OpenAICompatible) generateWithReferences(ctx context.Context, req generation.CreateRequest, references []referenceImage, width int, height int) ([]generation.GeneratedImage, error) {
	count, err := openAIImageCount(req.Params)
	if err != nil {
		return nil, err
	}
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

func (p OpenAICompatible) editWithCompatibleFields(ctx context.Context, fields map[string]string, references []referenceImage, width int, height int) ([]generation.GeneratedImage, error) {
	imageAliases := []string{}
	if !p.usesOfficialOpenAIEndpoint() {
		imageAliases = append(imageAliases, "image")
	}
	return p.editWithFields(ctx, fields, references, width, height, "image[]", imageAliases...)
}

func addOptionalImageEditFields(fields map[string]string, params map[string]any, count int) {
	if count > 1 {
		fields["n"] = strconv.Itoa(count)
	}
	for _, item := range []struct {
		field string
		keys  []string
	}{
		{field: "output_format", keys: []string{"output_format", "outputFormat"}},
		{field: "moderation", keys: []string{"moderation"}},
	} {
		if value := firstStringParam(params, item.keys...); value != "" {
			fields[item.field] = value
		}
	}
	if quality := gptImageProviderQuality(params); quality != "" {
		fields["quality"] = quality
	}
	if value, ok := params["output_compression"]; ok && value != nil {
		fields["output_compression"] = fmt.Sprint(value)
	} else if value, ok := params["outputCompression"]; ok && value != nil {
		fields["output_compression"] = fmt.Sprint(value)
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

func (p OpenAICompatible) usesOfficialOpenAIEndpoint() bool {
	parsed, err := url.Parse(strings.TrimSpace(p.baseURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "api.openai.com" || strings.HasSuffix(host, ".openai.com")
}

func (p OpenAICompatible) addCompatibleReferenceFields(ctx context.Context, fields map[string]string, references []referenceImage) error {
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
	if len(referenceURLs[0]) <= maxCompatibleReferenceFieldBytes {
		fields["image_url"] = referenceURLs[0]
	}
	if len(encoded) <= maxCompatibleReferenceFieldBytes {
		fields["image_urls"] = encoded
		fields["reference_images"] = encoded
		fields["referenceImages"] = encoded
		fields["images"] = encoded
	}
	return nil
}

func (p OpenAICompatible) editWithFields(ctx context.Context, fields map[string]string, references []referenceImage, width int, height int, imageField string, aliasImageFields ...string) ([]generation.GeneratedImage, error) {
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
		for _, aliasField := range aliasImageFields {
			if strings.TrimSpace(aliasField) == "" || aliasField == imageField {
				continue
			}
			aliasPart, err := writer.CreateFormFile(aliasField, filename)
			if err != nil {
				return nil, err
			}
			if _, err := aliasPart.Write(raw); err != nil {
				return nil, err
			}
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
	setImageIdempotencyHeader(ctx, httpReq)

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
	raw, err := p.doJSONWithRetry(ctx, p.generationsEndpoint(), payload)
	if err != nil {
		return nil, err
	}
	return decodeGeneratedImages(raw, width, height)
}

func (p OpenAICompatible) doJSONWithRetry(ctx context.Context, endpoint string, payload []byte) ([]byte, error) {
	// Generation POSTs are not retried: any transport/HTTP response after Do
	// starts is potentially an accepted operation at the provider.
	raw, _, err := p.doJSON(ctx, endpoint, payload)
	return raw, err
}

func (p OpenAICompatible) doJSON(ctx context.Context, endpoint string, payload []byte) ([]byte, int, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	setImageIdempotencyHeader(ctx, httpReq)

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
		err := fmt.Errorf("image provider returned %d: %s", res.StatusCode, providerErrorText(raw))
		if res.StatusCode == http.StatusTooManyRequests || res.StatusCode == http.StatusForbidden || res.StatusCode == http.StatusUnauthorized {
			return nil, res.StatusCode, providerexecution.ClassifiedError{Class: providerexecution.DefinitiveNotSubmitted, Err: err}
		}
		return nil, res.StatusCode, err
	}
	return raw, res.StatusCode, nil
}

func (p OpenAICompatible) editWithResponses(ctx context.Context, req generation.CreateRequest, references []referenceImage, width int, height int) ([]generation.GeneratedImage, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = p.model
	}
	if model == "" {
		model = "gpt-image-2"
	}
	prompt := imageEditPrompt(req, references)
	referenceURLs, err := p.referenceDataURLs(ctx, references)
	if err != nil {
		return nil, err
	}
	if len(referenceURLs) == 0 {
		return nil, errors.New("reference image is required for responses image edit")
	}
	tool := responsesImageTool(req.Params, true)
	body := map[string]any{
		"model":       model,
		"input":       responsesInput(prompt, referenceURLs),
		"tools":       []map[string]any{tool},
		"tool_choice": "required",
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.responsesEndpoint(), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	setImageIdempotencyHeader(ctx, httpReq)

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
		return nil, fmt.Errorf("image provider responses edit returned %d: %s", res.StatusCode, string(raw))
	}
	return decodeResponsesGeneratedImages(raw, width, height)
}

func setImageIdempotencyHeader(ctx context.Context, req *http.Request) {
	if key, _ := ctx.Value(providerOperationKeyContextKey{}).(string); strings.TrimSpace(key) != "" {
		req.Header.Set("Idempotency-Key", key)
	}
}

func responsesImageTool(params map[string]any, isEdit bool) map[string]any {
	action := "generate"
	if isEdit {
		action = "edit"
	}
	tool := map[string]any{
		"type":   "image_generation",
		"action": action,
	}
	if size := normalizedResponsesImageSize(params); size != "" {
		tool["size"] = size
	}
	if quality := gptImageProviderQuality(params); quality != "" {
		tool["quality"] = quality
	}
	if outputFormat := firstStringParam(params, "output_format", "outputFormat"); outputFormat != "" {
		tool["output_format"] = outputFormat
	}
	if moderation := firstStringParam(params, "moderation"); moderation != "" {
		tool["moderation"] = moderation
	}
	if value, ok := params["output_compression"]; ok && value != nil && fmt.Sprint(value) != "<nil>" {
		tool["output_compression"] = value
	} else if value, ok := params["outputCompression"]; ok && value != nil && fmt.Sprint(value) != "<nil>" {
		tool["output_compression"] = value
	}
	return tool
}

func normalizedResponsesImageSize(params map[string]any) string {
	size, _, _ := imageSize(params)
	return size
}

func responsesInput(prompt string, referenceURLs []string) []map[string]any {
	content := make([]map[string]any, 0, len(referenceURLs)+1)
	content = append(content, map[string]any{
		"type": "input_text",
		"text": "Use the following text as the complete prompt. Do not rewrite it:\n" + prompt,
	})
	for _, imageURL := range referenceURLs {
		content = append(content, map[string]any{
			"type":      "input_image",
			"image_url": imageURL,
		})
	}
	return []map[string]any{{
		"role":    "user",
		"content": content,
	}}
}

func imageEditPrompt(req generation.CreateRequest, references []referenceImage) string {
	prompt := strings.TrimSpace(firstStringParam(req.Params, "effectivePrompt", "promptForApi"))
	if prompt == "" {
		prompt = strings.TrimSpace(req.Prompt)
	}
	return prompt
}

func shouldTryResponsesFirst(params map[string]any) bool {
	requestMode := strings.ToLower(strings.TrimSpace(fmt.Sprint(params["imageRequestMode"])))
	if requestMode == "responses" || requestMode == "openai-responses" {
		return true
	}
	if requestMode != "" && requestMode != "<nil>" {
		return false
	}
	apiMode := strings.ToLower(strings.TrimSpace(fmt.Sprint(params["apiMode"])))
	if apiMode == "responses" || apiMode == "openai-responses" {
		return true
	}
	return false
}

func normalizedImageQuality(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "auto", "low", "medium", "high":
		return strings.ToLower(strings.TrimSpace(value))
	case "standard":
		return "auto"
	case "draft":
		return "low"
	default:
		return ""
	}
}

func gptImageProviderQuality(params map[string]any) string {
	if quality := normalizedImageQuality(firstStringParam(params, "quality")); quality != "" {
		return quality
	}
	return "low"
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

func decodeResponsesGeneratedImages(raw []byte, width int, height int) ([]generation.GeneratedImage, error) {
	var decoded struct {
		Output []struct {
			Type   string `json:"type"`
			Result any    `json:"result"`
		} `json:"output"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	images := make([]generation.GeneratedImage, 0, len(decoded.Output))
	for _, item := range decoded.Output {
		if item.Type != "image_generation_call" {
			continue
		}
		imageURL := responseResultImageURL(item.Result)
		if imageURL == "" {
			continue
		}
		images = append(images, generation.GeneratedImage{
			URL:         imageURL,
			ContentType: "image/png",
			Width:       width,
			Height:      height,
			Source:      "model-provider",
		})
	}
	if len(images) == 0 {
		return nil, errors.New("responses image provider returned no images")
	}
	return images, nil
}

func responseResultImageURL(result any) string {
	switch value := result.(type) {
	case string:
		return normalizeResponseImageValue(value)
	case map[string]any:
		for _, key := range []string{"b64_json", "base64", "image", "data", "url"} {
			if imageURL := normalizeResponseImageValue(strings.TrimSpace(fmt.Sprint(value[key]))); imageURL != "" {
				return imageURL
			}
		}
	}
	return ""
}

func normalizeResponseImageValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "<nil>" {
		return ""
	}
	if strings.HasPrefix(value, "data:image/") || strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	return "data:image/png;base64," + value
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
	if localPath, ok := p.localReferenceImagePath(ref.URL); ok {
		raw, contentType, err := readLocalReferenceImage(localPath)
		if err != nil {
			return nil, "", err
		}
		return raw, imageFilename(name, contentType), nil
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
		urls := []string{ref.URL}
		if strings.HasPrefix(ref.URL, "http://") {
			urls = append(urls, "https://"+strings.TrimPrefix(ref.URL, "http://"))
		}
		var lastErr error
		for _, imageURL := range urls {
			for attempt := 0; attempt < 3; attempt++ {
				raw, contentType, err := p.fetchReferenceImage(ctx, imageURL)
				if err == nil {
					return raw, imageFilename(name, contentType), nil
				}
				lastErr = err
				if !isRetryableReferenceError(err) {
					break
				}
				select {
				case <-ctx.Done():
					return nil, "", ctx.Err()
				case <-time.After(time.Duration(attempt+1) * 500 * time.Millisecond):
				}
			}
		}
		return nil, "", lastErr
	}
	return nil, "", fmt.Errorf("reference image must be data URL or HTTP URL")
}

func (p OpenAICompatible) fetchReferenceImage(ctx context.Context, imageURL string) ([]byte, string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, "", err
	}
	httpReq.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36")
	res, err := p.client.Do(httpReq)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, "", referenceStatusError{status: res.StatusCode}
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxReferenceImageBytes))
	if err != nil {
		return nil, "", err
	}
	return raw, res.Header.Get("Content-Type"), nil
}

func (p OpenAICompatible) localReferenceImagePath(rawURL string) (string, bool) {
	if strings.TrimSpace(p.referenceImageDir) == "" {
		return "", false
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", false
	}
	const prefix = "/api/v1/reference-images/"
	if !strings.HasPrefix(parsed.Path, prefix) {
		return "", false
	}
	name := strings.TrimSpace(strings.TrimPrefix(parsed.Path, prefix))
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return "", false
	}
	return filepath.Join(p.referenceImageDir, name), true
}

func readLocalReferenceImage(path string) ([]byte, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("open local reference image: %w", err)
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxReferenceImageBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("read local reference image: %w", err)
	}
	if len(raw) == 0 {
		return nil, "", errors.New("local reference image is empty")
	}
	if len(raw) > maxReferenceImageBytes {
		return nil, "", errors.New("local reference image is too large")
	}
	return raw, http.DetectContentType(raw), nil
}

type referenceStatusError struct {
	status int
}

func (e referenceStatusError) Error() string {
	return fmt.Sprintf("reference image returned %d", e.status)
}

func isRetryableReferenceError(err error) bool {
	if err == nil {
		return false
	}
	var statusErr referenceStatusError
	if errors.As(err, &statusErr) {
		return statusErr.status == http.StatusTooManyRequests || statusErr.status >= 500
	}
	return isTimeoutError(err)
}

func (p OpenAICompatible) prepareReferenceImages(ctx context.Context, references []referenceImage) ([]referenceImage, error) {
	prepared := make([]referenceImage, 0, len(references))
	for index, ref := range references {
		if strings.HasPrefix(ref.URL, "data:image/") {
			prepared = append(prepared, ref)
			continue
		}
		raw, filename, err := p.referenceBytes(ctx, ref, index)
		if err != nil {
			return nil, err
		}
		raw, filename = normalizeReferenceUpload(raw, filename)
		ref.URL = referenceDataURL(raw, filename)
		if strings.TrimSpace(ref.Name) == "" {
			ref.Name = filename
		}
		prepared = append(prepared, ref)
	}
	return prepared, nil
}

func normalizeReferenceUpload(raw []byte, filename string) ([]byte, string) {
	if len(raw) <= maxCompatibleReferencePartBytes {
		return raw, filename
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return raw, filename
	}
	if encoded, ok := encodeReferenceJPEG(img, 88); ok && len(encoded) <= maxCompatibleReferencePartBytes {
		return encoded, jpegReferenceFilename(filename)
	}
	for _, maxSide := range []int{768, 640, 512} {
		scaled := scaleImageToFit(img, maxSide)
		for _, quality := range []int{86, 80, 74} {
			if encoded, ok := encodeReferenceJPEG(scaled, quality); ok && len(encoded) <= maxCompatibleReferencePartBytes {
				return encoded, jpegReferenceFilename(filename)
			}
		}
	}
	if encoded, ok := encodeReferenceJPEG(scaleImageToFit(img, 512), 68); ok {
		return encoded, jpegReferenceFilename(filename)
	}
	return raw, filename
}

func encodeReferenceJPEG(src image.Image, quality int) ([]byte, bool) {
	flattened := image.NewRGBA(src.Bounds())
	for y := flattened.Bounds().Min.Y; y < flattened.Bounds().Max.Y; y++ {
		for x := flattened.Bounds().Min.X; x < flattened.Bounds().Max.X; x++ {
			flattened.Set(x, y, color.White)
		}
	}
	drawImage(flattened, src)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, flattened, &jpeg.Options{Quality: quality}); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

func drawImage(dst *image.RGBA, src image.Image) {
	srcBounds := src.Bounds()
	for y := srcBounds.Min.Y; y < srcBounds.Max.Y; y++ {
		for x := srcBounds.Min.X; x < srcBounds.Max.X; x++ {
			dst.Set(x, y, src.At(x, y))
		}
	}
}

func scaleImageToFit(src image.Image, maxSide int) image.Image {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= maxSide && height <= maxSide {
		return src
	}
	ratio := float64(maxSide) / float64(width)
	if height > width {
		ratio = float64(maxSide) / float64(height)
	}
	newWidth := int(math.Round(float64(width) * ratio))
	newHeight := int(math.Round(float64(height) * ratio))
	if newWidth < 1 {
		newWidth = 1
	}
	if newHeight < 1 {
		newHeight = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	for y := 0; y < newHeight; y++ {
		srcY := bounds.Min.Y + int(float64(y)*float64(height)/float64(newHeight))
		if srcY >= bounds.Max.Y {
			srcY = bounds.Max.Y - 1
		}
		for x := 0; x < newWidth; x++ {
			srcX := bounds.Min.X + int(float64(x)*float64(width)/float64(newWidth))
			if srcX >= bounds.Max.X {
				srcX = bounds.Max.X - 1
			}
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}
	return dst
}

func jpegReferenceFilename(filename string) string {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return "reference.jpg"
	}
	lower := strings.ToLower(filename)
	for _, ext := range []string{".png", ".webp", ".gif", ".jpeg", ".jpg"} {
		if strings.HasSuffix(lower, ext) {
			return filename[:len(filename)-len(ext)] + ".jpg"
		}
	}
	return filename + ".jpg"
}

func (p OpenAICompatible) referenceDataURLs(ctx context.Context, references []referenceImage) ([]string, error) {
	items := make([]string, 0, len(references))
	for index, ref := range references {
		raw, filename, err := p.referenceBytes(ctx, ref, index)
		if err != nil {
			return nil, err
		}
		items = append(items, referenceDataURL(raw, filename))
	}
	return items, nil
}

func referenceDataURL(raw []byte, filename string) string {
	contentType := "image/png"
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		contentType = "image/jpeg"
	case strings.HasSuffix(lower, ".webp"):
		contentType = "image/webp"
	}
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(raw)
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
	return status == http.StatusTooManyRequests ||
		status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable ||
		status == http.StatusGatewayTimeout
}

func isRetryableProviderError(status int, err error) bool {
	// Retained for compatibility with focused tests/callers: generation POSTs
	// are never generically retryable, including 429/502/503/504.
	return false
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

func (p OpenAICompatible) generationsEndpoint() string {
	return providerEndpoint(p.baseURL, p.generationEndpoint, "images/generations")
}

func (p OpenAICompatible) editsEndpoint() string {
	return providerEndpoint(p.baseURL, p.editEndpoint, "images/edits")
}

func (p OpenAICompatible) responsesEndpoint() string {
	return providerEndpoint(p.baseURL, p.responseEndpoint, "responses")
}

func providerEndpoint(baseURL string, configuredPath string, defaultPath string) string {
	base := strings.TrimRight(baseURL, "/")
	path := strings.TrimSpace(configuredPath)
	if path == "" {
		parsed, err := url.Parse(base)
		if err == nil && strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), "/v1") {
			return base + "/" + strings.TrimLeft(defaultPath, "/")
		}
		return base + "/v1/" + strings.TrimLeft(defaultPath, "/")
	}
	if parsed, err := url.Parse(path); err == nil && parsed.IsAbs() {
		return path
	}
	if strings.HasPrefix(path, "/") {
		parsed, err := url.Parse(base)
		if err == nil && parsed.Scheme != "" && parsed.Host != "" {
			return parsed.Scheme + "://" + parsed.Host + path
		}
		return base + path
	}
	return base + "/" + strings.TrimLeft(path, "/")
}

func imageSize(params map[string]any) (string, int, int) {
	raw := strings.ToLower(strings.TrimSpace(fmt.Sprint(params["size"])))
	if raw == "" || raw == "<nil>" || raw == "auto" {
		return "auto", 0, 0
	}
	width, height, ok := parseGPTImageSize(raw)
	if !ok {
		return raw, 0, 0
	}
	return fmt.Sprintf("%dx%d", width, height), width, height
}

func validateOpenAIImageParameters(params map[string]any) error {
	if err := ValidateGPTImageSize(params["size"]); err != nil {
		return err
	}
	if err := ValidateGPTImageQuality(params["quality"]); err != nil {
		return err
	}
	_, err := openAIImageCount(params)
	return err
}

func openAIImageCount(params map[string]any) (int, error) {
	value, ok := params["n"]
	if !ok {
		value, ok = params["count"]
	}
	if !ok {
		return 1, nil
	}
	return parseGPTImageCount(value)
}

const (
	gptImageMinPixels    = 655360
	gptImageMaxPixels    = 8294400
	gptImageMaxEdge      = 3840
	gptImageMaxAspect    = 3
	gptImageMinCount     = 1
	gptImageMaxCount     = 10
	gptImageSizeMultiple = 16
)

func ValidateGPTImageSize(value any) error {
	if value == nil {
		return nil
	}
	size := strings.ToLower(strings.TrimSpace(fmt.Sprint(value)))
	if size == "" || size == "<nil>" || size == "auto" {
		return nil
	}
	width, height, ok := parseGPTImageSize(size)
	if !ok {
		return fmt.Errorf("unsupported OpenAI image size %q; use auto or WIDTHxHEIGHT", size)
	}
	if width%gptImageSizeMultiple != 0 || height%gptImageSizeMultiple != 0 {
		return fmt.Errorf("unsupported OpenAI image size %q; width and height must be multiples of 16", size)
	}
	if width > gptImageMaxEdge || height > gptImageMaxEdge {
		return fmt.Errorf("unsupported OpenAI image size %q; each edge must be <= %d", size, gptImageMaxEdge)
	}
	longEdge, shortEdge := width, height
	if height > width {
		longEdge, shortEdge = height, width
	}
	if shortEdge == 0 || longEdge > shortEdge*gptImageMaxAspect {
		return fmt.Errorf("unsupported OpenAI image size %q; aspect ratio must be between 1:3 and 3:1", size)
	}
	pixels := width * height
	if pixels < gptImageMinPixels || pixels > gptImageMaxPixels {
		return fmt.Errorf("unsupported OpenAI image size %q; pixel count must be between %d and %d", size, gptImageMinPixels, gptImageMaxPixels)
	}
	return nil
}

func ValidateGPTImageQuality(value any) error {
	if value == nil {
		return nil
	}
	quality := strings.ToLower(strings.TrimSpace(fmt.Sprint(value)))
	switch quality {
	case "", "<nil>", "auto", "low", "medium", "high":
		return nil
	default:
		return fmt.Errorf("unsupported OpenAI image quality %q; supported qualities: auto, low, medium, high", quality)
	}
}

func parseGPTImageSize(size string) (int, int, bool) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(size)), "x")
	if len(parts) != 2 {
		return 0, 0, false
	}
	width, err := strconv.Atoi(parts[0])
	if err != nil || width <= 0 {
		return 0, 0, false
	}
	height, err := strconv.Atoi(parts[1])
	if err != nil || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}

func parseGPTImageCount(value any) (int, error) {
	var count int
	switch typed := value.(type) {
	case int:
		count = typed
	case int32:
		count = int(typed)
	case int64:
		count = int(typed)
	case float64:
		if typed != float64(int(typed)) {
			return 0, fmt.Errorf("unsupported OpenAI image count %v; expected an integer from %d to %d", value, gptImageMinCount, gptImageMaxCount)
		}
		count = int(typed)
	default:
		return 0, fmt.Errorf("unsupported OpenAI image count %v; expected an integer from %d to %d", value, gptImageMinCount, gptImageMaxCount)
	}
	if count < gptImageMinCount || count > gptImageMaxCount {
		return 0, fmt.Errorf("unsupported OpenAI image count %d; expected an integer from %d to %d", count, gptImageMinCount, gptImageMaxCount)
	}
	return count, nil
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
