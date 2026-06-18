package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
		client:  &http.Client{Timeout: time.Duration(timeoutMS) * time.Millisecond},
	}
}

func (p imageProvider) enabled() bool {
	return p.baseURL != "" && p.apiKey != ""
}

func (p imageProvider) generate(ctx context.Context, req createGenerationTaskRequest) ([]generatedImage, error) {
	if !p.enabled() {
		return nil, nil
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

func (p imageProvider) generateWithBody(ctx context.Context, body map[string]any, width int, height int) ([]generatedImage, error) {
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
