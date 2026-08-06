package ppt

import (
	"errors"
	"path/filepath"
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
	if _, err := service.UpdateSlideImage(testOwner("user_001"), created.TaskID, "slide_1", "https://example.com/ppt.png"); err != nil {
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
	if slideImageRef(task.Slides[0]) != "https://example.com/ppt.png" || task.Slides[0].ImageURL != "" {
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
	if _, err := service.UpdateSlideImage(testOwner("user_edit"), created.TaskID, "slide_1", "https://example.test/visual.png"); err != nil {
		t.Fatal(err)
	}
	updated, err := service.UpdateSlideContent(testOwner("user_edit"), created.TaskID, "slide_1", Slide{Blocks: []SlideBlock{{Type: "title", Text: "新标题"}, {Type: "paragraph", Text: "新内容"}, {Type: "bullets", Items: []string{"要点一", "要点二"}}, {Type: "note", Text: "演讲备注"}, {Type: "image", ImageRef: "https://example.test/visual.png"}}, Layout: "imageText"})
	if err != nil {
		t.Fatal(err)
	}
	slide := updated.Slides[0]
	if slideTitle(slide) != "新标题" || slide.Layout != "imageText" || slideImageRef(slide) != "https://example.test/visual.png" || slide.Title != "" || slide.ImageURL != "" {
		t.Fatalf("unexpected updated slide: %#v", slide)
	}
	reloaded := NewPersistentService(path)
	persisted, err := reloaded.GetTask(testOwner("user_edit"), created.TaskID)
	if err != nil || firstSlideBlockText(persisted.Slides[0], "note") != "演讲备注" || len(persisted.Slides[0].Blocks) != 5 {
		t.Fatalf("slide content was not persisted: task=%#v err=%v", persisted, err)
	}
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
