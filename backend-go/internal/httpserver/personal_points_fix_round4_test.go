package httpserver

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestValidatePersonalWalletTransitionRejectsInvalidPointsAndOverflow(t *testing.T) {
	const maxInt64 = int64(1<<63 - 1)
	cases := []struct {
		name          string
		entryType     string
		points        int64
		availableFrom int64
		availableTo   int64
		frozenFrom    int64
		frozenTo      int64
	}{
		{name: "unknown_type", entryType: "UNKNOWN", points: 1, availableFrom: 0, availableTo: 1, frozenFrom: 0, frozenTo: 0},
		{name: "zero_points", entryType: "GRANT", points: 0, availableFrom: 0, availableTo: 0, frozenFrom: 0, frozenTo: 0},
		{name: "negative_points", entryType: "GRANT", points: -1, availableFrom: 0, availableTo: 0, frozenFrom: 0, frozenTo: 0},
		{name: "negative_available", entryType: "GRANT", points: 1, availableFrom: -1, availableTo: 0, frozenFrom: 0, frozenTo: 0},
		{name: "grant_available_overflow", entryType: "GRANT", points: 1, availableFrom: maxInt64, availableTo: maxInt64, frozenFrom: 0, frozenTo: 0},
		{name: "reserve_frozen_overflow", entryType: "RESERVE", points: 1, availableFrom: 1, availableTo: 0, frozenFrom: maxInt64, frozenTo: maxInt64},
		{name: "release_available_overflow", entryType: "RELEASE", points: 1, availableFrom: maxInt64, availableTo: maxInt64, frozenFrom: 1, frozenTo: 0},
		{name: "adjustment_positive_overflow", entryType: "ADJUSTMENT", points: 1, availableFrom: maxInt64, availableTo: maxInt64, frozenFrom: 0, frozenTo: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validatePersonalWalletTransition(tc.entryType, tc.points, tc.availableFrom, tc.availableTo, tc.frozenFrom, tc.frozenTo); err == nil {
				t.Fatalf("invalid %s wallet transition was accepted", tc.name)
			}
		})
	}
}

func TestJsonStorePersonalPointServiceRejectsInvalidLegacyWalletTransitionsAtomically(t *testing.T) {
	cases := []struct {
		name          string
		entryType     string
		points        float64
		availableFrom float64
		availableTo   float64
		frozenFrom    float64
		frozenTo      float64
	}{
		{name: "grant_non_conserving", entryType: "GRANT", points: 1, availableFrom: 0, availableTo: 999, frozenFrom: 0, frozenTo: 0},
		{name: "reserve_non_conserving", entryType: "RESERVE", points: 2, availableFrom: 5, availableTo: 3, frozenFrom: 0, frozenTo: 0},
		{name: "capture_non_conserving", entryType: "CAPTURE", points: 2, availableFrom: 7, availableTo: 8, frozenFrom: 3, frozenTo: 1},
		{name: "release_non_conserving", entryType: "RELEASE", points: 2, availableFrom: 1, availableTo: 3, frozenFrom: 4, frozenTo: 3},
		{name: "expire_non_conserving", entryType: "EXPIRE", points: 1, availableFrom: 0, availableTo: 1, frozenFrom: 0, frozenTo: 0},
		{name: "refund_non_conserving", entryType: "REFUND", points: 2, availableFrom: 1, availableTo: 1, frozenFrom: 0, frozenTo: 0},
		{name: "adjustment_non_conserving", entryType: "ADJUSTMENT", points: 2, availableFrom: 5, availableTo: 5, frozenFrom: 0, frozenTo: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "platform.json")
			store := newJSONStore(path)
			if err := store.update(func(data *platformData) error {
				data.PointAccounts = []adminPointAccount{{ID: "transition-account", UserID: "transition-user", Available: 1}}
				data.WalletLedger = []walletLedgerEntry{{
					ID: "invalid-transition", AccountID: "transition-account", UserID: "transition-user",
					EntryType: tc.entryType, Points: tc.points, AvailableBefore: tc.availableFrom, AvailableAfter: tc.availableTo,
					FrozenBefore: tc.frozenFrom, FrozenAfter: tc.frozenTo, IdempotencyKey: "invalid-transition-key",
					ReferenceType: "TEST", ReferenceID: tc.name, CreatedAt: "2026-08-04T00:00:00Z",
				}}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			primaryBefore, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			sidecarPath := path + ".personal-points.json"
			sidecarBefore := []byte(`{"accounts":[],"wallet_ledger":[]}`)
			if err := os.WriteFile(sidecarPath, sidecarBefore, 0o600); err != nil {
				t.Fatal(err)
			}

			service := store.PersonalPointService()
			if _, err := service.GetBalance(context.Background(), "transition-account", "transition-user"); err == nil {
				t.Fatalf("invalid legacy %s transition was accepted", tc.entryType)
			}

			primaryAfter, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(primaryAfter) != string(primaryBefore) {
				t.Fatalf("invalid %s migration modified primary platform JSON", tc.entryType)
			}
			sidecarAfter, err := os.ReadFile(sidecarPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(sidecarAfter) != string(sidecarBefore) {
				t.Fatalf("invalid %s migration partially wrote sidecar: %s", tc.entryType, sidecarAfter)
			}
		})
	}
}

func TestJsonStorePersonalPointServiceMigratesValidLegacyRefundAndAdjustments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "platform.json")
	store := newJSONStore(path)
	if err := store.update(func(data *platformData) error {
		data.PointAccounts = []adminPointAccount{{ID: "valid-transition-account", UserID: "valid-transition-user", Available: 1}}
		data.WalletLedger = []walletLedgerEntry{
			{ID: "valid-refund", AccountID: "valid-transition-account", UserID: "valid-transition-user", EntryType: "REFUND", Points: 2, AvailableBefore: 1, AvailableAfter: 3, FrozenBefore: 0, FrozenAfter: 0, IdempotencyKey: "valid-refund-key", ReferenceType: "REFUND", ReferenceID: "refund-ref", CreatedAt: "2026-08-04T00:00:00Z"},
			{ID: "valid-adjustment-positive", AccountID: "valid-transition-account", UserID: "valid-transition-user", EntryType: "ADJUSTMENT", Points: 3, AvailableBefore: 3, AvailableAfter: 6, FrozenBefore: 0, FrozenAfter: 0, IdempotencyKey: "valid-adjustment-positive-key", ReferenceType: "ADJUSTMENT", ReferenceID: "adjustment-positive-ref", CreatedAt: "2026-08-04T00:00:01Z"},
			{ID: "valid-adjustment-negative", AccountID: "valid-transition-account", UserID: "valid-transition-user", EntryType: "ADJUSTMENT", Points: 2, AvailableBefore: 6, AvailableAfter: 4, FrozenBefore: 0, FrozenAfter: 0, IdempotencyKey: "valid-adjustment-negative-key", ReferenceType: "ADJUSTMENT", ReferenceID: "adjustment-negative-ref", CreatedAt: "2026-08-04T00:00:02Z"},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	service := store.PersonalPointService()
	if _, err := service.GetBalance(context.Background(), "valid-transition-account", "valid-transition-user"); err != nil {
		t.Fatal(err)
	}
	state, err := service.repo.(*JSONPersonalPointStore).readState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]PersonalPointWalletLedgerEntry{
		"refund-ref":            {EntryType: "REFUND", Points: 2, AvailableBefore: 1, AvailableAfter: 3, FrozenBefore: 0, FrozenAfter: 0},
		"adjustment-positive-ref": {EntryType: "ADJUSTMENT", Points: 3, AvailableBefore: 3, AvailableAfter: 6, FrozenBefore: 0, FrozenAfter: 0},
		"adjustment-negative-ref": {EntryType: "ADJUSTMENT", Points: 2, AvailableBefore: 6, AvailableAfter: 4, FrozenBefore: 0, FrozenAfter: 0},
	}
	for _, entry := range state.WalletLedger {
		if _, ok := want[entry.ReferenceID]; !ok {
			continue
		}
		if entry.AccountID != "valid-transition-account" || entry.UserID != "valid-transition-user" {
			t.Fatalf("valid migrated transition ownership = %+v", entry)
		}
		delete(want, entry.ReferenceID)
	}
	if len(want) != 0 {
		t.Fatalf("valid refund/adjustment transitions were not migrated: %+v; ledger=%+v", want, state.WalletLedger)
	}
}

func TestJsonStorePersonalPointServiceRejectsLaterInvalidLegacyTransitionAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "platform.json")
	store := newJSONStore(path)
	if err := store.update(func(data *platformData) error {
		data.PointAccounts = []adminPointAccount{{ID: "atomic-transition-account", UserID: "atomic-transition-user", Available: 1}}
		data.WalletLedger = []walletLedgerEntry{
			{ID: "valid-before-invalid", AccountID: "atomic-transition-account", UserID: "atomic-transition-user", EntryType: "GRANT", Points: 1, AvailableBefore: 0, AvailableAfter: 1, FrozenBefore: 0, FrozenAfter: 0, IdempotencyKey: "valid-before-invalid-key", ReferenceType: "TEST", ReferenceID: "valid-before-invalid", CreatedAt: "2026-08-04T00:00:00Z"},
			{ID: "invalid-after-valid", AccountID: "atomic-transition-account", UserID: "atomic-transition-user", EntryType: "REFUND", Points: 1, AvailableBefore: 1, AvailableAfter: 1, FrozenBefore: 0, FrozenAfter: 0, IdempotencyKey: "invalid-after-valid-key", ReferenceType: "TEST", ReferenceID: "invalid-after-valid", CreatedAt: "2026-08-04T00:00:01Z"},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	primaryBefore, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sidecarPath := path + ".personal-points.json"
	sidecarBefore := []byte(`{"accounts":[{"id":"sentinel"}],"wallet_ledger":[]}`)
	if err := os.WriteFile(sidecarPath, sidecarBefore, 0o600); err != nil {
		t.Fatal(err)
	}
	service := store.PersonalPointService()
	if _, err := service.GetBalance(context.Background(), "atomic-transition-account", "atomic-transition-user"); err == nil {
		t.Fatal("later invalid legacy transition was accepted")
	}
	primaryAfter, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(primaryAfter) != string(primaryBefore) {
		t.Fatal("later invalid migration modified primary platform JSON")
	}
	sidecarAfter, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(sidecarAfter) != string(sidecarBefore) {
		t.Fatalf("later invalid migration partially wrote sidecar: %s", sidecarAfter)
	}
}
