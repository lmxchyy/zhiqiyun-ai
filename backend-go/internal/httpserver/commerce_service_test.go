package httpserver

import "testing"

func TestCalculateCommissionSettlementFixedRules(t *testing.T) {
	tests := []struct {
		name           string
		ctx            commissionOrderContext
		wantDirect     int
		wantParent     int
		wantOperation  int
		wantTokenValue int
		wantPlatform   int
	}{
		{
			name: "direct member package",
			ctx: commissionOrderContext{
				OrderType:            orderTypeUserRechargeDirect,
				PlanType:             planTypeMemberPackage,
				AmountCents:          99600,
				DirectAgentID:        "channel_b",
				OperationCenterID:    "operation_center_1",
				TokenGrantAmount:     40000,
				TokenGrantValueCents: 40000,
			},
			wantDirect:     30000,
			wantOperation:  20000,
			wantTokenValue: 40000,
			wantPlatform:   9600,
		},
		{
			name: "second level member package",
			ctx: commissionOrderContext{
				OrderType:            orderTypeUserRechargeSecondLevel,
				PlanType:             planTypeMemberPackage,
				AmountCents:          99600,
				DirectAgentID:        "channel_b",
				ParentAgentID:        "channel_a",
				OperationCenterID:    "operation_center_1",
				TokenGrantAmount:     40000,
				TokenGrantValueCents: 40000,
			},
			wantDirect:     30000,
			wantParent:     5000,
			wantOperation:  20000,
			wantTokenValue: 40000,
			wantPlatform:   4600,
		},
		{
			name: "agent join does not pay parent agent",
			ctx: commissionOrderContext{
				OrderType:            orderTypeAgentJoin,
				PlanType:             planTypeAgentJoinPackage,
				AmountCents:          99600,
				DirectAgentID:        "channel_b",
				ParentAgentID:        "channel_a",
				OperationCenterID:    "operation_center_1",
				TokenGrantAmount:     20000,
				TokenGrantValueCents: 20000,
			},
			wantDirect:     30000,
			wantParent:     0,
			wantOperation:  20000,
			wantTokenValue: 20000,
			wantPlatform:   29600,
		},
		{
			name: "operation center join is platform income",
			ctx: commissionOrderContext{
				OrderType:   orderTypeOperationCenterJoin,
				PlanType:    planTypeOperationCenterPackage,
				AmountCents: 500000,
			},
			wantPlatform: 500000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := calculateCommissionSettlement(tt.ctx)
			if err != nil {
				t.Fatalf("calculateCommissionSettlement() error = %v", err)
			}
			if got.DirectAgentRewardCents != tt.wantDirect ||
				got.ParentAgentRewardCents != tt.wantParent ||
				got.OperationCenterRewardCents != tt.wantOperation ||
				got.TokenGrantValueCents != tt.wantTokenValue ||
				got.PlatformIncomeCents != tt.wantPlatform {
				t.Fatalf("settlement = %+v", got)
			}
		})
	}
}

func TestPlanTokenGrantAmountFollowsRightsValueForIdentityPackages(t *testing.T) {
	memberPlan := adminPlan{
		ID:          "legacy_member_996",
		PlanType:    planTypeMemberPackage,
		Points:      400,
		GrantPoints: 400,
		TokenAmount: 400,
		Entitlements: map[string]any{
			"planType":              planTypeMemberPackage,
			"tokenGrantAmount":      400,
			"tokenRightsValueCents": 40000,
			"businessDescription":   "400 CNY AI token rights",
		},
	}
	if got := planTokenGrantAmount(memberPlan); got != 40000 {
		t.Fatalf("member package token grant = %d, want 40000", got)
	}

	agentPlan := adminPlan{
		ID:          "legacy_agent_996",
		PlanType:    planTypeAgentJoinPackage,
		Points:      200,
		GrantPoints: 200,
		TokenAmount: 200,
		Entitlements: map[string]any{
			"planType":              planTypeAgentJoinPackage,
			"tokenGrantAmount":      200,
			"tokenRightsValueCents": 20000,
		},
	}
	if got := planTokenGrantAmount(agentPlan); got != 20000 {
		t.Fatalf("agent package token grant = %d, want 20000", got)
	}

	rechargePlan := adminPlan{
		ID:          "legacy_recharge",
		PlanType:    planTypeTokenRecharge,
		TokenAmount: 2500,
		Entitlements: map[string]any{
			"planType":              "recharge",
			"tokenRightsValueCents": 999999,
		},
	}
	if got := planTokenGrantAmount(rechargePlan); got != 2500 {
		t.Fatalf("recharge package token grant = %d, want explicit 2500", got)
	}
}

func TestCommerceMemberPackageFulfillmentIgnoresLegacyLowGrantSnapshot(t *testing.T) {
	now := "2026-07-05T00:00:00Z"
	data := adminPlatformData{
		Users: []adminUser{
			{ID: "user_u", Name: "U", Role: "MEMBER", Status: "ACTIVE"},
		},
		Plans: canonicalBillingPlans(),
		PointAccounts: []adminPointAccount{
			{ID: "points_u", UserID: "user_u"},
		},
		Orders: []adminOrder{
			{
				ID:                "order_legacy_snapshot",
				UserID:            "user_u",
				PlanID:            "plan_ai_creator_996",
				AmountCents:       99600,
				Status:            "PAID",
				TokenAmount:       400,
				TokenGrantAmount:  400,
				FulfillmentStatus: "PENDING",
				BusinessOrderType: "USER_PACKAGE",
				CreatedAt:         now,
				PriceSnapshot:     map[string]any{"planType": planTypeMemberPackage, "tokenGrantAmount": 400, "tokenAmount": 400, "tokenGrantValueCents": 40000},
			},
		},
	}
	if err := applyCommerceOrderFulfillment(&data, &data.Orders[0], now); err != nil {
		t.Fatalf("applyCommerceOrderFulfillment() error = %v", err)
	}
	if data.Orders[0].TokenGrantAmount != 40000 || data.Orders[0].TokenAmount != 40000 {
		t.Fatalf("legacy snapshot was not normalized on fulfillment: %+v", data.Orders[0])
	}
	if len(data.TokenRecords) != 1 || data.TokenRecords[0].Amount != 40000 {
		t.Fatalf("token records = %+v, want one 40000 grant", data.TokenRecords)
	}
	if data.PointAccounts[0].Available != 40000 {
		t.Fatalf("point account available = %d, want 40000", data.PointAccounts[0].Available)
	}
}

func TestCommerceAgentJoinFulfillmentIsIdempotentAndSkipsParentReward(t *testing.T) {
	now := "2026-07-05T00:00:00Z"
	data := adminPlatformData{
		Users: []adminUser{
			{ID: "user_a", Name: "A", Role: "AGENT_L3", Status: "ACTIVE"},
			{ID: "user_b", Name: "B", Role: "AGENT_L2", Status: "ACTIVE"},
			{ID: "user_c", Name: "C", Role: "MEMBER", Status: "ACTIVE", ReferredBy: "user_b"},
			{ID: "user_oc", Name: "OC", Role: "OPERATION_CENTER", Status: "ACTIVE"},
		},
		Plans: canonicalBillingPlans(),
		PointAccounts: []adminPointAccount{
			{ID: "points_c", UserID: "user_c", Available: 10},
		},
		ChannelAgents: []adminChannelAgent{
			{ID: "channel_a", UserID: "user_a", Level: 3, Status: "ACTIVE", InviteCode: "A001", OperationCenterID: "operation_center_1"},
			{ID: "channel_b", UserID: "user_b", ParentID: "channel_a", Level: 2, Status: "ACTIVE", InviteCode: "B001", OperationCenterID: "operation_center_1"},
		},
		OperationCenters: []adminOperationCenter{
			{ID: "operation_center_1", UserID: "user_oc", Name: "OC", Status: "ACTIVE", CreatedAt: now},
		},
		Orders: []adminOrder{
			{
				ID:            "order_agent_c",
				UserID:        "user_c",
				PlanID:        "plan_agent_join_996",
				AmountCents:   99600,
				Status:        "PAID",
				PaidAt:        now,
				PriceSnapshot: orderPriceSnapshot(adminOrderMutation{UserID: "user_c", PlanID: "plan_agent_join_996", AmountCents: 99600}),
				CreatedAt:     now,
			},
		},
	}

	if err := applyCommerceOrderFulfillment(&data, &data.Orders[0], now); err != nil {
		t.Fatalf("applyCommerceOrderFulfillment() error = %v", err)
	}
	if err := applyCommerceOrderFulfillment(&data, &data.Orders[0], now); err != nil {
		t.Fatalf("second applyCommerceOrderFulfillment() error = %v", err)
	}

	assertCommissionAmount(t, data.Commissions, receiverTypeAgent, "channel_b", commissionTypeDirectAgentReward, 30000)
	assertNoCommission(t, data.Commissions, receiverTypeAgent, "channel_a", commissionTypeParentAgentReward)
	assertCommissionAmount(t, data.Commissions, receiverTypeOperationCenter, "operation_center_1", commissionTypeOperationCenterReward, 20000)
	assertCommissionAmount(t, data.Commissions, receiverTypePlatform, "platform", commissionTypePlatformIncome, 29600)

	if len(data.TokenRecords) != 1 || data.TokenRecords[0].Amount != 20000 || data.TokenRecords[0].BalanceAfter != 20010 {
		t.Fatalf("unexpected token records: %+v", data.TokenRecords)
	}
	if account := pointMap(data.PointAccounts)["user_c"]; account.Available != 20010 || account.TotalGranted != 20000 {
		t.Fatalf("unexpected point account: %+v", account)
	}
	userC := userMap(data.Users)["user_c"]
	if userC.AgentStatus != agentStatusActive || userC.Role != "MEMBER" {
		t.Fatalf("user C identity must stay user + agent profile, got %+v", userC)
	}
	agentC, ok := channelAgentForUser(data.ChannelAgents, "user_c")
	if !ok || agentC.ParentID != "channel_b" || agentC.OperationCenterID != "operation_center_1" || agentC.TokenRightsAmount != 20000 {
		t.Fatalf("unexpected user C agent: %+v, exists=%v", agentC, ok)
	}
	if len(data.Commissions) != 3 {
		t.Fatalf("commissions should be idempotent, got %d: %+v", len(data.Commissions), data.Commissions)
	}
}

func TestCommerceMemberPackageFulfillmentDoesNotOpenAgentIdentity(t *testing.T) {
	now := "2026-07-05T00:00:00Z"
	data := adminPlatformData{
		Users: []adminUser{
			{ID: "user_a", Name: "A", Role: "AGENT_L2", Status: "ACTIVE"},
			{ID: "user_u", Name: "U", Role: "MEMBER", Status: "ACTIVE", ReferredBy: "user_a"},
			{ID: "user_oc", Name: "OC", Role: "OPERATION_CENTER", Status: "ACTIVE"},
		},
		Plans: canonicalBillingPlans(),
		ChannelAgents: []adminChannelAgent{
			{ID: "channel_a", UserID: "user_a", Level: 2, Status: "ACTIVE", InviteCode: "A001", OperationCenterID: "operation_center_1"},
		},
		OperationCenters: []adminOperationCenter{
			{ID: "operation_center_1", UserID: "user_oc", Name: "OC", Status: "ACTIVE", CreatedAt: now},
		},
		Orders: []adminOrder{
			{
				ID:            "order_member_u",
				UserID:        "user_u",
				PlanID:        "plan_ai_creator_996",
				AmountCents:   99600,
				Status:        "PAID",
				PaidAt:        now,
				PriceSnapshot: orderPriceSnapshot(adminOrderMutation{UserID: "user_u", PlanID: "plan_ai_creator_996", AmountCents: 99600}),
				CreatedAt:     now,
			},
		},
	}

	if err := applyCommerceOrderFulfillment(&data, &data.Orders[0], now); err != nil {
		t.Fatalf("applyCommerceOrderFulfillment() error = %v", err)
	}
	if err := applyCommerceOrderFulfillment(&data, &data.Orders[0], now); err != nil {
		t.Fatalf("second applyCommerceOrderFulfillment() error = %v", err)
	}

	if data.Orders[0].OrderType != orderTypeUserRechargeDirect || data.Orders[0].PlatformIncomeCents != 9600 || data.Orders[0].TokenGrantAmount != 40000 {
		t.Fatalf("unexpected order fulfillment: %+v", data.Orders[0])
	}
	if data.Orders[0].BuyerUserID != "user_u" || data.Orders[0].TokenAmount != 40000 || stringValue(data.Orders[0].RewardSnapshot["businessOrderType"]) != "USER_PACKAGE" {
		t.Fatalf("unexpected order identity snapshot: %+v", data.Orders[0])
	}
	assertCommissionAmount(t, data.Commissions, receiverTypeAgent, "channel_a", commissionTypeDirectAgentReward, 30000)
	assertCommissionAmount(t, data.Commissions, receiverTypeOperationCenter, "operation_center_1", commissionTypeOperationCenterReward, 20000)
	assertCommissionAmount(t, data.Commissions, receiverTypePlatform, "platform", commissionTypePlatformIncome, 9600)
	assertNoCommission(t, data.Commissions, receiverTypeAgent, "channel_a", commissionTypeParentAgentReward)
	if len(data.Commissions) != 3 {
		t.Fatalf("commissions should be idempotent, got %d: %+v", len(data.Commissions), data.Commissions)
	}

	if len(data.TokenRecords) != 1 || data.TokenRecords[0].Amount != 40000 || data.TokenRecords[0].ChangeType != "MEMBER_PACKAGE_GRANT" {
		t.Fatalf("unexpected token records: %+v", data.TokenRecords)
	}
	if account := pointMap(data.PointAccounts)["user_u"]; account.Available != 40000 || account.TotalGranted != 40000 {
		t.Fatalf("unexpected point account: %+v", account)
	}
	userU := userMap(data.Users)["user_u"]
	if userU.MemberLevel != memberLevelPro || userU.AgentStatus != agentStatusNone || userU.OperationCenterStatus != operationStatusNone {
		t.Fatalf("unexpected user identity: %+v", userU)
	}
	if agent, ok := channelAgentForUser(data.ChannelAgents, "user_u"); ok {
		t.Fatalf("member package must not open agent identity: %+v", agent)
	}
}

func TestCommerceSecondLevelMemberPackageUsesTwoLevelRewards(t *testing.T) {
	now := "2026-07-05T00:00:00Z"
	data := adminPlatformData{
		Users: []adminUser{
			{ID: "user_parent", Name: "A", Role: "MEMBER", Status: "ACTIVE"},
			{ID: "user_direct", Name: "B", Role: "MEMBER", Status: "ACTIVE"},
			{ID: "user_customer", Name: "U", Role: "MEMBER", Status: "ACTIVE", ReferredBy: "user_direct"},
			{ID: "user_oc", Name: "OC", Role: "OPERATION_CENTER", Status: "ACTIVE"},
		},
		Plans: canonicalBillingPlans(),
		ChannelAgents: []adminChannelAgent{
			{ID: "channel_parent", UserID: "user_parent", Level: 2, Status: "ACTIVE", InviteCode: "A001", OperationCenterID: "operation_center_1"},
			{ID: "channel_direct", UserID: "user_direct", ParentID: "channel_parent", Level: 2, Status: "ACTIVE", InviteCode: "B001", OperationCenterID: "operation_center_1"},
		},
		OperationCenters: []adminOperationCenter{
			{ID: "operation_center_1", UserID: "user_oc", Name: "OC", Status: "ACTIVE", CreatedAt: now},
		},
		Orders: []adminOrder{
			{
				ID:            "order_member_customer",
				UserID:        "user_customer",
				PlanID:        "plan_ai_creator_996",
				AmountCents:   99600,
				Status:        "PAID",
				PaidAt:        now,
				PriceSnapshot: orderPriceSnapshot(adminOrderMutation{UserID: "user_customer", PlanID: "plan_ai_creator_996", AmountCents: 99600}),
				CreatedAt:     now,
			},
		},
	}

	if err := applyCommerceOrderFulfillment(&data, &data.Orders[0], now); err != nil {
		t.Fatalf("applyCommerceOrderFulfillment() error = %v", err)
	}

	assertCommissionAmount(t, data.Commissions, receiverTypeAgent, "channel_direct", commissionTypeDirectAgentReward, 30000)
	assertCommissionAmount(t, data.Commissions, receiverTypeAgent, "channel_parent", commissionTypeParentAgentReward, 5000)
	assertCommissionAmount(t, data.Commissions, receiverTypeOperationCenter, "operation_center_1", commissionTypeOperationCenterReward, 20000)
	assertCommissionAmount(t, data.Commissions, receiverTypePlatform, "platform", commissionTypePlatformIncome, 4600)
	if stringValue(data.Orders[0].RewardSnapshot["agentSelfAIUsageBillingScope"]) != "USER_IDENTITY" {
		t.Fatalf("reward snapshot must declare user-wallet AI usage scope: %+v", data.Orders[0].RewardSnapshot)
	}
}

func TestCommerceOperationCenterJoinFulfillmentIsPlatformIncome(t *testing.T) {
	now := "2026-07-05T00:00:00Z"
	data := adminPlatformData{
		Users: []adminUser{
			{ID: "user_oc", Name: "运营用户", Role: "MEMBER", Status: "ACTIVE"},
		},
		Plans: canonicalBillingPlans(),
		Orders: []adminOrder{
			{
				ID:            "order_operation_center",
				UserID:        "user_oc",
				PlanID:        "plan_operation_center_5000",
				AmountCents:   500000,
				Status:        "PAID",
				PaidAt:        now,
				PriceSnapshot: orderPriceSnapshot(adminOrderMutation{UserID: "user_oc", PlanID: "plan_operation_center_5000", AmountCents: 500000}),
				CreatedAt:     now,
			},
		},
	}

	if err := applyCommerceOrderFulfillment(&data, &data.Orders[0], now); err != nil {
		t.Fatalf("applyCommerceOrderFulfillment() error = %v", err)
	}
	if err := applyCommerceOrderFulfillment(&data, &data.Orders[0], now); err != nil {
		t.Fatalf("second applyCommerceOrderFulfillment() error = %v", err)
	}

	if data.Orders[0].OrderType != orderTypeOperationCenterJoin || data.Orders[0].PlatformIncomeCents != 500000 || data.Orders[0].TokenGrantAmount != 0 {
		t.Fatalf("unexpected order fulfillment: %+v", data.Orders[0])
	}
	assertCommissionAmount(t, data.Commissions, receiverTypePlatform, "platform", commissionTypePlatformIncome, 500000)
	assertNoCommission(t, data.Commissions, receiverTypeAgent, "", commissionTypeDirectAgentReward)
	assertNoCommission(t, data.Commissions, receiverTypeOperationCenter, "", commissionTypeOperationCenterReward)
	if len(data.TokenRecords) != 0 {
		t.Fatalf("operation center join should not grant tokens by default: %+v", data.TokenRecords)
	}
	userOC := userMap(data.Users)["user_oc"]
	if userOC.OperationCenterStatus != operationStatusActive || userOC.AgentStatus == agentStatusActive {
		t.Fatalf("unexpected user identity: %+v", userOC)
	}
	center, ok := operationCenterForUser(data.OperationCenters, "user_oc")
	if !ok || center.Status != "ACTIVE" || center.JoinOrderID != "order_operation_center" || center.JoinFeeCents != 500000 {
		t.Fatalf("unexpected operation center: %+v, exists=%v", center, ok)
	}
	if len(data.Commissions) != 1 {
		t.Fatalf("commissions should be idempotent, got %d: %+v", len(data.Commissions), data.Commissions)
	}
}

func TestSettlementAmountMismatchReturnsError(t *testing.T) {
	err := validateSettlementAmount(99600, commissionSettlementResult{
		DirectAgentRewardCents:     30000,
		OperationCenterRewardCents: 20000,
		TokenGrantValueCents:       40000,
		PlatformIncomeCents:        9601,
	})
	if err == nil {
		t.Fatal("validateSettlementAmount() expected mismatch error")
	}
}

func assertCommissionAmount(t *testing.T, items []adminCommission, receiverType string, receiverID string, commissionType string, amount int) {
	t.Helper()
	for _, item := range items {
		if item.ReceiverType == receiverType && item.ReceiverID == receiverID && item.CommissionType == commissionType {
			if item.AmountCents != amount {
				t.Fatalf("%s/%s/%s amount = %d, want %d", receiverType, receiverID, commissionType, item.AmountCents, amount)
			}
			return
		}
	}
	t.Fatalf("commission not found: receiverType=%s receiverID=%s commissionType=%s in %+v", receiverType, receiverID, commissionType, items)
}

func assertNoCommission(t *testing.T, items []adminCommission, receiverType string, receiverID string, commissionType string) {
	t.Helper()
	for _, item := range items {
		if item.ReceiverType == receiverType && item.ReceiverID == receiverID && item.CommissionType == commissionType {
			t.Fatalf("unexpected commission: %+v", item)
		}
	}
}
