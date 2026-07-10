package vectorstore

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"unicode"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

type Memory struct {
	mu      sync.RWMutex
	records map[string]map[string]knowledgeapp.VectorRecord
}

func NewMemory() *Memory {
	return &Memory{records: map[string]map[string]knowledgeapp.VectorRecord{}}
}

func (m *Memory) Code() string { return "memory" }

func (m *Memory) Upsert(_ context.Context, indexID string, records []knowledgeapp.VectorRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.records[indexID] == nil {
		m.records[indexID] = map[string]knowledgeapp.VectorRecord{}
	}
	for _, record := range records {
		m.records[indexID][record.ChunkID] = record
	}
	return nil
}

func (m *Memory) DeleteByDocumentVersion(_ context.Context, access knowledgeapp.AccessContext, indexID string, documentVersionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for chunkID, record := range m.records[indexID] {
		if record.TenantID == access.TenantID && record.DocumentVersionID == documentVersionID {
			delete(m.records[indexID], chunkID)
		}
	}
	return nil
}

func (m *Memory) Search(_ context.Context, request knowledgeapp.SearchRequest, queryVector []float32) ([]knowledgeapp.RetrievalHit, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	allowedKB := map[string]bool{}
	for _, id := range request.KnowledgeBaseIDs {
		allowedKB[id] = true
	}
	mode := strings.ToUpper(strings.TrimSpace(request.Mode))
	if mode == "" {
		mode = "HYBRID"
	}
	vectorWeight, keywordWeight := request.VectorWeight, request.KeywordWeight
	if vectorWeight == 0 && keywordWeight == 0 {
		vectorWeight, keywordWeight = 0.7, 0.3
	}
	hits := []knowledgeapp.RetrievalHit{}
	for _, index := range m.records {
		for _, record := range index {
			if record.TenantID != request.Access.TenantID || (len(allowedKB) > 0 && !allowedKB[record.KnowledgeBaseID]) {
				continue
			}
			if !matchesFilters(record.FilterMetadata, request.Filters) {
				continue
			}
			vectorScore := cosine(queryVector, record.Embedding)
			keywordScore := keywordSimilarity(request.Query, record.SearchText)
			finalScore := vectorScore
			switch mode {
			case "FULLTEXT":
				finalScore = keywordScore
			case "HYBRID":
				finalScore = vectorScore*vectorWeight + keywordScore*keywordWeight
			}
			if finalScore < request.Threshold {
				continue
			}
			hits = append(hits, hitFromRecord(record, vectorScore, keywordScore, finalScore))
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].FinalScore > hits[j].FinalScore })
	topK := request.TopK
	if topK <= 0 {
		topK = 8
	}
	if len(hits) > topK {
		hits = hits[:topK]
	}
	for index := range hits {
		hits[index].InitialRank = index + 1
		hits[index].FinalRank = index + 1
	}
	return hits, nil
}

func matchesFilters(metadata map[string]any, filters map[string]any) bool {
	for key, expected := range filters {
		actual, exists := metadata[key]
		if !exists || !filterValueEqual(actual, expected) {
			return false
		}
	}
	return true
}

func filterValueEqual(actual any, expected any) bool {
	switch value := expected.(type) {
	case map[string]any:
		actualMap, ok := actual.(map[string]any)
		return ok && matchesFilters(actualMap, value)
	case []any:
		actualItems, ok := actual.([]any)
		if !ok || len(actualItems) != len(value) {
			return false
		}
		for index := range value {
			if !filterValueEqual(actualItems[index], value[index]) {
				return false
			}
		}
		return true
	default:
		return strings.EqualFold(strings.TrimSpace(fmt.Sprint(actual)), strings.TrimSpace(fmt.Sprint(expected)))
	}
}

func hitFromRecord(record knowledgeapp.VectorRecord, vectorScore float64, keywordScore float64, finalScore float64) knowledgeapp.RetrievalHit {
	metadata := record.FilterMetadata
	return knowledgeapp.RetrievalHit{
		TenantID:          record.TenantID,
		KnowledgeBaseID:   record.KnowledgeBaseID,
		ChunkID:           record.ChunkID,
		DocumentID:        stringValue(metadata["documentId"]),
		DocumentVersionID: record.DocumentVersionID,
		DocumentName:      stringValue(metadata["documentName"]),
		Title:             stringValue(metadata["title"]),
		Content:           record.SearchText,
		VectorScore:       vectorScore,
		KeywordScore:      keywordScore,
		FinalScore:        finalScore,
		SourceLocator:     mapValue(metadata["sourceLocator"]),
		Metadata:          metadata,
	}
}

func cosine(left []float32, right []float32) float64 {
	if len(left) == 0 || len(left) != len(right) {
		return 0
	}
	dot, leftNorm, rightNorm := float64(0), float64(0), float64(0)
	for index := range left {
		dot += float64(left[index] * right[index])
		leftNorm += float64(left[index] * left[index])
		rightNorm += float64(right[index] * right[index])
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
}

func keywordSimilarity(query string, content string) float64 {
	queryTokens := tokenSet(query)
	if len(queryTokens) == 0 {
		return 0
	}
	contentTokens := tokenSet(content)
	matched := 0
	for token := range queryTokens {
		if contentTokens[token] {
			matched++
		}
	}
	return float64(matched) / float64(len(queryTokens))
}

func tokenSet(value string) map[string]bool {
	result := map[string]bool{}
	word := strings.Builder{}
	flush := func() {
		if word.Len() > 0 {
			result[word.String()] = true
			word.Reset()
		}
	}
	for _, current := range strings.ToLower(value) {
		if unicode.Is(unicode.Han, current) {
			flush()
			result[string(current)] = true
			continue
		}
		if unicode.IsLetter(current) || unicode.IsDigit(current) {
			word.WriteRune(current)
		} else {
			flush()
		}
	}
	flush()
	return result
}

func stringValue(value any) string {
	if typed, ok := value.(string); ok {
		return typed
	}
	return ""
}

func mapValue(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return map[string]any{}
}
