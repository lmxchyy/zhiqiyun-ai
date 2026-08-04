-- Targeted verification fixture for migration 104.
-- Run with psql -v ON_ERROR_STOP=1 after migrations 002..104 have been
-- applied to an isolated PostgreSQL database. Every mutation is savepoint
-- scoped so this fixture leaves the database unchanged.

\set ON_ERROR_STOP on

BEGIN;

DO $$
BEGIN
  IF to_regclass('public.xz_point_expiry_policy_versions') IS NULL THEN
    RAISE EXCEPTION 'migration 104 policy table is missing';
  END IF;
  IF NOT EXISTS (
    SELECT 1
    FROM pg_trigger
    WHERE tgrelid = 'xz_point_expiry_policy_versions'::regclass
      AND tgname = 'trg_xz_point_expiry_policy_versions_immutable'
      AND NOT tgisinternal
  ) THEN
    RAISE EXCEPTION 'migration 104 policy immutability trigger is missing';
  END IF;
END;
$$;

DO $$
DECLARE
  published_count bigint;
  policy_status text;
BEGIN
  SELECT count(*) INTO published_count
  FROM xz_point_expiry_policy_versions
  WHERE status = 'PUBLISHED';
  IF published_count <> 1 THEN
    RAISE EXCEPTION 'expected exactly one current PUBLISHED policy, got %', published_count;
  END IF;

  SELECT status INTO policy_status
  FROM xz_point_expiry_policy_versions
  WHERE id = 'point_expiry_policy_v1';
  IF policy_status IS DISTINCT FROM 'PUBLISHED' THEN
    RAISE EXCEPTION 'initial policy v1 is not PUBLISHED';
  END IF;
END;
$$;

-- The repaired path closes the current version once and creates the next
-- version in the same transaction. No old PUBLISHED row remains.
SAVEPOINT legal_publish;
UPDATE xz_point_expiry_policy_versions
SET status = 'ARCHIVED',
    effective_to = effective_from + interval '1 hour',
    updated_at = GREATEST(updated_at, now())
WHERE id = 'point_expiry_policy_v1';

INSERT INTO xz_point_expiry_policy_versions(
  id, version, revision, enabled, duration_value, duration_unit, time_zone,
  source_types, effective_from, status, created_by, change_reason, metadata
)
VALUES (
  'verify_policy_v2', 2, 2, TRUE, 3, 'CALENDAR_MONTH', 'Asia/Shanghai',
  '["REGISTRATION_GIFT","ACTIVITY_GIFT","ADMIN_GIFT"]'::jsonb,
  now() + interval '2 hours', 'PUBLISHED', 'verify:104',
  'legal same-transaction publish', '{"fixture":"104"}'::jsonb
);

DO $$
DECLARE
  published_count bigint;
  archived_to timestamptz;
BEGIN
  SELECT count(*) INTO published_count
  FROM xz_point_expiry_policy_versions
  WHERE status = 'PUBLISHED';
  IF published_count <> 1 THEN
    RAISE EXCEPTION 'legal publish left % PUBLISHED rows', published_count;
  END IF;
  SELECT effective_to INTO archived_to
  FROM xz_point_expiry_policy_versions
  WHERE id = 'point_expiry_policy_v1' AND status = 'ARCHIVED';
  IF archived_to IS NULL THEN
    RAISE EXCEPTION 'legal publish did not close v1';
  END IF;
END;
$$;
ROLLBACK TO SAVEPOINT legal_publish;

-- Each block below must fail. If the mutation is accepted, the deliberately
-- raised assertion error is re-thrown and the fixture fails.
DO $$
BEGIN
  BEGIN
    UPDATE xz_point_expiry_policy_versions
    SET duration_value = duration_value + 1
    WHERE id = 'point_expiry_policy_v1';
    RAISE EXCEPTION 'business-field tamper was accepted';
  EXCEPTION WHEN OTHERS THEN
    IF position('business-field tamper was accepted' IN SQLERRM) > 0 THEN
      RAISE;
    END IF;
  END;
END;
$$;

DO $$
BEGIN
  BEGIN
    DELETE FROM xz_point_expiry_policy_versions
    WHERE id = 'point_expiry_policy_v1';
    RAISE EXCEPTION 'direct DELETE of PUBLISHED policy was accepted';
  EXCEPTION WHEN OTHERS THEN
    IF position('direct DELETE of PUBLISHED policy was accepted' IN SQLERRM) > 0 THEN
      RAISE;
    END IF;
  END;
END;
$$;

DO $$
BEGIN
  BEGIN
    UPDATE xz_point_expiry_policy_versions
    SET updated_at = updated_at + interval '1 second'
    WHERE id = 'point_expiry_policy_v1';
    RAISE EXCEPTION 'updated_at-only PUBLISHED update was accepted';
  EXCEPTION WHEN OTHERS THEN
    IF position('updated_at-only PUBLISHED update was accepted' IN SQLERRM) > 0 THEN
      RAISE;
    END IF;
  END;
END;
$$;

DO $$
BEGIN
  BEGIN
    UPDATE xz_point_expiry_policy_versions
    SET status = 'ARCHIVED', effective_to = NULL, updated_at = now()
    WHERE id = 'point_expiry_policy_v1';
    RAISE EXCEPTION 'PUBLISHED closure without effective_to was accepted';
  EXCEPTION WHEN OTHERS THEN
    IF position('PUBLISHED closure without effective_to was accepted' IN SQLERRM) > 0 THEN
      RAISE;
    END IF;
  END;
END;
$$;

DO $$
BEGIN
  BEGIN
    UPDATE xz_point_expiry_policy_versions
    SET status = 'ARCHIVED', effective_to = effective_from, updated_at = now()
    WHERE id = 'point_expiry_policy_v1';
    RAISE EXCEPTION 'PUBLISHED closure with invalid effective_to was accepted';
  EXCEPTION WHEN OTHERS THEN
    IF position('PUBLISHED closure with invalid effective_to was accepted' IN SQLERRM) > 0 THEN
      RAISE;
    END IF;
  END;
END;
$$;

DO $$
BEGIN
  BEGIN
    UPDATE xz_point_expiry_policy_versions
    SET status = 'ARCHIVED',
        effective_to = effective_from + interval '1 hour',
        updated_at = updated_at - interval '1 second'
    WHERE id = 'point_expiry_policy_v1';
    RAISE EXCEPTION 'PUBLISHED closure with older updated_at was accepted';
  EXCEPTION WHEN OTHERS THEN
    IF position('PUBLISHED closure with older updated_at was accepted' IN SQLERRM) > 0 THEN
      RAISE;
    END IF;
  END;
END;
$$;

DO $$
BEGIN
  BEGIN
    UPDATE xz_point_expiry_policy_versions
    SET status = 'UNKNOWN'
    WHERE id = 'point_expiry_policy_v1';
    RAISE EXCEPTION 'unknown policy status was accepted';
  EXCEPTION WHEN OTHERS THEN
    IF position('unknown policy status was accepted' IN SQLERRM) > 0 THEN
      RAISE;
    END IF;
  END;
END;
$$;

-- A closed version is immutable and cannot be deleted. The nested savepoint
-- makes an archived row solely for this probe.
SAVEPOINT archived_policy_probes;
UPDATE xz_point_expiry_policy_versions
SET status = 'ARCHIVED',
    effective_to = effective_from + interval '1 hour',
    updated_at = GREATEST(updated_at, now())
WHERE id = 'point_expiry_policy_v1';

DO $$
BEGIN
  BEGIN
    UPDATE xz_point_expiry_policy_versions
    SET change_reason = change_reason || ' tamper'
    WHERE id = 'point_expiry_policy_v1';
    RAISE EXCEPTION 'second ARCHIVED update was accepted';
  EXCEPTION WHEN OTHERS THEN
    IF position('second ARCHIVED update was accepted' IN SQLERRM) > 0 THEN
      RAISE;
    END IF;
  END;
END;
$$;

DO $$
BEGIN
  BEGIN
    DELETE FROM xz_point_expiry_policy_versions
    WHERE id = 'point_expiry_policy_v1';
    RAISE EXCEPTION 'direct DELETE of ARCHIVED policy was accepted';
  EXCEPTION WHEN OTHERS THEN
    IF position('direct DELETE of ARCHIVED policy was accepted' IN SQLERRM) > 0 THEN
      RAISE;
    END IF;
  END;
END;
$$;
ROLLBACK TO SAVEPOINT archived_policy_probes;

-- A second PUBLISHED version cannot be inserted while the current one is
-- open; this also proves the guard needed by concurrent publishers.
DO $$
BEGIN
  BEGIN
    INSERT INTO xz_point_expiry_policy_versions(
      id, version, revision, enabled, duration_value, duration_unit, time_zone,
      source_types, effective_from, status, created_by, change_reason, metadata
    ) VALUES (
      'verify_policy_duplicate_published', 2, 2, TRUE, 3, 'CALENDAR_MONTH',
      'Asia/Shanghai', '["REGISTRATION_GIFT"]'::jsonb, now(), 'PUBLISHED',
      'verify:104', 'duplicate publish', '{}'::jsonb
    );
    RAISE EXCEPTION 'second PUBLISHED version was accepted';
  EXCEPTION WHEN OTHERS THEN
    IF position('second PUBLISHED version was accepted' IN SQLERRM) > 0 THEN
      RAISE;
    END IF;
  END;
END;
$$;

ROLLBACK;
