package parser

import (
	"bytes"
	"context"
	"regexp"
	"strings"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

var markdownHeading = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*$`)

type Markdown struct{}

func (Markdown) Code() string { return "markdown" }

func (Markdown) Supports(mimeType string, fileName string) bool {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	fileName = strings.ToLower(strings.TrimSpace(fileName))
	return strings.Contains(mimeType, "markdown") || strings.HasSuffix(fileName, ".md") || strings.HasSuffix(fileName, ".markdown")
}

func (Markdown) Parse(_ context.Context, source knowledgeapp.SourceDocument) ([]knowledgeapp.DocumentUnit, error) {
	raw := bytes.TrimPrefix(source.Content, []byte{0xef, 0xbb, 0xbf})
	lines := strings.Split(strings.ToValidUTF8(string(raw), "�"), "\n")
	units := make([]knowledgeapp.DocumentUnit, 0)
	title := strings.TrimSuffix(strings.TrimSuffix(source.Name, ".markdown"), ".md")
	level := 0
	buffer := make([]string, 0)
	flush := func() {
		content := strings.TrimSpace(strings.Join(buffer, "\n"))
		if content == "" {
			buffer = buffer[:0]
			return
		}
		unitNo := len(units) + 1
		units = append(units, knowledgeapp.DocumentUnit{
			UnitType: "SECTION",
			UnitNo:   unitNo,
			Title:    title,
			Content:  content,
			Locator:  map[string]any{"type": "SECTION", "section": unitNo, "heading": title},
			Metadata: map[string]any{"parser": "markdown", "headingLevel": level},
		})
		buffer = buffer[:0]
	}
	for _, line := range lines {
		if match := markdownHeading.FindStringSubmatch(strings.TrimSpace(line)); len(match) == 3 {
			flush()
			level = len(match[1])
			title = strings.TrimSpace(match[2])
			continue
		}
		buffer = append(buffer, strings.TrimRight(line, "\r"))
	}
	flush()
	if len(units) == 0 {
		units = append(units, knowledgeapp.DocumentUnit{UnitType: "SECTION", UnitNo: 1, Title: title, Content: "", Locator: map[string]any{"type": "SECTION", "section": 1}})
	}
	return units, nil
}
