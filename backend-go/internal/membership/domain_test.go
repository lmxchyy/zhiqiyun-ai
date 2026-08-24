package membership

import (
	"testing"
	"time"
)

func TestNormalizeManualGrantAndResolveExpiry(t *testing.T) {
	request, err := NormalizeManualGrant(ManualGrantRequest{PlanID: "plan_ai_creator_996", DurationDays: 365, Reason: "company benefit", IdempotencyKey: "grant-1"})
	if err != nil {
		t.Fatal(err)
	}
	if request.PlanID != "plan_ai_creator_996" || request.DurationDays != 365 {
		t.Fatalf("normalized request = %+v", request)
	}

	now := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)
	kept, err := ResolveExpiry(now, now.AddDate(1, 0, 0), 365)
	if err != nil {
		t.Fatal(err)
	}
	if !kept.Equal(now.AddDate(1, 0, 0)) {
		t.Fatalf("existing later expiry shortened: %s", kept)
	}
	if _, err := NormalizeManualGrant(ManualGrantRequest{PlanID: "x", DurationDays: 365, Reason: "", IdempotencyKey: "grant-2"}); err != ErrInvalidGrant {
		t.Fatalf("invalid request error = %v, want %v", err, ErrInvalidGrant)
	}
}
