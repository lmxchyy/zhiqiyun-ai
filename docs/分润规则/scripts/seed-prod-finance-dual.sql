-- CR-2026-OC-008 production change-window finance dual-control seed
-- Run ONLY in approved production change window.
-- Do NOT reuse user_000001 / SUPER_ADMIN as either party.
BEGIN;

-- Fixed IDs also recorded in env/prod-change-window.env.example
-- submitter: 98b945ba-878d-4c40-ae28-20ea3da1a6a5
-- approver:  e7af2217-3e0b-4ce8-b29c-7f74a1a4091f

INSERT INTO xz_users (id, email, name, role, status, password_hash, created_at, updated_at, raw)
VALUES
  ('98b945ba-878d-4c40-ae28-20ea3da1a6a5',
   'finance.submitter.oc008@internal.local',
   'OC Finance Submitter',
   'FINANCE', 'ACTIVE', NULL, now()::text, now()::text,
   '{"source":"CR-2026-OC-008","purpose":"manual_refund_submitter"}'::jsonb),
  ('e7af2217-3e0b-4ce8-b29c-7f74a1a4091f',
   'finance.approver.oc008@internal.local',
   'OC Finance Approver',
   'FINANCE', 'ACTIVE', NULL, now()::text, now()::text,
   '{"source":"CR-2026-OC-008","purpose":"manual_refund_approver"}'::jsonb)
ON CONFLICT (id) DO UPDATE
SET email = EXCLUDED.email,
    name = EXCLUDED.name,
    role = 'FINANCE',
    status = 'ACTIVE',
    password_hash = NULL,
    updated_at = now()::text;

WITH org AS (
  SELECT id AS organization_id
  FROM xz_organizations
  WHERE tenant_id = 'tenant_default'
  ORDER BY id
  LIMIT 1
)
INSERT INTO xz_user_roles (user_id, tenant_id, organization_id, role, status, assigned_at, updated_at)
SELECT u.user_id, 'tenant_default', org.organization_id, 'FINANCE', 'ACTIVE', now(), now()
FROM (
  VALUES
    ('98b945ba-878d-4c40-ae28-20ea3da1a6a5'),
    ('e7af2217-3e0b-4ce8-b29c-7f74a1a4091f')
) AS u(user_id)
CROSS JOIN org
ON CONFLICT (user_id, tenant_id, organization_id, role) DO UPDATE
SET status = 'ACTIVE', updated_at = now();

-- Privilege separation: finance accounts must not hold review/super-admin roles
DELETE FROM xz_user_roles
WHERE user_id IN (
  '98b945ba-878d-4c40-ae28-20ea3da1a6a5',
  'e7af2217-3e0b-4ce8-b29c-7f74a1a4091f'
)
AND role IN ('SUPER_ADMIN', 'OPERATION', 'ADMIN');

COMMIT;

SELECT u.id, u.email, u.status, r.role, r.status AS role_status
FROM xz_users u
JOIN xz_user_roles r ON r.user_id = u.id
WHERE u.id IN (
  '98b945ba-878d-4c40-ae28-20ea3da1a6a5',
  'e7af2217-3e0b-4ce8-b29c-7f74a1a4091f'
)
ORDER BY u.id, r.role;
