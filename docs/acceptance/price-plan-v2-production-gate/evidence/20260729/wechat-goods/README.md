# WeChat virtual goods evidence — 2026-07-29

## Observed (线上版本)

Screenshot: `../wechat-online-props-20260729.png` and `wechat-online-props-20260729.png` (copy under wechat-goods/).

| productId | price (¥) | note |
|---|---:|---|
| MEMBER_PRO_YEAR_996 | 996 | legacy / alternate |
| MEMBER_YEAR_996 | 996 | MEMBER NORMAL ✅ |
| AGENT_JOIN_996 | 996 | AGENT NORMAL ✅ (official restored) |
| IMAGE_PACK_1000 | 80 | unrelated |
| TOKEN_TEST_1FEN | 0.01 | Token only — NOT member/agent TEST |
| TOKEN_CUSTOM_1YUAN | 0.1 | Token only |

## Created (开发版本) + operator publish confirmation

Screenshot (dev list): `61-dev-list-both-tests.png` / `../wechat-dev-props-after-test-create.png`

| productId | name | price (¥) | remark |
|---|---|---:|---|
| MEMBER_TEST_1YUAN | 会员沙箱1元测试 | 1 | V2门禁TEST勿改价 |
| AGENT_TEST_1YUAN | 代理沙箱1元测试 | 1 | V2门禁TEST勿改价 |

Icon used for upload: `wechat-test-prop-icon-200x200.png` (200×200 PNG).

Official `MEMBER_YEAR_996` / `AGENT_JOIN_996` prices were **not** changed.

**Operator confirmation 2026-07-29:** 「道具已经创建完成并发布」— reconciles prior agent note that production online TEST rows were intentionally not created. Visual verify of **线上版本** list showing both TEST productIds is **optional pending** (no browser WeChat session this round; screenshot not captured under this folder).

## Remaining

- Dual-sign: price owner second signature still pending (do not invent PASS).
- V2 tables / bindings still absent → force-equality cannot be closed → production gate overall remains NO-GO.
- Optional: screenshot 线上版本 props list with `MEMBER_TEST_1YUAN` + `AGENT_TEST_1YUAN` into this folder.
- Do not mark WeChat gate PASS without dual-sign + force-equality.
