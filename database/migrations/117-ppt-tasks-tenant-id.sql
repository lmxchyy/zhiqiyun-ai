BEGIN;

-- FIX-PPT-01: Add tenant_id column to xz_ppt_tasks for multi-tenant persistence
ALTER TABLE xz_ppt_tasks ADD COLUMN IF NOT EXISTS tenant_id VARCHAR(128) NOT NULL DEFAULT 'tenant_default';
ALTER TABLE xz_ppt_tasks ALTER COLUMN tenant_id SET DEFAULT 'tenant_default';

COMMIT;
