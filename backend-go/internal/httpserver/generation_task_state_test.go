package httpserver

import "testing"

func TestGenerationTaskStateTransitionGuard(t *testing.T) {
	states := []GenerationTaskState{
		GenerationTaskCreated, GenerationTaskQueued, GenerationTaskRunning,
		GenerationTaskSucceeded, GenerationTaskFailed, GenerationTaskCancelRequested,
		GenerationTaskCancelled, GenerationTaskExpired, GenerationTaskManualReview,
	}
	for _, state := range states {
		if !CanTransitionGenerationTaskState(state, state) {
			t.Errorf("%s should allow idempotent self transition", state)
		}
	}
	for _, terminal := range []GenerationTaskState{GenerationTaskSucceeded, GenerationTaskFailed, GenerationTaskCancelled, GenerationTaskExpired} {
		for _, next := range states {
			if next != terminal && CanTransitionGenerationTaskState(terminal, next) {
				t.Errorf("terminal %s must not transition to %s", terminal, next)
			}
		}
	}
	allowed := [][2]GenerationTaskState{
		{GenerationTaskCreated, GenerationTaskQueued},
		{GenerationTaskQueued, GenerationTaskRunning},
		{GenerationTaskRunning, GenerationTaskSucceeded},
		{GenerationTaskRunning, GenerationTaskFailed},
		{GenerationTaskRunning, GenerationTaskManualReview},
		{GenerationTaskRunning, GenerationTaskCancelRequested},
		{GenerationTaskCancelRequested, GenerationTaskCancelled},
		{GenerationTaskManualReview, GenerationTaskFailed},
	}
	for _, transition := range allowed {
		if !CanTransitionGenerationTaskState(transition[0], transition[1]) {
			t.Errorf("expected transition %s -> %s", transition[0], transition[1])
		}
	}
}

func TestGenerationTaskStateLegacyReadCompatibility(t *testing.T) {
	cases := []struct {
		name string
		task generationTask
		want GenerationTaskState
	}{
		{"legacy processing", generationTask{Status: "PROCESSING", TaskStatus: "CREATED"}, GenerationTaskRunning},
		{"legacy completed", generationTask{Status: "COMPLETED"}, GenerationTaskSucceeded},
		{"pr1 canonical", generationTask{Status: "PROCESSING", TaskStatus: "FAILED"}, GenerationTaskFailed},
		{"cancel spelling", generationTask{Status: "CANCELED"}, GenerationTaskCancelled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := canonicalGenerationTaskState(tc.task); got != tc.want {
				t.Fatalf("state = %s, want %s", got, tc.want)
			}
		})
	}
}
