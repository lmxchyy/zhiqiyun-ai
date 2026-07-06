package httpserver

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
)

func TestBuildPPTXUsesValidPowerPointThemeAndSlideSize(t *testing.T) {
	payload, err := buildPPTX(pptapp.Task{
		TaskID:     "ppt_test",
		UserID:     "user_test",
		Status:     pptapp.StatusSuccess,
		Title:      "PPTX validation",
		Prompt:     "PPTX validation",
		SlideCount: 1,
		Theme:      "business",
		Slides: []pptapp.Slide{{
			ID:           "slide_1",
			Page:         1,
			Title:        "PPTX validation",
			Content:      "Validate direct PowerPoint open.",
			BulletPoints: []string{"screen16x9", "complete theme lists"},
			Layout:       "cover",
		}},
	})
	if err != nil {
		t.Fatalf("buildPPTX() error = %v", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatalf("zip.NewReader() error = %v", err)
	}
	presentation := readZipEntry(t, reader, "ppt/presentation.xml")
	if strings.Contains(presentation, `type="wide"`) {
		t.Fatal(`presentation.xml contains invalid slide size type "wide"`)
	}
	if !strings.Contains(presentation, `type="screen16x9"`) {
		t.Fatal(`presentation.xml missing valid slide size type "screen16x9"`)
	}
	theme := readZipEntry(t, reader, "ppt/theme/theme1.xml")
	if strings.Count(theme, "<a:ln ") < 3 {
		t.Fatalf("theme line styles = %d, want at least 3", strings.Count(theme, "<a:ln "))
	}
	if strings.Count(theme, "<a:effectStyle>") < 3 {
		t.Fatalf("theme effect styles = %d, want at least 3", strings.Count(theme, "<a:effectStyle>"))
	}
	if strings.Count(theme, "<a:bgFillStyleLst>") != 1 {
		t.Fatalf("theme bgFillStyleLst count = %d, want 1", strings.Count(theme, "<a:bgFillStyleLst>"))
	}
}

func readZipEntry(t *testing.T, reader *zip.Reader, name string) string {
	t.Helper()
	for _, entry := range reader.File {
		if entry.Name != name {
			continue
		}
		rc, err := entry.Open()
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		defer rc.Close()
		raw, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return string(raw)
	}
	t.Fatalf("missing zip entry %s", name)
	return ""
}
