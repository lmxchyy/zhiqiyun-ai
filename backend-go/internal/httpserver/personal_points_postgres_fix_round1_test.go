package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func openPersonalPointFixRound1Postgres(t *testing.T) (*sql.DB, *PostgresPersonalPointStore, context.Context) {
	t.Helper()
	dsn := os.Getenv("XIANZHI_PERSONAL_POINT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("XIANZHI_PERSONAL_POINT_TEST_DATABASE_URL is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, table := range []string{"xz_point_accounts", "xz_wallet_ledger", "xz_point_expiry_policy_versions", "xz_personal_point_lots", "xz_personal_point_reservations", "xz_personal_point_reservation_allocations", "xz_personal_point_lot_movements"} {
		var name sql.NullString
		if err := db.QueryRowContext(ctx, `SELECT to_regclass($1)`, "public."+table).Scan(&name); err != nil {
			t.Fatal(err)
		}
		if !name.Valid {
			t.Fatalf("migration-backed table %s is missing", table)
		}
	}
	return db, NewPostgresPersonalPointStore(db), ctx
}

func TestPersonalPointsPostgresFixRound1IdentityIdempotencyAndLocks(t *testing.T) {
	db, store, ctx := openPersonalPointFixRound1Postgres(t)
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	accountOne, accountTwo, zeroAccount, reserveAccount, concurrentAccount := "fix-one-"+suffix, "fix-two-"+suffix, "fix-zero-"+suffix, "fix-reserve-"+suffix, "fix-concurrent-"+suffix

	grant := func(accountID string, points int64, key string, grantedAt time.Time) PersonalPointGrantResult {
		t.Helper()
		result, err := store.grant(ctx, PersonalPointGrantCommand{AccountID: accountID, UserID: "user-" + accountID, Source: PointSourceRecharge, Points: points, IdempotencyKey: key, GrantedAt: grantedAt})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	grant(accountOne, 3, "same-caller-key", time.Now().UTC())
	grant(accountTwo, 5, "same-caller-key", time.Now().UTC())
	var walletRows int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM xz_wallet_ledger WHERE account_id IN ($1,$2) AND entry_type='RECHARGE'`, accountOne, accountTwo).Scan(&walletRows); err != nil {
		t.Fatal(err)
	}
	if walletRows != 2 {
		t.Fatalf("cross-account same key wallet rows=%d, want 2", walletRows)
	}

	zeroCmd := PersonalPointGrantCommand{AccountID: zeroAccount, UserID: "user-" + zeroAccount, Source: PointSourceRecharge, Points: 4, IdempotencyKey: "zero-granted-at"}
	if _, err := store.grant(ctx, zeroCmd); err != nil {
		t.Fatal(err)
	}
	second, err := store.grant(ctx, zeroCmd)
	if err != nil {
		t.Fatalf("zero GrantedAt retry = %v", err)
	}
	if !second.Idempotent {
		t.Fatalf("zero GrantedAt retry = %+v, want idempotent", second)
	}

	grant(reserveAccount, 10, "reserve-fingerprint-grant", time.Now().UTC())
	firstReserve, err := store.reserve(ctx, PersonalPointReserveCommand{AccountID: reserveAccount, UserID: "user-" + reserveAccount, BusinessType: "IMAGE", BusinessID: "reserve-task", RequestedPoints: 2, IdempotencyKey: "reserve-fingerprint", ReservedAt: time.Date(2026, 8, 4, 1, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if firstReserve.Reservation.ID == "" {
		t.Fatal("reserve returned empty reservation")
	}
	_, err = store.reserve(ctx, PersonalPointReserveCommand{AccountID: reserveAccount, UserID: "user-" + reserveAccount, BusinessType: "IMAGE", BusinessID: "reserve-task", RequestedPoints: 2, IdempotencyKey: "reserve-fingerprint", ReservedAt: time.Date(2026, 8, 4, 2, 0, 0, 0, time.UTC)})
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("reserve same key different normalized payload error=%v, want conflict", err)
	}

	grant(concurrentAccount, 10, "concurrent-grant", time.Now().UTC())
	var wait sync.WaitGroup
	results := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, reserveErr := store.reserve(context.Background(), PersonalPointReserveCommand{AccountID: concurrentAccount, UserID: "user-" + concurrentAccount, BusinessType: "IMAGE", BusinessID: fmt.Sprintf("concurrent-task-%d", index), RequestedPoints: 2, IdempotencyKey: fmt.Sprintf("concurrent-key-%d", index)})
			results <- reserveErr
		}(i)
	}
	wait.Wait()
	close(results)
	successes := 0
	for reserveErr := range results {
		if reserveErr == nil {
			successes++
		} else if !errors.Is(reserveErr, ErrInsufficientPoints) {
			t.Fatalf("concurrent reserve error=%v", reserveErr)
		}
	}
	if successes != 5 {
		t.Fatalf("concurrent reserve successes=%d, want 5", successes)
	}
}

func TestPersonalPointsPostgresExpiryLeavesReservedAndExpiresReleasedPoints(t *testing.T) {
	db, store, ctx := openPersonalPointFixRound1Postgres(t)
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	grantAt := time.Date(2024, 1, 31, 15, 45, 0, 0, time.UTC)
	reserveAt := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	expireAt := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)

	captureAccount := "pg-expiry-capture-" + suffix
	if _, err := store.grant(ctx, PersonalPointGrantCommand{AccountID: captureAccount, UserID: "user-" + captureAccount, Source: PointSourceRegistrationGift, Points: 4, IdempotencyKey: "gift", GrantedAt: grantAt}); err != nil {
		t.Fatal(err)
	}
	reserved, err := store.reserve(ctx, PersonalPointReserveCommand{AccountID: captureAccount, UserID: "user-" + captureAccount, BusinessType: "IMAGE", BusinessID: "capture-task", RequestedPoints: 4, IdempotencyKey: "reserve", ReservedAt: reserveAt})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.expire(ctx, PersonalPointExpiryCommand{AccountID: captureAccount, UserID: "user-" + captureAccount, Now: expireAt}); err != nil {
		t.Fatal(err)
	}
	captured, err := store.capture(ctx, PersonalPointCaptureCommand{AccountID: captureAccount, UserID: "user-" + captureAccount, ReservationID: reserved.Reservation.ID, Points: 4, IdempotencyKey: "capture-after-expiry", CapturedAt: expireAt})
	if err != nil {
		t.Fatalf("reserved points should remain capturable after deadline: %v", err)
	}
	if captured.Reservation.CapturedPoints != 4 || captured.Reservation.ExpiredPoints != 0 {
		t.Fatalf("capture after expiry reservation = %+v", captured.Reservation)
	}

	releaseAccount := "pg-expiry-release-" + suffix
	if _, err := store.grant(ctx, PersonalPointGrantCommand{AccountID: releaseAccount, UserID: "user-" + releaseAccount, Source: PointSourceRegistrationGift, Points: 4, IdempotencyKey: "gift", GrantedAt: grantAt}); err != nil {
		t.Fatal(err)
	}
	reserved, err = store.reserve(ctx, PersonalPointReserveCommand{AccountID: releaseAccount, UserID: "user-" + releaseAccount, BusinessType: "IMAGE", BusinessID: "release-task", RequestedPoints: 4, IdempotencyKey: "reserve", ReservedAt: reserveAt})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.release(ctx, PersonalPointReleaseCommand{AccountID: releaseAccount, UserID: "user-" + releaseAccount, ReservationID: reserved.Reservation.ID, IdempotencyKey: "release-after-expiry", ReleasedAt: expireAt}); err != nil {
		t.Fatal(err)
	}
	var available, reservedPoints, expiredPoints int64
	var status string
	if err := db.QueryRowContext(ctx, `SELECT available_points,reserved_points,expired_points,status FROM xz_personal_point_lots WHERE account_id=$1`, releaseAccount).Scan(&available, &reservedPoints, &expiredPoints, &status); err != nil {
		t.Fatal(err)
	}
	if available != 0 || reservedPoints != 0 || expiredPoints != 4 || status != "EXPIRED" {
		t.Fatalf("release-after-expiry lot=(%d,%d,%d,%s), want (0,0,4,EXPIRED)", available, reservedPoints, expiredPoints, status)
	}
}
