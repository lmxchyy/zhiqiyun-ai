package httpserver

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestPersonalPointsExpiryDoesNotTouchReservedAndCaptureAfterDeadline(t *testing.T) {
	ctx := context.Background()
	store := NewJSONPersonalPointStore(filepath.Join(t.TempDir(), "points.json"))
	service := NewPersonalPointService(store)
	grantAt := time.Date(2024, 1, 31, 15, 45, 0, 0, time.UTC)
	if _, err := service.Grant(ctx, PersonalPointGrantCommand{AccountID: "expiry-reserved", UserID: "expiry-user", Source: PointSourceRegistrationGift, Points: 4, IdempotencyKey: "gift", GrantedAt: grantAt}); err != nil {
		t.Fatal(err)
	}
	reservation, err := service.Reserve(ctx, PersonalPointReserveCommand{AccountID: "expiry-reserved", UserID: "expiry-user", BusinessType: "IMAGE", BusinessID: "reserved-task", RequestedPoints: 4, IdempotencyKey: "reserve", ReservedAt: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Expire(ctx, PersonalPointExpiryCommand{AccountID: "expiry-reserved", UserID: "expiry-user", Now: time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	captured, err := service.Capture(ctx, PersonalPointCaptureCommand{AccountID: "expiry-reserved", UserID: "expiry-user", ReservationID: reservation.Reservation.ID, Points: 4, IdempotencyKey: "capture-after-expiry", CapturedAt: time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("reserved points should remain capturable after lot deadline: %v", err)
	}
	if captured.Reservation.CapturedPoints != 4 || captured.Reservation.ExpiredPoints != 0 {
		t.Fatalf("capture after deadline = %+v", captured.Reservation)
	}
	state, err := store.readState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Movements) != 3 || state.Movements[2].MovementType != "CAPTURE" {
		t.Fatalf("expire touched reserved allocation: movements=%+v", state.Movements)
	}
}

func TestPersonalPointsReleaseAfterDeadlineThenExpiresSameLot(t *testing.T) {
	ctx := context.Background()
	store := NewJSONPersonalPointStore(filepath.Join(t.TempDir(), "points.json"))
	service := NewPersonalPointService(store)
	grantAt := time.Date(2024, 1, 31, 15, 45, 0, 0, time.UTC)
	if _, err := service.Grant(ctx, PersonalPointGrantCommand{AccountID: "release-expiry", UserID: "release-user", Source: PointSourceRegistrationGift, Points: 4, IdempotencyKey: "gift", GrantedAt: grantAt}); err != nil {
		t.Fatal(err)
	}
	reservation, err := service.Reserve(ctx, PersonalPointReserveCommand{AccountID: "release-expiry", UserID: "release-user", BusinessType: "IMAGE", BusinessID: "release-task", RequestedPoints: 4, IdempotencyKey: "reserve", ReservedAt: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Release(ctx, PersonalPointReleaseCommand{AccountID: "release-expiry", UserID: "release-user", ReservationID: reservation.Reservation.ID, IdempotencyKey: "release-after-expiry", ReleasedAt: time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	state, err := store.readState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Lots) != 1 || state.Lots[0].AvailablePoints != 0 || state.Lots[0].ReservedPoints != 0 || state.Lots[0].ExpiredPoints != 4 || state.Lots[0].Status != "EXPIRED" {
		t.Fatalf("release-after-expiry lot = %+v", state.Lots)
	}
	if len(state.Movements) != 4 || state.Movements[2].MovementType != "RELEASE" || state.Movements[3].MovementType != "EXPIRE" {
		t.Fatalf("release-after-expiry movements = %+v", state.Movements)
	}
	balance, err := service.GetBalance(ctx, "release-expiry", "release-user")
	if err != nil {
		t.Fatal(err)
	}
	if balance.Available != 0 || balance.Frozen != 0 {
		t.Fatalf("release-after-expiry balance = %+v", balance)
	}
}

func TestPersonalPointsLegacySourceFailsClosed(t *testing.T) {
	service, _ := newPersonalPointTestService(t)
	_, err := service.Grant(context.Background(), PersonalPointGrantCommand{AccountID: "legacy", UserID: "legacy-user", Source: PointSourceLegacy, Points: 1, IdempotencyKey: "legacy-grant"})
	if !errors.Is(err, ErrUnknownPointSource) {
		t.Fatalf("legacy grant error = %v, want fail closed", err)
	}
}

func TestPersonalPointsPolicySelectsOnlyCurrentlyEffectivePublishedCalendarMonth(t *testing.T) {
	ctx := context.Background()
	store := NewJSONPersonalPointStore(filepath.Join(t.TempDir(), "points.json"))
	service := NewPersonalPointService(store)
	now := time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)
	if err := store.SetPolicy(PointExpiryPolicy{ID: "future", Version: 2, Revision: 1, Enabled: true, DurationValue: 9, DurationUnit: "CALENDAR_MONTH", TimeZone: "Asia/Shanghai", SourceTypes: []string{string(PointSourceRegistrationGift)}, Status: "PUBLISHED", EffectiveFrom: now.Add(24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	result, err := service.Grant(ctx, PersonalPointGrantCommand{AccountID: "policy-effective", UserID: "policy-user", Source: PointSourceRegistrationGift, Points: 1, IdempotencyKey: "future-policy", GrantedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if result.Lot.PolicySnapshot.Version != 1 {
		t.Fatalf("future policy was selected: %+v", result.Lot.PolicySnapshot)
	}
	if err := store.SetPolicy(PointExpiryPolicy{ID: "invalid-day", Version: 3, Revision: 1, Enabled: true, DurationValue: 1, DurationUnit: "DAY", TimeZone: "Asia/Shanghai", SourceTypes: []string{string(PointSourceActivityGift)}, Status: "PUBLISHED", EffectiveFrom: now.Add(-time.Hour)}); !errors.Is(err, ErrInvalidPointCommand) {
		t.Fatalf("DAY policy set error = %v, want fail closed", err)
	}
}

func TestPersonalPointsJsonStatusAndPermanentUseExpiryOnly(t *testing.T) {
	ctx := context.Background()
	store := NewJSONPersonalPointStore(filepath.Join(t.TempDir(), "points.json"))
	service := NewPersonalPointService(store)
	if err := store.SetPolicy(PointExpiryPolicy{ID: "disabled", Version: 2, Revision: 1, Enabled: false, DurationValue: 1, DurationUnit: "CALENDAR_MONTH", TimeZone: "Asia/Shanghai", SourceTypes: []string{string(PointSourceRegistrationGift)}, Status: "PUBLISHED", EffectiveFrom: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	grant, err := service.Grant(ctx, PersonalPointGrantCommand{AccountID: "status", UserID: "status-user", Source: PointSourceRegistrationGift, Points: 2, IdempotencyKey: "disabled-gift"})
	if err != nil {
		t.Fatal(err)
	}
	if !grant.Lot.Permanent() || !grant.Lot.ExpiresAt.IsZero() {
		t.Fatalf("disabled gift is not permanent: %+v", grant.Lot)
	}
	reservation, err := service.Reserve(ctx, PersonalPointReserveCommand{AccountID: "status", UserID: "status-user", BusinessType: "IMAGE", BusinessID: "status-task", RequestedPoints: 2, IdempotencyKey: "status-reserve"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Capture(ctx, PersonalPointCaptureCommand{AccountID: "status", UserID: "status-user", ReservationID: reservation.Reservation.ID, Points: 2, IdempotencyKey: "status-capture"}); err != nil {
		t.Fatal(err)
	}
	state, err := store.readState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.Lots[0].Status != "EXHAUSTED" {
		t.Fatalf("JSON lot status = %q, want EXHAUSTED", state.Lots[0].Status)
	}
}

func TestJsonStorePersonalPointServiceReusesRepositoryAndImportsLegacyBalance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "platform.json")
	store := newJSONStore(path)
	if err := store.update(func(data *platformData) error {
		data.PointAccounts = []adminPointAccount{{ID: "points-import", UserID: "import-user", Available: 7, Frozen: 0}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	first := store.PersonalPointService()
	second := store.PersonalPointService()
	if first == nil || second == nil || first.repo != second.repo {
		t.Fatal("PersonalPointService did not reuse the shared repository")
	}
	balance, err := first.GetBalance(context.Background(), "points-import", "import-user")
	if err != nil {
		t.Fatal(err)
	}
	if balance.Available != 7 || balance.Frozen != 0 {
		t.Fatalf("imported legacy balance = %+v", balance)
	}
	state, err := second.repo.(*JSONPersonalPointStore).readState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Lots) != 1 || state.Lots[0].SourceType != PointSourceLegacy || state.Lots[0].OriginalPoints != 7 || !state.Lots[0].Permanent() {
		t.Fatalf("legacy import lots = %+v", state.Lots)
	}
}
