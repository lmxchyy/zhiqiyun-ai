package ppt

import (
	"errors"
	"path/filepath"
	"testing"
)

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
		UserID:      "user_001",
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
	if _, err := service.UpdateSlideImage("user_001", created.TaskID, "slide_1", "https://example.com/ppt.png"); err != nil {
		t.Fatalf("UpdateSlideImage() error = %v", err)
	}

	reloaded := NewPersistentService(path)
	task, err := reloaded.GetTask("user_001", created.TaskID)
	if err != nil {
		t.Fatalf("GetTask() after reload error = %v", err)
	}
	if task.Title != "持久化验证" {
		t.Fatalf("task title = %q, want 持久化验证", task.Title)
	}
	if len(task.Slides) != 1 {
		t.Fatalf("slides length = %d, want 1", len(task.Slides))
	}
	if task.Slides[0].ImageURL != "https://example.com/ppt.png" {
		t.Fatalf("image URL = %q, want persisted image URL", task.Slides[0].ImageURL)
	}
	history := reloaded.History("user_001")
	if len(history) != 1 || history[0].TaskID != created.TaskID {
		t.Fatalf("History() = %+v, want generated task", history)
	}
}

func TestGenerateWithConcurrencyRejectsActiveTask(t *testing.T) {
	service := NewService()
	request := GenerateRequest{UserID: "limited_user", Prompt: "first deck", SlideCount: 5}
	if _, err := service.GenerateWithConcurrency(request, 0, 1); err != nil {
		t.Fatalf("first generation: %v", err)
	}
	if _, err := service.GenerateWithConcurrency(request, 0, 1); !errors.Is(err, ErrConcurrency) {
		t.Fatalf("second generation error = %v, want ErrConcurrency", err)
	}
	if _, err := service.GenerateWithConcurrency(GenerateRequest{UserID: "another_user", Prompt: "other deck", SlideCount: 5}, 0, 1); err != nil {
		t.Fatalf("other user should have a separate slot: %v", err)
	}
	if _, err := NewService().GenerateWithConcurrency(request, 1, 1); !errors.Is(err, ErrConcurrency) {
		t.Fatalf("external active task should exhaust PPT slot: %v", err)
	}
}
