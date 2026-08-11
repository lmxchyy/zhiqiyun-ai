package channelrules

import (
	"context"
	"strings"
	"testing"
	"time"

	commissionapp "xianzhi-ai/backend-go/internal/app/commission"
)

type stubRuleStore struct {
	bundle       RuleBundle
	relationship RelationshipSnapshot
	saved        OrderRuleSnapshot
	published    bool
}

func (s *stubRuleStore) LoadEffectiveRuleBundle(context.Context, RuleBundleQuery) (RuleBundle, error) {
	return s.bundle, nil
}

func (s *stubRuleStore) ResolveRelationshipSnapshot(context.Context, RelationshipQuery) (RelationshipSnapshot, error) {
	return s.relationship, nil
}

func (s *stubRuleStore) SaveOrderRuleSnapshot(_ context.Context, snapshot OrderRuleSnapshot) error {
	s.saved = snapshot
	return nil
}

func (s *stubRuleStore) LoadRuleBundleByID(context.Context, string, string) (RuleBundle, error) {
	return s.bundle, nil
}

func (s *stubRuleStore) PublishRuleBundle(context.Context, PublishRuleSetRequest) error {
	s.published = true
	return nil
}

func TestChannelRuleServiceResolvesAndPersistsImmutableOrderSnapshot(t *testing.T) {
	paidAt := time.Date(2026, time.July, 26, 8, 0, 0, 0, time.UTC)
	store := &stubRuleStore{
		bundle: RuleBundle{
			RuleSet: CommercialRuleSet{ID: "rules-v1", TenantID: "tenant_default", Code: "CHANNEL_V132", Version: 1, Status: RuleSetPublished, EffectiveStartAt: paidAt.Add(-time.Hour)},
			Plan: PlanConfigVersion{
				ID: "plan-version-1", TenantID: "tenant_default", RuleSetID: "rules-v1", PlanID: "plan_ai_creator_996",
				IdentityType: "MEMBER", PriceCents: 99600, TokenRightsValueCents: 40000,
			},
			Rules: []commissionapp.CommissionRule{
				serviceRule("agent", commissionapp.BeneficiaryAgent, 1, commissionapp.CalculationFixedAmount, 30000, 10, paidAt),
				serviceRule("operation", commissionapp.BeneficiaryOperationCenter, 1, commissionapp.CalculationFixedAmount, 20000, 20, paidAt),
				serviceRule("platform", commissionapp.BeneficiaryPlatform, 0, commissionapp.CalculationRemainderToPlatform, 0, 1000, paidAt),
			},
		},
		relationship: RelationshipSnapshot{DirectAgentID: "agent-direct", OperationCenterID: "oc-1", EffectiveAt: paidAt},
	}
	service := NewChannelRuleService(store)

	resolved, err := service.ResolveOrder(context.Background(), ResolveOrderRequest{
		TenantID: "tenant_default", OrderID: "order-1", OrderNo: "ORDER1", PlanID: "plan_ai_creator_996",
		SourceUserID: "member-1", PaidAmountCents: 99600, BusinessTime: paidAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.RuleSet.ID != "rules-v1" || resolved.Plan.ID != "plan-version-1" {
		t.Fatalf("unexpected resolved bundle: %+v", resolved)
	}
	if store.saved.OrderID != "order-1" || store.saved.RuleSetID != "rules-v1" || store.saved.RuleSetVersion != 1 {
		t.Fatalf("snapshot was not persisted: %+v", store.saved)
	}
	if store.saved.DirectAgentID != "agent-direct" || store.saved.OperationCenterID != "oc-1" {
		t.Fatalf("relationship snapshot mismatch: %+v", store.saved)
	}
}

func TestChannelRuleServiceRejectsAncestorRuleBeforeSnapshot(t *testing.T) {
	paidAt := time.Date(2026, time.July, 26, 8, 0, 0, 0, time.UTC)
	store := &stubRuleStore{
		bundle: RuleBundle{
			RuleSet: CommercialRuleSet{ID: "rules-v1", TenantID: "tenant_default", Code: "CHANNEL_V132", Version: 1, Status: RuleSetPublished, EffectiveStartAt: paidAt.Add(-time.Hour)},
			Plan:    PlanConfigVersion{ID: "plan-version-1", TenantID: "tenant_default", RuleSetID: "rules-v1", PlanID: "plan_ai_creator_996", IdentityType: "MEMBER", PriceCents: 99600},
			Rules: []commissionapp.CommissionRule{
				serviceRule("ancestor", commissionapp.BeneficiaryAgent, 2, commissionapp.CalculationFixedAmount, 5000, 10, paidAt),
				serviceRule("platform", commissionapp.BeneficiaryPlatform, 0, commissionapp.CalculationRemainderToPlatform, 0, 1000, paidAt),
			},
		},
	}
	service := NewChannelRuleService(store)

	_, err := service.ResolveOrder(context.Background(), ResolveOrderRequest{
		TenantID: "tenant_default", OrderID: "order-2", OrderNo: "ORDER2", PlanID: "plan_ai_creator_996",
		SourceUserID: "member-2", PaidAmountCents: 99600, BusinessTime: paidAt,
	})
	if err == nil || !strings.Contains(err.Error(), ErrCodeAncestorCommissionForbidden) {
		t.Fatalf("expected ancestor rule rejection, got %v", err)
	}
	if store.saved.OrderID != "" {
		t.Fatalf("invalid rules must not be snapshotted: %+v", store.saved)
	}
}

func TestChannelRuleServicePublishesOnlyValidatedRuleBundle(t *testing.T) {
	effectiveAt := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	store := &stubRuleStore{bundle: RuleBundle{
		RuleSet: CommercialRuleSet{ID: "rules-v2", TenantID: "tenant_default", Code: "CHANNEL_V132", Version: 2, Status: RuleSetDraft, EffectiveStartAt: effectiveAt},
		Plan:    PlanConfigVersion{ID: "plan-version-2", TenantID: "tenant_default", RuleSetID: "rules-v2", PlanID: "plan_ai_creator_996", IdentityType: "MEMBER", PriceCents: 99600, TokenRightsValueCents: 40000},
		Rules: []commissionapp.CommissionRule{
			serviceRule("agent", commissionapp.BeneficiaryAgent, 1, commissionapp.CalculationFixedAmount, 30000, 10, effectiveAt),
			serviceRule("operation", commissionapp.BeneficiaryOperationCenter, 1, commissionapp.CalculationFixedAmount, 20000, 20, effectiveAt),
			serviceRule("platform", commissionapp.BeneficiaryPlatform, 0, commissionapp.CalculationRemainderToPlatform, 0, 1000, effectiveAt),
		},
		ReferralRules: []ReferralRewardRule{
			{ID: "ref-1", TenantID: "tenant_default", RuleSetID: "rules-v2", Code: "OC_REFERS_OC", Version: 1, ReferrerType: "OPERATION_CENTER", BeneficiaryType: "OPERATION_CENTER", BeneficiaryRelation: "REFERRER", AmountCents: 300000, FreezeDays: 7, Status: RuleSetDraft},
		},
	}}
	service := NewChannelRuleService(store)

	err := service.PublishRuleSet(context.Background(), PublishRuleSetRequest{
		TenantID: "tenant_default", RuleSetID: "rules-v2", OperatorID: "admin-1", PublishedAt: effectiveAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !store.published {
		t.Fatal("validated rule bundle was not published")
	}
}

func serviceRule(code string, beneficiary commissionapp.BeneficiaryType, level int, calculation commissionapp.CalculationType, fixed int64, priority int, effectiveAt time.Time) commissionapp.CommissionRule {
	return commissionapp.CommissionRule{
		ID: "rule-" + code, TenantID: "tenant_default", Code: strings.ToUpper(code), Name: code,
		ProductType: string(ScenarioMemberPurchase), BeneficiaryRole: beneficiary, RelationshipLevel: level,
		CalculationType: calculation, FixedAmountCents: commissionapp.AmountCents(fixed), Priority: priority,
		RefundPolicy: "REVERSE_OR_RECOVER", EffectiveStartAt: effectiveAt.Add(-time.Hour), Version: 1, Status: "ACTIVE",
	}
}
