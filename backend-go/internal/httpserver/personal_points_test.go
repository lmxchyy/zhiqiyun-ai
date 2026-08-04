package httpserver

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func newPersonalPointTestService(t *testing.T) (*PersonalPointService, *JSONPersonalPointStore) {
	t.Helper()
	store := NewJSONPersonalPointStore(filepath.Join(t.TempDir(), "personal-points.json"))
	return NewPersonalPointService(store), store
}

func TestPersonalPointsMissingAccountReadsZeroAndReserveDoesNotCreate(t *testing.T) {
	ctx := context.Background()
	service, store := newPersonalPointTestService(t)

	balance, err := service.GetBalance(ctx, "missing-account", "user-missing")
	if err != nil {
		t.Fatal(err)
	}
	if balance.Available != 0 || balance.Frozen != 0 || balance.Total != 0 {
		t.Fatalf("missing balance = %+v, want zero", balance)
	}
	if got := store.AccountCount(); got != 0 {
		t.Fatalf("missing account read created %d account(s)", got)
	}

	_, err = service.Reserve(ctx, PersonalPointReserveCommand{
		AccountID: "missing-account", UserID: "user-missing", BusinessType: "IMAGE_GENERATION",
		BusinessID: "task-missing", RequestedPoints: 1, IdempotencyKey: "reserve-missing",
	})
	if !errors.Is(err, ErrInsufficientPoints) {
		t.Fatalf("reserve error = %v, want insufficient points", err)
	}
	if got := store.AccountCount(); got != 0 {
		t.Fatalf("missing account reserve created %d account(s)", got)
	}
}

func TestPersonalPointsRegistrationUsesCurrentPlanGrant(t *testing.T) {
	service, store := newPersonalPointTestService(t)
	plan := adminPlan{ID: "plan_free", GrantPoints: 321, Points: 7, DurationDays: 14, Active: true}

	result, err := service.GrantRegistration(context.Background(), PersonalPointRegistrationGrantCommand{
		AccountID: "account-registration", UserID: "user-registration", PlanID: plan.ID,
		PlanGrantPoints: int64(planPoints(plan)), IdempotencyKey: "registration:user-registration",
		GrantedAt: time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Lot.OriginalPoints != 321 {
		t.Fatalf("registration grant = %d, want current plan grant 321", result.Lot.OriginalPoints)
	}
	if result.Lot.OriginalPoints == 959 {
		t.Fatal("registration grant resurrected removed 959 default")
	}
	if got := store.AccountCount(); got != 1 {
		t.Fatalf("account count = %d, want 1", got)
	}
}

func TestPersonalPointsGiftPolicySnapshotAndMonthEndClamp(t *testing.T) {
	service, _ := newPersonalPointTestService(t)
	grantAt := time.Date(2024, 1, 31, 15, 45, 0, 0, time.UTC)
	result, err := service.Grant(context.Background(), PersonalPointGrantCommand{
		AccountID: "account-gift", UserID: "user-gift", Source: PointSourceRegistrationGift,
		Points: 10, IdempotencyKey: "gift-registration", GrantedAt: grantAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Lot.PolicyVersionID == "" || result.Lot.PolicySnapshot.Version != 1 || !result.Lot.PolicySnapshot.Enabled {
		t.Fatalf("gift policy snapshot = %+v", result.Lot.PolicySnapshot)
	}
	if got, want := result.Lot.ExpiresAt.In(time.FixedZone("CST", 8*60*60)), time.Date(2024, 4, 30, 23, 45, 0, 0, time.FixedZone("CST", 8*60*60)); !got.Equal(want) {
		t.Fatalf("gift expiry = %s, want Asia/Shanghai month-end clamp near %s", got, want)
	}

	for _, source := range []PointSource{PointSourceActivityGift, PointSourceAdminGift} {
		lot, err := service.Grant(context.Background(), PersonalPointGrantCommand{
			AccountID: "account-gift", UserID: "user-gift", Source: source,
			Points: 1, IdempotencyKey: "gift-" + string(source), GrantedAt: grantAt,
		})
		if err != nil {
			t.Fatal(err)
		}
		if lot.Lot.PolicyVersionID == "" || lot.Lot.PolicySnapshot.TimeZone != "Asia/Shanghai" {
			t.Fatalf("%s snapshot = %+v", source, lot.Lot.PolicySnapshot)
		}
	}

	for _, source := range []PointSource{PointSourceRecharge, PointSourceCorrection, PointSourceAdminCorrection} {
		lot, err := service.Grant(context.Background(), PersonalPointGrantCommand{
			AccountID: "account-gift", UserID: "user-gift", Source: source,
			Points: 1, IdempotencyKey: "permanent-" + string(source), GrantedAt: grantAt,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !lot.Lot.Permanent() || lot.Lot.PolicyVersionID != "" || !lot.Lot.ExpiresAt.IsZero() {
			t.Fatalf("%s lot is not permanent: %+v", source, lot.Lot)
		}
	}

	if _, err := service.Grant(context.Background(), PersonalPointGrantCommand{
		AccountID: "account-gift", UserID: "user-gift", Source: PointSource("UNTRUSTED"),
		Points: 1, IdempotencyKey: "unknown-source", GrantedAt: grantAt,
	}); !errors.Is(err, ErrUnknownPointSource) {
		t.Fatalf("unknown source error = %v", err)
	}
}

func TestPersonalPointsGrantIdempotencyAndConflict(t *testing.T) {
	service, _ := newPersonalPointTestService(t)
	cmd := PersonalPointGrantCommand{AccountID: "account-idem", UserID: "user-idem", Source: PointSourceRecharge, Points: 20, IdempotencyKey: "grant-key", GrantedAt: time.Now().UTC()}
	first, err := service.Grant(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Grant(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Idempotent || second.Lot.ID != first.Lot.ID {
		t.Fatalf("duplicate grant = %+v, first = %+v", second, first)
	}
	conflicting := cmd
	conflicting.Points = 21
	if _, err := service.Grant(context.Background(), conflicting); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting grant error = %v", err)
	}
}

func TestPersonalPointsFEFOCaptureReleaseAndLazyExpiry(t *testing.T) {
	service, _ := newPersonalPointTestService(t)
	ctx := context.Background()
	grantAt := time.Now().UTC()
	if _, err := service.Grant(ctx, PersonalPointGrantCommand{AccountID: "account-fefo", UserID: "user-fefo", Source: PointSourceRegistrationGift, Points: 5, IdempotencyKey: "gift-fefo", GrantedAt: grantAt}); err != nil {
		t.Fatal(err)
	}
	permanent, err := service.Grant(ctx, PersonalPointGrantCommand{AccountID: "account-fefo", UserID: "user-fefo", Source: PointSourceRecharge, Points: 7, IdempotencyKey: "paid-fefo", GrantedAt: grantAt})
	if err != nil {
		t.Fatal(err)
	}
	reserve, err := service.Reserve(ctx, PersonalPointReserveCommand{AccountID: "account-fefo", UserID: "user-fefo", BusinessType: "IMAGE_GENERATION", BusinessID: "task-fefo", RequestedPoints: 8, IdempotencyKey: "reserve-fefo", ReservedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if len(reserve.Allocations) != 2 || reserve.Allocations[0].LotID == permanent.Lot.ID {
		t.Fatalf("FEFO allocations = %+v, want gift lot first", reserve.Allocations)
	}

	captured, err := service.Capture(ctx, PersonalPointCaptureCommand{AccountID: "account-fefo", UserID: "user-fefo", ReservationID: reserve.Reservation.ID, Points: 3, IdempotencyKey: "capture-fefo"})
	if err != nil {
		t.Fatal(err)
	}
	if captured.Reservation.CapturedPoints != 3 || captured.Reservation.ReservedPoints != 5 {
		t.Fatalf("partial capture = %+v", captured.Reservation)
	}
	if _, err := service.Capture(ctx, PersonalPointCaptureCommand{AccountID: "account-fefo", UserID: "user-fefo", ReservationID: reserve.Reservation.ID, Points: 3, IdempotencyKey: "capture-fefo"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Release(ctx, PersonalPointReleaseCommand{AccountID: "account-fefo", UserID: "user-fefo", ReservationID: reserve.Reservation.ID, IdempotencyKey: "release-fefo", ReleasedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	balance, err := service.GetBalance(ctx, "account-fefo", "user-fefo")
	if err != nil {
		t.Fatal(err)
	}
	if balance.Available != 9 || balance.Frozen != 0 {
		t.Fatalf("balance after capture/release = %+v, want available 9 frozen 0", balance)
	}

	// A second account proves lazy expiry before reserve and immediate EXPIRE on the old lot.
	if _, err := service.Grant(ctx, PersonalPointGrantCommand{AccountID: "account-expiry", UserID: "user-expiry", Source: PointSourceRegistrationGift, Points: 4, IdempotencyKey: "gift-expiry", GrantedAt: time.Date(2024, 1, 31, 15, 45, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Grant(ctx, PersonalPointGrantCommand{AccountID: "account-expiry", UserID: "user-expiry", Source: PointSourceRecharge, Points: 2, IdempotencyKey: "paid-expiry", GrantedAt: grantAt}); err != nil {
		t.Fatal(err)
	}
	reservation, err := service.Reserve(ctx, PersonalPointReserveCommand{AccountID: "account-expiry", UserID: "user-expiry", BusinessType: "IMAGE_GENERATION", BusinessID: "task-expiry", RequestedPoints: 2, IdempotencyKey: "reserve-expiry", ReservedAt: time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if len(reservation.Allocations) != 1 || reservation.Allocations[0].SourceType != PointSourceRecharge {
		t.Fatalf("expired gift was not lazily removed: %+v", reservation.Allocations)
	}
	if got := service.MovementCount(ctx, "account-expiry", "EXPIRE"); got != 1 {
		t.Fatalf("expire movement count = %d, want one", got)
	}
}

func TestPersonalPointsConcurrentReserveConservesBalance(t *testing.T) {
	service, _ := newPersonalPointTestService(t)
	ctx := context.Background()
	if _, err := service.Grant(ctx, PersonalPointGrantCommand{AccountID: "account-concurrent", UserID: "user-concurrent", Source: PointSourceRecharge, Points: 10, IdempotencyKey: "paid-concurrent", GrantedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}

	const workers = 10
	var wait sync.WaitGroup
	results := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, err := service.Reserve(ctx, PersonalPointReserveCommand{AccountID: "account-concurrent", UserID: "user-concurrent", BusinessType: "IMAGE_GENERATION", BusinessID: "task-" + string(rune('a'+index)), RequestedPoints: 2, IdempotencyKey: "reserve-" + string(rune('a'+index))})
			results <- err
		}(i)
	}
	wait.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, ErrInsufficientPoints) {
			t.Fatalf("unexpected concurrent reserve error: %v", err)
		}
	}
	if successes != 5 {
		t.Fatalf("concurrent reserve successes = %d, want 5", successes)
	}
	balance, err := service.GetBalance(ctx, "account-concurrent", "user-concurrent")
	if err != nil {
		t.Fatal(err)
	}
	if balance.Available != 0 || balance.Frozen != 10 {
		t.Fatalf("concurrent balance = %+v, want available 0 frozen 10", balance)
	}
}
