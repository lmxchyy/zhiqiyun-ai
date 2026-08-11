# 生产迁移 097→100 证据（2026-07-29）

| 项 | 值 |
|---|---|
| 授权 | 用户已代行发布负责人 + DBA；本轮价格负责人双签后按 handoff 继续生产迁移（三开关保持 false） |
| 执行时间 | 2026-07-29 05:19+08（UTC 2026-07-28 21:19） |
| releaseCommit | `e8f191805ca1d6c9a4b214ee91312aeb796c0b10` |
| 库 | `zhiqiyun` / `zhiqiyun_prod` / Postgres 16.14 / 容器 `zhiqiyun-ai-prod-postgres-1` |
| 迁移来源 | `migrations-freeze/`（git archive 冻结包；**非**服务器工作树） |
| 结果 | **097→100 EXIT=0**；**VALIDATE 097/098/099/100 `STILL_NOT_VALID=0`** |
| V2 三开关 | 迁移前后均为 **false**（未改动） |

## SHA256（applied = release-manifest）

| 文件 | SHA256 |
|---|---|
| 097 | `784E6D2A3556CA0EA8B07287B5719D14F3DEDF76DD0228443A1C791FB87BB9E7` |
| 098 | `AD68192E66E026CE138283CADDC6FB066E60865926DCD46F2CE6BA304E8CF8E2` |
| 099 | `1D12CAD4D7927A851B72B267F6CC354EDB8FCF1B90A7EF963C8D3FD17B01C3A9` |
| 100 | `8646A68650838B4F501F8B8410D2D888DEB9661942F3DA927C85F9E202C68649` |

说明：服务器 `/opt/zhiqiyun-ai/database/migrations/` 当时仍为工作树 hash（FF49…），**未使用**；实际应用的是本目录 `migrations-freeze/`。

## 基线

| 指标 | 迁移前 | 迁移后 |
|---|---:|---:|
| orders | 51 | 51 |
| plans | 24 | 24 |
| amount_sum | 3484992 | 3484992 |
| xz_price_plans rows | 表不存在 | 0 |
| xz_wechat_virtual_goods rows | 表不存在 | 0 |
| bindings rows | 表不存在 | 0 |

## 耗时（秒）

| 步骤 | SEC |
|---|---:|
| 097 | 0 |
| 098 | 1 |
| 099 | 0 |
| 100 | 0 |
| VALIDATE（097–100 命名约束） | <2；全部 EXIT=0 |

## 明确未做

- 未开启任何 V2 开关
- 未创建 V2 pricePlan / good / binding 业务行 → **强制等式仍 BLOCKED**
- 未跑沙箱真机 QA → **禁止发明 §5 PASS**
- 未宣称总 GO

## 本地证据文件

`pre-migrate-snapshot.txt`、`migrate.log`、`migrate.meta`、`migration-sha256-applied.txt`、`post-migrate-baseline.txt`、`post-migrate-flags.txt`、`validate-fk.log`、`all-097-100-constraints.txt`、`still-not-valid-count.txt`、`migrations-freeze/*.sql`
