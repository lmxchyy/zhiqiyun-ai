package httpserver

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNormalizeAdminMembershipGrantRequest(t *testing.T) {
	req, err := normalizeAdminMembershipGrantRequest(adminMembershipGrantRequest{
		PlanID: " plan_ai_creator_996 ", DurationDays: 365,
		Reason: " 合作客户赠送 ", IdempotencyKey: " grant-001 ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.PlanID != "plan_ai_creator_996" || req.Reason != "合作客户赠送" || req.IdempotencyKey != "grant-001" {
		t.Fatalf("request was not normalized: %+v", req)
	}
	if _, err := normalizeAdminMembershipGrantRequest(adminMembershipGrantRequest{PlanID: "plan_ai_creator_996", DurationDays: 3651, Reason: "x", IdempotencyKey: "x"}); err == nil {
		t.Fatal("expected overlong membership validity to be rejected")
	}
}

func TestAdminManualExpiryUsesExplicitDays(t *testing.T) {
	start := time.Date(2026, 8, 23, 3, 0, 0, 0, time.UTC)
	expires, err := adminManualExpiry(start, 365)
	if err != nil {
		t.Fatal(err)
	}
	want := start.AddDate(0, 0, 365)
	if !expires.Equal(want) {
		t.Fatalf("expiry=%s want=%s", expires, want)
	}
}

func TestResolveAdminMembershipExpiryNeverShortensExistingMembership(t *testing.T) {
	now := time.Date(2026, 8, 23, 3, 0, 0, 0, time.UTC)
	longer := now.AddDate(0, 0, 730)
	got, err := resolveAdminMembershipExpiry(now, longer.Format(time.RFC3339Nano), 365)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(longer) {
		t.Fatalf("expiry=%s want existing longer expiry=%s", got, longer)
	}

	shorter := now.AddDate(0, 0, 30)
	got, err = resolveAdminMembershipExpiry(now, shorter.Format(time.RFC3339Nano), 365)
	if err != nil {
		t.Fatal(err)
	}
	want := now.AddDate(0, 0, 365)
	if !got.Equal(want) {
		t.Fatalf("expiry=%s want new grant expiry=%s", got, want)
	}

	if _, err := resolveAdminMembershipExpiry(now, "not-a-timestamp", 365); err == nil {
		t.Fatal("malformed existing expiry must fail closed")
	}
}

func TestFindAdminMembershipPlanAccepts996WithoutGrantingPoints(t *testing.T) {
	plan, err := findAdminMembershipPlan(canonicalSubscriptionPlans(), "plan_ai_creator_996")
	if err != nil {
		t.Fatal(err)
	}
	if plan.ID != "plan_ai_creator_996" || plan.DurationDays != 365 || planMemberLevel(plan) != memberLevelPro {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if planTokenGrantAmount(plan) != 40000 {
		t.Fatalf("fixture must retain paid-plan 40000 point entitlement, got %d", planTokenGrantAmount(plan))
	}
}

func TestAdminMembershipUsesCanonicalDefaultTenant(t *testing.T) {
	if adminMembershipTenantID != "tenant_default" {
		t.Fatalf("admin membership tenant = %q, want tenant_default", adminMembershipTenantID)
	}
}

func TestDecodeAdminPointMutationAllowsExplicitGiftValidityAndMembershipEnvelope(t *testing.T) {
	gift := httptest.NewRequest("POST", "/", strings.NewReader(`{"points":1000,"validityDays":365,"reason":"客户赠送","idempotencyKey":"gift-1"}`))
	giftReq, err := decodeAdminPointMutation(gift)
	if err != nil {
		t.Fatal(err)
	}
	if giftReq.Points != 1000 || giftReq.ValidityDays != 365 {
		t.Fatalf("unexpected gift request: %+v", giftReq)
	}

	membership := httptest.NewRequest("POST", "/", strings.NewReader(`{"points":0,"reason":"客户赠送","idempotencyKey":"member-1","membership":{"planId":"plan_ai_creator_996","durationDays":365,"reason":"客户赠送","idempotencyKey":"member-1"}}`))
	memberReq, err := decodeAdminPointMutation(membership)
	if err != nil {
		t.Fatal(err)
	}
	if memberReq.Membership == nil || memberReq.Membership.PlanID != "plan_ai_creator_996" || memberReq.Membership.DurationDays != 365 {
		t.Fatalf("unexpected membership request: %+v", memberReq)
	}
}

func TestDecodeAdminPointMutationStillRejectsClientExpiresAt(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"points":1000,"expiresAt":"2099-01-01T00:00:00Z","reason":"x","idempotencyKey":"gift-1"}`))
	if _, err := decodeAdminPointMutation(r); err == nil {
		t.Fatal("raw expiresAt must remain server-controlled")
	}
}

func TestAdminGiftReferenceIncludesValidityForIdempotencyFingerprint(t *testing.T) {
	if got := adminGiftReferenceID("gift-1", 365); got != "gift-1:validity-days:365" {
		t.Fatalf("unexpected reference id: %s", got)
	}
	if got := adminGiftReferenceID("gift-1", 0); got != "gift-1" {
		t.Fatalf("unexpected default reference id: %s", got)
	}
}
