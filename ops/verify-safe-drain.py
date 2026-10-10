#!/usr/bin/env python3
"""Read-only drain observations; supports approved quarantine manifest exemptions.

Compatible with Python 3.6. See docs/architecture/issue199-quarantine-drain.md.
"""
import datetime
import hashlib
import json
import math
import os
import re
import shlex
import sys
import time
import types

# Execute proof-bound source bytes, never an ignored/unbound .pyc cache.
sys.dont_write_bytecode = True
# Source-only loading: approval helper is protected by the same Prestage proof.
_approval_path = os.path.join(os.path.dirname(__file__), 'quarantine-approval.py')
approval = types.ModuleType('quarantine_approval')
approval.__file__ = _approval_path
with open(_approval_path, 'rb') as _source:
    exec(compile(_source.read(), _approval_path, 'exec'), approval.__dict__)
# Complete canonical projection API; live approval still fails closed until the
# Carrier-bound authority registry is provisioned with the human approver key.
_snapshot_path = os.path.join(os.path.dirname(__file__), 'quarantine-live-snapshot.py')
live_snapshot = types.ModuleType('quarantine_live_snapshot')
live_snapshot.__file__ = _snapshot_path
with open(_snapshot_path, 'rb') as _source:
    exec(compile(_source.read(), _snapshot_path, 'exec'), live_snapshot.__dict__)
_runtime_path = os.path.join(os.path.dirname(__file__), 'verify-release-runtime.py')
runtime = types.ModuleType('release_runtime')
runtime.__file__ = _runtime_path
with open(_runtime_path, 'rb') as _source:
    exec(compile(_source.read(), _runtime_path, 'exec'), runtime.__dict__)
# Shared source-only psql transport; included in the verified Prestage proof.
_transport_path = os.path.join(os.path.dirname(__file__), 'quarantine-psql-transport.py')
transport = types.ModuleType('quarantine_psql_transport')
transport.__file__ = _transport_path
with open(_transport_path, 'rb') as _source:
    exec(compile(_source.read(), _transport_path, 'exec'), transport.__dict__)

GateError = runtime.GateError

_ISO8601_RE = re.compile(
    r'^(\d{4})-(\d{2})-(\d{2})[T ](\d{2}):(\d{2}):(\d{2})(?:\.(\d+))?(?:Z|([+-]\d{2}):?(\d{2}))?$'
)

def parse_iso8601_utc(ts_str):
    if not isinstance(ts_str, str):
        raise ValueError("Timestamp must be a string")
    m = _ISO8601_RE.match(ts_str.strip())
    if not m:
        raise ValueError(f"Invalid ISO 8601 timestamp: {ts_str}")
    year, month, day, hour, minute, second, frac, tz_h, tz_m = m.groups()
    microsecond = int((frac or '0')[:6].ljust(6, '0'))
    if tz_h is not None and tz_m is not None:
        offset_minutes = int(tz_h) * 60 + (int(tz_m) if int(tz_h) >= 0 else -int(tz_m))
        tz = datetime.timezone(datetime.timedelta(minutes=offset_minutes))
    else:
        tz = datetime.timezone.utc
    dt = datetime.datetime(
        int(year), int(month), int(day),
        int(hour), int(minute), int(second),
        microsecond, tzinfo=tz
    )
    return dt.astimezone(datetime.timezone.utc)

def build_sql(exempt_ids=None):
    if exempt_ids:
        clean_ids = [str(int(i)) for i in exempt_ids]
        not_in_clause = f" AND e.id NOT IN ({','.join(clean_ids)})"
    else:
        not_in_clause = ""
    return f"""
SELECT
 (SELECT count(*) FROM public.xz_generation_tasks
  WHERE lease_until > now()
     OR status IS NULL OR task_status IS NULL
     OR upper(status) NOT IN ('COMPLETED','SUCCEEDED','FAILED','CANCELLED')
     OR upper(task_status) NOT IN ('SUCCEEDED','FAILED','CANCELLED')
     OR (CASE upper(status) WHEN 'COMPLETED' THEN 'SUCCEEDED' ELSE upper(status) END)
        IS DISTINCT FROM upper(task_status)
     OR id IS NULL)
 + (SELECT count(*) FROM public.provider_executions e
    WHERE (e.status IS NULL OR e.status NOT IN ('succeeded','failed'){not_in_clause})
       OR e.id IS NULL OR e.task_id IS NULL OR e.attempt IS NULL OR e.attempt < 1
       OR (SELECT count(*) FROM public.xz_generation_tasks t WHERE t.id = e.task_id) <> 1)
 + (SELECT count(*) FROM (SELECT id FROM public.xz_generation_tasks GROUP BY id HAVING count(*) > 1) d)
 + (SELECT count(*) FROM (SELECT id FROM public.provider_executions GROUP BY id HAVING count(*) > 1) d)
 + (SELECT count(*) FROM (SELECT task_id, attempt FROM public.provider_executions
                         GROUP BY task_id, attempt HAVING count(*) > 1) d)
 + (SELECT count(*) FROM public.outbox_events
    WHERE status IS NULL OR status NOT IN ('published','failed'))
 + (SELECT count(*) FROM public.video_task_outbox
    WHERE state IS NULL OR state NOT IN ('published','failed'));
"""


SQL = build_sql()


HISTORY_SCHEMA = {'consumer_inbox': [['consumer_name', 'text', 'NO', None, None, None], ['created_at', 'timestamptz', 'NO', None, None, None], ['error_message', 'text', 'YES', None, None, None], ['event_id', 'text', 'NO', None, None, None], ['id', 'int8', 'NO', None, 64, 0], ['metadata', 'jsonb', 'YES', None, None, None], ['processed_at', 'timestamptz', 'YES', None, None, None], ['result', 'text', 'YES', None, None, None]], 'outbox_events': [['aggregate_id', 'text', 'NO', None, None, None], ['aggregate_type', 'text', 'NO', None, None, None], ['attempt_count', 'int4', 'NO', None, 32, 0], ['claim_owner', 'text', 'YES', None, None, None], ['claimed_at', 'timestamptz', 'YES', None, None, None], ['created_at', 'timestamptz', 'NO', None, None, None], ['event_id', 'text', 'NO', None, None, None], ['event_type', 'text', 'NO', None, None, None], ['event_version', 'int4', 'NO', None, 32, 0], ['id', 'int8', 'NO', None, 64, 0], ['last_error', 'text', 'YES', None, None, None], ['next_attempt_at', 'timestamptz', 'NO', None, None, None], ['payload', 'jsonb', 'NO', None, None, None], ['published_at', 'timestamptz', 'YES', None, None, None], ['status', 'text', 'NO', None, None, None], ['trace_id', 'text', 'YES', None, None, None], ['updated_at', 'timestamptz', 'NO', None, None, None]], 'provider_execution_quarantine': [['approval_id', 'text', 'NO', None, None, None], ['attempt', 'int4', 'NO', None, 32, 0], ['created_at', 'timestamptz', 'NO', None, None, None], ['evidence_sha256', 'bpchar', 'NO', 64, None, None], ['execution_id', 'int8', 'NO', None, 64, 0], ['expires_at', 'timestamptz', 'NO', None, None, None], ['generation', 'int8', 'YES', None, 64, 0], ['not_before', 'timestamptz', 'NO', None, None, None], ['release_sha', 'bpchar', 'NO', 40, None, None], ['snapshot_sha256', 'bpchar', 'NO', 64, None, None], ['task_id', 'text', 'NO', None, None, None]], 'schema_migrations': [['applied_at', 'timestamptz', 'NO', None, None, None], ['filename', 'text', 'NO', None, None, None]], 'video_task_outbox': [['aggregate_id', 'text', 'NO', None, None, None], ['aggregate_type', 'varchar', 'NO', 32, None, None], ['attempts', 'int4', 'NO', None, 32, 0], ['available_at', 'timestamptz', 'NO', None, None, None], ['created_at', 'timestamptz', 'NO', None, None, None], ['event_type', 'varchar', 'NO', 64, None, None], ['id', 'int8', 'NO', None, 64, 0], ['last_error', 'text', 'YES', None, None, None], ['payload', 'jsonb', 'NO', None, None, None], ['published_at', 'timestamptz', 'YES', None, None, None], ['state', 'varchar', 'NO', 16, None, None], ['tenant_id', 'text', 'NO', None, None, None]]}

# Non-enrolled historical classification is independent of exact9 approvals.
# Eligibility never treats a NULL generation as a fence. Full evidence stays
# inside PostgreSQL; only canonical row digests leave this read-only snapshot.
HISTORY_PROTOCOL = 1
# Exact source constants in generation_worker.go; pending claims have no task
# metadata until completion, so neither age nor NULL metadata proves inactivity.
HISTORY_IMAGE_CONSUMERS = ('generation-image-canary-worker', 'generation-image-normal-worker')
HISTORY_KEYS_SQL = """
SELECT c.relname,p.contype::text,pg_catalog.pg_get_constraintdef(p.oid)
FROM pg_catalog.pg_constraint p JOIN pg_catalog.pg_class c ON c.oid=p.conrelid
JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
WHERE n.nspname='public' AND c.relname IN ('xz_generation_tasks','provider_executions')
AND p.contype IN ('p','u') ORDER BY c.relname COLLATE "C",p.contype::text COLLATE "C"
"""
HISTORY_KEYS = [('provider_executions', 'p', 'PRIMARY KEY (id)'),
                ('provider_executions', 'u', 'UNIQUE (task_id, attempt)'),
                ('xz_generation_tasks', 'p', 'PRIMARY KEY (id)')]
HISTORY_RAW_STRINGS = (('id', 'id'), ('userId', 'user_id'), ('type', 'type'),
                       ('status', 'status'), ('model', 'model'), ('prompt', 'prompt'))
HISTORY_RAW_OPTIONAL_STRINGS = (('tenantId', 'tenant_id'), ('organizationId', 'organization_id'),
                               ('moduleCode', 'module_code'), ('billingAccountType', 'billing_account_type'),
                               ('billingAccountId', 'billing_account_id'), ('billingType', 'billing_type'))
# generationTaskForUpdate decodes these fields from raw; only fencing and its
# explicit financial/status scan columns override raw. Do not coerce JSON NULL
# or accept encoding/json's case-insensitive aliases for behavior identities.
HISTORY_RAW_PARITY_SQL = ' AND '.join(
    ["jsonb_typeof(t.raw->'%s')='string' AND t.raw->>'%s'=t.%s" % (tag, tag, column)
     for tag, column in HISTORY_RAW_STRINGS] +
    ["((t.%s IS NOT NULL AND t.%s<>'' AND jsonb_typeof(t.raw->'%s')='string' AND t.raw->>'%s'=t.%s) OR ((t.%s IS NULL OR t.%s='') AND (NOT(t.raw ? '%s') OR t.raw->'%s'='\"\"'::jsonb)))" % (column, column, tag, tag, column, column, column, tag, tag)
     for tag, column in HISTORY_RAW_OPTIONAL_STRINGS] +
    ["jsonb_typeof(t.raw->'params')='object' AND t.raw->'params'=t.params"] +
    ["NOT EXISTS(SELECT 1 FROM jsonb_object_keys(t.raw) k WHERE lower(k)=lower('%s') AND k<>'%s')" % (tag, tag)
     for tag, column in HISTORY_RAW_STRINGS + HISTORY_RAW_OPTIONAL_STRINGS + (('params', 'params'),)])
HISTORY_CTE = """
WITH RECURSIVE historical_success AS (
 SELECT t.id FROM public.xz_generation_tasks t JOIN public.provider_executions e ON e.task_id=t.id
 WHERE t.type IN ('TEXT_TO_IMAGE','IMAGE_TO_IMAGE')
 AND t.status='PROCESSING' AND t.task_status='DISPATCHING'
 AND t.id IS NOT NULL AND btrim(t.id)<>''
 AND t.execution_generation>0 AND e.task_execution_generation>0
 AND e.task_execution_generation<t.execution_generation
 AND NOT coalesce(t.lease_until>now(),false)
 AND jsonb_typeof(t.params)='object' AND jsonb_typeof(t.raw)='object'
 AND """ + HISTORY_RAW_PARITY_SQL + """
 AND coalesce(t.params->>'generation_dispatch_mode','')=''
 AND NOT (t.params ? 'generation_async_canary')
 AND NOT (t.params ? '_generation_fair_scheduled')
 AND NOT (t.params ? '_generation_dispatch_owner')
 AND e.id>0 AND e.attempt=1 AND e.capability='image'
 AND e.status='succeeded' AND e.error_class='provider_succeeded' AND e.error_code IS NULL
 AND e.request_fingerprint::text ~ '^[0-9a-f]{64}$'
 AND jsonb_typeof(e.result_metadata)='array' AND e.result_metadata<>'[]'::jsonb
 AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(e.result_metadata)='array' THEN e.result_metadata ELSE '[]'::jsonb END) v WHERE jsonb_typeof(v)<>'object' OR jsonb_typeof(v->'URL') IS DISTINCT FROM 'string' OR btrim(v->>'URL')='' OR (v ? 'ContentType' AND jsonb_typeof(v->'ContentType')<>'string'))
 AND e.next_check_at IS NULL
 AND (SELECT count(*) FROM public.xz_generation_tasks u WHERE u.id=t.id)=1
 AND (SELECT count(*) FROM public.provider_executions u WHERE u.task_id=t.id)=1
 AND (SELECT count(*) FROM public.provider_executions u WHERE u.id=e.id)=1
 AND NOT EXISTS (SELECT 1 FROM public.outbox_events o WHERE o.aggregate_id=t.id AND (o.status IS NULL OR o.status NOT IN ('published','failed')))
 AND NOT EXISTS (SELECT 1 FROM public.video_task_outbox o WHERE o.aggregate_id=t.id AND (o.state IS NULL OR o.state NOT IN ('published','failed')))
 AND NOT EXISTS (SELECT 1 FROM public.xz_assets a WHERE a.task_id=t.id)
 AND t.result_ids='[]'::jsonb AND NOT (t.raw ? 'generatedImages')
 AND NOT EXISTS (SELECT 1 FROM public.provider_execution_quarantine q WHERE q.task_id=t.id)
), historical_orphan AS (
 SELECT e.id FROM public.provider_executions e
 WHERE e.id>0 AND e.attempt=1 AND btrim(e.task_id)<>''
 AND e.status='failed' AND e.error_class='definitive_not_submitted' AND e.capability='image'
 AND e.task_execution_generation IS NULL AND e.provider_request_id IS NULL
 AND e.result_metadata IS NULL AND e.next_check_at IS NULL AND e.error_code IS NULL
 AND e.request_fingerprint::text ~ '^[0-9a-f]{64}$'
 AND (SELECT count(*) FROM public.schema_migrations WHERE filename='119-execution-generation-fencing.sql')=1
 AND e.created_at<(SELECT applied_at FROM public.schema_migrations WHERE filename='119-execution-generation-fencing.sql')
 AND e.updated_at<(SELECT applied_at FROM public.schema_migrations WHERE filename='119-execution-generation-fencing.sql')
 AND (SELECT count(*) FROM public.xz_generation_tasks t WHERE t.id=e.task_id)=0
 AND (SELECT count(*) FROM public.provider_executions u WHERE u.task_id=e.task_id)=1
 AND (SELECT count(*) FROM public.provider_executions u WHERE u.id=e.id)=1
 AND NOT EXISTS (SELECT 1 FROM public.outbox_events o WHERE o.aggregate_id=e.task_id AND (o.status IS NULL OR o.status NOT IN ('published','failed')))
 AND NOT EXISTS (SELECT 1 FROM public.video_task_outbox o WHERE o.aggregate_id=e.task_id AND (o.state IS NULL OR o.state NOT IN ('published','failed')))
 AND NOT EXISTS (SELECT 1 FROM public.xz_assets a WHERE a.task_id=e.task_id)
 AND NOT EXISTS (SELECT 1 FROM public.provider_execution_quarantine q WHERE q.execution_id=e.id)
), historical_ids AS (
 SELECT id AS task_id FROM historical_success
 UNION SELECT task_id FROM public.provider_executions WHERE id IN (SELECT id FROM historical_orphan)
), historical_tasks AS (
 SELECT * FROM public.xz_generation_tasks WHERE id IN (SELECT task_id FROM historical_ids)
)
"""


def build_history_sql(exempt_ids=None):
    # Keep the original strict count and independent lease predicate intact.
    strict = build_sql(exempt_ids).strip().rstrip(';')
    strict += " + (SELECT count(*) FROM public.consumer_inbox WHERE processed_at IS NULL AND (consumer_name IN ('generation-image-canary-worker','generation-image-normal-worker') OR metadata->>'task_id' IN (SELECT task_id FROM historical_ids) OR event_id IN (SELECT event_id FROM public.outbox_events WHERE aggregate_id IN (SELECT task_id FROM historical_ids))))"
    strict = strict.replace('WHERE lease_until > now()\n     OR status IS NULL',
                            'WHERE lease_until > now()\n     OR (status IS NULL')
    strict = strict.replace('OR id IS NULL)',
                            'OR id IS NULL) AND NOT EXISTS (SELECT 1 FROM historical_success h WHERE h.id=xz_generation_tasks.id))')
    strict = strict.replace('t.id = e.task_id) <> 1)',
                            't.id = e.task_id) <> 1 AND NOT EXISTS (SELECT 1 FROM historical_orphan h WHERE h.id=e.id))')
    # Hash only classified history and its linked context. Global strict
    # counters above still watch ALL leases, identity errors and transport.
    # Unrelated funded normal work must be able to finish during the drain.
    task_ids = "SELECT task_id FROM historical_ids"
    executions = "SELECT id FROM public.provider_executions WHERE task_id IN (" + task_ids + ")"
    accounts = "SELECT billing_account_id FROM historical_tasks WHERE billing_account_id IS NOT NULL"
    users = "SELECT user_id FROM historical_tasks WHERE user_id IS NOT NULL"
    accounts += " UNION SELECT id FROM public.xz_point_accounts WHERE user_id IN (" + users + ") UNION SELECT account_id FROM public.xz_wallet_ledger WHERE task_id IN (" + task_ids + ")"
    tenants = "SELECT tenant_id FROM historical_tasks WHERE tenant_id IS NOT NULL"
    reservations = "SELECT id FROM public.xz_personal_point_reservations WHERE business_id IN (" + task_ids + ")"
    lots = "SELECT lot_id FROM public.xz_personal_point_reservation_allocations WHERE reservation_id IN (" + reservations + ") UNION SELECT id FROM public.xz_personal_point_lots WHERE account_id IN (" + accounts + ") OR user_id IN (" + users + ")"
    predicates = {
        'xz_generation_tasks': 'r.id IN (' + task_ids + ')',
        'provider_executions': 'r.task_id IN (' + task_ids + ')',
        'provider_execution_correlations': 'r.execution_id IN (' + executions + ')',
        'xz_billing_events': "r.task_id IN (" + task_ids + ") OR r.raw->>'taskId' IN (" + task_ids + ") OR r.id IN (SELECT billing_event_id FROM public.xz_wallet_ledger WHERE task_id IN (" + task_ids + "))",
        'xz_billing_lifecycle_events': 'r.task_id IN (' + task_ids + ')',
        'xz_wallet_ledger': 'r.task_id IN (' + task_ids + ') OR r.reference_id IN (' + task_ids + ')',
        'xz_personal_point_reservations': 'r.business_id IN (' + task_ids + ')',
        'xz_personal_point_reservation_allocations': 'r.reservation_id IN (' + reservations + ')',
        'xz_personal_point_lot_movements': 'r.reservation_id IN (' + reservations + ') OR r.reference_id IN (' + task_ids + ')',
        'xz_personal_point_lots': 'r.id IN (' + lots + ')',
        'xz_point_accounts': 'r.id IN (' + accounts + ') OR r.user_id IN (' + users + ')',
        'xz_user_wallets': 'r.user_id IN (' + users + ')',
        'xz_tenant_wallets': 'r.tenant_id IN (' + tenants + ')',
        'outbox_events': 'r.aggregate_id IN (' + task_ids + ')',
        'video_task_outbox': 'r.aggregate_id IN (' + task_ids + ')',
        'consumer_inbox': "r.event_id IN (SELECT event_id FROM public.outbox_events WHERE aggregate_id IN (" + task_ids + ")) OR r.metadata->>'task_id' IN (" + task_ids + ")",
        'provider_execution_quarantine': 'r.task_id IN (' + task_ids + ') OR r.execution_id IN (' + executions + ')',
        'schema_migrations': "r.filename='119-execution-generation-fencing.sql'",
    }
    # Reuse one source-bound output graph for the classified root set.
    # Materialize its closure once, not once per table and historical task.
    graph = live_snapshot._ASSET_GRAPH.split('result_refs AS (', 1)[1]
    graph = ', t AS (SELECT * FROM historical_tasks), result_refs AS (' + graph
    # Identity roots include taskless orphans; t remains actual task rows only.
    # Traverse relations with UNION (visited nodes), including cycles/diamonds.
    graph = graph.replace("UNION SELECT 'f:'||ref,'a:'||id FROM asset_refs",
                          "UNION SELECT 'f:'||ref,'a:'||id FROM asset_refs"
                          " UNION SELECT 'f:'||source_file_id,'f:'||target_file_id FROM public.xz_file_relations"
                          " UNION SELECT 'f:'||target_file_id,'f:'||source_file_id FROM public.xz_file_relations")
    graph = graph.replace('FROM public.xz_file_objects f,t', 'FROM public.xz_file_objects f,historical_ids h')
    graph = graph.replace("f.business_id=t.id OR left(f.original_name,length(t.id)+1)=t.id||'-'",
                          "f.business_id=h.task_id OR left(f.original_name,length(h.task_id)+1)=h.task_id||'-'")
    graph = graph.replace('SELECT to_jsonb(id) FROM t UNION', 'SELECT to_jsonb(task_id) FROM historical_ids UNION')
    graph = graph.replace("SELECT 1 FROM t WHERE left(m.file_name,length(t.id)+1)=t.id||'-'",
                          "SELECT 1 FROM historical_ids h WHERE left(m.file_name,length(h.task_id)+1)=h.task_id||'-'")
    for table, alias, key in [('xz_assets','aa','id'), ('xz_file_objects','ff','file_id'),
                              ('xz_storage_configs','cc','id'), ('xz_file_relations','rr','id'),
                              ('xz_storage_jobs','jj','id'), ('xz_multipart_uploads','mm','id')]:
        predicates[table] = 'r.' + key + ' IN (SELECT ' + key + ' FROM ' + alias + ')'
    predicates['xz_multipart_upload_parts'] = 'r.upload_id IN (SELECT id FROM mm)'
    parts = []
    for table, predicate in sorted(predicates.items()):
        parts.append("'%s', (SELECT coalesce(jsonb_agg(encode(public.digest(convert_to(to_jsonb(r)::text,'UTF8'),'sha256'),'hex') ORDER BY to_jsonb(r)::text),'[]'::jsonb) FROM public.%s r WHERE %s)" % (table, table, predicate))
    digest = "encode(public.digest(convert_to(jsonb_build_object(" + ','.join(parts) + ")::text,'UTF8'),'sha256'),'hex')"
    return 'SELECT * FROM (' + HISTORY_CTE + graph + ' SELECT (' + strict + ')::bigint, ' + digest + ') historical_observation;'


def history_observation(target, exempt_ids):
    conn = None
    try:
        # Revalidate the proof/image/helper binding on EVERY observation. An
        # absent optional capability returns strict; malformed evidence rejects.
        if not target.history_enabled():
            raise GateError('historical capability disappeared during drain')
        conn = target.connect()
        cur = conn.cursor()
        cur.execute('BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY')
        live_snapshot._settings(cur)
        schema = dict(live_snapshot.SCHEMA)
        for extra in (live_snapshot.FINANCIAL_SCHEMA, live_snapshot.ENTERPRISE_FINANCIAL_SCHEMA,
                      live_snapshot.ASSET_STORAGE_SCHEMA, HISTORY_SCHEMA):
            schema.update(extra)
        live_snapshot._schema(cur, schema)
        cur.execute(HISTORY_KEYS_SQL)
        if [tuple(row) for row in cur.fetchall()] != HISTORY_KEYS:
            raise GateError('historical identity schema rejected')
        cur.execute(build_history_sql(exempt_ids))
        rows = cur.fetchall()
        if (len(rows) != 1 or len(rows[0]) != 2 or type(rows[0][0]) is not int or rows[0][0] < 0 or
                not isinstance(rows[0][1], str) or not re.fullmatch('[0-9a-f]{64}', rows[0][1])):
            raise GateError('invalid historical evidence projection')
        cur.execute('ROLLBACK')
        target.check()
        return tuple(rows[0])
    except GateError:
        raise
    except Exception:
        raise GateError('historical read-only observation rejected')
    finally:
        if conn is not None:
            conn.close()


def database_count(cmd, execute, sql_query=None, target=None):
    if sql_query is None:
        sql_query = SQL
    if target is not None:
        conn = None
        try:
            conn = target.connect()
            cur = conn.cursor()
            cur.execute('BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY')
            cur.execute(sql_query)
            rows = cur.fetchall()
            if len(rows) != 1 or len(rows[0]) != 1 or type(rows[0][0]) is not int or rows[0][0] < 0:
                raise GateError('invalid drain count')
            cur.execute('ROLLBACK')
            target.check()
            return rows[0][0]
        except GateError:
            raise
        except Exception:
            raise GateError('PostgreSQL drain check query execution failed')
        finally:
            if conn is not None:
                conn.close()
    shell = ('PGPASSWORD="$POSTGRES_PASSWORD" '
             'PGOPTIONS="-c default_transaction_read_only=on -c statement_timeout=10000" '
             'psql -X -U "$POSTGRES_USER" -d "$POSTGRES_DB" '
             '-v ON_ERROR_STOP=1 -t -A -c ' + shlex.quote(sql_query))
    try:
        result = execute(cmd + ['exec', '-T', 'postgres', 'sh', '-c', shell])
        if not re.fullmatch(r'[0-9]+', result):
            raise ValueError('invalid count')
        return int(result)
    except Exception:
        raise GateError('PostgreSQL drain check query execution failed')


def validate_manifest(cmd, execute, manifest_path, expected_release_sha, expected_manifest_sha256=None, cursor=None, target=None):
    if not os.path.isfile(manifest_path):
        raise GateError("quarantine manifest input missing")
    try:
        with open(manifest_path, "rb") as f:
            raw_bytes = f.read(approval.MAX_BYTES + 1)
            manifest = approval.decode(raw_bytes)
    except Exception as e:
        raise GateError("malformed quarantine manifest JSON")

    if expected_manifest_sha256:
        actual_hash = hashlib.sha256(raw_bytes).hexdigest()
        if actual_hash.lower() != expected_manifest_sha256.lower():
            raise GateError(f"manifest SHA256 mismatch: expected {expected_manifest_sha256}, got {actual_hash}")

    release_sha = manifest.get("release_sha", "")
    if str(release_sha).lower() == "f9cdf44ca79272ad7cead33dfb1d35fdf155f05f":
        raise GateError("PERMANENTLY_REJECTED_CARRIER: Base commit f9cdf44ca is permanently disqualified from production enrollment.")

    if release_sha != expected_release_sha:
        raise GateError("manifest release_sha mismatch")

    # Validate manifest and recompute live snapshot on read-only sampling cursor
    if cursor is not None:
        try:
            verified = live_snapshot.validate_live_snapshot_in_transaction(
                cursor, raw_bytes, expected_release_sha, 'drain-exemption', expected_manifest_sha256
            )
            return [e['execution_id'] for e in verified.get('executions', [])]
        except live_snapshot.SnapshotError as err:
            raise GateError(str(err))
        except Exception as err:
            raise GateError(f"live snapshot validation failed: {err}")

    # Host validation stays on the existing full projector, never a count-only fallback.
    if target is None:
        raise GateError('verified Prestage database target required')
    conn = None
    try:
        conn = target.connect()
        cur = conn.cursor()
        cur.execute('BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY')
        verified = live_snapshot.validate_live_snapshot_in_transaction(
            cur, raw_bytes, expected_release_sha, 'drain-exemption', expected_manifest_sha256)
        cur.execute('ROLLBACK')
        target.check()
        return [e['execution_id'] for e in verified['executions']]
    except live_snapshot.SnapshotError as err:
        raise GateError(str(err))
    except Exception:
        raise GateError('safe drain live snapshot sampling failed')
    finally:
        if conn is not None:
            conn.close()


def observe(cmd, execute, sql_query=None, target=None, historical=False, exempt_ids=None):
    if sql_query is None:
        sql_query = SQL
    ids = []
    for service in ('xianzhi-ai', 'smartvideo-worker'):
        try:
            cid = execute(cmd + ['ps', '-q', service])
        except Exception:
            raise GateError('Container inspection failed during drain check')
        if not re.fullmatch(r'[a-zA-Z0-9_-]+', cid or ''):
            raise GateError('Required old API/worker container is absent or ambiguous')
        ids.append(cid)
    count = history_observation(target, exempt_ids) if historical else database_count(cmd, execute, sql_query, target)
    queues = json.loads(execute(cmd + ['exec', '-T', 'xianzhi-ai', 'python3', '-c', runtime.BROKER]))
    runtime.queues_ok(queues, 'pre')
    return count, ids


def verify(compose, env, timeout, execute=runtime.run, pause=time.sleep, clock=time.monotonic, manifest_path=None, release_sha=None, expected_manifest_sha256=None, target=None):
    timeout = float(timeout)
    if not math.isfinite(timeout) or timeout < 0 or timeout > 300:
        raise GateError('invalid drain timeout (must be 0..300 seconds)')
    cmd = ['docker', 'compose', '-f', compose, '--env-file', env]

    exempt_ids = None
    if (manifest_path is None) != (release_sha is None):
        raise GateError('manifest and release must be supplied together')
    if manifest_path is not None:
        if expected_manifest_sha256 is None:
            try:
                with open(manifest_path, 'rb') as stream:
                    expected_manifest_sha256 = hashlib.sha256(stream.read(approval.MAX_BYTES + 1)).hexdigest()
            except OSError:
                raise GateError('manifest input failed')
        exempt_ids = validate_manifest(cmd, execute, manifest_path, release_sha, expected_manifest_sha256=expected_manifest_sha256, target=target)

    sql_query = build_sql(exempt_ids)

    historical = target is not None and hasattr(target, 'history_enabled') and target.history_enabled()
    deadline = clock() + timeout
    initial_ids = None
    initial_evidence = None
    while True:
        if manifest_path is not None:
            validate_manifest(cmd, execute, manifest_path, release_sha,
                              expected_manifest_sha256=expected_manifest_sha256, target=target)
        if historical:
            evidence, ids = observe(cmd, execute, sql_query, target, historical=True, exempt_ids=exempt_ids)
            count, digest = evidence
            if initial_evidence is not None and digest != initial_evidence:
                raise GateError('historical row set or evidence changed; fresh drain required')
            initial_evidence = digest
        else:
            count, ids = observe(cmd, execute, sql_query, target)
        if initial_ids is not None and ids != initial_ids:
            raise GateError('runtime identity changed during drain observation')
        initial_ids = ids
        if count == 0:
            pause(1)
            if manifest_path is not None:
                validate_manifest(cmd, execute, manifest_path, release_sha,
                                  expected_manifest_sha256=expected_manifest_sha256, target=target)
            if historical:
                (final_count, final_digest), final_ids = observe(cmd, execute, sql_query, target, historical=True, exempt_ids=exempt_ids)
                if final_digest != initial_evidence:
                    raise GateError('historical row set or evidence changed; fresh drain required')
            else:
                final_count, final_ids = observe(cmd, execute, sql_query, target)
            if final_count or final_ids != ids:
                raise GateError('release observation changed; retry from a fresh drain')
            return
        if clock() >= deadline:
            raise GateError('active valid leases or in-flight operations in progress; safe drain blocked')
        pause(2)


def main():
    try:
        manifest_path = None
        release_sha = None
        expected_manifest_sha256 = None
        args = sys.argv[1:]
        proof_path = None
        if len(args) >= 2 and args[-2] == '--prestage-proof':
            proof_path = args[-1]
            args = args[:-2]
        if len(args) == 3:
            compose, env, timeout = args
        elif len(args) >= 7 and args[3] == '--manifest' and args[5] == '--release-sha':
            compose, env, timeout = args[0], args[1], args[2]
            manifest_path = args[4]
            release_sha = args[6]
            if len(args) == 9 and args[7] == '--expected-manifest-sha256':
                expected_manifest_sha256 = args[8]
            elif len(args) != 7:
                raise GateError('usage: verify-safe-drain.py compose env timeout [--manifest <path> --release-sha <sha> [--expected-manifest-sha256 <sha>]]')
        else:
            raise GateError('usage: verify-safe-drain.py compose env timeout [--manifest <path> --release-sha <sha> [--expected-manifest-sha256 <sha>]]')
        if manifest_path and expected_manifest_sha256:
            with open(manifest_path, 'rb') as stream:
                actual_hash = hashlib.sha256(stream.read(approval.MAX_BYTES + 1)).hexdigest()
            if actual_hash != expected_manifest_sha256:
                raise GateError('manifest SHA256 mismatch')
        if str(release_sha).lower() == 'f9cdf44ca79272ad7cead33dfb1d35fdf155f05f':
            raise GateError('PERMANENTLY_REJECTED_CARRIER: Base commit f9cdf44ca is permanently disqualified from production enrollment.')
        if proof_path is None:
            raise GateError('verified --prestage-proof is required')
        with open(proof_path, 'r', encoding='utf-8') as stream:
            proof_release = json.load(stream)['git_sha']
        target = transport.Target(compose, env, proof_path, release_sha or proof_release)
        verify(compose, env, timeout, manifest_path=manifest_path, release_sha=release_sha, expected_manifest_sha256=expected_manifest_sha256, target=target)
    except Exception as error:
        detail = str(error) if isinstance(error, GateError) else 'invalid drain observation'
        print('[deploy] ERROR: SAFE_DRAIN_REJECTED: ' + detail, file=sys.stderr)
        return 1
    if manifest_path:
        print(f'[deploy] drain observations clear with approved quarantine exemptions (Release SHA: {release_sha})')
    else:
        print('[deploy] drain observations clear; no exemptions applied')
    return 0


if __name__ == '__main__':
    sys.exit(main())
