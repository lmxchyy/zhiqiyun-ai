package ocr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

type HTTP struct {
	code, endpoint, apiKey string
	client                 *http.Client
}

func NewHTTP(code, endpoint, apiKey string, timeout time.Duration) HTTP {
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	return HTTP{code: strings.TrimSpace(code), endpoint: strings.TrimSpace(endpoint), apiKey: strings.TrimSpace(apiKey), client: &http.Client{Timeout: timeout}}
}
func (p HTTP) Code() string {
	if p.code == "" {
		return "http_ocr"
	}
	return p.code
}
func (p HTTP) Recognize(ctx context.Context, source knowledgeapp.SourceDocument) ([]knowledgeapp.DocumentUnit, error) {
	if p.endpoint == "" {
		return nil, errors.New("OCR endpoint is required")
	}
	body, _ := json.Marshal(map[string]any{"fileName": source.Name, "mimeType": source.MIMEType, "contentBase64": base64.StdEncoding.EncodeToString(source.Content)})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	response, err := p.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(response.Body, 64<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("OCR provider HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(raw)))
	}
	var decoded struct {
		Pages []struct {
			Page  int    `json:"page"`
			Text  string `json:"text"`
			Title string `json:"title"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	units := make([]knowledgeapp.DocumentUnit, 0, len(decoded.Pages))
	for index, page := range decoded.Pages {
		number := page.Page
		if number <= 0 {
			number = index + 1
		}
		units = append(units, knowledgeapp.DocumentUnit{UnitType: "page", UnitNo: index + 1, Title: page.Title, Content: page.Text, Locator: map[string]any{"page": number}, Metadata: map[string]any{"ocr": true, "provider": p.Code()}})
	}
	return units, nil
}

var _ knowledgeapp.OCRProvider = HTTP{}
