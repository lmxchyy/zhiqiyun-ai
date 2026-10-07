#!/usr/bin/env python3
"""Canonical read-only DB snapshot for offline quarantine approval candidates.

Python >=3.6. A canonical snapshot binds core task/execution state, personal
financial effects, xz_assets artwork/results, and linked storage metadata. It
never claims remote object health or historical provider-attempt attribution
when those facts cannot be proven from the database. No DB write or signer.
"""
import datetime
import hashlib
import json
import os
import re
import sys
import types

# Execute protected SOURCE bytes, never executable .pyc cache or supplied helper.
sys.dont_write_bytecode = True
_approval_path = os.path.join(os.path.dirname(__file__), 'quarantine-approval.py')
_approval = types.ModuleType('quarantine_approval')
_approval.__file__ = _approval_path
with open(_approval_path, 'rb') as _source:
    exec(compile(_source.read(), _approval_path, 'exec'), _approval.__dict__)

CORE_VERSION = 'issue203-core-only-pg-text-sha256-v1'
FINANCIAL_VERSION = 'issue203-personal-financial-only-pg-text-sha256-v1'
PARTIAL_VERSION = 'issue203-core-personal-financial-only-v1'
MISSING_FAMILIES = ()
COMPLETE_SNAPSHOT_FAMILIES = ('core-task-execution', 'personal-financial-ledger',
                              'artwork-assets-provider-results-storage-metadata')
MAX_ROWS = 10000
# Go strings.TrimSpace (stringValue/upperTrim), not SQL's space-only trim.
GO_TRIM_SPACE = '\t\n\v\f\r \u0085\u00a0\u1680' + ''.join(chr(c) for c in range(0x2000, 0x200b)) + '\u2028\u2029\u202f\u205f\u3000'


class SnapshotError(Exception):
    pass


def block(code):
    raise SnapshotError('QUARANTINE_LIVE_' + code)


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':'),
                      ensure_ascii=True, allow_nan=False).encode('ascii')


# Exact schema114/119/120 + runtime task schema through123. Entries are
# [name, PostgreSQL udt, nullable, max length, numeric precision, scale].
# No discovered field may silently disappear; additions also require review.
SCHEMA = {
    'provider_execution_correlations': [
        ['base_url_host', 'text', 'NO', None, None, None],
        ['created_at', 'timestamptz', 'NO', None, None, None],
        ['endpoint_path', 'text', 'NO', None, None, None],
        ['error_code', 'text', 'NO', None, None, None],
        ['error_hash', 'text', 'NO', None, None, None],
        ['execution_id', 'int8', 'NO', None, 64, 0],
        ['http_status', 'int4', 'YES', None, 32, 0],
        ['id', 'int8', 'NO', None, 64, 0],
        ['job_role', 'text', 'NO', None, None, None],
        ['kind', 'text', 'NO', None, None, None],
        ['provider_code', 'text', 'NO', None, None, None],
        ['provider_job_id', 'text', 'YES', None, None, None],
        ['provider_state', 'text', 'NO', None, None, None],
    ],
    'provider_executions': [
        ['attempt', 'int4', 'NO', None, 32, 0],
        ['capability', 'text', 'NO', None, None, None],
        ['created_at', 'timestamptz', 'NO', None, None, None],
        ['error_class', 'text', 'YES', None, None, None],
        ['error_code', 'text', 'YES', None, None, None],
        ['failed_at', 'timestamptz', 'YES', None, None, None],
        ['id', 'int8', 'NO', None, 64, 0],
        ['last_checked_at', 'timestamptz', 'YES', None, None, None],
        ['last_error', 'text', 'YES', None, None, None],
        ['next_check_at', 'timestamptz', 'YES', None, None, None],
        ['processing_at', 'timestamptz', 'YES', None, None, None],
        ['provider', 'text', 'NO', None, None, None],
        ['provider_channel', 'text', 'NO', None, None, None],
        ['provider_model', 'text', 'NO', None, None, None],
        ['provider_operation_key', 'text', 'NO', None, None, None],
        ['provider_request_id', 'text', 'YES', None, None, None],
        ['request_fingerprint', 'bpchar', 'NO', 64, None, None],
        ['result_metadata', 'jsonb', 'YES', None, None, None],
        ['status', 'text', 'NO', None, None, None],
        ['submitted_at', 'timestamptz', 'YES', None, None, None],
        ['succeeded_at', 'timestamptz', 'YES', None, None, None],
        ['task_execution_generation', 'int8', 'YES', None, 64, 0],
        ['task_id', 'text', 'NO', None, None, None],
        ['unknown_at', 'timestamptz', 'YES', None, None, None],
        ['updated_at', 'timestamptz', 'NO', None, None, None],
    ],
    'xz_generation_tasks': [
        ['ai_label_status', 'text', 'YES', None, None, None],
        ['billing_account_id', 'text', 'YES', None, None, None],
        ['billing_account_type', 'text', 'NO', None, None, None],
        ['billing_rule_version_id', 'text', 'YES', None, None, None],
        ['billing_status', 'text', 'NO', None, None, None],
        ['billing_type', 'text', 'YES', None, None, None],
        ['captured_points', 'numeric', 'NO', None, 18, 6],
        ['client_request_id', 'text', 'YES', None, None, None],
        ['created_at', 'text', 'YES', None, None, None],
        ['error', 'jsonb', 'NO', None, None, None],
        ['estimated_margin', 'numeric', 'YES', None, 18, 6],
        ['execution_generation', 'int8', 'NO', None, 64, 0],
        ['id', 'text', 'NO', None, None, None],
        ['input_audit_status', 'text', 'YES', None, None, None],
        ['last_heartbeat_at', 'timestamptz', 'YES', None, None, None],
        ['lease_until', 'timestamptz', 'YES', None, None, None],
        ['model', 'text', 'YES', None, None, None],
        ['module_code', 'text', 'YES', None, None, None],
        ['organization_id', 'text', 'YES', None, None, None],
        ['output_audit_status', 'text', 'YES', None, None, None],
        ['params', 'jsonb', 'NO', None, None, None],
        ['point_cost', 'int8', 'NO', None, 64, 0],
        ['progress', 'int4', 'NO', None, 32, 0],
        ['prompt', 'text', 'YES', None, None, None],
        ['provider_channel', 'text', 'YES', None, None, None],
        ['quoted_points', 'numeric', 'NO', None, 18, 6],
        ['raw', 'jsonb', 'NO', None, None, None],
        ['refunded_points', 'numeric', 'NO', None, 18, 6],
        ['released_points', 'numeric', 'NO', None, 18, 6],
        ['reserved_points', 'numeric', 'NO', None, 18, 6],
        ['result_ids', 'jsonb', 'NO', None, None, None],
        ['status', 'text', 'YES', None, None, None],
        ['supplier_cost', 'numeric', 'YES', None, 18, 6],
        ['task_status', 'text', 'NO', None, None, None],
        ['tenant_id', 'text', 'YES', None, None, None],
        ['terminal', 'text', 'YES', None, None, None],
        ['type', 'text', 'YES', None, None, None],
        ['updated_at', 'text', 'YES', None, None, None],
        ['user_id', 'text', 'YES', None, None, None],
        ['worker_finished_at', 'text', 'YES', None, None, None],
        ['worker_id', 'text', 'YES', None, None, None],
    ],
}


# Migration021/030/048/088/103 exact financial columns through123.
FINANCIAL_SCHEMA = {
    'xz_billing_events': [
        ['agent_id', 'text', 'YES', None, None, None],
        ['amount_cents', 'int8', 'NO', None, 64, 0],
        ['balance_after', 'int8', 'NO', None, 64, 0],
        ['balance_before', 'int8', 'NO', None, 64, 0],
        ['id', 'text', 'NO', None, None, None],
        ['metadata', 'jsonb', 'NO', None, None, None],
        ['metric_code', 'text', 'YES', None, None, None],
        ['model', 'text', 'YES', None, None, None],
        ['module_code', 'text', 'YES', None, None, None],
        ['occurred_at', 'text', 'YES', None, None, None],
        ['operation_center_id', 'text', 'YES', None, None, None],
        ['point_cost', 'int8', 'NO', None, 64, 0],
        ['quantity', 'int8', 'NO', None, 64, 0],
        ['raw', 'jsonb', 'NO', None, None, None],
        ['status', 'text', 'YES', None, None, None],
        ['task_id', 'text', 'YES', None, None, None],
        ['tenant_id', 'text', 'YES', None, None, None],
        ['transaction_id', 'text', 'YES', None, None, None],
        ['unit_amount_cents', 'int8', 'NO', None, 64, 0],
        ['user_id', 'text', 'YES', None, None, None],
    ],
    'xz_billing_lifecycle_events': [
        ['billing_status', 'text', 'NO', None, None, None],
        ['created_at', 'timestamptz', 'NO', None, None, None],
        ['event_type', 'text', 'NO', None, None, None],
        ['id', 'text', 'NO', None, None, None],
        ['idempotency_key', 'text', 'NO', None, None, None],
        ['metadata', 'jsonb', 'NO', None, None, None],
        ['model_code', 'text', 'YES', None, None, None],
        ['points', 'numeric', 'NO', None, 18, 6],
        ['provider_channel', 'text', 'YES', None, None, None],
        ['rule_version_id', 'text', 'YES', None, None, None],
        ['task_id', 'text', 'NO', None, None, None],
        ['tenant_id', 'text', 'YES', None, None, None],
        ['user_id', 'text', 'YES', None, None, None],
    ],
    'xz_personal_point_lot_movements': [
        ['account_id', 'text', 'NO', None, None, None],
        ['available_after', 'int8', 'NO', None, 64, 0],
        ['available_before', 'int8', 'NO', None, 64, 0],
        ['consumed_after', 'int8', 'NO', None, 64, 0],
        ['consumed_before', 'int8', 'NO', None, 64, 0],
        ['created_at', 'timestamptz', 'NO', None, None, None],
        ['expired_after', 'int8', 'NO', None, 64, 0],
        ['expired_before', 'int8', 'NO', None, 64, 0],
        ['id', 'text', 'NO', None, None, None],
        ['idempotency_key', 'text', 'NO', None, None, None],
        ['lot_id', 'text', 'NO', None, None, None],
        ['metadata', 'jsonb', 'NO', None, None, None],
        ['movement_type', 'text', 'NO', None, None, None],
        ['points', 'int8', 'NO', None, 64, 0],
        ['reference_id', 'text', 'NO', None, None, None],
        ['reference_type', 'text', 'NO', None, None, None],
        ['reservation_id', 'text', 'YES', None, None, None],
        ['reserved_after', 'int8', 'NO', None, 64, 0],
        ['reserved_before', 'int8', 'NO', None, 64, 0],
        ['reversed_after', 'int8', 'NO', None, 64, 0],
        ['reversed_before', 'int8', 'NO', None, 64, 0],
        ['user_id', 'text', 'NO', None, None, None],
    ],
    'xz_personal_point_lots': [
        ['account_id', 'text', 'NO', None, None, None],
        ['available_points', 'int8', 'NO', None, 64, 0],
        ['consumed_points', 'int8', 'NO', None, 64, 0],
        ['created_at', 'timestamptz', 'NO', None, None, None],
        ['expired_points', 'int8', 'NO', None, 64, 0],
        ['expires_at', 'timestamptz', 'YES', None, None, None],
        ['granted_at', 'timestamptz', 'NO', None, None, None],
        ['id', 'text', 'NO', None, None, None],
        ['idempotency_key', 'text', 'NO', None, None, None],
        ['metadata', 'jsonb', 'NO', None, None, None],
        ['original_points', 'int8', 'NO', None, 64, 0],
        ['policy_snapshot', 'jsonb', 'NO', None, None, None],
        ['policy_version_id', 'text', 'YES', None, None, None],
        ['reference_id', 'text', 'NO', None, None, None],
        ['reference_type', 'text', 'NO', None, None, None],
        ['reserved_points', 'int8', 'NO', None, 64, 0],
        ['reversed_points', 'int8', 'NO', None, 64, 0],
        ['source_type', 'text', 'NO', None, None, None],
        ['status', 'text', 'NO', None, None, None],
        ['updated_at', 'timestamptz', 'NO', None, None, None],
        ['user_id', 'text', 'NO', None, None, None],
    ],
    'xz_personal_point_reservation_allocations': [
        ['account_id', 'text', 'NO', None, None, None],
        ['allocated_points', 'int8', 'NO', None, 64, 0],
        ['captured_points', 'int8', 'NO', None, 64, 0],
        ['created_at', 'timestamptz', 'NO', None, None, None],
        ['expired_points', 'int8', 'NO', None, 64, 0],
        ['id', 'text', 'NO', None, None, None],
        ['lot_id', 'text', 'NO', None, None, None],
        ['metadata', 'jsonb', 'NO', None, None, None],
        ['released_points', 'int8', 'NO', None, 64, 0],
        ['reservation_id', 'text', 'NO', None, None, None],
        ['reserved_points', 'int8', 'NO', None, 64, 0],
        ['status', 'text', 'NO', None, None, None],
        ['updated_at', 'timestamptz', 'NO', None, None, None],
        ['user_id', 'text', 'NO', None, None, None],
    ],
    'xz_personal_point_reservations': [
        ['account_id', 'text', 'NO', None, None, None],
        ['business_id', 'text', 'NO', None, None, None],
        ['business_type', 'text', 'NO', None, None, None],
        ['captured_points', 'int8', 'NO', None, 64, 0],
        ['created_at', 'timestamptz', 'NO', None, None, None],
        ['expired_points', 'int8', 'NO', None, 64, 0],
        ['id', 'text', 'NO', None, None, None],
        ['idempotency_key', 'text', 'NO', None, None, None],
        ['metadata', 'jsonb', 'NO', None, None, None],
        ['released_points', 'int8', 'NO', None, 64, 0],
        ['requested_points', 'int8', 'NO', None, 64, 0],
        ['reserved_points', 'int8', 'NO', None, 64, 0],
        ['status', 'text', 'NO', None, None, None],
        ['updated_at', 'timestamptz', 'NO', None, None, None],
        ['user_id', 'text', 'NO', None, None, None],
    ],
    'xz_point_accounts': [
        ['available', 'int8', 'NO', None, 64, 0],
        ['frozen', 'int8', 'NO', None, 64, 0],
        ['id', 'text', 'NO', None, None, None],
        ['raw', 'jsonb', 'NO', None, None, None],
        ['user_id', 'text', 'YES', None, None, None],
    ],
    'xz_tenant_wallets': [
        ['cash_balance_cents', 'int8', 'NO', None, 64, 0],
        ['created_at', 'timestamptz', 'NO', None, None, None],
        ['frozen_points', 'int8', 'NO', None, 64, 0],
        ['metadata', 'jsonb', 'NO', None, None, None],
        ['point_balance', 'int8', 'NO', None, 64, 0],
        ['status', 'text', 'NO', None, None, None],
        ['tenant_id', 'text', 'NO', None, None, None],
        ['total_bonus_units', 'int8', 'NO', None, 64, 0],
        ['total_recharge_units', 'int8', 'NO', None, 64, 0],
        ['updated_at', 'timestamptz', 'NO', None, None, None],
        ['version', 'int8', 'NO', None, 64, 0],
    ],
    'xz_user_wallets': [
        ['cash_balance_cents', 'int8', 'NO', None, 64, 0],
        ['frozen_token', 'int8', 'NO', None, 64, 0],
        ['raw', 'jsonb', 'NO', None, None, None],
        ['token_balance', 'int8', 'NO', None, 64, 0],
        ['total_token_granted', 'int8', 'NO', None, 64, 0],
        ['total_token_used', 'int8', 'NO', None, 64, 0],
        ['updated_at', 'timestamptz', 'NO', None, None, None],
        ['user_id', 'text', 'NO', None, None, None],
    ],
    'xz_wallet_ledger': [
        ['account_id', 'text', 'NO', None, None, None],
        ['available_after', 'numeric', 'NO', None, 18, 6],
        ['available_before', 'numeric', 'NO', None, 18, 6],
        ['billing_event_id', 'text', 'YES', None, None, None],
        ['created_at', 'timestamptz', 'NO', None, None, None],
        ['entry_type', 'text', 'NO', None, None, None],
        ['frozen_after', 'numeric', 'NO', None, 18, 6],
        ['frozen_before', 'numeric', 'NO', None, 18, 6],
        ['id', 'text', 'NO', None, None, None],
        ['idempotency_key', 'text', 'NO', None, None, None],
        ['metadata', 'jsonb', 'NO', None, None, None],
        ['points', 'numeric', 'NO', None, 18, 6],
        ['reference_id', 'text', 'NO', None, None, None],
        ['reference_type', 'text', 'NO', None, None, None],
        ['remark', 'text', 'NO', None, None, None],
        ['task_id', 'text', 'YES', None, None, None],
        ['tenant_id', 'text', 'YES', None, None, None],
        ['user_id', 'text', 'YES', None, None, None],
    ],
}


def _query(cursor, sql, parameters=(), limit=None):
    # Every fixed-source SELECT is client-size bounded, including discovery and
    # duplicate checks. Exact-cardinality callers request two rows so their own
    # predicate diagnoses ambiguity; history uses a sentinel, NEVER truncation.
    bound = MAX_ROWS + 1 if limit is None else limit
    try:
        cursor.execute('SELECT * FROM (' + sql + ') AS bounded_projection LIMIT ' + str(bound), parameters)
        rows = cursor.fetchall()
    except Exception:
        # Never return driver SQL, rows, prompts, URLs or credentials on failure.
        block('DB_QUERY_FAILED')
    if limit is None and len(rows) > MAX_ROWS:
        block('COUNT_LIMIT')
    return rows


def _one(cursor, sql, parameters=()):
    rows = _query(cursor, sql, parameters, limit=2)
    if len(rows) != 1:
        block('AMBIGUOUS_COUNT')
    return rows[0]


def transaction_clock(cursor):
    """Check caller-owned coherent transaction; no independent autocommit check."""
    try:
        if cursor.connection.get_transaction_status() != 2:  # PQTRANS_INTRANS
            block('TRANSACTION_REQUIRED')
    except SnapshotError:
        raise
    except Exception:
        block('DB_QUERY_FAILED')
    isolation, now = _one(cursor, "SELECT current_setting('transaction_isolation'), clock_timestamp()")
    if isolation not in ('repeatable read', 'serializable'):
        block('TRANSACTION_REQUIRED')
    if not isinstance(now, datetime.datetime) or now.tzinfo is None:
        block('DB_CLOCK_INVALID')
    return now.astimezone(datetime.timezone.utc)


def _schema(cursor, schema=SCHEMA):
    tables = sorted(schema)
    rows = _query(cursor, """
SELECT table_name,column_name,udt_name,is_nullable,character_maximum_length,
       numeric_precision,numeric_scale
FROM information_schema.columns
WHERE table_schema='public' AND table_name=ANY(%s)
ORDER BY table_name COLLATE "C",column_name COLLATE "C"
""", (tables,))
    expected = [(table,) + tuple(column) for table in tables for column in schema[table]]
    if [tuple(row) for row in rows] != expected:
        block('SCHEMA_MISMATCH')
    relations = _query(cursor, """
SELECT c.relname,c.relkind,c.relrowsecurity,c.relforcerowsecurity
FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
WHERE n.nspname='public' AND c.relname=ANY(%s) ORDER BY c.relname COLLATE "C"
""", (tables,))
    if [tuple(row) for row in relations] != [(table, 'r', False, False) for table in tables]:
        block('SCHEMA_MISMATCH')


def _settings(cursor):
    # Trusted fixed SQL only. Canonical text casts never depend on session locale
    # or timezone. Nullable values are SQL NULL, not empty string or JSON null.
    for sql in ("SET LOCAL search_path = pg_catalog, public", "SET LOCAL TIME ZONE 'UTC'", "SET LOCAL DateStyle = 'ISO, YMD'",
                "SET LOCAL IntervalStyle = 'iso_8601'", "SET LOCAL extra_float_digits = 3",
                "SET LOCAL bytea_output = 'hex'", "SET LOCAL statement_timeout = 10000"):
        try:
            cursor.execute(sql)
        except Exception:
            block('DB_QUERY_FAILED')


def _rows(cursor, table, predicate, parameters, schema=SCHEMA):
    # Identifiers/predicate come ONLY from this protected source, never Manifest.
    # Every field is hashed INSIDE PostgreSQL. Sensitive raw data is not sent to
    # Python/psql/logs. Representation: ordered [column,type,NULL|sha256(UTF8
    # PostgreSQL text cast)] triples, including persistent timestamps and JSONB.
    fields = ','.join("CASE WHEN \"%s\" IS NULL THEN NULL ELSE "
                      "encode(public.digest(convert_to(\"%s\"::text,'UTF8'),'sha256'),'hex') END"
                      % (column[0], column[0]) for column in schema[table])
    rows = _query(cursor, 'SELECT ' + fields + ' FROM public.' + table +
                  ' WHERE ' + predicate, parameters)
    if len(rows) > MAX_ROWS:
        block('COUNT_LIMIT')
    for row in rows:
        if len(row) != len(schema[table]):
            block('SCHEMA_MISMATCH')
        for column, value in zip(schema[table], row):
            if value is None:
                if column[2] == 'NO':
                    block('NULL_REQUIRED_FIELD')
            elif not isinstance(value, str) or not re.fullmatch('[0-9a-f]{64}', value):
                block('PROJECTION_INVALID')
    # Multiset order is bytewise canonical row order, independent of planner,
    # insertion order, collation and query row order. Duplicates are not removed.
    projected = [[[column[0], column[1], value] for column, value in zip(schema[table], row)]
                 for row in rows]
    return sorted(projected, key=canonical)


def _legacy_generation_evidence(cursor, item, execution_created_at):
    """Prove a Carrier-pinned NULL row predates the fencing migration."""
    if not _approval.legacy_identity_pinned(item):
        block('LEGACY_IDENTITY_NOT_PINNED')
    applied_at = _one(cursor, "SELECT applied_at FROM public.schema_migrations WHERE filename=%s",
                      ('119-execution-generation-fencing.sql',))[0]
    task_created_raw = _one(cursor, 'SELECT created_at FROM public.xz_generation_tasks WHERE id=%s',
                            (item['task_id'],))[0]
    if (not isinstance(applied_at, datetime.datetime) or applied_at.tzinfo is None or
            not isinstance(execution_created_at, datetime.datetime) or execution_created_at.tzinfo is None or
            not isinstance(task_created_raw, str)):
        block('LEGACY_MIGRATION_PROOF_INVALID')
    match = re.fullmatch(r'(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d)(?:\.(\d{1,9}))?Z', task_created_raw)
    if match is None:
        block('LEGACY_MIGRATION_PROOF_INVALID')
    try:
        task_created = datetime.datetime.strptime(match.group(1), '%Y-%m-%dT%H:%M:%S').replace(
            tzinfo=datetime.timezone.utc) + datetime.timedelta(
                microseconds=int((match.group(2) or '').ljust(6, '0')[:6] or '0'))
    except ValueError:
        block('LEGACY_MIGRATION_PROOF_INVALID')
    applied_at = applied_at.astimezone(datetime.timezone.utc)
    execution_created_at = execution_created_at.astimezone(datetime.timezone.utc)
    # One-second margin refuses an ambiguous nanosecond/microsecond boundary.
    if not (execution_created_at < applied_at and
            task_created < applied_at - datetime.timedelta(seconds=1)):
        block('LEGACY_MIGRATION_PROOF_INVALID')
    fmt = '%Y-%m-%dT%H:%M:%S.%fZ'
    return {'identity_sha256': _approval.legacy_identity_sha256(
                item['execution_id'], item['task_id'], item['attempt']),
            'migration119_applied_at': applied_at.strftime(fmt),
            'execution_created_at': execution_created_at.strftime(fmt),
            'task_created_at': task_created.strftime(fmt)}


def _entries(entries):
    if not isinstance(entries, list) or not 0 < len(entries) <= 1000:
        block('APPROVED_COUNT_INVALID')
    ids, attempts = set(), set()
    for item in entries:
        if not isinstance(item, dict):
            block('IDENTITY_INVALID')
        for key in ('execution_id', 'attempt', 'task_generation'):
            if type(item.get(key)) is not int or not 0 < item[key] <= 9223372036854775807:
                block('IDENTITY_INVALID')
        task_id = item.get('task_id')
        if (not isinstance(task_id, str) or not 0 < len(task_id.encode('utf-8')) <= 256 or
                'generation' not in item):
            block('IDENTITY_INVALID')
        if item['generation'] is None:
            if not _approval.legacy_identity_pinned(item):
                block('LEGACY_IDENTITY_NOT_PINNED')
        elif type(item['generation']) is not int or not 0 < item['generation'] <= 9223372036854775807:
            block('IDENTITY_INVALID')
        if re.fullmatch(r'[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}', task_id):
            block('UNSUPPORTED_LINKAGE')
        pair = (task_id, item['attempt'])
        if item['execution_id'] in ids or pair in attempts:
            block('DUPLICATE_APPROVED_IDENTITY')
        ids.add(item['execution_id'])
        attempts.add(pair)


def project_core_in_transaction(cursor, entries):
    """Return CORE_ONLY typed hashes from the caller's SAME coherent transaction.

    Does not start/commit a transaction, lock absence rows, infer terminal provider
    state or claim safe activation. Caller must provide repeatable-read/serializable.
    Future registration must call this API within its agreed locking transaction,
    not validate in Python then start an independent SQL registration transaction.
    """
    _entries(entries)
    transaction_clock(cursor)  # observation time deliberately EXCLUDED from hash
    _settings(cursor)
    _schema(cursor)
    snapshots = {}
    for item in sorted(entries, key=lambda entry: entry['execution_id']):
        tid = item['task_id']
        task = _query(cursor, """
SELECT execution_generation,status,task_status,user_id,tenant_id,
       jsonb_typeof(result_ids),type,model,
       (jsonb_typeof(params)='object'
        AND (NOT (params ? 'billing_scope') OR jsonb_typeof(params->'billing_scope')='string')
        AND (
          (upper(btrim(billing_account_type,%s)) IN ('','PERSONAL')
           AND upper(btrim(coalesce(params->>'billing_scope',''),%s)) IN ('','PERSONAL')
           AND (tenant_id IS NULL OR tenant_id IN ('','tenant_default')))
          OR
          (upper(btrim(billing_account_type,%s)) = 'ENTERPRISE'
           AND upper(btrim(coalesce(params->>'billing_scope',''),%s)) = 'ENTERPRISE'
           AND tenant_id IS NOT NULL AND tenant_id LIKE 'tenant_%%' AND tenant_id NOT IN ('','tenant_default')
           AND (NOT (params ? 'tenant_id') OR params->>'tenant_id'=tenant_id)
           AND (NOT (raw ? 'tenantId') OR raw->>'tenantId'=tenant_id))
        ))
FROM public.xz_generation_tasks WHERE id=%s
""", (GO_TRIM_SPACE, GO_TRIM_SPACE, GO_TRIM_SPACE, GO_TRIM_SPACE, tid), limit=2)
        if len(task) != 1:
            block('TASK_COUNT_MISMATCH')
        gen, status, task_status, user, tenant, result_type, task_type, model, valid_scope = task[0]
        if gen != item['task_generation']:
            block('TASK_GENERATION_MISMATCH')
        # Return only the linkage predicate, never raw params. Runtime recognizes
        # enterprise in either location and rejects unknown/conflicting scopes;
        # malformed JSON types are conservatively unsupported here.
        if (valid_scope is not True or
                _one(cursor, 'SELECT count(*) FROM public.generation_tasks WHERE id::text=%s', (tid,))[0]):
            block('UNSUPPORTED_LINKAGE')
        if (not user or not task_type or not model or
                status not in ('CREATED', 'PENDING', 'QUEUED', 'RUNNING', 'PROCESSING', 'COMPLETED', 'SUCCEEDED', 'FAILED', 'CANCELLED', 'MANUAL_REVIEW') or
                task_status not in ('CREATED', 'QUEUED', 'RUNNING', 'SUCCEEDED', 'FAILED', 'CANCELLED', 'MANUAL_REVIEW') or
                result_type != 'array'):
            block('NULL_OR_UNKNOWN_TASK_STATE')
        # Unknown provider status remains unknown; no terminality/safety inference.
        executions = _query(cursor, """
SELECT id,task_id,attempt,task_execution_generation,status,provider,provider_channel,
       provider_model,capability,request_fingerprint,created_at
FROM public.provider_executions WHERE task_id=%s ORDER BY id
""", (tid,))
        if not 0 < len(executions) <= MAX_ROWS:
            block('EXECUTION_COUNT_MISMATCH')
        seen_ids, seen_attempts = set(), set()
        selected = []
        for execution in executions:
            eid, linked_tid, attempt, generation, state, provider, channel, provider_model, capability, fingerprint, created_at = execution
            if (type(eid) is not int or eid <= 0 or linked_tid != tid or type(attempt) is not int or
                    attempt <= 0 or (generation is None and
                                     (eid != item['execution_id'] or item['generation'] is not None)) or
                    (generation is not None and (type(generation) is not int or generation <= 0)) or
                    state not in ('prepared', 'submitting', 'submitted', 'processing', 'succeeded', 'failed', 'unknown') or
                    not provider or channel is None or not isinstance(channel, str) or not provider_model or not capability or
                    not isinstance(fingerprint, str) or not re.fullmatch('[0-9a-f]{64}', fingerprint)):
                block('NULL_OR_UNKNOWN_EXECUTION_STATE')
            if eid in seen_ids or attempt in seen_attempts:
                block('DUPLICATE_EXECUTION_IDENTITY')
            seen_ids.add(eid)
            seen_attempts.add(attempt)
            if eid == item['execution_id']:
                selected.append(execution)
        if len(selected) != 1:
            block('EXECUTION_IDENTITY_MISMATCH')
        if selected[0][2] != item['attempt']:
            block('ATTEMPT_MISMATCH')
        if selected[0][3] != item['generation']:
            block('EXECUTION_GENERATION_MISMATCH')
        if item['generation'] is None and (len(executions) != 1 or item['attempt'] != 1 or
                                           selected[0][4] not in ('unknown', 'submitted')):
            block('LEGACY_IDENTITY_AMBIGUOUS')
        legacy_evidence = (_legacy_generation_evidence(cursor, item, selected[0][10])
                           if item['generation'] is None else None)
        # Detect duplicate ID anywhere, including another task.
        counts = _query(cursor, 'SELECT id,count(*) FROM public.provider_executions WHERE id=ANY(%s) GROUP BY id',
                        (sorted(seen_ids),))
        if len(counts) != len(seen_ids) or any(row[1] != 1 for row in counts):
            block('DUPLICATE_EXECUTION_IDENTITY')
        correlations = _query(cursor, """
SELECT id,execution_id,kind FROM public.provider_execution_correlations
WHERE execution_id=ANY(%s)
""", (sorted(seen_ids),))
        if len(correlations) > MAX_ROWS:
            block('COUNT_LIMIT')
        correlation_ids = [row[0] for row in correlations]
        if (len(set(correlation_ids)) != len(correlation_ids) or
                any(type(row[0]) is not int or row[0] <= 0 or row[1] not in seen_ids or not row[2]
                    for row in correlations)):
            block('CORRELATION_IDENTITY_INVALID')
        if correlation_ids:
            counts = _query(cursor, 'SELECT id,count(*) FROM public.provider_execution_correlations WHERE id=ANY(%s) GROUP BY id',
                            (sorted(correlation_ids),))
            if len(counts) != len(correlation_ids) or any(row[1] != 1 for row in counts):
                block('CORRELATION_IDENTITY_INVALID')
        families = {
            'task': _rows(cursor, 'xz_generation_tasks', 'id=%s', (tid,)),
            'attempts': _rows(cursor, 'provider_executions', 'task_id=%s', (tid,)),
            'correlations': _rows(cursor, 'provider_execution_correlations', 'execution_id=ANY(%s)', (sorted(seen_ids),)),
        }
        if (len(families['task']) != 1 or len(families['attempts']) != len(executions) or
                len(families['correlations']) != len(correlations)):
            block('AMBIGUOUS_COUNT')
        snapshots[item['execution_id']] = {
            'version': CORE_VERSION, 'scope': 'CORE_ONLY', 'schema': SCHEMA,
            'identity': {key: item[key] for key in ('execution_id', 'task_id', 'attempt', 'generation', 'task_generation')},
            'counts': {key: len(value) for key, value in families.items()},
            'families': families,
        }
        if legacy_evidence is not None:
            snapshots[item['execution_id']]['legacy_generation_evidence'] = legacy_evidence
    return snapshots


def core_sha256(snapshot):
    """Internal CORE_ONLY digest; never substitute for signed snapshot_sha256."""
    if not isinstance(snapshot, dict) or snapshot.get('version') != CORE_VERSION or snapshot.get('scope') != 'CORE_ONLY':
        block('CORE_SCOPE_INVALID')
    return hashlib.sha256(canonical(snapshot)).hexdigest()


def compare_core_in_transaction(cursor, entries, expected_core_sha256):
    """Internal comparison seam, NOT authenticated final approval acceptance."""
    snapshots = project_core_in_transaction(cursor, entries)
    if not isinstance(expected_core_sha256, dict) or set(expected_core_sha256) != set(snapshots):
        block('APPROVED_COUNT_INVALID')
    for eid, snapshot in snapshots.items():
        expected = expected_core_sha256[eid]
        if (not isinstance(expected, str) or not re.fullmatch('[0-9a-f]{64}', expected) or
                core_sha256(snapshot) != expected):
            block('CORE_HASH_MISMATCH')
    return snapshots


def sample_core_read_only(connection, entries):
    """Own a fresh explicit READ ONLY transaction; never reuse active work.

    DB-API connection must be idle with autocommit enabled. psycopg2's status is
    used to reject an existing transaction (0 is PQTRANS_IDLE). No mutations.
    """
    try:
        if connection.closed:
            block('DB_QUERY_FAILED')
        if connection.autocommit is not True or connection.get_transaction_status() != 0:
            block('TRANSACTION_REQUIRED')
        cursor = connection.cursor()
        cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY')
    except SnapshotError:
        raise
    except Exception:
        block('DB_QUERY_FAILED')
    try:
        return project_core_in_transaction(cursor, entries)
    finally:
        try:
            cursor.execute('ROLLBACK')
            cursor.close()
        except Exception:
            block('DB_QUERY_FAILED')


def _financial_family(cursor, table, predicate, parameters, account_id, user_id,
                      state_sql, unique_keys):
    """Fixed-source identifiers only; return hashes, never financial metadata."""
    rows = _query(cursor, 'SELECT id,account_id,user_id,(' + state_sql +
                  ') FROM public.' + table + ' WHERE ' + predicate, parameters)
    if len(rows) > MAX_ROWS:
        block('COUNT_LIMIT')
    ids = [row[0] for row in rows]
    if len(set(ids)) != len(ids) or any(not row[0] for row in rows):
        block('FINANCIAL_DUPLICATE_IDENTITY')
    if any(row[1:3] != (account_id, user_id) for row in rows):
        block('FINANCIAL_OWNER_MISMATCH')
    if any(row[3] is not True for row in rows):
        block('FINANCIAL_STATE_INVALID')
    if ids:
        counts = _query(cursor, 'SELECT id,count(*) FROM public.' + table +
                        ' WHERE id=ANY(%s) GROUP BY id', (ids,))
        if len(counts) != len(ids) or any(row[1] != 1 for row in counts):
            block('FINANCIAL_DUPLICATE_IDENTITY')
        for keys in unique_keys:
            duplicates = _query(cursor, 'SELECT count(*) FROM public.' + table +
                                ' WHERE (' + keys + ') IN (SELECT ' + keys + ' FROM public.' + table +
                                ' WHERE id=ANY(%s)) GROUP BY ' + keys + ' HAVING count(*)<>1', (ids,))
            if duplicates:
                block('FINANCIAL_DUPLICATE_KEY')
    projected = _rows(cursor, table, predicate, parameters, FINANCIAL_SCHEMA)
    if len(projected) != len(rows):
        block('AMBIGUOUS_COUNT')
    return projected, ids


def project_core_financial_in_transaction(cursor, entries):
    """Partial CORE + FINANCIAL family on the SAME caller cursor.

    Repeatable-read/serializable observation, not absence locking or registration
    closure. Account-wide lots/movements/wallet history conservatively include
    shared balances and ALL historical entries, not merely each latest event.
    Legacy wallet-only requires an actual exact RESERVE proof; absence of personal
    rows alone is NEVER proof of no effects.
    """
    cores = project_core_in_transaction(cursor, entries)
    _schema(cursor, FINANCIAL_SCHEMA)
    snapshots = {}
    for item in sorted(entries, key=lambda value: value['execution_id']):
        tid = item['task_id']
        user, tenant, acct_type, scope, engine, marker_account, marker_reservation, task_valid = _one(cursor, """
SELECT user_id,coalesce(tenant_id,''),
       upper(btrim(billing_account_type,%s)),
       upper(btrim(coalesce(params->>'billing_scope',''),%s)),
       coalesce(raw->>'billingEngine',''),
       coalesce(raw->>'personalPointAccountId',''),coalesce(raw->>'personalPointReservationId',''),
       (jsonb_typeof(raw)='object'
        AND NOT EXISTS (SELECT 1 FROM jsonb_each(raw) f
          WHERE f.key IN ('billingEngine','personalPointAccountId','personalPointReservationId')
            AND jsonb_typeof(f.value)<>'string')
        AND (NOT (raw ? 'id') OR raw->>'id'=id)
        AND (NOT (raw ? 'userId') OR raw->>'userId'=user_id)
        AND billing_status IN ('UNQUOTED','QUOTED','RESERVED','CAPTURED','RELEASED','REFUNDED','BILLING_FAILED')
        AND point_cost>0 AND point_cost<=9007199254740991 AND reserved_points>0)
FROM public.xz_generation_tasks WHERE id=%s
""", (GO_TRIM_SPACE, GO_TRIM_SPACE, tid))
        if task_valid is not True:
            block('FINANCIAL_TASK_LINKAGE_INVALID')
        is_personal = acct_type in ('', 'PERSONAL') and scope in ('', 'PERSONAL') and tenant in ('', 'tenant_default')
        is_enterprise = acct_type == 'ENTERPRISE' and scope == 'ENTERPRISE' and tenant.startswith('tenant_') and tenant != 'tenant_default'
        if not is_personal and not is_enterprise:
            block('FINANCIAL_TASK_LINKAGE_INVALID')

        if is_personal:
            modern = engine == 'PERSONAL_LOT_V1' and bool(marker_account) and bool(marker_reservation)
            if not modern and (engine or marker_account or marker_reservation):
                block('FINANCIAL_MARKER_INVALID')
            accounts = _query(cursor, 'SELECT id,user_id,available,frozen FROM public.xz_point_accounts WHERE user_id=%s OR id=%s',
                              (user, marker_account), limit=2)
            if len(accounts) != 1 or not accounts[0][0]:
                block('FINANCIAL_ACCOUNT_COUNT')
            aid, owner, available, frozen = accounts[0]
            if owner != user or (modern and aid != marker_account):
                block('FINANCIAL_OWNER_MISMATCH')
            if available is None or frozen is None or available < 0 or frozen < 0:
                block('FINANCIAL_STATE_INVALID')
            linkage = _one(cursor, """
SELECT (coalesce(billing_account_id,'') IN ('',%s)
        AND (NOT (params ? 'billing_account_id') OR
             (jsonb_typeof(params->'billing_account_id')='string' AND params->>'billing_account_id' IN ('',%s)))
        AND (NOT (raw ? 'billingAccountId') OR
             (jsonb_typeof(raw->'billingAccountId')='string' AND raw->>'billingAccountId' IN ('',%s))))
FROM public.xz_generation_tasks WHERE id=%s
""", (user, user, user, tid))[0]
            if linkage is not True:
                block('FINANCIAL_ACCOUNT_LINKAGE_INVALID')
            wallets = _query(cursor, 'SELECT token_balance,frozen_token FROM public.xz_user_wallets WHERE user_id=%s', (user,), limit=2)
            if len(wallets) != 1:
                block('FINANCIAL_WALLET_COUNT')
            if wallets[0] != (available, frozen):
                block('FINANCIAL_BALANCE_CONFLICT')
        else:
            modern = False
            if engine or marker_account or marker_reservation:
                block('FINANCIAL_MARKER_INVALID')
            aid = tenant
            wallets = _query(cursor, 'SELECT tenant_id,point_balance,frozen_points,status FROM public.xz_tenant_wallets WHERE tenant_id=%s',
                             (tenant,), limit=2)
            if len(wallets) != 1 or wallets[0][3] != 'ACTIVE':
                block('FINANCIAL_ACCOUNT_COUNT')
            available, frozen = wallets[0][1], wallets[0][2]
            if available is None or frozen is None or available < 0 or frozen < 0:
                block('FINANCIAL_STATE_INVALID')
            linkage = _one(cursor, """
SELECT (coalesce(billing_account_id,'')=%s
        AND (NOT (params ? 'billing_account_id') OR
             (jsonb_typeof(params->'billing_account_id')='string' AND params->>'billing_account_id'=%s))
        AND (NOT (raw ? 'billingAccountId') OR
             (jsonb_typeof(raw->'billingAccountId')='string' AND raw->>'billingAccountId'=%s)))
FROM public.xz_generation_tasks WHERE id=%s
""", (tenant, tenant, tenant, tid))[0]
            if linkage is not True:
                block('FINANCIAL_ACCOUNT_LINKAGE_INVALID')

        reserve_key = 'generation:reserve:' + tid
        reservations = _query(cursor, """
SELECT id,account_id,user_id,business_type,business_id,idempotency_key,
       requested_points,reserved_points,captured_points,released_points,expired_points
FROM public.xz_personal_point_reservations
WHERE id=%s OR business_id=%s OR idempotency_key=%s
""", (marker_reservation if is_personal else '', tid, reserve_key), limit=2)
        if len(reservations) != (1 if modern else 0):
            block('FINANCIAL_RESERVATION_COUNT')
        rid = marker_reservation if modern else ''
        if modern and reservations[0][:6] != (rid, aid, user, 'GENERATION_TASK', tid, reserve_key):
            block('FINANCIAL_RESERVATION_LINKAGE_INVALID')
        # All supported runtime spellings. Imported legacy wallet keys are
        # checked separately against their DB-side legacy_ledger_id metadata.
        keys = [tid + ':' + kind for kind in ('RESERVE','CAPTURE','RELEASE','REFUND')]
        for kind, command in [('reserve','reserve'),('capture','capture'),('release','release'),('release','durable-release')]:
            keys.append('personal-point:' + kind + ':' + aid + ':generation:' + command + ':' + tid)
        linked = _query(cursor, """
SELECT id,account_id,user_id,task_id,reference_type,reference_id,entry_type,
       idempotency_key,points,
       (jsonb_typeof(metadata)='object'
        AND (NOT (metadata ? 'legacy_ledger_id') OR
             (jsonb_typeof(metadata->'legacy_ledger_id')='string' AND btrim(metadata->>'legacy_ledger_id',%s)<>''))
        AND idempotency_key IN (
          'personal-point:legacy-wallet:' || %s || ':' || %s || ':' || entry_type ||
             CASE WHEN NOT (metadata ? 'legacy_ledger_id') THEN '' ELSE ':' || btrim(metadata->>'legacy_ledger_id',%s) END,
          'personal-point:legacy-wallet:' || %s || ':' || btrim(metadata->>'legacy_ledger_id',%s) || ':' || btrim(metadata->>'legacy_ledger_id',%s))),
       (metadata ? 'legacy_ledger_id'),
       (CASE WHEN %s LIKE 'tenant_%%' THEN tenant_id = %s ELSE (tenant_id IS NULL OR tenant_id IN ('','tenant_default')) END)
FROM public.xz_wallet_ledger
WHERE task_id=%s OR reference_id=%s OR idempotency_key=ANY(%s)
ORDER BY id COLLATE "C"
""", (GO_TRIM_SPACE, aid, tid, GO_TRIM_SPACE, aid, GO_TRIM_SPACE, GO_TRIM_SPACE, aid, aid, tid, tid, keys))
        if not 0 < len(linked) <= MAX_ROWS:
            block('FINANCIAL_LEDGER_COUNT')
        reserves = []
        for row in linked:
            lid, la, lu, lt, rt, ref, kind, key, points, imported, import_metadata, tenant_ok = row
            if (la != aid or lu != user or lt != tid or rt != 'GENERATION_TASK' or ref != tid or tenant_ok is not True):
                block('FINANCIAL_LEDGER_LINKAGE_INVALID')
            if kind not in ('RESERVE','CAPTURE','RELEASE','REFUND'):
                block('FINANCIAL_LEDGER_STATE_INVALID')
            allowed = [tid + ':' + kind]
            if modern and kind in ('RESERVE','CAPTURE','RELEASE'):
                allowed.append('personal-point:' + kind.lower() + ':' + aid + ':generation:' + kind.lower() + ':' + tid)
                if kind == 'RELEASE':
                    allowed.append('personal-point:release:' + aid + ':generation:durable-release:' + tid)
            if (key not in allowed or import_metadata) and imported is not True:
                block('FINANCIAL_LEDGER_KEY_INVALID')
            if points is None or points <= 0 or points != int(points):
                block('FINANCIAL_STATE_INVALID')
            if kind == 'RESERVE':
                reserves.append(row)
        if len(reserves) != 1:
            block('FINANCIAL_RESERVE_COUNT')
        amounts = {kind: sum(row[8] for row in linked if row[6] == kind)
                   for kind in ('RESERVE','CAPTURE','RELEASE','REFUND')}
        if amounts['CAPTURE'] + amounts['RELEASE'] > amounts['RESERVE']:
            block('FINANCIAL_SETTLEMENT_CONFLICT')
        if modern and (reservations[0][6] != amounts['RESERVE'] or
                       reservations[0][8] != amounts['CAPTURE'] or reservations[0][9] != amounts['RELEASE']):
            block('FINANCIAL_RESERVE_CONFLICT')
        if is_enterprise and (amounts['RELEASE'] != amounts['RESERVE'] or amounts['CAPTURE'] != 0 or amounts['REFUND'] != 0):
            block('FINANCIAL_SETTLEMENT_CONFLICT')

        # Active reservations require generationTaskExactReservationPointCost's
        # legacy params even after modern attribution. Synchronous modern capture
        # (and settled historical wallet history) has no such writer requirement.
        # If optional legacy evidence exists it must still agree, never conflict.
        reserve_valid = _one(cursor, """
SELECT (point_cost=%s AND reserved_points=point_cost
        AND (NOT (params ? 'billingReserved') OR params->'billingReserved'='true'::jsonb)
        AND (NOT (params ? 'billingReservationPointCost') OR
             (jsonb_typeof(params->'billingReservationPointCost')='number' AND params->'billingReservationPointCost'=to_jsonb(point_cost)))
        AND CASE WHEN billing_status='RESERVED' THEN
          params->'billingReserved'='true'::jsonb AND params->'billingReservationPointCost'=to_jsonb(point_cost)
          AND captured_points=0 AND released_points=0 AND refunded_points=0
          AND (NOT (params ? 'billingRefunded') OR params->'billingRefunded'='false'::jsonb)
        ELSE billing_status IN ('CAPTURED','RELEASED','REFUNDED')
          AND captured_points=%s AND released_points=%s AND refunded_points=%s
          AND captured_points+released_points=reserved_points
          AND CASE billing_status WHEN 'CAPTURED' THEN captured_points=point_cost AND refunded_points=0
              WHEN 'RELEASED' THEN released_points=point_cost AND refunded_points=0
              ELSE refunded_points>0 AND refunded_points<=captured_points END END)
FROM public.xz_generation_tasks WHERE id=%s
""", (reserves[0][8], amounts['CAPTURE'], amounts['RELEASE'], amounts['REFUND'], tid))[0]
        if reserve_valid is not True:
            block('FINANCIAL_RESERVE_CONFLICT')
        if is_personal:
            families = {
                'account': _rows(cursor, 'xz_point_accounts', 'id=%s', (aid,), FINANCIAL_SCHEMA),
                'wallet': _rows(cursor, 'xz_user_wallets', 'user_id=%s', (user,), FINANCIAL_SCHEMA),
            }
            if len(families['account']) != 1 or len(families['wallet']) != 1:
                block('FINANCIAL_ACCOUNT_COUNT')
            owner_predicate, owner_parameters = 'account_id=%s OR user_id=%s', (aid, user)
            common = "id<>'' AND idempotency_key<>''"
            reservation_states = ("reserved_points>=0 AND captured_points>=0 AND released_points>=0 AND expired_points>=0 AND "
                "((status='RESERVED' AND reserved_points>0) OR (status='PARTIAL' AND (reserved_points>0 OR captured_points>0)) "
                "OR (status='CAPTURED' AND captured_points>0) OR (status='RELEASED' AND released_points>0) "
                "OR (status='EXPIRED' AND expired_points>0) OR (status='CANCELLED' AND released_points+expired_points>0))")
            families['reservations'], rids = _financial_family(cursor, 'xz_personal_point_reservations',
                owner_predicate + ' OR id=%s OR business_id=%s OR idempotency_key=%s', (aid,user,rid,tid,reserve_key), aid,user,
                common + ' AND ' + reservation_states + ' AND requested_points>0 AND requested_points=reserved_points+captured_points+released_points+expired_points',
                ['account_id,idempotency_key','account_id,business_type,business_id'])
            families['lots'], lots = _financial_family(cursor, 'xz_personal_point_lots', owner_predicate, owner_parameters, aid,user,
                common + " AND status IN ('ACTIVE','EXHAUSTED','EXPIRED','REVERSED','LEGACY') AND source_type IN "
                "('REGISTRATION_GIFT','ACTIVITY_GIFT','ADMIN_GIFT','RECHARGE','MEMBERSHIP_GRANT','MEMBER_PACKAGE_GRANT','AGENT_GRANT','AGENT_JOIN_GRANT','OPERATION_CENTER_GRANT','ORDER_GRANT','COMMERCE_ORDER','UNIFIED_PAYMENT_GRANT','WECHAT_VIRTUAL_ORDER','WECHAT_VIRTUAL_COUPON','COUPON_GRANT','REFUND','RELEASE','ADJUSTMENT','ADMIN_CORRECTION','CORRECTION','LEGACY','SYSTEM_DEFAULT','REVERSAL','MANUAL')"
                ' AND original_points>0 AND available_points>=0 AND reserved_points>=0 AND consumed_points>=0 AND expired_points>=0 AND reversed_points>=0 '
                'AND original_points=available_points+reserved_points+consumed_points+expired_points+reversed_points '
                "AND ((source_type='LEGACY' AND status='LEGACY' AND expires_at IS NULL AND policy_version_id IS NULL) OR (source_type<>'LEGACY' AND status<>'LEGACY')) "
                'AND (expires_at IS NULL OR expires_at>granted_at)',
                ['account_id,idempotency_key'])
            lot_balances = _one(cursor, 'SELECT coalesce(sum(available_points),0),coalesce(sum(reserved_points),0) '
                                'FROM public.xz_personal_point_lots WHERE account_id=%s AND user_id=%s', (aid,user))
            if lot_balances != (available, frozen):
                block('FINANCIAL_LOT_BALANCE_CONFLICT')
            allocation_pred = owner_predicate + ' OR reservation_id=ANY(%s) OR lot_id=ANY(%s)'
            allocation_params = (aid,user,rids,lots)
            families['allocations'], allocations = _financial_family(cursor, 'xz_personal_point_reservation_allocations',
                allocation_pred, allocation_params, aid,user,
                "id<>'' AND status<>'CANCELLED' AND allocated_points>0 AND " + reservation_states +
                ' AND allocated_points=reserved_points+captured_points+released_points+expired_points', ['reservation_id,lot_id'])
            allocation_links = _query(cursor, 'SELECT reservation_id,lot_id FROM public.xz_personal_point_reservation_allocations WHERE ' + allocation_pred, allocation_params)
            if any(row[0] not in rids or row[1] not in lots for row in allocation_links):
                block('FINANCIAL_ALLOCATION_LINKAGE_INVALID')
            if modern:
                totals = _one(cursor, """
SELECT count(*),sum(allocated_points),sum(reserved_points),sum(captured_points),sum(released_points),sum(expired_points)
FROM public.xz_personal_point_reservation_allocations WHERE reservation_id=%s
""", (rid,))
                if totals[0] == 0 or tuple(totals[1:]) != tuple(reservations[0][6:]):
                    block('FINANCIAL_ALLOCATION_COUNT_OR_TOTAL')
            families['movements'], movements = _financial_family(cursor, 'xz_personal_point_lot_movements',
                owner_predicate + ' OR reservation_id=ANY(%s) OR lot_id=ANY(%s)', (aid,user,rids,lots), aid,user,
                common + " AND movement_type IN ('OPENING','GRANT','RESERVE','CAPTURE','RELEASE','EXPIRE','ADJUSTMENT','REVERSE') AND points>0 "
                'AND least(available_before,available_after,reserved_before,reserved_after,consumed_before,consumed_after,expired_before,expired_after,reversed_before,reversed_after)>=0 '
                "AND CASE WHEN movement_type='OPENING' THEN available_before=0 AND reserved_before=0 AND consumed_before=0 AND expired_before=0 AND reversed_before=0 "
                'AND points=available_after+reserved_after+consumed_after+expired_after+reversed_after ELSE '
                "(CASE movement_type WHEN 'ADJUSTMENT' THEN available_after IN (available_before-points,available_before+points) "
                "WHEN 'GRANT' THEN available_after=available_before+points WHEN 'RELEASE' THEN available_after=available_before+points "
                "WHEN 'CAPTURE' THEN available_after=available_before ELSE available_after=available_before-points END) "
                "AND reserved_after=reserved_before+CASE movement_type WHEN 'RESERVE' THEN points WHEN 'CAPTURE' THEN -points WHEN 'RELEASE' THEN -points ELSE 0 END "
                "AND consumed_after=consumed_before+CASE WHEN movement_type='CAPTURE' THEN points ELSE 0 END "
                "AND expired_after=expired_before+CASE WHEN movement_type='EXPIRE' THEN points ELSE 0 END "
                "AND reversed_after=reversed_before+CASE WHEN movement_type='REVERSE' THEN points ELSE 0 END END", ['lot_id,idempotency_key'])
            movement_links = _query(cursor, "SELECT lot_id,reservation_id FROM public.xz_personal_point_lot_movements WHERE account_id=%s OR user_id=%s OR reservation_id=ANY(%s) OR lot_id=ANY(%s)", (aid,user,rids,lots))
            if any(row[0] not in lots or (row[1] is not None and row[1] not in rids) for row in movement_links):
                block('FINANCIAL_MOVEMENT_LINKAGE_INVALID')
            task_movements = _query(cursor, """
SELECT movement_type,idempotency_key,points FROM public.xz_personal_point_lot_movements
WHERE reservation_id=%s
""", (rid,)) if modern else []
            task_lots = [row[1] for row in _query(cursor,
                'SELECT reservation_id,lot_id FROM public.xz_personal_point_reservation_allocations WHERE reservation_id=%s', (rid,))]
            reserve_movements = []
            movement_amounts = {'CAPTURE': 0, 'RELEASE': 0}
            for kind, key, points in task_movements:
                commands = {'RESERVE': ['reserve'], 'CAPTURE': ['capture'], 'RELEASE': ['release','durable-release']}
                if kind in commands:
                    allowed_keys = [kind.lower() + ':generation:' + command + ':' + tid + ':' + lot
                                    for command in commands[kind] for lot in task_lots]
                    if key not in allowed_keys:
                        block('FINANCIAL_MOVEMENT_KEY_INVALID')
                    if kind == 'RESERVE':
                        reserve_movements.append((key,points))
                    else:
                        movement_amounts[kind] += points
                elif kind == 'EXPIRE':
                    allowed_keys = ['expire:' + lot + ':' + rid + ':generation:' + command + ':' + tid
                                    for lot in task_lots for command in ('release','durable-release')]
                    if key not in allowed_keys:
                        block('FINANCIAL_MOVEMENT_KEY_INVALID')
                else:
                    block('FINANCIAL_MOVEMENT_STATE_INVALID')
            # Migration105/JSON legacy attribution writes allocations but deliberately
            # no economic RESERVE movement. Its legacy wallet proof is mandatory above.
            normal_reserve = reserves[0][7] == 'personal-point:reserve:' + aid + ':' + reserve_key
            if modern and normal_reserve and (len(reserve_movements) != len(task_lots) or
                                             sum(row[1] for row in reserve_movements) != reservations[0][6]):
                block('FINANCIAL_MOVEMENT_COUNT_OR_TOTAL')
            if modern and any(movement_amounts[kind] != amounts[kind] for kind in movement_amounts):
                block('FINANCIAL_MOVEMENT_COUNT_OR_TOTAL')
            families['ledger'], ledger_ids = _financial_family(cursor, 'xz_wallet_ledger',
                owner_predicate + ' OR task_id=%s OR reference_id=%s OR idempotency_key=ANY(%s)', (aid,user,tid,tid,keys), aid,user,
                common + " AND entry_type IN ('RECHARGE','GRANT','RESERVE','CAPTURE','RELEASE','REFUND','ADJUSTMENT','EXPIRE') AND points>=0 "
                "AND available_before>=0 AND available_after>=0 AND frozen_before>=0 AND frozen_after>=0 "
                "AND CASE entry_type WHEN 'RESERVE' THEN available_after=available_before-points AND frozen_after=frozen_before+points "
                "WHEN 'CAPTURE' THEN available_after=available_before AND frozen_after=frozen_before-points "
                "WHEN 'RELEASE' THEN available_after=available_before+points AND frozen_after=frozen_before-points "
                "WHEN 'EXPIRE' THEN available_after=available_before-points AND frozen_after=frozen_before "
                "WHEN 'ADJUSTMENT' THEN available_after IN (available_before-points,available_before+points) AND frozen_after=frozen_before "
                "ELSE available_after=available_before+points AND frozen_after=frozen_before END", ['idempotency_key'])
            events = _query(cursor, """
SELECT id,(user_id=%s AND (tenant_id IS NULL OR tenant_id IN ('','tenant_default'))
           AND task_id=%s AND idempotency_key=task_id || ':' || event_type
           AND event_type IN ('QUOTE','RESERVE','CAPTURE','RELEASE','REFUND','BILLING_FAILED')
           AND billing_status=CASE event_type WHEN 'QUOTE' THEN 'QUOTED' WHEN 'RESERVE' THEN 'RESERVED'
             WHEN 'CAPTURE' THEN 'CAPTURED' WHEN 'RELEASE' THEN 'RELEASED' WHEN 'REFUND' THEN 'REFUNDED' ELSE 'BILLING_FAILED' END)
FROM public.xz_billing_lifecycle_events WHERE task_id=%s OR idempotency_key=ANY(%s)
""", (user,tid,tid,[tid+':'+kind for kind in ('QUOTE','RESERVE','CAPTURE','RELEASE','REFUND','BILLING_FAILED')]))
            if any(not row[0] or row[1] is not True for row in events):
                block('FINANCIAL_EVENT_LINKAGE_INVALID')
            event_ids = [row[0] for row in events]
            counts = _query(cursor, 'SELECT id,count(*) FROM public.xz_billing_lifecycle_events WHERE id=ANY(%s) GROUP BY id', (event_ids,))
            if len(counts) != len(event_ids) or any(row[1] != 1 for row in counts):
                block('FINANCIAL_DUPLICATE_IDENTITY')
            if _query(cursor, 'SELECT count(*) FROM public.xz_billing_lifecycle_events WHERE idempotency_key IN (SELECT idempotency_key FROM public.xz_billing_lifecycle_events WHERE id=ANY(%s)) GROUP BY idempotency_key HAVING count(*)<>1', (event_ids,)):
                block('FINANCIAL_DUPLICATE_KEY')
            families['billing_lifecycle_events'] = _rows(cursor, 'xz_billing_lifecycle_events', 'id=ANY(%s)', (event_ids,), FINANCIAL_SCHEMA)
            if len(families['billing_lifecycle_events']) != len(events):
                block('AMBIGUOUS_COUNT')
            history_predicate = "task_id=%s OR raw->>'taskId'=%s OR id IN (SELECT billing_event_id FROM public.xz_wallet_ledger WHERE task_id=%s)"
            history_params = (tid,tid,tid)
            history = _query(cursor, """
SELECT id,(user_id=%s AND task_id=%s
    AND (tenant_id IS NULL OR tenant_id IN ('','tenant_default'))
    AND jsonb_typeof(raw)='object'
    AND (NOT (raw ? 'id') OR raw->>'id'=id)
    AND (NOT (raw ? 'taskId') OR raw->>'taskId'=task_id)
    AND (NOT (raw ? 'userId') OR raw->>'userId'=user_id))
FROM public.xz_billing_events WHERE """ + history_predicate, (user,tid)+history_params)
            history_ids = [row[0] for row in history]
            if any(not row[0] or row[1] is not True for row in history):
                block('FINANCIAL_HISTORY_LINKAGE_INVALID')
            counts = _query(cursor, 'SELECT id,count(*) FROM public.xz_billing_events WHERE id=ANY(%s) GROUP BY id', (history_ids,))
            if len(counts) != len(history_ids) or any(row[1]!=1 for row in counts):
                block('FINANCIAL_DUPLICATE_IDENTITY')
            if _query(cursor, "SELECT count(*) FROM public.xz_billing_events WHERE transaction_id IN (SELECT transaction_id FROM public.xz_billing_events WHERE id=ANY(%s)) AND transaction_id<>'' GROUP BY transaction_id HAVING count(*)<>1", (history_ids,)):
                block('FINANCIAL_DUPLICATE_KEY')
            families['billing_history'] = _rows(cursor, 'xz_billing_events', history_predicate, history_params, FINANCIAL_SCHEMA)
            if len(families['billing_history']) != len(history):
                block('AMBIGUOUS_COUNT')
            financial = {'version': FINANCIAL_VERSION, 'scope': 'PERSONAL_FINANCIAL_ONLY',
                         'schema': FINANCIAL_SCHEMA, 'path': 'PERSONAL_LOT_V1' if modern else 'LEGACY_WALLET_ONLY',
                         'counts': {key: len(value) for key,value in families.items()}, 'families': families}
            snapshots[item['execution_id']] = {'version': PARTIAL_VERSION, 'scope': 'CORE_PERSONAL_FINANCIAL_ONLY',
                                              'core': cores[item['execution_id']], 'financial': financial}
        else:
            families = {
                'tenant_wallet': _rows(cursor, 'xz_tenant_wallets', 'tenant_id=%s', (tenant,), FINANCIAL_SCHEMA),
                'ledger': _rows(cursor, 'xz_wallet_ledger', 'task_id=%s', (tid,), FINANCIAL_SCHEMA),
                'billing_lifecycle_events': _rows(cursor, 'xz_billing_lifecycle_events', 'task_id=%s', (tid,), FINANCIAL_SCHEMA),
            }
            if len(families['tenant_wallet']) != 1:
                block('FINANCIAL_ACCOUNT_COUNT')
            events = _query(cursor, """
SELECT id,(user_id=%s AND tenant_id=%s
           AND task_id=%s AND idempotency_key=task_id || ':' || event_type
           AND event_type IN ('QUOTE','RESERVE','RELEASE')
           AND billing_status=CASE event_type WHEN 'QUOTE' THEN 'QUOTED' WHEN 'RESERVE' THEN 'RESERVED'
             WHEN 'RELEASE' THEN 'RELEASED' END)
FROM public.xz_billing_lifecycle_events WHERE task_id=%s OR idempotency_key=ANY(%s)
""", (user, tenant, tid, tid, [tid+':'+kind for kind in ('QUOTE','RESERVE','RELEASE')]))
            if len(events) != 3 or any(not row[0] or row[1] is not True for row in events):
                block('FINANCIAL_EVENT_LINKAGE_INVALID')
            financial = {
                'version': FINANCIAL_VERSION, 'scope': 'ENTERPRISE_FINANCIAL_ONLY',
                'schema': FINANCIAL_SCHEMA, 'path': 'ENTERPRISE_WALLET_ONLY',
                'counts': {key: len(value) for key, value in families.items()},
                'families': families
            }
            snapshots[item['execution_id']] = {
                'version': PARTIAL_VERSION, 'scope': 'CORE_ENTERPRISE_FINANCIAL_ONLY',
                'core': cores[item['execution_id']], 'financial': financial
            }
    return snapshots


def core_financial_sha256(snapshot):
    """Scoped partial digest, NEVER the approved final snapshot_sha256."""
    if not isinstance(snapshot, dict) or snapshot.get('version') != PARTIAL_VERSION or snapshot.get('scope') not in ('CORE_PERSONAL_FINANCIAL_ONLY', 'CORE_ENTERPRISE_FINANCIAL_ONLY'):
        block('PARTIAL_SCOPE_INVALID')
    return hashlib.sha256(canonical(snapshot)).hexdigest()


def compare_core_financial_in_transaction(cursor, entries, expected_partial_sha256):
    snapshots = project_core_financial_in_transaction(cursor, entries)
    if not isinstance(expected_partial_sha256, dict) or set(expected_partial_sha256) != set(snapshots):
        block('APPROVED_COUNT_INVALID')
    for eid, snapshot in snapshots.items():
        expected = expected_partial_sha256[eid]
        if not isinstance(expected, str) or not re.fullmatch('[0-9a-f]{64}', expected) or core_financial_sha256(snapshot) != expected:
            block('PARTIAL_HASH_MISMATCH')
    return snapshots


def sample_core_financial_read_only(connection, entries):
    """Own READ ONLY sampling; this transaction is NOT registration locking."""
    try:
        if connection.closed:
            block('DB_QUERY_FAILED')
        if connection.autocommit is not True or connection.get_transaction_status() != 0:
            block('TRANSACTION_REQUIRED')
        cursor = connection.cursor()
        cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY')
    except SnapshotError:
        raise
    except Exception:
        block('DB_QUERY_FAILED')
    try:
        return project_core_financial_in_transaction(cursor, entries)
    finally:
        try:
            cursor.execute('ROLLBACK')
            cursor.close()
        except Exception:
            block('DB_QUERY_FAILED')


def validate_live_approval_in_transaction(cursor, raw_bytes, expected_file_sha256,
                                           release_sha, operation):
    """Verify signed authority and recompute the complete declared DB snapshot.

    This remains a read-only gate. External object health and exact historical
    provider-attempt ownership are explicit snapshot limitations, not silently
    promoted to proven facts.
    """
    return validate_live_snapshot_in_transaction(
        cursor, raw_bytes, release_sha, operation, expected_file_sha256)


# Exact migrations021/044/046/047/107; claim identity is migration115 index.
ASSET_STORAGE_SCHEMA = {'xz_assets': [['created_at', 'text', 'YES', None, None, None],
               ['deleted_at', 'timestamptz', 'YES', None, None, None],
               ['favorite', 'bool', 'NO', None, None, None],
               ['id', 'text', 'NO', None, None, None],
               ['media_type', 'text', 'YES', None, None, None],
               ['metadata', 'jsonb', 'NO', None, None, None],
               ['name', 'text', 'YES', None, None, None],
               ['organization_id', 'text', 'YES', None, None, None],
               ['raw', 'jsonb', 'NO', None, None, None],
               ['task_id', 'text', 'YES', None, None, None],
               ['tenant_id', 'text', 'YES', None, None, None],
               ['thumbnail_url', 'text', 'YES', None, None, None],
               ['updated_at', 'text', 'YES', None, None, None],
               ['url', 'text', 'YES', None, None, None],
               ['user_id', 'text', 'YES', None, None, None]],
 'xz_file_objects': [['bucket', 'text', 'NO', None, None, None],
                     ['business_id', 'text', 'YES', None, None, None],
                     ['business_type', 'text', 'NO', None, None, None],
                     ['created_at', 'timestamptz', 'NO', None, None, None],
                     ['deleted_at', 'timestamptz', 'YES', None, None, None],
                     ['duration_ms', 'int8', 'YES', None, 64, 0],
                     ['etag', 'text', 'YES', None, None, None],
                     ['expires_at', 'timestamptz', 'YES', None, None, None],
                     ['extension', 'text', 'YES', None, None, None],
                     ['file_hash', 'text', 'YES', None, None, None],
                     ['file_id', 'text', 'NO', None, None, None],
                     ['file_size', 'int8', 'NO', None, 64, 0],
                     ['hash_algorithm', 'text', 'YES', None, None, None],
                     ['height', 'int4', 'YES', None, 32, 0],
                     ['is_temporary', 'bool', 'NO', None, None, None],
                     ['metadata', 'jsonb', 'NO', None, None, None],
                     ['mime_type', 'text', 'YES', None, None, None],
                     ['object_key', 'text', 'NO', None, None, None],
                     ['original_name', 'text', 'NO', None, None, None],
                     ['page_count', 'int4', 'YES', None, 32, 0],
                     ['provider', 'text', 'NO', None, None, None],
                     ['recycle_expires_at', 'timestamptz', 'YES', None, None, None],
                     ['reference_count', 'int4', 'NO', None, 32, 0],
                     ['reserved_size', 'int8', 'NO', None, 64, 0],
                     ['status', 'text', 'NO', None, None, None],
                     ['storage_config_id', 'text', 'NO', None, None, None],
                     ['stored_name', 'text', 'NO', None, None, None],
                     ['tenant_id', 'text', 'NO', None, None, None],
                     ['updated_at', 'timestamptz', 'NO', None, None, None],
                     ['user_id', 'text', 'NO', None, None, None],
                     ['visibility', 'text', 'NO', None, None, None],
                     ['width', 'int4', 'YES', None, 32, 0]],
 'xz_file_relations': [['created_at', 'timestamptz', 'NO', None, None, None],
                       ['id', 'text', 'NO', None, None, None],
                       ['relation_type', 'text', 'NO', None, None, None],
                       ['source_file_id', 'text', 'NO', None, None, None],
                       ['target_file_id', 'text', 'NO', None, None, None],
                       ['tenant_id', 'text', 'NO', None, None, None]],
 'xz_multipart_upload_parts': [['completed_at', 'timestamptz', 'NO', None, None, None],
                               ['etag', 'text', 'NO', None, None, None],
                               ['part_number', 'int4', 'NO', None, 32, 0],
                               ['size_bytes', 'int8', 'NO', None, 64, 0],
                               ['upload_id', 'text', 'NO', None, None, None]],
 'xz_multipart_uploads': [['completed_at', 'timestamptz', 'YES', None, None, None],
                          ['content_type', 'text', 'NO', None, None, None],
                          ['created_at', 'timestamptz', 'NO', None, None, None],
                          ['expires_at', 'timestamptz', 'NO', None, None, None],
                          ['file_id', 'text', 'NO', None, None, None],
                          ['file_name', 'text', 'NO', None, None, None],
                          ['id', 'text', 'NO', None, None, None],
                          ['idempotency_key', 'text', 'YES', None, None, None],
                          ['object_key', 'text', 'NO', None, None, None],
                          ['owner_user_id', 'text', 'NO', None, None, None],
                          ['part_size', 'int8', 'NO', None, 64, 0],
                          ['provider_upload_id', 'text', 'NO', None, None, None],
                          ['state', 'text', 'NO', None, None, None],
                          ['tenant_id', 'text', 'NO', None, None, None],
                          ['total_parts', 'int4', 'NO', None, 32, 0],
                          ['total_size', 'int8', 'NO', None, 64, 0]],
 'xz_storage_configs': [['access_key_encrypted', 'text', 'YES', None, None, None],
                        ['bucket', 'text', 'NO', None, None, None],
                        ['cdn_domain', 'text', 'YES', None, None, None],
                        ['created_at', 'timestamptz', 'NO', None, None, None],
                        ['created_by', 'text', 'YES', None, None, None],
                        ['deleted_at', 'timestamptz', 'YES', None, None, None],
                        ['endpoint', 'text', 'NO', None, None, None],
                        ['force_path_style', 'bool', 'NO', None, None, None],
                        ['id', 'text', 'NO', None, None, None],
                        ['is_default', 'bool', 'NO', None, None, None],
                        ['is_system', 'bool', 'NO', None, None, None],
                        ['last_test_at', 'timestamptz', 'YES', None, None, None],
                        ['last_test_message', 'text', 'YES', None, None, None],
                        ['last_test_status', 'text', 'YES', None, None, None],
                        ['name', 'text', 'NO', None, None, None],
                        ['object_prefix', 'text', 'NO', None, None, None],
                        ['provider', 'text', 'NO', None, None, None],
                        ['public_domain', 'text', 'YES', None, None, None],
                        ['purpose', 'text', 'NO', None, None, None],
                        ['region', 'text', 'YES', None, None, None],
                        ['secret_key_encrypted', 'text', 'YES', None, None, None],
                        ['session_token_encrypted', 'text', 'YES', None, None, None],
                        ['signing_endpoint', 'text', 'YES', None, None, None],
                        ['status', 'text', 'NO', None, None, None],
                        ['tenant_id', 'text', 'NO', None, None, None],
                        ['updated_at', 'timestamptz', 'NO', None, None, None],
                        ['updated_by', 'text', 'YES', None, None, None],
                        ['use_ssl', 'bool', 'NO', None, None, None]],
 'xz_storage_jobs': [['attempt', 'int4', 'NO', None, 32, 0],
                     ['created_at', 'timestamptz', 'NO', None, None, None],
                     ['error_code', 'text', 'YES', None, None, None],
                     ['error_message', 'text', 'YES', None, None, None],
                     ['file_id', 'text', 'YES', None, None, None],
                     ['id', 'text', 'NO', None, None, None],
                     ['job_type', 'text', 'NO', None, None, None],
                     ['max_attempts', 'int4', 'NO', None, 32, 0],
                     ['metadata', 'jsonb', 'NO', None, None, None],
                     ['run_after', 'timestamptz', 'NO', None, None, None],
                     ['status', 'text', 'NO', None, None, None],
                     ['tenant_id', 'text', 'NO', None, None, None],
                     ['updated_at', 'timestamptz', 'NO', None, None, None]]}

ASSET_STORAGE_VERSION = 'issue203-task-assets-storage-db-only-v1'
ASSET_PARTIAL_VERSION = 'issue203-core-financial-assets-db-partial-v1'

# No artwork table and no invented execution/generation property on artifacts.
# Bidirectional closure: task/result IDs -> assets <-> file references, plus
# durable business/name claims -> files. Discovery is deliberately NOT filtered
# by owner/tenant/status; those are validated after discovery. Reference images
# and arbitrary user uploads are not generated outputs and are not swept in.
_ASSET_GRAPH = r"""
WITH RECURSIVE t AS (SELECT * FROM public.xz_generation_tasks WHERE id=%s),
result_refs AS (
 SELECT value #>> '{}' AS id, ordinality AS result_index FROM t,
 jsonb_array_elements(t.result_ids) WITH ORDINALITY
 UNION
 SELECT value #>> '{}', ordinality FROM t,
 jsonb_array_elements(CASE WHEN jsonb_typeof(raw->'resultIds')='array' THEN raw->'resultIds' ELSE '[]'::jsonb END) WITH ORDINALITY
), records AS (
 SELECT value AS record FROM t, jsonb_array_elements(
 CASE WHEN jsonb_typeof(params->'generated_storage_files')='array' THEN params->'generated_storage_files' ELSE '[]'::jsonb END)
 UNION ALL
 SELECT value FROM t, jsonb_array_elements(
 CASE WHEN jsonb_typeof(raw->'params'->'generated_storage_files')='array' THEN raw->'params'->'generated_storage_files' ELSE '[]'::jsonb END)
), asset_refs AS (
 SELECT a.id, ref FROM public.xz_assets a CROSS JOIN LATERAL (
   SELECT value #>> '{}' AS ref FROM jsonb_each(
     CASE WHEN jsonb_typeof(a.metadata)='object' THEN a.metadata ELSE '{}'::jsonb END)
   WHERE key IN ('fileId','storageFileId','coverFileId','thumbnailFileId')
   UNION SELECT value #>> '{}' FROM jsonb_each(
     CASE WHEN jsonb_typeof(a.raw->'metadata')='object' THEN a.raw->'metadata' ELSE '{}'::jsonb END)
   WHERE key IN ('fileId','storageFileId','coverFileId','thumbnailFileId')
   UNION SELECT btrim(substr(btrim(v),11)) FROM (VALUES(a.url),(a.thumbnail_url),(a.raw->>'url'),(a.raw->>'thumbnailUrl')) q(v)
     WHERE lower(btrim(v)) LIKE 'storage://%%'
 ) refs WHERE ref IS NOT NULL AND ref<>''
), task_file_refs AS (
 SELECT value #>> '{}' AS ref FROM records, jsonb_each(CASE WHEN jsonb_typeof(record)='object' THEN record ELSE '{}'::jsonb END)
 WHERE key IN ('fileId','coverFileId')
 UNION
 SELECT btrim(substr(btrim(value #>> '{}'),11)) FROM t,
 jsonb_each(raw) WHERE key IN ('imageUrl','outputUrl','resultUrl','thumbnailUrl') AND lower(btrim(value #>> '{}')) LIKE 'storage://%%'
 UNION
 SELECT btrim(substr(btrim(v #>> '{}'),11)) FROM public.provider_executions e,t,
 LATERAL jsonb_path_query(e.result_metadata,'$.**') v
 WHERE e.task_id=t.id AND jsonb_typeof(v)='string' AND lower(btrim(v #>> '{}')) LIKE 'storage://%%'
), edges AS (
 SELECT 'a:'||id AS src, 'f:'||ref AS dst FROM asset_refs
 UNION SELECT 'f:'||ref,'a:'||id FROM asset_refs
), nodes(node) AS (
 SELECT 'a:'||a.id FROM public.xz_assets a,t
 WHERE a.task_id=t.id OR a.raw->>'taskId'=t.id OR a.id IN (SELECT id FROM result_refs)
 UNION SELECT 'f:'||f.file_id FROM public.xz_file_objects f,t
 WHERE f.business_id=t.id OR left(f.original_name,length(t.id)+1)=t.id||'-'
 UNION SELECT 'f:'||ref FROM task_file_refs
 UNION SELECT e.dst FROM nodes n JOIN edges e ON e.src=n.node
), aa AS (SELECT a.* FROM public.xz_assets a WHERE 'a:'||a.id IN (SELECT node FROM nodes)),
ff AS (SELECT f.* FROM public.xz_file_objects f WHERE 'f:'||f.file_id IN (SELECT node FROM nodes)),
cc AS (SELECT c.* FROM public.xz_storage_configs c WHERE c.id IN (SELECT storage_config_id FROM ff)
 OR (EXISTS(SELECT 1 FROM ff) AND c.tenant_id IN ('tenant_default','platform') AND c.is_default AND c.deleted_at IS NULL)),
rr AS (SELECT r.* FROM public.xz_file_relations r WHERE r.source_file_id IN (SELECT file_id FROM ff) OR r.target_file_id IN (SELECT file_id FROM ff)),
jj AS (SELECT j.* FROM public.xz_storage_jobs j WHERE j.file_id IN (SELECT file_id FROM ff)
 OR EXISTS(SELECT 1 FROM jsonb_path_query(j.metadata,'$.**') v WHERE v IN (SELECT to_jsonb(id) FROM t UNION SELECT to_jsonb(file_id) FROM ff UNION SELECT to_jsonb(object_key) FROM ff))),
mm AS (SELECT m.* FROM public.xz_multipart_uploads m WHERE m.file_id IN (SELECT file_id FROM ff)
 OR m.object_key IN (SELECT object_key FROM ff) OR EXISTS(SELECT 1 FROM t WHERE left(m.file_name,length(t.id)+1)=t.id||'-')),
pp AS (SELECT p.* FROM public.xz_multipart_upload_parts p WHERE p.upload_id IN (SELECT id FROM mm))
"""


def _asset_check(cursor, tid, sql, code, parameters=()):
    if _one(cursor, _ASSET_GRAPH + ' SELECT (' + sql + ')', (tid,) + parameters)[0] is not True:
        block(code)


def _asset_family(cursor, tid, table, alias, key):
    count, distinct = _one(cursor, _ASSET_GRAPH + ' SELECT count(*),count(DISTINCT ' + key + ') FROM ' + alias, (tid,))
    if count > MAX_ROWS:
        block('COUNT_LIMIT')
    if count != distinct:
        block('ASSET_DUPLICATE_IDENTITY')
    fields = ','.join("CASE WHEN \"%s\" IS NULL THEN NULL ELSE encode(public.digest(convert_to(\"%s\"::text,'UTF8'),'sha256'),'hex') END" % (c[0],c[0]) for c in ASSET_STORAGE_SCHEMA[table])
    rows = _query(cursor, _ASSET_GRAPH + ' SELECT ' + fields + ' FROM ' + alias, (tid,))
    if len(rows) != count:
        block('AMBIGUOUS_COUNT')
    projected = []
    for row in rows:
        if len(row) != len(ASSET_STORAGE_SCHEMA[table]):
            block('SCHEMA_MISMATCH')
        for column, value in zip(ASSET_STORAGE_SCHEMA[table],row):
            if value is None and column[2]=='NO':
                block('NULL_REQUIRED_FIELD')
            if value is not None and (not isinstance(value,str) or not re.fullmatch('[0-9a-f]{64}',value)):
                block('PROJECTION_INVALID')
        projected.append([[column[0],column[1],value] for column,value in zip(ASSET_STORAGE_SCHEMA[table],row)])
    return sorted(projected,key=canonical)


def project_core_financial_assets_in_transaction(cursor, entries):
    """PARTIAL task-level DB projection, not exact historical attempt attribution.

    Same cursor/transaction as the unchanged financial projection. Does not prove
    external object presence/health, provider terminality, absence of late success,
    generation fencing, registration atomicity or final snapshot completeness.
    """
    snapshots = project_core_financial_in_transaction(cursor,entries)
    _schema(cursor,ASSET_STORAGE_SCHEMA)
    for item in entries:
        tid = item['task_id']
        # Nullable/missing provider result_metadata is a known pre-result state,
        # not proof of failure. Present results must use the actual writer shape.
        _asset_check(cursor,tid,"""
NOT EXISTS(SELECT 1 FROM t WHERE jsonb_typeof(raw)<>'object'
 OR (raw ? 'id' AND raw->>'id' IS DISTINCT FROM id)
 OR (raw ? 'userId' AND raw->>'userId' IS DISTINCT FROM user_id)
 OR (params ? 'tenant_id' AND coalesce(nullif(params->>'tenant_id',''),'tenant_default')<>coalesce(nullif(tenant_id,''),'tenant_default'))
 OR (raw ? 'resultIds' AND (jsonb_typeof(raw->'resultIds')<>'array' OR raw->'resultIds'<>result_ids))
 OR EXISTS(SELECT 1 FROM jsonb_array_elements(result_ids) v WHERE jsonb_typeof(v)<>'string' OR v #>> '{}'='')
 OR (SELECT count(*) FROM jsonb_array_elements(result_ids))<>(SELECT count(DISTINCT v) FROM jsonb_array_elements(result_ids) v)
 OR (params ? 'generated_storage_files' AND jsonb_typeof(params->'generated_storage_files')<>'array')
 OR (raw->'params' ? 'generated_storage_files' AND raw->'params'->'generated_storage_files' IS DISTINCT FROM params->'generated_storage_files'))
AND NOT EXISTS(SELECT 1 FROM records WHERE jsonb_typeof(record)<>'object' OR jsonb_typeof(record->'fileId') IS DISTINCT FROM 'string' OR record->>'fileId'=''
 OR (record ? 'coverFileId' AND (jsonb_typeof(record->'coverFileId')<>'string' OR record->>'coverFileId'='')))
""",'RESULT_STATE_INVALID')
        _asset_check(cursor,tid,"""
NOT EXISTS(SELECT 1 FROM public.provider_executions e,t WHERE e.task_id=t.id AND
 ((e.result_metadata IS NOT NULL AND (e.result_metadata='null'::jsonb OR
  (e.capability='image' AND (jsonb_typeof(e.result_metadata)<>'array' OR e.result_metadata='[]'::jsonb)) OR
  (e.capability='video' AND jsonb_typeof(e.result_metadata)<>'object') OR e.capability NOT IN ('image','video')))
 OR (e.status='succeeded' AND e.result_metadata IS NULL)))
""",'PROVIDER_RESULT_STATE_INVALID')
        # JSON unnest is guarded even for malformed states; errors never expose SQL.
        _asset_check(cursor,tid,"""
NOT EXISTS(SELECT 1 FROM public.provider_executions e,t,
 jsonb_array_elements(CASE WHEN jsonb_typeof(e.result_metadata)='array' THEN e.result_metadata ELSE '[]'::jsonb END) v
 WHERE e.task_id=t.id AND (jsonb_typeof(v)<>'object' OR jsonb_typeof(v->'URL') IS DISTINCT FROM 'string' OR v->>'URL'=''))
""",'PROVIDER_RESULT_STATE_INVALID')

        families = {}
        for name,table,alias,key in [
            ('assets','xz_assets','aa','id'),('files','xz_file_objects','ff','file_id'),
            ('configs','xz_storage_configs','cc','id'),('relations','xz_file_relations','rr','id'),
            ('jobs','xz_storage_jobs','jj','id'),('multipart','xz_multipart_uploads','mm','id'),
            ('parts','xz_multipart_upload_parts','pp','(upload_id,part_number)')]:
            families[name] = _asset_family(cursor,tid,table,alias,key)
        if any(families[name] for name in ('relations','jobs','multipart','parts')):
            block('UNSUPPORTED_STORAGE_LINK')
        _asset_check(cursor,tid,"""
NOT EXISTS(SELECT 1 FROM nodes WHERE (left(node,2)='a:' AND NOT EXISTS(SELECT 1 FROM aa WHERE 'a:'||id=node))
 OR (left(node,2)='f:' AND NOT EXISTS(SELECT 1 FROM ff WHERE 'f:'||file_id=node)))
AND NOT EXISTS(SELECT 1 FROM result_refs WHERE NOT EXISTS(SELECT 1 FROM aa WHERE aa.id=result_refs.id))
""",'RESULT_REFERENCE_MISSING')
        _asset_check(cursor,tid,"""
NOT EXISTS(SELECT 1 FROM aa a,t WHERE a.user_id IS DISTINCT FROM t.user_id OR a.task_id IS DISTINCT FROM t.id
 OR coalesce(a.tenant_id,'')<>coalesce(t.tenant_id,'') OR coalesce(a.organization_id,'')<>coalesce(t.organization_id,'')
 OR a.media_type NOT IN ('image','video') OR a.media_type IS NULL
 OR jsonb_typeof(a.metadata)<>'object' OR jsonb_typeof(a.raw)<>'object'
 OR a.raw->>'id' IS DISTINCT FROM a.id OR a.raw->>'taskId' IS DISTINCT FROM a.task_id OR a.raw->>'userId' IS DISTINCT FROM a.user_id
 OR coalesce(a.raw->>'tenantId','')<>coalesce(a.tenant_id,'') OR coalesce(a.raw->>'organizationId','')<>coalesce(a.organization_id,'')
 OR a.raw->>'mediaType' IS DISTINCT FROM a.media_type OR a.raw->>'url' IS DISTINCT FROM a.url
 OR coalesce(a.raw->>'thumbnailUrl','')<>coalesce(a.thumbnail_url,'') OR a.raw->'metadata' IS DISTINCT FROM a.metadata
 OR jsonb_typeof(a.metadata->'index') IS DISTINCT FROM 'number'
 OR NOT EXISTS(SELECT 1 FROM result_refs r WHERE r.id=a.id AND a.metadata->>'index'=r.result_index::text))
AND NOT EXISTS(SELECT metadata->>'index' FROM aa GROUP BY metadata->>'index' HAVING count(*)<>1)
AND NOT EXISTS(SELECT 1 FROM public.xz_generation_tasks other,t WHERE other.id<>t.id AND
 (EXISTS(SELECT 1 FROM aa WHERE other.result_ids ? aa.id OR other.raw->'resultIds' ? aa.id)))
""",'ASSET_LINKAGE_INVALID')
        _asset_check(cursor,tid,"""
NOT EXISTS(SELECT 1 FROM public.xz_generation_tasks other,t WHERE other.id<>t.id AND
 (EXISTS(SELECT 1 FROM jsonb_path_query(other.params->'generated_storage_files','$.**') v,ff WHERE v=to_jsonb(ff.file_id))
 OR EXISTS(SELECT 1 FROM jsonb_path_query(other.raw->'params'->'generated_storage_files','$.**') v,ff WHERE v=to_jsonb(ff.file_id))
 OR EXISTS(SELECT 1 FROM jsonb_each(other.raw) v,ff WHERE v.key IN ('imageUrl','outputUrl','resultUrl','thumbnailUrl') AND lower(btrim(v.value #>> '{}'))='storage://'||ff.file_id)))
AND NOT EXISTS(SELECT 1 FROM public.provider_executions e,t,jsonb_path_query(e.result_metadata,'$.**') v,ff
 WHERE e.task_id<>t.id AND lower(btrim(v #>> '{}'))='storage://'||ff.file_id)
""",'STORAGE_REVERSE_REFERENCE_CONFLICT')
        if families['assets'] or families['files']:
            _asset_check(cursor,tid,"""
(SELECT count(*) FROM public.provider_executions e,t WHERE e.task_id=t.id)=1
AND NOT EXISTS(SELECT 1 FROM public.provider_executions e,t WHERE e.task_id=t.id AND e.task_execution_generation IS DISTINCT FROM t.execution_generation)
""",'ASSET_ATTEMPT_ATTRIBUTION_AMBIGUOUS')
        _asset_check(cursor,tid,"""
NOT EXISTS(SELECT 1 FROM ff WHERE storage_config_id='env_default')
""",'UNSUPPORTED_STORAGE_CONFIG')
        _asset_check(cursor,tid,"""
NOT EXISTS(SELECT 1 FROM ff f,t WHERE f.user_id<>t.user_id OR f.tenant_id<>coalesce(nullif(t.tenant_id,''),'tenant_default')
 OR f.business_id IS DISTINCT FROM t.id OR f.business_type<>'generation_result' OR f.visibility<>'PRIVATE'
 OR f.status NOT IN ('PENDING_UPLOAD','ACTIVE','UPLOAD_FAILED','QUARANTINED','DELETE_PENDING','DELETED','EXPIRED')
 OR left(f.original_name,length(t.id)+1)<>t.id||'-'
 OR substr(f.original_name,length(t.id)+2) !~ '^[0-9]{2,}(-cover)?[.][a-z0-9]+$'
 OR f.file_size<0 OR f.reserved_size<0 OR f.reference_count<0 OR jsonb_typeof(f.metadata)<>'object'
 OR f.hash_algorithm IS DISTINCT FROM 'sha256' OR f.file_hash !~ '^[0-9a-f]{64}$' OR f.file_hash IS NULL
 OR f.object_key IS DISTINCT FROM 'tenants/'||f.tenant_id||'/generation_result/artifacts/'||
 substr(encode(public.digest(convert_to(f.tenant_id||'|generation_result|'||f.business_id||'|'||f.original_name||'|'||f.file_hash,'UTF8'),'sha256'),'hex'),1,32)||'.'||f.extension
 OR f.stored_name IS DISTINCT FROM substr(f.object_key,length('tenants/'||f.tenant_id||'/generation_result/artifacts/')+1)
 OR (f.status='ACTIVE' AND (f.file_size<=0 OR f.reserved_size<>0))
 OR (f.status='PENDING_UPLOAD' AND (f.file_size<>0 OR f.reserved_size<=0))
 OR NOT EXISTS(SELECT 1 FROM cc c WHERE c.id=f.storage_config_id AND c.tenant_id IN (f.tenant_id,'platform')
 AND c.provider=f.provider AND c.bucket=f.bucket AND c.deleted_at IS NULL AND c.status IN ('ENABLED','DISABLED')))
AND NOT EXISTS(SELECT 1 FROM public.xz_file_objects f WHERE (f.storage_config_id,f.bucket,f.object_key) IN (SELECT storage_config_id,bucket,object_key FROM ff)
 GROUP BY f.storage_config_id,f.bucket,f.object_key HAVING count(*)<>1)
AND NOT EXISTS(SELECT 1 FROM public.xz_file_objects f WHERE (f.tenant_id,f.business_type,f.business_id,f.original_name) IN
 (SELECT tenant_id,business_type,business_id,original_name FROM ff) AND f.status IN ('ACTIVE','PENDING_UPLOAD')
 GROUP BY f.tenant_id,f.business_type,f.business_id,f.original_name HAVING count(*)<>1)
AND NOT EXISTS(SELECT tenant_id FROM cc WHERE is_default AND status='ENABLED' AND deleted_at IS NULL GROUP BY tenant_id HAVING count(*)<>1)
""",'STORAGE_CLAIM_INVALID')
        _asset_check(cursor,tid,"""
NOT EXISTS(SELECT 1 FROM aa a WHERE a.metadata->'storageManaged' IS DISTINCT FROM 'true'::jsonb
 OR jsonb_typeof(a.metadata->'fileId') IS DISTINCT FROM 'string'
 OR a.metadata->>'fileId' IS DISTINCT FROM a.metadata->>'storageFileId'
 OR EXISTS(SELECT 1 FROM jsonb_each(a.metadata) v WHERE key IN ('fileId','storageFileId','coverFileId','thumbnailFileId') AND (jsonb_typeof(value)<>'string' OR value #>> '{}'=''))
 OR (lower(btrim(a.url)) LIKE 'storage://%%' AND btrim(substr(btrim(a.url),11)) IS DISTINCT FROM a.metadata->>'fileId')
 OR (a.metadata ? 'coverFileId' AND NOT EXISTS(SELECT 1 FROM ff f WHERE f.file_id=a.metadata->>'coverFileId'
 AND f.original_name=a.task_id||'-'||lpad(a.metadata->>'index',2,'0')||'-cover.'||f.extension
 AND a.thumbnail_url='storage://'||f.file_id))
 OR NOT EXISTS(SELECT 1 FROM ff f WHERE f.file_id=a.metadata->>'fileId'
 AND a.metadata->>'storageTenantId'=f.tenant_id AND a.metadata->>'storageProvider'=f.provider
 AND a.metadata->>'storageBucket'=f.bucket AND a.metadata->>'storageObjectKey'=f.object_key
 AND a.metadata->>'fileSize'=f.file_size::text AND a.metadata->>'fileSizeBytes'=f.file_size::text
 AND a.metadata->>'contentType'=f.mime_type
 AND f.original_name= a.task_id||'-'||lpad(a.metadata->>'index',2,'0')||'.'||f.extension)
 OR (a.media_type='video' AND a.url IS DISTINCT FROM 'storage://'||(a.metadata->>'fileId')))
AND NOT EXISTS(SELECT 1 FROM records WHERE NOT EXISTS(SELECT 1 FROM ff f WHERE f.file_id=record->>'fileId'
 AND record->>'tenantId'=f.tenant_id AND record->>'provider'=f.provider AND record->>'bucket'=f.bucket
 AND record->>'objectKey'=f.object_key AND record->>'fileSize'=f.file_size::text AND record->>'contentType'=f.mime_type))
""",'STORAGE_REFERENCE_CONFLICT')
        # Migration115 is a constraint, not a separate claim row.
        _asset_check(cursor,tid,"""
EXISTS(SELECT 1 FROM pg_catalog.pg_index i JOIN pg_catalog.pg_class c ON c.oid=i.indexrelid
 WHERE c.relnamespace='public'::regnamespace AND c.relname='ux_file_objects_generation_artifact_identity'
 AND i.indrelid='public.xz_file_objects'::regclass AND i.indisunique AND i.indisvalid AND i.indisready
 AND pg_get_indexdef(i.indexrelid,1,true)='tenant_id' AND pg_get_indexdef(i.indexrelid,2,true)='business_type'
 AND pg_get_indexdef(i.indexrelid,3,true)='business_id' AND pg_get_indexdef(i.indexrelid,4,true)='original_name'
 AND i.indnatts=4 AND pg_get_expr(i.indpred,i.indrelid) = '((business_id IS NOT NULL) AND (status = ANY (ARRAY[''PENDING_UPLOAD''::text, ''ACTIVE''::text])))')
""",'STORAGE_CLAIM_SCHEMA_INVALID')
        storage = {'version':ASSET_STORAGE_VERSION,'scope':'TASK_LINKED_DB_METADATA_ONLY',
                   'schema':ASSET_STORAGE_SCHEMA,'counts':{k:len(v) for k,v in families.items()},'families':families,
                   'external_object_health':'UNPROVEN','historical_execution_attribution':'UNPROVEN'}
        snapshots[item['execution_id']] = {'version':ASSET_PARTIAL_VERSION,'scope':'CORE_FINANCIAL_ASSETS_DB_PARTIAL',
            'core_financial':snapshots[item['execution_id']], 'asset_storage':storage}
    return snapshots


def core_financial_assets_sha256(snapshot):
    """Never an approved snapshot_sha256, even when all supported rows match."""
    if not isinstance(snapshot,dict) or snapshot.get('version')!=ASSET_PARTIAL_VERSION or snapshot.get('scope')!='CORE_FINANCIAL_ASSETS_DB_PARTIAL':
        block('PARTIAL_SCOPE_INVALID')
    return hashlib.sha256(canonical(snapshot)).hexdigest()


def compare_core_financial_assets_in_transaction(cursor, entries, expected_partial_sha256):
    snapshots = project_core_financial_assets_in_transaction(cursor,entries)
    if not isinstance(expected_partial_sha256,dict) or set(expected_partial_sha256)!=set(snapshots):
        block('APPROVED_COUNT_INVALID')
    for eid,snapshot in snapshots.items():
        if core_financial_assets_sha256(snapshot)!=expected_partial_sha256[eid]:
            block('ASSET_PARTIAL_HASH_MISMATCH')
    return snapshots


def sample_core_financial_assets_read_only(connection,entries):
    """Fresh explicit read-only coherent sample; enrollment locking is NOT done."""
    try:
        if connection.closed:
            block('DB_QUERY_FAILED')
        if connection.autocommit is not True or connection.get_transaction_status()!=0:
            block('TRANSACTION_REQUIRED')
        cursor=connection.cursor()
        cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY')
    except SnapshotError:
        raise
    except Exception:
        block('DB_QUERY_FAILED')
    try:
        return project_core_financial_assets_in_transaction(cursor,entries)
    finally:
        try:
            cursor.execute('ROLLBACK')
            cursor.close()
        except Exception:
            block('DB_QUERY_FAILED')


CANONICAL_LIVE_VERSION = 'issue203-live-canonical-snapshot-sha256-v1'


def canonical_live_snapshot(core_data, financial_data, asset_data, entry):
    snapshot = {
        'version': CANONICAL_LIVE_VERSION,
        'scope': 'CANONICAL_LIVE_SNAPSHOT',
        'execution_id': entry['execution_id'],
        'task_id': entry['task_id'],
        'attempt': entry['attempt'],
        'generation': entry['generation'],
        'task_generation': entry.get('task_generation', entry['generation']),
        'core': core_data,
        'financial': financial_data,
        'asset_storage': asset_data,
    }
    if entry['generation'] is None:
        evidence = core_data.get('legacy_generation_evidence')
        if not isinstance(evidence, dict):
            block('LEGACY_MIGRATION_PROOF_INVALID')
        snapshot.update(task_execution_generation=None, legacy_generation_unverifiable=True,
                        generation_resolution_reason=_approval.LEGACY_GENERATION_REASON,
                        generation_resolution_evidence=evidence)
    return snapshot


def canonical_live_snapshot_sha256(snapshot):
    if (not isinstance(snapshot, dict) or
            snapshot.get('version') != CANONICAL_LIVE_VERSION or
            snapshot.get('scope') != 'CANONICAL_LIVE_SNAPSHOT'):
        block('CANONICAL_SCOPE_INVALID')
    return hashlib.sha256(canonical(snapshot)).hexdigest()


def project_canonical_live_snapshot_in_transaction(cursor, entries):
    raw_snapshots = project_core_financial_assets_in_transaction(cursor, entries)
    result = {}
    for item in entries:
        eid = item['execution_id']
        raw = raw_snapshots[eid]
        cf = raw['core_financial']
        snap = canonical_live_snapshot(cf['core'], cf['financial'], raw['asset_storage'], item)
        result[eid] = snap
    return result


def sample_canonical_live_read_only(connection, entries):
    """Fresh explicit read-only coherent sample combining core + financial + assets."""
    try:
        if connection.closed:
            block('DB_QUERY_FAILED')
        if connection.autocommit is not True or connection.get_transaction_status() != 0:
            block('TRANSACTION_REQUIRED')
        cursor = connection.cursor()
        cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY')
    except SnapshotError:
        raise
    except Exception:
        block('DB_QUERY_FAILED')
    try:
        return project_canonical_live_snapshot_in_transaction(cursor, entries)
    finally:
        try:
            cursor.execute('ROLLBACK')
            cursor.close()
        except Exception:
            block('DB_QUERY_FAILED')


def build_unsigned_candidate_in_transaction(cursor, entries, release_sha, key_id,
                                            not_before, expires_at):
    """Build a no-signature candidate from this same coherent DB snapshot."""
    now = transaction_clock(cursor)
    snapshots = project_canonical_live_snapshot_in_transaction(cursor, entries)
    return _approval.build_unsigned_candidate(
        entries, snapshots, release_sha, key_id, not_before, expires_at, now)


def sample_unsigned_candidate_read_only(connection, entries, release_sha, key_id,
                                        not_before, expires_at):
    """Sample and build an unsigned candidate in one explicit READ ONLY transaction."""
    try:
        if connection.closed:
            block('DB_QUERY_FAILED')
        if connection.autocommit is not True or connection.get_transaction_status() != 0:
            block('TRANSACTION_REQUIRED')
        cursor = connection.cursor()
        cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY')
    except SnapshotError:
        raise
    except Exception:
        block('DB_QUERY_FAILED')
    try:
        return build_unsigned_candidate_in_transaction(
            cursor, entries, release_sha, key_id, not_before, expires_at)
    finally:
        try:
            cursor.execute('ROLLBACK')
            cursor.close()
        except Exception:
            block('DB_QUERY_FAILED')


def validate_live_snapshot_in_transaction(cursor, raw_bytes, release_sha, operation,
                                          expected_manifest_sha256=None):
    """Verify approval signature and recompute canonical live snapshot_sha256 from DB.

    Fails closed if signature, authority, clock window, DB state, snapshot_sha256,
    or evidence_sha256 fails to match.
    """
    if not isinstance(raw_bytes, bytes) or len(raw_bytes) == 0:
        block('MANIFEST_BYTES_MISMATCH')
    if expected_manifest_sha256 is not None:
        if (not isinstance(expected_manifest_sha256, str) or
                not re.fullmatch('[0-9a-f]{64}', expected_manifest_sha256) or
                hashlib.sha256(raw_bytes).hexdigest() != expected_manifest_sha256):
            block('MANIFEST_BYTES_MISMATCH')
    now = transaction_clock(cursor)
    try:
        manifest = _approval.verify(raw_bytes, release_sha, operation, now)
    except Exception:
        block('APPROVAL_INVALID')

    entries = manifest.get('executions', [])
    if not isinstance(entries, list) or len(entries) == 0:
        block('APPROVED_COUNT_INVALID')

    live_snapshots = project_canonical_live_snapshot_in_transaction(cursor, entries)
    if len(live_snapshots) != len(entries):
        block('APPROVED_COUNT_INVALID')

    for item in entries:
        eid = item['execution_id']
        if eid not in live_snapshots:
            block('EXECUTION_MISSING')
        live_snap = live_snapshots[eid]
        live_sha = canonical_live_snapshot_sha256(live_snap)
        if live_sha != item['snapshot_sha256']:
            block('SNAPSHOT_SHA256_MISMATCH')

        evidence = item.get('evidence')
        if not isinstance(evidence, dict):
            block('EVIDENCE_INVALID')
        if evidence.get('approval_id') != item['approval_id']:
            block('EVIDENCE_APPROVAL_MISMATCH')
        if evidence.get('snapshot_sha256') != live_sha:
            block('EVIDENCE_SNAPSHOT_MISMATCH')
        if hashlib.sha256(canonical(evidence)).hexdigest() != item['evidence_sha256']:
            block('EVIDENCE_SHA256_MISMATCH')

    return manifest
