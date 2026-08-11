package httpserver

import (
	"path/filepath"
	"testing"
)

const billingAcceptanceUserID = "user_billing_acceptance"

func newBillingAcceptanceStore(t *testing.T) *jsonStore {
	t.Helper()
	store := newJSONStore(filepath.Join(t.TempDir(), "platform.json"))
	err := store.update(func(data *platformData) error {
		data.PointAccounts = []adminPointAccount{{
			ID:        "points_billing_acceptance",
			UserID:    billingAcceptanceUserID,
			Available: 2000,
			Frozen:    0,
		}}
		return nil
	})
	if err != nil {
		t.Fatalf("seed point account: %v", err)
	}
	return store
}

func videoAcceptanceRequest(clientRequestID string) createGenerationTaskRequest {
	return createGenerationTaskRequest{
		ClientRequestID: clientRequestID,
		UserID:          billingAcceptanceUserID,
		Type:            "TEXT_TO_VIDEO",
		ModuleCode:      moduleVideoGeneration,
		Prompt:          "5 second 720p billing acceptance video",
		Model:           "seedance-fast-2.0",
		Params: map[string]any{
			"duration":   5,
			"resolution": "720p",
		},
	}
}

func TestBillingCenterV1Acceptance(t *testing.T) {
	t.Run("5s 720p success quotes reserves and captures 600", func(t *testing.T) {
		store := newBillingAcceptanceStore(t)
		req := videoAcceptanceRequest("accept-success")
		pending, err := store.CreatePendingGenerationTask(req)
		if err != nil {
			t.Fatalf("create pending task: %v", err)
		}
		if pending.QuotedPoints != 600 || pending.ReservedPoints != 600 || pending.BillingStatus != billingStatusReserved {
			t.Fatalf("pending billing snapshot = quoted %.0f reserved %.0f status %s, want 600/600/RESERVED", pending.QuotedPoints, pending.ReservedPoints, pending.BillingStatus)
		}
		account, err := store.PointAccount(billingAcceptanceUserID)
		if err != nil {
			t.Fatalf("point account after reserve: %v", err)
		}
		if account.Available != 1400 || account.Frozen != 600 {
			t.Fatalf("account after reserve = available %d frozen %d, want 1400/600", account.Available, account.Frozen)
		}

		completed, err := store.CompleteGenerationTask(pending.ID, req)
		if err != nil {
			t.Fatalf("complete task: %v", err)
		}
		if completed.TaskStatus != taskStatusSucceeded || completed.BillingStatus != billingStatusCaptured || completed.CapturedPoints != 600 {
			t.Fatalf("completed task = taskStatus %s billingStatus %s captured %.0f", completed.TaskStatus, completed.BillingStatus, completed.CapturedPoints)
		}
		account, _ = store.PointAccount(billingAcceptanceUserID)
		if account.Available != 1400 || account.Frozen != 0 {
			t.Fatalf("account after capture = available %d frozen %d, want 1400/0", account.Available, account.Frozen)
		}
		events, err := store.ListBillingLifecycleEvents()
		if err != nil || countTaskEvents(events, pending.ID, "QUOTE", "RESERVE", "CAPTURE") != 3 {
			t.Fatalf("success lifecycle events are incomplete: events=%v err=%v", events, err)
		}
		ledger, err := store.ListWalletLedger()
		if err != nil || countTaskLedger(ledger, pending.ID, "RESERVE", "CAPTURE") != 2 {
			t.Fatalf("success wallet ledger is incomplete: ledger=%v err=%v", ledger, err)
		}
	})

	t.Run("upstream failure releases 600 and restores balance", func(t *testing.T) {
		store := newBillingAcceptanceStore(t)
		pending, err := store.CreatePendingGenerationTask(videoAcceptanceRequest("accept-failure"))
		if err != nil {
			t.Fatalf("create pending task: %v", err)
		}
		failed, err := store.FailGenerationTask(pending.ID, "upstream failed")
		if err != nil {
			t.Fatalf("fail task: %v", err)
		}
		if failed.TaskStatus != taskStatusFailed || failed.BillingStatus != billingStatusReleased || failed.ReleasedPoints != 600 {
			t.Fatalf("failed task = taskStatus %s billingStatus %s released %.0f", failed.TaskStatus, failed.BillingStatus, failed.ReleasedPoints)
		}
		account, _ := store.PointAccount(billingAcceptanceUserID)
		if account.Available != 2000 || account.Frozen != 0 {
			t.Fatalf("account after release = available %d frozen %d, want 2000/0", account.Available, account.Frozen)
		}
		events, _ := store.ListBillingLifecycleEvents()
		if countTaskEvents(events, pending.ID, "QUOTE", "RESERVE", "RELEASE") != 3 {
			t.Fatalf("failure lifecycle events are incomplete: %v", events)
		}
		ledger, _ := store.ListWalletLedger()
		if countTaskLedger(ledger, pending.ID, "RESERVE", "RELEASE") != 2 {
			t.Fatalf("failure wallet ledger is incomplete: %v", ledger)
		}
	})

	t.Run("same clientRequestId creates one task and one reserve", func(t *testing.T) {
		store := newBillingAcceptanceStore(t)
		req := videoAcceptanceRequest("accept-idempotent")
		first, err := store.CreatePendingGenerationTask(req)
		if err != nil {
			t.Fatalf("first submission: %v", err)
		}
		second, err := store.CreatePendingGenerationTask(req)
		if err != nil {
			t.Fatalf("duplicate submission: %v", err)
		}
		if first.ID != second.ID {
			t.Fatalf("duplicate submission returned task %s, want %s", second.ID, first.ID)
		}
		tasks, _ := store.ListGenerationTasks()
		if len(tasks) != 1 {
			t.Fatalf("task count = %d, want 1", len(tasks))
		}
		ledger, _ := store.ListWalletLedger()
		if countTaskLedger(ledger, first.ID, "RESERVE") != 1 {
			t.Fatalf("reserve ledger count is not 1: %v", ledger)
		}
		account, _ := store.PointAccount(billingAcceptanceUserID)
		if account.Available != 1400 || account.Frozen != 600 {
			t.Fatalf("duplicate reserve changed account twice: available %d frozen %d", account.Available, account.Frozen)
		}
	})

	t.Run("old task keeps old price and new task uses published version", func(t *testing.T) {
		store := newBillingAcceptanceStore(t)
		oldTask, err := store.CreatePendingGenerationTask(videoAcceptanceRequest("accept-version-old"))
		if err != nil {
			t.Fatalf("create old task: %v", err)
		}
		if oldTask.QuotedPoints != 600 || oldTask.BillingRuleVersionID == "" {
			t.Fatalf("old task snapshot = quote %.0f version %q", oldTask.QuotedPoints, oldTask.BillingRuleVersionID)
		}

		rules, err := store.ListBillingRuleVersions()
		if err != nil {
			t.Fatalf("list rules: %v", err)
		}
		var source billingRuleVersion
		for _, rule := range rules {
			if rule.ModelCode == "seedance-fast-2.0" && rule.Status == "PUBLISHED" {
				source = rule
				break
			}
		}
		if source.ID == "" {
			t.Fatal("seedance published rule not found")
		}
		_, err = store.UpdateAdminBillingRule(source.ID, adminBillingRuleMutation{
			BillingType:         "per_second",
			BasePrice:           20,
			MinimumCharge:       1,
			ParameterMultiplier: source.ParameterRules,
			Status:              "DRAFT",
		})
		if err != nil {
			t.Fatalf("create rule draft: %v", err)
		}
		rules, _ = store.ListBillingRuleVersions()
		var draft billingRuleVersion
		for _, rule := range rules {
			if rule.ModelCode == source.ModelCode && rule.Status == "DRAFT" && rule.Version > source.Version {
				draft = rule
				break
			}
		}
		if draft.ID == "" {
			t.Fatal("new draft version not found")
		}
		validation, err := store.ValidateBillingRuleVersion(draft.ID)
		if err != nil || !validation.Valid {
			t.Fatalf("draft validation = valid %v issues %v err %v", validation.Valid, validation.Issues, err)
		}
		published, err := store.PublishBillingRuleVersion(draft.ID)
		if err != nil {
			t.Fatalf("publish new version: %v", err)
		}

		completedOld, err := store.CompleteGenerationTask(oldTask.ID, videoAcceptanceRequest("accept-version-old"))
		if err != nil {
			t.Fatalf("complete old task: %v", err)
		}
		if completedOld.CapturedPoints != 600 || completedOld.BillingRuleVersionID != oldTask.BillingRuleVersionID {
			t.Fatalf("old task changed price/version: captured %.0f version %s", completedOld.CapturedPoints, completedOld.BillingRuleVersionID)
		}

		newTask, err := store.CreatePendingGenerationTask(videoAcceptanceRequest("accept-version-new"))
		if err != nil {
			t.Fatalf("create new task: %v", err)
		}
		if newTask.QuotedPoints != 150 || newTask.BillingRuleVersionID != published.ID {
			t.Fatalf("new task = quote %.0f version %s, want 150/%s", newTask.QuotedPoints, newTask.BillingRuleVersionID, published.ID)
		}
	})
}

func countTaskEvents(items []billingLifecycleEvent, taskID string, eventTypes ...string) int {
	wanted := map[string]bool{}
	for _, eventType := range eventTypes {
		wanted[eventType] = true
	}
	count := 0
	for _, item := range items {
		if item.TaskID == taskID && wanted[item.EventType] {
			count++
		}
	}
	return count
}

func countTaskLedger(items []walletLedgerEntry, taskID string, entryTypes ...string) int {
	wanted := map[string]bool{}
	for _, entryType := range entryTypes {
		wanted[entryType] = true
	}
	count := 0
	for _, item := range items {
		if item.TaskID == taskID && wanted[item.EntryType] {
			count++
		}
	}
	return count
}
