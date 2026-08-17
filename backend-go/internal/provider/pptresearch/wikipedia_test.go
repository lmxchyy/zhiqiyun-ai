package pptresearch

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
)

func TestWikipediaResearchProviderReturnsStructuredTraceableEvidence(t *testing.T) {
	retrievedAt := time.Date(2026, 8, 16, 8, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/w/rest.php/v1/search/page" || r.URL.Query().Get("q") != "新能源汽车" || r.URL.Query().Get("limit") != "5" {
			t.Fatalf("unexpected wikipedia request: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pages":[{"id":123,"key":"新能源汽车","title":"新能源汽车","excerpt":"<span class=\"searchmatch\">新能源汽车</span>采用新型动力系统。","description":"采用非常规车用燃料或新型车载动力装置的汽车"},{"id":123,"key":"新能源汽车","title":"重复结果","description":"应被稳定来源去重"}]}`))
	}))
	defer server.Close()
	provider := &WikipediaResearchProvider{
		client: server.Client(), baseURL: server.URL, articleBaseURL: server.URL + "/wiki/",
		now: func() time.Time { return retrievedAt }, maxSources: 5,
	}
	pack, err := provider.Research(t.Context(), pptapp.IntentSpec{Topic: "新能源汽车", Language: "zh-CN", ResearchRequired: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Sources) != 1 || len(pack.Claims) != 1 || len(pack.Citations) != 1 || len(pack.Datasets) != 0 {
		t.Fatalf("research pack is not structured/deduplicated: %+v", pack)
	}
	if pack.Sources[0].ID != pptapp.StableResearchSourceID("wikipedia-zh", "page:123") || pack.Sources[0].RetrievedAt != retrievedAt {
		t.Fatalf("source identity mismatch: %+v", pack.Sources[0])
	}
	if pack.Claims[0].SourceID != pack.Sources[0].ID || len(pack.Claims[0].CitationRefs) != 1 || pack.Claims[0].CitationRefs[0] != pack.Citations[0].ID || pack.Citations[0].SourceID != pack.Sources[0].ID {
		t.Fatalf("Source -> Claim -> Citation trace is broken: %+v", pack)
	}
	if err := pptapp.ValidateResearchPack(pack); err != nil {
		t.Fatalf("provider returned invalid ResearchPack: %v", err)
	}
}

func TestWikipediaResearchProviderRejectsEmptyEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pages":[]}`))
	}))
	defer server.Close()
	provider := &WikipediaResearchProvider{client: server.Client(), baseURL: server.URL, articleBaseURL: server.URL + "/wiki/", now: time.Now, maxSources: 5}
	if _, err := provider.Research(t.Context(), pptapp.IntentSpec{Topic: "unknown", Language: "en-US", ResearchRequired: true}); err == nil {
		t.Fatal("empty research evidence was accepted")
	}
}
