package httpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestJsonStorePersonalPointServiceMigratesLegacyPlatformWalletLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "platform.json")
	store := newJSONStore(path)
	if err := store.update(func(data *platformData) error {
		data.PointAccounts = []adminPointAccount{{ID: "legacy-wallet-account", UserID: "legacy-wallet-user", Available: 7}}
		data.WalletLedger = []walletLedgerEntry{{
			ID: "legacy-ledger-1", AccountID: "legacy-wallet-account", UserID: "legacy-wallet-user",
			EntryType: "GRANT", Points: 7, AvailableBefore: 0, AvailableAfter: 7, FrozenBefore: 0, FrozenAfter: 0,
			IdempotencyKey: "same-old-key", ReferenceType: "ORDER", ReferenceID: "legacy-ref",
			Remark: "old audit", Metadata: map[string]any{"source": "legacy", "business": "import"}, CreatedAt: "2026-08-04T00:00:00Z",
		}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	service := store.PersonalPointService()
	if _, err := service.GetBalance(context.Background(), "legacy-wallet-account", "legacy-wallet-user"); err != nil {
		t.Fatal(err)
	}
	state, err := service.repo.(*JSONPersonalPointStore).readState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range state.WalletLedger {
		if entry.ReferenceID == "legacy-ref" && entry.EntryType == "GRANT" {
			found = true
			if entry.AccountID != "legacy-wallet-account" || entry.UserID != "legacy-wallet-user" || entry.Points != 7 || entry.AvailableBefore != 0 || entry.AvailableAfter != 7 || entry.FrozenBefore != 0 || entry.FrozenAfter != 0 {
				t.Fatalf("migrated legacy ledger transition = %+v", entry)
			}
			if !strings.Contains(entry.IdempotencyKey, "legacy-wallet-account") || !strings.Contains(entry.IdempotencyKey, "same-old-key") {
				t.Fatalf("migrated legacy ledger key = %q", entry.IdempotencyKey)
			}
			if entry.Metadata["source"] != "legacy" || entry.Metadata["business"] != "import" {
				t.Fatalf("migrated legacy metadata = %+v", entry.Metadata)
			}
			if entry.OccurredAt.Format(time.RFC3339Nano) != "2026-08-04T00:00:00Z" || !entry.CreatedAt.Equal(entry.OccurredAt) {
				t.Fatalf("migrated legacy timestamps = occurred=%s created=%s", entry.OccurredAt, entry.CreatedAt)
			}
		}
	}
	if !found {
		t.Fatalf("legacy platform wallet ledger was not migrated: %+v", state.WalletLedger)
	}
	legacyImports := 0
	for _, entry := range state.WalletLedger {
		if entry.AccountID == "legacy-wallet-account" && entry.ReferenceType == "LEGACY_IMPORT" {
			legacyImports++
		}
	}
	if legacyImports != 1 {
		t.Fatalf("legacy synthetic grant was duplicated or lost: %d entries", legacyImports)
	}
}

func TestJsonStorePersonalPointServiceMigratesLegacyWalletLedgerAcrossAccountsAndRetries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "platform.json")
	store := newJSONStore(path)
	if err := store.update(func(data *platformData) error {
		data.PointAccounts = []adminPointAccount{
			{ID: "legacy-account-a", UserID: "legacy-user-a", Available: 1},
			{ID: "legacy-account-b", UserID: "legacy-user-b", Available: 1},
		}
		data.WalletLedger = []walletLedgerEntry{
			{ID: "legacy-ledger-a", AccountID: "legacy-account-a", UserID: "legacy-user-a", EntryType: "GRANT", Points: 1, AvailableBefore: 0, AvailableAfter: 1, IdempotencyKey: "shared-key", ReferenceType: "ORDER", ReferenceID: "ref-a", Metadata: map[string]any{"account": "a"}, CreatedAt: "2026-08-04T01:00:00Z"},
			{ID: "legacy-ledger-b", AccountID: "legacy-account-b", UserID: "legacy-user-b", EntryType: "GRANT", Points: 1, AvailableBefore: 0, AvailableAfter: 1, IdempotencyKey: "shared-key", ReferenceType: "ORDER", ReferenceID: "ref-b", Metadata: map[string]any{"account": "b"}, CreatedAt: "2026-08-04T01:00:01Z"},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	first := store.PersonalPointService()
	if _, err := first.GetBalance(context.Background(), "legacy-account-a", "legacy-user-a"); err != nil {
		t.Fatal(err)
	}
	state, err := first.repo.(*JSONPersonalPointStore).readState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]string{}
	for _, entry := range state.WalletLedger {
		if entry.ReferenceID == "ref-a" || entry.ReferenceID == "ref-b" {
			keys[entry.ReferenceID] = entry.IdempotencyKey
		}
	}
	if len(keys) != 2 || keys["ref-a"] == keys["ref-b"] || !strings.Contains(keys["ref-a"], "legacy-account-a") || !strings.Contains(keys["ref-b"], "legacy-account-b") {
		t.Fatalf("cross-account migrated keys = %+v", keys)
	}
	beforeCount := len(state.WalletLedger)
	second := newJSONStore(path).PersonalPointService()
	if _, err := second.GetBalance(context.Background(), "legacy-account-b", "legacy-user-b"); err != nil {
		t.Fatal(err)
	}
	state, err = second.repo.(*JSONPersonalPointStore).readState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(state.WalletLedger) != beforeCount {
		t.Fatalf("repeated migration changed ledger count from %d to %d: %+v", beforeCount, len(state.WalletLedger), state.WalletLedger)
	}
}

func TestJsonStorePersonalPointServiceRejectsInvalidLegacyWalletLedgerAtomically(t *testing.T) {
	cases := []struct {
		name  string
		field string
		value string
	}{
		{name: "fractional", field: "points", value: "1.5"},
		{name: "negative", field: "availableBefore", value: "-1"},
		{name: "overflow", field: "points", value: "9223372036854775808"},
		{name: "nan", field: "points", value: "NaN"},
		{name: "infinity", field: "points", value: "Infinity"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "platform.json")
			sidecarPath := path + ".personal-points.json"
			sidecarBefore := []byte(`{"accounts":[],"wallet_ledger":[]}`)
			if err := os.WriteFile(sidecarPath, sidecarBefore, 0o600); err != nil {
				t.Fatal(err)
			}
			platform := `{"pointAccounts":[{"id":"bad-account","userId":"bad-user","available":1}],"walletLedger":[{"id":"bad-ledger","accountId":"bad-account","userId":"bad-user","entryType":"GRANT","points":POINTS,"availableBefore":0,"availableAfter":1,"frozenBefore":0,"frozenAfter":0,"idempotencyKey":"bad-key","referenceType":"ORDER","referenceId":"bad-ref","metadata":{"keep":"me"},"createdAt":"2026-08-04T00:00:00Z"}]}`
			platform = strings.Replace(platform, "POINTS", tc.value, 1)
			if err := os.WriteFile(path, []byte(platform), 0o600); err != nil {
				t.Fatal(err)
			}
			platformBefore, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			service := newJSONStore(path).PersonalPointService()
			if _, err := service.GetBalance(context.Background(), "bad-account", "bad-user"); err == nil {
				t.Fatal("invalid legacy wallet ledger was accepted")
			}
			platformAfter, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(platformAfter) != string(platformBefore) {
				t.Fatal("invalid migration modified the primary platform JSON")
			}
			sidecarAfter, err := os.ReadFile(sidecarPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(sidecarAfter) != string(sidecarBefore) {
				t.Fatalf("invalid migration partially wrote sidecar: %s", sidecarAfter)
			}
		})
	}
}

func TestJsonStorePersonalPointServiceCompletesPartialLegacyLedgerMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "platform.json")
	store := newJSONStore(path)
	if err := store.update(func(data *platformData) error {
		data.PointAccounts = []adminPointAccount{{ID: "partial-account", UserID: "partial-user", Available: 2}}
		data.WalletLedger = []walletLedgerEntry{{ID: "partial-one", AccountID: "partial-account", UserID: "partial-user", EntryType: "GRANT", Points: 1, AvailableBefore: 0, AvailableAfter: 1, IdempotencyKey: "partial-key-1", ReferenceType: "ORDER", ReferenceID: "partial-ref-1", CreatedAt: "2026-08-04T00:00:00Z"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	first := store.PersonalPointService()
	if _, err := first.GetBalance(context.Background(), "partial-account", "partial-user"); err != nil {
		t.Fatal(err)
	}
	if err := store.update(func(data *platformData) error {
		data.WalletLedger = append(data.WalletLedger, walletLedgerEntry{ID: "partial-two", AccountID: "partial-account", UserID: "partial-user", EntryType: "RESERVE", Points: 1, AvailableBefore: 2, AvailableAfter: 1, FrozenBefore: 0, FrozenAfter: 1, IdempotencyKey: "partial-key-2", ReferenceType: "TASK", ReferenceID: "partial-ref-2", CreatedAt: "2026-08-04T00:00:01Z"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	second := newJSONStore(path).PersonalPointService()
	if _, err := second.GetBalance(context.Background(), "partial-account", "partial-user"); err != nil {
		t.Fatal(err)
	}
	state, err := second.repo.(*JSONPersonalPointStore).readState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	refs := map[string]int{}
	for _, entry := range state.WalletLedger {
		if entry.ReferenceID == "partial-ref-1" || entry.ReferenceID == "partial-ref-2" {
			refs[entry.ReferenceID]++
		}
	}
	if refs["partial-ref-1"] != 1 || refs["partial-ref-2"] != 1 {
		t.Fatalf("partial migration refs = %+v, ledger=%+v", refs, state.WalletLedger)
	}
}
