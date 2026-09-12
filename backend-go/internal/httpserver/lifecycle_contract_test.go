package httpserver

import (
	"context"
	"path/filepath"
	"testing"
)

func TestFailGenerationTaskDurableIsIdempotentForJSONLifecycle(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	writeGenerationBillingPointSeed(t, dataPath, "lifecycle-contract-user", 3)
	store := newJSONStore(dataPath)
	task, err := store.CreatePendingGenerationTask(generationBillingTestRequest("lifecycle-contract-user", 2))
	if err != nil {
		t.Fatal(err)
	}

	first, err := store.FailGenerationTaskDurable(task.ID, "durable failure")
	if err != nil {
		t.Fatalf("first durable failure: %v", err)
	}
	accountAfterFirst := generationBillingPointAccount(t, store, "lifecycle-contract-user")
	stateAfterFirst, err := store.PersonalPointService().repo.(*JSONPersonalPointStore).readState(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	second, err := store.FailGenerationTaskDurable(task.ID, "duplicate durable failure")
	if err != nil {
		t.Fatalf("duplicate durable failure: %v", err)
	}
	accountAfterSecond := generationBillingPointAccount(t, store, "lifecycle-contract-user")
	stateAfterSecond, err := store.PersonalPointService().repo.(*JSONPersonalPointStore).readState(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if first.Status != "FAILED" || second.Status != "FAILED" {
		t.Fatalf("durable failure statuses = %q/%q, want FAILED/FAILED", first.Status, second.Status)
	}
	if accountAfterFirst != accountAfterSecond {
		t.Fatalf("duplicate durable failure changed account: first=%+v second=%+v", accountAfterFirst, accountAfterSecond)
	}
	if len(stateAfterFirst.Reservations) != len(stateAfterSecond.Reservations) || len(stateAfterFirst.WalletLedger) != len(stateAfterSecond.WalletLedger) {
		t.Fatalf("duplicate durable failure changed ledger cardinality: first reservations=%d ledger=%d second reservations=%d ledger=%d", len(stateAfterFirst.Reservations), len(stateAfterFirst.WalletLedger), len(stateAfterSecond.Reservations), len(stateAfterSecond.WalletLedger))
	}
}
