package httpserver

import (
	"context"
	"testing"
	"time"
)

func grantPermanentTestPoints(t *testing.T, store *jsonStore, userID string, points int64) {
	t.Helper()
	if store == nil || userID == "" || points <= 0 {
		t.Fatalf("invalid permanent point fixture: store=%v userID=%q points=%d", store != nil, userID, points)
	}
	account, err := store.PointAccount(userID)
	if err != nil {
		t.Fatalf("resolve point account for %s: %v", userID, err)
	}
	grant, err := store.PersonalPointService().Grant(context.Background(), PersonalPointGrantCommand{
		AccountID:      account.ID,
		UserID:         userID,
		Source:         PointSourceRecharge,
		Points:         points,
		ReferenceType:  "TEST_FIXTURE",
		ReferenceID:    t.Name(),
		IdempotencyKey: "test-permanent-recharge:" + t.Name() + ":" + userID,
		Reason:         "HTTP test fixture",
		GrantedAt:      time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("grant permanent test points for %s: %v", userID, err)
	}
	if grant.Lot.SourceType != PointSourceRecharge || !grant.Lot.Permanent() {
		t.Fatalf("permanent test grant is not a recharge lot: %+v", grant.Lot)
	}
}
