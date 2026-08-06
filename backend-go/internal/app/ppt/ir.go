package ppt

import (
	"fmt"
	"strings"
)

type SlideBlock struct {
	Type     string   `json:"type"`
	Text     string   `json:"text,omitempty"`
	Items    []string `json:"items,omitempty"`
	ImageRef string   `json:"imageRef,omitempty"`
}

func NormalizeSlideIR(slide Slide) Slide {
	normalized := slide
	normalized.Title = ""
	normalized.Content = ""
	normalized.BulletPoints = nil
	normalized.ImageURL = ""
	normalized.SpeakerNotes = ""
	normalized.Blocks = normalizeSlideBlocks(slide.Blocks)
	return normalized
}

func SlideFromOutline(item OutlineSlide, req GenerateRequest) Slide {
	page := item.Page
	if page <= 0 {
		page = 1
	}
	item.Page = page
	total := req.SlideCount
	if total < page {
		total = page
	}
	input := VisualPlannerInput{
		DeckTheme: req.Theme, SlideType: item.SlideType, SlideTitle: item.Title,
		CoreIdea: item.Summary, ContentSummary: item.Summary, Layout: item.Layout,
		ImagePosition: req.ImageComposition, ImageStyle: req.ImageStyle,
		PeopleStyle: req.PeopleStyle, ImageLighting: req.ImageLighting,
		ImageComposition: req.ImageComposition,
	}
	plan := NormalizeVisualPlan(VisualPlan{}, input)
	blocks := []SlideBlock{
		{Type: "title", Text: strings.TrimSpace(item.Title)},
		{Type: "paragraph", Text: strings.TrimSpace(item.Summary)},
		{Type: "bullets", Items: append([]string(nil), item.BulletPoints...)},
	}
	if imageRef := slideImageURL(item, "", req); strings.TrimSpace(imageRef) != "" {
		blocks = append(blocks, SlideBlock{Type: "image", ImageRef: imageRef})
	}
	blocks = append(blocks, SlideBlock{Type: "note", Text: fmt.Sprintf("Page %d speaker notes can be refined after deck review.", page)})
	return NormalizeSlideIR(Slide{
		ID:         fmt.Sprintf("slide_%d", page),
		Page:       page,
		Blocks:     blocks,
		Layout:     normalizeSlideLayout(item.Layout, page-1, total, req.ImageSource),
		SlideType:  NormalizeSlideType(item.SlideType),
		VisualPlan: &plan,
	})
}

func normalizeSlideBlocks(blocks []SlideBlock) []SlideBlock {
	normalized := make([]SlideBlock, 0, len(blocks))
	for _, block := range blocks {
		block.Type = strings.ToLower(strings.TrimSpace(block.Type))
		block.Text = strings.TrimSpace(block.Text)
		block.ImageRef = strings.TrimSpace(block.ImageRef)
		block.Items = normalizeBlockItems(block.Items)
		switch block.Type {
		case "title", "subtitle", "paragraph", "note":
			if block.Text == "" {
				continue
			}
			block.Items = nil
			block.ImageRef = ""
		case "bullets":
			if len(block.Items) == 0 {
				continue
			}
			block.Text = ""
			block.ImageRef = ""
		case "image":
			if block.ImageRef == "" {
				continue
			}
			block.Text = ""
			block.Items = nil
		default:
			continue
		}
		normalized = append(normalized, block)
	}
	return normalized
}

func normalizeBlockItems(items []string) []string {
	normalized := make([]string, 0, len(items))
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			normalized = append(normalized, item)
		}
	}
	return normalized
}

func slideTitle(slide Slide) string { return firstSlideBlockText(slide, "title") }

func slideContent(slide Slide) string { return firstSlideBlockText(slide, "paragraph") }

func slideImageRef(slide Slide) string {
	for _, block := range slide.Blocks {
		if block.Type == "image" {
			return block.ImageRef
		}
	}
	return ""
}

func firstSlideBlockText(slide Slide, blockType string) string {
	for _, block := range slide.Blocks {
		if block.Type == blockType {
			return block.Text
		}
	}
	return ""
}

func setSlideImageRef(slide Slide, imageRef string) Slide {
	slide = NormalizeSlideIR(slide)
	imageRef = strings.TrimSpace(imageRef)
	blocks := make([]SlideBlock, 0, len(slide.Blocks)+1)
	inserted := false
	for _, block := range slide.Blocks {
		if block.Type == "image" {
			if imageRef != "" && !inserted {
				blocks = append(blocks, SlideBlock{Type: "image", ImageRef: imageRef})
				inserted = true
			}
			continue
		}
		if block.Type == "note" && imageRef != "" && !inserted {
			blocks = append(blocks, SlideBlock{Type: "image", ImageRef: imageRef})
			inserted = true
		}
		blocks = append(blocks, block)
	}
	if imageRef != "" && !inserted {
		blocks = append(blocks, SlideBlock{Type: "image", ImageRef: imageRef})
	}
	slide.Blocks = normalizeSlideBlocks(blocks)
	slide.Title, slide.Content, slide.BulletPoints, slide.ImageURL, slide.SpeakerNotes = "", "", nil, "", ""
	return slide
}
