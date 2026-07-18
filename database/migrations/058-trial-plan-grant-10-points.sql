-- New users on the trial plan receive 10 points. Existing user balances are
-- intentionally left unchanged; this only updates the plan used by new grants.

UPDATE xz_plans
SET grant_points = 10,
    raw = jsonb_set(
      jsonb_set(coalesce(raw, '{}'::jsonb), '{points}', '10'::jsonb, true),
      '{grantPoints}',
      '10'::jsonb,
      true
    )
WHERE id = 'plan_free'
   OR lower(coalesce(code, '')) = 'trial';
