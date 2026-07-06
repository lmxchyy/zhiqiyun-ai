package httpserver

import (
	"fmt"
	"time"
)

type agentLevelPolicy struct {
	Level                int
	Code                 string
	Name                 string
	Identity             string
	OpenMethod           string
	Audience             string
	Permissions          []string
	Limitations          []string
	MembershipCommission string
	RechargeCommission   string
	EnterpriseCommission string
	RegionalRebate       string
	OpenCondition        string
	KeepCondition        string
	ManualReview         bool
}

func agentLevelPolicies() []agentLevelPolicy {
	return []agentLevelPolicy{
		{
			Level: 0, Code: "L0", Name: "普通用户", Identity: "普通用户", OpenMethod: "注册即为普通用户", Audience: "普通使用 AI 工具的客户",
			Permissions: []string{"购买会员", "使用 AI 工具", "充值 AI 点数"},
			Limitations: []string{"不能推广返佣", "没有代理后台"},
			OpenCondition: "注册账号", KeepCondition: "无",
		},
		{
			Level: 1, Code: "L1", Name: "推广员", Identity: "轻量分销", OpenMethod: "申请即可/邀请开通", Audience: "个人推广",
			Permissions: []string{"专属邀请链接", "推广用户拿返佣", "查看推广订单"},
			Limitations: []string{"不可设置自己的价格", "不可开下级代理"},
			MembershipCommission: "10%-20%", RechargeCommission: "5%-10%", EnterpriseCommission: "按单谈",
			OpenCondition: "注册申请", KeepCondition: "无",
		},
		{
			Level: 2, Code: "L2", Name: "初级代理商", Identity: "正式代理", OpenMethod: "审核 + 代理套餐/业绩达标", Audience: "小团队、个人渠道",
			Permissions: []string{"代理后台", "客户管理", "订单管理", "佣金结算", "销售会员套餐", "销售点数包", "销售企业版线索"},
			Limitations: []string{"不能开二级代理，或者只能推荐给平台审核"},
			MembershipCommission: "20%-30%", RechargeCommission: "10%-15%", EnterpriseCommission: "10%-20%",
			OpenCondition: "累计销售 1000 元或缴纳 999 年费", KeepCondition: "月销售 ≥ 1000",
		},
		{
			Level: 3, Code: "L3", Name: "高级代理商", Identity: "区域/行业代理", OpenMethod: "销售额达标或人工签约", Audience: "企服老板、培训机构、代运营老板",
			Permissions: []string{"更高返佣", "企业客户报备", "专属客服支持", "申请独立品牌页", "申请行业解决方案报价", "发展一级推广员"},
			Limitations: []string{"只能发展一级推广员"},
			MembershipCommission: "30%-40%", RechargeCommission: "15%-25%", EnterpriseCommission: "20%-30%",
			OpenCondition: "累计销售 1 万或缴纳 2999 年费", KeepCondition: "月销售 ≥ 5000",
		},
		{
			Level: 4, Code: "L4", Name: "城市合伙人", Identity: "城市/区域运营商", OpenMethod: "合同签约 + 业绩承诺", Audience: "城市/区域渠道运营商",
			Permissions: []string{"城市区域保护", "独立代理站点", "自定义品牌名称/logo", "管理区域代理", "企业客户报价", "区域业绩返点", "培训和销售物料"},
			Limitations: []string{"不轻易开放，必须人工审核"},
			MembershipCommission: "40%左右", RechargeCommission: "20%-30%", EnterpriseCommission: "30%左右", RegionalRebate: "5%-10%",
			OpenCondition: "月销售 ≥ 5 万或合同签约", KeepCondition: "季度销售 ≥ 10 万", ManualReview: true,
		},
		{
			Level: 5, Code: "L5", Name: "联合运营商", Identity: "大渠道/战略合作", OpenMethod: "商务签约", Audience: "大渠道/战略合作方",
			Permissions: []string{"独立 SaaS 站点", "独立支付账户/或平台代收", "独立客户体系", "独立价格策略", "专属模型额度池", "专属企业服务政策", "定制功能"},
			Limitations: []string{"未来主控 SaaS + 代理商 SaaS 高级形态，必须商务签约"},
			OpenCondition: "战略合作", KeepCondition: "单独合同", ManualReview: true,
		},
	}
}

func agentLevelPolicyByLevel(level int) agentLevelPolicy {
	for _, item := range agentLevelPolicies() {
		if item.Level == level {
			return item
		}
	}
	return agentLevelPolicies()[0]
}

func agentRoleForLevel(level int) string {
	if level <= 0 {
		return "MEMBER"
	}
	if level > 5 {
		level = 5
	}
	return fmt.Sprintf("AGENT_L%d", level)
}

func isAgentLevel(level int) bool {
	return level >= 1 && level <= 5
}

func agentLevelLabel(level int) string {
	policy := agentLevelPolicyByLevel(level)
	return policy.Code + " " + policy.Name
}

func agentLevelPolicyRows() []map[string]any {
	items := []map[string]any{}
	for _, policy := range agentLevelPolicies() {
		items = append(items, map[string]any{
			"level":                policy.Level,
			"code":                 policy.Code,
			"name":                 policy.Name,
			"identity":             policy.Identity,
			"openMethod":           policy.OpenMethod,
			"audience":             policy.Audience,
			"permissions":          policy.Permissions,
			"limitations":          policy.Limitations,
			"membershipCommission": policy.MembershipCommission,
			"rechargeCommission":   policy.RechargeCommission,
			"enterpriseCommission": policy.EnterpriseCommission,
			"regionalRebate":       policy.RegionalRebate,
			"openCondition":        policy.OpenCondition,
			"keepCondition":        policy.KeepCondition,
			"manualReview":         policy.ManualReview,
			"status":               "ACTIVE",
		})
	}
	return items
}

func defaultMarketingUpgradePlans() []map[string]any {
	return []map[string]any{
		{"id": "upgrade_l0_to_l1", "fromRole": "MEMBER", "toRole": "AGENT_L1", "priceCents": 0, "conditionType": "APPLICATION", "cyclePolicy": "注册申请，保级无要求", "status": "ACTIVE", "metadata": map[string]any{"openCondition": "注册申请", "keepCondition": "无", "manualReview": false}},
		{"id": "upgrade_l1_to_l2", "fromRole": "AGENT_L1", "toRole": "AGENT_L2", "priceCents": 99900, "conditionType": "SALES_OR_ANNUAL_FEE", "cyclePolicy": "累计销售 1000 元或缴纳 999 年费；月销售 ≥ 1000 保级", "status": "ACTIVE", "metadata": map[string]any{"openCondition": "累计销售 1000 元或缴纳 999 年费", "keepCondition": "月销售 ≥ 1000", "manualReview": true}},
		{"id": "upgrade_l2_to_l3", "fromRole": "AGENT_L2", "toRole": "AGENT_L3", "priceCents": 299900, "conditionType": "SALES_OR_ANNUAL_FEE", "cyclePolicy": "累计销售 1 万或缴纳 2999 年费；月销售 ≥ 5000 保级", "status": "ACTIVE", "metadata": map[string]any{"openCondition": "累计销售 1 万或缴纳 2999 年费", "keepCondition": "月销售 ≥ 5000", "manualReview": true}},
		{"id": "upgrade_l3_to_l4", "fromRole": "AGENT_L3", "toRole": "AGENT_L4", "priceCents": 0, "conditionType": "CONTRACT_AND_PERFORMANCE", "cyclePolicy": "月销售 ≥ 5 万或合同签约；季度销售 ≥ 10 万保级", "status": "MANUAL_REVIEW", "metadata": map[string]any{"openCondition": "月销售 ≥ 5 万或合同签约", "keepCondition": "季度销售 ≥ 10 万", "manualReview": true}},
		{"id": "upgrade_l4_to_l5", "fromRole": "AGENT_L4", "toRole": "AGENT_L5", "priceCents": 0, "conditionType": "STRATEGIC_CONTRACT", "cyclePolicy": "战略合作开通；单独合同保级", "status": "MANUAL_REVIEW", "metadata": map[string]any{"openCondition": "战略合作", "keepCondition": "单独合同", "manualReview": true}},
	}
}

func defaultMarketingRoleRows() []map[string]any {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	items := []map[string]any{}
	for _, policy := range agentLevelPolicies() {
		items = append(items, map[string]any{
			"id":        "role_" + policy.Code,
			"code":      agentRoleForLevel(policy.Level),
			"name":      policy.Name,
			"level":     policy.Level,
			"status":    "ACTIVE",
			"metadata":  policyRowMetadata(policy),
			"createdAt": now,
			"updatedAt": now,
		})
	}
	return items
}

func policyRowMetadata(policy agentLevelPolicy) map[string]any {
	return map[string]any{
		"code":                 policy.Code,
		"identity":             policy.Identity,
		"openMethod":           policy.OpenMethod,
		"audience":             policy.Audience,
		"permissions":          policy.Permissions,
		"limitations":          policy.Limitations,
		"membershipCommission": policy.MembershipCommission,
		"rechargeCommission":   policy.RechargeCommission,
		"enterpriseCommission": policy.EnterpriseCommission,
		"regionalRebate":       policy.RegionalRebate,
		"openCondition":        policy.OpenCondition,
		"keepCondition":        policy.KeepCondition,
		"manualReview":         policy.ManualReview,
	}
}
