-- Force-equality verification for PRODUCTION V2 MEMBER/AGENT objects.
-- Compares pricePlan.sale_price_cents = binding.provider_price_snapshot_cents = good.platform_price_cents
-- and channel/environment/offerId/mode alignment. Does not invent WeChat console live price.

\pset pager off

SELECT 'counts' AS section,
       (SELECT count(*) FROM xz_price_plans) AS price_plans,
       (SELECT count(*) FROM xz_wechat_virtual_goods) AS goods,
       (SELECT count(*) FROM xz_price_plan_payment_bindings) AS bindings,
       (SELECT count(*) FROM xz_plan_versions WHERE status='ACTIVE') AS active_versions;

SELECT pp.code,
       pp.plan_id,
       pp.price_type AS kind,
       pp.environment,
       pp.sale_price_cents,
       pp.bonus_points AS gift_points,
       pp.enabled,
       pp.is_default,
       pp.status AS price_status,
       b.id AS binding_id,
       b.enabled AS binding_enabled,
       b.status AS binding_status,
       b.provider_price_snapshot_cents,
       g.id AS good_id,
       g.product_id,
       g.offer_id,
       g.mode,
       g.platform_price_cents,
       g.published,
       g.enabled AS good_enabled,
       g.status AS good_status,
       g.verification_status,
       (pp.sale_price_cents = b.provider_price_snapshot_cents
        AND pp.sale_price_cents = g.platform_price_cents
        AND b.provider_price_snapshot_cents = g.platform_price_cents) AS price_equal,
       (pp.channel = b.channel AND b.channel = g.channel AND pp.channel = 'WECHAT_VIRTUAL') AS channel_ok,
       (pp.environment = b.environment AND b.environment = g.environment AND pp.environment = 'PRODUCTION') AS env_ok,
       (g.offer_id = '1450579876' AND g.mode = 'short_series_goods') AS offer_mode_ok
FROM xz_price_plans pp
JOIN xz_price_plan_payment_bindings b ON b.price_plan_id = pp.id
JOIN xz_wechat_virtual_goods g ON g.id = b.wechat_good_id
WHERE pp.environment = 'PRODUCTION'
ORDER BY pp.price_type, pp.plan_id, pp.code;

SELECT 'force_equality_blockers' AS check_name, count(*) AS blocker_count
FROM xz_price_plans pp
JOIN xz_price_plan_payment_bindings b ON b.price_plan_id = pp.id
JOIN xz_wechat_virtual_goods g ON g.id = b.wechat_good_id
WHERE pp.environment = 'PRODUCTION'
  AND (
    pp.sale_price_cents IS DISTINCT FROM b.provider_price_snapshot_cents
    OR pp.sale_price_cents IS DISTINCT FROM g.platform_price_cents
    OR b.provider_price_snapshot_cents IS DISTINCT FROM g.platform_price_cents
    OR pp.channel IS DISTINCT FROM b.channel
    OR b.channel IS DISTINCT FROM g.channel
    OR pp.environment IS DISTINCT FROM b.environment
    OR b.environment IS DISTINCT FROM g.environment
    OR g.offer_id IS DISTINCT FROM '1450579876'
    OR g.mode IS DISTINCT FROM 'short_series_goods'
    OR pp.bonus_points <> 0
  );

SELECT 'missing_required_matrix_rows' AS check_name, count(*) AS missing_count
FROM (
  VALUES
    ('plan_ai_creator_996','NORMAL','MEMBER_YEAR_996',99600),
    ('plan_agent_join_996','NORMAL','AGENT_JOIN_996',99600),
    ('plan_ai_creator_996','TEST','MEMBER_TEST_1YUAN',100),
    ('plan_agent_join_996','TEST','AGENT_TEST_1YUAN',100)
) AS need(plan_id, kind, product_id, cents)
LEFT JOIN xz_price_plans pp
  ON pp.plan_id = need.plan_id
 AND pp.price_type = need.kind
 AND pp.environment = 'PRODUCTION'
 AND pp.sale_price_cents = need.cents
LEFT JOIN xz_price_plan_payment_bindings b
  ON b.price_plan_id = pp.id AND b.enabled AND b.status = 'ACTIVE'
LEFT JOIN xz_wechat_virtual_goods g
  ON g.id = b.wechat_good_id
 AND g.product_id = need.product_id
 AND g.platform_price_cents = need.cents
 AND g.published
WHERE pp.id IS NULL OR b.id IS NULL OR g.id IS NULL;
