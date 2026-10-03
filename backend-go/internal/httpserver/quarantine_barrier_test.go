package httpserver

import "testing"

func TestGenerationTaskIDFromPointKey(t *testing.T) {
	if got := generationTaskIDFromPointKey("generation:capture:task_000234"); got != "task_000234" {
		t.Fatal(got)
	}
	if got := generationTaskIDFromPointKey("generation:release:task_000221"); got != "task_000221" {
		t.Fatal(got)
	}
	if got := generationTaskIDFromPointKey("generation:durable-release:task_000324"); got != "task_000324" {
		t.Fatal(got)
	}
	if got := generationTaskIDFromPointKey("generation:reserve:task_000323"); got != "task_000323" {
		t.Fatal(got)
	}
	if got := generationTaskIDFromPointKey("knowledge:capture:1"); got != "" {
		t.Fatal(got)
	}
}
