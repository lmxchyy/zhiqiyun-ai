-- Canonical L0-L5 agent level policy, upgrade rules and commission recommendations.

INSERT INTO xz_marketing_roles (id, code, name, level, status, metadata)
VALUES
  ('role_L0', 'MEMBER', '普通用户', 0, 'ACTIVE', '{"code":"L0","identity":"普通用户","openMethod":"注册即为普通用户","audience":"普通使用 AI 工具的客户","permissions":["购买会员","使用 AI 工具","充值 AI 点数"],"limitations":["不能推广返佣","没有代理后台"],"openCondition":"注册账号","keepCondition":"无","manualReview":false}'::jsonb),
  ('role_L1', 'AGENT_L1', '推广员', 1, 'ACTIVE', '{"code":"L1","identity":"轻量分销","openMethod":"申请即可/邀请开通","audience":"个人推广","permissions":["专属邀请链接","推广用户拿返佣","查看推广订单"],"limitations":["不可设置自己的价格","不可开下级代理"],"membershipCommission":"10%-20%","rechargeCommission":"5%-10%","enterpriseCommission":"按单谈","openCondition":"注册申请","keepCondition":"无","manualReview":false}'::jsonb),
  ('role_L2', 'AGENT_L2', '初级代理商', 2, 'ACTIVE', '{"code":"L2","identity":"正式代理","openMethod":"审核 + 代理套餐/业绩达标","audience":"小团队、个人渠道","permissions":["代理后台","客户管理","订单管理","佣金结算","销售会员套餐","销售点数包","销售企业版线索"],"limitations":["不能开二级代理，或者只能推荐给平台审核"],"membershipCommission":"20%-30%","rechargeCommission":"10%-15%","enterpriseCommission":"10%-20%","openCondition":"累计销售 1000 元或缴纳 999 年费","keepCondition":"月销售 ≥ 1000","manualReview":true}'::jsonb),
  ('role_L3', 'AGENT_L3', '高级代理商', 3, 'ACTIVE', '{"code":"L3","identity":"区域/行业代理","openMethod":"销售额达标或人工签约","audience":"企服老板、培训机构、代运营老板","permissions":["更高返佣","企业客户报备","专属客服支持","申请独立品牌页","申请行业解决方案报价","发展一级推广员"],"limitations":["只能发展一级推广员"],"membershipCommission":"30%-40%","rechargeCommission":"15%-25%","enterpriseCommission":"20%-30%","openCondition":"累计销售 1 万或缴纳 2999 年费","keepCondition":"月销售 ≥ 5000","manualReview":true}'::jsonb),
  ('role_L4', 'AGENT_L4', '城市合伙人', 4, 'ACTIVE', '{"code":"L4","identity":"城市/区域运营商","openMethod":"合同签约 + 业绩承诺","audience":"城市/区域渠道运营商","permissions":["城市区域保护","独立代理站点","自定义品牌名称/logo","管理区域代理","企业客户报价","区域业绩返点","培训和销售物料"],"limitations":["不轻易开放，必须人工审核"],"membershipCommission":"40%左右","rechargeCommission":"20%-30%","enterpriseCommission":"30%左右","regionalRebate":"5%-10%","openCondition":"月销售 ≥ 5 万或合同签约","keepCondition":"季度销售 ≥ 10 万","manualReview":true}'::jsonb),
  ('role_L5', 'AGENT_L5', '联合运营商', 5, 'ACTIVE', '{"code":"L5","identity":"大渠道/战略合作","openMethod":"商务签约","audience":"大渠道/战略合作方","permissions":["独立 SaaS 站点","独立支付账户/或平台代收","独立客户体系","独立价格策略","专属模型额度池","专属企业服务政策","定制功能"],"limitations":["未来主控 SaaS + 代理商 SaaS 高级形态，必须商务签约"],"openCondition":"战略合作","keepCondition":"单独合同","manualReview":true}'::jsonb)
ON CONFLICT (id) DO UPDATE
SET code = EXCLUDED.code,
    name = EXCLUDED.name,
    level = EXCLUDED.level,
    status = EXCLUDED.status,
    metadata = EXCLUDED.metadata,
    updated_at = now();

INSERT INTO xz_marketing_upgrade_plans (id, from_role, to_role, price_cents, condition_type, status, metadata)
VALUES
  ('upgrade_l0_to_l1', 'MEMBER', 'AGENT_L1', 0, 'APPLICATION', 'ACTIVE', '{"cyclePolicy":"注册申请，保级无要求","openCondition":"注册申请","keepCondition":"无","manualReview":false}'::jsonb),
  ('upgrade_l1_to_l2', 'AGENT_L1', 'AGENT_L2', 99900, 'SALES_OR_ANNUAL_FEE', 'ACTIVE', '{"cyclePolicy":"累计销售 1000 元或缴纳 999 年费；月销售 ≥ 1000 保级","openCondition":"累计销售 1000 元或缴纳 999 年费","keepCondition":"月销售 ≥ 1000","manualReview":true}'::jsonb),
  ('upgrade_l2_to_l3', 'AGENT_L2', 'AGENT_L3', 299900, 'SALES_OR_ANNUAL_FEE', 'ACTIVE', '{"cyclePolicy":"累计销售 1 万或缴纳 2999 年费；月销售 ≥ 5000 保级","openCondition":"累计销售 1 万或缴纳 2999 年费","keepCondition":"月销售 ≥ 5000","manualReview":true}'::jsonb),
  ('upgrade_l3_to_l4', 'AGENT_L3', 'AGENT_L4', 0, 'CONTRACT_AND_PERFORMANCE', 'MANUAL_REVIEW', '{"cyclePolicy":"月销售 ≥ 5 万或合同签约；季度销售 ≥ 10 万保级","openCondition":"月销售 ≥ 5 万或合同签约","keepCondition":"季度销售 ≥ 10 万","manualReview":true}'::jsonb),
  ('upgrade_l4_to_l5', 'AGENT_L4', 'AGENT_L5', 0, 'STRATEGIC_CONTRACT', 'MANUAL_REVIEW', '{"cyclePolicy":"战略合作开通；单独合同保级","openCondition":"战略合作","keepCondition":"单独合同","manualReview":true}'::jsonb)
ON CONFLICT (id) DO UPDATE
SET from_role = EXCLUDED.from_role,
    to_role = EXCLUDED.to_role,
    price_cents = EXCLUDED.price_cents,
    condition_type = EXCLUDED.condition_type,
    status = EXCLUDED.status,
    metadata = EXCLUDED.metadata,
    updated_at = now();

INSERT INTO xz_marketing_commission_rules (id, name, order_type, earner_role, relation_depth, fixed_amount_cents, rate, max_total_rate, status, metadata)
VALUES
  ('rule_membership_l1_direct', 'L1 推广员会员套餐返佣', 'PLAN_ORDER', 'AGENT_L1', 1, 0, 0.15, 0.20, 'ACTIVE', '{"level":1,"range":"10%-20%","policy":"会员套餐返佣"}'::jsonb),
  ('rule_recharge_l1_direct', 'L1 推广员点数包返佣', 'COMPUTE_RECHARGE', 'AGENT_L1', 1, 0, 0.08, 0.10, 'ACTIVE', '{"level":1,"range":"5%-10%","policy":"点数包返佣"}'::jsonb),
  ('rule_membership_l2_direct', 'L2 初级代理会员套餐返佣', 'PLAN_ORDER', 'AGENT_L2', 1, 0, 0.25, 0.30, 'ACTIVE', '{"level":2,"range":"20%-30%","policy":"会员套餐返佣"}'::jsonb),
  ('rule_recharge_l2_direct', 'L2 初级代理点数充值返佣', 'COMPUTE_RECHARGE', 'AGENT_L2', 1, 0, 0.12, 0.15, 'ACTIVE', '{"level":2,"range":"10%-15%","policy":"点数充值返佣"}'::jsonb),
  ('rule_enterprise_l2_direct', 'L2 初级代理企业项目返佣', 'ENTERPRISE_PROJECT', 'AGENT_L2', 1, 0, 0.15, 0.20, 'ACTIVE', '{"level":2,"range":"10%-20%","policy":"企业项目返佣"}'::jsonb),
  ('rule_membership_l3_direct', 'L3 高级代理会员套餐返佣', 'PLAN_ORDER', 'AGENT_L3', 1, 0, 0.35, 0.40, 'ACTIVE', '{"level":3,"range":"30%-40%","policy":"会员套餐返佣"}'::jsonb),
  ('rule_recharge_l3_direct', 'L3 高级代理点数充值返佣', 'COMPUTE_RECHARGE', 'AGENT_L3', 1, 0, 0.20, 0.25, 'ACTIVE', '{"level":3,"range":"15%-25%","policy":"点数充值返佣"}'::jsonb),
  ('rule_enterprise_l3_direct', 'L3 高级代理企业版项目返佣', 'ENTERPRISE_PROJECT', 'AGENT_L3', 1, 0, 0.25, 0.30, 'ACTIVE', '{"level":3,"range":"20%-30%","policy":"企业版项目返佣"}'::jsonb),
  ('rule_membership_l4_direct', 'L4 城市合伙人会员套餐返佣', 'PLAN_ORDER', 'AGENT_L4', 1, 0, 0.40, 0.40, 'ACTIVE', '{"level":4,"range":"40%左右","policy":"会员套餐返佣","manualReview":true}'::jsonb),
  ('rule_recharge_l4_direct', 'L4 城市合伙人点数充值返佣', 'COMPUTE_RECHARGE', 'AGENT_L4', 1, 0, 0.25, 0.30, 'ACTIVE', '{"level":4,"range":"20%-30%","policy":"点数充值返佣","manualReview":true}'::jsonb),
  ('rule_enterprise_l4_direct', 'L4 城市合伙人企业项目返佣', 'ENTERPRISE_PROJECT', 'AGENT_L4', 1, 0, 0.30, 0.30, 'ACTIVE', '{"level":4,"range":"30%左右","policy":"企业项目返佣","manualReview":true}'::jsonb),
  ('rule_region_l4_team', 'L4 城市合伙人区域团队业绩返点', 'TEAM_PERFORMANCE', 'AGENT_L4', 2, 0, 0.08, 0.10, 'ACTIVE', '{"level":4,"range":"5%-10%","policy":"区域团队业绩返点","manualReview":true}'::jsonb),
  ('rule_l5_strategic_contract', 'L5 联合运营商战略合作政策', 'STRATEGIC_CONTRACT', 'AGENT_L5', 1, 0, 0, 0, 'MANUAL_REVIEW', '{"level":5,"policy":"独立 SaaS、独立价格和专属额度池，按单独合同执行","manualReview":true}'::jsonb)
ON CONFLICT (id) DO UPDATE
SET name = EXCLUDED.name,
    order_type = EXCLUDED.order_type,
    earner_role = EXCLUDED.earner_role,
    relation_depth = EXCLUDED.relation_depth,
    fixed_amount_cents = EXCLUDED.fixed_amount_cents,
    rate = EXCLUDED.rate,
    max_total_rate = EXCLUDED.max_total_rate,
    status = EXCLUDED.status,
    metadata = EXCLUDED.metadata,
    updated_at = now();
