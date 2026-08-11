-- Member/agent price-plan V2 production preflight.
-- psql only. SELECT-only. No production data or schema changes are performed.
-- Run with a database role whose transaction_read_only result is "on".
-- Save the complete stdout/stderr as release evidence.

\set ON_ERROR_STOP on
\pset pager off
\timing on

BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
SET LOCAL statement_timeout = '120s';
SET LOCAL lock_timeout = '3s';

\echo '=== A. Database identity: DBA must confirm every value ==='
SELECT
  current_database() AS database_name,
  current_user AS database_user,
  current_schema() AS schema_name,
  inet_server_addr() AS server_address,
  inet_server_port() AS server_port,
  pg_is_in_recovery() AS is_replica,
  current_setting('transaction_read_only') AS transaction_read_only,
  current_setting('server_version') AS postgres_version;

\echo '=== B. Long transactions and lock waits: returned rows require review ==='
SELECT
  pid,
  usename,
  application_name,
  client_addr,
  state,
  wait_event_type,
  wait_event,
  clock_timestamp() - xact_start AS transaction_age
FROM pg_stat_activity
WHERE datname = current_database()
  AND pid <> pg_backend_pid()
  AND (
    (xact_start IS NOT NULL
      AND xact_start < clock_timestamp() - interval '5 minutes')
    OR wait_event_type = 'Lock'
  )
ORDER BY xact_start NULLS LAST;

SELECT count(*) AS waiting_lock_count
FROM pg_locks
WHERE NOT granted;

\echo '=== C. Required pre-097 schema: every returned row is a blocker ==='
WITH required(table_name, column_name) AS (
  VALUES
    ('xz_plans','id'),
    ('xz_plans','code'),
    ('xz_plans','price_cents'),
    ('xz_users','id'),
    ('xz_users','role'),
    ('xz_orders','id'),
    ('xz_orders','order_no'),
    ('xz_orders','tenant_id'),
    ('xz_orders','user_id'),
    ('xz_orders','buyer_user_id'),
    ('xz_orders','plan_id'),
    ('xz_orders','amount_cents'),
    ('xz_orders','status'),
    ('xz_orders','fulfillment_status'),
    ('xz_orders','entitlement_status'),
    ('xz_orders','created_at'),
    ('xz_audit_logs','id'),
    ('xz_audit_logs','actor_id'),
    ('xz_audit_logs','actor_role'),
    ('xz_audit_logs','action'),
    ('xz_audit_logs','resource'),
    ('xz_audit_logs','resource_id'),
    ('xz_audit_logs','created_at'),
    ('xz_role_permissions','role'),
    ('xz_role_permissions','permission')
)
SELECT required.*
FROM required
LEFT JOIN information_schema.columns actual
  ON actual.table_schema = current_schema()
 AND actual.table_name = required.table_name
 AND actual.column_name = required.column_name
WHERE actual.column_name IS NULL
ORDER BY required.table_name, required.column_name;

\echo '=== D. 097 table state: present_count must be 0 or 6, never 1-5 ==='
WITH expected(relname) AS (
  VALUES
    ('xz_plan_versions'),
    ('xz_price_plans'),
    ('xz_wechat_virtual_goods'),
    ('xz_price_plan_payment_bindings'),
    ('xz_price_plan_user_whitelist'),
    ('xz_order_price_quotes')
)
SELECT
  count(*) FILTER (
    WHERE to_regclass(current_schema() || '.' || relname) IS NOT NULL
  ) AS present_count,
  count(*) FILTER (
    WHERE to_regclass(current_schema() || '.' || relname) IS NULL
  ) AS missing_count
FROM expected;

\echo '=== D2. 097 order-column state: present_count must be 0 or 11, never 1-10 ==='
WITH expected(column_name) AS (
  VALUES
    ('plan_version_id'),
    ('price_plan_id'),
    ('price_quote_id'),
    ('snapshot_version'),
    ('transaction_price_cents'),
    ('wechat_product_id_snapshot'),
    ('wechat_goods_price_cents'),
    ('payment_environment'),
    ('rights_snapshot'),
    ('commission_rule_version_snapshot'),
    ('commission_snapshot_v2')
)
SELECT
  count(*) FILTER (WHERE actual.column_name IS NOT NULL) AS present_count,
  count(*) FILTER (WHERE actual.column_name IS NULL) AS missing_count
FROM expected
LEFT JOIN information_schema.columns actual
  ON actual.table_schema = current_schema()
 AND actual.table_name = 'xz_orders'
 AND actual.column_name = expected.column_name;

\echo '=== D3. Migration marker objects: first application expects zero rows ==='
SELECT namespace.nspname AS schema_name,
       relation.relname AS object_name,
       relation.relkind AS object_kind
FROM pg_class relation
JOIN pg_namespace namespace ON namespace.oid = relation.relnamespace
WHERE namespace.nspname = current_schema()
  AND relation.relname ~ '_(097|098|099|100)$'
ORDER BY relation.relkind, relation.relname;

\echo '=== D4. Critical 097 constraint-name collisions: first application expects zero rows ==='
SELECT namespace.nspname AS schema_name,
       relation.relname AS table_name,
       constraint_item.conname
FROM pg_constraint constraint_item
JOIN pg_class relation ON relation.oid = constraint_item.conrelid
JOIN pg_namespace namespace ON namespace.oid = relation.relnamespace
WHERE constraint_item.conname IN (
  'fk_xz_orders_plan_version_097',
  'fk_xz_orders_price_plan_097',
  'fk_xz_orders_price_quote_097',
  'ck_xz_orders_snapshot_v2_097'
)
ORDER BY constraint_item.conname, namespace.nspname, relation.relname;

\echo '=== E. Plan codes: duplicate_code must return zero rows ==='
SELECT btrim(code) AS duplicate_code, count(*) AS duplicate_count
FROM xz_plans
WHERE nullif(btrim(code), '') IS NOT NULL
GROUP BY btrim(code)
HAVING count(*) > 1
ORDER BY duplicate_count DESC, duplicate_code;

\echo '=== F. Historical code warnings: these rows must not be newly onboarded to V2 ==='
SELECT id, code
FROM xz_plans
WHERE nullif(btrim(code), '') IS NULL
   OR code !~ '^[a-z][a-z0-9_]*[a-z0-9]$'
ORDER BY id;

\echo '=== G. Production sizing estimates; exact count/sum belongs in the isolated copy ==='
SELECT
  table_stat.relname AS table_name,
  table_stat.n_live_tup AS estimated_live_rows,
  pg_total_relation_size(table_stat.relid) AS total_bytes,
  pg_size_pretty(pg_total_relation_size(table_stat.relid)) AS total_size
FROM pg_stat_user_tables table_stat
WHERE table_stat.relname IN (
  'xz_plans',
  'xz_orders',
  'xz_audit_logs',
  'xz_role_permissions'
)
ORDER BY pg_total_relation_size(table_stat.relid) DESC;

SELECT CASE WHEN
  to_regclass(current_schema() || '.xz_plan_versions') IS NOT NULL
  AND to_regclass(current_schema() || '.xz_price_plans') IS NOT NULL
  AND to_regclass(current_schema() || '.xz_wechat_virtual_goods') IS NOT NULL
  AND to_regclass(current_schema() || '.xz_price_plan_payment_bindings') IS NOT NULL
  AND to_regclass(current_schema() || '.xz_price_plan_user_whitelist') IS NOT NULL
  AND to_regclass(current_schema() || '.xz_order_price_quotes') IS NOT NULL
THEN 'true' ELSE 'false' END AS has_097
\gset

\if :has_097
\echo '=== H. 097 V2 core integrity: every blocker_count must be zero ==='
SELECT 'multiple_active_plan_versions' AS check_name, count(*) AS blocker_count
FROM (
  SELECT plan_id
  FROM xz_plan_versions
  WHERE status = 'ACTIVE'
  GROUP BY plan_id
  HAVING count(*) > 1
) violations

UNION ALL

SELECT 'multiple_defaults_without_currency', count(*)
FROM (
  SELECT plan_id, channel, environment
  FROM xz_price_plans
  WHERE is_default
  GROUP BY plan_id, channel, environment
  HAVING count(*) > 1
) violations

UNION ALL

SELECT 'price_plan_version_plan_mismatch', count(*)
FROM xz_price_plans price
JOIN xz_plan_versions version ON version.id = price.plan_version_id
WHERE price.plan_id IS DISTINCT FROM version.plan_id

UNION ALL

SELECT 'enabled_binding_price_or_scope_mismatch', count(*)
FROM xz_price_plan_payment_bindings binding
JOIN xz_price_plans price ON price.id = binding.price_plan_id
JOIN xz_wechat_virtual_goods good ON good.id = binding.wechat_good_id
WHERE (binding.enabled OR binding.status = 'ACTIVE')
  AND (
    binding.channel IS DISTINCT FROM price.channel
    OR binding.environment IS DISTINCT FROM price.environment
    OR binding.channel IS DISTINCT FROM good.channel
    OR binding.environment IS DISTINCT FROM good.environment
    OR binding.provider_price_snapshot_cents IS DISTINCT FROM price.sale_price_cents
    OR binding.provider_price_snapshot_cents IS DISTINCT FROM good.platform_price_cents
  )

UNION ALL

SELECT 'available_quote_configuration_drift', count(*)
FROM xz_order_price_quotes quote
LEFT JOIN xz_price_plans price ON price.id = quote.price_plan_id
LEFT JOIN xz_price_plan_payment_bindings binding
  ON binding.id = quote.payment_binding_id
LEFT JOIN xz_wechat_virtual_goods good ON good.id = quote.wechat_good_id
WHERE quote.status = 'AVAILABLE'
  AND quote.expires_at > statement_timestamp()
  AND (
    price.id IS NULL
    OR binding.id IS NULL
    OR good.id IS NULL
    OR binding.price_plan_id IS DISTINCT FROM quote.price_plan_id
    OR binding.wechat_good_id IS DISTINCT FROM quote.wechat_good_id
    OR quote.transaction_price_cents IS DISTINCT FROM price.sale_price_cents
    OR quote.provider_price_snapshot_cents
       IS DISTINCT FROM binding.provider_price_snapshot_cents
    OR quote.wechat_goods_price_cents IS DISTINCT FROM good.platform_price_cents
    OR quote.channel IS DISTINCT FROM price.channel
    OR quote.channel IS DISTINCT FROM binding.channel
    OR quote.channel IS DISTINCT FROM good.channel
    OR quote.environment IS DISTINCT FROM price.environment
    OR quote.environment IS DISTINCT FROM binding.environment
    OR quote.environment IS DISTINCT FROM good.environment
    OR quote.offer_id IS DISTINCT FROM good.offer_id
    OR quote.wechat_product_id IS DISTINCT FROM good.product_id
    OR quote.payment_mode IS DISTINCT FROM good.mode
  )

UNION ALL

SELECT 'order_v2_snapshot_invalid', count(*)
FROM xz_orders
WHERE snapshot_version = 2
  AND (
    plan_id IS NULL
    OR plan_version_id IS NULL
    OR price_plan_id IS NULL
    OR price_quote_id IS NULL
    OR buyer_user_id IS DISTINCT FROM user_id
    OR transaction_price_cents IS NULL
    OR transaction_price_cents <= 0
    OR amount_cents IS DISTINCT FROM transaction_price_cents
    OR nullif(btrim(wechat_product_id_snapshot), '') IS NULL
    OR wechat_goods_price_cents IS DISTINCT FROM transaction_price_cents
    OR payment_environment NOT IN ('PRODUCTION','SANDBOX')
    OR rights_snapshot IS NULL
    OR commission_snapshot_v2 IS NULL
  );

\echo '=== I. 097 NOT VALID FK orphans: every blocker_count must be zero ==='
SELECT 'order_plan_version_orphan' AS check_name, count(*) AS blocker_count
FROM xz_orders orders
LEFT JOIN xz_plan_versions version ON version.id = orders.plan_version_id
WHERE orders.plan_version_id IS NOT NULL AND version.id IS NULL

UNION ALL

SELECT 'order_price_plan_orphan', count(*)
FROM xz_orders orders
LEFT JOIN xz_price_plans price ON price.id = orders.price_plan_id
WHERE orders.price_plan_id IS NOT NULL AND price.id IS NULL

UNION ALL

SELECT 'order_price_quote_orphan', count(*)
FROM xz_orders orders
LEFT JOIN xz_order_price_quotes quote ON quote.id = orders.price_quote_id
WHERE orders.price_quote_id IS NOT NULL AND quote.id IS NULL;
\endif

SELECT CASE WHEN EXISTS (
  SELECT 1
  FROM information_schema.columns
  WHERE table_schema = current_schema()
    AND table_name = 'xz_wechat_virtual_goods'
    AND column_name = 'verification_status'
) THEN 'true' ELSE 'false' END AS has_098
\gset

\if :has_098
\echo '=== J. 098 WeChat manual verification: every blocker_count must be zero ==='
SELECT 'wechat_verification_status_invalid' AS check_name, count(*) AS blocker_count
FROM xz_wechat_virtual_goods
WHERE (
  verification_status IN (
    'UNCONFIRMED',
    'MANUALLY_CONFIRMED_PUBLISHED',
    'PRICE_MISMATCH',
    'VERIFICATION_EXPIRED',
    'DISABLED'
  )
) IS NOT TRUE

UNION ALL

SELECT 'wechat_manual_confirmation_incomplete', count(*)
FROM xz_wechat_virtual_goods
WHERE verification_status = 'MANUALLY_CONFIRMED_PUBLISHED'
  AND (
    nullif(btrim(verified_by), '') IS NOT NULL
    AND verified_at IS NOT NULL
    AND nullif(btrim(verification_reason), '') IS NOT NULL
    AND verification_snapshot @> jsonb_build_object(
      'productId', product_id,
      'offerId', offer_id,
      'environment', environment,
      'platformPriceCents', platform_price_cents
    )
  ) IS NOT TRUE

UNION ALL

SELECT 'wechat_verification_expiry_invalid', count(*)
FROM xz_wechat_virtual_goods
WHERE verification_expires_at IS NOT NULL
  AND verified_at IS NOT NULL
  AND verification_expires_at <= verified_at

UNION ALL

SELECT 'enabled_wechat_good_not_ready', count(*)
FROM xz_price_plans price
JOIN xz_price_plan_payment_bindings binding ON binding.price_plan_id = price.id
JOIN xz_wechat_virtual_goods good ON good.id = binding.wechat_good_id
WHERE price.enabled
  AND binding.enabled
  AND binding.status = 'ACTIVE'
  AND (
    good.enabled IS NOT TRUE
    OR good.published IS NOT TRUE
    OR good.status <> 'PUBLISHED'
    OR good.verification_status <> 'MANUALLY_CONFIRMED_PUBLISHED'
    OR (
      good.verification_expires_at IS NOT NULL
      AND good.verification_expires_at <= clock_timestamp()
    )
  );
\endif

SELECT CASE WHEN EXISTS (
  SELECT 1
  FROM information_schema.columns
  WHERE table_schema = current_schema()
    AND table_name = 'xz_price_plans'
    AND column_name = 'currency'
) THEN 'true' ELSE 'false' END AS has_099
\gset

\if :has_099
\echo '=== K. 099 price-plan contracts: every blocker_count must be zero ==='
SELECT 'multiple_defaults_with_currency' AS check_name, count(*) AS blocker_count
FROM (
  SELECT plan_id, channel, environment, currency
  FROM xz_price_plans
  WHERE is_default
  GROUP BY plan_id, channel, environment, currency
  HAVING count(*) > 1
) violations

UNION ALL

SELECT 'currency_invalid', count(*)
FROM xz_price_plans
WHERE (currency ~ '^[A-Z]{3}$') IS NOT TRUE

UNION ALL

SELECT 'audience_invalid', count(*)
FROM xz_price_plans
WHERE (audience_type IN ('PUBLIC','RULE','WHITELIST','INVITE','TEST')) IS NOT TRUE

UNION ALL

SELECT 'audience_rule_not_object', count(*)
FROM xz_price_plans
WHERE (jsonb_typeof(audience_rule) = 'object') IS NOT TRUE

UNION ALL

SELECT 'price_plan_code_invalid', count(*)
FROM xz_price_plans
WHERE (code ~ '^[a-z][a-z0-9_]{1,62}[a-z0-9]$') IS NOT TRUE

UNION ALL

SELECT 'test_scope_invalid', count(*)
FROM xz_price_plans
WHERE price_type = 'TEST'
  AND (
    is_default = FALSE
    AND is_visible = FALSE
    AND (audience_type <> 'PUBLIC' OR created_by IS NULL)
  ) IS NOT TRUE

UNION ALL

SELECT 'default_state_invalid', count(*)
FROM xz_price_plans
WHERE is_default
  AND (
    price_type <> 'TEST'
    AND enabled
    AND status = 'ACTIVE'
    AND is_visible
    AND audience_type = 'PUBLIC'
  ) IS NOT TRUE

UNION ALL

SELECT 'enabled_gift_points_price_plan', count(*)
FROM xz_price_plans
WHERE enabled AND bonus_points > 0;
\endif

SELECT CASE WHEN to_regclass(current_schema() || '.xz_channel_rollout_configs') IS NOT NULL
THEN 'true' ELSE 'false' END AS has_rollout_config
\gset

\if :has_rollout_config
\echo '=== L. V132 compatibility gate: returned rows are blockers ==='
SELECT tenant_id, mode, enabled, real_switch_enabled
FROM xz_channel_rollout_configs
WHERE enabled = true
  AND real_switch_enabled = true
  AND mode IN ('V132','CANARY','V132_CANARY','V132_FULL')
ORDER BY tenant_id;
\else
\echo 'BLOCKER: xz_channel_rollout_configs is missing'
\endif

SELECT CASE WHEN EXISTS (
  SELECT 1
  FROM information_schema.columns
  WHERE table_schema = current_schema()
    AND table_name = 'xz_price_plan_user_whitelist'
    AND column_name = 'lifecycle_status'
) THEN 'true' ELSE 'false' END AS has_100
\gset

\if :has_100
\echo '=== M. 100 whitelist, quote pin, and audit: every blocker_count must be zero ==='
SELECT 'multiple_effective_active_whitelists' AS check_name, count(*) AS blocker_count
FROM (
  SELECT price_plan_id, user_id
  FROM xz_price_plan_user_whitelist
  WHERE coalesce(
    lifecycle_status,
    CASE WHEN enabled THEN 'ACTIVE' ELSE 'DISABLED' END
  ) = 'ACTIVE'
  GROUP BY price_plan_id, user_id
  HAVING count(*) > 1
) violations

UNION ALL

SELECT 'whitelist_lifecycle_invalid', count(*)
FROM xz_price_plan_user_whitelist
WHERE lifecycle_status IS NOT NULL
  AND lifecycle_status NOT IN ('ACTIVE','EXPIRED','DISABLED')

UNION ALL

SELECT 'whitelist_enabled_inconsistent', count(*)
FROM xz_price_plan_user_whitelist
WHERE lifecycle_status IS NOT NULL
  AND enabled IS DISTINCT FROM (lifecycle_status = 'ACTIVE')

UNION ALL

SELECT 'quote_pin_partial', count(*)
FROM xz_order_price_quotes
WHERE NOT (
  (
    whitelist_entry_id IS NULL
    AND whitelist_revision IS NULL
    AND whitelist_checked_at IS NULL
  ) OR (
    nullif(btrim(whitelist_entry_id), '') IS NOT NULL
    AND whitelist_revision IS NOT NULL
    AND whitelist_revision > 0
    AND whitelist_checked_at IS NOT NULL
  )
)

UNION ALL

SELECT 'quote_pin_orphan_or_wrong_owner', count(*)
FROM xz_order_price_quotes quote
LEFT JOIN xz_price_plan_user_whitelist whitelist
  ON whitelist.id = quote.whitelist_entry_id
WHERE quote.whitelist_entry_id IS NOT NULL
  AND (
    whitelist.id IS NULL
    OR quote.price_plan_id IS DISTINCT FROM whitelist.price_plan_id
    OR quote.user_id IS DISTINCT FROM whitelist.user_id
  )

UNION ALL

SELECT 'live_unpinned_test_quote', count(*)
FROM xz_order_price_quotes
WHERE entry_type = 'TEST'
  AND status = 'AVAILABLE'
  AND expires_at > clock_timestamp()
  AND whitelist_entry_id IS NULL

UNION ALL

SELECT 'live_test_quote_with_ineligible_whitelist', count(*)
FROM xz_order_price_quotes quote
JOIN xz_price_plan_user_whitelist whitelist ON whitelist.id = quote.whitelist_entry_id
WHERE quote.entry_type = 'TEST'
  AND quote.status = 'AVAILABLE'
  AND quote.expires_at > clock_timestamp()
  AND (
    whitelist.enabled IS NOT TRUE
    OR whitelist.lifecycle_status <> 'ACTIVE'
    OR (whitelist.effective_at IS NOT NULL AND whitelist.effective_at > clock_timestamp())
    OR (whitelist.expires_at IS NOT NULL AND whitelist.expires_at <= clock_timestamp())
  )

UNION ALL

SELECT 'pricing_audit_result_invalid', count(*)
FROM xz_audit_logs
WHERE domain LIKE 'PRICING%'
  AND (result IS NULL OR result NOT IN ('SUCCEEDED','FAILED'))

UNION ALL

SELECT 'pricing_audit_required_fields_invalid', count(*)
FROM xz_audit_logs
WHERE domain LIKE 'PRICING%'
  AND (
    nullif(btrim(request_id), '') IS NULL
    OR nullif(btrim(actor_id), '') IS NULL
    OR nullif(btrim(actor_role), '') IS NULL
    OR nullif(btrim(action), '') IS NULL
    OR nullif(btrim(resource), '') IS NULL
    OR nullif(btrim(resource_id), '') IS NULL
    OR nullif(btrim(change_reason), '') IS NULL
    OR result NOT IN ('SUCCEEDED','FAILED')
    OR (
      result = 'SUCCEEDED'
      AND (
        revision_before IS NULL
        OR revision_after IS NULL
        OR after_snapshot IS NULL
        OR (before_snapshot IS NULL AND action !~ '\.(create|clone)$')
      )
    )
  );

\echo '=== N. 100 identity index: one exact row is required ==='
SELECT
  indexed_table.relname AS indexed_table,
  index_method.amname AS index_method,
  index_item.indisunique,
  index_item.indisvalid,
  index_item.indisready,
  index_item.indnkeyatts,
  index_item.indnatts,
  index_item.indpred IS NOT NULL AS has_predicate,
  index_item.indexprs IS NOT NULL AS has_expressions,
  (
    SELECT string_agg(attribute.attname, ',' ORDER BY key_item.ordinality)
    FROM unnest(index_item.indkey) WITH ORDINALITY key_item(attnum, ordinality)
    JOIN pg_attribute attribute
      ON attribute.attrelid = index_item.indrelid
     AND attribute.attnum = key_item.attnum
  ) AS columns
FROM pg_class index_relation
JOIN pg_namespace namespace ON namespace.oid = index_relation.relnamespace
JOIN pg_index index_item ON index_item.indexrelid = index_relation.oid
JOIN pg_class indexed_table ON indexed_table.oid = index_item.indrelid
JOIN pg_am index_method ON index_method.oid = index_relation.relam
WHERE namespace.nspname = current_schema()
  AND index_relation.relname = 'ux_xz_price_plan_whitelist_identity_100';
\endif

\echo '=== O. Actual pricing grants: every non-SUPER_ADMIN row needs approval ==='
SELECT
  role,
  array_agg(permission ORDER BY permission) AS pricing_permissions
FROM xz_role_permissions
WHERE permission LIKE 'pricing:%'
GROUP BY role
ORDER BY role;

SELECT
  users.role,
  users.status,
  count(*) AS account_count
FROM xz_users users
WHERE users.role IN (
  SELECT DISTINCT role
  FROM xz_role_permissions
  WHERE permission LIKE 'pricing:%'
)
GROUP BY users.role, users.status
ORDER BY users.role, users.status;

\echo '=== P. 097-100 NOT VALID status: preserve output for validation approval ==='
SELECT
  conrelid::regclass AS table_name,
  conname,
  contype,
  convalidated,
  pg_get_constraintdef(oid, true) AS definition
FROM pg_constraint
WHERE conname ~ '_(097|098|099|100)$'
ORDER BY conrelid::regclass::text, conname;

\if :has_100
\echo '=== P2. Full expected constraint ownership: expected zero rows ==='
WITH expected(table_name, constraint_name) AS (
  VALUES
    ('xz_orders','fk_xz_orders_plan_version_097'),
    ('xz_orders','fk_xz_orders_price_plan_097'),
    ('xz_orders','fk_xz_orders_price_quote_097'),
    ('xz_orders','ck_xz_orders_snapshot_v2_097'),
    ('xz_wechat_virtual_goods','ck_xz_wechat_goods_verification_status_098'),
    ('xz_wechat_virtual_goods','ck_xz_wechat_goods_manual_confirmation_098'),
    ('xz_wechat_virtual_goods','ck_xz_wechat_goods_verification_expiry_098'),
    ('xz_price_plans','ck_xz_price_plans_currency_099'),
    ('xz_price_plans','ck_xz_price_plans_audience_099'),
    ('xz_price_plans','ck_xz_price_plans_audience_rule_099'),
    ('xz_price_plans','ck_xz_price_plans_code_format_099'),
    ('xz_price_plans','ck_xz_price_plans_test_scope_099'),
    ('xz_price_plans','ck_xz_price_plans_default_state_099'),
    ('xz_order_price_quotes','ck_xz_order_price_quotes_whitelist_pin_100'),
    ('xz_price_plan_user_whitelist','ck_xz_price_plan_whitelist_lifecycle_100'),
    ('xz_price_plan_user_whitelist','ck_xz_price_plan_whitelist_enabled_100'),
    ('xz_order_price_quotes','fk_xz_order_price_quotes_whitelist_100'),
    ('xz_audit_logs','ck_xz_audit_logs_pricing_result_100'),
    ('xz_audit_logs','ck_xz_audit_logs_pricing_required_100')
)
SELECT
  expected.table_name,
  expected.constraint_name,
  count(constraint_item.oid) AS found_count,
  string_agg(
    coalesce(namespace.nspname, '(missing)') || '.' || coalesce(relation.relname, '(missing)'),
    ',' ORDER BY namespace.nspname, relation.relname
  ) AS actual_owners
FROM expected
LEFT JOIN pg_constraint constraint_item
  ON constraint_item.conname = expected.constraint_name
LEFT JOIN pg_class relation ON relation.oid = constraint_item.conrelid
LEFT JOIN pg_namespace namespace ON namespace.oid = relation.relnamespace
GROUP BY expected.table_name, expected.constraint_name
HAVING count(constraint_item.oid) <> 1
   OR coalesce(bool_and(
        namespace.nspname = current_schema()
        AND relation.relname = expected.table_name
      ), false) IS NOT TRUE
ORDER BY expected.table_name, expected.constraint_name;
\endif

\echo '=== Q. V2 order/quote rollback inventory ==='
\if :has_097
SELECT status, fulfillment_status, entitlement_status, count(*) AS order_count
FROM xz_orders
WHERE snapshot_version = 2
GROUP BY status, fulfillment_status, entitlement_status
ORDER BY status, fulfillment_status, entitlement_status;

SELECT status, entry_type, count(*) AS quote_count
FROM xz_order_price_quotes
WHERE snapshot_version = 2
GROUP BY status, entry_type
ORDER BY status, entry_type;
\endif

\echo '=== R. Relevant table sizes for lock-window rehearsal ==='
SELECT
  class.relname,
  pg_total_relation_size(class.oid) AS total_bytes,
  pg_size_pretty(pg_total_relation_size(class.oid)) AS total_size
FROM pg_class class
JOIN pg_namespace namespace ON namespace.oid = class.relnamespace
WHERE namespace.nspname = current_schema()
  AND class.relkind IN ('r','p')
  AND class.relname IN (
    'xz_plans',
    'xz_orders',
    'xz_audit_logs',
    'xz_role_permissions',
    'xz_plan_versions',
    'xz_price_plans',
    'xz_wechat_virtual_goods',
    'xz_price_plan_payment_bindings',
    'xz_price_plan_user_whitelist',
    'xz_order_price_quotes'
  )
ORDER BY pg_total_relation_size(class.oid) DESC;

ROLLBACK;
