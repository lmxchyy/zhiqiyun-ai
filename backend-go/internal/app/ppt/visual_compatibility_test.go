package ppt

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyTaskWithoutVisualPlanLoadsNormally(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ppt-tasks.json")
	legacy := persistedState{Tasks: []persistedTask{{Task: Task{TaskID: "ppt_legacy", Slides: []Slide{{ID: "slide_1", Title: "Legacy", Content: "Body", Layout: "imageText"}}}, UserID: "user_1"}}}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewPersistentService(path)
	task, err := service.GetTask("user_1", "ppt_legacy")
	if err != nil {
		t.Fatalf("legacy task read failed: %v", err)
	}
	if len(task.Slides) != 1 || task.Slides[0].SlideType != "text_image" {
		t.Fatalf("legacy defaults not applied: %#v", task.Slides)
	}
}

func TestFailedVisualPlanUpdateKeepsOldImageAndContent(t *testing.T) {
	service := NewService()
	created, err := service.Generate(GenerateRequest{UserID: "user_1", Prompt: "deck", ImageSource: "ai", Outline: &Outline{Title: "deck", Slides: []OutlineSlide{{Title: "Title", Summary: "Body", SlideType: "text_image"}}}})
	if err != nil {
		t.Fatal(err)
	}
	task, slide, err := service.GetSlide("user_1", created.TaskID, "slide_1")
	if err != nil {
		t.Fatal(err)
	}
	oldContent, oldImage := slide.Content, slide.ImageURL
	plan := *slide.VisualPlan
	if _, err := service.UpdateSlideVisualPlan("user_1", task.TaskID, slide.ID, plan, "task_visual", "failed", "provider timeout"); err != nil {
		t.Fatal(err)
	}
	_, updated, err := service.GetSlide("user_1", task.TaskID, slide.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Content != oldContent || updated.ImageURL != oldImage {
		t.Fatalf("failed visual operation changed slide: %#v", updated)
	}
}

func TestRestoreSlideVisualSwapsCurrentAndHistoricalImagesWithoutChangingContent(t *testing.T) {
	service := NewService()
	created, err := service.Generate(GenerateRequest{UserID: "user_1", Prompt: "deck", ImageSource: "ai", ImageModel: "image-model", Outline: &Outline{Title: "deck", Slides: []OutlineSlide{{Title: "Title", Summary: "Body", SlideType: "text_image"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.UpdateSlideImage("user_1", created.TaskID, "slide_1", "https://example.test/old.png"); err != nil {
		t.Fatal(err)
	}
	if _, err = service.UpdateSlideImage("user_1", created.TaskID, "slide_1", "https://example.test/current.png"); err != nil {
		t.Fatal(err)
	}
	_, before, err := service.GetSlide("user_1", created.TaskID, "slide_1")
	if err != nil {
		t.Fatal(err)
	}
	var historical VisualAsset
	for _, asset := range before.VisualHistory {
		if asset.URL == "https://example.test/old.png" {
			historical = asset
		}
	}
	if historical.URL == "" {
		t.Fatalf("expected generated historical visual: %#v", before.VisualHistory)
	}
	updated, err := service.RestoreSlideVisual("user_1", created.TaskID, "slide_1", historical.CreatedAt, historical.URL)
	if err != nil {
		t.Fatal(err)
	}
	slide := updated.Slides[0]
	if slide.ImageURL != "https://example.test/old.png" || slide.Content != before.Content || slide.Title != before.Title {
		t.Fatalf("restore changed content or selected wrong image: %#v", slide)
	}
	if !containsVisualURL(slide.VisualHistory, "https://example.test/current.png") {
		t.Fatalf("current visual was not retained for rollback: %#v", slide.VisualHistory)
	}
	if slide.VisualPlan == nil || !slide.VisualPlan.ImageRequired || slide.VisualPlan.TextInImage {
		t.Fatalf("restored visual plan is invalid: %#v", slide.VisualPlan)
	}
	if _, err = service.RestoreSlideVisual("user_2", created.TaskID, "slide_1", historical.CreatedAt, historical.URL); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("cross-tenant restore should be hidden as not found, got %v", err)
	}
}

func TestCompleteSlideVisualAtomicallyUpdatesPlanImageAndHistoryMetadata(t *testing.T) {
	service := NewService()
	created, err := service.Generate(GenerateRequest{UserID: "user_1", Prompt: "deck", ImageSource: "ai", ImageModel: "deck-model", Outline: &Outline{Title: "deck", Slides: []OutlineSlide{{Title: "Title", Summary: "Body", SlideType: "text_image"}}}})
	if err != nil {
		t.Fatal(err)
	}
	_, original, err := service.GetSlide("user_1", created.TaskID, "slide_1")
	if err != nil {
		t.Fatal(err)
	}
	firstPlan := *original.VisualPlan
	firstPlan.TextInImage = true
	firstCreatedAt := "2026-07-16T01:00:00Z"
	if _, err = service.CompleteSlideVisual("user_1", created.TaskID, original.ID, firstPlan, VisualAsset{URL: "https://example.test/first.png", TaskID: "image_task_1", ModelName: "model_a", CreatedAt: firstCreatedAt}); err != nil {
		t.Fatal(err)
	}
	secondPlan := firstPlan
	secondPlan.Style = "consistent corporate 3d"
	updated, err := service.CompleteSlideVisual("user_1", created.TaskID, original.ID, secondPlan, VisualAsset{URL: "https://example.test/second.png", TaskID: "image_task_2", ModelName: "model_b", CreatedAt: "2026-07-16T02:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	slide := updated.Slides[0]
	if slide.Title != original.Title || slide.Content != original.Content || slide.ImageURL != "https://example.test/second.png" {
		t.Fatalf("atomic completion changed content or selected wrong image: %#v", slide)
	}
	if slide.VisualPlan == nil || slide.VisualPlan.TextInImage || slide.VisualPlan.Style != secondPlan.Style || slide.VisualTaskID != "image_task_2" || slide.VisualModelName != "model_b" {
		t.Fatalf("atomic completion metadata mismatch: %#v", slide)
	}
	var firstHistory VisualAsset
	for _, asset := range slide.VisualHistory {
		if asset.URL == "https://example.test/first.png" {
			firstHistory = asset
		}
	}
	if firstHistory.TaskID != "image_task_1" || firstHistory.ModelName != "model_a" || firstHistory.CreatedAt != firstCreatedAt {
		t.Fatalf("historical visual metadata was not preserved: %#v", slide.VisualHistory)
	}
	if _, err = service.UpdateSlideVisualPlan("user_1", created.TaskID, original.ID, secondPlan, "", "processing", ""); err != nil {
		t.Fatal(err)
	}
	_, processing, err := service.GetSlide("user_1", created.TaskID, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if processing.VisualTaskID != "image_task_2" || processing.ImageURL != "https://example.test/second.png" {
		t.Fatalf("planning status overwrote current visual metadata: %#v", processing)
	}
	secondPlan.VisualType = "none"
	disabled, err := service.DisableSlideVisual("user_1", created.TaskID, original.ID, secondPlan)
	if err != nil {
		t.Fatal(err)
	}
	disabledSlide := disabled.Slides[0]
	if disabledSlide.ImageURL != "" || disabledSlide.VisualTaskID != "" || disabledSlide.VisualPlan == nil || disabledSlide.VisualPlan.ImageRequired || disabledSlide.Content != original.Content {
		t.Fatalf("disable visual was not atomic: %#v", disabledSlide)
	}
	var secondHistory VisualAsset
	for _, asset := range disabledSlide.VisualHistory {
		if asset.URL == "https://example.test/second.png" {
			secondHistory = asset
		}
	}
	if secondHistory.TaskID != "image_task_2" || secondHistory.ModelName != "model_b" || secondHistory.CreatedAt != "2026-07-16T02:00:00Z" {
		t.Fatalf("disabled current visual metadata was not archived: %#v", disabledSlide.VisualHistory)
	}
}

func containsVisualURL(items []VisualAsset, target string) bool {
	for _, item := range items {
		if item.URL == target {
			return true
		}
	}
	return false
}
