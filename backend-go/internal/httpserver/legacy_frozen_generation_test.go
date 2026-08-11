package httpserver

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

type countingStateBackend struct {
	delegate stateBackend
	writes   int
}

func (b *countingStateBackend) Read() ([]byte, error) { return b.delegate.Read() }

func (b *countingStateBackend) Write(content []byte) error {
	b.writes++
	return b.delegate.Write(content)
}

func TestLegacyFrozenGenerationV0CompletesThroughPersonalLot(t *testing.T) {
	store, taskID := seedLegacyFrozenGeneration(t, 0, false)

	completed, err := store.CompleteGenerationTask(taskID, createGenerationTaskRequest{})
	if err != nil {
		t.Fatalf("complete legacy frozen generation: %v", err)
	}
	if completed.Status != "SUCCEEDED" {
		t.Fatalf("completed status=%q, want SUCCEEDED", completed.Status)
	}
	assertGenerationPersonalLotMarkers(t, completed)
	assertLegacyFrozenTerminalState(t, store, taskID, "CAPTURED", 7, 0, 3, 0)

	before := loadPlatformDataForTest(t, store)
	if _, err := store.CompleteGenerationTask(taskID, createGenerationTaskRequest{}); err != nil {
		t.Fatalf("replay complete legacy frozen generation: %v", err)
	}
	after := loadPlatformDataForTest(t, store)
	if len(after.BillingEvents) != len(before.BillingEvents) || len(after.Assets) != len(before.Assets) || len(after.PersonalPoints.WalletLedger) != len(before.PersonalPoints.WalletLedger) || len(after.PersonalPoints.Movements) != len(before.PersonalPoints.Movements) || len(after.PersonalPoints.Reservations) != len(before.PersonalPoints.Reservations) || len(after.PersonalPoints.Allocations) != len(before.PersonalPoints.Allocations) {
		t.Fatalf("complete replay was not idempotent: before events/assets/ledger/movements/reservations/allocations=%d/%d/%d/%d/%d/%d after=%d/%d/%d/%d/%d/%d", len(before.BillingEvents), len(before.Assets), len(before.PersonalPoints.WalletLedger), len(before.PersonalPoints.Movements), len(before.PersonalPoints.Reservations), len(before.PersonalPoints.Allocations), len(after.BillingEvents), len(after.Assets), len(after.PersonalPoints.WalletLedger), len(after.PersonalPoints.Movements), len(after.PersonalPoints.Reservations), len(after.PersonalPoints.Allocations))
	}
}

func TestLegacyFrozenGenerationV0FailsThroughPersonalLot(t *testing.T) {
	store, taskID := seedLegacyFrozenGeneration(t, 0, false)

	failed, err := store.FailGenerationTask(taskID, "provider failed")
	if err != nil {
		t.Fatalf("fail legacy frozen generation: %v", err)
	}
	if failed.Status != "FAILED" {
		t.Fatalf("failed status=%q, want FAILED", failed.Status)
	}
	assertGenerationPersonalLotMarkers(t, failed)
	assertLegacyFrozenTerminalState(t, store, taskID, "RELEASED", 10, 0, 0, 3)

	before := loadPlatformDataForTest(t, store)
	if _, err := store.FailGenerationTask(taskID, "duplicate failure"); err != nil {
		t.Fatalf("replay fail legacy frozen generation: %v", err)
	}
	after := loadPlatformDataForTest(t, store)
	if len(after.PersonalPoints.WalletLedger) != len(before.PersonalPoints.WalletLedger) || len(after.PersonalPoints.Movements) != len(before.PersonalPoints.Movements) || len(after.PersonalPoints.Reservations) != len(before.PersonalPoints.Reservations) || len(after.PersonalPoints.Allocations) != len(before.PersonalPoints.Allocations) {
		t.Fatalf("fail replay was not idempotent: before ledger/movements/reservations/allocations=%d/%d/%d/%d after=%d/%d/%d/%d", len(before.PersonalPoints.WalletLedger), len(before.PersonalPoints.Movements), len(before.PersonalPoints.Reservations), len(before.PersonalPoints.Allocations), len(after.PersonalPoints.WalletLedger), len(after.PersonalPoints.Movements), len(after.PersonalPoints.Reservations), len(after.PersonalPoints.Allocations))
	}
}

func TestLegacyFrozenGenerationV0CancelsAtomically(t *testing.T) {
	store, taskID := seedLegacyFrozenGeneration(t, 0, false)
	backend := &countingStateBackend{delegate: store.backend}
	store.backend = backend

	cancelled, err := store.CancelGenerationTaskForUser("legacy-user", taskID)
	if err != nil {
		t.Fatalf("cancel legacy frozen generation: %v", err)
	}
	if cancelled.Status != "CANCELLED" || cancelled.TaskStatus != taskStatusCancelled {
		t.Fatalf("cancelled task status=%q taskStatus=%q", cancelled.Status, cancelled.TaskStatus)
	}
	assertGenerationPersonalLotMarkers(t, cancelled)
	assertLegacyFrozenTerminalState(t, store, taskID, "RELEASED", 10, 0, 0, 3)
	if backend.writes != 1 {
		t.Fatalf("cancel persisted %d documents, want one atomic write", backend.writes)
	}
	beforeReplay := loadPlatformDataForTest(t, store)
	replayed, err := store.CancelGenerationTaskForUser("legacy-user", taskID)
	if err != nil {
		t.Fatalf("replay cancel legacy frozen generation: %v", err)
	}
	if replayed.Status != "CANCELLED" || replayed.TaskStatus != taskStatusCancelled {
		t.Fatalf("replayed cancelled task status=%q taskStatus=%q", replayed.Status, replayed.TaskStatus)
	}
	afterReplay := loadPlatformDataForTest(t, store)
	if len(afterReplay.PersonalPoints.WalletLedger) != len(beforeReplay.PersonalPoints.WalletLedger) || len(afterReplay.PersonalPoints.Movements) != len(beforeReplay.PersonalPoints.Movements) || len(afterReplay.PersonalPoints.Reservations) != len(beforeReplay.PersonalPoints.Reservations) || len(afterReplay.PersonalPoints.Allocations) != len(beforeReplay.PersonalPoints.Allocations) {
		t.Fatalf("cancel replay duplicated point state: before ledger/movements/reservations/allocations=%d/%d/%d/%d after=%d/%d/%d/%d", len(beforeReplay.PersonalPoints.WalletLedger), len(beforeReplay.PersonalPoints.Movements), len(beforeReplay.PersonalPoints.Reservations), len(beforeReplay.PersonalPoints.Allocations), len(afterReplay.PersonalPoints.WalletLedger), len(afterReplay.PersonalPoints.Movements), len(afterReplay.PersonalPoints.Reservations), len(afterReplay.PersonalPoints.Allocations))
	}
}

func TestLegacyFrozenGenerationV1UpgradesBeforeTerminalMutation(t *testing.T) {
	store, taskID := seedLegacyFrozenGeneration(t, 1, false)

	failed, err := store.FailGenerationTask(taskID, "provider failed")
	if err != nil {
		t.Fatalf("fail v1 legacy frozen generation: %v", err)
	}
	assertGenerationPersonalLotMarkers(t, failed)
	assertLegacyFrozenTerminalState(t, store, taskID, "RELEASED", 10, 0, 0, 3)
}

func TestLegacyFrozenGenerationAmbiguousReserveEvidenceFailsAtomically(t *testing.T) {
	store, taskID := seedLegacyFrozenGeneration(t, 0, true)
	before, err := store.backend.Read()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.FailGenerationTask(taskID, "provider failed"); !errors.Is(err, ErrPersonalPointImportConflict) {
		t.Fatalf("ambiguous legacy reserve error=%v, want import conflict", err)
	}
	after, err := store.backend.Read()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("ambiguous legacy reserve changed the platform document")
	}
}

func TestLegacyFrozenGenerationTerminalLedgerEvidenceFailsAtomically(t *testing.T) {
	store, taskID := seedLegacyFrozenGeneration(t, 0, false)
	if err := store.update(func(data *platformData) error {
		data.WalletLedger = append(data.WalletLedger, walletLedgerEntry{
			ID: "legacy-capture", AccountID: "legacy-account", UserID: "legacy-user", TaskID: taskID,
			EntryType: "CAPTURE", Points: 3, AvailableBefore: 7, AvailableAfter: 7, FrozenBefore: 3, FrozenAfter: 0,
			IdempotencyKey: taskID + ":CAPTURE", ReferenceType: "GENERATION_TASK", ReferenceID: taskID, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before, err := store.backend.Read()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.FailGenerationTask(taskID, "provider failed"); !errors.Is(err, ErrPersonalPointImportConflict) {
		t.Fatalf("terminal legacy evidence error=%v, want import conflict", err)
	}
	after, err := store.backend.Read()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("terminal legacy evidence changed the platform document")
	}
}

func TestLegacyFrozenGenerationRejectsGiftLotAttribution(t *testing.T) {
	store, taskID := seedLegacyFrozenGeneration(t, 1, false)
	now := time.Now().UTC().Add(-time.Hour)
	if err := store.update(func(data *platformData) error {
		data.PersonalPoints.Lots = []PersonalPointLot{
			{ID: "gift-lot", AccountID: "legacy-account", UserID: "legacy-user", SourceType: PointSourceRegistrationGift, ReferenceType: "REGISTRATION", ReferenceID: "gift", OriginalPoints: 3, ReservedPoints: 3, GrantedAt: now, IdempotencyKey: "gift", Status: "ACTIVE"},
			{ID: "legacy-lot", AccountID: "legacy-account", UserID: "legacy-user", SourceType: PointSourceLegacy, ReferenceType: "LEGACY_IMPORT", ReferenceID: "legacy-account", OriginalPoints: 7, AvailablePoints: 7, GrantedAt: now, IdempotencyKey: "legacy-import:legacy-account", Status: "LEGACY"},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before, err := store.backend.Read()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.FailGenerationTask(taskID, "provider failed"); !errors.Is(err, ErrPersonalPointImportConflict) {
		t.Fatalf("gift lot attribution error=%v, want import conflict", err)
	}
	after, err := store.backend.Read()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("gift lot attribution failure changed the platform document")
	}
}

func TestLegacyFrozenGenerationScopeMismatchFailsAtomically(t *testing.T) {
	tests := []struct {
		name               string
		billingAccountType string
		billingScope       string
	}{
		{name: "conflicting scopes", billingAccountType: contextPersonal, billingScope: contextEnterprise},
		{name: "unknown scope", billingAccountType: "AGENT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, taskID := seedLegacyFrozenGeneration(t, 0, false)
			if err := store.update(func(data *platformData) error {
				for i := range data.GenerationTasks {
					if data.GenerationTasks[i].ID != taskID {
						continue
					}
					data.GenerationTasks[i].BillingAccountType = tt.billingAccountType
					if tt.billingScope != "" {
						data.GenerationTasks[i].Params["billing_scope"] = tt.billingScope
					}
					return nil
				}
				return errors.New("task missing")
			}); err != nil {
				t.Fatal(err)
			}
			before, err := store.backend.Read()
			if err != nil {
				t.Fatal(err)
			}

			if _, err := store.FailGenerationTask(taskID, "provider failed"); !errors.Is(err, ErrPersonalPointContextMismatch) {
				t.Fatalf("scope mismatch error=%v, want context mismatch", err)
			}
			after, err := store.backend.Read()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("scope mismatch changed the platform document")
			}
		})
	}
}

func TestPersonalGenerationReservationDriftFailsAtomically(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*platformData, string) error
	}{
		{
			name: "task amount differs from reservation",
			mutate: func(data *platformData, taskID string) error {
				return mutateGenerationReservationFixture(data, taskID, false)
			},
		},
		{
			name: "partially captured reservation",
			mutate: func(data *platformData, taskID string) error {
				return mutateGenerationReservationFixture(data, taskID, true)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "store.json")
			writeGenerationBillingPointSeed(t, path, "drift-user", 3)
			store := newJSONStore(path)
			task, err := store.CreatePendingGenerationTask(generationBillingTestRequest("drift-user", 3))
			if err != nil {
				t.Fatal(err)
			}
			if err := store.update(func(data *platformData) error { return tt.mutate(data, task.ID) }); err != nil {
				t.Fatal(err)
			}
			before, err := store.backend.Read()
			if err != nil {
				t.Fatal(err)
			}

			if _, err := store.CompleteGenerationTask(task.ID, createGenerationTaskRequest{}); !errors.Is(err, ErrPersonalPointImportConflict) {
				t.Fatalf("reservation drift complete error=%v, want import conflict", err)
			}
			after, err := store.backend.Read()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("reservation drift changed the platform document")
			}
		})
	}
}

func mutateGenerationReservationFixture(data *platformData, taskID string, partial bool) error {
	for i := range data.GenerationTasks {
		if data.GenerationTasks[i].ID != taskID {
			continue
		}
		data.GenerationTasks[i].PointCost = 2
		data.GenerationTasks[i].ReservedPoints = 2
		data.GenerationTasks[i].Params[generationBillingReservationPointCostKey] = 2
		if !partial {
			return nil
		}
		reservationID := data.GenerationTasks[i].PersonalPointReservationID
		for reservationIndex := range data.PersonalPoints.Reservations {
			if data.PersonalPoints.Reservations[reservationIndex].ID != reservationID {
				continue
			}
			data.PersonalPoints.Reservations[reservationIndex].ReservedPoints = 2
			data.PersonalPoints.Reservations[reservationIndex].CapturedPoints = 1
		}
		for allocationIndex := range data.PersonalPoints.Allocations {
			if data.PersonalPoints.Allocations[allocationIndex].ReservationID != reservationID {
				continue
			}
			data.PersonalPoints.Allocations[allocationIndex].ReservedPoints = 2
			data.PersonalPoints.Allocations[allocationIndex].CapturedPoints = 1
		}
		return nil
	}
	return errors.New("task missing")
}

func TestPersonalGenerationWithoutCompleteLotMarkersFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	writeGenerationBillingPointSeed(t, path, "marker-user", 3)
	store := newJSONStore(path)
	task, err := store.CreatePendingGenerationTask(generationBillingTestRequest("marker-user", 1))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.update(func(data *platformData) error {
		data.PersonalPointImport.Version = 2
		for i := range data.GenerationTasks {
			if data.GenerationTasks[i].ID == task.ID {
				data.GenerationTasks[i].BillingEngine = ""
				return nil
			}
		}
		return errors.New("task missing")
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := store.CompleteGenerationTask(task.ID, createGenerationTaskRequest{}); !errors.Is(err, ErrPersonalPointReservationMarkerMissing) {
		t.Fatalf("incomplete personal lot marker complete error=%v", err)
	}
	if _, err := store.FailGenerationTask(task.ID, "provider failed"); !errors.Is(err, ErrPersonalPointReservationMarkerMissing) {
		t.Fatalf("incomplete personal lot marker fail error=%v", err)
	}
}

func TestPersonalPointProjectionReplayPreservesGenerationMarkers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	writeGenerationBillingPointSeed(t, path, "projection-user", 3)
	store := newJSONStore(path)
	task, err := store.CreatePendingGenerationTask(generationBillingTestRequest("projection-user", 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PersonalPointService().GetBalance(context.Background(), task.PersonalPointAccountID, task.UserID); err != nil {
		t.Fatal(err)
	}
	stored := generationBillingTaskByID(t, loadPlatformDataForTest(t, store).GenerationTasks, task.ID)
	if stored.BillingEngine != task.BillingEngine || stored.PersonalPointAccountID != task.PersonalPointAccountID || stored.PersonalPointReservationID != task.PersonalPointReservationID {
		t.Fatalf("projection replay changed task markers: before=%+v after=%+v", task, stored)
	}
}

func seedLegacyFrozenGeneration(t *testing.T, importVersion int, duplicateReserve bool) (*jsonStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "store.json")
	store := newJSONStore(path)
	now := time.Now().UTC().Add(-time.Minute)
	taskID := "legacy-task"
	params := generationBillingReservationParams(map[string]any{"count": float64(1)}, now.Format(time.RFC3339Nano), 3, 10, 7)
	task := generationTask{
		ID: taskID, UserID: "legacy-user", Type: "TEXT_TO_IMAGE", Model: "mock-standard", Status: "PROCESSING",
		TaskStatus: taskStatusRunning, BillingStatus: billingStatusReserved, PointCost: 3, QuotedPoints: 3, ReservedPoints: 3,
		Prompt: "legacy frozen generation", Params: params, CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano),
	}
	legacyReserve := walletLedgerEntry{
		ID: "legacy-reserve", AccountID: "legacy-account", UserID: task.UserID, TaskID: task.ID, EntryType: "RESERVE", Points: 3,
		AvailableBefore: 10, AvailableAfter: 7, FrozenBefore: 0, FrozenAfter: 3, IdempotencyKey: task.ID + ":RESERVE",
		ReferenceType: "GENERATION_TASK", ReferenceID: task.ID, Metadata: map[string]any{"source": "legacy_generation"}, CreatedAt: now.Format(time.RFC3339Nano),
	}
	data := platformData{
		PointAccounts:   []adminPointAccount{{ID: "legacy-account", UserID: task.UserID, Available: 7, Frozen: 3, TotalGranted: 10}},
		WalletLedger:    []walletLedgerEntry{legacyReserve},
		GenerationTasks: []generationTask{task},
		Counters:        map[string]int{},
	}
	if duplicateReserve {
		duplicate := legacyReserve
		duplicate.ID = "legacy-reserve-duplicate"
		duplicate.IdempotencyKey = task.ID + ":RESERVE:DUPLICATE"
		data.WalletLedger = append(data.WalletLedger, duplicate)
	}
	if importVersion == 1 {
		state := personalPointState{
			Accounts: []PersonalPointAccount{{ID: "legacy-account", UserID: task.UserID, AvailablePoints: 7, FrozenPoints: 3, TotalGranted: 10}},
			Lots:     []PersonalPointLot{{ID: "legacy-lot", AccountID: "legacy-account", UserID: task.UserID, SourceType: PointSourceLegacy, ReferenceType: "LEGACY_IMPORT", ReferenceID: "legacy-account", OriginalPoints: 10, AvailablePoints: 7, ReservedPoints: 3, GrantedAt: now, IdempotencyKey: "legacy-import:legacy-account", Status: "LEGACY"}},
		}
		normalizePersonalPointState(&state)
		data.PersonalPoints = state
		data.PersonalPointImport = personalPointImportState{Version: 1, ImportedAt: now}
	}
	if err := store.save(data); err != nil {
		t.Fatal(err)
	}
	return store, taskID
}

func assertGenerationPersonalLotMarkers(t *testing.T, task generationTask) {
	t.Helper()
	if task.BillingEngine != personalLotBillingEngine || task.PersonalPointAccountID == "" || task.PersonalPointReservationID == "" {
		t.Fatalf("personal lot markers=%q/%q/%q", task.BillingEngine, task.PersonalPointAccountID, task.PersonalPointReservationID)
	}
}

func assertLegacyFrozenTerminalState(t *testing.T, store *jsonStore, taskID, reservationStatus string, available, frozen, captured, released int64) {
	t.Helper()
	data := loadPlatformDataForTest(t, store)
	if data.PersonalPointImport.Version != 2 {
		t.Fatalf("personal point import version=%d, want 2", data.PersonalPointImport.Version)
	}
	task := generationBillingTaskByID(t, data.GenerationTasks, taskID)
	assertGenerationPersonalLotMarkers(t, task)
	if len(data.PersonalPoints.Accounts) != 1 || data.PersonalPoints.Accounts[0].AvailablePoints != available || data.PersonalPoints.Accounts[0].FrozenPoints != frozen {
		t.Fatalf("terminal personal account=%+v, want available=%d frozen=%d", data.PersonalPoints.Accounts, available, frozen)
	}
	if len(data.PersonalPoints.Reservations) != 1 {
		t.Fatalf("terminal reservations=%+v, want one", data.PersonalPoints.Reservations)
	}
	reservation := data.PersonalPoints.Reservations[0]
	if reservation.ID != task.PersonalPointReservationID || reservation.Status != reservationStatus || reservation.CapturedPoints != captured || reservation.ReleasedPoints != released || reservation.ReservedPoints != 0 {
		t.Fatalf("terminal reservation=%+v", reservation)
	}
	if len(data.PersonalPoints.Lots) != 1 {
		t.Fatalf("terminal lots=%+v, want one", data.PersonalPoints.Lots)
	}
	lot := data.PersonalPoints.Lots[0]
	if lot.AvailablePoints != available || lot.ReservedPoints != 0 || lot.ConsumedPoints != captured || lot.ExpiredPoints != 0 || lot.ReversedPoints != 0 {
		t.Fatalf("terminal lot=%+v, want available/reserved/consumed/expired/reversed=%d/0/%d/0/0", lot, available, captured)
	}
	if len(data.PersonalPoints.Allocations) != 1 {
		t.Fatalf("terminal allocations=%+v, want one", data.PersonalPoints.Allocations)
	}
	allocation := data.PersonalPoints.Allocations[0]
	if allocation.ReservationID != reservation.ID || allocation.LotID != lot.ID || allocation.ReservedPoints != 0 || allocation.CapturedPoints != captured || allocation.ReleasedPoints != released || allocation.ExpiredPoints != 0 || allocation.Status != reservationStatus {
		t.Fatalf("terminal allocation=%+v", allocation)
	}
	if len(data.PointAccounts) != 1 || int64(data.PointAccounts[0].Available) != available || int64(data.PointAccounts[0].Frozen) != frozen || data.PointAccounts[0].ID != task.PersonalPointAccountID || data.PointAccounts[0].UserID != task.UserID {
		t.Fatalf("terminal point account projection=%+v", data.PointAccounts)
	}
	entryType := "RELEASE"
	beforeAvailable, afterAvailable := int64(7), int64(10)
	if captured > 0 {
		entryType = "CAPTURE"
		afterAvailable = 7
	}
	foundLedger := false
	for _, entry := range data.PersonalPoints.WalletLedger {
		if entry.EntryType != entryType || entry.ReferenceType != "GENERATION_TASK" || entry.ReferenceID != taskID {
			continue
		}
		foundLedger = true
		if entry.Points != 3 || entry.AvailableBefore != beforeAvailable || entry.AvailableAfter != afterAvailable || entry.FrozenBefore != 3 || entry.FrozenAfter != 0 {
			t.Fatalf("terminal wallet ledger entry=%+v", entry)
		}
	}
	if !foundLedger {
		t.Fatalf("terminal %s wallet ledger missing: %+v", entryType, data.PersonalPoints.WalletLedger)
	}
	foundMovement := false
	for _, movement := range data.PersonalPoints.Movements {
		if movement.ReservationID != reservation.ID || movement.MovementType != entryType {
			continue
		}
		foundMovement = true
		if movement.Points != 3 || movement.LotID != lot.ID || movement.ReservedBefore != 3 || movement.ReservedAfter != 0 {
			t.Fatalf("terminal lot movement=%+v", movement)
		}
	}
	if !foundMovement {
		t.Fatalf("terminal %s movement missing: %+v", entryType, data.PersonalPoints.Movements)
	}
}

func loadPlatformDataForTest(t *testing.T, store *jsonStore) platformData {
	t.Helper()
	data, err := store.load()
	if err != nil {
		t.Fatal(err)
	}
	return data
}
