package httpserver

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPendingGenerationTaskReservesAndRefundsPoints(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	writeGenerationBillingPointSeed(t, dataPath, "user_billing", 2)
	store := newJSONStore(dataPath)
	req := generationBillingTestRequest("user_billing", 2)

	task, err := store.CreatePendingGenerationTask(req)
	if err != nil {
		t.Fatalf("create pending generation task: %v", err)
	}
	if !generationTaskBillingReserved(task) {
		t.Fatalf("pending task was not marked as billing reserved: %+v", task.Params)
	}
	if task.PointCost != 2 {
		t.Fatalf("pending task point cost = %d, want 2", task.PointCost)
	}
	account := generationBillingPointAccount(t, store, "user_billing")
	if account.Available != 0 {
		t.Fatalf("available points after pending = %d, want 0", account.Available)
	}

	_, err = store.CreatePendingGenerationTask(req)
	if err == nil || !strings.Contains(err.Error(), "insufficient remaining points") {
		t.Fatalf("second pending task error = %v, want insufficient points", err)
	}

	failed, err := store.FailGenerationTask(task.ID, "provider failed")
	if err != nil {
		t.Fatalf("fail generation task: %v", err)
	}
	if !generationTaskBillingRefunded(failed) {
		t.Fatalf("failed task was not marked as billing refunded: %+v", failed.Params)
	}
	account = generationBillingPointAccount(t, store, "user_billing")
	if account.Available != 2 {
		t.Fatalf("available points after fail refund = %d, want 2", account.Available)
	}

	if _, err := store.FailGenerationTask(task.ID, "duplicate failure"); err != nil {
		t.Fatalf("repeat fail generation task: %v", err)
	}
	account = generationBillingPointAccount(t, store, "user_billing")
	if account.Available != 2 {
		t.Fatalf("available points after repeat fail = %d, want 2", account.Available)
	}
}

func TestCompletingReservedGenerationTaskDoesNotDoubleCharge(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	writeGenerationBillingPointSeed(t, dataPath, "user_billing", 3)
	store := newJSONStore(dataPath)
	req := generationBillingTestRequest("user_billing", 2)

	task, err := store.CreatePendingGenerationTask(req)
	if err != nil {
		t.Fatalf("create pending generation task: %v", err)
	}
	account := generationBillingPointAccount(t, store, "user_billing")
	if account.Available != 1 {
		t.Fatalf("available points after pending = %d, want 1", account.Available)
	}

	completed, err := store.CompleteGenerationTask(task.ID, createGenerationTaskRequest{})
	if err != nil {
		t.Fatalf("complete reserved generation task: %v", err)
	}
	if completed.Status != "SUCCEEDED" {
		t.Fatalf("completed task status = %q, want SUCCEEDED", completed.Status)
	}
	account = generationBillingPointAccount(t, store, "user_billing")
	if account.Available != 1 {
		t.Fatalf("available points after completion = %d, want 1", account.Available)
	}

	data, err := store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	events := 0
	for _, event := range data.BillingEvents {
		if event.TaskID != task.ID {
			continue
		}
		events++
		if event.BalanceBefore != 3 || event.BalanceAfter != 1 || event.PointCost != 2 {
			t.Fatalf("billing event balances = before %d after %d cost %d, want 3/1/2", event.BalanceBefore, event.BalanceAfter, event.PointCost)
		}
	}
	if events != 1 {
		t.Fatalf("billing events for task = %d, want 1", events)
	}

	if _, err := store.CompleteGenerationTask(task.ID, createGenerationTaskRequest{}); err != nil {
		t.Fatalf("repeat complete generation task: %v", err)
	}
	account = generationBillingPointAccount(t, store, "user_billing")
	if account.Available != 1 {
		t.Fatalf("available points after repeat completion = %d, want 1", account.Available)
	}
}

func TestRepairStaleGenerationTasksFailsAndRefundsReservedTask(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	writeGenerationBillingPointSeed(t, dataPath, "user_billing", 4)
	store := newJSONStore(dataPath)
	req := generationBillingTestRequest("user_billing", 2)

	staleTask, err := store.CreatePendingGenerationTask(req)
	if err != nil {
		t.Fatalf("create stale pending generation task: %v", err)
	}
	freshTask, err := store.CreatePendingGenerationTask(req)
	if err != nil {
		t.Fatalf("create fresh pending generation task: %v", err)
	}

	staleTime := time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339Nano)
	if err := store.update(func(data *platformData) error {
		for i := range data.GenerationTasks {
			if data.GenerationTasks[i].ID == staleTask.ID {
				data.GenerationTasks[i].CreatedAt = staleTime
				data.GenerationTasks[i].UpdatedAt = staleTime
				return nil
			}
		}
		return fmt.Errorf("generation task not found: %s", staleTask.ID)
	}); err != nil {
		t.Fatal(err)
	}

	api{store: store}.repairStaleGenerationTasks(15 * time.Minute)

	tasks, err := store.ListGenerationTasks()
	if err != nil {
		t.Fatal(err)
	}
	repaired := generationBillingTaskByID(t, tasks, staleTask.ID)
	if repaired.Status != "FAILED" {
		t.Fatalf("stale task status = %q, want FAILED", repaired.Status)
	}
	if !generationTaskBillingRefunded(repaired) {
		t.Fatalf("stale task was not marked as billing refunded: %+v", repaired.Params)
	}
	if repaired.WorkerFinishedAt == "" || repaired.FailureReason == "" {
		t.Fatalf("stale task missing failure metadata: %+v", repaired)
	}
	fresh := generationBillingTaskByID(t, tasks, freshTask.ID)
	if fresh.Status != "PROCESSING" {
		t.Fatalf("fresh task status = %q, want PROCESSING", fresh.Status)
	}
	if generationTaskBillingRefunded(fresh) {
		t.Fatalf("fresh task should not be refunded: %+v", fresh.Params)
	}
	account := generationBillingPointAccount(t, store, "user_billing")
	if account.Available != 2 {
		t.Fatalf("available points after stale repair = %d, want 2", account.Available)
	}
}

func TestDeleteAssetForUserSoftDeletesAsset(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "store.json")
	writeGenerationBillingPointSeed(t, dataPath, "user_billing", 2)
	store := newJSONStore(dataPath)
	req := generationBillingTestRequest("user_billing", 1)

	task, err := store.CreatePendingGenerationTask(req)
	if err != nil {
		t.Fatalf("create pending generation task: %v", err)
	}
	completed, err := store.CompleteGenerationTask(task.ID, createGenerationTaskRequest{
		Type:   req.Type,
		Prompt: req.Prompt,
		Model:  req.Model,
		Params: req.Params,
		GeneratedImages: []generatedImage{{
			URL:         "https://example.test/generated.png",
			ContentType: "image/png",
			Width:       512,
			Height:      512,
			Source:      "test-provider",
		}},
	})
	if err != nil {
		t.Fatalf("complete generation task: %v", err)
	}
	if len(completed.ResultIDs) != 1 {
		t.Fatalf("completed result ids = %+v, want 1 id", completed.ResultIDs)
	}
	assetID := completed.ResultIDs[0]

	if err := store.DeleteAssetForUser("user_billing", assetID); err != nil {
		t.Fatalf("delete asset: %v", err)
	}
	assets, err := store.ListAssets()
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 0 {
		t.Fatalf("active assets after delete = %+v, want none", assets)
	}
	adminData, err := store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	if len(adminData.Assets) != 0 {
		t.Fatalf("admin active assets after delete = %+v, want none", adminData.Assets)
	}
	rawData, err := store.load()
	if err != nil {
		t.Fatal(err)
	}
	if len(rawData.Assets) != 1 {
		t.Fatalf("raw assets after soft delete = %d, want 1", len(rawData.Assets))
	}
	if rawData.Assets[0].DeletedAt == "" || stringValue(rawData.Assets[0].Metadata["deletedAt"]) == "" {
		t.Fatalf("soft-deleted asset missing tombstone: %+v", rawData.Assets[0])
	}
	storedTask := generationBillingTaskByID(t, rawData.GenerationTasks, task.ID)
	for _, resultID := range storedTask.ResultIDs {
		if resultID == assetID {
			t.Fatalf("task result ids still contain deleted asset: %+v", storedTask.ResultIDs)
		}
	}
	if err := store.DeleteAssetForUser("user_billing", assetID); !errors.Is(err, errAssetNotFound) {
		t.Fatalf("second delete error = %v, want errAssetNotFound", err)
	}
}

func generationBillingTestRequest(userID string, count int) createGenerationTaskRequest {
	return createGenerationTaskRequest{
		UserID: userID,
		Type:   "TEXT_TO_IMAGE",
		Prompt: "billing reservation test",
		Model:  "mock-standard",
		Params: map[string]any{
			"count": float64(count),
		},
	}
}

func writeGenerationBillingPointSeed(t *testing.T, dataPath string, userID string, available int) {
	t.Helper()
	raw := fmt.Sprintf(`{"pointAccounts":[{"id":"points_test","userId":%q,"available":%d}],"counters":{}}`, userID, available)
	if err := os.WriteFile(dataPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
}

func generationBillingPointAccount(t *testing.T, store *jsonStore, userID string) pointAccount {
	t.Helper()
	account, err := store.PointAccount(userID)
	if err != nil {
		t.Fatal(err)
	}
	return account
}

func generationBillingTaskByID(t *testing.T, tasks []generationTask, id string) generationTask {
	t.Helper()
	for _, task := range tasks {
		if task.ID == id {
			return task
		}
	}
	t.Fatalf("generation task %s not found in %+v", id, tasks)
	return generationTask{}
}
