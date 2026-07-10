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
	"sort"
	"strings"
	"time"
)

type OpenAICompatibleOptions struct {
	Code      string
	BaseURL   string
	APIKey    string
	Model     string
	Dimension int
	Timeout   time.Duration
}

type OpenAICompatible struct {
	options OpenAICompatibleOptions
	client  *http.Client
}

func NewOpenAICompatible(options OpenAICompatibleOptions) OpenAICompatible {
	if options.Code == "" {
		options.Code = "openai-compatible"
	}
	if options.Timeout <= 0 {
		options.Timeout = 90 * time.Second
	}
	return OpenAICompatible{options: options, client: &http.Client{Timeout: options.Timeout}}
}

func (p OpenAICompatible) Code() string   { return p.options.Code }
func (p OpenAICompatible) Dimension() int { return p.options.Dimension }

func (p OpenAICompatible) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if strings.TrimSpace(p.options.BaseURL) == "" || strings.TrimSpace(p.options.APIKey) == "" || strings.TrimSpace(p.options.Model) == "" {
		return nil, errors.New("embedding provider requires base url, api key and model")
	}
	payload, err := json.Marshal(map[string]any{"model": p.options.Model, "input": texts})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, embeddingEndpoint(p.options.BaseURL), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.options.APIKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("embedding provider returned HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(raw)))
	}
	var decoded struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	if len(decoded.Data) != len(texts) {
		return nil, fmt.Errorf("embedding provider returned %d vectors for %d texts", len(decoded.Data), len(texts))
	}
	sort.Slice(decoded.Data, func(i, j int) bool { return decoded.Data[i].Index < decoded.Data[j].Index })
	result := make([][]float32, 0, len(decoded.Data))
	for _, item := range decoded.Data {
		if p.options.Dimension > 0 && len(item.Embedding) != p.options.Dimension {
			return nil, fmt.Errorf("embedding dimension mismatch: got %d want %d", len(item.Embedding), p.options.Dimension)
		}
		result = append(result, item.Embedding)
	}
	return result, nil
}

func embeddingEndpoint(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(base)
	if err == nil && strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), "/v1") {
		return base + "/embeddings"
	}
	return base + "/v1/embeddings"
}
