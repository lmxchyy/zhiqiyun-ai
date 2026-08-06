package ppt

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestGetTaskDoesNotMaterializeStatusFromCreatedAt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ppt-tasks.json")
	service := NewPersistentService(path)
	createdAt := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	service.tasks["ppt_old_draft"] = Task{
		TaskID:    "ppt_old_draft",
		UserID:    "user_old_draft",
		TenantID:  "tenant_default",
		Stage:     StageDraft,
		Status:    StatusPending,
		Progress:  0,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}
	if err := service.saveLocked(); err != nil {
		t.Fatalf("save persisted draft: %v", err)
	}

	reloaded := NewPersistentService(path)
	task, err := reloaded.GetTask(testOwner("user_old_draft"), "ppt_old_draft")
	if err != nil {
		t.Fatalf("GetTask() error = %v", err)
	}
	if task.Stage != StageDraft || task.Status != StatusPending || task.Progress != 0 || task.CurrentPage != 0 {
		t.Fatalf("GetTask() materialized old draft from wall clock: %#v", task)
	}
}

func TestPersistentServiceKeepsTasksAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ppt-tasks.json")
	service := NewPersistentService(path)
	outline := &Outline{
		Title: "持久化验证",
		Slides: []OutlineSlide{{
			Page:         1,
			Title:        "持久化验证",
			Summary:      "验证 PPT 任务重启后仍可读取。",
			BulletPoints: []string{"生成任务", "重启服务", "找回历史"},
			Layout:       "cover",
		}},
	}
	created, err := service.Generate(GenerateRequest{
		Owner:       testOwner("user_001"),
		Prompt:      "持久化验证",
		SlideCount:  1,
		Language:    "zh",
		ImageSource: "ai",
		ImageModel:  "gpt-image-2",
		Outline:     outline,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if _, err := service.UpdateSlideImage(testOwner("user_001"), created.TaskID, "slide_1", "storage://tenant_default/ppt_image"); err != nil {
		t.Fatalf("UpdateSlideImage() error = %v", err)
	}

	reloaded := NewPersistentService(path)
	task, err := reloaded.GetTask(testOwner("user_001"), created.TaskID)
	if err != nil {
		t.Fatalf("GetTask() after reload error = %v", err)
	}
	if task.Title != "持久化验证" {
		t.Fatalf("task title = %q, want 持久化验证", task.Title)
	}
	if len(task.Slides) != 1 {
		t.Fatalf("slides length = %d, want 1", len(task.Slides))
	}
	if slideImageRef(task.Slides[0]) != "storage://tenant_default/ppt_image" || task.Slides[0].ImageURL != "" {
		t.Fatalf("canonical image ref not persisted: %#v", task.Slides[0])
	}
	history := reloaded.History(testOwner("user_001"))
	if len(history) != 1 || history[0].TaskID != created.TaskID {
		t.Fatalf("History() = %+v, want generated task", history)
	}
}

func TestUpdateSlideContentPersistsWithoutReplacingVisualState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ppt-tasks.json")
	service := NewPersistentService(path)
	created, err := service.Generate(GenerateRequest{
		Owner: testOwner("user_edit"), Prompt: "编辑验证", SlideCount: 1,
		Outline: &Outline{Title: "编辑验证", Slides: []OutlineSlide{{Page: 1, Title: "旧标题", Summary: "旧内容", BulletPoints: []string{"旧要点"}, Layout: "content"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpdateSlideImage(testOwner("user_edit"), created.TaskID, "slide_1", "storage://tenant_default/visual_edit"); err != nil {
		t.Fatal(err)
	}
	updated, err := service.UpdateSlideContent(testOwner("user_edit"), created.TaskID, "slide_1", Slide{Blocks: []SlideBlock{{Type: "title", Text: "新标题"}, {Type: "paragraph", Text: "新内容"}, {Type: "bullets", Items: []string{"要点一", "要点二"}}, {Type: "note", Text: "演讲备注"}, {Type: "image", ImageRef: "storage://tenant_default/visual_edit"}}, Layout: "imageText"})
	if err != nil {
		t.Fatal(err)
	}
	slide := updated.Slides[0]
	if slideTitle(slide) != "新标题" || slide.Layout != "imageText" || slideImageRef(slide) != "storage://tenant_default/visual_edit" || slide.Title != "" || slide.ImageURL != "" {
		t.Fatalf("unexpected updated slide: %#v", slide)
	}
	reloaded := NewPersistentService(path)
	persisted, err := reloaded.GetTask(testOwner("user_edit"), created.TaskID)
	if err != nil || firstSlideBlockText(persisted.Slides[0], "note") != "演讲备注" || len(persisted.Slides[0].Blocks) != 5 {
		t.Fatalf("slide content was not persisted: task=%#v err=%v", persisted, err)
	}
}

func TestUpdateSlideImageRejectsNonCanonicalAndCrossTenantReferencesBeforeMutation(t *testing.T) {
	invalidReferences := []string{
		"",
		"https://example.test/image.png",
		"data:image/png;base64,AAAA",
		"file:///tmp/image.png",
		"storage://tenant_other/image_1",
		" storage://tenant_default/image_1",
		"storage://tenant_default/image_1 ",
		"storage://TENANT_DEFAULT/image_1",
		"storage://tenant_default/folder/image_1",
		"storage://tenant_default/image_1?download=1",
		"storage://tenant_default/%69mage_1",
	}
	for _, reference := range invalidReferences {
		t.Run(reference, func(t *testing.T) {
			service, taskID := newCanonicalVisualReferenceTestTask(t, "user_invalid_image")
			service.mu.Lock()
			before := cloneTask(service.tasks[taskID])
			service.mu.Unlock()
			if _, err := service.UpdateSlideImage(testOwner("user_invalid_image"), taskID, "slide_1", reference); err == nil {
				t.Fatalf("UpdateSlideImage(%q) succeeded, want canonical reference rejection", reference)
			}
			service.mu.Lock()
			after := cloneTask(service.tasks[taskID])
			service.mu.Unlock()
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("rejected image reference changed canonical state\nbefore=%#v\nafter=%#v", before, after)
			}
		})
	}

	service, taskID := newCanonicalVisualReferenceTestTask(t, "user_valid_image")
	const canonical = "storage://tenant_default/image_1"
	updated, err := service.UpdateSlideImage(testOwner("user_valid_image"), taskID, "slide_1", canonical)
	if err != nil {
		t.Fatalf("UpdateSlideImage(canonical) error = %v", err)
	}
	if got := slideImageRef(updated.Slides[0]); got != canonical {
		t.Fatalf("canonical image reference = %q, want %q", got, canonical)
	}
}

func TestPostgresUpdateSlideImageRejectsEmptyBeforeMutation(t *testing.T) {
	db, state := newPPTPostgresTestDB(t)
	service := NewPostgresService(db)
	owner := testOwner("user_postgres_empty_image")
	response, err := service.Generate(GenerateRequest{
		Owner: owner, Prompt: "Reject empty image", SlideCount: 1, ImageSource: "ai",
		Outline: &Outline{Slides: []OutlineSlide{{Title: "Cover", SlideType: "cover"}}},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	const original = "storage://tenant_default/original_image"
	if _, err := service.UpdateSlideImage(owner, response.TaskID, "slide_1", original); err != nil {
		t.Fatalf("UpdateSlideImage(original) error = %v", err)
	}
	before, ok := state.snapshot(response.TaskID)
	if !ok {
		t.Fatalf("task %q was not persisted", response.TaskID)
	}
	writesBefore := state.strictUpsertCount()

	if _, err := service.UpdateSlideImage(owner, response.TaskID, "slide_1", ""); !errors.Is(err, ErrInvalidVisualReference) {
		t.Fatalf("UpdateSlideImage(empty) error = %v, want ErrInvalidVisualReference", err)
	}
	assertPPTPostgresRowUnchanged(t, state, response.TaskID, before)
	if writesAfter := state.strictUpsertCount(); writesAfter != writesBefore {
		t.Fatalf("rejected empty image caused PostgreSQL write: before=%d after=%d", writesBefore, writesAfter)
	}
	reread, err := NewPostgresService(db).GetTask(owner, response.TaskID)
	if err != nil {
		t.Fatalf("fresh GetTask() error = %v", err)
	}
	if got := slideImageRef(reread.Slides[0]); got != original {
		t.Fatalf("rejected empty image changed persisted reference to %q, want %q", got, original)
	}
}

func TestCanonicalVisualReferenceValidationCoversContentCompletionAndRestore(t *testing.T) {
	const (
		canonical   = "storage://tenant_default/image_current"
		crossTenant = "storage://tenant_other/image_cross"
	)

	t.Run("slide content image block", func(t *testing.T) {
		service, taskID := newCanonicalVisualReferenceTestTask(t, "user_content_ref")
		_, err := service.UpdateSlideContent(testOwner("user_content_ref"), taskID, "slide_1", Slide{Blocks: []SlideBlock{
			{Type: "title", Text: "Title"},
			{Type: "image", ImageRef: crossTenant},
		}})
		if err == nil {
			t.Fatal("cross-tenant content image block succeeded")
		}
	})

	t.Run("visual completion", func(t *testing.T) {
		service, taskID := newCanonicalVisualReferenceTestTask(t, "user_complete_ref")
		before, err := service.GetTask(testOwner("user_complete_ref"), taskID)
		if err != nil {
			t.Fatal(err)
		}
		beforeReference := slideImageRef(before.Slides[0])
		_, err = service.CompleteSlideVisual(testOwner("user_complete_ref"), taskID, "slide_1", VisualPlan{VisualType: "illustration", ImageRequired: true}, VisualAsset{URL: crossTenant})
		if err == nil {
			t.Fatal("cross-tenant visual completion succeeded")
		}
		task, getErr := service.GetTask(testOwner("user_complete_ref"), taskID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if got := slideImageRef(task.Slides[0]); got != beforeReference {
			t.Fatalf("rejected completed visual changed canonical state: before=%q after=%q", beforeReference, got)
		}
	})

	t.Run("visual restore", func(t *testing.T) {
		service, taskID := newCanonicalVisualReferenceTestTask(t, "user_restore_ref")
		task := service.tasks[taskID]
		task.Slides[0] = setSlideImageRef(task.Slides[0], canonical)
		task.Slides[0].VisualHistory = []VisualAsset{{URL: crossTenant, CreatedAt: "2026-08-06T00:00:00Z"}}
		service.tasks[taskID] = task

		_, err := service.RestoreSlideVisual(testOwner("user_restore_ref"), taskID, "slide_1", "2026-08-06T00:00:00Z", crossTenant)
		if err == nil {
			t.Fatal("cross-tenant visual restore succeeded")
		}
		unchanged, getErr := service.GetTask(testOwner("user_restore_ref"), taskID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if got := slideImageRef(unchanged.Slides[0]); got != canonical {
			t.Fatalf("rejected restore changed current image to %q", got)
		}
	})
}

func TestGeneratedSlidesDoNotPersistNonCanonicalPlaceholderVisuals(t *testing.T) {
	service, taskID := newCanonicalVisualReferenceTestTask(t, "user_no_placeholder")
	task, err := service.GetTask(testOwner("user_no_placeholder"), taskID)
	if err != nil {
		t.Fatal(err)
	}
	if got := slideImageRef(task.Slides[0]); got != "" {
		t.Fatalf("generated canonical task persisted a non-storage placeholder: %q", got)
	}
}

func newCanonicalVisualReferenceTestTask(t *testing.T, userID string) (*Service, string) {
	t.Helper()
	service := NewService()
	created, err := service.Generate(GenerateRequest{
		Owner: testOwner(userID), Prompt: "canonical visual reference", SlideCount: 1, ImageSource: "ai",
		Outline: &Outline{Title: "Canonical", Slides: []OutlineSlide{{Page: 1, Title: "Slide", Summary: "Body", SlideType: "text_image"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return service, created.TaskID
}

func TestGenerateWithConcurrencyRejectsActiveTask(t *testing.T) {
	service := NewService()
	request := GenerateRequest{Owner: testOwner("limited_user"), Prompt: "first deck", SlideCount: 5}
	if _, err := service.GenerateWithConcurrency(request, 0, 1); err != nil {
		t.Fatalf("first generation: %v", err)
	}
	if _, err := service.GenerateWithConcurrency(request, 0, 1); !errors.Is(err, ErrConcurrency) {
		t.Fatalf("second generation error = %v, want ErrConcurrency", err)
	}
	if _, err := service.GenerateWithConcurrency(GenerateRequest{Owner: testOwner("another_user"), Prompt: "other deck", SlideCount: 5}, 0, 1); err != nil {
		t.Fatalf("other user should have a separate slot: %v", err)
	}
	if _, err := NewService().GenerateWithConcurrency(request, 1, 1); !errors.Is(err, ErrConcurrency) {
		t.Fatalf("external active task should exhaust PPT slot: %v", err)
	}
}

func TestGenerateWithConcurrencyReusesClientRequestID(t *testing.T) {
	service := NewService()
	request := GenerateRequest{Owner: testOwner("connector_user"), ClientRequestID: "feishu:message-1", Prompt: "招商方案", SlideCount: 8}
	first, err := service.GenerateWithConcurrency(request, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.GenerateWithConcurrency(request, 0, 1)
	if err != nil {
		t.Fatalf("idempotent request was rejected: %v", err)
	}
	if first.TaskID != second.TaskID {
		t.Fatalf("idempotent task ids differ: %s != %s", first.TaskID, second.TaskID)
	}
	if history := service.History(testOwner("connector_user")); len(history) != 1 {
		t.Fatalf("idempotent request created %d tasks", len(history))
	}
}
