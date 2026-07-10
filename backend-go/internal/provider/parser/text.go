package parser

import (
	"bytes"
	"context"
	"strings"
	"unicode/utf8"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

type PlainText struct{}

func (PlainText) Code() string { return "plain_text" }

func (PlainText) Supports(mimeType string, fileName string) bool {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	fileName = strings.ToLower(strings.TrimSpace(fileName))
	return strings.HasPrefix(mimeType, "text/plain") || strings.HasSuffix(fileName, ".txt")
}

func (PlainText) Parse(_ context.Context, source knowledgeapp.SourceDocument) ([]knowledgeapp.DocumentUnit, error) {
	raw := bytes.TrimPrefix(source.Content, []byte{0xef, 0xbb, 0xbf})
	content := strings.TrimSpace(strings.ToValidUTF8(string(raw), "�"))
	if !utf8.ValidString(content) {
		content = strings.ToValidUTF8(content, "�")
	}
	return []knowledgeapp.DocumentUnit{{
		UnitType: "SECTION",
		UnitNo:   1,
		Title:    strings.TrimSuffix(source.Name, ".txt"),
		Content:  content,
		Locator:  map[string]any{"type": "SECTION", "section": 1},
		Metadata: map[string]any{"parser": "plain_text"},
	}}, nil
}
