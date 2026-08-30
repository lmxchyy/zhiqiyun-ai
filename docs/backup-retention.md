# Backup Retention Phase 2

`ops/backup-retention.sh` is a production-backup inventory and retention-policy command.
The default mode is a dry run; `--apply` delegates to the bounded controlled apply
implementation and is intended for an explicit, reviewed production window only.

The default mode is dry-run. It only inventories files, classifies them, calculates
`KEEP`, `DELETE_CANDIDATE`, `MANUAL_REVIEW`, `ANALYZE_ONLY`, and out-of-scope results,
and reports expected reclaimable bytes plus a human-readable size. Phase 2 does not
support COS and does not delete, migrate, recompress, or restore backups.

## Safety boundaries

- `--apply` requires positive `--max-count` and `--max-bytes`, enforces a maximum
  controlled batch of five files, and acquires an exclusive `flock`.
- Event backups are `MANUAL_REVIEW` and never automatic delete candidates.
- `backups/releases` and `backups/release` are `ANALYZE_ONLY`.
- Symlinks are skipped and are never followed for inventory or deletion planning.
- The root must exist, resolve to a directory, and must not be `/`, `/opt`, or
  `/opt/zhiqiyun-ai`.
- Production use is limited to `/opt/zhiqiyun-ai/backups`; a non-production
  root must be a test temporary directory.

## Retention rules

- Deploy database backups: newest five; older compatible `db_*.sql` and
  `db_*.sql.gz` files become candidates.
- Daily `xianzhi-*.sql`/`.sql.gz`: files modified within 14 days are kept.
- Weekly: the newest or already-kept file covers each of the newest four natural
  weeks when a candidate exists.
- Monthly: the newest or already-kept file covers each of the newest three
  calendar months when a candidate exists.
- Compose backups: newest 20 kept.
- Deploy logs: files modified within 30 days kept; older files are candidates.
- Unknown paths and configured out-of-scope paths are kept.

## Examples

```bash
./ops/backup-retention.sh
./ops/backup-retention.sh --json
./ops/backup-retention.sh --root /tmp/example-backups --now 2026-08-22T15:52:44+08:00 --json
```

The JSON report contains `summary`, `keep`, `delete_candidates`, `delete_eligible`,
`delete_eligible_bytes`, `manual_review`, `analyze_only`, `out_of_scope`,
`expected_reclaimed_bytes`, and `warnings`.

## Off-site deletion gate

`delete_candidates` is only the retention-policy result. Each candidate is also
checked against its adjacent `.offsite.json` evidence and the current local
file. `delete_eligible` contains only files whose evidence is exactly
`OFFSITE_VERIFIED`, whose recorded remote size and SHA256 agree, and whose
current local size and SHA256 still agree. The evidence must also contain a
verification timestamp and an object key under `backups/postgres/` ending in
the exact local backup filename. Missing, malformed, stale, or
inconsistent evidence remains local-only and is never fail-open eligible.

The apply command consumes only a stable, oldest-first prefix of `delete_eligible`
within both explicit limits. It writes an immutable manifest containing absolute
path, byte count, SHA256, remote object key, and retention reason. Immediately
before each deletion it rechecks the exact regular file, size, SHA256, current
retention report, and a live read-only OBS HEAD through the dedicated uploader;
it then invokes one exact `rm -- <path>` and stops on the first failure. A
missing verifier configuration fails closed. The post-delete check repeats the
same live HEAD. It never deletes `.meta.json`, `.sha256`, `.offsite.json`, or any
OBS object. After the batch it runs another dry run and reports the audit record;
sidecar cleanup remains a follow-up because sidecars are not defined as the local
retention unit by this contract.
