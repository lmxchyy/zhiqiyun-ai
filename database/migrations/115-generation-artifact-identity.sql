BEGIN;

-- Resolve duplicates before adding the durable claim.  Keep the oldest ACTIVE
-- row (otherwise the oldest pending row), and retire all other live claims.
-- Retiring rather than deleting preserves references from existing artifacts.
WITH ranked AS (
  SELECT file_id,
         row_number() OVER (
           PARTITION BY tenant_id, business_type, business_id, original_name
           ORDER BY (status = 'ACTIVE') DESC, created_at ASC, file_id ASC
         ) AS rank
  FROM xz_file_objects
  WHERE business_id IS NOT NULL AND status IN ('PENDING_UPLOAD','ACTIVE')
)
UPDATE xz_file_objects f
SET status = 'UPLOAD_FAILED',
    reserved_size = 0,
    deleted_at = COALESCE(deleted_at, now()),
    updated_at = now(),
    metadata = COALESCE(metadata, '{}'::jsonb) || jsonb_build_object('migration_note', 'duplicate generation artifact claim retired')
FROM ranked r
WHERE f.file_id = r.file_id AND r.rank > 1;

-- Durable claim identity for generated artifacts. Uploads use this identity
-- to reuse the same row/object after a crash; the remote upload is never part
-- of the transaction that creates this claim.
CREATE UNIQUE INDEX IF NOT EXISTS ux_file_objects_generation_artifact_identity
  ON xz_file_objects (tenant_id, business_type, business_id, original_name)
  WHERE business_id IS NOT NULL AND status IN ('PENDING_UPLOAD','ACTIVE');

COMMIT;
