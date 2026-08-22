# Backup Retention Phase 2

`ops/backup-retention.sh` is a production-backup inventory and retention-policy dry run.

The default mode is dry-run. It only inventories files, classifies them, calculates
`KEEP`, `DELETE_CANDIDATE`, `MANUAL_REVIEW`, `ANALYZE_ONLY`, and out-of-scope results,
and reports expected reclaimable bytes plus a human-readable size. Phase 2 does not
support COS and does not delete, migrate, recompress, or restore backups.

## Safety boundaries

- `--apply` is intentionally unavailable and returns `APPLY_NOT_IMPLEMENTED`.
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

The JSON report contains `summary`, `keep`, `delete_candidates`, `manual_review`,
`analyze_only`, `out_of_scope`, `expected_reclaimed_bytes`, and `warnings`.
