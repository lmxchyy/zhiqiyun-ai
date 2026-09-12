package ppt

import "testing"

func TestPR4ADurableProgressDoesNotDependOnElapsedTime(t *testing.T) {
	first := Task{TaskID: "ppt_progress", Status: StatusPending, SlideCount: 5, CreatedAt: "2000-01-01T00:00:00Z"}
	second := first
	first = materializeTask(first)
	second = materializeTask(second)
	if first.Progress != 5 || second.Progress != 5 || first.Stage != StageQueued || second.Stage != StageQueued {
		t.Fatalf("queued progress must be durable and time-independent: first=%+v second=%+v", first, second)
	}
}

func TestPR4ADurableProgressCheckpointMapping(t *testing.T) {
	task := Task{TaskID: "ppt_progress", Status: StatusProcessing, SlideCount: 5, Outline: &Outline{Slides: make([]OutlineSlide, 5)}}
	for i := 0; i < 5; i++ {
		task.Slides = append(task.Slides, Slide{ID: string(rune('1' + i)), Page: i + 1})
	}
	applyDurableProgress(&task)
	if task.Stage != StageOutline || task.Progress != 20 || task.CurrentPage != 1 {
		t.Fatalf("outline checkpoint mapping: %+v", task)
	}
	task.Slides[0].VisualStatus = "success"
	task.Slides[0].VisualPlan = &VisualPlan{VisualType: "illustration"}
	applyDurableProgress(&task)
	if task.CompletedPages != 1 || task.Progress != 41 || task.CurrentPage != 2 {
		t.Fatalf("one of five slide mapping: %+v", task)
	}
	before := task.Progress
	task.ArtifactStatus = "ready"
	applyDurableProgress(&task)
	if task.Stage != StagePPTXReady || task.Progress != 95 || task.Progress < before {
		t.Fatalf("artifact checkpoint mapping: %+v", task)
	}
	task.Status = StatusSuccess
	applyDurableProgress(&task)
	if task.Stage != StageSucceeded || task.Progress != 100 || task.CurrentPage != 5 {
		t.Fatalf("settled mapping: %+v", task)
	}
}

func TestPR4AProgressNeverRegresses(t *testing.T) {
	task := Task{Status: StatusProcessing, SlideCount: 5, Progress: 65, Stage: StageSlideImages}
	applyDurableProgress(&task)
	if task.Progress != 65 {
		t.Fatalf("progress regressed without a new durable checkpoint: got %d", task.Progress)
	}
}
