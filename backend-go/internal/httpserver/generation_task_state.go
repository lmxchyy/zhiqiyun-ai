package httpserver

import (
	"fmt"
	"strings"
)

// GenerationTaskState is the persisted lifecycle state. TaskStatus is the
// canonical value; Status remains a legacy projection and is read as a
// fallback for rows written before PR1.
type GenerationTaskState string

const (
	GenerationTaskCreated         GenerationTaskState = "CREATED"
	GenerationTaskQueued          GenerationTaskState = "QUEUED"
	GenerationTaskRunning         GenerationTaskState = "RUNNING"
	GenerationTaskSucceeded       GenerationTaskState = "SUCCEEDED"
	GenerationTaskFailed          GenerationTaskState = "FAILED"
	GenerationTaskCancelRequested GenerationTaskState = "CANCEL_REQUESTED"
	GenerationTaskCancelled       GenerationTaskState = "CANCELLED"
	GenerationTaskExpired         GenerationTaskState = "EXPIRED"
	GenerationTaskManualReview    GenerationTaskState = "MANUAL_REVIEW"
)

func canonicalGenerationTaskState(task generationTask) GenerationTaskState {
	taskStatus := strings.ToUpper(strings.TrimSpace(task.TaskStatus))
	legacy := canonicalLegacyGenerationTaskState(task.Status)
	// PR1 defaults task_status to CREATED. For an old row, a meaningful legacy
	// status must win over that default so reads do not regress.
	if taskStatus == "" || (taskStatus == string(GenerationTaskCreated) && legacy != GenerationTaskCreated) {
		return legacy
	}
	return canonicalTaskState(taskStatus)
}

func canonicalLegacyGenerationTaskState(value string) GenerationTaskState {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "PENDING", "QUEUED":
		return GenerationTaskQueued
	case "PROCESSING", "RUNNING", "GENERATING", "RETRYING":
		return GenerationTaskRunning
	case "COMPLETED", "SUCCESS", "SUCCEEDED":
		return GenerationTaskSucceeded
	case "FAILED", "ERROR":
		return GenerationTaskFailed
	case "CANCEL_REQUESTED":
		return GenerationTaskCancelRequested
	case "CANCELLED", "CANCELED":
		return GenerationTaskCancelled
	case "EXPIRED", "TIMEOUT", "TIMED_OUT":
		return GenerationTaskExpired
	case "MANUAL_REVIEW", "REVIEW":
		return GenerationTaskManualReview
	default:
		return GenerationTaskCreated
	}
}

func canonicalTaskState(value string) GenerationTaskState {
	return canonicalLegacyGenerationTaskState(value)
}

func generationTaskStateTerminal(state GenerationTaskState) bool {
	switch state {
	case GenerationTaskSucceeded, GenerationTaskFailed, GenerationTaskCancelled, GenerationTaskExpired:
		return true
	default:
		return false
	}
}

// CanTransitionGenerationTaskState intentionally permits queued -> succeeded
// and queued -> failed: existing workers can settle without a separate RUNNING
// write. It still makes terminal states immutable (apart from idempotent self).
func CanTransitionGenerationTaskState(from, to GenerationTaskState) bool {
	if from == to {
		return true
	}
	if generationTaskStateTerminal(from) {
		return false
	}
	switch from {
	case GenerationTaskCreated:
		return to == GenerationTaskQueued || to == GenerationTaskRunning || to == GenerationTaskFailed || to == GenerationTaskCancelRequested
	case GenerationTaskQueued:
		return to == GenerationTaskRunning || to == GenerationTaskSucceeded || to == GenerationTaskFailed || to == GenerationTaskCancelRequested || to == GenerationTaskCancelled || to == GenerationTaskExpired || to == GenerationTaskManualReview
	case GenerationTaskRunning:
		return to == GenerationTaskSucceeded || to == GenerationTaskFailed || to == GenerationTaskCancelRequested || to == GenerationTaskCancelled || to == GenerationTaskExpired || to == GenerationTaskManualReview
	case GenerationTaskCancelRequested:
		return to == GenerationTaskCancelled || to == GenerationTaskFailed
	case GenerationTaskManualReview:
		return to == GenerationTaskSucceeded || to == GenerationTaskFailed || to == GenerationTaskCancelled || to == GenerationTaskExpired
	default:
		return false
	}
}

func guardGenerationTaskTransition(task generationTask, to GenerationTaskState) error {
	from := canonicalGenerationTaskState(task)
	if !CanTransitionGenerationTaskState(from, to) {
		return fmt.Errorf("invalid generation task transition: %s -> %s", from, to)
	}
	return nil
}
