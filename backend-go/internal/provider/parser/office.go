package parser

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

type DOCX struct{}
type XLSX struct{}
type PPTX struct{}
type LegacyOffice struct{}

func (DOCX) Code() string         { return "docx" }
func (XLSX) Code() string         { return "xlsx" }
func (PPTX) Code() string         { return "pptx" }
func (LegacyOffice) Code() string { return "legacy_office" }

func (DOCX) Supports(mimeType, fileName string) bool {
	return extension(fileName) == ".docx" || strings.Contains(strings.ToLower(mimeType), "wordprocessingml")
}
func (XLSX) Supports(mimeType, fileName string) bool {
	return extension(fileName) == ".xlsx" || strings.Contains(strings.ToLower(mimeType), "spreadsheetml")
}
func (PPTX) Supports(mimeType, fileName string) bool {
	return extension(fileName) == ".pptx" || strings.Contains(strings.ToLower(mimeType), "presentationml")
}
func (LegacyOffice) Supports(_ string, fileName string) bool {
	ext := extension(fileName)
	return ext == ".doc" || ext == ".xls" || ext == ".ppt"
}

func (DOCX) Parse(_ context.Context, source knowledgeapp.SourceDocument) ([]knowledgeapp.DocumentUnit, error) {
	files, err := zipFiles(source.Content)
	if err != nil {
		return nil, err
	}
	raw := files["word/document.xml"]
	if len(raw) == 0 {
		return nil, fmt.Errorf("docx document.xml is missing")
	}
	paragraphs := xmlParagraphs(raw, "p", "t")
	units := make([]knowledgeapp.DocumentUnit, 0, len(paragraphs))
	for index, text := range paragraphs {
		if strings.TrimSpace(text) != "" {
			units = append(units, knowledgeapp.DocumentUnit{UnitType: "paragraph", UnitNo: index + 1, Content: text, Locator: map[string]any{"paragraph": index + 1}})
		}
	}
	return units, nil
}

func (XLSX) Parse(_ context.Context, source knowledgeapp.SourceDocument) ([]knowledgeapp.DocumentUnit, error) {
	files, err := zipFiles(source.Content)
	if err != nil {
		return nil, err
	}
	shared := xmlTexts(files["xl/sharedStrings.xml"], "t")
	names := archiveNames(files, "xl/worksheets/sheet", ".xml")
	units := make([]knowledgeapp.DocumentUnit, 0)
	for sheetIndex, name := range names {
		rows := xlsxRows(files[name], shared)
		for rowIndex, row := range rows {
			units = append(units, knowledgeapp.DocumentUnit{UnitType: "row", UnitNo: len(units) + 1, Title: fmt.Sprintf("Sheet %d", sheetIndex+1), Content: strings.Join(row, "\t"), Locator: map[string]any{"sheet": sheetIndex + 1, "row": rowIndex + 1}})
		}
	}
	return units, nil
}

func (PPTX) Parse(_ context.Context, source knowledgeapp.SourceDocument) ([]knowledgeapp.DocumentUnit, error) {
	files, err := zipFiles(source.Content)
	if err != nil {
		return nil, err
	}
	names := archiveNames(files, "ppt/slides/slide", ".xml")
	units := make([]knowledgeapp.DocumentUnit, 0, len(names))
	for index, name := range names {
		texts := xmlTexts(files[name], "t")
		content := strings.Join(nonEmpty(texts), "\n")
		title := ""
		if len(texts) > 0 {
			title = strings.TrimSpace(texts[0])
		}
		if content != "" {
			units = append(units, knowledgeapp.DocumentUnit{UnitType: "slide", UnitNo: index + 1, Title: title, Content: content, Locator: map[string]any{"page": index + 1, "slide": index + 1}})
		}
	}
	return units, nil
}

func (LegacyOffice) Parse(_ context.Context, source knowledgeapp.SourceDocument) ([]knowledgeapp.DocumentUnit, error) {
	text := extractLegacyText(source.Content)
	if len([]rune(text)) < 12 {
		return nil, fmt.Errorf("legacy Office file contains no extractable text; configure an external Office converter")
	}
	return []knowledgeapp.DocumentUnit{{UnitType: "document", UnitNo: 1, Content: text, Locator: map[string]any{"source": "legacy-office"}, Metadata: map[string]any{"bestEffort": true}}}, nil
}

func zipFiles(content []byte) (map[string][]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, err
	}
	files := make(map[string][]byte, len(reader.File))
	for _, file := range reader.File {
		stream, err := file.Open()
		if err != nil {
			return nil, err
		}
		raw, readErr := io.ReadAll(io.LimitReader(stream, 64<<20))
		_ = stream.Close()
		if readErr != nil {
			return nil, readErr
		}
		files[file.Name] = raw
	}
	return files, nil
}

func xmlTexts(raw []byte, localName string) []string {
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	result := []string{}
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != localName {
			continue
		}
		var text string
		if decoder.DecodeElement(&text, &start) == nil {
			result = append(result, text)
		}
	}
	return result
}

func xmlParagraphs(raw []byte, paragraphName, textName string) []string {
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	result, current := []string{}, strings.Builder{}
	inParagraph := false
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch typed := token.(type) {
		case xml.StartElement:
			if typed.Name.Local == paragraphName {
				inParagraph = true
				current.Reset()
			} else if inParagraph && typed.Name.Local == textName {
				var text string
				if decoder.DecodeElement(&text, &typed) == nil {
					current.WriteString(text)
				}
			}
		case xml.EndElement:
			if typed.Name.Local == paragraphName && inParagraph {
				result = append(result, strings.TrimSpace(current.String()))
				inParagraph = false
			}
		}
	}
	return result
}

func xlsxRows(raw []byte, shared []string) [][]string {
	type cell struct {
		T      string `xml:"t,attr"`
		V      string `xml:"v"`
		Inline string `xml:"is>t"`
	}
	type row struct {
		Cells []cell `xml:"c"`
	}
	var sheet struct {
		Rows []row `xml:"sheetData>row"`
	}
	if xml.Unmarshal(raw, &sheet) != nil {
		return nil
	}
	result := make([][]string, 0, len(sheet.Rows))
	for _, current := range sheet.Rows {
		values := make([]string, 0, len(current.Cells))
		for _, item := range current.Cells {
			value := item.V
			if item.Inline != "" {
				value = item.Inline
			} else if item.T == "s" {
				index, _ := strconv.Atoi(item.V)
				if index >= 0 && index < len(shared) {
					value = shared[index]
				}
			}
			values = append(values, value)
		}
		result = append(result, values)
	}
	return result
}

func archiveNames(files map[string][]byte, prefix, suffix string) []string {
	names := []string{}
	for name := range files {
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, suffix) {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(i, j int) bool { return naturalArchiveOrder(names[i]) < naturalArchiveOrder(names[j]) })
	return names
}

func naturalArchiveOrder(value string) int {
	digits := strings.Builder{}
	for _, current := range value {
		if unicode.IsDigit(current) {
			digits.WriteRune(current)
		}
	}
	number, _ := strconv.Atoi(digits.String())
	return number
}

func extractLegacyText(raw []byte) string {
	parts := []string{}
	for _, candidate := range []string{printableASCII(raw), printableUTF16(raw)} {
		for _, line := range strings.Split(candidate, "\n") {
			line = strings.Join(strings.Fields(line), " ")
			if len([]rune(line)) >= 4 {
				parts = append(parts, line)
			}
		}
	}
	return strings.Join(nonEmpty(parts), "\n")
}

func printableASCII(raw []byte) string {
	result := strings.Builder{}
	for _, current := range raw {
		if current == '\n' || current == '\r' || current == '\t' || (current >= 32 && current < 127) {
			result.WriteByte(current)
		} else {
			result.WriteByte('\n')
		}
	}
	return result.String()
}

func printableUTF16(raw []byte) string {
	values := make([]uint16, 0, len(raw)/2)
	for index := 0; index+1 < len(raw); index += 2 {
		value := uint16(raw[index]) | uint16(raw[index+1])<<8
		if value == 9 || value == 10 || value == 13 || (value >= 32 && value != 0xffff) {
			values = append(values, value)
		} else {
			values = append(values, 10)
		}
	}
	return string(utf16.Decode(values))
}

func nonEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, strings.TrimSpace(value))
		}
	}
	return result
}

func extension(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if index := strings.LastIndex(name, "."); index >= 0 {
		return name[index:]
	}
	return ""
}
