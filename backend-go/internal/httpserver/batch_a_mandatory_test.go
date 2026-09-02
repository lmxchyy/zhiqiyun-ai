package httpserver

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestTEST_G_JSONPostgresDurableFailureMismatchExplicitErrorNoMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	writeGenerationBillingPointSeed(t, path, "batch-g-user", 3)
	store := newJSONStore(path)
	task, err := store.CreatePendingGenerationTask(generationBillingTestRequest("batch-g-user", 2))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.update(func(data *platformData) error {
		for i := range data.GenerationTasks {
			if data.GenerationTasks[i].ID == task.ID {
				data.GenerationTasks[i].PointCost = 99
				return nil
			}
		}
		return errors.New("task not found")
	}); err != nil {
		t.Fatal(err)
	}
	before := generationBillingPointAccount(t, store, "batch-g-user")
	if _, err := store.FailGenerationTaskDurable(task.ID, "stale"); !errors.Is(err, ErrPersonalPointImportConflict) {
		t.Fatalf("durable mismatch error=%v, want ErrPersonalPointImportConflict", err)
	}
	after := generationBillingPointAccount(t, store, "batch-g-user")
	if before != after {
		t.Fatalf("mismatch mutated account: before=%+v after=%+v", before, after)
	}
	got, err := store.ListGenerationTasks()
	if err != nil {
		t.Fatal(err)
	}
	if generationBillingTaskByID(t, got, task.ID).Status == "FAILED" {
		t.Fatal("mismatch incorrectly failed task/released points")
	}
}
