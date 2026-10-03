"""OFFLINE/HYPOTHETICAL ONLY. Not imported by deployment; no I/O or CLI.

trusted_scenarios is an independent *synthetic test oracle*, NOT an approval
file, signature verification service or runtime fence. The returned deployment
decision is always BLOCKED, even for a matching hypothetical scenario.
"""
import hashlib
import json
import re


FIELDS = {
    'execution_id', 'task_id', 'attempt', 'generation', 'execution_status',
    'provider_request_id', 'task_status', 'task_status_v2', 'lease_until',
    'owner', 'financial', 'assets', 'storage', 'results', 'correlation',
    'logs', 'snapshot_sha256', 'approval', 'release_sha', 'not_before', 'expires_at'
}
SNAPSHOT_FIELDS = FIELDS - {'snapshot_sha256', 'approval', 'release_sha', 'not_before', 'expires_at'}


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(',', ':'),
                                     allow_nan=False).encode('utf-8')).hexdigest()


def snapshot_digest(entry):
    return digest({key: entry[key] for key in SNAPSHOT_FIELDS})


def valid_entry(entry, release_sha, now):
    if not isinstance(entry, dict) or set(entry) != FIELDS:
        return False
    for key in ('execution_id', 'task_id', 'attempt', 'generation'):
        if type(entry[key]) is not int or entry[key] < 1:
            return False
    if entry['execution_status'] not in ('unknown', 'submitted'):
        return False
    if entry['task_status'] != 'FAILED' or entry['task_status_v2'] != 'FAILED':
        return False
    if entry['owner'] is not None:
        return False
    lease = entry['lease_until']
    if lease is not None and (type(lease) is not int or lease > now):
        return False
    request = entry['provider_request_id']
    if request is not None and (not isinstance(request, str) or not request.strip()):
        return False
    for key in ('financial', 'assets', 'storage', 'results', 'correlation', 'logs'):
        evidence = entry[key]
        if not isinstance(evidence, dict) or set(evidence) != {'source', 'sha256'}:
            return False
        if not isinstance(evidence['source'], str) or not evidence['source'].strip():
            return False
        if not isinstance(evidence['sha256'], str) or not re.fullmatch('[0-9a-f]{64}', evidence['sha256']):
            return False
    approval = entry['approval']
    if not isinstance(approval, dict) or set(approval) != {'identity', 'time', 'reference'}:
        return False
    for key in ('identity', 'reference'):
        if not isinstance(approval[key], str) or not approval[key].strip():
            return False
    if type(approval['time']) is not int or approval['time'] > now:
        return False
    if not isinstance(release_sha, str) or not re.fullmatch('[0-9a-f]{40}', release_sha):
        return False
    if entry['release_sha'] != release_sha:
        return False
    if type(entry['not_before']) is not int or type(entry['expires_at']) is not int:
        return False
    return (entry['not_before'] <= now < entry['expires_at']
            and entry['snapshot_sha256'] == snapshot_digest(entry))


def evaluate_hypothetical(entries, requested_ids, trusted_scenarios, release_sha, now):
    """Exclude only exact oracle-bound requests in a synthetic offline model.

    Each oracle value has entry_sha256 and recovery_isolated. The latter models
    an unimplemented restart/cutover fence; it never establishes a real fence.
    Untrusted JSON cannot manufacture an entry in this independent test oracle.
    Malformed observations invalidate the entire evaluation, not just one row.
    """
    blocked = {'deployment': 'BLOCKED', 'model': 'BLOCKED', 'excluded': [], 'remaining': []}
    try:
        if type(now) is not int or not isinstance(entries, list) or not entries:
            return blocked
        if not isinstance(requested_ids, list) or any(type(i) is not int for i in requested_ids):
            return blocked
        if not all(valid_entry(e, release_sha, now) for e in entries):
            return blocked
        ids = [e['execution_id'] for e in entries]
        tasks = [e['task_id'] for e in entries]
        if len(set(ids)) != len(ids) or len(set(tasks)) != len(tasks):
            return blocked
        blocked['remaining'] = sorted(ids)
        if len(set(requested_ids)) != len(requested_ids) or not set(requested_ids).issubset(ids):
            return blocked
        excluded = []
        for entry in entries:
            eid = entry['execution_id']
            oracle = trusted_scenarios.get(eid, {})
            if (eid in requested_ids and oracle.get('entry_sha256') == digest(entry)
                    and oracle.get('recovery_isolated') is True):
                excluded.append(eid)
        remaining = sorted(set(ids) - set(excluded))
        return {'deployment': 'BLOCKED', 'model': 'BLOCKED' if remaining else 'HYPOTHETICAL_ONLY',
                'excluded': sorted(excluded), 'remaining': remaining}
    except (KeyError, TypeError, ValueError, AttributeError):
        return blocked
