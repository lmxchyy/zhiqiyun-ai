insert into xz_marketing_commission_rules (id, name, order_type, earner_role, relation_depth, fixed_amount_cents, rate, max_total_rate, status, metadata, created_at, updated_at)
values
  ('rule_membership_l3_diff_from_l2', 'L3 上级代理会员套餐差额分润', 'PLAN_ORDER', 'AGENT_L3', 2, 0, 0.10, 0.35, 'ACTIVE', '{"level":3,"range":"L3-L2 差额 10%","policy":"团队差额分润"}'::jsonb, now(), now()),
  ('rule_recharge_l3_diff_from_l2', 'L3 上级代理点数充值差额分润', 'COMPUTE_RECHARGE', 'AGENT_L3', 2, 0, 0.08, 0.20, 'ACTIVE', '{"level":3,"range":"L3-L2 差额 8%","policy":"团队差额分润"}'::jsonb, now(), now()),
  ('rule_enterprise_l3_diff_from_l2', 'L3 上级代理企业项目差额分润', 'ENTERPRISE_PROJECT', 'AGENT_L3', 2, 0, 0.10, 0.25, 'ACTIVE', '{"level":3,"range":"L3-L2 差额 10%","policy":"团队差额分润"}'::jsonb, now(), now())
on conflict (id) do update set
  name = excluded.name,
  order_type = excluded.order_type,
  earner_role = excluded.earner_role,
  relation_depth = excluded.relation_depth,
  fixed_amount_cents = excluded.fixed_amount_cents,
  rate = excluded.rate,
  max_total_rate = excluded.max_total_rate,
  status = excluded.status,
  metadata = excluded.metadata,
  updated_at = now();
