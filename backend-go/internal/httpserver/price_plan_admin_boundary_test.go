package httpserver

import (
	"bytes"
	"net/http/httptest"
	"testing"
	"time"
)

func TestApplyPricePlanUpdateExplicitlyClearsValidityBounds(t *testing.T) {
	now := time.Date(2026, time.July, 28, 8, 0, 0, 0, time.UTC)
	current := validDraftPricePlanForBoundaryTest(now)
	revision := int64(1)
	request := httptest.NewRequest("PATCH", "/api/v1/admin/price-plans/price_plan_test", bytes.NewBufferString(`{
		"revision":1,
		"clearValidFrom":true,
		"clearValidUntil":true,
		"changeReason":"remove validity window"
	}`))
	var mutation pricePlanUpdateMutation
	if err := decodeStrictJSON(request, &mutation); err != nil {
		t.Fatalf("explicit validity clear fields were rejected by PATCH decoding: %v", err)
	}
	mutation.Revision = &revision

	updated, economicChanged, err := applyPricePlanUpdate(current, mutation)
	if err != nil {
		t.Fatalf("explicit validity clear failed: %v", err)
	}
	if updated.ValidFrom != nil || updated.ValidUntil != nil {
		t.Fatalf("validity bounds were not cleared: from=%v until=%v", updated.ValidFrom, updated.ValidUntil)
	}
	if !economicChanged {
		t.Fatal("clearing validity bounds must be treated as an economic change")
	}
}

func TestApplyPricePlanUpdateRejectsClearAndValueForSameValidityBound(t *testing.T) {
	now := time.Date(2026, time.July, 28, 8, 0, 0, 0, time.UTC)
	current := validDraftPricePlanForBoundaryTest(now)
	tests := []struct {
		name string
		body string
	}{
		{
			name: "valid from",
			body: `{"revision":1,"validFrom":"2026-07-29T08:00:00Z","clearValidFrom":true,"changeReason":"invalid mutation"}`,
		},
		{
			name: "valid until",
			body: `{"revision":1,"validUntil":"2026-07-30T08:00:00Z","clearValidUntil":true,"changeReason":"invalid mutation"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest("PATCH", "/api/v1/admin/price-plans/price_plan_test", bytes.NewBufferString(tt.body))
			var mutation pricePlanUpdateMutation
			if err := decodeStrictJSON(request, &mutation); err != nil {
				t.Fatalf("PATCH decoding failed before conflict validation: %v", err)
			}
			_, _, err := applyPricePlanUpdate(current, mutation)
			if err == nil {
				t.Fatal("clear flag and explicit value were accepted together")
			}
			coded, ok := err.(interface{ BusinessCode() string })
			if !ok || coded.BusinessCode() != "PRICE_PLAN_VALIDITY_MUTATION_CONFLICT" {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidatePricePlanCodeRejectsAdjacentPriceAmountsWithoutRejectingIdentifiers(t *testing.T) {
	for _, code := range []string{
		"member_rmb996",
		"campaign_price100",
		"agent_amount199",
		"member_yuan1",
		"member_1yuan",
		"member_yuan_1",
		"member_yuan_offer",
	} {
		if err := validatePricePlanCode(code); err == nil {
			t.Errorf("price-semantic code %q was accepted", code)
		}
	}

	for _, code := range []string{"plan_2026", "member_v10"} {
		if err := validatePricePlanCode(code); err != nil {
			t.Errorf("identifier code %q was rejected: %v", code, err)
		}
	}
}

func validDraftPricePlanForBoundaryTest(now time.Time) pricePlanAdminView {
	validFrom := now.Add(-time.Hour)
	validUntil := now.Add(time.Hour)
	return pricePlanAdminView{
		ID: "price_plan_test", PlanID: "plan_member", PlanVersionID: "version_member",
		Code: "member_normal", Name: "Member normal", storedKind: "NORMAL", Kind: "NORMAL",
		Channel: "WECHAT_VIRTUAL", Environment: "PRODUCTION", Currency: "CNY",
		SalePriceCents: 99600, ListPriceCents: 99600, ValidFrom: &validFrom, ValidUntil: &validUntil,
		AudienceType: "PUBLIC", AudienceRule: map[string]any{}, IsVisible: true, Status: "DRAFT", Revision: 1,
	}
}
