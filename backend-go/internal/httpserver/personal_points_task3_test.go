package httpserver

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestPersonalPointPolicyPublishRequiresCurrentRevisionAndReason(t *testing.T) {
	ctx := context.Background()
	service := NewPersonalPointService(NewJSONPersonalPointStore(filepath.Join(t.TempDir(), "points.json")))

	current, err := service.CurrentPolicy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != 1 || current.DurationValue != 3 || current.TimeZone != "Asia/Shanghai" {
		t.Fatalf("default policy = %+v", current)
	}
	updated, err := service.PublishPolicy(ctx, PersonalPointPolicyPublishCommand{
		ExpectedRevision: current.Revision,
		Enabled:          false,
		DurationValue:    3,
		ChangeReason:     "campaign ended",
		ActorID:          "admin-policy",
		PublishedAt:      time.Date(2026, 8, 4, 1, 2, 3, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 || updated.Version != 2 || updated.Enabled || updated.ChangeReason != "campaign ended" || updated.CreatedBy != "admin-policy" {
		t.Fatalf("published policy = %+v", updated)
	}
	_, err = service.PublishPolicy(ctx, PersonalPointPolicyPublishCommand{
		ExpectedRevision: current.Revision,
		Enabled:          true,
		DurationValue:    3,
		ChangeReason:     "stale update",
		ActorID:          "admin-policy",
	})
	if !errors.Is(err, ErrPointPolicyRevisionConflict) {
		t.Fatalf("stale publish error = %v", err)
	}
	_, err = service.PublishPolicy(ctx, PersonalPointPolicyPublishCommand{
		ExpectedRevision: updated.Revision,
		Enabled:          true,
		DurationValue:    3,
		ActorID:          "admin-policy",
	})
	if !errors.Is(err, ErrInvalidPointCommand) {
		t.Fatalf("missing reason error = %v", err)
	}
}

func TestPersonalPointPostgresSummaryLotQueryAndExpiryBatch(t *testing.T) {
	_, store, ctx := openPersonalPointFixRound1Postgres(t)
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	accountID, userID := "task3-summary-"+suffix, "task3-user-"+suffix
	service := NewPersonalPointService(store)
	if _, err := service.Grant(ctx, PersonalPointGrantCommand{
		AccountID: accountID, UserID: userID, Source: PointSourceAdminGift, Points: 11,
		IdempotencyKey: "task3-expiring", GrantedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Grant(ctx, PersonalPointGrantCommand{
		AccountID: accountID, UserID: userID, Source: PointSourceRecharge, Points: 13,
		IdempotencyKey: "task3-permanent", GrantedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	lots, err := service.ListLots(ctx, accountID, userID, PersonalPointLotFilter{})
	if err != nil || len(lots) != 2 {
		t.Fatalf("lots=%+v err=%v", lots, err)
	}
	result, err := service.ExpireDue(ctx, time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC), 100)
	if err != nil {
		t.Fatal(err)
	}
	if result.AccountsProcessed < 1 || result.PointsExpired < 11 {
		t.Fatalf("expiry result=%+v", result)
	}
	summary, err := service.Summary(ctx, accountID, userID, time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if summary.Available != 13 || summary.PermanentAvailable != 13 || summary.ExpiringAvailable != 0 || summary.NextExpiryPoints != 0 {
		t.Fatalf("summary=%+v", summary)
	}
}

func TestPersonalPointSummaryAndLotQuerySeparatePermanentAndExpiringBalances(t *testing.T) {
	ctx := context.Background()
	service := NewPersonalPointService(NewJSONPersonalPointStore(filepath.Join(t.TempDir(), "points.json")))
	grantedAt := time.Date(2026, 1, 31, 2, 0, 0, 0, time.UTC)

	gift, err := service.Grant(ctx, PersonalPointGrantCommand{
		AccountID: "summary-account", UserID: "summary-user", Source: PointSourceAdminGift,
		Points: 25, IdempotencyKey: "summary-gift", GrantedAt: grantedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Grant(ctx, PersonalPointGrantCommand{
		AccountID: "summary-account", UserID: "summary-user", Source: PointSourceRecharge,
		Points: 40, IdempotencyKey: "summary-paid", GrantedAt: grantedAt,
	}); err != nil {
		t.Fatal(err)
	}

	summary, err := service.Summary(ctx, "summary-account", "summary-user", grantedAt)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Available != 65 || summary.Frozen != 0 || summary.Total != 65 || summary.PermanentAvailable != 40 || summary.ExpiringAvailable != 25 || summary.NextExpiryPoints != 25 || !summary.NextExpiryAt.Equal(gift.Lot.ExpiresAt) {
		t.Fatalf("summary = %+v, gift expiry = %s", summary, gift.Lot.ExpiresAt)
	}
	lots, err := service.ListLots(ctx, "summary-account", "summary-user", PersonalPointLotFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(lots) != 2 || lots[0].UserID != "summary-user" || lots[1].UserID != "summary-user" {
		t.Fatalf("lots = %+v", lots)
	}
	if _, err := service.ListLots(ctx, "summary-account", "other-user", PersonalPointLotFilter{}); !errors.Is(err, ErrPointOwnership) {
		t.Fatalf("cross-user lot query error = %v", err)
	}
}

func TestPersonalPointExpiryBatchIsBoundedAndIdempotent(t *testing.T) {
	ctx := context.Background()
	service := NewPersonalPointService(NewJSONPersonalPointStore(filepath.Join(t.TempDir(), "points.json")))
	grantedAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, accountID := range []string{"expiry-worker-a", "expiry-worker-b"} {
		if _, err := service.Grant(ctx, PersonalPointGrantCommand{
			AccountID: accountID, UserID: "expiry-user-" + accountID,
			Source: PointSourceActivityGift, Points: int64(i + 1), IdempotencyKey: "worker-gift", GrantedAt: grantedAt,
		}); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)
	first, err := service.ExpireDue(ctx, now, 1)
	if err != nil {
		t.Fatal(err)
	}
	if first.AccountsProcessed != 1 || first.PointsExpired < 1 {
		t.Fatalf("first batch = %+v", first)
	}
	second, err := service.ExpireDue(ctx, now, 10)
	if err != nil {
		t.Fatal(err)
	}
	if second.AccountsProcessed != 1 || second.PointsExpired < 1 {
		t.Fatalf("second batch = %+v", second)
	}
	third, err := service.ExpireDue(ctx, now, 10)
	if err != nil {
		t.Fatal(err)
	}
	if third.AccountsProcessed != 0 || third.PointsExpired != 0 {
		t.Fatalf("idempotent third batch = %+v", third)
	}
}
