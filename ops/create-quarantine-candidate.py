#!/usr/bin/env python3
"""Create a DB-backed unsigned quarantine candidate; never approves or signs."""
import argparse
import json
import os
import sys
import types

ROOT = os.path.dirname(os.path.dirname(os.path.realpath(__file__)))


def source_module(name, path):
    module = types.ModuleType(name)
    module.__file__ = path
    with open(path, 'rb') as source:
        exec(compile(source.read(), path, 'exec'), module.__dict__)
    return module


approval = source_module('quarantine_approval', os.path.join(ROOT, 'ops', 'quarantine-approval.py'))
live_snapshot = source_module('quarantine_live_snapshot', os.path.join(ROOT, 'ops', 'quarantine-live-snapshot.py'))
transport = source_module('quarantine_psql_transport', os.path.join(ROOT, 'ops', 'quarantine-psql-transport.py'))
live_snapshot._approval.REGISTRY_PATH = approval.REGISTRY_PATH


class ReadOnlyTarget:
    """Bind a single live postgres container/config without needing a Prestage proof."""
    def __init__(self, compose, env):
        self.compose = os.path.realpath(compose)
        self.env = os.path.realpath(env)
        self.expected = transport.binding(self.compose, self.env)
        self.check()

    def check(self):
        if transport.binding(self.compose, self.env) != self.expected:
            raise transport.TransportError('QUARANTINE_DB_BINDING_CHANGED')

    def connect(self):
        self.check()
        return transport.Connection(self)


def lock_present(prestage_dir):
    return any(os.path.lexists(os.path.join(prestage_dir, name))
               for name in ('release.lock', 'release.lock.recovering'))


def write_exclusive(path, data):
    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL
    if hasattr(os, 'O_NOFOLLOW'):
        flags |= os.O_NOFOLLOW
    fd = os.open(path, flags, 0o600)
    try:
        with os.fdopen(fd, 'wb') as output:
            fd = None
            output.write(data)
            output.flush()
            os.fsync(output.fileno())
    finally:
        if fd is not None:
            os.close(fd)


def parse_args(argv):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--compose', required=True)
    parser.add_argument('--env-file', required=True)
    parser.add_argument('--entries', required=True, help='JSON array of exact execution identities')
    parser.add_argument('--release-sha', required=True)
    parser.add_argument('--not-before', required=True)
    parser.add_argument('--expires-at', required=True)
    parser.add_argument('--key-id', default='UNPROVISIONED')
    parser.add_argument('--output', required=True)
    parser.add_argument('--prestage-dir', default=os.environ.get('PRESTAGE_DIR', '.prestage'))
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(sys.argv[1:] if argv is None else argv)
    try:
        entries_path = os.path.realpath(args.entries)
        with open(entries_path, 'rb') as source:
            raw = source.read(approval.MAX_BYTES + 1)
        if len(raw) > approval.MAX_BYTES:
            raise ValueError('entries input too large')
        entries = approval.decode(raw)
        if not isinstance(entries, list):
            raise ValueError('entries input must be an array')
        prestage_dir = args.prestage_dir
        if not os.path.isabs(prestage_dir):
            prestage_dir = os.path.join(ROOT, prestage_dir)
        if lock_present(prestage_dir):
            raise RuntimeError('SUPPRESSED_BY_RELEASE_LOCK')
        target = ReadOnlyTarget(args.compose, args.env_file)
        connection = target.connect()
        try:
            candidate = live_snapshot.sample_unsigned_candidate_read_only(
                connection, entries, args.release_sha, args.key_id,
                args.not_before, args.expires_at)
        finally:
            connection.close()
        target.check()
        if lock_present(prestage_dir):
            raise RuntimeError('SUPPRESSED_BY_RELEASE_LOCK')
        output_path = os.path.abspath(args.output)
        parent = os.path.dirname(output_path)
        if not os.path.isdir(parent):
            raise ValueError('output directory missing')
        output_path = os.path.join(os.path.realpath(parent), os.path.basename(output_path))
        root_path = os.path.realpath(ROOT)
        if output_path == root_path or output_path.startswith(root_path + os.sep):
            raise ValueError('candidate output must be outside the release worktree')
        write_exclusive(output_path, approval.canonical(candidate))
        print(json.dumps({'status': candidate['status'], 'authority_id': candidate['authority_id'],
                          'release_sha': candidate['release_sha'], 'record_count': candidate['record_count'],
                          'candidate_path': output_path}, sort_keys=True, separators=(',', ':')))
        return 0
    except Exception as error:
        # Never return database credentials, shell output, prompts or row values.
        message = str(error)
        if message not in ('SUPPRESSED_BY_RELEASE_LOCK', 'QUARANTINE_CANDIDATE_INVALID',
                           'QUARANTINE_CANDIDATE_WINDOW_INVALID', 'QUARANTINE_CANDIDATE_COUNT_INVALID',
                           'QUARANTINE_CANDIDATE_IDENTITY_INVALID', 'QUARANTINE_CANDIDATE_SNAPSHOT_INVALID',
                           'QUARANTINE_CANDIDATE_TOO_LARGE', 'QUARANTINE_DB_BINDING_CHANGED'):
            message = 'QUARANTINE_CANDIDATE_FAILED:' + type(error).__name__
        sys.stderr.write(message + '\n')
        return 1


if __name__ == '__main__':
    sys.exit(main())
