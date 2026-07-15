-- 主控 SaaS 企业管理第二阶段：补齐历史订单的企业归属。
-- 仅对“只有一个有效企业成员关系”的用户回填；多企业成员的历史订单保持未归属，
-- 避免根据 user_id 将同一订单暴露给多个企业。
BEGIN;

ALTER TABLE xz_orders ADD COLUMN IF NOT EXISTS tenant_id TEXT;

WITH unique_enterprise_membership AS (
  SELECT member.user_id, min(member.tenant_id) AS tenant_id
  FROM xz_tenant_members member
  JOIN xz_tenants tenant
    ON tenant.id = member.tenant_id
   AND tenant.tenant_type = 'ENTERPRISE'
  WHERE upper(coalesce(nullif(member.member_status, ''), member.status, 'ACTIVE')) = 'ACTIVE'
  GROUP BY member.user_id
  HAVING count(DISTINCT member.tenant_id) = 1
)
UPDATE xz_orders orders
SET tenant_id = membership.tenant_id,
    price_snapshot = jsonb_set(coalesce(orders.price_snapshot, '{}'::jsonb), '{tenantId}', to_jsonb(membership.tenant_id), true),
    raw = jsonb_set(coalesce(orders.raw, '{}'::jsonb), '{tenantId}', to_jsonb(membership.tenant_id), true)
FROM unique_enterprise_membership membership
WHERE coalesce(orders.tenant_id, '') = ''
  AND membership.user_id = orders.user_id;

CREATE INDEX IF NOT EXISTS idx_xz_orders_tenant
  ON xz_orders(tenant_id, created_at DESC);

COMMIT;
