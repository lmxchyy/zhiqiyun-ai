# Operation Center 089-096 production-copy rehearsal difference report

- Environment: xz_rehearsal_prodcopy / production-sanitized-copy
- Backup reference: prod-live-sanitized-20260726T210342Z
- Change request: CR-2026-OC-008
- Gate passed: True
- Inferred schema before: 088
- Rollout fingerprint unchanged: True
- Rule fingerprint unchanged: True
- Historical order count: 47 -> 47
- Source: ai.zs-kjhn.cn / zhiqiyun live dump + sanitize-v1.0 + migrations 079-088 then 089-096

## Pre-steps
- Restored production live dump to xz_rehearsal_prodcopy
- Applied sanitize-v1.0 (null password_hash, mask mobile)
- Applied migrations 079-088 successfully (10 files)

## Migration timings
- 089-operation-center-lifecycle-refund-saga.sql: 731 ms; lock risk=LOW; waiting locks=0 -> 0
- 090-operation-center-referral-eligibility.sql: 124 ms; lock risk=LOW; waiting locks=0 -> 0
- 091-operation-center-referral-reward-grant.sql: 109 ms; lock risk=LOW; waiting locks=0 -> 0
- 092-operation-center-referral-reward-release.sql: 24 ms; lock risk=LOW; waiting locks=0 -> 0
- 093-operation-center-referral-reward-reversal.sql: 45 ms; lock risk=LOW; waiting locks=0 -> 0
- 094-operation-center-refund-saga-orchestrator.sql: 15 ms; lock risk=LOW; waiting locks=0 -> 0
- 095-payment-refund-adapter-query-verification.sql: 19 ms; lock risk=LOW; waiting locks=0 -> 0
- 096-operation-center-refund-management.sql: 136 ms; lock risk=LOW; waiting locks=0 -> 0

## Relation size deltas (top growth)
- xz_operation_center_refund_tasks: 0 -> 106496 (delta=106496)
- xz_commission_wallet_ledger: 40960 -> 106496 (delta=65536)
- xz_referral_eligibilities: 0 -> 65536 (delta=65536)
- xz_referral_reward_release_tasks: 0 -> 57344 (delta=57344)
- xz_operation_center_state_transitions: 0 -> 49152 (delta=49152)
- xz_operation_center_review_events: 0 -> 49152 (delta=49152)
- xz_operation_center_manual_refunds: 0 -> 40960 (delta=40960)
- xz_operation_center_service_orders: 40960 -> 81920 (delta=40960)
- xz_operation_center_refund_request_events: 0 -> 40960 (delta=40960)
- xz_referral_rewards: 49152 -> 90112 (delta=40960)
- xz_operation_center_manual_refund_events: 0 -> 32768 (delta=32768)
- xz_operation_center_review_events_tenant_id_idempotency_key_key: 0 -> 8192 (delta=8192)
- xz_referral_eligibilities_idempotency_key_key: 0 -> 8192 (delta=8192)
- xz_operation_center_review_events_pkey: 0 -> 8192 (delta=8192)
- xz_referral_reward_release_tasks_pkey: 0 -> 8192 (delta=8192)
- xz_referral_reward_release_tasks_referral_reward_id_key: 0 -> 8192 (delta=8192)
- xz_operation_center_state_tra_entity_type_entity_id_idempot_key: 0 -> 8192 (delta=8192)
- xz_operation_center_state_transitions_pkey: 0 -> 8192 (delta=8192)
- xz_referral_reward_release_tasks_tenant_id_idempotency_key_key: 0 -> 8192 (delta=8192)
- xz_operation_center_manual_refund_tenant_id_idempotency_key_key: 0 -> 8192 (delta=8192)

## Notes
- This rehearsal uses a real production data-volume dump under CR-2026-OC-008.
- Production schema/data were not modified.
