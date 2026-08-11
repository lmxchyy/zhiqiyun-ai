package channelrules

import (
	"context"
	"testing"
	"time"

	commissionapp "xianzhi-ai/backend-go/internal/app/commission"
)

type shadowStubStore struct {
	stubRuleStore
	shadowBundle RuleBundle
}

func validMemberRuleBundle(effectiveAt time.Time) RuleBundle {
	return RuleBundle{
		RuleSet: CommercialRuleSet{
			ID: "rules-shadow-v1", TenantID: "tenant_default", Code: "CHANNEL_V132",
			Version: 1, Status: RuleSetDraft, EffectiveStartAt: effectiveAt.Add(-time.Hour),
		},
		Plan: PlanConfigVersion{
			ID: "plan-shadow-v1", TenantID: "tenant_default", RuleSetID: "rules-shadow-v1",
			PlanID: "plan_ai_creator_996", IdentityType: "MEMBER", PriceCents: 99600,
			TokenGrantAmount: 40000, TokenRightsValueCents: 40000,
		},
		Rules: []commissionapp.CommissionRule{
			serviceRule("agent", commissionapp.BeneficiaryAgent, 1, commissionapp.CalculationFixedAmount, 30000, 10, effectiveAt),
			serviceRule("operation", commissionapp.BeneficiaryOperationCenter, 1, commissionapp.CalculationFixedAmount, 20000, 20, effectiveAt),
			serviceRule("platform", commissionapp.BeneficiaryPlatform, 0, commissionapp.CalculationRemainderToPlatform, 0, 1000, effectiveAt),
		},
	}
}

func (s *shadowStubStore) LoadShadowRuleBundle(context.Context, RuleBundleQuery) (RuleBundle, error) {
	return s.shadowBundle, nil
}

func TestResolveShadowOrderUsesProvidedRelationshipWithoutSavingSnapshot(t *testing.T) {
	paidAt := time.Date(2026, time.July, 26, 9, 0, 0, 0, time.UTC)
	base := validMemberRuleBundle(paidAt)
	store := &shadowStubStore{shadowBundle: base}
	service := NewChannelRuleService(store)
	relationship := RelationshipSnapshot{
		SourceUserID: "member-1", DirectAgentID: "agent-direct", OperationCenterID: "oc-1",
		EffectiveAt: paidAt, SourceType: "LEGACY_FULFILLMENT_CONTEXT", SourceID: "order-1",
	}

	resolved, err := service.ResolveShadowOrder(context.Background(), ResolveOrderRequest{
		TenantID: "tenant_default", OrderID: "order-1", OrderNo: "ORDER1", PlanID: "plan_ai_creator_996",
		SourceUserID: "member-1", PaidAmountCents: 99600, BusinessTime: paidAt,
	}, relationship)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Relationship.DirectAgentID != "agent-direct" || resolved.Relationship.OperationCenterID != "oc-1" {
		t.Fatalf("unexpected relationship: %+v", resolved.Relationship)
	}
	if store.saved.OrderID != "" {
		t.Fatalf("shadow resolution must not persist an order snapshot: %+v", store.saved)
	}
}
