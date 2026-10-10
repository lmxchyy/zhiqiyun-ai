#!/usr/bin/env python3
"""Synthetic history policy regression, NOT an official packaged Proof.

Mandatory mode: HISTORICAL_DRAIN_REQUIRE_POSTGRES=1 with the isolated test DSN.
Fixtures live in rolled-back transactions; no shared history is rewritten.
"""
import copy
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import types
import unittest
from unittest import mock
import uuid

sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parent.parent


def load(name, relative):
    path = ROOT / relative
    module = types.ModuleType(name)
    module.__file__ = str(path)
    exec(compile(path.read_bytes(), str(path), 'exec'), module.__dict__)
    return module


drain = load('historical_drain', 'ops/verify-safe-drain.py')
capability = load('historical_capability', 'ops/verify-image-quarantine-capability.py')


def full_row(table, **values):
    """Complete source-schema fixture, with explicit operation-specific values."""
    defaults = {'text': '', 'varchar': '', 'bpchar': 'a' * 64, 'int8': 0, 'int4': 0,
                'numeric': 0, 'bool': False, 'jsonb': {}, 'timestamptz': '2002-01-01T00:00:00+00:00'}
    row = {f[0]: None if f[2] == 'YES' else copy.deepcopy(defaults[f[1]]) for f in capability.history_schema()[table]}
    for field in capability.history_schema()[table]:
        if field[1] == 'bpchar' and len(field) > 3:
            row[field[0]] = 'a' * field[3]
    if table == 'xz_generation_tasks':
        row.update(result_ids=[], error=None)
    row.update(values)
    return row


def history_item():
    owner = 'history-protocol-fixture'
    before = {table: [] for table in capability.HISTORY_TABLES}
    before['schema_migrations'] = [full_row('schema_migrations', filename='119-execution-generation-fencing.sql')]
    for index, name in enumerate(capability.HISTORY_OPS):
        task = owner + '-' + name
        execution = full_row('provider_executions', id=index + 1, task_id=task, attempt=1, provider='fixture', provider_model='fixture', capability='image', request_fingerprint=capability.history_request_fingerprint(task, 'canary' in name))
        if name == 'orphan':
            execution.update(status='failed', error_class='definitive_not_submitted', task_execution_generation=None, provider_request_id=None, result_metadata=None, next_check_at=None, created_at='2001-01-01T00:00:00+00:00', updated_at='2001-01-02T00:00:00+00:00')
        else:
            params = {'provider': 'fixture'}
            if not name.endswith('legacy'):
                params['generation_dispatch_mode'] = 'canary' if 'canary' in name else 'normal'
            if 'canary' in name:
                params['generation_async_canary'] = True
            worker = None if name == 'text-normal-legacy' else ('' if name == 'image-normal-legacy' else 'fixture-worker')
            kind = 'IMAGE_TO_IMAGE' if name.startswith('image') else 'TEXT_TO_IMAGE'
            raw = dict(id=task, userId=owner, type=kind, status='PROCESSING', model='', prompt='', billingAccountType='PERSONAL', params=params, updatedAt='2001-01-01T00:00:00Z')
            before['xz_generation_tasks'].append(full_row('xz_generation_tasks', id=task, user_id=owner, type=kind, status='PROCESSING', task_status='DISPATCHING', execution_generation=3, params=params, raw=raw, model='', prompt='', billing_account_type='PERSONAL', error=None, worker_id=worker, lease_until='2001-01-01T01:00:00+00:00', last_heartbeat_at='2001-01-01T00:00:00+00:00', updated_at=raw['updatedAt'], billing_status='RESERVED', result_ids=[]))
            execution.update(status='succeeded', error_class='provider_succeeded', task_execution_generation=2, result_metadata=[{'URL': 'data:image/png;base64,aGVsbG8=', 'ContentType': 'image/png'}])
        before['provider_executions'].append(execution)
    item = dict(owner=owner, role='api', before=before, after=copy.deepcopy(before), effects=[], response=dict(protocol=1, history_protocol=1, owner=owner, role='api', release_sha='a' * 40, phase='history', operations={name: 'read-only-rejected' for name in capability.HISTORY_OPS}))
    rehash(item)
    return item


def rehash(item):
    for phase in ('before', 'after'):
        item[phase + '_sha256'] = capability.sha(capability.canonical(item[phase]).encode('utf-8'))


def control_item():
    item = history_item()
    owner = item['owner']
    task = owner + '-normal-control'
    user = owner + '-control-user'
    account, reservation, lot = task + '-account', task + '-reservation', task + '-lot'
    params = {'generation_dispatch_mode': 'normal', '_generation_dispatch_owner': 'fixture-control-dispatch', 'billingReserved': True, 'billingReservationPointCost': 10, 'provider': task + '-channel'}
    raw = dict(id=task, userId=user, type='TEXT_TO_IMAGE', model='gpt-image-2', prompt='funded consumer control', status='PROCESSING', params=params, personalPointAccountId=account, personalPointReservationId=reservation, billingEngine='PERSONAL_LOT_V1', billingStatus='RESERVED', pointCost=10)
    before = item['before']
    before['xz_generation_tasks'].append(full_row('xz_generation_tasks', id=task, user_id=user, type='TEXT_TO_IMAGE', model=raw['model'], prompt=raw['prompt'], status='PROCESSING', task_status='DISPATCHING', execution_generation=1, worker_id='fixture-control-dispatch', params=params, raw=raw, point_cost=10, reserved_points=10, billing_status='RESERVED'))
    before['xz_point_accounts'] = [full_row('xz_point_accounts', id=account, user_id=user, available=100, frozen=10, raw={'totalGranted': 110})]
    before['xz_personal_point_reservations'] = [full_row('xz_personal_point_reservations', id=reservation, account_id=account, user_id=user, business_type='GENERATION_TASK', business_id=task, requested_points=10, reserved_points=10, status='RESERVED')]
    before['xz_personal_point_lots'] = [full_row('xz_personal_point_lots', id=lot, account_id=account, user_id=user, original_points=110, available_points=100, reserved_points=10, source_type='RECHARGE', status='ACTIVE')]
    after = copy.deepcopy(before)
    completed = after['xz_generation_tasks'][-1]
    completed.update(status='COMPLETED', task_status='SUCCEEDED', billing_status='CAPTURED', result_ids=[task + '-asset'], captured_points=10, released_points=0)
    completed['raw'].update(status='COMPLETED', resultIds=completed['result_ids'])
    after['xz_point_accounts'][0]['frozen'] = 0
    after['xz_personal_point_reservations'][0].update(reserved_points=0, captured_points=10)
    after['xz_personal_point_lots'][0].update(reserved_points=0, consumed_points=10)
    after['provider_executions'].append(full_row('provider_executions', id=10, task_id=task, status='succeeded', attempt=1, task_execution_generation=1, result_metadata=[{'URL': 'storage://' + task + '-file'}]))
    after['xz_assets'] = [full_row('xz_assets', id=task + '-asset', task_id=task, url='', metadata={'storageFileId': task + '-file'})]
    after['xz_file_objects'] = [full_row('xz_file_objects', file_id=task + '-file', business_id=task, status='ACTIVE', visibility='PRIVATE', file_size=5)]
    after['consumer_inbox'] = [full_row('consumer_inbox', id=1, consumer_name='generation-image-normal-worker', event_id=task + '-event', processed_at='2002-01-01T00:00:00Z', result='completed', metadata={'task_id': task})]
    after['xz_wallet_ledger'] = [full_row('xz_wallet_ledger', id=task + '-capture', task_id=task, entry_type='CAPTURE', points=10)]
    after['xz_personal_point_lot_movements'] = [full_row('xz_personal_point_lot_movements', id=task + '-movement', reservation_id=reservation, movement_type='CAPTURE', points=10)]
    item.update(after=after, effects=[{'method': 'POST', 'path': path, 'bytes': 10} for path in ('/consumer-provider', '/object')])
    item['response'].update(phase='history-control', operations={'normal-consumer': 'completed'})
    rehash(item)
    return item


def seed_orphan_storage(cur, owner, orphan):
    """Owned cycle/diamond graph plus unlinked file sharing only the config."""
    cur.execute("INSERT INTO xz_storage_configs(id,tenant_id,name,provider,endpoint,bucket) VALUES(%s,'tenant_default','history closure','s3','http://fixture-sink','fixture')", (owner + '-config',))
    for name in ('root', 'left', 'right', 'leaf', 'unlinked'):
        fid = owner + '-' + name
        cur.execute("INSERT INTO xz_file_objects(file_id,tenant_id,user_id,storage_config_id,provider,bucket,object_key,original_name,stored_name,mime_type,file_size,business_type,business_id,visibility,status) VALUES(%s,'tenant_default',%s,%s,'s3','fixture',%s,%s,%s,'image/png',5,'generation_result',%s,'PRIVATE','ACTIVE')", (fid, owner, owner + '-config', fid, fid, fid, orphan if name == 'root' else owner + '-unrelated'))
        cur.execute("INSERT INTO xz_storage_jobs(id,tenant_id,file_id,job_type,metadata) VALUES(%s,'tenant_default',%s,'fixture','{}')", (fid + '-job', fid))
        cur.execute("INSERT INTO xz_multipart_uploads(id,tenant_id,owner_user_id,file_id,provider_upload_id,object_key,file_name,total_size,part_size,total_parts,state,expires_at) VALUES(%s,'tenant_default',%s,%s,'fixture',%s,%s,5,5,1,'uploading',now()+interval '1 hour')", (fid + '-upload', owner, fid, fid, fid))
        cur.execute("INSERT INTO xz_multipart_upload_parts(upload_id,part_number,etag,size_bytes) VALUES(%s,1,'fixture',5)", (fid + '-upload',))
    for index, (source, target) in enumerate((('root', 'left'), ('root', 'right'), ('left', 'leaf'), ('right', 'leaf'), ('leaf', 'root'))):
        cur.execute("INSERT INTO xz_file_relations(id,tenant_id,source_file_id,target_file_id,relation_type) VALUES(%s,'tenant_default',%s,%s,'fixture')", (owner + '-relation-' + str(index), owner + '-' + source, owner + '-' + target))
    mutations = [
        ("UPDATE xz_storage_configs SET name='changed' WHERE id=%s", (owner + '-config',)),
        ("UPDATE xz_file_relations SET relation_type='changed' WHERE id=%s", (owner + '-relation-0',))]
    for name in ('root', 'left', 'right', 'leaf'):
        fid = owner + '-' + name
        mutations += [
            ("UPDATE xz_file_objects SET file_size=6 WHERE file_id=%s", (fid,)),
            ("UPDATE xz_storage_jobs SET metadata='{\"changed\":true}',status='RETRY' WHERE id=%s", (fid + '-job',)),
            ("UPDATE xz_multipart_uploads SET state='completed' WHERE id=%s", (fid + '-upload',)),
            ("UPDATE xz_multipart_upload_parts SET etag='changed' WHERE upload_id=%s", (fid + '-upload',))]
    return mutations


class HistoricalSourceTests(unittest.TestCase):
    def test_fixed_challenge_budgets_and_timeout_emits_no_evidence(self):
        self.assertEqual(capability.challenge_budget(True, True), 900)
        self.assertEqual(capability.challenge_budget(True, False), 900)
        self.assertEqual(capability.challenge_budget(False, True), 300)
        self.assertEqual(capability.challenge_budget(False, False), 180)
        original, argv = capability.image_identity, sys.argv
        def expired(docker, *args):
            remaining = docker.deadline - capability.time.monotonic()
            self.assertGreater(remaining, 299)
            self.assertLessEqual(remaining, 300)
            docker.deadline = capability.time.monotonic() - 1
            return docker.run(['version'])  # Refuses BEFORE any Docker process.
        try:
            capability.image_identity = expired
            with tempfile.TemporaryDirectory(prefix='historical-timeout-') as work:
                output = Path(work) / 'must-not-exist.json'
                sys.argv = ['capability', '--image', 'fixture', '--release-sha', 'a' * 40, '--output', str(output)]
                with self.assertRaisesRegex(capability.Refused, 'deadline exceeded'):
                    capability.main()
                self.assertFalse(output.exists())
        finally:
            capability.image_identity, sys.argv = original, argv

    def test_cleanup_restores_absolute_deadline_even_on_failure(self):
        for fail in (False, True):
            with tempfile.TemporaryDirectory(prefix='history-cleanup-deadline-') as work:
                docker = capability.Docker()
                docker.evidence_dir = work
                deadline = capability.time.monotonic() - 1  # cleanup must also work AFTER expiry
                docker.deadline = deadline
                fixture = capability.Fixture(docker)
                fixture.resources = [('container', 'unit-owned-container')]
                def inspect(kind, identity):
                    self.assertIsNone(docker.deadline)
                    return {'Id': identity, 'Config': {'Labels': {capability.LABEL: fixture.owner}}}
                def remove(args):
                    self.assertIsNone(docker.deadline)
                    if fail:
                        raise capability.Refused('forced owned cleanup failure')
                with mock.patch.object(docker, 'inspect', inspect), mock.patch.object(docker, 'run', remove):
                    if fail:
                        with self.assertRaisesRegex(capability.Refused, 'cleanup failed'):
                            fixture.cleanup()
                    else:
                        fixture.cleanup()
                self.assertEqual(docker.deadline, deadline)
                with self.assertRaisesRegex(capability.Refused, 'deadline exceeded'):
                    docker.run(['version'])  # expires before any Docker subprocess

    def test_standalone_unset_deadline_logging_does_not_change_budget(self):
        # Some diagnostic fixture callers have no aggregate deadline. Logging
        # must not invent one; mandatory attest always sets its fixed budget.
        docker = capability.Docker()
        fixture = capability.Fixture(docker)
        self.assertIsNone(docker.deadline)
        with mock.patch.object(docker, 'inspect', side_effect=capability.Refused('reached owned image inspection')):
            with self.assertRaisesRegex(capability.Refused, 'reached owned image inspection'):
                fixture.provision('unit-source-no-image')
        self.assertIsNone(docker.deadline)
        with mock.patch.object(docker, 'text', side_effect=capability.Refused('reached owned candidate creation')):
            with self.assertRaisesRegex(capability.Refused, 'reached owned candidate creation'):
                fixture.challenge('unit-source-no-image', capability.ROLES['api'], 'history')
        self.assertIsNone(docker.deadline)

    def test_whole_attestation_expiry_transitions_emit_no_evidence(self):
        # Isolated lifecycle doubles, NOT a packaged proof. Every case expires
        # deliberately; no successful attestation is returned or serialized.
        for transition in ('after-unavailable', 'between-history-roles', 'final-identity'):
            with self.subTest(transition=transition), tempfile.TemporaryDirectory(prefix='history-transition-') as work:
                clock = {'now': 0.0, 'cleanups': 0}
                clock_api = types.SimpleNamespace(monotonic=lambda: clock['now'], sleep=lambda n: None, time=lambda: 1)
                testcase = self
                class UnitDocker(capability.Docker):
                    def __init__(inner):
                        super().__init__()
                        inner.evidence_dir = work
                    def run(inner, args, **kwargs):
                        if inner.deadline is not None and clock['now'] >= inner.deadline:
                            return super().run(args, **kwargs)  # production pre-subprocess expiry check
                        return types.SimpleNamespace(returncode=0, stdout=b'')
                class UnitFixture(capability.Fixture):
                    def provision(inner, image):
                        testcase.assertEqual(inner.docker.deadline, 300.0)
                        if clock['now'] >= inner.docker.deadline:
                            inner.docker.run(['version'])
                        inner.samples = 0
                    def seed(inner, blocked):
                        inner.phase = 'blocked' if blocked else 'allowed'
                    def sql(inner, query):
                        return '1'  # source-only unavailable fault installation double
                    def snapshot(inner, tables=capability.TABLES):
                        inner.samples += 1
                        value = 'after' if inner.phase == 'allowed' and inner.samples > 1 else 'before'
                        return {table: [dict(stage=value)] for table in tables}
                    def effects(inner):
                        return [dict(path=path, bytes=1) for path in ('/object', '/provider', '/provider-video')] if inner.phase == 'allowed' else []
                    def seed_history(inner):
                        inner.phase = 'history'
                    def seed_history_control(inner):
                        inner.phase = 'history-control'
                    def challenge(inner, image, binary, phase):
                        testcase.assertEqual(inner.docker.deadline, 300.0)
                        inner.phase = phase
                        role = next(role for role, path in capability.ROLES.items() if path == binary)
                        if phase == 'blocked':
                            operations = {op: 'EXECUTION_QUARANTINED_READONLY' for op in capability.OPS}
                        elif phase == 'unavailable':
                            operations = {op: 'quarantine barrier unavailable' for op in capability.OPS}
                        else:
                            operations = {op: 'unit-lifecycle-only' for op in capability.OPS}
                        inner.diagnostic = ''
                        return 0, json.dumps(dict(owner=inner.owner, role=role, release_sha='a' * 40, phase=phase, operations=operations, nil_dependency_checks=6))
                    def cleanup(inner):
                        previous = inner.docker.deadline
                        super().cleanup()
                        testcase.assertEqual(inner.docker.deadline, previous)
                        testcase.assertEqual(previous, 300.0)
                        clock['cleanups'] += 1
                        point = {'after-unavailable': 10, 'between-history-roles': 11, 'final-identity': 12}[transition]
                        if clock['cleanups'] == point:
                            clock['now'] = 301.0
                def identity(docker, *args):
                    testcase.assertEqual(docker.deadline, 300.0)
                    if clock['now'] >= docker.deadline:
                        return docker.run(['version'])
                    return {'local_image_id': 'unit-nonbehavior-image'}
                output = Path(work) / 'must-not-exist.json'
                argv = ['capability', '--image', 'unit-nonbehavior-image', '--release-sha', 'a' * 40, '--output', str(output)]
                with mock.patch.object(capability, 'time', clock_api), mock.patch.object(capability, 'Docker', UnitDocker), mock.patch.object(capability, 'Fixture', UnitFixture), mock.patch.object(capability, 'image_identity', identity), mock.patch.object(capability, 'check_allowed'), mock.patch.object(capability, 'check_history_observation'), mock.patch.object(capability, 'check_history_control'), mock.patch.object(sys, 'argv', argv):
                    with self.assertRaisesRegex(capability.Refused, 'deadline exceeded'):
                        capability.main()
                self.assertFalse(output.exists())

    def test_nested_protocol_types_reject_boolean_and_float(self):
        row = history_item()
        capability.check_history_observation(row, 'a' * 40)
        for field in ('protocol', 'history_protocol'):
            for bad in (True, 1.0):
                mutation = copy.deepcopy(row)
                mutation['response'][field] = bad
                with self.assertRaises(capability.Refused):
                    capability.check_history_observation(mutation, 'a' * 40)
        for bad in (True, 1.0):
            with self.assertRaises(capability.Refused):
                capability.verify_history({'history': {'version': bad, 'observations': []}}, 'a' * 40)

    def test_control_protocol_types_reject_boolean_and_float(self):
        snapshot = {table: [] for table in capability.HISTORY_TABLES}
        digest = capability.sha(capability.canonical(snapshot).encode('utf-8'))
        item = dict(owner='fixture-owner', role='api', before=snapshot, after=snapshot, before_sha256=digest, after_sha256=digest,
                    response=dict(owner='fixture-owner', role='api', phase='history-control', release_sha='1' * 40, protocol=1, history_protocol=1, operations={'normal-consumer': 'completed'}))
        for field in ('protocol', 'history_protocol'):
            for bad in (True, 1.0):
                changed = json.loads(json.dumps(item))
                changed['response'][field] = bad
                with self.subTest(field=field, bad=repr(bad)), self.assertRaisesRegex(capability.Refused, 'control evidence incomplete'):
                    capability.check_history_control(changed, '1' * 40)

    def test_rehashed_success_orphan_field_deletion_and_types(self):
        baseline = history_item()
        capability.check_history_observation(baseline, 'a' * 40)
        for table, index in (('xz_generation_tasks', 0), ('provider_executions', 0), ('provider_executions', 8), ('schema_migrations', 0)):
            for field in baseline['before'][table][index]:
                bad = copy.deepcopy(baseline)
                del bad['before'][table][index][field]
                bad['after'] = copy.deepcopy(bad['before'])
                rehash(bad)
                with self.subTest(table=table, index=index, deleted=field), self.assertRaises(capability.Refused):
                    capability.check_history_observation(bad, 'a' * 40)
        for table, index, field, value in (
                ('xz_generation_tasks', 0, 'execution_generation', True),
                ('xz_generation_tasks', 0, 'execution_generation', 3.0),
                ('xz_generation_tasks', 0, 'point_cost', None),
                ('xz_generation_tasks', 0, 'captured_points', True),
                ('xz_generation_tasks', 0, 'lease_until', None),
                ('xz_generation_tasks', 0, 'raw', {}),
                ('xz_generation_tasks', 0, 'last_heartbeat_at', '2001-01-01'),
                ('provider_executions', 0, 'result_metadata', []),
                ('provider_executions', 0, 'result_metadata', [{'URL': 'truncated'}]),
                ('provider_executions', 0, 'id', 1.0),
                ('provider_executions', 8, 'task_execution_generation', 0),
                ('provider_executions', 8, 'next_check_at', ''),
                ('schema_migrations', 0, 'applied_at', '2002-01-01')):
            bad = copy.deepcopy(baseline)
            bad['before'][table][index][field] = value
            bad['after'] = copy.deepcopy(bad['before'])
            rehash(bad)
            with self.subTest(table=table, field=field, value=value), self.assertRaises(capability.Refused):
                capability.check_history_observation(bad, 'a' * 40)

    def test_rehashed_control_field_deletion_truncation_and_types(self):
        baseline = control_item()
        capability.check_history_control(baseline, 'a' * 40)
        for phase in ('before', 'after'):
            for table, rows in baseline[phase].items():
                for index, row in enumerate(rows):
                    for field in row:
                        bad = copy.deepcopy(baseline)
                        del bad[phase][table][index][field]
                        rehash(bad)
                        with self.subTest(phase=phase, table=table, deleted=field), self.assertRaises(capability.Refused):
                            capability.check_history_control(bad, 'a' * 40)
        for table, field, wrong in (('xz_generation_tasks', 'raw', {}), ('xz_generation_tasks', 'execution_generation', 1.0), ('xz_generation_tasks', 'params', {}), ('xz_point_accounts', 'frozen', False), ('xz_file_objects', 'file_size', 5.0), ('consumer_inbox', 'metadata', {}), ('consumer_inbox', 'processed_at', '2002-01-01'), ('xz_wallet_ledger', 'points', True)):
            bad = copy.deepcopy(baseline)
            bad['after'][table][-1][field] = wrong
            rehash(bad)
            with self.subTest(table=table, field=field, wrong=wrong), self.assertRaises(capability.Refused):
                capability.check_history_control(bad, 'a' * 40)

    def test_all_source_snapshot_table_row_types(self):
        for table in capability.HISTORY_TABLES:
            snapshot = {t: [] for t in capability.HISTORY_TABLES}
            snapshot[table] = [full_row(table)]
            capability.check_history_snapshot(snapshot)
            for field in snapshot[table][0]:
                bad = copy.deepcopy(snapshot)
                del bad[table][0][field]
                with self.subTest(table=table, field=field), self.assertRaises(capability.Refused):
                    capability.check_history_snapshot(bad)
            for descriptor in capability.history_schema()[table]:
                name, kind, nullable = descriptor[:3]
                wrong = [] if kind != 'jsonb' else float('nan')
                if kind in ('int8', 'int4', 'numeric'):
                    wrong = True
                bad = copy.deepcopy(snapshot)
                bad[table][0][name] = wrong
                with self.subTest(table=table, field=name, kind=kind), self.assertRaises(capability.Refused):
                    capability.check_history_snapshot(bad)

    def test_consumer_names_match_actual_source_constants(self):
        source = (ROOT / 'backend-go/internal/httpserver/generation_worker.go').read_text()
        names = re.findall(r'const generationImage(?:Canary|Normal)Consumer = "([^"]+)"', source)
        self.assertEqual(set(names), set(drain.HISTORY_IMAGE_CONSUMERS))

    def test_fixed_descriptors_and_sql_mutation_rejection(self):
        for ids in (None, list(range(1, 10)), [41, 55, 77]):
            sql = drain.transport.bind(drain.build_history_sql(ids), ())
            self.assertEqual(drain.transport.query_types(sql), (20, 25))
            with self.assertRaises(drain.transport.TransportError):
                drain.transport.query_types(sql.replace('lease_until > now()', 'false'))
        self.assertEqual(drain.transport.query_types(drain.HISTORY_KEYS_SQL), (19, 25, 25))

    def test_v1_no_history_stays_strict_and_invalid_protocol_rejects(self):
        self.assertFalse(capability.verify_history({}, 'a' * 40))
        for history in ({}, {'version': 2}, {'version': 1, 'observations': []}):
            with self.assertRaises(capability.Refused):
                capability.verify_history({'history': history}, 'a' * 40)

    def test_full_owner_evidence_is_not_an_eligibility_literal(self):
        self.assertNotIn('scheduler', drain.HISTORY_CTE)
        self.assertIn('to_jsonb(r)', drain.build_history_sql())
        self.assertIn("NOT (t.params ? '_generation_dispatch_owner')", drain.HISTORY_CTE)


class HistoricalPostgresTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        dsn = os.environ.get('XIANZHI_PROVIDER_EXECUTION_TEST_DATABASE_URL')
        if not dsn:
            if os.environ.get('HISTORICAL_DRAIN_REQUIRE_POSTGRES') == '1':
                raise RuntimeError('mandatory historical DB absent, no SKIP')
            raise unittest.SkipTest('optional disposable PostgreSQL absent; NOT certified')
        if dsn != 'postgres://codex:codex@127.0.0.1:55441/xianzhi_test?sslmode=disable':
            raise RuntimeError('refuse non-owned test DSN')
        import psycopg2
        cls.db = psycopg2.connect(dsn)

    @classmethod
    def tearDownClass(cls):
        cls.db.close()

    def setUp(self):
        self.owner = 'history-test-' + uuid.uuid4().hex
        self.task = self.owner + '-task'
        self.orphan = self.owner + '-orphan'
        self.cur = self.db.cursor()
        self.cur.execute("SET LOCAL statement_timeout='15s'")
        # The schema-file replay DB lacks the migration runner's own ledger.
        # Reproduce it only inside this rolled-back synthetic fixture.
        self.cur.execute('CREATE TABLE IF NOT EXISTS public.schema_migrations(filename TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())')
        self.cur.execute("INSERT INTO public.schema_migrations(filename) VALUES('119-execution-generation-fencing.sql') ON CONFLICT DO NOTHING")
        raw = dict(id=self.task, userId=self.owner, type='TEXT_TO_IMAGE', status='PROCESSING', model='', prompt='', billingAccountType='PERSONAL', params={})
        self.cur.execute("INSERT INTO xz_generation_tasks(id,user_id,type,model,prompt,status,task_status,execution_generation,params,raw,lease_until,last_heartbeat_at) VALUES(%s,%s,'TEXT_TO_IMAGE','','','PROCESSING','DISPATCHING',3,'{}',%s::jsonb,now()-interval '1 hour',now()-interval '2 hours')", (self.task, self.owner, json.dumps(raw)))
        self.cur.execute("INSERT INTO provider_executions(task_id,provider,provider_model,capability,attempt,status,error_class,request_fingerprint,task_execution_generation,result_metadata) VALUES(%s,'fixture','fixture','image',1,'succeeded','provider_succeeded',%s,2,%s::jsonb)", (self.task, 'a' * 64, json.dumps([{'URL': 'data:image/png;base64,aGVsbG8=', 'ContentType': 'image/png'}])))
        self.cur.execute("INSERT INTO provider_executions(task_id,provider,provider_model,capability,attempt,status,error_class,request_fingerprint,task_execution_generation,created_at,updated_at) VALUES(%s,'fixture','fixture','image',1,'failed','definitive_not_submitted',%s,NULL,'2001-01-01','2001-01-02')", (self.orphan, 'b' * 64))

    def tearDown(self):
        self.db.rollback()
        self.cur.close()

    def observation(self, historical=True):
        query = drain.build_history_sql() if historical else drain.SQL
        sql = drain.transport.bind(query, ())
        self.cur.execute(sql)
        native = self.cur.fetchone()
        # Real PostgreSQL fixed wire projection/decoder parity within this
        # rolled-back fixture transaction; full Docker CLI/Target stays CI-only.
        token = uuid.uuid4().hex
        types = drain.transport.query_types(sql)
        self.cur.execute(drain.transport.select_statement(sql, token, types))
        frame = json.dumps(self.cur.fetchone()[0]).encode('utf-8')
        self.assertEqual(drain.transport.decode_frame(frame, token, types), [native])
        return native

    def mutate(self, sql, parameters=()):
        self.cur.execute(sql, parameters)

    def test_owner_independent_positive_and_strict_default(self):
        baseline = self.observation()
        self.assertEqual(baseline[0], 0)
        self.assertGreaterEqual(self.observation(False)[0], 2)
        for worker in (None, '', 'fixture-arbitrary-worker'):
            self.mutate('UPDATE xz_generation_tasks SET worker_id=%s WHERE id=%s', (worker, self.task))
            self.assertEqual(self.observation()[0], 0)
        schema = dict(drain.live_snapshot.SCHEMA)
        for extra in (drain.live_snapshot.FINANCIAL_SCHEMA, drain.live_snapshot.ENTERPRISE_FINANCIAL_SCHEMA, drain.live_snapshot.ASSET_STORAGE_SCHEMA, drain.HISTORY_SCHEMA):
            schema.update(extra)
        drain.live_snapshot._schema(self.cur, schema)
        self.cur.execute(drain.HISTORY_KEYS_SQL)
        self.assertEqual(self.cur.fetchall(), drain.HISTORY_KEYS)

    def test_real_complete_history_snapshot_and_rehashed_truncation(self):
        # Execute the SAME seed/snapshot routines used by packaged verification
        # against the owned rolled-back DB; no hand-written abbreviated rows.
        class SQLFixture:
            owner = self.owner + '-real-snapshot'
            def sql(inner, query):
                self.cur.execute(query)
                return json.dumps(self.cur.fetchone()[0]) if self.cur.description else ''
        fixture = SQLFixture()
        capability.Fixture.seed_history(fixture)
        before = capability.Fixture.snapshot(fixture, capability.HISTORY_TABLES)
        item = history_item()
        item.update(owner=fixture.owner, before=before, after=copy.deepcopy(before))
        item['response']['owner'] = fixture.owner
        rehash(item)
        # Exclude this test's other newly owned policy rows from the snapshot;
        # seed_history itself is kept byte-for-byte identical to packaged SQL.
        for table, key, prefix in (('xz_generation_tasks', 'id', fixture.owner + '-'), ('provider_executions', 'task_id', fixture.owner + '-')):
            item['before'][table] = [row for row in before[table] if row[key].startswith(prefix)]
        item['after'] = copy.deepcopy(item['before'])
        rehash(item)
        capability.check_history_observation(item, 'a' * 40)
        for table, index in (('xz_generation_tasks', 0), ('provider_executions', 0), ('provider_executions', 8)):
            for key in item['before'][table][index]:
                bad = copy.deepcopy(item)
                del bad['before'][table][index][key]
                bad['after'] = copy.deepcopy(bad['before'])
                rehash(bad)
                with self.subTest(table=table, key=key), self.assertRaises(capability.Refused):
                    capability.check_history_observation(bad, 'a' * 40)

    def test_owned_history_control_failure_observations_survive_rollback(self):
        # Actual seed/snapshot SQL in NEW owned rolled-back fixtures, controlled
        # failing candidate callables only. These files are NONBEHAVIOR source
        # diagnostics, never packaged evidence or an accepted capability Proof.
        root = ROOT / '.evidence/drain-forwardfix/next/fix-1/failure-snapshots' / uuid.uuid4().hex
        identity = dict(local_image_id='unit-source-not-a-packaged-image', source_test_nonbehavior=True)
        for phase in ('history', 'history-control'):
            for failure in ('before', 'nonzero', 'validator', 'challenge', 'timeout', 'after', 'effects'):
                with self.subTest(phase=phase, failure=failure):
                    self.cur.execute('SAVEPOINT owned_failure_fixture')
                    testcase = self
                    class OwnedSQLFixture(capability.Fixture):
                        def sql(inner, query):
                            testcase.cur.execute(query)
                            return json.dumps(testcase.cur.fetchone()[0]) if testcase.cur.description else ''
                        def snapshot(inner, tables=capability.TABLES):
                            inner.snapshot_calls += 1
                            if ((failure == 'before' and inner.snapshot_calls == 1) or
                                    (failure == 'after' and inner.snapshot_calls == 2)):
                                raise capability.Refused('forced ' + failure + ' unavailable ' + inner.password)
                            return super().snapshot(tables)
                        def challenge(inner, image, binary, observed_phase):
                            saved = json.loads(Path(inner.observation_path).read_text(encoding='utf-8'))
                            testcase.assertIsNotNone(saved['before'])
                            testcase.assertIsNone(saved['after'])
                            testcase.assertEqual(saved['status'], 'incomplete')
                            tid = inner.owner + ('-normal-control' if phase == 'history-control' else '-text-normal-legacy')
                            testcase.cur.execute('UPDATE xz_generation_tasks SET worker_id=%s WHERE id=%s', ('forced-owned-drift', tid))
                            inner.diagnostic = 'forced source diagnostic ' + inner.password
                            if failure == 'challenge':
                                raise capability.Refused(inner.diagnostic)
                            if failure == 'timeout':
                                raise subprocess.TimeoutExpired(['unit-controlled-candidate'], 300, output=('captured timeout stdout ' + inner.password).encode('utf-8'))
                            return (0 if failure == 'validator' else 7), '{}'
                        def effects(inner):
                            if failure == 'effects':
                                raise capability.Refused('forced effects unavailable ' + inner.password)
                            return []
                    docker = capability.Docker()
                    docker.evidence_dir = str(root / (phase + '-' + failure))
                    deadline = capability.time.monotonic() + 300
                    docker.deadline = deadline
                    fixture = OwnedSQLFixture(docker)
                    fixture.snapshot_calls = 0
                    try:
                        fixture.begin_history_observation(identity, 'api', phase)
                        fixture.seed_history()
                        if phase == 'history-control':
                            fixture.seed_history_control()
                        try:
                            before, after, effects, code, output = fixture.observe_history_challenge(identity['local_image_id'], capability.ROLES['api'], phase)
                            if code:
                                raise capability.Refused('forced candidate nonzero ' + fixture.diagnostic)
                            item = dict(owner=fixture.owner, role='api', before=before, after=after, effects=effects, response=json.loads(output), before_sha256=capability.sha(capability.canonical(before).encode('utf-8')), after_sha256=capability.sha(capability.canonical(after).encode('utf-8')))
                            checker = capability.check_history_observation if phase == 'history' else capability.check_history_control
                            checker(item, 'a' * 40)  # deliberately rejects, never emits a PASS
                        except (capability.Refused, subprocess.TimeoutExpired) as error:
                            fixture.finish_history_observation(error)
                        else:
                            self.fail('forced failure unexpectedly accepted')
                    finally:
                        fixture.cleanup()
                        self.cur.execute('ROLLBACK TO SAVEPOINT owned_failure_fixture')
                    self.assertEqual(docker.deadline, deadline)
                    text = Path(fixture.observation_path).read_text(encoding='utf-8')
                    saved = json.loads(text)
                    self.assertTrue(saved['fixture_nonofficial'])
                    self.assertTrue(saved['image_identity']['source_test_nonbehavior'])
                    self.assertEqual(saved['owner_label'], capability.LABEL)
                    self.assertEqual(saved['owner'], fixture.owner)
                    self.assertEqual(saved['status'], 'failed')
                    self.assertNotIn(fixture.password, text)
                    self.assertIn('[redacted]', text)
                    if failure == 'before':
                        self.assertIsNone(saved['before'])
                        self.assertIn('before', saved['unavailable'])
                    else:
                        self.assertIsNotNone(saved['before'])
                    if failure in ('before', 'after'):
                        self.assertIsNone(saved['after'])
                        self.assertIn('after', saved['unavailable'])
                    else:
                        self.assertIsNotNone(saved['after'])
                        self.assertNotEqual(saved['before'], saved['after'])
                    if failure in ('before', 'effects'):
                        self.assertIsNone(saved['effects'])
                        self.assertIn('effects', saved['unavailable'])
                    else:
                        self.assertEqual(saved['effects'], [])
                    if failure in ('before', 'challenge'):
                        self.assertIsNone(saved['stdout'])
                        self.assertIn('stdout', saved['unavailable'])
                    else:
                        self.assertIsNotNone(saved['stdout'])
                    if failure == 'timeout':
                        self.assertIn('captured timeout stdout [redacted]', saved['stdout'])
                    self.assertEqual(saved['exit_code'], None if failure in ('before', 'challenge', 'timeout') else 0 if failure == 'validator' else 7)
                    self.assertEqual(saved['unavailable']['validation'], 'not accepted')
                    self.cur.execute('SELECT count(*) FROM xz_generation_tasks WHERE id LIKE %s', (fixture.owner + '-%',))
                    self.assertEqual(self.cur.fetchone()[0], 0)  # fixture gone, diagnostics retained

    def test_raw_runtime_projection_parity_blocks(self):
        self.assertEqual(self.observation()[0], 0)
        for tag in ('id', 'userId', 'status', 'type', 'model', 'prompt', 'params', 'billingAccountType'):
            for value in (None, {'drift': True}, 'COMPLETED' if tag == 'status' else 'drift'):
                self.cur.execute('SAVEPOINT raw_parity')
                self.mutate('UPDATE xz_generation_tasks SET raw=jsonb_set(raw,ARRAY[%s],%s::jsonb) WHERE id=%s', (tag, json.dumps(value), self.task))
                self.assertGreater(self.observation()[0], 0, (tag, value))
                self.cur.execute('ROLLBACK TO SAVEPOINT raw_parity')
        for tag, column in drain.HISTORY_RAW_STRINGS:
            self.cur.execute('SAVEPOINT raw_missing')
            self.mutate('UPDATE xz_generation_tasks SET raw=raw-%s WHERE id=%s', (tag, self.task))
            self.assertGreater(self.observation()[0], 0, tag)
            self.cur.execute('ROLLBACK TO SAVEPOINT raw_missing')
        for tag, column in drain.HISTORY_RAW_OPTIONAL_STRINGS:
            self.cur.execute('SAVEPOINT raw_optional')
            assignment = "=''" if column == 'billing_account_type' else '=NULL'
            self.mutate('UPDATE xz_generation_tasks SET ' + column + assignment + ' WHERE id=%s', (self.task,))
            for raw_value in ({}, {tag: ''}):
                self.mutate('UPDATE xz_generation_tasks SET raw=(raw-%s)||%s::jsonb WHERE id=%s', (tag, json.dumps(raw_value), self.task))
                self.assertEqual(self.observation()[0], 0)
            self.mutate('UPDATE xz_generation_tasks SET raw=raw||jsonb_build_object(%s,NULL) WHERE id=%s', (tag, self.task))
            self.assertGreater(self.observation()[0], 0)
            self.cur.execute('ROLLBACK TO SAVEPOINT raw_optional')
        for alias in ('Status', 'ID', 'Params', 'billingaccounttype'):
            self.cur.execute('SAVEPOINT raw_alias')
            self.mutate('UPDATE xz_generation_tasks SET raw=raw||jsonb_build_object(%s,\'drift\') WHERE id=%s', (alias, self.task))
            self.assertGreater(self.observation()[0], 0, alias)
            self.cur.execute('ROLLBACK TO SAVEPOINT raw_alias')
        # These raw fencing/financial copies ARE overridden by the column scan.
        self.mutate("UPDATE xz_generation_tasks SET raw=raw||'{\"executionGeneration\":0,\"workerId\":\"obsolete\",\"leaseUntil\":\"2099-01-01T00:00:00Z\",\"taskStatus\":\"SUCCEEDED\",\"capturedPoints\":999}'::jsonb WHERE id=%s", (self.task,))
        self.assertEqual(self.observation()[0], 0)
        self.mutate("UPDATE xz_generation_tasks SET raw='{}' WHERE id=%s", (self.task,))
        self.assertGreater(self.observation()[0], 0)

    def test_orphan_entire_linked_storage_cycle_diamond_drift(self):
        mutations = seed_orphan_storage(self.cur, self.owner, self.orphan)
        before = self.observation()
        self.assertEqual(before[0], 0)
        for sql, parameters in mutations:
            with self.subTest(sql=sql, parameters=parameters):
                self.cur.execute('SAVEPOINT storage_drift')
                self.mutate(sql, parameters)
                after = self.observation()
                self.assertEqual(before[0], after[0])
                self.assertNotEqual(before[1], after[1])
                self.cur.execute('ROLLBACK TO SAVEPOINT storage_drift')
        self.mutate("UPDATE xz_storage_jobs SET status='COMPLETED' WHERE id=%s", (self.owner + '-unlinked-job',))
        self.mutate('UPDATE xz_file_objects SET file_size=6 WHERE file_id=%s', (self.owner + '-unlinked',))
        self.assertEqual(before, self.observation())

    def test_shape_mutations_block(self):
        cases = [
            ("UPDATE xz_generation_tasks SET lease_until=now()+interval '1 hour' WHERE id=%s", self.task),
            ("UPDATE xz_generation_tasks SET type='TEXT_TO_VIDEO' WHERE id=%s", self.task),
            ("UPDATE xz_generation_tasks SET task_status='RUNNING' WHERE id=%s", self.task),
            ("UPDATE xz_generation_tasks SET params='{\"generation_dispatch_mode\":\"normal\"}' WHERE id=%s", self.task),
            ("UPDATE xz_generation_tasks SET params='{\"generation_async_canary\":false}' WHERE id=%s", self.task),
            ("UPDATE xz_generation_tasks SET params='{\"_generation_fair_scheduled\":false}' WHERE id=%s", self.task),
            ("UPDATE xz_generation_tasks SET params='{\"_generation_dispatch_owner\":\"\"}' WHERE id=%s", self.task),
            ("UPDATE provider_executions SET status='failed' WHERE task_id=%s", self.task),
            ("UPDATE provider_executions SET result_metadata='[{}]' WHERE task_id=%s", self.task),
            ("UPDATE provider_executions SET result_metadata='[]' WHERE task_id=%s", self.task),
            ("UPDATE provider_executions SET provider_request_id='request' WHERE task_id=%s", self.orphan),
            ("UPDATE provider_executions SET next_check_at=now() WHERE task_id=%s", self.orphan),
            ("UPDATE provider_executions SET result_metadata='[]' WHERE task_id=%s", self.orphan),
            ("UPDATE provider_executions SET task_execution_generation=1 WHERE task_id=%s", self.orphan),
            ("UPDATE provider_executions SET created_at=now(),updated_at=now() WHERE task_id=%s", self.orphan),
        ]
        for generation in ('NULL', '-1', '0', '3', '4'):
            cases.append(('UPDATE provider_executions SET task_execution_generation=' + generation + ' WHERE task_id=%s', self.task))
        for state in ('prepared', 'submitting', 'submitted', 'processing', 'unknown'):
            cases.append(("UPDATE provider_executions SET status='" + state + "' WHERE task_id=%s", self.task))
        for sql, task in cases:
            with self.subTest(sql=sql):
                self.cur.execute('SAVEPOINT mutation')
                self.mutate(sql, (task,))
                self.assertGreater(self.observation()[0], 0)
                self.cur.execute('ROLLBACK TO SAVEPOINT mutation')
        self.cur.execute('SAVEPOINT multiple_attempt')
        self.mutate("INSERT INTO provider_executions(task_id,provider,provider_model,capability,attempt,status,request_fingerprint,task_execution_generation) VALUES(%s,'fixture','fixture','image',2,'prepared',%s,3)", (self.task, 'd' * 64))
        self.assertGreater(self.observation()[0], 0)
        self.cur.execute('ROLLBACK TO SAVEPOINT multiple_attempt')
        self.mutate("INSERT INTO outbox_events(event_id,aggregate_type,aggregate_id,event_type,payload) VALUES(%s,'generation_task',%s,'fixture','{}')", (self.owner, self.task))
        self.assertGreater(self.observation()[0], 0)

    def test_same_count_history_drift_rejects(self):
        before = self.observation()
        self.mutate('UPDATE xz_generation_tasks SET worker_id=%s WHERE id=%s', ('changed-owner', self.task))
        after = self.observation()
        self.assertEqual(before[0], after[0])
        self.assertNotEqual(before[1], after[1])
        old = drain.observe
        observations = iter([(before, ['api', 'worker']), (after, ['api', 'worker'])])
        drain.observe = lambda *args, **kwargs: next(observations)
        class SyntheticTarget:
            def history_enabled(self):
                return True
        try:
            with self.assertRaisesRegex(drain.GateError, 'evidence changed'):
                drain.verify('synthetic-compose', 'synthetic-env', 0, target=SyntheticTarget(), pause=lambda n: None)
        finally:
            drain.observe = old

    def test_same_count_unbound_pending_history_inbox_remains_blocked(self):
        normal = self.owner + '-normal'
        self.mutate("INSERT INTO xz_generation_tasks(id,type,status,task_status,params,raw) VALUES(%s,'TEXT_TO_IMAGE','PROCESSING','RUNNING','{}','{}')", (normal,))
        before = self.observation()
        self.assertEqual(before[0], 1)
        # Real ClaimTx shape: no metadata and no outbox. Swap one active normal
        # task for a pending historical envelope; count-only equality is unsafe.
        event = self.task + '-unbound-envelope'
        self.mutate('INSERT INTO consumer_inbox(consumer_name,event_id,processed_at) VALUES(%s,%s,NULL)', (drain.HISTORY_IMAGE_CONSUMERS[1], event))
        self.mutate("UPDATE xz_generation_tasks SET status='COMPLETED',task_status='SUCCEEDED' WHERE id=%s", (normal,))
        after = self.observation()
        self.assertEqual(before, after)
        old = drain.observe
        drain.observe = lambda *args, **kwargs: (after, ['api', 'worker'])
        class SyntheticTarget:
            def history_enabled(self):
                return True
        try:
            with self.assertRaisesRegex(drain.GateError, 'in-flight operations'):
                drain.verify('synthetic-compose', 'synthetic-env', 0, target=SyntheticTarget(), pause=lambda n: None)
        finally:
            drain.observe = old

    def test_linked_pending_inbox_same_count_digest_drift(self):
        event = self.owner + '-linked'
        self.mutate("INSERT INTO consumer_inbox(consumer_name,event_id,metadata) VALUES('generation-image-normal-worker',%s,%s::jsonb)", (event, json.dumps({'task_id': self.task})))
        before = self.observation()
        self.mutate("UPDATE consumer_inbox SET metadata=metadata||'{\"changed\":true}'::jsonb WHERE event_id=%s", (event,))
        after = self.observation()
        self.assertEqual(before[0], 1)
        self.assertEqual(before[0], after[0])
        self.assertNotEqual(before[1], after[1])

    def test_unrelated_normal_pending_completion_can_drain(self):
        before = self.observation()
        event = self.owner + '-normal-pending'
        self.mutate('INSERT INTO consumer_inbox(consumer_name,event_id) VALUES(%s,%s)', (drain.HISTORY_IMAGE_CONSUMERS[0], event))
        active = self.observation()
        self.assertEqual(active[0], 1)
        self.assertEqual(before[1], active[1])
        self.mutate("UPDATE consumer_inbox SET processed_at=now(),result='completed',metadata='{\"task_id\":\"unrelated-normal\"}' WHERE event_id=%s", (event,))
        after = self.observation()
        self.assertEqual(before, after)
        old = drain.observe
        observations = iter([(active, ['api', 'worker']), (after, ['api', 'worker']), (after, ['api', 'worker'])])
        drain.observe = lambda *args, **kwargs: next(observations)
        class SyntheticTarget:
            def history_enabled(self):
                return True
        try:
            drain.verify('synthetic-compose', 'synthetic-env', 10, target=SyntheticTarget(), pause=lambda n: None)
        finally:
            drain.observe = old

    def test_unrelated_normal_inflight_can_finish_and_drain(self):
        normal = self.owner + '-normal'
        before = self.observation()
        self.mutate("INSERT INTO xz_generation_tasks(id,user_id,type,status,task_status,execution_generation,worker_id,lease_until,params,raw) VALUES(%s,%s,'TEXT_TO_IMAGE','PROCESSING','RUNNING',1,'valid-normal-worker',now()+interval '1 hour','{}','{}')", (normal, normal))
        self.mutate("INSERT INTO provider_executions(task_id,provider,provider_model,capability,attempt,status,request_fingerprint,task_execution_generation) VALUES(%s,'fixture','fixture','image',1,'processing',%s,1)", (normal, 'c' * 64))
        active = self.observation()
        self.assertGreater(active[0], 0)
        self.assertEqual(before[1], active[1])
        self.mutate("UPDATE provider_executions SET status='succeeded',result_metadata='[{\"URL\":\"fixture\"}]' WHERE task_id=%s", (normal,))
        self.mutate("UPDATE xz_generation_tasks SET status='COMPLETED',task_status='SUCCEEDED',lease_until=NULL,worker_id=NULL,billing_status='CAPTURED' WHERE id=%s", (normal,))
        after = self.observation()
        self.assertEqual(before, after)
        old = drain.observe
        observations = iter([(active, ['api', 'worker']), (after, ['api', 'worker']), (after, ['api', 'worker'])])
        drain.observe = lambda *args, **kwargs: next(observations)
        class SyntheticTarget:
            def history_enabled(self):
                return True
        try:
            drain.verify('synthetic-compose', 'synthetic-env', 30, target=SyntheticTarget(), pause=lambda n: None)
        finally:
            drain.observe = old


if __name__ == '__main__' and len(sys.argv) == 3 and sys.argv[1] == '--assert-go':
    required = {'TestHistoricalImageSucceededExecutionFencesPostgres',
                'TestHistoricalImageSucceededCompatibilityPostgres',
                'TestHistoricalImageLiveOwnerAndUnknownControlsPostgres',
                'TestHistoricalImageConsumerTransportPostgres'}
    events = [json.loads(line) for line in Path(sys.argv[2]).read_text().splitlines()]
    passed = {e.get('Test') for e in events if e.get('Action') == 'pass'}
    failures = [e for e in events if e.get('Action') in ('skip', 'fail')]
    if failures or not required.issubset(passed):
        raise RuntimeError('required historical Go integration coverage absent/failed/skipped')
    print('HISTORICAL_GO_RESULT top_level=4 skips=0')
    sys.exit(0)

if __name__ == '__main__':
    result = unittest.TextTestRunner(verbosity=2).run(unittest.defaultTestLoader.loadTestsFromModule(sys.modules[__name__]))
    print('HISTORICAL_RESULT tests=%d failures=%d errors=%d skipped=%d' % (result.testsRun, len(result.failures), len(result.errors), len(result.skipped)))
    required = os.environ.get('HISTORICAL_DRAIN_REQUIRE_POSTGRES') == '1'
    sys.exit(0 if result.wasSuccessful() and (not required or (result.testsRun >= 24 and not result.skipped)) else 1)
