package ppt

import (
	"reflect"
	"testing"
)

func TestNormalizeSlideIRKeepsBlocksAsOnlyCanonicalContent(t *testing.T) {
	slide := Slide{Blocks: []SlideBlock{
		{Type: "title", Text: "IR title"},
		{Type: "paragraph", Text: "IR paragraph"},
		{Type: "bullets", Items: []string{"One", "Two"}},
		{Type: "image", ImageRef: "asset://slide-image"},
		{Type: "note", Text: "IR note"},
	}}

	got := NormalizeSlideIR(slide)
	if got.Title != "" || got.Content != "" || got.ImageURL != "" || got.SpeakerNotes != "" || len(got.BulletPoints) != 0 {
		t.Fatalf("NormalizeSlideIR() dual-wrote legacy fields: %#v", got)
	}
	if !reflect.DeepEqual(got.Blocks, slide.Blocks) {
		t.Fatalf("NormalizeSlideIR() blocks = %#v, want %#v", got.Blocks, slide.Blocks)
	}
}

func TestCloneTaskDeepCopiesBlocksMessagesAndRecords(t *testing.T) {
	original := Task{
		Slides: []Slide{{
			Blocks: []SlideBlock{{Type: "bullets", Items: []string{"original item"}}},
		}},
		AgentMessages: []AgentMessage{{Role: "user", Content: "original message"}},
		IdempotencyRecords: []IdempotencyRecord{{
			Scope: "message", Key: "original key", ResponseJSON: `{"ok":true}`,
		}},
	}

	cloned := cloneTask(original)
	cloned.Slides[0].Blocks[0].Items[0] = "changed item"
	cloned.AgentMessages[0].Content = "changed message"
	cloned.IdempotencyRecords[0].Key = "changed key"

	if got := original.Slides[0].Blocks[0].Items[0]; got != "original item" {
		t.Fatalf("original block item mutated through clone: %q", got)
	}
	if got := original.AgentMessages[0].Content; got != "original message" {
		t.Fatalf("original message mutated through clone: %q", got)
	}
	if got := original.IdempotencyRecords[0].Key; got != "original key" {
		t.Fatalf("original idempotency record mutated through clone: %q", got)
	}
}

func TestSlideFromOutlineBuildsNormalizedIR(t *testing.T) {
	outline := OutlineSlide{
		Page: 2, Title: "Page title", Summary: "Page summary",
		BulletPoints: []string{"First point", "Second point"}, Layout: "content", SlideType: "text_image",
	}
	req := GenerateRequest{SlideCount: 3, Theme: "business", ImageSource: "none"}

	got := SlideFromOutline(outline, req)
	if got.ID != "slide_2" || got.Page != 2 || got.Layout != "content" || got.SlideType != "text_image" {
		t.Fatalf("SlideFromOutline() metadata = %#v", got)
	}
	wantBlocks := []SlideBlock{
		{Type: "title", Text: "Page title"},
		{Type: "paragraph", Text: "Page summary"},
		{Type: "bullets", Items: []string{"First point", "Second point"}},
		{Type: "note", Text: "Page 2 speaker notes can be refined after deck review."},
	}
	if !reflect.DeepEqual(got.Blocks, wantBlocks) {
		t.Fatalf("SlideFromOutline() blocks = %#v, want %#v", got.Blocks, wantBlocks)
	}
}

func TestSlideFromOutlineDefaultsMissingPageBeforeImageConstruction(t *testing.T) {
	got := SlideFromOutline(OutlineSlide{Title: "Cover", SlideType: "cover"}, GenerateRequest{
		SlideCount: 1, ImageSource: "ai",
	})
	if got.Page != 1 || got.ID != "slide_1" || !hasSlideImageBlock(got) {
		t.Fatalf("SlideFromOutline() missing-page normalization = %#v", got)
	}
}

func hasSlideImageBlock(slide Slide) bool {
	for _, block := range slide.Blocks {
		if block.Type == "image" && block.ImageRef != "" {
			return true
		}
	}
	return false
}

func TestSetSlideImageRefRemovesLegacyAndIRImage(t *testing.T) {
	slide := Slide{
		ImageURL: "https://example.test/old.png",
		Blocks:   []SlideBlock{{Type: "image", ImageRef: "https://example.test/old.png"}},
	}

	got := setSlideImageRef(slide, "")
	if got.ImageURL != "" {
		t.Fatalf("setSlideImageRef() retained legacy imageUrl %q", got.ImageURL)
	}
	for _, block := range got.Blocks {
		if block.Type == "image" {
			t.Fatalf("setSlideImageRef() retained image block %#v", block)
		}
	}
}
