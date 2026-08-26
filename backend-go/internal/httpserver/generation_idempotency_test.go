package httpserver

import (
	"sync"
	"testing"
)

func TestGenerationDuplicateReturnsExistingTaskAsReplay(t *testing.T) {
	store := newBillingAcceptanceStore(t)
	req := videoAcceptanceRequest("idem-replay")

	first, err := store.CreatePendingGenerationTask(req)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	if first.IdempotentReplay {
		t.Fatal("first create was marked as replay")
	}

	second, err := store.CreatePendingGenerationTask(req)
	if err != nil {
		t.Fatalf("duplicate create: %v", err)
	}
	if !second.IdempotentReplay {
		t.Fatal("duplicate create was not marked as replay")
	}
	if second.ID != first.ID {
		t.Fatalf("duplicate task id = %s, want %s", second.ID, first.ID)
	}
}

func TestGenerationDuplicateConcurrentRequestsCreateOneTask(t *testing.T) {
	store := newBillingAcceptanceStore(t)
	req := videoAcceptanceRequest("idem-concurrent")

	const requests = 8
	results := make(chan generationTask, requests)
	errs := make(chan error, requests)
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			task, err := store.CreatePendingGenerationTask(req)
			if err != nil {
				errs <- err
				return
			}
			results <- task
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent create: %v", err)
	}

	var first generationTask
	for task := range results {
		if first.ID == "" {
			first = task
			continue
		}
		if task.ID != first.ID {
			t.Fatalf("concurrent task id = %s, want %s", task.ID, first.ID)
		}
	}
	tasks, err := store.ListGenerationTasks()
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("task count = %d, want 1", len(tasks))
	}
	ledger, err := store.ListWalletLedger()
	if err != nil {
		t.Fatal(err)
	}
	if countTaskLedger(ledger, first.ID, "RESERVE") != 1 {
		t.Fatalf("reserve count for %s is not 1", first.ID)
	}
}
