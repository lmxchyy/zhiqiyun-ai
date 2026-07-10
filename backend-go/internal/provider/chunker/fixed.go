package chunker

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

type Fixed struct{}

func (Fixed) Code() string { return "fixed" }

func (Fixed) Chunk(_ context.Context, units []knowledgeapp.DocumentUnit, options knowledgeapp.ChunkOptions) ([]knowledgeapp.Chunk, error) {
	if options.ChunkSize <= 0 {
		options.ChunkSize = 800
	}
	if options.Overlap < 0 {
		options.Overlap = 0
	}
	targetRunes := options.ChunkSize * 2
	overlapRunes := options.Overlap * 2
	if overlapRunes >= targetRunes {
		overlapRunes = targetRunes / 5
	}
	chunks := make([]knowledgeapp.Chunk, 0)
	for _, unit := range units {
		content := normalizeText(unit.Content)
		if content == "" {
			continue
		}
		runes := []rune(content)
		for start := 0; start < len(runes); {
			end := start + targetRunes
			if end > len(runes) {
				end = len(runes)
			} else {
				end = nearestBoundary(runes, start, end)
			}
			piece := strings.TrimSpace(string(runes[start:end]))
			if piece != "" {
				chunks = append(chunks, knowledgeapp.Chunk{
					SequenceNo:    len(chunks) + 1,
					Content:       piece,
					TokenCount:    estimateTokens(piece),
					Title:         unit.Title,
					TitlePath:     nonEmptyStrings(unit.Title),
					SourceLocator: cloneMap(unit.Locator),
					Metadata:      map[string]any{"unitType": unit.UnitType, "unitNo": unit.UnitNo},
					Status:        "ACTIVE",
				})
			}
			if end >= len(runes) {
				break
			}
			next := end - overlapRunes
			if next <= start {
				next = end
			}
			start = next
		}
	}
	return chunks, nil
}

func normalizeText(value string) string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	result := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if !blank && len(result) > 0 {
				result = append(result, "")
			}
			blank = true
			continue
		}
		blank = false
		result = append(result, line)
	}
	return strings.TrimSpace(strings.Join(result, "\n"))
}

func nearestBoundary(runes []rune, start int, target int) int {
	minimum := start + (target-start)*3/4
	for index := target; index > minimum; index-- {
		if strings.ContainsRune("。！？!?；;\n", runes[index-1]) {
			return index
		}
	}
	return target
}

func estimateTokens(value string) int {
	tokens := 0
	inWord := false
	for _, current := range value {
		if unicode.Is(unicode.Han, current) || unicode.Is(unicode.Hiragana, current) || unicode.Is(unicode.Katakana, current) {
			tokens++
			inWord = false
			continue
		}
		if unicode.IsLetter(current) || unicode.IsDigit(current) {
			if !inWord {
				tokens++
				inWord = true
			}
		} else {
			inWord = false
		}
	}
	if tokens == 0 && utf8.RuneCountInString(value) > 0 {
		return 1
	}
	return tokens
}

func cloneMap(source map[string]any) map[string]any {
	result := map[string]any{}
	for key, value := range source {
		result[key] = value
	}
	return result
}

func nonEmptyStrings(values ...string) []string {
	result := []string{}
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}
