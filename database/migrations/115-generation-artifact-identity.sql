BEGIN;

-- Durable claim identity for generated artifacts.  Uploads use this identity
-- to reuse the same row/object after a crash; the remote upload is never part
-- of the transaction that creates this claim.
CREATE UNIQUE INDEX IF NOT EXISTS ux_file_objects_generation_artifact_identity
  ON xz_file_objects (tenant_id, business_type, business_id, original_name)
  WHERE business_id IS NOT NULL AND status IN ('PENDING_UPLOAD','ACTIVE');

COMMIT;
