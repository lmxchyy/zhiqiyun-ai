package parser

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

func TestDefaultRegistryParsesStructuredFormats(t *testing.T) {
	t.Parallel()
	registry := NewDefaultRegistry()
	cases := []struct {
		name, mime string
		content    []byte
		want       string
	}{
		{name: "data.csv", mime: "text/csv", content: []byte("name,value\n企业版,100\n"), want: "企业版"},
		{name: "page.html", mime: "text/html", content: []byte("<h1>产品说明</h1><p>企业版支持知识库</p>"), want: "企业版支持知识库"},
		{name: "manual.docx", mime: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", content: officeArchive(t, map[string]string{"word/document.xml": `<w:document xmlns:w="w"><w:body><w:p><w:r><w:t>企业版手册</w:t></w:r></w:p></w:body></w:document>`}), want: "企业版手册"},
		{name: "data.xlsx", mime: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", content: officeArchive(t, map[string]string{"xl/sharedStrings.xml": `<sst><si><t>成员配额</t></si></sst>`, "xl/worksheets/sheet1.xml": `<worksheet><sheetData><row><c t="s"><v>0</v></c><c><v>100</v></c></row></sheetData></worksheet>`}), want: "成员配额"},
		{name: "slides.pptx", mime: "application/vnd.openxmlformats-officedocument.presentationml.presentation", content: officeArchive(t, map[string]string{"ppt/slides/slide1.xml": `<p:sld xmlns:p="p" xmlns:a="a"><a:t>知识库架构</a:t><a:t>Hybrid Search</a:t></p:sld>`}), want: "Hybrid Search"},
		{name: "simple.pdf", mime: "application/pdf", content: []byte("%PDF-1.4\nBT (企业知识库支持引用来源) Tj ET"), want: "引用来源"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			units, code, err := registry.Parse(context.Background(), knowledgeapp.SourceDocument{Name: test.name, MIMEType: test.mime, Content: test.content})
			if err != nil {
				t.Fatal(err)
			}
			all := strings.Builder{}
			for _, unit := range units {
				all.WriteString(unit.Content)
			}
			if !strings.Contains(all.String(), test.want) {
				t.Fatalf("parser %s content %q does not contain %q", code, all.String(), test.want)
			}
		})
	}
}

func officeArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	buffer := bytes.NewBuffer(nil)
	archive := zip.NewWriter(buffer)
	for name, content := range files {
		entry, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
