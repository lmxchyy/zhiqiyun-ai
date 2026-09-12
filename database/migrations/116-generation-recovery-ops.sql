BEGIN;

-- PR4A: narrowly scoped generation recovery permissions. The API still
-- validates the durable state machine; these grants only establish who may
-- inspect or invoke the finite recovery actions.
INSERT INTO xz_role_permissions(role, permission)
VALUES
  ('SUPER_ADMIN', 'generation:recovery:view'),
  ('SUPER_ADMIN', 'generation:recovery:manage'),
  ('PLATFORM_ADMIN', 'generation:recovery:view'),
  ('PLATFORM_ADMIN', 'generation:recovery:manage'),
  ('ADMIN', 'generation:recovery:view'),
  ('ADMIN', 'generation:recovery:manage')
ON CONFLICT (role, permission) DO NOTHING;

COMMIT;
