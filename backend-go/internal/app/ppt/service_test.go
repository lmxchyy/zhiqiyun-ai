package ppt

import (
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
