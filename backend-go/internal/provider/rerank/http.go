package rerank

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

type HTTPOptions struct {
	Code      string
	BaseURL   string
	APIKey    string
	Model     string
	TimeoutMS int
}

type HTTP struct {
	code, baseURL, apiKey, model string
	client                       *http.Client
}

func NewHTTP(options HTTPOptions) *HTTP {
	timeout := time.Duration(options.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &HTTP{code: strings.TrimSpace(options.Code), baseURL: strings.TrimRight(strings.TrimSpace(options.BaseURL), "/"), apiKey: strings.TrimSpace(options.APIKey), model: strings.TrimSpace(options.Model), client: &http.Client{Timeout: timeout}}
}

func (r *HTTP) Code() string {
	if r.code == "" {
		return "http-rerank"
	}
	return r.code
}

func (r *HTTP) Rerank(ctx context.Context, query string, hits []knowledgeapp.RetrievalHit, topK int) ([]knowledgeapp.RetrievalHit, error) {
	if len(hits) == 0 {
		return hits, nil
	}
	if r == nil || r.baseURL == "" || r.model == "" {
		return nil, fmt.Errorf("rerank endpoint and model are required")
	}
	documents := make([]string, len(hits))
	for index := range hits {
		documents[index] = hits[index].Content
	}
	if topK <= 0 || topK > len(hits) {
		topK = len(hits)
	}
	payload, _ := json.Marshal(map[string]any{"model": r.model, "query": query, "documents": documents, "top_n": topK, "return_documents": false})
	endpoint := r.baseURL
	if !strings.HasSuffix(endpoint, "/rerank") {
		endpoint += "/v1/rerank"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if r.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+r.apiKey)
	}
	res, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("rerank provider returned HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(raw)))
	}
	var response struct {
		Results []struct {
			Index int     `json:"index"`
			Score float64 `json:"relevance_score"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("decode rerank response: %w", err)
	}
	result := make([]knowledgeapp.RetrievalHit, 0, len(response.Results))
	for rank, item := range response.Results {
		if item.Index < 0 || item.Index >= len(hits) {
			continue
		}
		hit := hits[item.Index]
		hit.RerankScore, hit.FinalScore, hit.FinalRank = item.Score, item.Score, rank+1
		result = append(result, hit)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("rerank provider returned no valid results")
	}
	return result, nil
}

var _ knowledgeapp.Reranker = (*HTTP)(nil)
