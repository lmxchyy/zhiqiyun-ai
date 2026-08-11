# 代理正式价恢复记录（2026-07-29）

| 项 | 值 |
|---|---|
| 操作 | 生产 `xz_plans` 恢复代理正式价 |
| 计划 | `plan_agent_join_996` |
| 变更前 | `price_cents=100`, `grant_points=100`, `raw.priceCents=100` |
| 变更后 | `price_cents=99600`, `grant_points=20000`, `raw.priceCents=99600`, `entitlements.creditUnits=20000` |
| 未改 | `wechat_product_id=AGENT_JOIN_996`（正式/沙箱映射保持）；三开关仍 false；未跑 097–100 |
| 依据 | 价格负责人确认：正式 ¥996，¥1 仅为临时测试 |
| SQL | `_agent_price_restore_996.sql` |
| 结果 | `restored_rows=1`，COMMIT 成功 |
