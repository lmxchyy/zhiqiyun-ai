package cleaner

import (
	"context"
	"regexp"
	"strings"
	"unicode/utf8"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

type Standard struct{}

var tocLine = regexp.MustCompile(`(?i)^.{1,100}(\.{3,}|…{2,})\s*\d+\s*$`)

func (Standard) Code() string { return "standard" }

func (Standard) Normalize(_ context.Context, units []knowledgeapp.DocumentUnit) ([]knowledgeapp.DocumentUnit, map[string]any, error) {
	lineFrequency := map[string]int{}
	for _, unit := range units {
		lines := meaningfulLines(unit.Content)
		if len(lines) > 0 {
			lineFrequency[lines[0]]++
			if len(lines) > 1 {
				lineFrequency[lines[len(lines)-1]]++
			}
		}
	}
	repeated := map[string]bool{}
	threshold := len(units) * 3 / 5
	if threshold < 3 {
		threshold = 3
	}
	for line, count := range lineFrequency {
		if count >= threshold && len([]rune(line)) <= 120 {
			repeated[line] = true
		}
	}
	seen := map[string]bool{}
	result := make([]knowledgeapp.DocumentUnit, 0, len(units))
	removedRepeated, removedDuplicate, removedTOC, repairedUTF8 := 0, 0, 0, 0
	for _, unit := range units {
		content := unit.Content
		if !utf8.ValidString(content) {
			content = strings.ToValidUTF8(content, "")
			repairedUTF8++
		}
		lines := meaningfulLines(content)
		cleaned := make([]string, 0, len(lines))
		tocLines := 0
		for _, line := range lines {
			if repeated[line] {
				removedRepeated++
				continue
			}
			if tocLine.MatchString(line) {
				tocLines++
				continue
			}
			cleaned = append(cleaned, line)
		}
		if tocLines > 2 && tocLines >= len(lines)/2 {
			removedTOC++
			continue
		}
		content = strings.TrimSpace(strings.Join(cleaned, "\n"))
		if content == "" {
			continue
		}
		key := strings.ToLower(content)
		if seen[key] {
			removedDuplicate++
			continue
		}
		seen[key] = true
		unit.Content = content
		if strings.TrimSpace(unit.Title) == "" {
			unit.Title = inferTitle(cleaned)
		}
		if unit.Metadata == nil {
			unit.Metadata = map[string]any{}
		}
		unit.Metadata["normalized"] = true
		result = append(result, unit)
	}
	for index := range result {
		result[index].UnitNo = index + 1
	}
	return result, map[string]any{
		"code": "standard", "removedRepeatedLines": removedRepeated, "removedDuplicateUnits": removedDuplicate,
		"removedTOCUnits": removedTOC, "repairedUTF8Units": repairedUTF8,
	}, nil
}

func meaningfulLines(value string) []string {
	value = strings.ReplaceAll(value, "\u0000", "")
	value = strings.ReplaceAll(value, "\r", "\n")
	lines := strings.Split(value, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(strings.ReplaceAll(line, "�", "")), " ")
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func inferTitle(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	first := strings.TrimSpace(strings.TrimLeft(lines[0], "#"))
	if len([]rune(first)) <= 100 {
		return first
	}
	return ""
}

var _ knowledgeapp.DocumentNormalizer = Standard{}
