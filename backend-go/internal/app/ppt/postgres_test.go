package ppt

import (
	"encoding/json"
	"testing"
)

func TestDecodePersistedTasksImportsLegacySlidesWithoutVisualPlan(t *testing.T) {
	raw, err := json.Marshal(persistedState{Tasks: []persistedTask{{
		UserID: "user_legacy",
		Task:   Task{TaskID: "ppt_legacy", Status: StatusSuccess, Slides: []Slide{{ID: "slide_1", Title: "Legacy", Content: "Body"}}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	tasks := decodePersistedTasks(raw)
	if len(tasks) != 1 || tasks[0].UserID != "user_legacy" {
		t.Fatalf("unexpected imported tasks: %#v", tasks)
	}
	if tasks[0].Slides[0].SlideType != "text_image" {
		t.Fatalf("legacy slide type = %q", tasks[0].Slides[0].SlideType)
	}
	if tasks[0].Slides[0].VisualPlan != nil {
		t.Fatal("legacy task import must not require a visual plan")
	}
}

func TestTaskFromGenerateRequestKeepsDeckVisualStyleAndNoTextDefault(t *testing.T) {
	req := normalizeRequest(GenerateRequest{
		UserID: "user_a", Prompt: "Enterprise AI", SlideCount: 1, Theme: "techBlue",
		ImageStyle: "corporate 3D", PeopleStyle: "natural", ImageLighting: "soft",
		Outline: &Outline{Slides: []OutlineSlide{{Title: "Cover", Summary: "AI assistant", SlideType: "cover"}}},
	})
	task := taskFromGenerateRequest(req)
	if task.UserID != req.UserID || task.ImageStyle != "corporate 3D" || task.TextInImage {
		t.Fatalf("unexpected task visual defaults: %#v", task)
	}
	if len(task.Slides) != 1 || task.Slides[0].VisualPlan == nil || task.Slides[0].VisualPlan.TextInImage {
		t.Fatalf("unexpected slide visual plan: %#v", task.Slides)
	}
}

func TestNewPostgresServiceWithoutDatabaseFallsBackToFileService(t *testing.T) {
	service := NewPostgresService(nil, "")
	if service == nil || service.db != nil {
		t.Fatalf("expected file service fallback: %#v", service)
	}
}
