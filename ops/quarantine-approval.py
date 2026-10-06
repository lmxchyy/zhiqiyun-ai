#!/usr/bin/env python3
"""Independent quarantine approval verifier; never a proof-key or live-state oracle.

Production trust is ONLY the root-provisioned registry below. No CLI/environment
root override. Private signing keys must not be provisioned on the release host.
The caller must separately recompute the live snapshot and check the DB clock.
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

REGISTRY_PATH = '/etc/zhiqiyun/quarantine-approval/registry.json'
PURPOSE = 'quarantine-enrollment'
OPERATIONS = ['drain-exemption', 'enroll']
ALGORITHM = 'RSA-PKCS1-v1_5-SHA256'
BLOCKED_CARRIER = 'f9cdf44ca79272ad7cead33dfb1d35fdf155f05f'
MAX_BYTES = 1048576


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
            not _text(manifest['authority_id']) or not _text(manifest['key_id'])):
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
        for field in ('execution_id', 'attempt', 'generation', 'task_generation'):
            if type(item[field]) is not int or not 0 < item[field] <= 9223372036854775807:
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
        if (not _text(authority['authority_id']) or not _text(authority['key_id']) or
                authority['purpose'] != PURPOSE or authority['algorithm'] != ALGORITHM or
                type(authority['revoked']) is not bool or not _hex(authority['public_key_sha256'], 64)):
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
    public_key = _trusted_read(pinned['public_key_file'])
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
