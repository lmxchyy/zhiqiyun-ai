-- Bootstrap first ACTIVE entitlement versions for legacy MEMBER/AGENT plans.
-- Required because admin createVersion refuses plans with zero MEMBER/AGENT versions
-- (managedBusinessTypeForUpdate → BUSINESS_PLAN_NOT_FOUND). Idempotent.
-- giftPoints remain on V1 plans; V2 price-plan giftPoints will be 0 via admin API.

BEGIN;

INSERT INTO xz_plan_versions (
  id, plan_id, version_no, business_type, rights_snapshot,
  member_level, agent_level, token_amount, points_amount, duration_days,
  commission_rule_version, commission_snapshot, status,
  effective_at, created_by, updated_by, activated_by, activated_at, change_reason
)
SELECT
  'plan_version_member_996_v1',
  'plan_ai_creator_996',
  1,
  'MEMBER',
  '{"memberLevel":"PRO","tokenAmount":40000,"pointsAmount":0,"durationDays":365}'::jsonb,
  'PRO',
  NULL,
  40000,
  0,
  365,
  'COMMISSION_996_STANDARD',
  '{"rules":[]}'::jsonb,
  'ACTIVE',
  now(),
  'user_000001',
  'user_000001',
  'user_000001',
  now(),
  'price-plan-v2-production-gate bootstrap entitlement 20260729'
WHERE NOT EXISTS (
  SELECT 1 FROM xz_plan_versions WHERE plan_id = 'plan_ai_creator_996'
);

INSERT INTO xz_plan_versions (
  id, plan_id, version_no, business_type, rights_snapshot,
  member_level, agent_level, token_amount, points_amount, duration_days,
  commission_rule_version, commission_snapshot, status,
  effective_at, created_by, updated_by, activated_by, activated_at, change_reason
)
SELECT
  'plan_version_agent_996_v1',
  'plan_agent_join_996',
  1,
  'AGENT',
  '{"agentLevel":"AGENT","tokenAmount":20000,"pointsAmount":0,"durationDays":0}'::jsonb,
  NULL,
  'AGENT',
  20000,
  0,
  0,
  'COMMISSION_996_STANDARD',
  '{"rules":[]}'::jsonb,
  'ACTIVE',
  now(),
  'user_000001',
  'user_000001',
  'user_000001',
  now(),
  'price-plan-v2-production-gate bootstrap entitlement 20260729'
WHERE NOT EXISTS (
  SELECT 1 FROM xz_plan_versions WHERE plan_id = 'plan_agent_join_996'
);

SELECT id, plan_id, business_type, status, version_no, member_level, agent_level,
       token_amount, points_amount, duration_days
FROM xz_plan_versions
WHERE plan_id IN ('plan_ai_creator_996', 'plan_agent_join_996')
ORDER BY plan_id, version_no;

COMMIT;
