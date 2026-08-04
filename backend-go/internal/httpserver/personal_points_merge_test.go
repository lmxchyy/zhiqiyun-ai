package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"
)

func TestMergePersonalPointStatePreservesSourceLotExpiryAndHistory(t *testing.T) {
	now := time.Date(2026, 8, 4, 8, 0, 0, 0, time.UTC)
	expiresAt := time.Date(2026, 11, 4, 8, 0, 0, 0, time.UTC)
	policy := PointPolicySnapshot{Version: 3, Enabled: true, DurationValue: 3, DurationUnit: "CALENDAR_MONTH", TimeZone: "Asia/Shanghai"}
	state := personalPointState{
		Accounts: []PersonalPointAccount{
			{ID: "account-target", UserID: "user-target", AvailablePoints: 5, TotalGranted: 5},
			{ID: "account-source", UserID: "user-source", AvailablePoints: 7, TotalGranted: 7},
		},
		Lots: []PersonalPointLot{
			{ID: "lot-target", AccountID: "account-target", UserID: "user-target", SourceType: PointSourceRecharge, ReferenceType: "ORDER", ReferenceID: "order-target", OriginalPoints: 5, AvailablePoints: 5, GrantedAt: now.Add(-time.Hour), IdempotencyKey: "target-grant", Status: "ACTIVE"},
			{ID: "lot-source", AccountID: "account-source", UserID: "user-source", SourceType: PointSourceAdminGift, ReferenceType: "ADMIN_GIFT", ReferenceID: "gift-1", OriginalPoints: 7, AvailablePoints: 7, GrantedAt: now.Add(-2 * time.Hour), ExpiresAt: expiresAt, PolicyVersionID: "point-expiry-v3", PolicySnapshot: policy, IdempotencyKey: "source-grant", Status: "ACTIVE"},
		},
		Policies: []PointExpiryPolicy{defaultPersonalPointPolicy()},
	}

	result, err := mergePersonalPointState(&state, "user-target", "user-source", "merge-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if result.AccountsMoved != 1 || result.PointsMoved != 7 {
		t.Fatalf("merge result = %+v", result)
	}
	target, err := personalPointAccountForUserState(&state, "user-target")
	if err != nil {
		t.Fatal(err)
	}
	source, err := personalPointAccountForUserState(&state, "user-source")
	if err != nil {
		t.Fatal(err)
	}
	if target.AvailablePoints != 12 || source.AvailablePoints != 0 {
		t.Fatalf("balances after merge target=%d source=%d", target.AvailablePoints, source.AvailablePoints)
	}
	var sourceLot, transferred PersonalPointLot
	for _, lot := range state.Lots {
		switch {
		case lot.ID == "lot-source":
			sourceLot = lot
		case lot.AccountID == target.ID && lot.ReferenceID == "gift-1":
			transferred = lot
		}
	}
	if sourceLot.ReversedPoints != 7 || sourceLot.AvailablePoints != 0 || sourceLot.Status != "REVERSED" {
		t.Fatalf("source lot history was not preserved: %+v", sourceLot)
	}
	if transferred.SourceType != PointSourceAdminGift || transferred.AvailablePoints != 7 || !transferred.ExpiresAt.Equal(expiresAt) || transferred.PolicyVersionID != "point-expiry-v3" || !reflect.DeepEqual(transferred.PolicySnapshot, policy) {
		t.Fatalf("transferred lot lost source/expiry/policy: %+v", transferred)
	}
	if err := validatePersonalPointState(&state); err != nil {
		t.Fatalf("merged state is invalid: %v", err)
	}
}

func TestPostgresMergePersonalPointTxTransfersLotsAndRollsBackWithCaller(t *testing.T) {
	db, store, ctx := openPersonalPointFixRound1Postgres(t)
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	targetAccount, targetUser := "merge-target-account-"+suffix, "merge-target-user-"+suffix
	sourceAccount, sourceUser := "merge-source-account-"+suffix, "merge-source-user-"+suffix
	if _, err := store.grant(ctx, PersonalPointGrantCommand{AccountID: targetAccount, UserID: targetUser, Source: PointSourceRecharge, Points: 5, ReferenceType: "ORDER", ReferenceID: "target-order", IdempotencyKey: "target-grant"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.grant(ctx, PersonalPointGrantCommand{AccountID: sourceAccount, UserID: sourceUser, Source: PointSourceRecharge, Points: 7, ReferenceType: "ORDER", ReferenceID: "source-order", IdempotencyKey: "source-grant"}); err != nil {
		t.Fatal(err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.mergeTx(ctx, tx, targetUser, sourceUser, "merge-"+suffix, time.Now().UTC())
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if result.AccountsMoved != 1 || result.PointsMoved != 7 {
		_ = tx.Rollback()
		t.Fatalf("merge result = %+v", result)
	}
	var targetAvailable, sourceAvailable int64
	if err := tx.QueryRowContext(ctx, `SELECT available FROM xz_point_accounts WHERE id=$1`, targetAccount).Scan(&targetAvailable); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT available FROM xz_point_accounts WHERE id=$1`, sourceAccount).Scan(&sourceAvailable); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if targetAvailable != 12 || sourceAvailable != 0 {
		_ = tx.Rollback()
		t.Fatalf("transactional balances target=%d source=%d", targetAvailable, sourceAvailable)
	}
	var reversed, transferred int64
	if err := tx.QueryRowContext(ctx, `SELECT reversed_points FROM xz_personal_point_lots WHERE account_id=$1`, sourceAccount).Scan(&reversed); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT available_points FROM xz_personal_point_lots WHERE account_id=$1 AND reference_id='source-order'`, targetAccount).Scan(&transferred); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if reversed != 7 || transferred != 7 {
		_ = tx.Rollback()
		t.Fatalf("lot transfer source reversed=%d target opening=%d", reversed, transferred)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT available FROM xz_point_accounts WHERE id=$1`, targetAccount).Scan(&targetAvailable); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT available FROM xz_point_accounts WHERE id=$1`, sourceAccount).Scan(&sourceAvailable); err != nil {
		t.Fatal(err)
	}
	if targetAvailable != 5 || sourceAvailable != 7 {
		t.Fatalf("caller rollback did not restore balances target=%d source=%d", targetAvailable, sourceAvailable)
	}
}

func TestPostgresMergePersonalPointTxRejectsEitherActiveReservation(t *testing.T) {
	db, store, ctx := openPersonalPointFixRound1Postgres(t)
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	targetAccount, targetUser := "merge-res-target-account-"+suffix, "merge-res-target-user-"+suffix
	sourceAccount, sourceUser := "merge-res-source-account-"+suffix, "merge-res-source-user-"+suffix
	for _, item := range []struct{ account, user string }{{targetAccount, targetUser}, {sourceAccount, sourceUser}} {
		if _, err := store.grant(ctx, PersonalPointGrantCommand{AccountID: item.account, UserID: item.user, Source: PointSourceRecharge, Points: 5, ReferenceType: "ORDER", ReferenceID: item.account, IdempotencyKey: "grant"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.reserve(ctx, PersonalPointReserveCommand{AccountID: targetAccount, UserID: targetUser, BusinessType: "GENERATION_TASK", BusinessID: "task-" + suffix, RequestedPoints: 1, IdempotencyKey: "reserve"}); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.mergeTx(ctx, tx, targetUser, sourceUser, "merge-"+suffix, time.Now().UTC())
	if !errors.Is(err, ErrPersonalPointMergeActiveReservation) {
		_ = tx.Rollback()
		t.Fatalf("merge error = %v, want active reservation conflict", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var targetAvailable, targetFrozen, sourceAvailable int64
	if err := db.QueryRowContext(ctx, `SELECT available,frozen FROM xz_point_accounts WHERE id=$1`, targetAccount).Scan(&targetAvailable, &targetFrozen); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT available FROM xz_point_accounts WHERE id=$1`, sourceAccount).Scan(&sourceAvailable); err != nil {
		t.Fatal(err)
	}
	if targetAvailable != 4 || targetFrozen != 1 || sourceAvailable != 5 {
		t.Fatalf("rejected merge mutated balances target=(%d,%d) source=%d", targetAvailable, targetFrozen, sourceAvailable)
	}
}

func TestMergePersonalPointStateRejectsActiveReservationWithoutMutation(t *testing.T) {
	now := time.Date(2026, 8, 4, 8, 0, 0, 0, time.UTC)
	state := personalPointState{
		Accounts: []PersonalPointAccount{
			{ID: "account-target", UserID: "user-target"},
			{ID: "account-source", UserID: "user-source", FrozenPoints: 2, TotalGranted: 2},
		},
		Lots:         []PersonalPointLot{{ID: "lot-source", AccountID: "account-source", UserID: "user-source", SourceType: PointSourceRecharge, ReferenceType: "ORDER", ReferenceID: "order-1", OriginalPoints: 2, ReservedPoints: 2, GrantedAt: now, IdempotencyKey: "grant-1", Status: "ACTIVE"}},
		Reservations: []PersonalPointReservation{{ID: "reservation-1", AccountID: "account-source", UserID: "user-source", BusinessType: "GENERATION_TASK", BusinessID: "task-1", RequestedPoints: 2, ReservedPoints: 2, Status: "RESERVED", IdempotencyKey: "reserve-1", CreatedAt: now, UpdatedAt: now}},
		Allocations:  []PersonalPointAllocation{{ID: "allocation-1", ReservationID: "reservation-1", LotID: "lot-source", AccountID: "account-source", UserID: "user-source", SourceType: PointSourceRecharge, AllocatedPoints: 2, ReservedPoints: 2, Status: "RESERVED"}},
		Policies:     []PointExpiryPolicy{defaultPersonalPointPolicy()},
	}
	before, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}

	_, err = mergePersonalPointState(&state, "user-target", "user-source", "merge-1", now)
	if !errors.Is(err, ErrPersonalPointMergeActiveReservation) {
		t.Fatalf("merge error = %v, want active reservation conflict", err)
	}
	after, marshalErr := json.Marshal(state)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if string(after) != string(before) {
		t.Fatalf("state changed on rejected merge\nbefore=%s\nafter=%s", before, after)
	}
}

func TestMergePersonalPointStateRejectsTargetReservationWhenSourceHasNoAccount(t *testing.T) {
	now := time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC)
	state := personalPointState{
		Accounts:     []PersonalPointAccount{{ID: "account-target", UserID: "user-target", FrozenPoints: 1, TotalGranted: 1}},
		Lots:         []PersonalPointLot{{ID: "lot-target", AccountID: "account-target", UserID: "user-target", SourceType: PointSourceRecharge, ReferenceType: "ORDER", ReferenceID: "order-1", OriginalPoints: 1, ReservedPoints: 1, GrantedAt: now, IdempotencyKey: "grant-1", Status: "ACTIVE"}},
		Reservations: []PersonalPointReservation{{ID: "reservation-1", AccountID: "account-target", UserID: "user-target", BusinessType: "GENERATION_TASK", BusinessID: "task-1", RequestedPoints: 1, ReservedPoints: 1, Status: "RESERVED", IdempotencyKey: "reserve-1", CreatedAt: now, UpdatedAt: now}},
		Allocations:  []PersonalPointAllocation{{ID: "allocation-1", ReservationID: "reservation-1", LotID: "lot-target", AccountID: "account-target", UserID: "user-target", SourceType: PointSourceRecharge, AllocatedPoints: 1, ReservedPoints: 1, Status: "RESERVED"}},
		Policies:     []PointExpiryPolicy{defaultPersonalPointPolicy()},
	}
	before, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	_, err = mergePersonalPointState(&state, "user-target", "user-source-without-account", "merge-target-only", now)
	if !errors.Is(err, ErrPersonalPointMergeActiveReservation) {
		t.Fatalf("merge error = %v, want target active reservation conflict", err)
	}
	after, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("target-only reservation conflict changed point state")
	}
}

func TestPostgresMergePersonalPointTxRejectsTargetReservationWhenSourceHasNoAccount(t *testing.T) {
	db, store, ctx := openPersonalPointFixRound1Postgres(t)
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	targetAccount := "merge-target-only-account-" + suffix
	targetUser := "merge-target-only-user-" + suffix
	if _, err := store.grant(ctx, PersonalPointGrantCommand{AccountID: targetAccount, UserID: targetUser, Source: PointSourceRecharge, Points: 2, ReferenceType: "ORDER", ReferenceID: "target-order", IdempotencyKey: "grant"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.reserve(ctx, PersonalPointReserveCommand{AccountID: targetAccount, UserID: targetUser, BusinessType: "GENERATION_TASK", BusinessID: "task-" + suffix, RequestedPoints: 1, IdempotencyKey: "reserve"}); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.mergeTx(ctx, tx, targetUser, "merge-source-without-account-"+suffix, "merge-"+suffix, time.Now().UTC())
	if !errors.Is(err, ErrPersonalPointMergeActiveReservation) {
		_ = tx.Rollback()
		t.Fatalf("merge error = %v, want target active reservation conflict", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var available, frozen int64
	if err := db.QueryRowContext(ctx, `SELECT available,frozen FROM xz_point_accounts WHERE id=$1`, targetAccount).Scan(&available, &frozen); err != nil {
		t.Fatal(err)
	}
	if available != 1 || frozen != 1 {
		t.Fatalf("rejected merge changed target points available=%d frozen=%d", available, frozen)
	}
}

func TestExecuteAdminAuthMergePointHookFailsBeforeUserMutation(t *testing.T) {
	data := adminPlatformData{
		Users: []adminUser{
			{ID: "user-target", Email: "target@example.test", Status: "ACTIVE"},
			{ID: "user-source", Email: "source@example.test", Status: "ACTIVE", WeChatOpenIDs: []string{"openid-source"}},
		},
		AuthMergeRequests: []adminAuthMergeRequest{{ID: "merge-1", PrimaryUserID: "user-target", SecondaryUserID: "user-source", Status: "PENDING"}},
	}
	before := cloneAdminPlatformDataForTest(t, data)

	_, _, err := executeAdminAuthMergeRequestOnDataWithPointMerge(&data, "merge-1", adminAuthMergeExecuteRequest{TargetUserID: "user-target", Confirm: true}, func(_, _ string) (int, error) {
		return 0, ErrPersonalPointMergeActiveReservation
	})
	if !errors.Is(err, ErrPersonalPointMergeActiveReservation) {
		t.Fatalf("execute error = %v, want active reservation conflict", err)
	}
	if !reflect.DeepEqual(data, before) {
		t.Fatalf("auth data changed before point merge succeeded\nbefore=%+v\nafter=%+v", before, data)
	}
}

func TestJSONAuthMergeActiveReservationLeavesWholeFileUnchanged(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "platform.json")
	store := newJSONStore(path)
	target, err := store.CreateAdminCustomer(adminCustomerMutation{Name: "Merge Target", Email: "merge-target@example.test", Status: "ACTIVE"})
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.CreateAdminCustomer(adminCustomerMutation{Name: "Merge Source", Email: "merge-source@example.test", Status: "ACTIVE"})
	if err != nil {
		t.Fatal(err)
	}
	sourceAccountID := "merge-source-account"
	if _, err := store.PersonalPointService().Grant(ctx, PersonalPointGrantCommand{AccountID: sourceAccountID, UserID: source.ID, Source: PointSourceRecharge, Points: 2, ReferenceType: "ORDER", ReferenceID: "merge-source-order", IdempotencyKey: "merge-source-grant"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PersonalPointService().Reserve(ctx, PersonalPointReserveCommand{AccountID: sourceAccountID, UserID: source.ID, BusinessType: "GENERATION_TASK", BusinessID: "merge-source-task", RequestedPoints: 1, IdempotencyKey: "merge-source-reserve"}); err != nil {
		t.Fatal(err)
	}
	request, err := store.CreateAdminAuthMergeRequest(adminAuthMergeRequestMutation{PrimaryUserID: target.ID, SecondaryUserID: source.ID, ConflictCode: "AUTH_ACCOUNT_MERGE_REQUIRED", Source: "atomic-test", Reason: "active point reservation"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.ExecuteAdminAuthMergeRequest(request.ID, adminAuthMergeExecuteRequest{TargetUserID: target.ID, Confirm: true})
	if !errors.Is(err, ErrPersonalPointMergeActiveReservation) {
		t.Fatalf("execute error = %v, want active reservation conflict", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("JSON auth merge conflict changed the atomic platform file")
	}
	reloaded, err := store.AdminData()
	if err != nil {
		t.Fatal(err)
	}
	users := userMap(reloaded.Users)
	if users[target.ID].Status != "ACTIVE" || users[source.ID].Status != "ACTIVE" {
		t.Fatalf("rejected merge changed users target=%+v source=%+v", users[target.ID], users[source.ID])
	}
	index := adminAuthMergeRequestIndex(reloaded.AuthMergeRequests, request.ID)
	if index < 0 || reloaded.AuthMergeRequests[index].Status != "PENDING" {
		t.Fatalf("rejected merge changed request: %+v", reloaded.AuthMergeRequests)
	}
}

func cloneAdminPlatformDataForTest(t *testing.T, data adminPlatformData) adminPlatformData {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	var cloned adminPlatformData
	if err := json.Unmarshal(raw, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}
