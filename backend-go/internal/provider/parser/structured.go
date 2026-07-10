package parser

import (
	"bytes"
	"context"
	"encoding/csv"
	"html"
	"regexp"
	"strings"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

type CSV struct{}

func (CSV) Code() string { return "csv" }
func (CSV) Supports(mimeType, fileName string) bool {
	return strings.EqualFold(extension(fileName), ".csv") || strings.Contains(strings.ToLower(mimeType), "csv")
}
func (CSV) Parse(_ context.Context, source knowledgeapp.SourceDocument) ([]knowledgeapp.DocumentUnit, error) {
	reader := csv.NewReader(bytes.NewReader(source.Content))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	units := make([]knowledgeapp.DocumentUnit, 0, len(records))
	for index, record := range records {
		units = append(units, knowledgeapp.DocumentUnit{UnitType: "row", UnitNo: index + 1, Content: strings.Join(record, "\t"), Locator: map[string]any{"row": index + 1}})
	}
	return units, nil
}

type HTML struct{}

var htmlBlockPattern = regexp.MustCompile(`(?is)<(?:script|style|noscript)[^>]*>.*?</(?:script|style|noscript)>`)
var htmlTagPattern = regexp.MustCompile(`(?s)<[^>]+>`)
var htmlHeadingPattern = regexp.MustCompile(`(?is)<h([1-6])[^>]*>(.*?)</h[1-6]>`)

func (HTML) Code() string { return "html" }
func (HTML) Supports(mimeType, fileName string) bool {
	ext := extension(fileName)
	return ext == ".html" || ext == ".htm" || strings.Contains(strings.ToLower(mimeType), "text/html")
}
func (HTML) Parse(_ context.Context, source knowledgeapp.SourceDocument) ([]knowledgeapp.DocumentUnit, error) {
	raw := string(source.Content)
	cleaned := htmlBlockPattern.ReplaceAllString(raw, " ")
	headings := htmlHeadingPattern.FindAllStringSubmatch(cleaned, -1)
	title := ""
	if len(headings) > 0 {
		title = cleanHTMLText(headings[0][2])
	}
	text := cleanHTMLText(cleaned)
	return []knowledgeapp.DocumentUnit{{UnitType: "section", UnitNo: 1, Title: title, Content: text, Locator: map[string]any{"source": "html"}, Metadata: map[string]any{"headingCount": len(headings)}}}, nil
}

func cleanHTMLText(value string) string {
	value = htmlTagPattern.ReplaceAllString(value, "\n")
	value = html.UnescapeString(value)
	lines := strings.Split(strings.ReplaceAll(value, "\r", ""), "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}
