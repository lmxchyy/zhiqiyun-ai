# Operation Center 089-096 migration rehearsal difference report

- Environment: isolated pgvector/pgvector:pg16 synthetic production-structure copy
- Backup reference: synthetic-088-structure-copy-20260727
- Gate passed: True
- Inferred schema before: 088
- Rollout fingerprint unchanged: True
- Rule fingerprint unchanged: True
- Historical order count: 0 -> 0

## Migration timings
- 089-operation-center-lifecycle-refund-saga.sql: 504 ms; lock risk=LOW; waiting locks=0 -> 0
- 090-operation-center-referral-eligibility.sql: 59 ms; lock risk=LOW; waiting locks=0 -> 0
- 091-operation-center-referral-reward-grant.sql: 51 ms; lock risk=LOW; waiting locks=0 -> 0
- 092-operation-center-referral-reward-release.sql: 25 ms; lock risk=LOW; waiting locks=0 -> 0
- 093-operation-center-referral-reward-reversal.sql: 50 ms; lock risk=LOW; waiting locks=0 -> 0
- 094-operation-center-refund-saga-orchestrator.sql: 17 ms; lock risk=LOW; waiting locks=0 -> 0
- 095-payment-refund-adapter-query-verification.sql: 21 ms; lock risk=LOW; waiting locks=0 -> 0
- 096-operation-center-refund-management.sql: 189 ms; lock risk=LOW; waiting locks=0 -> 0

## Limitation
- This is a schema-faithful synthetic copy, not a sanitized production data-volume backup. Real table cardinality, disk growth and lock duration must be rehearsed again before production approval.
