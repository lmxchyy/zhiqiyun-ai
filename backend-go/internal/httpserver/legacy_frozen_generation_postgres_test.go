package httpserver

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const legacyFrozenGenerationPostgresDSNEnv = "LEGACY_FROZEN_GENERATION_POSTGRES_TEST_DSN"

type legacyFrozenPostgresExpectation struct {
	userID          string
	accountID       string
	taskID          string
	status          string
	taskStatus      string
	billingStatus   string
	movementType    string
	available       int64
	consumed        int64
	captured        int64
	released        int64
	billingRefunded bool
}

type legacyFrozenPostgresReplayCounts struct {
	walletRows    int64
	movementRows  int64
	assetRows     int64
	lifecycleRows int64
}

func TestPostgresLegacyFrozenGenerationTerminalsReconcileAndReplay(t *testing.T) {
	dsn := os.Getenv(legacyFrozenGenerationPostgresDSNEnv)
	if dsn == "" {
		t.Skip(legacyFrozenGenerationPostgresDSNEnv + " is not configured")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	store := &postgresStore{db: db, ready: true}

	cases := []struct {
		name   string
		want   legacyFrozenPostgresExpectation
		mutate func() (generationTask, error)
		replay func() (generationTask, error)
	}{
		{
			name: "complete captures the attributed legacy reservation",
			want: legacyFrozenPostgresExpectation{
				userID: "qa_legacy_complete_user", accountID: "qa_legacy_complete_account", taskID: "qa_legacy_complete_task",
				status: "SUCCEEDED", taskStatus: taskStatusSucceeded, billingStatus: billingStatusCaptured,
				movementType: "CAPTURE", available: 7, consumed: 3, captured: 3,
			},
			mutate: func() (generationTask, error) {
				return store.CompleteGenerationTask("qa_legacy_complete_task", createGenerationTaskRequest{})
			},
			replay: func() (generationTask, error) {
				return store.CompleteGenerationTask("qa_legacy_complete_task", createGenerationTaskRequest{})
			},
		},
		{
			name: "failure releases the attributed legacy reservation",
			want: legacyFrozenPostgresExpectation{
				userID: "qa_legacy_fail_user", accountID: "qa_legacy_fail_account", taskID: "qa_legacy_fail_task",
				status: "FAILED", taskStatus: taskStatusFailed, billingStatus: billingStatusReleased,
				movementType: "RELEASE", available: 10, released: 3, billingRefunded: true,
			},
			mutate: func() (generationTask, error) {
				return store.FailGenerationTask("qa_legacy_fail_task", "qa provider failure")
			},
			replay: func() (generationTask, error) {
				return store.FailGenerationTask("qa_legacy_fail_task", "qa duplicate failure")
			},
		},
		{
			name: "cancellation releases atomically and replays as cancelled",
			want: legacyFrozenPostgresExpectation{
				userID: "qa_legacy_cancel_user", accountID: "qa_legacy_cancel_account", taskID: "qa_legacy_cancel_task",
				status: "CANCELLED", taskStatus: taskStatusCancelled, billingStatus: billingStatusReleased,
				movementType: "RELEASE", available: 10, released: 3, billingRefunded: true,
			},
			mutate: func() (generationTask, error) {
				return store.CancelGenerationTaskForUser("qa_legacy_cancel_user", "qa_legacy_cancel_task")
			},
			replay: func() (generationTask, error) {
				return store.CancelGenerationTaskForUser("qa_legacy_cancel_user", "qa_legacy_cancel_task")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertLegacyFrozenPostgresAttributedPrecondition(t, ctx, db, tc.want)
			before := legacyFrozenPostgresCounts(t, ctx, db, tc.want)

			got, err := tc.mutate()
			if err != nil {
				t.Fatalf("terminal mutation: %v", err)
			}
			assertLegacyFrozenReturnedTask(t, got, tc.want)
			assertLegacyFrozenPostgresState(t, ctx, db, tc.want)
			after := legacyFrozenPostgresCounts(t, ctx, db, tc.want)
			if after.walletRows != before.walletRows+1 || after.movementRows != before.movementRows+1 {
				t.Fatalf("terminal point artifacts before=%+v after=%+v, want exactly one wallet row and movement", before, after)
			}

			replayed, err := tc.replay()
			if err != nil {
				t.Fatalf("terminal replay: %v", err)
			}
			assertLegacyFrozenReturnedTask(t, replayed, tc.want)
			assertLegacyFrozenPostgresState(t, ctx, db, tc.want)
			afterReplay := legacyFrozenPostgresCounts(t, ctx, db, tc.want)
			if afterReplay != after {
				t.Fatalf("terminal replay created side effects: first=%+v replay=%+v", after, afterReplay)
			}
		})
	}
}

func assertLegacyFrozenPostgresAttributedPrecondition(t *testing.T, ctx context.Context, db *sql.DB, want legacyFrozenPostgresExpectation) {
	t.Helper()
	var engine, accountID, reservationID, reservationStatus, allocationStatus string
	var accountAvailable, accountFrozen, lotAvailable, lotReserved, reservationReserved, allocationReserved int64
	err := db.QueryRowContext(ctx, `
		SELECT task.raw->>'billingEngine',task.raw->>'personalPointAccountId',task.raw->>'personalPointReservationId',
		       account.available,account.frozen,lot.available_points,lot.reserved_points,
		       reservation.reserved_points,reservation.status,allocation.reserved_points,allocation.status
		FROM xz_generation_tasks task
		JOIN xz_point_accounts account ON account.id=task.raw->>'personalPointAccountId'
		JOIN xz_personal_point_reservations reservation ON reservation.id=task.raw->>'personalPointReservationId'
		JOIN xz_personal_point_reservation_allocations allocation ON allocation.reservation_id=reservation.id
		JOIN xz_personal_point_lots lot ON lot.id=allocation.lot_id
		WHERE task.id=$1
	`, want.taskID).Scan(&engine, &accountID, &reservationID, &accountAvailable, &accountFrozen, &lotAvailable, &lotReserved, &reservationReserved, &reservationStatus, &allocationReserved, &allocationStatus)
	if err != nil {
		t.Fatalf("migration 105 attribution precondition: %v", err)
	}
	if engine != personalLotBillingEngine || accountID != want.accountID || reservationID == "" ||
		accountAvailable != 7 || accountFrozen != 3 || lotAvailable != 7 || lotReserved != 3 ||
		reservationReserved != 3 || reservationStatus != "RESERVED" || allocationReserved != 3 || allocationStatus != "RESERVED" {
		t.Fatalf("migration 105 attribution drift engine/account/reservation=%q/%q/%q account=%d/%d lot=%d/%d reservation=%d/%s allocation=%d/%s",
			engine, accountID, reservationID, accountAvailable, accountFrozen, lotAvailable, lotReserved, reservationReserved, reservationStatus, allocationReserved, allocationStatus)
	}
}

func assertLegacyFrozenReturnedTask(t *testing.T, task generationTask, want legacyFrozenPostgresExpectation) {
	t.Helper()
	if task.ID != want.taskID || task.UserID != want.userID || task.Status != want.status || task.TaskStatus != want.taskStatus || task.BillingStatus != want.billingStatus {
		t.Fatalf("returned terminal task id/user/status/taskStatus/billingStatus=%q/%q/%q/%q/%q",
			task.ID, task.UserID, task.Status, task.TaskStatus, task.BillingStatus)
	}
	if task.BillingEngine != personalLotBillingEngine || task.PersonalPointAccountID != want.accountID || task.PersonalPointReservationID == "" {
		t.Fatalf("returned task lot markers=%q/%q/%q", task.BillingEngine, task.PersonalPointAccountID, task.PersonalPointReservationID)
	}
	if int64(task.ReservedPoints) != 3 || int64(task.CapturedPoints) != want.captured || int64(task.ReleasedPoints) != want.released || int64(task.CapturedPoints+task.ReleasedPoints) != 3 {
		t.Fatalf("returned task economic totals reserved/captured/released=%v/%v/%v", task.ReservedPoints, task.CapturedPoints, task.ReleasedPoints)
	}
}

func assertLegacyFrozenPostgresState(t *testing.T, ctx context.Context, db *sql.DB, want legacyFrozenPostgresExpectation) {
	t.Helper()
	assertLegacyFrozenPostgresAccountAndWallet(t, ctx, db, want)
	reservationID, lotID := assertLegacyFrozenPostgresLotReservationAllocation(t, ctx, db, want)
	assertLegacyFrozenPostgresWalletLedgerAndMovement(t, ctx, db, want, reservationID, lotID)
	assertLegacyFrozenPostgresTaskProjection(t, ctx, db, want)
}

func assertLegacyFrozenPostgresAccountAndWallet(t *testing.T, ctx context.Context, db *sql.DB, want legacyFrozenPostgresExpectation) {
	t.Helper()
	var available, frozen, rawAvailable, rawFrozen, rawGranted, rawUsed int64
	if err := db.QueryRowContext(ctx, `
		SELECT available,frozen,(raw->>'available')::bigint,(raw->>'frozen')::bigint,
		       (raw->>'totalGranted')::bigint,(raw->>'totalUsed')::bigint
		FROM xz_point_accounts WHERE id=$1 AND user_id=$2
	`, want.accountID, want.userID).Scan(&available, &frozen, &rawAvailable, &rawFrozen, &rawGranted, &rawUsed); err != nil {
		t.Fatal(err)
	}
	if available != want.available || frozen != 0 || rawAvailable != available || rawFrozen != frozen || rawGranted != 10 || rawUsed != want.consumed {
		t.Fatalf("point account columns/raw available/frozen/granted/used=%d/%d/%d/%d/%d/%d", available, frozen, rawAvailable, rawFrozen, rawGranted, rawUsed)
	}

	var tokenBalance, frozenToken, totalGranted, totalUsed, rawToken, rawWalletFrozen, rawWalletGranted, rawWalletUsed int64
	if err := db.QueryRowContext(ctx, `
		SELECT token_balance,frozen_token,total_token_granted,total_token_used,
		       (raw->>'tokenBalance')::bigint,(raw->>'frozenToken')::bigint,
		       (raw->>'totalTokenGranted')::bigint,(raw->>'totalTokenUsed')::bigint
		FROM xz_user_wallets WHERE user_id=$1
	`, want.userID).Scan(&tokenBalance, &frozenToken, &totalGranted, &totalUsed, &rawToken, &rawWalletFrozen, &rawWalletGranted, &rawWalletUsed); err != nil {
		t.Fatal(err)
	}
	if tokenBalance != available || frozenToken != frozen || totalGranted != 10 || totalUsed != want.consumed ||
		rawToken != tokenBalance || rawWalletFrozen != frozenToken || rawWalletGranted != totalGranted || rawWalletUsed != totalUsed {
		t.Fatalf("user wallet columns/raw balance/frozen/granted/used=%d/%d/%d/%d/%d/%d/%d/%d",
			tokenBalance, frozenToken, totalGranted, totalUsed, rawToken, rawWalletFrozen, rawWalletGranted, rawWalletUsed)
	}
}

func assertLegacyFrozenPostgresLotReservationAllocation(t *testing.T, ctx context.Context, db *sql.DB, want legacyFrozenPostgresExpectation) (string, string) {
	t.Helper()
	var lotID, lotStatus string
	var original, available, reserved, consumed, expired, reversed int64
	if err := db.QueryRowContext(ctx, `
		SELECT id,original_points,available_points,reserved_points,consumed_points,expired_points,reversed_points,status
		FROM xz_personal_point_lots WHERE account_id=$1 AND user_id=$2 AND source_type='LEGACY'
	`, want.accountID, want.userID).Scan(&lotID, &original, &available, &reserved, &consumed, &expired, &reversed, &lotStatus); err != nil {
		t.Fatal(err)
	}
	if original != 10 || available != want.available || reserved != 0 || consumed != want.consumed || expired != 0 || reversed != 0 || lotStatus != "LEGACY" || original != available+reserved+consumed+expired+reversed {
		t.Fatalf("legacy lot id/original/available/reserved/consumed/expired/reversed/status=%s/%d/%d/%d/%d/%d/%d/%s",
			lotID, original, available, reserved, consumed, expired, reversed, lotStatus)
	}

	var reservationID, reservationStatus string
	var requested, reservationReserved, captured, released, reservationExpired int64
	if err := db.QueryRowContext(ctx, `
		SELECT id,requested_points,reserved_points,captured_points,released_points,expired_points,status
		FROM xz_personal_point_reservations WHERE account_id=$1 AND user_id=$2 AND business_type='GENERATION_TASK' AND business_id=$3
	`, want.accountID, want.userID, want.taskID).Scan(&reservationID, &requested, &reservationReserved, &captured, &released, &reservationExpired, &reservationStatus); err != nil {
		t.Fatal(err)
	}
	if requested != 3 || reservationReserved != 0 || captured != want.captured || released != want.released || reservationExpired != 0 ||
		requested != reservationReserved+captured+released+reservationExpired || reservationStatus != want.billingStatus {
		t.Fatalf("reservation id/requested/reserved/captured/released/expired/status=%s/%d/%d/%d/%d/%d/%s",
			reservationID, requested, reservationReserved, captured, released, reservationExpired, reservationStatus)
	}

	var allocationStatus string
	var allocated, allocationReserved, allocationCaptured, allocationReleased, allocationExpired int64
	if err := db.QueryRowContext(ctx, `
		SELECT allocated_points,reserved_points,captured_points,released_points,expired_points,status
		FROM xz_personal_point_reservation_allocations
		WHERE reservation_id=$1 AND lot_id=$2 AND account_id=$3 AND user_id=$4
	`, reservationID, lotID, want.accountID, want.userID).Scan(&allocated, &allocationReserved, &allocationCaptured, &allocationReleased, &allocationExpired, &allocationStatus); err != nil {
		t.Fatal(err)
	}
	if allocated != 3 || allocationReserved != 0 || allocationCaptured != want.captured || allocationReleased != want.released || allocationExpired != 0 ||
		allocated != allocationReserved+allocationCaptured+allocationReleased+allocationExpired || allocationStatus != want.billingStatus {
		t.Fatalf("allocation allocated/reserved/captured/released/expired/status=%d/%d/%d/%d/%d/%s",
			allocated, allocationReserved, allocationCaptured, allocationReleased, allocationExpired, allocationStatus)
	}
	return reservationID, lotID
}

func assertLegacyFrozenPostgresWalletLedgerAndMovement(t *testing.T, ctx context.Context, db *sql.DB, want legacyFrozenPostgresExpectation, reservationID, lotID string) {
	t.Helper()
	var reservePoints, reserveAvailableBefore, reserveAvailableAfter, reserveFrozenBefore, reserveFrozenAfter int64
	if err := db.QueryRowContext(ctx, `
		SELECT points::bigint,available_before::bigint,available_after::bigint,frozen_before::bigint,frozen_after::bigint
		FROM xz_wallet_ledger
		WHERE task_id=$1 AND entry_type='RESERVE' AND idempotency_key=$1||':RESERVE'
	`, want.taskID).Scan(&reservePoints, &reserveAvailableBefore, &reserveAvailableAfter, &reserveFrozenBefore, &reserveFrozenAfter); err != nil {
		t.Fatal(err)
	}
	if reservePoints != 3 || reserveAvailableBefore != 10 || reserveAvailableAfter != 7 || reserveFrozenBefore != 0 || reserveFrozenAfter != 3 {
		t.Fatalf("historical RESERVE wallet transition=%d %d->%d %d->%d", reservePoints, reserveAvailableBefore, reserveAvailableAfter, reserveFrozenBefore, reserveFrozenAfter)
	}

	var points, availableBefore, availableAfter, frozenBefore, frozenAfter int64
	var referenceType, referenceID string
	if err := db.QueryRowContext(ctx, `
			SELECT points::bigint,available_before::bigint,available_after::bigint,frozen_before::bigint,frozen_after::bigint,reference_type,reference_id
			FROM xz_wallet_ledger WHERE task_id=$1 AND reference_id=$1 AND entry_type=$2
		`, want.taskID, want.movementType).Scan(&points, &availableBefore, &availableAfter, &frozenBefore, &frozenAfter, &referenceType, &referenceID); err != nil {
		t.Fatalf("terminal wallet ledger: %v", err)
	}
	wantAvailableAfter := int64(7)
	if want.movementType == "RELEASE" {
		wantAvailableAfter = 10
	}
	if points != 3 || availableBefore != 7 || availableAfter != wantAvailableAfter || frozenBefore != 3 || frozenAfter != 0 || referenceType != "GENERATION_TASK" || referenceID != want.taskID {
		t.Fatalf("terminal wallet transition=%s/%d %d->%d %d->%d reference=%s/%s", want.movementType, points, availableBefore, availableAfter, frozenBefore, frozenAfter, referenceType, referenceID)
	}

	var movementPoints, movementAvailableBefore, movementAvailableAfter, movementReservedBefore, movementReservedAfter, consumedBefore, consumedAfter int64
	if err := db.QueryRowContext(ctx, `
		SELECT points,available_before,available_after,reserved_before,reserved_after,consumed_before,consumed_after
		FROM xz_personal_point_lot_movements
		WHERE reservation_id=$1 AND lot_id=$2 AND movement_type=$3
	`, reservationID, lotID, want.movementType).Scan(&movementPoints, &movementAvailableBefore, &movementAvailableAfter, &movementReservedBefore, &movementReservedAfter, &consumedBefore, &consumedAfter); err != nil {
		t.Fatal(err)
	}
	if movementPoints != 3 || movementAvailableBefore != 7 || movementAvailableAfter != wantAvailableAfter || movementReservedBefore != 3 || movementReservedAfter != 0 || consumedBefore != 0 || consumedAfter != want.consumed {
		t.Fatalf("terminal lot movement=%s/%d available %d->%d reserved %d->%d consumed %d->%d",
			want.movementType, movementPoints, movementAvailableBefore, movementAvailableAfter, movementReservedBefore, movementReservedAfter, consumedBefore, consumedAfter)
	}
}

func assertLegacyFrozenPostgresTaskProjection(t *testing.T, ctx context.Context, db *sql.DB, want legacyFrozenPostgresExpectation) {
	t.Helper()
	var status, taskStatus, billingStatus, rawStatus, rawTaskStatus, rawBillingStatus, rawEngine, rawAccount, rawReservation, paramsRefunded, rawRefunded string
	var reserved, captured, released, rawReserved, rawCaptured, rawReleased int64
	if err := db.QueryRowContext(ctx, `
		SELECT status,task_status,billing_status,reserved_points::bigint,captured_points::bigint,released_points::bigint,
		       raw->>'status',raw->>'taskStatus',raw->>'billingStatus',
		       coalesce((raw->>'reservedPoints')::numeric::bigint,0),
		       coalesce((raw->>'capturedPoints')::numeric::bigint,0),
		       coalesce((raw->>'releasedPoints')::numeric::bigint,0),
		       raw->>'billingEngine',raw->>'personalPointAccountId',raw->>'personalPointReservationId',
		       coalesce(params->>'billingRefunded','false'),coalesce(raw->'params'->>'billingRefunded','false')
		FROM xz_generation_tasks WHERE id=$1 AND user_id=$2
	`, want.taskID, want.userID).Scan(&status, &taskStatus, &billingStatus, &reserved, &captured, &released,
		&rawStatus, &rawTaskStatus, &rawBillingStatus, &rawReserved, &rawCaptured, &rawReleased,
		&rawEngine, &rawAccount, &rawReservation, &paramsRefunded, &rawRefunded); err != nil {
		t.Fatal(err)
	}
	wantRefunded := fmt.Sprint(want.billingRefunded)
	if status != want.status || taskStatus != want.taskStatus || billingStatus != want.billingStatus ||
		reserved != 3 || captured != want.captured || released != want.released || captured+released != reserved ||
		rawStatus != status || rawTaskStatus != taskStatus || rawBillingStatus != billingStatus ||
		rawReserved != reserved || rawCaptured != captured || rawReleased != released ||
		rawEngine != personalLotBillingEngine || rawAccount != want.accountID || rawReservation == "" ||
		paramsRefunded != wantRefunded || rawRefunded != wantRefunded {
		t.Fatalf("task structured/raw drift status=%s/%s/%s raw=%s/%s/%s totals=%d/%d/%d rawTotals=%d/%d/%d markers=%s/%s/%s refunded=%s/%s",
			status, taskStatus, billingStatus, rawStatus, rawTaskStatus, rawBillingStatus,
			reserved, captured, released, rawReserved, rawCaptured, rawReleased,
			rawEngine, rawAccount, rawReservation, paramsRefunded, rawRefunded)
	}
}

func legacyFrozenPostgresCounts(t *testing.T, ctx context.Context, db *sql.DB, want legacyFrozenPostgresExpectation) legacyFrozenPostgresReplayCounts {
	t.Helper()
	var counts legacyFrozenPostgresReplayCounts
	queries := []struct {
		query string
		dest  *int64
	}{
		{`SELECT count(*) FROM xz_wallet_ledger WHERE task_id=$1`, &counts.walletRows},
		{`SELECT count(*) FROM xz_personal_point_lot_movements WHERE reservation_id=(SELECT raw->>'personalPointReservationId' FROM xz_generation_tasks WHERE id=$1)`, &counts.movementRows},
		{`SELECT count(*) FROM xz_assets WHERE task_id=$1`, &counts.assetRows},
		{`SELECT count(*) FROM xz_billing_lifecycle_events WHERE task_id=$1`, &counts.lifecycleRows},
	}
	for _, item := range queries {
		if err := db.QueryRowContext(ctx, item.query, want.taskID).Scan(item.dest); err != nil {
			t.Fatalf("replay count query %q: %v", item.query, err)
		}
	}
	return counts
}
