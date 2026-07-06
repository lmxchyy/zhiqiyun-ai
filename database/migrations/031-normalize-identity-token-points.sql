update xz_plans
set grant_points = 40000,
    entitlements = coalesce(entitlements, '{}'::jsonb)
      || jsonb_build_object('tokenGrantAmount', 40000, 'tokenRightsValueCents', 40000),
    raw = coalesce(raw, '{}'::jsonb)
      || jsonb_build_object('grantPoints', 40000, 'tokenAmount', 40000)
where id = 'plan_ai_creator_996';

update xz_orders
set token_grant_value_cents = 40000,
    price_snapshot = coalesce(price_snapshot, '{}'::jsonb)
      || jsonb_build_object('tokenGrantAmount', 40000, 'tokenAmount', 40000, 'tokenGrantValueCents', 40000),
    raw = coalesce(raw, '{}'::jsonb)
      || jsonb_build_object('tokenGrantAmount', 40000, 'tokenAmount', 40000, 'tokenGrantValueCents', 40000)
where plan_id = 'plan_ai_creator_996'
  and coalesce(fulfillment_status, '') <> 'FULFILLED';

update xz_plans
set grant_points = 20000,
    entitlements = coalesce(entitlements, '{}'::jsonb)
      || jsonb_build_object('tokenGrantAmount', 20000, 'tokenRightsValueCents', 20000),
    raw = coalesce(raw, '{}'::jsonb)
      || jsonb_build_object('grantPoints', 20000, 'tokenAmount', 20000)
where id = 'plan_agent_join_996';

update xz_orders
set token_grant_value_cents = 20000,
    price_snapshot = coalesce(price_snapshot, '{}'::jsonb)
      || jsonb_build_object('tokenGrantAmount', 20000, 'tokenAmount', 20000, 'tokenGrantValueCents', 20000),
    raw = coalesce(raw, '{}'::jsonb)
      || jsonb_build_object('tokenGrantAmount', 20000, 'tokenAmount', 20000, 'tokenGrantValueCents', 20000)
where plan_id = 'plan_agent_join_996'
  and coalesce(fulfillment_status, '') <> 'FULFILLED';
