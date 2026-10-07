#!/usr/bin/env python3
"""Independent quarantine approval verifier and unsigned-candidate builder.

Production trust is the registry pinned in this Carrier's source tree; no
CLI/environment trust-root override. Private signing keys never belong in the
repository, release host, Pi, or deploy tooling. Candidate generation does not
approve or sign; callers must recompute the live snapshot and use the DB clock.
"""
import base64
import datetime
import hashlib
import json
import os
import re
import stat
import subprocess
import sys
import tempfile

REGISTRY_PATH = os.path.join(os.path.dirname(os.path.realpath(__file__)),
                             'quarantine-approval', 'registry.json')
AUTHORITY_ID = 'prod-quarantine-approval-v1'
PURPOSE = 'quarantine-enrollment'
OPERATIONS = ['drain-exemption', 'enroll']
ALGORITHM = 'RSA-PKCS1-v1_5-SHA256'
BLOCKED_CARRIER = 'f9cdf44ca79272ad7cead33dfb1d35fdf155f05f'
MAX_BYTES = 1048576
MAX_CANDIDATE_BYTES = 32 * 1024 * 1024
CANDIDATE_VERSION = 'quarantine-approval-candidate-v1'
LIVE_SNAPSHOT_VERSION = 'issue203-live-canonical-snapshot-sha256-v1'
LEGACY_GENERATION_REASON = 'pre-migration-119-generation-not-recorded'
# Reviewed exact execution/task/attempt identities, never a wildcard for NULL rows.
# Digest input is canonical({'attempt': int, 'execution_id': int, 'task_id': str}).
LEGACY_NULL_IDENTITY_PINS = frozenset((
    'bd7773cfc2090bbfe77f01ce14db52fb3bfd863f0d7a3900847fc777a562ad5d',
    '73383748ac2cd7e1f98570f4e0113e3e7a51600b2958b68af5197f419d6ab84c',
    '455af65b21fa4dbc5e5553d16383e1b48056b88485e5eebc54cf0939210210ce',
    '37fcaee4ec7fabb8de0c8b9d9c88bebfea1cfb43458c9e149c33de9463bed0db',
    'db3c5f6a38fa62f45843e82acfa81ac73806d80e4fb9d054b5d38af22479504c',
    'bc0d9022032d930a5661f73f28b759b0b02ffadd4af8e1aaf3499702f199a7a6',
))


class ApprovalError(Exception):
    pass


def reject(code="QUARANTINE_APPROVAL_INVALID"):
    # Never include parsed data, paths, subprocess output or exception messages.
    raise ApprovalError(code)


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':'),
                      ensure_ascii=True, allow_nan=False).encode('ascii')


def _pairs(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            reject()
        result[key] = value
    return result


def decode(raw):
    if not isinstance(raw, bytes) or not 0 < len(raw) <= MAX_BYTES:
        reject()
    try:
        return json.loads(raw.decode('utf-8'), object_pairs_hook=_pairs,
                          parse_constant=lambda _: reject())
    except (ValueError, UnicodeError):
        reject()


def timestamp(value):
    # One spelling, full microsecond precision; no implicit timezone/truncation.
    if not isinstance(value, str) or not re.fullmatch(
            r'\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{6}Z', value):
        reject()
    try:
        return datetime.datetime.strptime(value, '%Y-%m-%dT%H:%M:%S.%fZ').replace(
            tzinfo=datetime.timezone.utc)
    except ValueError:
        reject()


def legacy_identity_sha256(execution_id, task_id, attempt):
    if (type(execution_id) is not int or execution_id <= 0 or
            not _text(task_id) or type(attempt) is not int or attempt <= 0):
        reject('QUARANTINE_CANDIDATE_IDENTITY_INVALID')
    return hashlib.sha256(canonical({'attempt': attempt, 'execution_id': execution_id,
                                     'task_id': task_id})).hexdigest()


def legacy_identity_pinned(identity):
    return legacy_identity_sha256(identity['execution_id'], identity['task_id'],
                                  identity['attempt']) in LEGACY_NULL_IDENTITY_PINS


def validate_legacy_snapshot(identity, snapshot):
    if identity['generation'] is not None:
        if any(key in snapshot for key in ('task_execution_generation', 'legacy_generation_unverifiable',
                                           'generation_resolution_reason', 'generation_resolution_evidence')):
            reject('QUARANTINE_CANDIDATE_SNAPSHOT_INVALID')
        return
    evidence = snapshot.get('generation_resolution_evidence')
    if (not legacy_identity_pinned(identity) or
            snapshot.get('task_execution_generation', False) is not None or
            snapshot.get('legacy_generation_unverifiable') is not True or
            snapshot.get('generation_resolution_reason') != LEGACY_GENERATION_REASON or
            not isinstance(evidence, dict) or
            set(evidence) != {'identity_sha256', 'migration119_applied_at',
                              'execution_created_at', 'task_created_at'} or
            evidence['identity_sha256'] != legacy_identity_sha256(
                identity['execution_id'], identity['task_id'], identity['attempt'])):
        reject('QUARANTINE_CANDIDATE_SNAPSHOT_INVALID')
    try:
        migration = timestamp(evidence['migration119_applied_at'])
        execution = timestamp(evidence['execution_created_at'])
        task = timestamp(evidence['task_created_at'])
    except (TypeError, ApprovalError):
        reject('QUARANTINE_CANDIDATE_SNAPSHOT_INVALID')
    if not execution < migration or not task < migration:
        reject('QUARANTINE_CANDIDATE_SNAPSHOT_INVALID')


def _hex(value, size):
    return isinstance(value, str) and re.fullmatch('[0-9a-f]{%d}' % size, value)


def _text(value):
    return isinstance(value, str) and 0 < len(value.encode('utf-8')) <= 256


def _keys(value, keys):
    if not isinstance(value, dict) or set(value) != set(keys):
        reject()


def check_runtime():
    if sys.version_info.major != 3 or sys.version_info < (3, 6):
        reject('QUARANTINE_APPROVAL_RUNTIME_INVALID')
    try:
        proc = subprocess.run(['openssl', 'version'], stdout=subprocess.PIPE,
                              stderr=subprocess.PIPE, timeout=10)
    except (OSError, subprocess.TimeoutExpired):
        reject('QUARANTINE_APPROVAL_RUNTIME_INVALID')
    # Only the explicitly supported OpenSSL implementation/version families.
    if proc.returncode or not re.match(br'^OpenSSL (1\.1\.1[a-z]*|3\.\d+\.\d+)\b', proc.stdout):
        reject('QUARANTINE_APPROVAL_RUNTIME_INVALID')


def _trusted_read(path):
    """Open root-owned paths without symlinks; freeze descriptor-read bytes.

    Walk from / using directory descriptors, avoiding lstat/open races and
    requiring every ancestor to be root owned and not group/world writable.
    """
    if not isinstance(path, str) or not path.startswith('/') or os.name != 'posix':
        reject('QUARANTINE_APPROVAL_ROOT_INVALID')
    parts = path.split('/')[1:]
    if not parts or any(p in ('', '.', '..') for p in parts):
        reject('QUARANTINE_APPROVAL_ROOT_INVALID')
    if not hasattr(os, 'O_NOFOLLOW') or not hasattr(os, 'O_DIRECTORY'):
        reject('QUARANTINE_APPROVAL_ROOT_INVALID')
    fd = None
    try:
        fd = os.open('/', os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        for index, component in enumerate(parts):
            info = os.fstat(fd)
            if info.st_uid != 0 or info.st_mode & 0o022:
                reject('QUARANTINE_APPROVAL_ROOT_INVALID')
            flags = os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK
            if index != len(parts) - 1:
                flags |= os.O_DIRECTORY
            next_fd = os.open(component, flags, dir_fd=fd)
            os.close(fd)
            fd = next_fd
        info = os.fstat(fd)
        if (info.st_uid != 0 or info.st_mode & 0o022 or
                not stat.S_ISREG(info.st_mode) or not 0 < info.st_size <= MAX_BYTES):
            reject('QUARANTINE_APPROVAL_ROOT_INVALID')
        with os.fdopen(fd, 'rb') as stream:
            fd = None
            raw = stream.read(MAX_BYTES + 1)
        if not 0 < len(raw) <= MAX_BYTES:
            reject('QUARANTINE_APPROVAL_ROOT_INVALID')
        return raw
    except OSError:
        reject('QUARANTINE_APPROVAL_ROOT_INVALID')
    finally:
        if fd is not None:
            os.close(fd)


def _openssl(args, data=None):
    try:
        result = subprocess.run(['openssl'] + args, input=data,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                timeout=10)
    except (OSError, subprocess.TimeoutExpired):
        reject('QUARANTINE_APPROVAL_SIGNATURE_INVALID')
    if result.returncode:
        reject('QUARANTINE_APPROVAL_SIGNATURE_INVALID')
    return result.stdout


def build_unsigned_candidate(entries, snapshots, release_sha, key_id,
                             not_before, expires_at, now):
    """Build a human-review candidate; contains no approval evidence/signature.

    `snapshots` must be the canonical DB projections from the protected live
    sampler. The function only hashes/serializes evidence; it never approves or
    signs. The result is intentionally not accepted by `verify`.
    """
    if (not _hex(release_sha, 40) or not _text(key_id) or
            not isinstance(now, datetime.datetime) or now.tzinfo is None):
        reject('QUARANTINE_CANDIDATE_INVALID')
    nb, exp = timestamp(not_before), timestamp(expires_at)
    now = now.astimezone(datetime.timezone.utc)
    if not nb <= now < exp or exp <= nb:
        reject('QUARANTINE_CANDIDATE_WINDOW_INVALID')
    if (not isinstance(entries, list) or not 0 < len(entries) <= 1000 or
            not isinstance(snapshots, dict)):
        reject('QUARANTINE_CANDIDATE_COUNT_INVALID')
    if {item.get('execution_id') for item in entries if isinstance(item, dict)} != set(snapshots):
        reject('QUARANTINE_CANDIDATE_COUNT_INVALID')

    records = []
    seen_ids, seen_attempts = set(), set()
    for item in sorted(entries, key=lambda value: value.get('execution_id', -1)):
        if (not isinstance(item, dict) or
                set(item) != {'execution_id', 'task_id', 'attempt', 'generation', 'task_generation'}):
            reject('QUARANTINE_CANDIDATE_IDENTITY_INVALID')
        for field in ('execution_id', 'attempt', 'task_generation'):
            if type(item.get(field)) is not int or not 0 < item[field] <= 9223372036854775807:
                reject('QUARANTINE_CANDIDATE_IDENTITY_INVALID')
        if item.get('generation') is not None and (type(item['generation']) is not int or
                not 0 < item['generation'] <= 9223372036854775807):
            reject('QUARANTINE_CANDIDATE_IDENTITY_INVALID')
        task_id = item.get('task_id')
        identity = (task_id, item['attempt'])
        if (not _text(task_id) or item['execution_id'] in seen_ids or identity in seen_attempts):
            reject('QUARANTINE_CANDIDATE_IDENTITY_INVALID')
        seen_ids.add(item['execution_id'])
        seen_attempts.add(identity)
        snapshot = snapshots[item['execution_id']]
        if (not isinstance(snapshot, dict) or
                snapshot.get('version') != LIVE_SNAPSHOT_VERSION or
                snapshot.get('scope') != 'CANONICAL_LIVE_SNAPSHOT' or
                snapshot.get('execution_id') != item['execution_id'] or
                snapshot.get('task_id') != task_id or
                snapshot.get('attempt') != item['attempt'] or
                snapshot.get('generation') != item['generation'] or
                snapshot.get('task_generation') != item['task_generation']):
            reject('QUARANTINE_CANDIDATE_SNAPSHOT_INVALID')
        validate_legacy_snapshot(item, snapshot)
        records.append({
            'identity': {key: item[key] for key in (
                'execution_id', 'task_id', 'attempt', 'generation', 'task_generation')},
            'snapshot_sha256': hashlib.sha256(canonical(snapshot)).hexdigest(),
            'snapshot': snapshot,
        })
    candidate = {
        'candidate_version': CANDIDATE_VERSION,
        'status': 'UNSIGNED_REQUIRES_HUMAN_REVIEW',
        'purpose': PURPOSE,
        'algorithm': ALGORITHM,
        'authority_id': AUTHORITY_ID,
        'key_id': key_id,
        'release_sha': release_sha,
        'operations': OPERATIONS,
        'not_before': not_before,
        'expires_at': expires_at,
        'record_count': len(records),
        'records': records,
    }
    encoded = canonical(candidate)
    if len(encoded) > MAX_CANDIDATE_BYTES:
        reject('QUARANTINE_CANDIDATE_TOO_LARGE')
    return candidate


def unsigned_manifest_bytes(candidate, review_bindings):
    """Bind human review references to a candidate; return canonical unsigned v2.

    This function has no signing capability. `verify` rejects its output until
    an independent authority appends a valid signature offline.
    """
    expected_candidate_keys = ['candidate_version', 'status', 'purpose', 'algorithm',
                               'authority_id', 'key_id', 'release_sha', 'operations',
                               'not_before', 'expires_at', 'record_count', 'records']
    _keys(candidate, expected_candidate_keys)
    if len(canonical(candidate)) > MAX_CANDIDATE_BYTES:
        reject('QUARANTINE_CANDIDATE_TOO_LARGE')
    if (candidate['candidate_version'] != CANDIDATE_VERSION or
            candidate['status'] != 'UNSIGNED_REQUIRES_HUMAN_REVIEW' or
            candidate['purpose'] != PURPOSE or candidate['algorithm'] != ALGORITHM or
            candidate['authority_id'] != AUTHORITY_ID or candidate['operations'] != OPERATIONS or
            not _hex(candidate['release_sha'], 40) or not _text(candidate['key_id'])):
        reject('QUARANTINE_CANDIDATE_INVALID')
    not_before, expires_at = timestamp(candidate['not_before']), timestamp(candidate['expires_at'])
    if not not_before < expires_at:
        reject('QUARANTINE_CANDIDATE_WINDOW_INVALID')
    records = candidate['records']
    if (not isinstance(records, list) or not 0 < len(records) <= 1000 or
            type(candidate['record_count']) is not int or
            candidate['record_count'] != len(records)):
        reject('QUARANTINE_CANDIDATE_COUNT_INVALID')
    if not isinstance(review_bindings, list) or len(review_bindings) != len(records):
        reject('QUARANTINE_REVIEW_BINDING_INVALID')
    bindings = {}
    for binding in review_bindings:
        _keys(binding, ['execution_id', 'approval_id', 'review_sha256'])
        if (type(binding['execution_id']) is not int or
                not _text(binding['approval_id']) or
                not _hex(binding['review_sha256'], 64) or
                binding['execution_id'] in bindings):
            reject('QUARANTINE_REVIEW_BINDING_INVALID')
        bindings[binding['execution_id']] = binding

    executions, seen_ids, seen_attempts, seen_approvals = [], set(), set(), set()
    for record in records:
        _keys(record, ['identity', 'snapshot_sha256', 'snapshot'])
        identity = record['identity']
        _keys(identity, ['execution_id', 'task_id', 'attempt', 'generation', 'task_generation'])
        for field in ('execution_id', 'attempt', 'task_generation'):
            if type(identity[field]) is not int or not 0 < identity[field] <= 9223372036854775807:
                reject('QUARANTINE_CANDIDATE_IDENTITY_INVALID')
        if identity['generation'] is not None and (type(identity['generation']) is not int or
                not 0 < identity['generation'] <= 9223372036854775807):
            reject('QUARANTINE_CANDIDATE_IDENTITY_INVALID')
        snapshot = record['snapshot']
        if (not _text(identity['task_id']) or not _hex(record['snapshot_sha256'], 64) or
                not isinstance(snapshot, dict) or
                snapshot.get('version') != LIVE_SNAPSHOT_VERSION or
                snapshot.get('scope') != 'CANONICAL_LIVE_SNAPSHOT' or
                any(snapshot.get(field) != identity[field] for field in
                    ('execution_id', 'task_id', 'attempt', 'generation', 'task_generation')) or
                hashlib.sha256(canonical(snapshot)).hexdigest() != record['snapshot_sha256']):
            reject('QUARANTINE_CANDIDATE_SNAPSHOT_INVALID')
        validate_legacy_snapshot(identity, snapshot)
        eid = identity['execution_id']
        binding = bindings.get(eid)
        attempt_key = (identity['task_id'], identity['attempt'])
        if (binding is None or eid in seen_ids or attempt_key in seen_attempts or
                binding['approval_id'] in seen_approvals):
            reject('QUARANTINE_CANDIDATE_IDENTITY_INVALID')
        seen_ids.add(eid)
        seen_attempts.add(attempt_key)
        seen_approvals.add(binding['approval_id'])
        evidence = {'approval_id': binding['approval_id'],
                    'snapshot_sha256': record['snapshot_sha256'],
                    'review_sha256': binding['review_sha256']}
        executions.append({
            'execution_id': eid,
            'task_id': identity['task_id'],
            'attempt': identity['attempt'],
            'generation': identity['generation'],
            'task_generation': identity['task_generation'],
            'snapshot_sha256': record['snapshot_sha256'],
            'evidence_sha256': hashlib.sha256(canonical(evidence)).hexdigest(),
            'evidence': evidence,
            'approval_id': binding['approval_id'],
            'release_sha': candidate['release_sha'],
            'not_before': candidate['not_before'],
            'expires_at': candidate['expires_at'],
        })
    if set(bindings) != seen_ids:
        reject('QUARANTINE_REVIEW_BINDING_INVALID')
    manifest = {
        'manifest_version': '2.0',
        'purpose': PURPOSE,
        'algorithm': ALGORITHM,
        'authority_id': AUTHORITY_ID,
        'key_id': candidate['key_id'],
        'release_sha': candidate['release_sha'],
        'operations': OPERATIONS,
        'approved_count': len(executions),
        'not_before': candidate['not_before'],
        'expires_at': candidate['expires_at'],
        'executions': executions,
    }
    raw = canonical(manifest)
    if len(raw) > MAX_BYTES:
        reject('QUARANTINE_CANDIDATE_TOO_LARGE')
    return raw


def _verify_signature(public_key, signature, payload):
    # Only frozen root-verified bytes, not mutable registry/key paths, are used.
    with tempfile.TemporaryDirectory(prefix='quarantine-approval-') as directory:
        key_path = os.path.join(directory, 'public.pem')
        signature_path = os.path.join(directory, 'signature.bin')
        with open(key_path, 'wb') as stream:
            stream.write(public_key)
        with open(signature_path, 'wb') as stream:
            stream.write(signature)
        der = _openssl(['pkey', '-pubin', '-in', key_path, '-outform', 'DER'])
        description = _openssl(['rsa', '-pubin', '-in', key_path, '-text', '-noout'])
        match = re.search(br'Public-Key: \((\d+) bit\)', description)
        if not match or not 3072 <= int(match.group(1)) <= 8192:
            reject('QUARANTINE_APPROVAL_SIGNATURE_INVALID')
        _openssl(['dgst', '-sha256', '-verify', key_path, '-signature', signature_path,
                  '-sigopt', 'rsa_padding_mode:pkcs1'], payload)
        return hashlib.sha256(der).hexdigest()


def verify(raw, release_sha, operation, now):
    """Verify signed authorization using a caller-supplied UTC clock.

    Drain/enrollment must supply the DB clock at their live validation boundaries.
    This function does not establish snapshot freshness or runtime isolation.
    """
    if operation not in OPERATIONS or not isinstance(now, datetime.datetime) or now.tzinfo is None:
        reject()
    manifest = decode(raw)
    _keys(manifest, ['manifest_version', 'purpose', 'algorithm', 'authority_id',
                     'key_id', 'release_sha', 'operations', 'approved_count',
                     'not_before', 'expires_at', 'executions', 'signature'])
    if (manifest['manifest_version'] != '2.0' or manifest['purpose'] != PURPOSE or
            manifest['algorithm'] != ALGORITHM or manifest['operations'] != OPERATIONS or
            not _hex(release_sha, 40) or release_sha == BLOCKED_CARRIER or
            manifest['release_sha'] != release_sha or
            manifest['authority_id'] != AUTHORITY_ID or not _text(manifest['key_id'])):
        reject()
    nb, exp = timestamp(manifest['not_before']), timestamp(manifest['expires_at'])
    if not nb <= now < exp:
        reject()
    entries = manifest['executions']
    if (not isinstance(entries, list) or not 0 < len(entries) <= 1000 or
            type(manifest['approved_count']) is not int or manifest['approved_count'] != len(entries)):
        reject()
    seen_ids, seen_attempts, seen_approvals = set(), set(), set()
    for item in entries:
        _keys(item, ['execution_id', 'task_id', 'attempt', 'generation', 'task_generation',
                     'snapshot_sha256', 'evidence_sha256', 'evidence', 'approval_id',
                     'release_sha', 'not_before', 'expires_at'])
        for field in ('execution_id', 'attempt', 'task_generation'):
            if type(item[field]) is not int or not 0 < item[field] <= 9223372036854775807:
                reject()
        if item['generation'] is None:
            if not legacy_identity_pinned(item):
                reject()
        elif type(item['generation']) is not int or not 0 < item['generation'] <= 9223372036854775807:
            reject()
        if not _text(item['task_id']) or not _text(item['approval_id']):
            reject()
        identity = (item['task_id'], item['attempt'])
        if (item['execution_id'] in seen_ids or identity in seen_attempts or
                item['approval_id'] in seen_approvals):
            reject()
        seen_ids.add(item['execution_id'])
        seen_attempts.add(identity)
        seen_approvals.add(item['approval_id'])
        if (item['release_sha'] != release_sha or item['not_before'] != manifest['not_before'] or
                item['expires_at'] != manifest['expires_at'] or not _hex(item['snapshot_sha256'], 64) or
                not _hex(item['evidence_sha256'], 64)):
            reject()
        evidence = item['evidence']
        _keys(evidence, ['approval_id', 'snapshot_sha256', 'review_sha256'])
        if (evidence['approval_id'] != item['approval_id'] or
                evidence['snapshot_sha256'] != item['snapshot_sha256'] or
                not _hex(evidence['review_sha256'], 64) or
                hashlib.sha256(canonical(evidence)).hexdigest() != item['evidence_sha256']):
            reject()
    check_runtime()
    registry = decode(_trusted_read(REGISTRY_PATH))
    _keys(registry, ['version', 'authorities'])
    if type(registry['version']) is not int or registry['version'] != 1 or not isinstance(registry['authorities'], list):
        reject()
    pinned = None
    seen = set()
    for authority in registry['authorities']:
        _keys(authority, ['authority_id', 'key_id', 'purpose', 'algorithm', 'public_key_file',
                          'public_key_sha256', 'revoked', 'not_before', 'expires_at'])
        if (authority['authority_id'] != AUTHORITY_ID or not _text(authority['key_id']) or
                authority['purpose'] != PURPOSE or authority['algorithm'] != ALGORITHM or
                type(authority['revoked']) is not bool or not _hex(authority['public_key_sha256'], 64) or
                not isinstance(authority['public_key_file'], str) or
                os.path.basename(authority['public_key_file']) != authority['public_key_file']):
            reject()
        identity = (authority['authority_id'], authority['key_id'])
        if identity in seen:
            reject()
        seen.add(identity)
        root_nb, root_exp = timestamp(authority['not_before']), timestamp(authority['expires_at'])
        if root_nb >= root_exp:
            reject()
        if identity == (manifest['authority_id'], manifest['key_id']):
            if authority['revoked'] or not root_nb <= nb <= now < exp <= root_exp:
                reject('QUARANTINE_APPROVAL_AUTHORITY_INVALID')
            pinned = authority
    if pinned is None:
        reject('QUARANTINE_APPROVAL_AUTHORITY_INVALID')
    public_key_path = os.path.join(os.path.dirname(REGISTRY_PATH), pinned['public_key_file'])
    public_key = _trusted_read(public_key_path)
    try:
        signature = base64.b64decode(manifest['signature'].encode('ascii'), validate=True)
    except (ValueError, AttributeError, UnicodeError):
        reject()
    if not 384 <= len(signature) <= 1024:
        reject()
    payload = dict(manifest)
    payload.pop('signature')
    fingerprint = _verify_signature(public_key, signature, canonical(payload))
    if fingerprint != pinned['public_key_sha256']:
        reject('QUARANTINE_APPROVAL_AUTHORITY_INVALID')
    return manifest
