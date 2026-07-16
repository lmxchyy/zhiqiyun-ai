-- Commission engine rule coverage and finance permissions.
-- Values are configuration rows consumed by CommissionEngine; business code contains no package amount constants.

BEGIN;

INSERT INTO permissions (code, name, module, action)
VALUES
  ('finance:commission-rule:view', '查看分润规则', 'finance', 'commission_rule_view'),
  ('finance:commission-rule:manage', '管理分润规则', 'finance', 'commission_rule_manage')
ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, module = EXCLUDED.module, action = EXCLUDED.action;

WITH matrix(role_code, permission_code) AS (
  VALUES
    ('FINANCE', 'finance:commission-rule:view'),
    ('FINANCE', 'finance:commission-rule:manage')
)
INSERT INTO role_permissions (role_id, permission_id)
SELECT role_row.id, permission_row.id
FROM matrix
JOIN roles role_row ON role_row.code = matrix.role_code
JOIN permissions permission_row ON permission_row.code = matrix.permission_code
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO xz_role_permissions (role, permission)
VALUES
  ('FINANCE', 'finance:commission-rule:view'),
  ('FINANCE', 'finance:commission-rule:manage')
ON CONFLICT (role, permission) DO NOTHING;

INSERT INTO xz_commission_rules (
  id, tenant_id, rule_code, rule_name, product_type, product_id,
  beneficiary_role, relationship_level, calculation_type, fixed_amount_cents,
  percentage_bps, priority, freeze_days, refund_policy, effective_start_at, version, status
)
VALUES
  ('commission_rule_agent_996_agent_v1', 'tenant_default', 'AGENT_996_DIRECT_AGENT', '996代理开通直属代理现金分润',
   'AGENT_JOIN_PACKAGE', 'plan_agent_join_996', 'AGENT', 1, 'FIXED_AMOUNT', 30000, 0, 10, 7,
   'REVERSE_OR_RECOVER', '2020-01-01T00:00:00Z', 1, 'ACTIVE'),
  ('commission_rule_agent_996_operation_v1', 'tenant_default', 'AGENT_996_OPERATION_CENTER', '996代理开通运营中心服务分润',
   'AGENT_JOIN_PACKAGE', 'plan_agent_join_996', 'OPERATION_CENTER', 1, 'FIXED_AMOUNT', 20000, 0, 20, 7,
   'REVERSE_OR_RECOVER', '2020-01-01T00:00:00Z', 1, 'ACTIVE'),
  ('commission_rule_agent_996_platform_v1', 'tenant_default', 'AGENT_996_PLATFORM_REMAINDER', '996代理开通平台现金留存',
   'AGENT_JOIN_PACKAGE', 'plan_agent_join_996', 'PLATFORM', 0, 'REMAINDER_TO_PLATFORM', 0, 0, 1000, 0,
   'REVERSE_OR_RECOVER', '2020-01-01T00:00:00Z', 1, 'ACTIVE'),
  ('commission_rule_operation_5000_platform_v1', 'tenant_default', 'OPERATION_5000_PLATFORM_REMAINDER', '5000运营中心开通平台现金留存',
   'OPERATION_CENTER_PACKAGE', 'plan_operation_center_5000', 'PLATFORM', 0, 'REMAINDER_TO_PLATFORM', 0, 0, 1000, 0,
   'REVERSE_OR_RECOVER', '2020-01-01T00:00:00Z', 1, 'ACTIVE')
ON CONFLICT (tenant_id, rule_code, version) DO NOTHING;

COMMIT;
