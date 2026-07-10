package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

type Gemini struct {
	baseURL, apiKey, model string
	dimension              int
	client                 *http.Client
}

type GeminiOptions struct {
	BaseURL, APIKey, Model string
	Dimension, TimeoutMS   int
}

func NewGemini(options GeminiOptions) Gemini {
	baseURL := strings.TrimRight(strings.TrimSpace(options.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	timeout := options.TimeoutMS
	if timeout <= 0 {
		timeout = 60000
	}
	return Gemini{baseURL: baseURL, apiKey: strings.TrimSpace(options.APIKey), model: strings.TrimSpace(options.Model), dimension: options.Dimension, client: &http.Client{Timeout: time.Duration(timeout) * time.Millisecond}}
}

func (g Gemini) Code() string   { return "gemini" }
func (g Gemini) Dimension() int { return g.dimension }
func (g Gemini) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if g.apiKey == "" || g.model == "" {
		return nil, errors.New("Gemini embedding requires API key and model")
	}
	requests := make([]map[string]any, len(texts))
	model := g.model
	if !strings.HasPrefix(model, "models/") {
		model = "models/" + model
	}
	for index, text := range texts {
		request := map[string]any{"model": model, "content": map[string]any{"parts": []map[string]string{{"text": text}}}}
		if g.dimension > 0 {
			request["outputDimensionality"] = g.dimension
		}
		requests[index] = request
	}
	body, _ := json.Marshal(map[string]any{"requests": requests})
	endpoint := fmt.Sprintf("%s/v1beta/%s:batchEmbedContents?key=%s", g.baseURL, model, url.QueryEscape(g.apiKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Gemini embedding HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(raw)))
	}
	var decoded struct {
		Embeddings []struct {
			Values []float32 `json:"values"`
		} `json:"embeddings"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	if len(decoded.Embeddings) != len(texts) {
		return nil, fmt.Errorf("Gemini returned %d embeddings for %d texts", len(decoded.Embeddings), len(texts))
	}
	result := make([][]float32, len(decoded.Embeddings))
	for index, item := range decoded.Embeddings {
		result[index] = item.Values
	}
	return result, nil
}

var _ knowledgeapp.Embedder = Gemini{}
