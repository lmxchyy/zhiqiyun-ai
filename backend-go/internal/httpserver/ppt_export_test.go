package httpserver

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
)

func TestBuildPPTXRendersBlocksFirstWithValidRelationships(t *testing.T) {
	payload, err := buildPPTX(pptapp.Task{
		TaskID: "ppt_ir_export", Title: "Deck & 总览", Prompt: "Fallback prompt", Theme: "business",
		Slides: []pptapp.Slide{
			{
				ID: "slide_blocks_1", Page: 1, Title: "STALE_LEGACY_TITLE", Content: "STALE_LEGACY_BODY",
				BulletPoints: []string{"STALE_LEGACY_BULLET"}, SpeakerNotes: "STALE_LEGACY_NOTE", Layout: "content",
				Blocks: []pptapp.SlideBlock{
					{Type: "title", Text: "区块标题 & <可信>"},
					{Type: "subtitle", Text: "区块副标题"},
					{Type: "paragraph", Text: "区块正文 > 旧字段"},
					{Type: "bullets", Items: []string{"区块要点一", "区块要点 & 二"}},
					{Type: "note", Text: "BLOCK_NOTE_MUST_NOT_RENDER"},
				},
			},
			{
				ID: "slide_blocks_2", Page: 2, Layout: "imageText",
				Blocks: []pptapp.SlideBlock{
					{Type: "title", Text: "第二页"},
					{Type: "paragraph", Text: "仍然只读取 Blocks"},
					{Type: "image", ImageRef: "mock://missing-image"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("buildPPTX() error = %v", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatalf("zip.NewReader() error = %v", err)
	}
	for _, required := range []string{
		"[Content_Types].xml", "_rels/.rels", "ppt/presentation.xml", "ppt/_rels/presentation.xml.rels",
		"ppt/slides/slide1.xml", "ppt/slides/_rels/slide1.xml.rels", "ppt/slides/slide2.xml", "ppt/slides/_rels/slide2.xml.rels",
		"ppt/slideMasters/slideMaster1.xml", "ppt/slideLayouts/slideLayout1.xml", "ppt/theme/theme1.xml",
	} {
		_ = readZipEntry(t, reader, required)
	}
	presentation := readZipEntry(t, reader, "ppt/presentation.xml")
	if !strings.Contains(presentation, `cx="12192000" cy="6858000" type="screen16x9"`) || strings.Count(presentation, "<p:sldId ") != 2 {
		t.Fatalf("presentation size/slide count invalid: %s", presentation)
	}
	slide := readZipEntry(t, reader, "ppt/slides/slide1.xml")
	for _, visible := range []string{"区块标题", "可信", "区块副标题", "区块正文", "区块要点一", "区块要点 &amp; 二"} {
		if !strings.Contains(slide, visible) {
			t.Fatalf("block slide missing %q: %s", visible, slide)
		}
	}
	for _, stale := range []string{"STALE_LEGACY_TITLE", "STALE_LEGACY_BODY", "STALE_LEGACY_BULLET", "STALE_LEGACY_NOTE", "BLOCK_NOTE_MUST_NOT_RENDER"} {
		if strings.Contains(slide, stale) {
			t.Fatalf("block slide rendered stale/note field %q: %s", stale, slide)
		}
	}
	for _, entry := range reader.File {
		if strings.HasPrefix(entry.Name, "ppt/media/") {
			t.Fatalf("untrusted block image was embedded: %s", entry.Name)
		}
		if strings.HasSuffix(entry.Name, ".xml") || strings.HasSuffix(entry.Name, ".rels") {
			assertPPTXXMLEntryValid(t, reader, entry.Name)
		}
	}
}

func TestBuildPPTXUsesValidPowerPointThemeAndSlideSize(t *testing.T) {
	payload, err := buildPPTX(pptapp.Task{
		TaskID: "ppt_test", UserID: "user_test", Status: pptapp.StatusSuccess,
		Title: "PPTX validation", Prompt: "PPTX validation", SlideCount: 1, Theme: "business",
		Slides: []pptapp.Slide{{
			ID: "slide_1", Page: 1, Layout: "cover",
			Blocks: []pptapp.SlideBlock{
				{Type: "title", Text: "PPTX validation"},
				{Type: "paragraph", Text: "Validate direct PowerPoint open."},
				{Type: "bullets", Items: []string{"screen16x9", "complete theme lists"}},
			},
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

func TestBuildPPTXSkipsUntrustedImageReferencesWithoutNetwork(t *testing.T) {
	var requests atomic.Int32
	var resolverCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("untrusted references must never be fetched"))
	}))
	defer server.Close()

	for _, imageRef := range []string{
		server.URL + "/image.png",
		"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAAB",
		"file:///etc/passwd",
		"C:\\Windows\\win.ini",
		"storage://tenant_other/file_other",
		"STORAGE://tenant_a/file_a",
		"storage://tenant_a/file_a?token=secret",
		"storage://tenant_a/file_a#fragment",
		"storage://user@tenant_a/file_a",
		"storage://tenant_a/path/file_a",
	} {
		t.Run(imageRef, func(t *testing.T) {
			payload, err := buildPPTXWithImageResolver(context.Background(), pptapp.Task{
				TenantID: "tenant_a", Title: "Safe export",
				Slides: []pptapp.Slide{{ID: "slide_1", Page: 1, Layout: "imageText", Blocks: []pptapp.SlideBlock{
					{Type: "title", Text: "Visible title"},
					{Type: "paragraph", Text: "Visible body"},
					{Type: "image", ImageRef: imageRef},
				}}},
			}, func(context.Context, string, string) (string, []byte, bool) {
				resolverCalls.Add(1)
				return "image/png", []byte("must not be called"), true
			})
			if err != nil {
				t.Fatal(err)
			}
			reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range reader.File {
				if strings.HasPrefix(entry.Name, "ppt/media/") {
					t.Fatalf("untrusted reference %q embedded media %s", imageRef, entry.Name)
				}
			}
			slide := readZipEntry(t, reader, "ppt/slides/slide1.xml")
			if !strings.Contains(slide, "Visible title") || !strings.Contains(slide, "Visible body") {
				t.Fatalf("skipping image removed slide text: %s", slide)
			}
		})
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("untrusted image references caused %d network requests, want 0", got)
	}
	if got := resolverCalls.Load(); got != 0 {
		t.Fatalf("untrusted image references reached authorized resolver %d times, want 0", got)
	}
}

func TestBuildPPTXEmbedsImageOnlyThroughAuthorizedCanonicalStorageResolver(t *testing.T) {
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	payload, err := buildPPTXWithImageResolver(context.Background(), pptapp.Task{
		TenantID: "tenant_a", Title: "Storage export",
		Slides: []pptapp.Slide{{ID: "slide_1", Page: 1, Layout: "imageText", Blocks: []pptapp.SlideBlock{
			{Type: "title", Text: "Authorized visual"},
			{Type: "paragraph", Text: "Body"},
			{Type: "image", ImageRef: "storage://tenant_a/file_a"},
		}}},
	}, func(_ context.Context, tenantID, fileID string) (string, []byte, bool) {
		calls.Add(1)
		if tenantID != "tenant_a" || fileID != "file_a" {
			t.Fatalf("resolver identity = %q/%q, want tenant_a/file_a", tenantID, fileID)
		}
		return "image/png", png, true
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("authorized resolver calls = %d, want 1", calls.Load())
	}
	reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	mediaFound := false
	for _, entry := range reader.File {
		if entry.Name == "ppt/media/image1.png" {
			mediaFound = true
		}
	}
	if !mediaFound {
		t.Fatal("authorized canonical storage image was not embedded")
	}
	rels := readZipEntry(t, reader, "ppt/slides/_rels/slide1.xml.rels")
	if !strings.Contains(rels, "../media/image1.png") {
		t.Fatalf("authorized image relationship missing: %s", rels)
	}
}

func TestParsePPTStorageReferenceAcceptsOnlyCanonicalReferences(t *testing.T) {
	tests := []struct {
		name  string
		value string
		ok    bool
	}{
		{name: "canonical", value: "storage://tenant_a/file_a", ok: true},
		{name: "surrounding whitespace", value: "  storage://tenant_a/file_a  "},
		{name: "uppercase scheme", value: "STORAGE://tenant_a/file_a"},
		{name: "query", value: "storage://tenant_a/file_a?token=secret"},
		{name: "fragment", value: "storage://tenant_a/file_a#fragment"},
		{name: "userinfo", value: "storage://user@tenant_a/file_a"},
		{name: "nested file path", value: "storage://tenant_a/path/file_a"},
		{name: "noncanonical encoding", value: "storage://tenant_a/%66ile_a"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tenantID, fileID, ok := parsePPTStorageReference(test.value)
			if ok != test.ok {
				t.Fatalf("parsePPTStorageReference(%q) ok = %v, want %v (%q/%q)", test.value, ok, test.ok, tenantID, fileID)
			}
		})
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

func assertPPTXXMLEntryValid(t *testing.T, reader *zip.Reader, name string) {
	t.Helper()
	decoder := xml.NewDecoder(strings.NewReader(readZipEntry(t, reader, name)))
	for {
		if _, err := decoder.Token(); err != nil {
			if err == io.EOF {
				return
			}
			t.Fatalf("invalid XML %s: %v", name, err)
		}
	}
}
