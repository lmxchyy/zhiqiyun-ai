#!/usr/bin/env python3
"""Issue203 fixed-source snapshot/enrollment transport (Python 3.6).

One psql process is one backend/transaction. This is not a general DB driver:
only the SELECTs, settings and enrollment INSERT in the protected callers are
supported. No credentials or raw subprocess errors are returned to callers.
"""
import datetime
import decimal
import hashlib
import json
import os
import queue
import re
import shlex
import subprocess
import threading
import types
import uuid

MAX_BYTES = 32 * 1024 * 1024
MAX_ROWS = 10001
TIMEOUT = 30


class TransportError(Exception):
    pass


def block():
    raise TransportError('QUARANTINE_DB_TRANSPORT_REJECTED')


def command(args):
    if os.name == 'nt':
        return ['C:/Program Files/Git/bin/bash.exe', '-c', ' '.join(shlex.quote(a) for a in args)]
    return args


def run(args, env=None):
    try:
        result = subprocess.run(command(args), stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                                timeout=TIMEOUT, env=env)
        if result.returncode or len(result.stdout) > MAX_BYTES:
            block()
        return result.stdout.decode('utf-8').strip()
    except Exception:
        block()


def digest_file(path):
    with open(path, 'rb') as stream:
        return hashlib.sha256(stream.read()).hexdigest()


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':')).encode('utf-8')


def binding(compose, env, image_reference=None):
    """Also used by Prestage; only hashes/identity, never rendered secrets persist."""
    try:
        compose, env = os.path.realpath(compose), os.path.realpath(env)
        if not os.path.getsize(compose) or not os.path.getsize(env):
            block()
        cmd = ['docker', 'compose', '-f', compose, '--env-file', env]
        render_env = dict(os.environ)
        if image_reference is not None:
            # Same approved image override as verify-prestage-proof.sh; never
            # override DB credentials/endpoints supplied by the bound env file.
            render_env['XIANZHI_IMAGE_REFERENCE'] = image_reference
        config = json.loads(run(cmd + ['config', '--format', 'json'], env=render_env))
        service = config['services']['postgres']
        project = config['name']
        expected_env = service['environment']
        for key in ('POSTGRES_USER', 'POSTGRES_DB', 'POSTGRES_PASSWORD'):
            if not isinstance(expected_env.get(key), str) or not expected_env[key]:
                block()
        # libpq treats dbname strings containing '=' or URI prefixes as a new
        # connection specification, which could override the explicit socket.
        for key in ('POSTGRES_USER', 'POSTGRES_DB'):
            if not re.fullmatch(r'[A-Za-z0-9_][A-Za-z0-9_.-]{0,62}', expected_env[key]):
                block()
        ids = run(cmd + ['ps', '--all', '-q', 'postgres']).splitlines()
        if len(ids) != 1 or not re.fullmatch('[0-9a-f]{64}', ids[0]):
            block()
        info = json.loads(run(['docker', 'inspect', ids[0]]))[0]
        labels = info['Config']['Labels']
        actual_env = dict(item.split('=', 1) for item in info['Config']['Env'])
        if (info['Id'] != ids[0] or info['State']['Running'] is not True or
                info['State'].get('Paused') or info['State'].get('Restarting') or
                labels.get('com.docker.compose.project') != project or
                labels.get('com.docker.compose.service') != 'postgres' or
                labels.get('com.docker.compose.oneoff', '').lower() != 'false' or
                info['Config']['Image'] != service['image'] or
                any(actual_env.get(k) != expected_env[k] for k in ('POSTGRES_USER', 'POSTGRES_DB', 'POSTGRES_PASSWORD'))):
            block()
        # A compose project must not conceal a second postgres container.
        all_ids = run(['docker', 'ps', '-aq', '--no-trunc', '--filter',
                      'label=com.docker.compose.project=' + project, '--filter',
                      'label=com.docker.compose.service=postgres']).splitlines()
        if all_ids != ids:
            block()
        return dict(compose_path=compose, env_path=env, compose_hash=digest_file(compose),
                    env_hash=digest_file(env), config_hash=hashlib.sha256(canonical(config)).hexdigest(),
                    project=project, container_id=ids[0], image_id=info['Image'],
                    started_at=info['State']['StartedAt'], restart_count=info['RestartCount'])
    except Exception:
        block()


class Target:
    def __init__(self, compose, env, proof_path, release_sha):
        try:
            before = digest_file(proof_path)
            root = os.path.dirname(os.path.dirname(os.path.realpath(__file__)))
            run(['bash', os.path.join(root, 'ops/verify-prestage-proof.sh'),
                 os.path.realpath(proof_path), release_sha, os.path.realpath(compose), os.path.realpath(env)])
            with open(proof_path, 'r', encoding='utf-8') as stream:
                proof = json.load(stream)
            if before != digest_file(proof_path):
                block()
            self.proof_path, self.proof_sha256, self.proof = proof_path, before, proof
            self.expected = proof['postgres_transport_binding']
            self.image_reference = proof['image_reference']
            self.compose, self.env = compose, env
            self.check()
        except Exception:
            block()

    def check(self):
        if binding(self.compose, self.env, self.image_reference) != self.expected:
            block()

    def history_enabled(self):
        # This object was constructed only AFTER official signed Proof/source
        # verification. Never accept a caller boolean, image tag or synthetic
        # receipt as authority to subtract historical blockers.
        try:
            if digest_file(self.proof_path) != self.proof_sha256:
                block()
            root = os.path.dirname(os.path.dirname(os.path.realpath(__file__)))
            for name, expected in self.proof['deploy_scripts_hash'].items():
                if digest_file(os.path.join(root, name)) != expected:
                    block()
            expiry = self.proof['expires_at']
            match = re.fullmatch(r'(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d)(?:\.(\d{1,6}))?(?:Z|\+00:00)', expiry)
            if match is None:
                block()
            end = datetime.datetime.strptime(match.group(1), '%Y-%m-%dT%H:%M:%S').replace(tzinfo=datetime.timezone.utc, microsecond=int((match.group(2) or '').ljust(6, '0')))
            if end <= datetime.datetime.now(datetime.timezone.utc):
                block()
            path = os.path.join(root, 'ops/verify-image-quarantine-capability.py')
            capability = types.ModuleType('historical_capability')
            capability.__file__ = path
            with open(path, 'rb') as stream:
                exec(compile(stream.read(), path, 'exec'), capability.__dict__)
            evidence = self.proof['runtime_capability']
            capability.verify(evidence, self.image_reference, self.proof['git_sha'], evidence['policy'])
            return capability.verify_history(evidence, self.proof['git_sha'])
        except Exception:
            block()

    def connect(self):
        self.check()
        return Connection(self)


def parameter(value):
    # Data becomes only hex digits or validated numeric tokens, never SQL syntax.
    # Empty arrays use an unknown literal, allowing ANY's left operand to infer
    # text[]/bigint[] exactly as psycopg2's '{}' adaptation does.
    if value is None:
        return 'NULL'
    if type(value) is int:
        if not -9223372036854775808 <= value <= 9223372036854775807:
            block()
        return str(value)
    if type(value) is decimal.Decimal:
        if not value.is_finite():
            block()
        return str(value) + '::numeric'
    if type(value) is str:
        if '\x00' in value or len(value.encode('utf-8')) > 1048576:
            block()
        return "convert_from(decode('" + value.encode('utf-8').hex() + "','hex'),'UTF8')"
    if type(value) is list:
        if len(value) > MAX_ROWS or any(type(v) not in (int, str) for v in value):
            block()
        if not value:
            return "'{}'"
        if len(set(type(v) for v in value)) != 1:
            block()
        return 'ARRAY[' + ','.join(parameter(v) for v in value) + ']'
    block()


def bind(sql, parameters):
    if type(sql) is not str or type(parameters) not in (tuple, list):
        block()
    # Protected callers use only %s/%% (including literal LIKE %%). Unknown
    # placeholders, mismatched arity and psql meta-commands fail closed.
    tokens = re.split(r'(%s|%%)', sql)
    if (sum(t == '%s' for t in tokens) != len(parameters) or
            any('%' in t for t in tokens if t not in ('%s', '%%'))):
        block()
    values = iter(parameters)
    output = ''.join(parameter(next(values)) if t == '%s' else '%' if t == '%%' else t for t in tokens)
    if '\\' in output or '\x00' in output:
        block()
    return output.strip().rstrip(';')


def query_types(sql):
    text = sql.strip().rstrip(';')
    # Exactly one variable fragment exists in the fixed drain-count source.
    # Other SQL changes (including column names/expressions) change the digest.
    text = re.sub(r' AND e\.id NOT IN \([0-9]+(?:,[0-9]+)*\)',
                  ' AND e.id NOT IN (<execution_ids>)', text)
    expected = QUERY_TYPES.get(hashlib.sha256(text.encode('utf-8')).hexdigest())
    if expected is None:
        expected = QUERY_TYPES.get(hashlib.sha256(text.replace('%%', '%').encode('utf-8')).hexdigest())
    if expected is None:
        block()
    return expected


def select_statement(text, token, expected):
    """A positional descriptor exists even for zero rows, not first-row inference.

    Each cell is its own JSON *text*. Scalar numeric decoding therefore retains
    precision/Decimal(0), without changing native JSON nested float semantics.
    PostgreSQL supplies actual base type OIDs, checked against reviewed source.
    """
    if not expected or any(oid not in CELL_TYPES for oid in expected):
        block()
    names = ['c%d' % i for i in range(len(expected))]
    cells = ','.join('to_json(q.' + name + ')::text' for name in names)
    oids = ','.join("(SELECT (CASE WHEN typbasetype=0 THEN oid ELSE typbasetype END)::int "
                    "FROM pg_catalog.pg_type WHERE oid=pg_typeof(q." + name + ')::oid)' for name in names)
    return ("WITH transport_rows AS MATERIALIZED (SELECT * FROM (" + text + ") source_rows LIMIT 10002), "
            "transport_meta AS (SELECT (SELECT count(*) FROM json_object_keys(row_to_json(ROW(q.*)))) AS columns, "
            "json_build_array(" + oids + ") AS types FROM (SELECT 1) seed LEFT JOIN transport_rows q(" + ','.join(names) + ") ON true LIMIT 1) "
            "SELECT json_build_object('token','" + token + "','count',(SELECT count(*) FROM transport_rows),"
            "'columns',columns,'types',types,'rows',coalesce((SELECT json_agg(json_build_array(" + cells + ")) "
            "FROM transport_rows q(" + ','.join(names) + ")), '[]'::json)) FROM transport_meta;\n")


CELL_TYPES = frozenset((16, 18, 19, 20, 21, 23, 25, 1042, 1043, 1184, 1700, 114, 3802, 1009, 1016))


def decode_cell(raw, oid):
    if raw is None:
        return None
    if type(raw) is not str or oid not in CELL_TYPES:
        block()
    value = json.loads(raw, parse_float=decimal.Decimal if oid == 1700 else float,
                       parse_constant=lambda _: block())
    if oid == 1700:
        if type(value) not in (int, decimal.Decimal):
            block()
        value = decimal.Decimal(value)
        if not value.is_finite():
            block()
    elif oid in (20, 21, 23):
        bits = {20: 64, 21: 16, 23: 32}[oid]
        if type(value) is not int or not -(2 ** (bits - 1)) <= value < 2 ** (bits - 1):
            block()
    elif oid == 16:
        if type(value) is not bool:
            block()
    elif oid == 1184:
        if type(value) is not str or not re.fullmatch(r'\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,6})?\+00:00', value):
            block()
        value = value[:-6]
        value = datetime.datetime.strptime(value, '%Y-%m-%dT%H:%M:%S.%f' if '.' in value else '%Y-%m-%dT%H:%M:%S').replace(tzinfo=datetime.timezone.utc)
    elif oid in (18, 19, 25, 1042, 1043):
        if type(value) is not str:
            block()
    elif oid in (1009, 1016):
        if type(value) is not list:
            block()
        for cell in value:
            if cell is not None and (type(cell) is not (str if oid == 1009 else int) or
                                    (oid == 1016 and not -9223372036854775808 <= cell <= 9223372036854775807)):
                block()
    # json/jsonb use standard json.loads exactly as native psycopg2 does,
    # including nested fractional floats, integer types, and duplicate keys.
    return value


def decode_frame(raw, token, expected):
    try:
        def unique(pairs):
            out = {}
            for key, value in pairs:
                if key in out:
                    block()
                out[key] = value
            return out
        data = json.loads(raw.decode('utf-8'), object_pairs_hook=unique,
                          parse_constant=lambda _: block())
        if (type(data) is not dict or set(data) != {'token', 'rows', 'count', 'columns', 'types'} or data['token'] != token or
                type(data['count']) is not int or not 0 <= data['count'] <= MAX_ROWS or
                type(data['columns']) is not int or data['columns'] != len(expected) or
                type(data['types']) is not list or any(type(oid) is not int for oid in data['types']) or
                tuple(data['types']) != tuple(expected) or any(oid not in CELL_TYPES for oid in expected) or
                type(data['rows']) is not list or len(data['rows']) != data['count']):
            block()
        rows = []
        for row in data['rows']:
            if type(row) is not list or len(row) != len(expected):
                block()
            rows.append(tuple(decode_cell(value, oid) for value, oid in zip(row, expected)))
        return rows
    except Exception:
        block()


# Reviewed fixed-source SELECT fingerprints -> positional PostgreSQL base OIDs.
# Fingerprint is SHA256 of strip().rstrip(";") BEFORE parameter binding. Only
# build_sql's validated integer NOT IN list is replaced with <execution_ids>.
# This table is source/proof-bound with this helper, never learned from a DB.
# Schema/catalog metadata uses base OIDs (psycopg2 also unwraps domains).
QUERY_TYPES = {
    # enroll-quarantine.py:enroll_quarantine_in_transaction | SELECT ( SELECT to_regclass('public.provider_execution_quarantine') IS NOT NULL ) AND ( SELECT EXISTS ( SELECT 1 FROM pg_trigger WHERE tgname = 'trg_provider_execution_quarantine_i
    'ada15365c61d809769bb4ee809f3d5865c10f410d516380aa81fd89a0d42db2f': (16,),
    # enroll-quarantine.py:enroll_quarantine_in_transaction | SELECT clock_timestamp() >= %s::timestamptz AND clock_timestamp() < %s::timestamptz
    '0d373850d1e61ca4fe6bef96469c20c190dfa1dcde9f4c646db74de0ad31138e': (16,),
    # enroll-quarantine.py:enroll_quarantine_in_transaction | SELECT count(*) FROM public.provider_execution_quarantine WHERE execution_id = ANY(%s) AND (clock_timestamp() < not_before OR clock_timestamp() >= expires_at)
    '35be79f3696f5ba71b5098c5ea9ef58e21b5e2a7f8547964e04b2fdd07543092': (20,),
    # enroll-quarantine.py:enroll_quarantine_in_transaction | SELECT execution_id, task_id, attempt, generation, snapshot_sha256, evidence_sha256, approval_id, release_sha, to_char(not_before AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"
    '5e2b4b18b5f1fae7a53c15b58aef2f3f3a82c9399090afc07647a1167929f5c0': (20, 25, 23, 20, 1042, 1042, 25, 1042, 25, 25),
    # enroll-quarantine.py:enroll_quarantine_in_transaction | SELECT id, status, task_status, lease_until, worker_id FROM public.xz_generation_tasks WHERE id = ANY(%s) ORDER BY id FOR UPDATE
    '8e15fe81324f282d10ca1928c8bb32f1b34ddfdac331b016e3be2655342c9477': (25, 25, 25, 1184, 25),
    # enroll-quarantine.py:enroll_quarantine_in_transaction | SELECT id, task_id, attempt, task_execution_generation FROM public.provider_executions WHERE id = ANY(%s) ORDER BY id FOR UPDATE
    'ce2176072bcc55041423162dcfd46f396a7d971dc01dcd606cff62a81a6857d6': (20, 25, 23, 20),
    # quarantine-live-snapshot.py:_query | SELECT * FROM ( SELECT (coalesce(billing_account_id,'') IN ('',%s) AND (NOT (params ? 'billing_account_id') OR (jsonb_typeof(params->'billing_account_id')='string' AND params->>'bi
    '4ea8fe923f21c66de76de41a26a389638d4d2f93e4c972aa5b85f1dcd09a370e': (16,),
    # quarantine-live-snapshot.py:_query | SELECT * FROM ( SELECT (point_cost=%s AND reserved_points=point_cost AND (NOT (params ? 'billingReserved') OR params->'billingReserved'='true'::jsonb) AND (NOT (params ? 'billingRe
    '0769896c58acd092362e2e5ee3cde842f33324c8afcbbac17f71f567e4923693': (16,),
    # quarantine-live-snapshot.py:_query | SELECT * FROM ( SELECT c.relname,c.relkind,c.relrowsecurity,c.relforcerowsecurity FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=
    '18ef2d7559d2bda1d81706b0a808278ee48efe7c0c80cf55361d6f36b174877d': (19, 18, 16, 16),
    # quarantine-live-snapshot.py:_query | SELECT * FROM ( SELECT count(*),sum(allocated_points),sum(reserved_points),sum(captured_points),sum(released_points),sum(expired_points) FROM public.xz_personal_point_reservation_a
    'ec94e1982eeaef55d07b79aede44d9e0b3b4d18586236a015daa4dcf6206488a': (20, 1700, 1700, 1700, 1700, 1700),
    # quarantine-live-snapshot.py:_query | SELECT * FROM ( SELECT execution_generation,status,task_status,user_id,tenant_id, jsonb_typeof(result_ids),type,model, (jsonb_typeof(params)='object' AND (NOT (params ? 'billing_sc
    '2d7aab522edd59afebb3b472d31efee13c38e44909b3281619691fa39bf420f8': (20, 25, 25, 25, 25, 25, 25, 25, 16),
    # quarantine-live-snapshot.py:_query | SELECT * FROM ( SELECT id,(user_id=%s AND (tenant_id IS NULL OR tenant_id IN ('','tenant_default')) AND task_id=%s AND idempotency_key=task_id || ':' || event_type AND event_type I
    'd1e38fd627abc596f8a1832e95e3bc3447b16a7b94261ea6c4a2d6ef7029a853': (25, 16),
    # quarantine-live-snapshot.py:_query | SELECT * FROM ( SELECT id,(user_id=%s AND task_id=%s AND (tenant_id IS NULL OR tenant_id IN ('','tenant_default')) AND jsonb_typeof(raw)='object' AND (NOT (raw ? 'id') OR raw->>'id
    'f2934ddccf0bff07f596ad61a25f2a28fd41221a3af4315f1be9842d5f19c92c': (25, 16),
    # quarantine-live-snapshot.py:_query | SELECT * FROM ( SELECT id,account_id,user_id,business_type,business_id,idempotency_key, requested_points,reserved_points,captured_points,released_points,expired_points FROM public.
    '454730cdcd8974bd3464605c323abb01ea7545c0af26c87971cb1810438fba33': (25, 25, 25, 25, 25, 25, 20, 20, 20, 20, 20),
    # quarantine-live-snapshot.py:_query | SELECT * FROM ( SELECT id,account_id,user_id,task_id,reference_type,reference_id,entry_type, idempotency_key,points, (jsonb_typeof(metadata)='object' AND (NOT (metadata ? 'legacy_l
    'e6fa4ee28e51c76e0c2467a87236ea15180a14b7b706136bec98269ede429894': (25, 25, 25, 25, 25, 25, 25, 25, 1700, 16, 16, 16),
    # quarantine-live-snapshot.py:_query | SELECT * FROM ( SELECT id,execution_id,kind FROM public.provider_execution_correlations WHERE execution_id=ANY(%s) ) AS bounded_projection LIMIT 10001
    'f35e8e9afc9529b7d662c5b8c8670dd19146a7cc96729b7408f70da4c9d4424d': (20, 20, 25),
    # quarantine-live-snapshot.py:_query | selected task provider executions including created_at (pre-119 proof)
    '2ce5a25ee07bf99b7d6a73cc54572713f91d63f9553712843fa52362db12667e': (20, 25, 23, 20, 25, 25, 25, 25, 25, 1042, 1184),
    # quarantine-live-snapshot.py:_query | exact migration ledger row, no caller-supplied timestamp
    'd1469e6526b3aff92ae8dbd6200598cc44dfe70fd61c3589b92097a38305a5c8': (1184,),
    # quarantine-live-snapshot.py:_query | exact task creation timestamp for legacy identity
    '5fe84364e9f1ea0ef2e9f190aa7cabb99a513c334f2fab4aef1db2b2be304681': (25,),
    # quarantine-live-snapshot.py:_query | core task linkage supporting personal and enterprise scopes
    '5e63145a0a7352656cbcce808e6842ada66052cb64f32373bf96f9a094c83c4d': (20, 25, 25, 25, 25, 25, 25, 25, 16),
    # quarantine-live-snapshot.py:_query | financial task linkage supporting personal and enterprise scopes
    'cd010041aae5aeb99f172ab3c348e686cb3163d68059c673b8a1a218d5b40012': (25, 25, 25, 25, 25, 25, 25, 16),
    # quarantine-live-snapshot.py:_query | enterprise tenant wallet query
    '5a4a967a89493b11c336af1b369f183645fc92af499ad05d45151b0d5f3d28b0': (25, 20, 20, 25),
    # quarantine-live-snapshot.py:_query | enterprise task account linkage
    '9e97f5930bc9769f4c0535cbc7e38119827f61777cdf47ea4faf8a333aa88267': (16,),
    # quarantine-live-snapshot.py:_query | wallet ledger query supporting personal and enterprise scopes
    'b7b64c6adfe4a4c0202efacf85fbeae3528722fcda80090a155b9f65ee7045d0': (25, 25, 25, 25, 25, 25, 25, 25, 1700, 16, 16, 16),
    # quarantine-live-snapshot.py:_query | enterprise tenant wallet family rows
    '2f4e2a4b8238a5b55746e24c05d615d4a15e42b832b12d58f078438797ab5364': (25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25),
    # quarantine-live-snapshot.py:_query | enterprise wallet ledger family rows
    'a093d2ad2a92d15d2d4f8f7bd07a2018d525266be79fddfba4e97f86cefe8a6b': (25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25),
    # quarantine-live-snapshot.py:_query | enterprise billing lifecycle events family rows
    '30a880180c7650b4135e9faebfa54cd1b408b7ebf3285baa5bb8f90c574eb00a': (25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25),
    # quarantine-live-snapshot.py:_query | enterprise billing lifecycle events verification
    '1764cc64cb2875617af0f4fbe75e68a2c8474b4751a06c39a71aee22e216f99e': (25, 16),
    # quarantine-live-snapshot.py:_query | SELECT * FROM ( SELECT movement_type,idempotency_key,points FROM public.xz_personal_point_lot_movements WHERE reservation_id=%s ) AS bounded_projection LIMIT 10001
    'b3cbade91e87660f96ea0cedd69aaa2338fc82e4093d4383f6fa53508deaea0e': (25, 25, 20),
    # quarantine-live-snapshot.py:_query | SELECT * FROM ( SELECT table_name,column_name,udt_name,is_nullable,character_maximum_length, numeric_precision,numeric_scale FROM information_schema.columns WHERE table_schema='pub
    '628a496c16d127aabe1a36665ef36920c100d7b2d37f7f2ad22e6a191510ee00': (19, 19, 19, 1043, 23, 23, 23),
    # quarantine-live-snapshot.py:_query | SELECT * FROM ( SELECT user_id,coalesce(raw->>'billingEngine',''), coalesce(raw->>'personalPointAccountId',''),coalesce(raw->>'personalPointReservationId',''), (jsonb_typeof(raw)='
    'd20d4398552cccd6112f37adfd145da1e4da943505b934c0cf2bfebefef73956': (25, 25, 25, 25, 16),
    # quarantine-live-snapshot.py:_query | asset graph: SELECT ( (SELECT count(*) FROM public.provider_executions e,t WHERE e.task_id=t.id)=1 AND NOT EXISTS(SELECT 1 FROM public.provider_executions e,t WHERE e.task_id=t.id
    'f01a80755b8c619d1a19b8c4dfc7790f28696f0f53f1efa46e1e26d2253fa2ab': (16,),
    # quarantine-live-snapshot.py:_query | asset graph: SELECT ( EXISTS(SELECT 1 FROM pg_catalog.pg_index i JOIN pg_catalog.pg_class c ON c.oid=i.indexrelid WHERE c.relnamespace='public'::regnamespace AND c.relname='ux_file
    '9ca4d0aad8fcfcee6dfde7fffddda9e1725a9ee63f60a96c5897c47f85cf0e95': (16,),
    # quarantine-live-snapshot.py:_query | asset graph: SELECT ( NOT EXISTS(SELECT 1 FROM aa a WHERE a.metadata->'storageManaged' IS DISTINCT FROM 'true'::jsonb OR jsonb_typeof(a.metadata->'fileId') IS DISTINCT FROM 'string
    '4e4df0f959a3dce98312dd677e568ba2bde1b2d62ad2d9c789e918a694a1a116': (16,),
    # quarantine-live-snapshot.py:_query | asset graph: SELECT ( NOT EXISTS(SELECT 1 FROM aa a,t WHERE a.user_id IS DISTINCT FROM t.user_id OR a.task_id IS DISTINCT FROM t.id OR coalesce(a.tenant_id,'')<>coalesce(t.tenant_i
    'eb79a47157f5923fec4caeec6cc5a8406935c3d6788c9de93494dd6df7adf431': (16,),
    # quarantine-live-snapshot.py:_query | asset graph: SELECT ( NOT EXISTS(SELECT 1 FROM ff WHERE storage_config_id='env_default') )) AS bounded_projection LIMIT 2
    '8eb6ef1352ee68e4db9f85e9383a660931e593f32501619072772e3d7e9d6618': (16,),
    # quarantine-live-snapshot.py:_query | asset graph: SELECT ( NOT EXISTS(SELECT 1 FROM ff f,t WHERE f.user_id<>t.user_id OR f.tenant_id<>coalesce(nullif(t.tenant_id,''),'tenant_default') OR f.business_id IS DISTINCT FROM
    'ca729d97a9f6ce7af4faa79641a50a028874b3f21a7656a9a1801829e0d8c13f': (16,),
    # quarantine-live-snapshot.py:_query | asset graph: SELECT ( NOT EXISTS(SELECT 1 FROM nodes WHERE (left(node,2)='a:' AND NOT EXISTS(SELECT 1 FROM aa WHERE 'a:'||id=node)) OR (left(node,2)='f:' AND NOT EXISTS(SELECT 1 FR
    '65e11291265c0bec81125c8d78c48be8bcc1c65e8e8548640ad89f9e18a74384': (16,),
    # quarantine-live-snapshot.py:_query | asset graph: SELECT ( NOT EXISTS(SELECT 1 FROM public.provider_executions e,t WHERE e.task_id=t.id AND ((e.result_metadata IS NOT NULL AND (e.result_metadata='null'::jsonb OR (e.ca
    '9084b62aed2ab9c1f0c70a4456f9e9283b6523ba6b7ce276e8fdd600b83298e8': (16,),
    # quarantine-live-snapshot.py:_query | asset graph: SELECT ( NOT EXISTS(SELECT 1 FROM public.provider_executions e,t, jsonb_array_elements(CASE WHEN jsonb_typeof(e.result_metadata)='array' THEN e.result_metadata ELSE '[
    '0fa33746daa7f9977c11bdd4c3f30d07454f27980bac8575ee40b5ec5b59942b': (16,),
    # quarantine-live-snapshot.py:_query | asset graph: SELECT ( NOT EXISTS(SELECT 1 FROM public.xz_generation_tasks other,t WHERE other.id<>t.id AND (EXISTS(SELECT 1 FROM jsonb_path_query(other.params->'generated_storage_f
    'a06d780bf3a7162e43823597ae017245bcffee42408424f3c3eb7cb133742a49': (16,),
    # quarantine-live-snapshot.py:_query | asset graph: SELECT ( NOT EXISTS(SELECT 1 FROM t WHERE jsonb_typeof(raw)<>'object' OR (raw ? 'id' AND raw->>'id' IS DISTINCT FROM id) OR (raw ? 'userId' AND raw->>'userId' IS DISTI
    '44c97e8653b9de4d39068413648230dd161950cb0ca53fb08486bd76891288b8': (16,),
    # quarantine-live-snapshot.py:_query | asset graph: SELECT CASE WHEN "access_key_encrypted" IS NULL THEN NULL ELSE encode(public.digest(convert_to("access_key_encrypted"::text,'UTF8'),'sha256'),'hex') END,CASE WHEN "buc
    'f184a45f409658b152f540171d13bcca99b85c96bd22a50f4275fc3381e946a0': (25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25),
    # quarantine-live-snapshot.py:_query | asset graph: SELECT CASE WHEN "attempt" IS NULL THEN NULL ELSE encode(public.digest(convert_to("attempt"::text,'UTF8'),'sha256'),'hex') END,CASE WHEN "created_at" IS NULL THEN NULL
    '825cdf23ba3aeb34db027a175317353fe9b5509fe39e6af05c845b03feda41b1': (25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25),
    # quarantine-live-snapshot.py:_query | asset graph: SELECT CASE WHEN "bucket" IS NULL THEN NULL ELSE encode(public.digest(convert_to("bucket"::text,'UTF8'),'sha256'),'hex') END,CASE WHEN "business_id" IS NULL THEN NULL
    '122761d255d2571a693cdd2c3c41f60f76f0dfbf7ebf945f171854fd400257cc': (25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25),
    # quarantine-live-snapshot.py:_query | asset graph: SELECT CASE WHEN "completed_at" IS NULL THEN NULL ELSE encode(public.digest(convert_to("completed_at"::text,'UTF8'),'sha256'),'hex') END,CASE WHEN "content_type" IS NU
    '903a54961ac99e9cd7e241bf1deae715fafea21c87bd9ef3e32665d8db9b7306': (25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25),
    # quarantine-live-snapshot.py:_query | asset graph: SELECT CASE WHEN "completed_at" IS NULL THEN NULL ELSE encode(public.digest(convert_to("completed_at"::text,'UTF8'),'sha256'),'hex') END,CASE WHEN "etag" IS NULL THEN
    'a4870757ad6d35e452f6651f18e0e677eb7cb0f048366b2bdeeddfc08e3efbf7': (25, 25, 25, 25, 25),
    # quarantine-live-snapshot.py:_query | asset graph: SELECT CASE WHEN "created_at" IS NULL THEN NULL ELSE encode(public.digest(convert_to("created_at"::text,'UTF8'),'sha256'),'hex') END,CASE WHEN "deleted_at" IS NULL THE
    '00a62133a363006646f4c11a7172d334c82cb001c2fce6f4691cff487aca1e78': (25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25),
    # quarantine-live-snapshot.py:_query | asset graph: SELECT CASE WHEN "created_at" IS NULL THEN NULL ELSE encode(public.digest(convert_to("created_at"::text,'UTF8'),'sha256'),'hex') END,CASE WHEN "id" IS NULL THEN NULL E
    '6d8702519768359f1aba5bc660cd28875acb19a2f9c6ae60d880f4deb11043d7': (25, 25, 25, 25, 25, 25),
    # quarantine-live-snapshot.py:_query | asset graph: SELECT count(*),count(DISTINCT (upload_id,part_number)) FROM pp) AS bounded_projection LIMIT 2
    '20c857c4bda963958071abe2e7e92d7a4beeffa3e6de74f5217b4926b0e40572': (20, 20),
    # quarantine-live-snapshot.py:_query | asset graph: SELECT count(*),count(DISTINCT file_id) FROM ff) AS bounded_projection LIMIT 2
    'bda6b0960a10fcb97df7ae6d131e75e0febecdbcb73c27dc4f8346fb66cb5e2b': (20, 20),
    # quarantine-live-snapshot.py:_query | asset graph: SELECT count(*),count(DISTINCT id) FROM aa) AS bounded_projection LIMIT 2
    'e6ae177337e8263e993ed2364af8a82d51c9af791bf072ffb826cc9a3f448de5': (20, 20),
    # quarantine-live-snapshot.py:_query | asset graph: SELECT count(*),count(DISTINCT id) FROM cc) AS bounded_projection LIMIT 2
    'e5a3c1bdc8ee381514ea255c4e3cddbb1a1a77f70af8a15244ccb2c65f131c53': (20, 20),
    # quarantine-live-snapshot.py:_query | asset graph: SELECT count(*),count(DISTINCT id) FROM jj) AS bounded_projection LIMIT 2
    '68850981d9035654272d6a389803363d0237a848dbf1239677d47be2567e42e1': (20, 20),
    # quarantine-live-snapshot.py:_query | asset graph: SELECT count(*),count(DISTINCT id) FROM mm) AS bounded_projection LIMIT 2
    'efc3dbc91cb358f7b64866f3b05fbc651afce587ecf29f89cff1282f01d5db89': (20, 20),
    # quarantine-live-snapshot.py:_query | asset graph: SELECT count(*),count(DISTINCT id) FROM rr) AS bounded_projection LIMIT 2
    '0f88693dab74577d58859c301626e94f38f2c68e2158876fb2412aae8e0c4bf6': (20, 20),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT CASE WHEN "account_id" IS NULL THEN NULL ELSE encode(public.digest(convert_to("account_id"::text,'UTF8'),'sha256'),'hex') END,CASE WHEN "allocated_points" IS
    'df4eea3a75c5217074dc6bbc3b5baf81dd62f30905e3216202b16b634bbc96bb': (25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT CASE WHEN "account_id" IS NULL THEN NULL ELSE encode(public.digest(convert_to("account_id"::text,'UTF8'),'sha256'),'hex') END,CASE WHEN "available_after" IS N
    '694033b2df98465edcca4c6dc69b4ebb4ddb5c9ce3169ab97f1864a014c464e9': (25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT CASE WHEN "account_id" IS NULL THEN NULL ELSE encode(public.digest(convert_to("account_id"::text,'UTF8'),'sha256'),'hex') END,CASE WHEN "available_after" IS N
    'ce08339fbb1967bdfe69df76aab82cc366e91eb31bcca88b5d635081a02484e9': (25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT CASE WHEN "account_id" IS NULL THEN NULL ELSE encode(public.digest(convert_to("account_id"::text,'UTF8'),'sha256'),'hex') END,CASE WHEN "available_points" IS
    'ab1a6f4d128fe411929251c44dde5d48ff2bde88f15fcb635fd76d39fb270d95': (25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT CASE WHEN "account_id" IS NULL THEN NULL ELSE encode(public.digest(convert_to("account_id"::text,'UTF8'),'sha256'),'hex') END,CASE WHEN "business_id" IS NULL
    'cf8d07b052e00dd945cd46886bc7fc760589f75216b43e7605ea36f65074c4c0': (25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT CASE WHEN "agent_id" IS NULL THEN NULL ELSE encode(public.digest(convert_to("agent_id"::text,'UTF8'),'sha256'),'hex') END,CASE WHEN "amount_cents" IS NULL THE
    '27e980e1e05a940ccf97d0ab4de7a837d6df1555dc926386728b00ab895e84f2': (25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT CASE WHEN "ai_label_status" IS NULL THEN NULL ELSE encode(public.digest(convert_to("ai_label_status"::text,'UTF8'),'sha256'),'hex') END,CASE WHEN "billing_acc
    '5e2766950b3a40bf81639403ce28c5be128edce2ce4bdfd11153e136a699b928': (25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT CASE WHEN "attempt" IS NULL THEN NULL ELSE encode(public.digest(convert_to("attempt"::text,'UTF8'),'sha256'),'hex') END,CASE WHEN "capability" IS NULL THEN NU
    'b81b74b82c279e7de98b520269f07300a0c3e3afafd80e980de763b97e0a2ed4': (25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT CASE WHEN "available" IS NULL THEN NULL ELSE encode(public.digest(convert_to("available"::text,'UTF8'),'sha256'),'hex') END,CASE WHEN "frozen" IS NULL THEN NU
    'cb0fc2509b0e2ed5cdc6dec28ebd12c81b8e240dbf6848d4fa6c044053ec3d2f': (25, 25, 25, 25, 25),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT CASE WHEN "base_url_host" IS NULL THEN NULL ELSE encode(public.digest(convert_to("base_url_host"::text,'UTF8'),'sha256'),'hex') END,CASE WHEN "created_at" IS
    '7ad4efb385dcb2aeffad9b50aeb35becabe7d9ddf3a162423209be5b2b5d7a59': (25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT CASE WHEN "billing_status" IS NULL THEN NULL ELSE encode(public.digest(convert_to("billing_status"::text,'UTF8'),'sha256'),'hex') END,CASE WHEN "created_at" I
    'da736af4280a3fb100ee733527028a1f36576cda515386cdae678a680fc32538': (25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25, 25),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT CASE WHEN "cash_balance_cents" IS NULL THEN NULL ELSE encode(public.digest(convert_to("cash_balance_cents"::text,'UTF8'),'sha256'),'hex') END,CASE WHEN "froze
    '11566e84e7a13e63175a7273943b600f3b8604c2261e16181b866f09f7f482b0': (25, 25, 25, 25, 25, 25, 25, 25),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT coalesce(sum(available_points),0),coalesce(sum(reserved_points),0) FROM public.xz_personal_point_lots WHERE account_id=%s AND user_id=%s) AS bounded_projectio
    '691fd9d8b8d61d69919d47896b7edd9ed1bc5d308ef0790dd1618506811189c2': (1700, 1700),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT count(*) FROM public.generation_tasks WHERE id::text=%s) AS bounded_projection LIMIT 2
    'a90ebf75101811c7daeb349ec8f4d8f2e91be82f2208126480af95801763c715': (20,),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT count(*) FROM public.xz_billing_events WHERE transaction_id IN (SELECT transaction_id FROM public.xz_billing_events WHERE id=ANY(%s)) AND transaction_id<>'' G
    'b49141e6350aab9d12663b3e8b9efa25d7f2ca8c5b2d757871c35c85a9512025': (20,),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT count(*) FROM public.xz_billing_lifecycle_events WHERE idempotency_key IN (SELECT idempotency_key FROM public.xz_billing_lifecycle_events WHERE id=ANY(%s)) GR
    '620d363b1d84f1acea81370b1f06819510909253f64ae0bb0c464b939b57d920': (20,),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT count(*) FROM public.xz_personal_point_lot_movements WHERE (lot_id,idempotency_key) IN (SELECT lot_id,idempotency_key FROM public.xz_personal_point_lot_moveme
    '34984e0c52a07f0365f6ac836ab362f5b245a91b23b08a24479dbd86c518259b': (20,),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT count(*) FROM public.xz_personal_point_lots WHERE (account_id,idempotency_key) IN (SELECT account_id,idempotency_key FROM public.xz_personal_point_lots WHERE
    '6888a4a725ae28c730e04ed7044e0845a43d0f1717ab1a1614438731fc1c375b': (20,),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT count(*) FROM public.xz_personal_point_reservation_allocations WHERE (reservation_id,lot_id) IN (SELECT reservation_id,lot_id FROM public.xz_personal_point_re
    '5fcb6702f157c212b0813b32965bbb575aaf408531093a8fe25e090ab864a7c5': (20,),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT count(*) FROM public.xz_personal_point_reservations WHERE (account_id,business_type,business_id) IN (SELECT account_id,business_type,business_id FROM public.x
    'd37d182fabb9a3e177cc3c5e3968258a86243ec8380d846f1fd0f5e2847d5007': (20,),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT count(*) FROM public.xz_personal_point_reservations WHERE (account_id,idempotency_key) IN (SELECT account_id,idempotency_key FROM public.xz_personal_point_res
    '8ef0bebea7999f6b90d6985e71465442a2647acfaab3c2708d59ed06ffae98bd': (20,),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT count(*) FROM public.xz_wallet_ledger WHERE (idempotency_key) IN (SELECT idempotency_key FROM public.xz_wallet_ledger WHERE id=ANY(%s)) GROUP BY idempotency_k
    'fdfe2832840537d1a90405e6aa3130a2d74f746e7536afbae038e1ad0600caef': (20,),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT current_setting('transaction_isolation'), clock_timestamp()) AS bounded_projection LIMIT 2
    '26ef111a57f4627d9a10c32027ade4bc158394d5dc2be6ae49688054567d3945': (25, 1184),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT id,account_id,user_id,(id<>'' AND idempotency_key<>'' AND entry_type IN ('RECHARGE','GRANT','RESERVE','CAPTURE','RELEASE','REFUND','ADJUSTMENT','EXPIRE') AND
    '6540155522c2846a1a871eec9a7299e7a10d51054bd847a13f0a12850686b140': (25, 25, 25, 16),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT id,account_id,user_id,(id<>'' AND idempotency_key<>'' AND movement_type IN ('OPENING','GRANT','RESERVE','CAPTURE','RELEASE','EXPIRE','ADJUSTMENT','REVERSE') A
    'ae1b3bc54e76792f6b0922a5130449786ed28ad3c9628aefeb8cfb16ad700e44': (25, 25, 25, 16),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT id,account_id,user_id,(id<>'' AND idempotency_key<>'' AND reserved_points>=0 AND captured_points>=0 AND released_points>=0 AND expired_points>=0 AND ((status=
    '167c20f3907955636416709cc75eed46c3c337841964c568c8b639286daf44e3': (25, 25, 25, 16),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT id,account_id,user_id,(id<>'' AND idempotency_key<>'' AND status IN ('ACTIVE','EXHAUSTED','EXPIRED','REVERSED','LEGACY') AND source_type IN ('REGISTRATION_GIF
    '305bcfa847f47064da6fb5c756c93e4b540c2b66409706628f3bcfdb34540968': (25, 25, 25, 16),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT id,account_id,user_id,(id<>'' AND status<>'CANCELLED' AND allocated_points>0 AND reserved_points>=0 AND captured_points>=0 AND released_points>=0 AND expired_
    '50fe61fe253671f91585924a3f22bcf6f53c9096f8d45d28e2fd0a1aca1fc66c': (25, 25, 25, 16),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT id,count(*) FROM public.provider_execution_correlations WHERE id=ANY(%s) GROUP BY id) AS bounded_projection LIMIT 10001
    'f557fca68f9dc5fe88ab2ca9469762b308ee9f6460da55e6f6ed0d36c5df0b2f': (20, 20),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT id,count(*) FROM public.provider_executions WHERE id=ANY(%s) GROUP BY id) AS bounded_projection LIMIT 10001
    '2c9184556961f9592e79fcb3dcb147e490feed73374b0f2f308eb04076663504': (20, 20),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT id,count(*) FROM public.xz_billing_events WHERE id=ANY(%s) GROUP BY id) AS bounded_projection LIMIT 10001
    '76842025dfbbf18252e5a0620e908f5707985e44d6fb35a240ea2c0b28811f2f': (25, 20),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT id,count(*) FROM public.xz_billing_lifecycle_events WHERE id=ANY(%s) GROUP BY id) AS bounded_projection LIMIT 10001
    'd39fcd82a8247047a35ab48134cc6e033d2206505e504ab17f1e5013c5fc52c7': (25, 20),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT id,count(*) FROM public.xz_personal_point_lot_movements WHERE id=ANY(%s) GROUP BY id) AS bounded_projection LIMIT 10001
    '012df4909736e93f04f9ff07ee0e40aa3a0fef93111a14f194aff4152940727d': (25, 20),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT id,count(*) FROM public.xz_personal_point_lots WHERE id=ANY(%s) GROUP BY id) AS bounded_projection LIMIT 10001
    '5a5d8949c48faea69c300369c835341ea2b04a6395420e97577188b1d45bf7cc': (25, 20),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT id,count(*) FROM public.xz_personal_point_reservation_allocations WHERE id=ANY(%s) GROUP BY id) AS bounded_projection LIMIT 10001
    '2058c823024ceaf6cfc69dcaf84be0804388cf852306bf12edff44e84f98cfa3': (25, 20),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT id,count(*) FROM public.xz_personal_point_reservations WHERE id=ANY(%s) GROUP BY id) AS bounded_projection LIMIT 10001
    '295b637e5bbc9f836330f39146b7e7159ad0b5d71db8c8fd894ff3e0e6c21c8e': (25, 20),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT id,count(*) FROM public.xz_wallet_ledger WHERE id=ANY(%s) GROUP BY id) AS bounded_projection LIMIT 10001
    '4d4025777e85cee1c92b85c0b163f929a69c997f2760968b83ade62471016361': (25, 20),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT id,user_id,available,frozen FROM public.xz_point_accounts WHERE user_id=%s OR id=%s) AS bounded_projection LIMIT 2
    '64808a7ad56e152fe2c9e284f1842bf9322bd1a9c3318f072754ca9c41ada683': (25, 25, 20, 20),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT lot_id,reservation_id FROM public.xz_personal_point_lot_movements WHERE account_id=%s OR user_id=%s OR reservation_id=ANY(%s) OR lot_id=ANY(%s)) AS bounded_pr
    '6aff6312b17c100d16a83d67fe769ad967ff7bfb02391b5b0fb91155cf91036b': (25, 25),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT reservation_id,lot_id FROM public.xz_personal_point_reservation_allocations WHERE account_id=%s OR user_id=%s OR reservation_id=ANY(%s) OR lot_id=ANY(%s)) AS
    '00f7b73ef2eb8006ef6b2a4a34c296fa17fd37518a2dd419ae9be829d6a1e459': (25, 25),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT reservation_id,lot_id FROM public.xz_personal_point_reservation_allocations WHERE reservation_id=%s) AS bounded_projection LIMIT 10001
    '6ae3b0c378240cd363181d6facb7bb08473aa6f9468429db7169bdfb9e94ce17': (25, 25),
    # quarantine-live-snapshot.py:_query | SELECT * FROM (SELECT token_balance,frozen_token FROM public.xz_user_wallets WHERE user_id=%s) AS bounded_projection LIMIT 2
    '3048c817c817bf3bb412475afd9fd7095947694e50fb687275f918cac9c97ddd': (20, 20),
    # verify-safe-drain.py:build_sql | SELECT (SELECT count(*) FROM public.xz_generation_tasks WHERE lease_until > now() OR status IS NULL OR task_status IS NULL OR upper(status) NOT IN ('COMPLETED','SUCCEEDED','FAILED'
    'c4309b3f93c36872684a1dd57ac23493bd06df508cc15ba595ce257539c6e484': (20,),
    # verify-safe-drain.py:build_sql | SELECT (SELECT count(*) FROM public.xz_generation_tasks WHERE lease_until > now() OR status IS NULL OR task_status IS NULL OR upper(status) NOT IN ('COMPLETED','SUCCEEDED','FAILED'
    'eb192946b9b0566787cd39b91df85143b6654cfb1f22a37fb2d532f61d444e83': (20,),
    # verify-safe-drain.py fixed historical projection and identity keys.
    'fffc2f157dbe8db61c072fcac74a2ef6833e00546a95f2e91df3d358a288cc6d': (20, 25),
    '8d8bfec9db665a52c361eefddb67342aebf481c4c62b1f83f6a64534dfe05a43': (20, 25),
    '7b5ce9c2e098c83c5ccbe6de49a294452524a648203df5b2721ea84fdc02cb18': (19, 25, 25),
}

class Connection:
    def __init__(self, target):
        self.target = target
        self.autocommit = True
        self.closed = False
        self.status = 0
        self.messages = queue.Queue(maxsize=4)
        self.process = None
        try:
            # Pin the inspected ID, not a service name that can retarget on exec.
            # Explicit Unix socket + container credentials only; ignore host PG*.
            shell = ('unset PGHOST PGHOSTADDR PGPORT PGSERVICE PGSERVICEFILE PGPASSFILE; '
                     'export PGPASSWORD="$POSTGRES_PASSWORD" PGCLIENTENCODING=UTF8; '
                     'export PGOPTIONS="-c statement_timeout=10000 -c idle_in_transaction_session_timeout=60000 -c timezone=UTC"; '
                     'exec psql -X -q -t -A -h /var/run/postgresql -p 5432 '
                     '-U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1')
            self.process = subprocess.Popen(command(['docker', 'exec', '-i', target.expected['container_id'],
                                                     'sh', '-c', shell]), stdin=subprocess.PIPE,
                                            stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
            thread = threading.Thread(target=self._read)
            thread.daemon = True
            thread.start()
            self.cursor().execute("SET application_name = 'issue203-quarantine-transport'")
            target.check()
        except Exception:
            self.close()
            block()

    def _read(self):
        try:
            while True:
                line = self.process.stdout.readline(MAX_BYTES + 1)
                self.messages.put(line, timeout=TIMEOUT)
                if not line or len(line) > MAX_BYTES:
                    return
        except Exception:
            return

    def get_transaction_status(self):
        return self.status

    def cursor(self):
        if self.closed:
            block()
        return Cursor(self)

    def close(self):
        self.closed = True
        if self.process is not None:
            try:
                self.process.stdin.close()  # EOF rolls back, NEVER implicit COMMIT
                self.process.wait(timeout=2)
            except Exception:
                self.process.kill()
                self.process.wait(timeout=2)
            self.process.stdout.close()


class Cursor:
    def __init__(self, connection):
        self.connection = connection
        self.rows = []

    def execute(self, sql, parameters=()):
        conn = self.connection
        try:
            if conn.closed or conn.process.poll() is not None:
                block()
            text = bind(sql, parameters)
            token = uuid.uuid4().hex
            if re.match(r'^SELECT\s', text, re.I):
                expected = query_types(sql)
                statement = select_statement(text, token, expected)
            else:
                expected = ()
                allowed = (text in (
                    'BEGIN ISOLATION LEVEL REPEATABLE READ',
                    'BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY', 'ROLLBACK', 'COMMIT',
                    "SET application_name = 'issue203-quarantine-transport'",
                    "SET LOCAL search_path = pg_catalog, public", "SET LOCAL TIME ZONE 'UTC'",
                    "SET LOCAL DateStyle = 'ISO, YMD'", "SET LOCAL IntervalStyle = 'iso_8601'",
                    "SET LOCAL extra_float_digits = 3", "SET LOCAL bytea_output = 'hex'",
                    "SET LOCAL statement_timeout = 10000") or
                    # enroll_quarantine_in_transaction's single ten-column INSERT.
                    hashlib.sha256(sql.strip().rstrip(';').encode('utf-8')).hexdigest() ==
                    '58bafe22c08eec2d0a485c8c331b0bc92948ecf53b78975dc600591800ef86f4')
                if not allowed:
                    block()
                if text == 'COMMIT':
                    conn.target.check()
                statement = text + ";\nSELECT json_build_object('token','" + token + "','count',0,'columns',0,'types','[]'::json,'rows','[]'::json);\n"
            statement += "\\echo END_" + token + '\n'
            conn.process.stdin.write(statement.encode('utf-8'))
            conn.process.stdin.flush()
            raw = conn.messages.get(timeout=TIMEOUT)
            if not raw or len(raw) > MAX_BYTES or not raw.endswith(b'\n'):
                block()
            self.rows = decode_frame(raw, token, expected)
            if conn.messages.get(timeout=TIMEOUT).strip() != ('END_' + token).encode('ascii'):
                block()
            if conn.process.poll() is not None:
                block()
            if text.startswith('BEGIN '):
                conn.status = 2
            elif text in ('ROLLBACK', 'COMMIT'):
                conn.status = 0
                if text == 'COMMIT':
                    conn.target.check()  # uncertainty is a failure, never retried
        except Exception:
            conn.close()
            block()

    def fetchall(self):
        rows, self.rows = self.rows, []
        return rows

    def fetchone(self):
        return self.rows.pop(0) if self.rows else None

    def close(self):
        self.rows = []

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()
