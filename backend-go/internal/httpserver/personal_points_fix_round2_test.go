package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPersonalPointsJsonRejectsLowercaseCalendarMonthWithoutReplacingPolicy(t *testing.T) {
	ctx := context.Background()
	store := NewJSONPersonalPointStore(filepath.Join(t.TempDir(), "points.json"))
	if err := store.SetPolicy(PointExpiryPolicy{
		ID: "lowercase-policy", Version: 2, Revision: 1, Enabled: true, DurationValue: 9,
		DurationUnit: "calendar_month", TimeZone: "Asia/Shanghai",
		SourceTypes: []string{string(PointSourceRegistrationGift)}, Status: "PUBLISHED",
		EffectiveFrom: time.Now().Add(-time.Hour),
	}); !errors.Is(err, ErrInvalidPointCommand) {
		t.Fatalf("lowercase duration unit error = %v, want fail closed", err)
	}
	result, err := NewPersonalPointService(store).Grant(ctx, PersonalPointGrantCommand{
		AccountID: "lowercase-policy-account", UserID: "lowercase-policy-user",
		Source: PointSourceRegistrationGift, Points: 1, IdempotencyKey: "grant",
		GrantedAt: time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Lot.PolicySnapshot.Version != 1 || result.Lot.PolicySnapshot.DurationUnit != "CALENDAR_MONTH" {
		t.Fatalf("lowercase policy replaced default policy: %+v", result.Lot.PolicySnapshot)
	}
}

func TestPersonalPointsJsonLoadsLegacyStateWithoutWalletLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "points.json")
	if err := os.WriteFile(path, []byte(`{"accounts":[],"lots":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewJSONPersonalPointStore(path)
	state, err := store.readState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.WalletLedger == nil {
		t.Fatal("legacy JSON without wallet_ledger should load as an empty ledger")
	}
	if len(state.WalletLedger) != 0 {
		t.Fatalf("legacy JSON wallet ledger = %+v, want empty", state.WalletLedger)
	}
}

func TestPersonalPointsJsonWalletLedgerCoversMutationsAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	store := NewJSONPersonalPointStore(filepath.Join(t.TempDir(), "points.json"))
	service := NewPersonalPointService(store)
	if _, err := service.Grant(ctx, PersonalPointGrantCommand{
		AccountID: "ledger-capture", UserID: "ledger-user", Source: PointSourceRecharge,
		Points: 10, IdempotencyKey: "same-caller-key", GrantedAt: time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Grant(ctx, PersonalPointGrantCommand{
		AccountID: "ledger-second", UserID: "ledger-user-2", Source: PointSourceRecharge,
		Points: 3, IdempotencyKey: "same-caller-key", GrantedAt: time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	reserved, err := service.Reserve(ctx, PersonalPointReserveCommand{
		AccountID: "ledger-capture", UserID: "ledger-user", BusinessType: "IMAGE", BusinessID: "capture-task",
		RequestedPoints: 4, IdempotencyKey: "reserve", ReservedAt: time.Date(2026, 8, 4, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Reserve(ctx, PersonalPointReserveCommand{
		AccountID: "ledger-capture", UserID: "ledger-user", BusinessType: "IMAGE", BusinessID: "capture-task",
		RequestedPoints: 4, IdempotencyKey: "reserve", ReservedAt: time.Date(2026, 8, 4, 1, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Capture(ctx, PersonalPointCaptureCommand{
		AccountID: "ledger-capture", UserID: "ledger-user", ReservationID: reserved.Reservation.ID,
		Points: 4, IdempotencyKey: "capture", CapturedAt: time.Date(2026, 8, 4, 2, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Capture(ctx, PersonalPointCaptureCommand{
		AccountID: "ledger-capture", UserID: "ledger-user", ReservationID: reserved.Reservation.ID,
		Points: 4, IdempotencyKey: "capture", CapturedAt: time.Date(2026, 8, 4, 2, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := service.Grant(ctx, PersonalPointGrantCommand{
		AccountID: "ledger-release", UserID: "ledger-release-user", Source: PointSourceRegistrationGift,
		Points: 4, IdempotencyKey: "gift", GrantedAt: time.Date(2024, 1, 31, 15, 45, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	releaseReservation, err := service.Reserve(ctx, PersonalPointReserveCommand{
		AccountID: "ledger-release", UserID: "ledger-release-user", BusinessType: "IMAGE", BusinessID: "release-task",
		RequestedPoints: 4, IdempotencyKey: "reserve", ReservedAt: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	releaseCmd := PersonalPointReleaseCommand{
		AccountID: "ledger-release", UserID: "ledger-release-user", ReservationID: releaseReservation.Reservation.ID,
		Points: 4, IdempotencyKey: "release", ReleasedAt: time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC),
	}
	if _, err := service.Release(ctx, releaseCmd); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Release(ctx, releaseCmd); err != nil {
		t.Fatal(err)
	}

	state, err := store.readState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.WalletLedger) != 8 {
		t.Fatalf("wallet ledger length = %d, want 8 (two grants + reserve/capture + grant/reserve/release/expire)", len(state.WalletLedger))
	}
	var releaseEntries []PersonalPointWalletLedgerEntry
	for _, entry := range state.WalletLedger {
		if entry.AccountID == "ledger-release" {
			releaseEntries = append(releaseEntries, entry)
		}
	}
	if len(releaseEntries) != 4 || releaseEntries[2].EntryType != "RELEASE" || releaseEntries[3].EntryType != "EXPIRE" {
		t.Fatalf("cross-period release ledger = %+v, want consecutive RELEASE then EXPIRE", releaseEntries)
	}
	if releaseEntries[2].AvailableBefore != 0 || releaseEntries[2].AvailableAfter != 4 || releaseEntries[2].FrozenBefore != 4 || releaseEntries[2].FrozenAfter != 0 || releaseEntries[2].Points != 4 {
		t.Fatalf("release transition = %+v", releaseEntries[2])
	}
	if releaseEntries[3].AvailableBefore != 4 || releaseEntries[3].AvailableAfter != 0 || releaseEntries[3].FrozenBefore != 0 || releaseEntries[3].FrozenAfter != 0 || releaseEntries[3].Points != 4 {
		t.Fatalf("expire transition = %+v", releaseEntries[3])
	}
	if releaseEntries[2].OccurredAt.After(releaseEntries[3].OccurredAt) {
		t.Fatalf("release/expire occurred_at order = %s then %s", releaseEntries[2].OccurredAt, releaseEntries[3].OccurredAt)
	}
	for _, accountID := range []string{"ledger-capture", "ledger-second", "ledger-release"} {
		seen := map[string]bool{}
		for _, entry := range state.WalletLedger {
			if entry.AccountID == accountID {
				if seen[entry.IdempotencyKey] {
					t.Fatalf("duplicate wallet ledger key for %s: %q", accountID, entry.IdempotencyKey)
				}
				seen[entry.IdempotencyKey] = true
			}
		}
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) == 0 {
		t.Fatal("wallet ledger state should be serializable")
	}
}

func TestPersonalPointsPostgresPreferredDSN(t *testing.T) {
	t.Setenv("PERSONAL_POINTS_POSTGRES_TEST_DSN", "preferred-dsn")
	t.Setenv("XIANZHI_PERSONAL_POINT_TEST_DATABASE_URL", "legacy-dsn")
	if got := personalPointPostgresTestDSN(); got != "preferred-dsn" {
		t.Fatalf("preferred postgres DSN = %q, want preferred-dsn", got)
	}
}
