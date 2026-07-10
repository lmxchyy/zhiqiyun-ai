package parser

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

type PDF struct{ OCR knowledgeapp.OCRProvider }

var pdfTextOperator = regexp.MustCompile(`(?s)\((.*?[^\\])\)\s*Tj`)
var pdfArrayOperator = regexp.MustCompile(`(?s)\[(.*?)\]\s*TJ`)
var pdfArrayText = regexp.MustCompile(`\((.*?[^\\])\)`)

func (PDF) Code() string { return "pdf" }
func (PDF) Supports(mimeType, fileName string) bool {
	return extension(fileName) == ".pdf" || strings.Contains(strings.ToLower(mimeType), "application/pdf")
}
func (p PDF) Parse(ctx context.Context, source knowledgeapp.SourceDocument) ([]knowledgeapp.DocumentUnit, error) {
	raw := string(source.Content)
	parts := []string{}
	for _, match := range pdfTextOperator.FindAllStringSubmatch(raw, -1) {
		parts = append(parts, decodePDFString(match[1]))
	}
	for _, array := range pdfArrayOperator.FindAllStringSubmatch(raw, -1) {
		line := strings.Builder{}
		for _, match := range pdfArrayText.FindAllStringSubmatch(array[1], -1) {
			line.WriteString(decodePDFString(match[1]))
		}
		parts = append(parts, line.String())
	}
	text := strings.Join(nonEmpty(parts), "\n")
	if len([]rune(text)) < 12 && p.OCR != nil {
		return p.OCR.Recognize(ctx, source)
	}
	if strings.TrimSpace(text) == "" {
		return nil, strconv.ErrSyntax
	}
	return []knowledgeapp.DocumentUnit{{UnitType: "page", UnitNo: 1, Content: text, Locator: map[string]any{"page": 1}, Metadata: map[string]any{"ocr": false, "bestEffort": true}}}, nil
}

func decodePDFString(value string) string {
	replacer := strings.NewReplacer(`\n`, "\n", `\r`, "\n", `\t`, "\t", `\(`, "(", `\)`, ")", `\\`, `\`)
	return strings.TrimSpace(replacer.Replace(value))
}
