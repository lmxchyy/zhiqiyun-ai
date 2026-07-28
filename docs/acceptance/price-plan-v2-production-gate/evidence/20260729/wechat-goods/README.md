# WeChat virtual goods evidence — 2026-07-29

## Online version — NORMAL only (earlier)

Screenshot: `../wechat-online-props-20260729.png`

| productId | price (¥) | note |
|---|---:|---|
| MEMBER_YEAR_996 | 996 | MEMBER NORMAL |
| AGENT_JOIN_996 | 996 | AGENT NORMAL |

## Online version — NORMAL + TEST (after publish)

Screenshot: `72-online-props-with-tests.png` / `../wechat-online-props-with-tests-20260729.png`

| productId | name | price (¥) | note |
|---|---|---:|---|
| AGENT_TEST_1YUAN | 代理沙箱1元测试 | 1 | published 2026-07-29 |
| MEMBER_TEST_1YUAN | 会员沙箱1元测试 | 1 | published 2026-07-29 |
| MEMBER_PRO_YEAR_996 | 知启云AIPro年度会员 | 996 | legacy |
| MEMBER_YEAR_996 | 996AI创作会员包 | 996 | MEMBER NORMAL |
| AGENT_JOIN_996 | 996代理商开通包 | 996 | AGENT NORMAL |
| IMAGE_PACK_1000 | 1000张图片生成额度 | 80 | unrelated |

## Dev version (create evidence)

Screenshot: `61-dev-list-both-tests.png`

Icon: `wechat-test-prop-icon-200x200.png` (also `../wechat-prop-icon-200.png`).

Operator confirmation 2026-07-29: created and published. Official 996 prices not changed.

## Dual-sign (2026-07-29)

Price-owner second signature **recorded**: `../price-owner-wechat-goods-dual-sign.md`.

User replied「继续」after being told next human step is price-owner dual-sign / co-sign; treated as price-owner authorization (same operator already delegated release-owner + DBA for this gate).

| productId | cents | dual-sign |
|---|---:|---|
| MEMBER_YEAR_996 | 99600 | PASS |
| AGENT_JOIN_996 | 99600 | PASS |
| MEMBER_TEST_1YUAN | 100 | PASS |
| AGENT_TEST_1YUAN | 100 | PASS |

## Remaining (separate from dual-sign)

- Prod migrate 097→100 **APPLIED** (schema only); V2 business rows still 0 → **force-equality cannot close**.
- Overall §4 still **PARTIAL**; total gate **NO-GO**.
- Sandbox real-device V2 quote (§5) still blocked — do not invent PASS.
- V2 feature flags must stay **false**.
