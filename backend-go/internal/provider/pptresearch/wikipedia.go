package pptresearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
)

var ErrNoResearchEvidence = errors.New("ppt v2 research provider returned no usable evidence")

type WikipediaResearchProvider struct {
	client         *http.Client
	baseURL        string
	articleBaseURL string
	now            func() time.Time
	maxSources     int
}

func NewWikipediaResearchProvider(client *http.Client) *WikipediaResearchProvider {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &WikipediaResearchProvider{client: client, now: time.Now, maxSources: 5}
}

type wikipediaSearchResponse struct {
	Pages []struct {
		ID          int64  `json:"id"`
		Key         string `json:"key"`
		Title       string `json:"title"`
		Excerpt     string `json:"excerpt"`
		Description string `json:"description"`
	} `json:"pages"`
}

func (p *WikipediaResearchProvider) Research(ctx context.Context, intent pptapp.IntentSpec) (pptapp.ResearchPack, error) {
	topic := strings.TrimSpace(intent.Topic)
	if topic == "" {
		return pptapp.ResearchPack{}, ErrNoResearchEvidence
	}
	baseURL, articleBaseURL, providerName := p.endpoints(intent.Language)
	limit := p.maxSources
	if limit <= 0 {
		limit = 5
	}
	endpoint, err := url.Parse(strings.TrimRight(baseURL, "/") + "/w/rest.php/v1/search/page")
	if err != nil {
		return pptapp.ResearchPack{}, err
	}
	query := endpoint.Query()
	query.Set("q", topic)
	query.Set("limit", strconv.Itoa(limit))
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return pptapp.ResearchPack{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "XianZhi-PPT-Agent/1.0")
	response, err := p.httpClient().Do(req)
	if err != nil {
		return pptapp.ResearchPack{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return pptapp.ResearchPack{}, fmt.Errorf("wikipedia research returned HTTP %d", response.StatusCode)
	}
	var result wikipediaSearchResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&result); err != nil {
		return pptapp.ResearchPack{}, err
	}
	retrievedAt := p.clock()().UTC()
	pack := pptapp.ResearchPack{VerificationStatus: pptapp.ResearchVerificationSourceSupported}
	for _, page := range result.Pages {
		title := strings.TrimSpace(page.Title)
		key := strings.TrimSpace(page.Key)
		if title == "" || key == "" {
			continue
		}
		claimText := strings.TrimSpace(page.Description)
		if claimText == "" {
			claimText = stripWikipediaMarkup(page.Excerpt)
		}
		if claimText == "" {
			continue
		}
		identity := "page:" + strconv.FormatInt(page.ID, 10)
		if page.ID <= 0 {
			identity = "key:" + key
		}
		sourceID := pptapp.StableResearchSourceID(providerName, identity)
		locator := strings.TrimRight(articleBaseURL, "/") + "/" + url.PathEscape(key)
		refSuffix := strings.TrimPrefix(sourceID, "source_")
		citationID := "citation_" + refSuffix
		claimID := "claim_" + refSuffix
		pack.Sources = append(pack.Sources, pptapp.ResearchSource{
			ID: sourceID, Provider: providerName, ProviderIdentity: identity, Title: title,
			Type: "encyclopedia", Locator: locator, RetrievedAt: retrievedAt,
		})
		pack.Citations = append(pack.Citations, pptapp.ResearchCitation{ID: citationID, SourceID: sourceID, Locator: locator, RetrievedAt: retrievedAt})
		pack.Claims = append(pack.Claims, pptapp.ResearchClaim{
			ID: claimID, SourceID: sourceID, CitationRefs: []string{citationID}, Text: title + "：" + claimText,
			VerificationStatus: pptapp.ResearchVerificationSourceSupported,
		})
	}
	pack, err = pptapp.NormalizeResearchPack(pack)
	if err != nil {
		return pptapp.ResearchPack{}, err
	}
	if len(pack.Claims) == 0 {
		return pptapp.ResearchPack{}, ErrNoResearchEvidence
	}
	return pack, nil
}

func (p *WikipediaResearchProvider) endpoints(language string) (string, string, string) {
	languageCode := "en"
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "zh") {
		languageCode = "zh"
	}
	baseURL := strings.TrimSpace(p.baseURL)
	if baseURL == "" {
		baseURL = "https://" + languageCode + ".wikipedia.org"
	}
	articleBaseURL := strings.TrimSpace(p.articleBaseURL)
	if articleBaseURL == "" {
		articleBaseURL = "https://" + languageCode + ".wikipedia.org/wiki/"
	}
	return baseURL, articleBaseURL, "wikipedia-" + languageCode
}

func (p *WikipediaResearchProvider) httpClient() *http.Client {
	if p.client != nil {
		return p.client
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func (p *WikipediaResearchProvider) clock() func() time.Time {
	if p.now != nil {
		return p.now
	}
	return time.Now
}

var wikipediaHTMLTag = regexp.MustCompile(`<[^>]+>`)
var wikipediaWhitespace = regexp.MustCompile(`\s+`)

func stripWikipediaMarkup(value string) string {
	value = wikipediaHTMLTag.ReplaceAllString(value, "")
	value = html.UnescapeString(value)
	return strings.TrimSpace(wikipediaWhitespace.ReplaceAllString(value, " "))
}

var _ pptapp.ResearchProvider = (*WikipediaResearchProvider)(nil)
