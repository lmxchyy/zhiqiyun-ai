package parser

import (
	"context"
	"fmt"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

type Registry struct {
	items []knowledgeapp.Parser
}

func NewRegistry(items ...knowledgeapp.Parser) Registry {
	filtered := make([]knowledgeapp.Parser, 0, len(items))
	for _, item := range items {
		if item != nil {
			filtered = append(filtered, item)
		}
	}
	return Registry{items: filtered}
}

func NewDefaultRegistry() Registry {
	return NewDefaultRegistryWithOCR(nil)
}

func NewDefaultRegistryWithOCR(ocr knowledgeapp.OCRProvider) Registry {
	return NewRegistry(Markdown{}, CSV{}, HTML{}, DOCX{}, XLSX{}, PPTX{}, PDF{OCR: ocr}, LegacyOffice{}, PlainText{})
}

func (r Registry) Parse(ctx context.Context, source knowledgeapp.SourceDocument) ([]knowledgeapp.DocumentUnit, string, error) {
	for _, item := range r.items {
		if item.Supports(source.MIMEType, source.Name) {
			units, err := item.Parse(ctx, source)
			return units, item.Code(), err
		}
	}
	return nil, "", fmt.Errorf("no parser supports %s (%s)", source.Name, source.MIMEType)
}
