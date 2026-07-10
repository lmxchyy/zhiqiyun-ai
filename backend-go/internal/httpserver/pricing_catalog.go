package httpserver

import "strings"

func canonicalSubscriptionPlans() []adminPlan {
	return []adminPlan{
		{
			ID: "plan_free", Code: "trial", Name: "体验版", PlanType: planTypeMemberPackage, PriceCents: 0, Points: 100, GrantPoints: 100, TokenAmount: 0, MemberLevel: memberLevelFree, DurationDays: 7, Concurrency: 1, Active: true,
			Entitlements: map[string]any{"planType": "subscription", "audience": "新用户体验", "validityText": "7 天有效", "displayPrice": "免费", "sort": 10},
		},
		{
			ID: "plan_month", Code: "basic", Name: "Basic", PlanType: planTypeMemberPackage, PriceCents: 2900, Points: 4000, GrantPoints: 4000, TokenAmount: 4000, MemberLevel: memberLevelBasic, DurationDays: 30, Concurrency: 3, Active: true,
			Entitlements: map[string]any{"planType": "subscription", "billingCycle": "recurring_monthly", "audience": "个人、小商家", "validityText": "连续包月", "displayPrice": "29 元/月", "monthlyPoints": 4000, "newapiGroup": "basic_group", "sort": 20},
		},
		{
			ID: "plan_basic_single", Code: "basic_single", Name: "Basic 单月购买", PlanType: planTypeMemberPackage, PriceCents: 3900, Points: 4000, GrantPoints: 4000, TokenAmount: 4000, MemberLevel: memberLevelBasic, DurationDays: 30, Concurrency: 3, Active: true,
			Entitlements: map[string]any{"planType": "subscription", "billingCycle": "single_month", "audience": "个人、小商家", "validityText": "单月购买", "displayPrice": "39 元/月", "monthlyPoints": 4000, "newapiGroup": "basic_group", "sort": 21},
		},
		{
			ID: "plan_basic_year", Code: "basic_year", Name: "Basic 连续包年", PlanType: planTypeMemberPackage, PriceCents: 29900, Points: 4000, GrantPoints: 4000, TokenAmount: 4000, MemberLevel: memberLevelBasic, DurationDays: 365, Concurrency: 3, Active: true,
			Entitlements: map[string]any{"planType": "subscription", "billingCycle": "recurring_yearly", "audience": "个人、小商家", "validityText": "连续包年", "displayPrice": "299 元/年", "monthlyPoints": 4000, "newapiGroup": "basic_group", "sort": 22},
		},
		{
			ID: "plan_pro", Code: "pro", Name: "Pro", PlanType: planTypeMemberPackage, PriceCents: 13900, Points: 20000, GrantPoints: 20000, TokenAmount: 20000, MemberLevel: memberLevelPro, DurationDays: 30, Concurrency: 8, Active: true,
			Entitlements: map[string]any{"planType": "subscription", "billingCycle": "recurring_monthly", "audience": "内容创作者、电商运营", "validityText": "连续包月", "displayPrice": "139 元/月", "monthlyPoints": 20000, "newapiGroup": "pro_group", "recommended": true, "sort": 30},
		},
		{
			ID: "plan_pro_single", Code: "pro_single", Name: "Pro 单月购买", PlanType: planTypeMemberPackage, PriceCents: 16900, Points: 20000, GrantPoints: 20000, TokenAmount: 20000, MemberLevel: memberLevelPro, DurationDays: 30, Concurrency: 8, Active: true,
			Entitlements: map[string]any{"planType": "subscription", "billingCycle": "single_month", "audience": "内容创作者、电商运营", "validityText": "单月购买", "displayPrice": "169 元/月", "monthlyPoints": 20000, "newapiGroup": "pro_group", "recommended": true, "sort": 31},
		},
		{
			ID: "plan_pro_year", Code: "pro_year", Name: "Pro 连续包年", PlanType: planTypeMemberPackage, PriceCents: 149900, Points: 20000, GrantPoints: 20000, TokenAmount: 20000, MemberLevel: memberLevelPro, DurationDays: 365, Concurrency: 8, Active: true,
			Entitlements: map[string]any{"planType": "subscription", "billingCycle": "recurring_yearly", "audience": "内容创作者、电商运营", "validityText": "连续包年", "displayPrice": "1499 元/年", "monthlyPoints": 20000, "newapiGroup": "pro_group", "recommended": true, "sort": 32},
		},
		{
			ID: "plan_year", Code: "ultimate", Name: "Ultimate", PlanType: planTypeMemberPackage, PriceCents: 69900, Points: 100000, GrantPoints: 100000, TokenAmount: 100000, MemberLevel: memberLevelEnterprise, DurationDays: 30, Concurrency: 20, Active: true,
			Entitlements: map[string]any{"planType": "subscription", "billingCycle": "recurring_monthly", "audience": "工作室、代运营团队", "validityText": "连续包月", "displayPrice": "699 元/月", "monthlyPoints": 100000, "newapiGroup": "ultimate_group", "sort": 40},
		},
		{
			ID: "plan_ultimate_single", Code: "ultimate_single", Name: "Ultimate 单月购买", PlanType: planTypeMemberPackage, PriceCents: 89900, Points: 100000, GrantPoints: 100000, TokenAmount: 100000, MemberLevel: memberLevelEnterprise, DurationDays: 30, Concurrency: 20, Active: true,
			Entitlements: map[string]any{"planType": "subscription", "billingCycle": "single_month", "audience": "工作室、代运营团队", "validityText": "单月购买", "displayPrice": "899 元/月", "monthlyPoints": 100000, "newapiGroup": "ultimate_group", "sort": 41},
		},
		{
			ID: "plan_ultimate_year", Code: "ultimate_year", Name: "Ultimate 连续包年", PlanType: planTypeMemberPackage, PriceCents: 799900, Points: 100000, GrantPoints: 100000, TokenAmount: 100000, MemberLevel: memberLevelEnterprise, DurationDays: 365, Concurrency: 20, Active: true,
			Entitlements: map[string]any{"planType": "subscription", "billingCycle": "recurring_yearly", "audience": "工作室、代运营团队", "validityText": "连续包年", "displayPrice": "7999 元/年", "monthlyPoints": 100000, "newapiGroup": "ultimate_group", "sort": 42},
		},
		{
			ID: "plan_enterprise", Code: "enterprise", Name: "企业版", PlanType: planTypeMemberPackage, PriceCents: 0, Points: 0, GrantPoints: 0, TokenAmount: 0, MemberLevel: memberLevelEnterprise, DurationDays: 365, Concurrency: 0, Active: true,
			Entitlements: map[string]any{"planType": "subscription", "billingCycle": "custom", "audience": "企业客户、代理商", "validityText": "定制", "displayPrice": "定制", "customPrice": true, "customPoints": true, "monthlyPoints": "custom", "newapiGroup": "enterprise_group", "sort": 50},
		},
		{
			ID: "plan_ai_creator_996", Code: "ai_creator_996", Name: "996 AI 创作会员包", PlanType: planTypeMemberPackage, PriceCents: 99600, Points: 40000, GrantPoints: 40000, TokenAmount: 40000, MemberLevel: memberLevelPro, DurationDays: 365, Concurrency: 8, Active: true,
			Entitlements: map[string]any{"planType": planTypeMemberPackage, "productType": planTypeMemberPackage, "displayPrice": "996 元", "tokenRightsValueCents": 40000, "tokenGrantAmount": 40000, "memberLevel": memberLevelPro, "newapiGroup": "pro_group", "businessDescription": "获得 400 元 AI 点数 / Token，不包含代理商资格", "sort": 1},
		},
	}
}

func canonicalRechargePackages() []adminPlan {
	return []adminPlan{
		{
			ID: "recharge_100", Code: "recharge_100", Name: "100 元点数包", PlanType: planTypeTokenRecharge, PriceCents: 10000, Points: 10000, GrantPoints: 10000, TokenAmount: 10000, DurationDays: 730, Concurrency: 0, Active: true,
			Entitlements: map[string]any{"planType": "recharge", "displayPrice": "100 元", "validityText": "充值 10,000 点", "sort": 101},
		},
		{
			ID: "recharge_400", Code: "recharge_400", Name: "400 元点数包", PlanType: planTypeTokenRecharge, PriceCents: 40000, Points: 40000, GrantPoints: 40000, TokenAmount: 40000, DurationDays: 730, Concurrency: 0, Active: true,
			Entitlements: map[string]any{"planType": "recharge", "displayPrice": "400 元", "validityText": "充值 40,000 点", "recommended": true, "sort": 102},
		},
		{
			ID: "recharge_small", Code: "small_pack", Name: "小额包", PlanType: planTypeTokenRecharge, PriceCents: 1990, Points: 2500, GrantPoints: 2500, TokenAmount: 2500, DurationDays: 730, Concurrency: 0, Active: true,
			Entitlements: map[string]any{"planType": "recharge", "displayPrice": "19.9 元", "validityText": "充值点数", "sort": 110},
		},
		{
			ID: "recharge_standard", Code: "standard_pack", Name: "标准包", PlanType: planTypeTokenRecharge, PriceCents: 9900, Points: 15000, GrantPoints: 15000, TokenAmount: 15000, DurationDays: 730, Concurrency: 0, Active: true,
			Entitlements: map[string]any{"planType": "recharge", "displayPrice": "99 元", "validityText": "充值点数", "sort": 120},
		},
		{
			ID: "recharge_business", Code: "business_pack", Name: "商业包", PlanType: planTypeTokenRecharge, PriceCents: 29900, Points: 50000, GrantPoints: 50000, TokenAmount: 50000, DurationDays: 730, Concurrency: 0, Active: true,
			Entitlements: map[string]any{"planType": "recharge", "displayPrice": "299 元", "validityText": "充值点数", "sort": 130},
		},
		{
			ID: "recharge_enterprise", Code: "enterprise_pack", Name: "企业包", PlanType: planTypeTokenRecharge, PriceCents: 99900, Points: 200000, GrantPoints: 200000, TokenAmount: 200000, DurationDays: 730, Concurrency: 0, Active: true,
			Entitlements: map[string]any{"planType": "recharge", "displayPrice": "999 元", "validityText": "充值点数", "sort": 140},
		},
	}
}

func canonicalIdentityPlans() []adminPlan {
	return []adminPlan{
		{
			ID: "plan_agent_join_996", Code: "agent_join_996", Name: "996 代理商开通包", PlanType: planTypeAgentJoinPackage, PriceCents: 99600, Points: 20000, GrantPoints: 20000, TokenAmount: 20000, AgentLevel: "AGENT", DurationDays: 365, Concurrency: 0, Active: true,
			Entitlements: map[string]any{"planType": planTypeAgentJoinPackage, "productType": planTypeAgentJoinPackage, "displayPrice": "996 元", "tokenRightsValueCents": 20000, "tokenGrantAmount": 20000, "opensAgent": true, "agentLevel": "AGENT", "businessDescription": "开通代理商身份，并获得 200 元 AI 点数 / Token", "sort": 2},
		},
		{
			ID: "plan_operation_center_5000", Code: "operation_center_5000", Name: "5000 运营中心开通包", PlanType: planTypeOperationCenterPackage, PriceCents: 500000, Points: 0, GrantPoints: 0, TokenAmount: 0, DurationDays: 365, Concurrency: 0, Active: true,
			Entitlements: map[string]any{"planType": planTypeOperationCenterPackage, "productType": planTypeOperationCenterPackage, "displayPrice": "5000 元", "tokenRightsValueCents": 0, "tokenGrantAmount": 0, "opensOperationCenter": true, "businessDescription": "开通运营中心身份，默认平台收入 5000 元", "sort": 3},
		},
	}
}

func canonicalBillingPlans() []adminPlan {
	items := append([]adminPlan{}, canonicalSubscriptionPlans()...)
	items = append(items, canonicalRechargePackages()...)
	items = append(items, canonicalIdentityPlans()...)
	return items
}

func planCatalogByID(id string) (adminPlan, bool) {
	id = strings.TrimSpace(id)
	for _, item := range canonicalBillingPlans() {
		if item.ID == id {
			return item, true
		}
	}
	return adminPlan{}, false
}

func rechargePackageByOrder(order adminOrder) (adminPlan, bool) {
	if item, ok := planCatalogByID(order.PlanID); ok && strings.EqualFold(stringValue(item.Entitlements["planType"]), "recharge") {
		return item, true
	}
	return adminPlan{}, false
}

func rechargePackageIDForAmount(amountCents int) string {
	for _, item := range canonicalRechargePackages() {
		if planPrice(item) == amountCents {
			return item.ID
		}
	}
	return ""
}

func mergeCanonicalPlans(existing []adminPlan) []adminPlan {
	result := append([]adminPlan{}, existing...)
	for _, plan := range canonicalBillingPlans() {
		replaced := false
		for i := range result {
			if result[i].ID == plan.ID {
				result[i] = plan
				replaced = true
				break
			}
		}
		if !replaced {
			result = append(result, plan)
		}
	}
	return result
}
