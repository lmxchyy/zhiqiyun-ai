package httpserver

import (
	"testing"

	channelrules "xianzhi-ai/backend-go/internal/app/channelrules"
)

func TestCompareCommerceShadowRecordsLegacyParentRewardWithoutCorrection(t *testing.T) {
	legacy := commissionSettlementResult{
		OrderType: orderTypeUserRechargeSecondLevel, DirectAgentRewardCents: 30000,
		ParentAgentRewardCents: 5000, OperationCenterRewardCents: 20000,
		TokenGrantAmount: 40000, TokenGrantValueCents: 40000, PlatformIncomeCents: 4600,
	}
	v132 := channelrules.OrderCalculation{
		Scenario: channelrules.ScenarioMemberPurchase, TokenGrantAmount: 40000,
		TokenRightsValueCents: 40000, DirectAgentAmountCents: 30000,
		OperationCenterAmountCents: 20000, PlatformAmountCents: 9600,
	}

	difference := compareCommerceShadow(legacy, v132)
	if difference.Match {
		t.Fatal("legacy parent commission must produce a shadow difference")
	}
	if difference.ParentAgentDeltaCents != -5000 || difference.PlatformDeltaCents != 5000 {
		t.Fatalf("unexpected difference: %+v", difference)
	}
	if legacy.ParentAgentRewardCents != 5000 || legacy.PlatformIncomeCents != 4600 {
		t.Fatalf("shadow comparison must not mutate legacy result: %+v", legacy)
	}
}
