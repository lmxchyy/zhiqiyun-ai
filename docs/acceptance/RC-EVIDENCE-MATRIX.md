# RC Evidence Matrix

- **采集时间**：2026-09-09
- **范围**：当前工作区 `main` / HEAD `bc00ae1f7`
- **模式**：只读证据收集；本轮不修改业务代码

## 证据摘要

| 项目 | 当前状态 | 缺失证据 | 验证方式 |
|---|---|---|---|
| Secrets / Secret rotation | `ROTATION_REQUIRED / BLOCKED` | 历史或本地环境中的真实凭据是否已全部失效、Provider/微信/云存储 Key 是否完成轮换 | 安全负责人提供轮换记录、旧 Key 失效验证、GitHub secret history 检查 |
| Payment | `BLOCKED / NOT VERIFIED` | 当前 release 的真实回调、官方查单补偿、人工补发、重复通知和失败重试；PostgreSQL test fixture | 启动匹配 migration 的独立 PostgreSQL，执行支付生命周期和真实 sandbox/生产白名单验证 |
| Connector | `PARTIAL` | 已有真实 Redis recovery/ack 证据；仍缺 failure exhaustion → DLQ、告警和人工恢复记录 | `XIANZHI_CONNECTOR_TEST_REDIS_URL=redis://127.0.0.1:63791/0 go test ... TestConnectorRedisQueueRecoveryAndAck`；补跑失败 DLQ 场景 |
| PPTX File Object | `PARTIAL / NOT VERIFIED` | 真实 API 导出、MinIO/R2 对象、任务 `storage://` reference、signed download HTTP 证据 | 启动 API + PostgreSQL + MinIO，创建 PPT、导出、重复下载并核对 `xz_file_objects` |
| Image Storage | `PARTIAL` | 本地 API 任务已得到 File Object/Asset/HTTP 200；Provider 为 mock/兼容路径，storage config 指向 R2，非 MinIO/生产白名单 | `task_000296` 证据见 `P1-ASSET-LIFECYCLE-EVIDENCE.md`；需真实 Provider 白名单 Canary |
| Backup Restore | `BLOCKED / NOT VERIFIED` | 当前 release 的远端 offsite upload、`OFFSITE_VERIFIED`、独立 PostgreSQL restore、校验和与 RPO/RTO | 生产备份或隔离副本执行 backup → offsite verify → clean restore → schema/data verification |
| API / Outbox / Worker | `PARTIAL / NOT VERIFIED` | API 与 generation-worker 已本地启动；仍缺该 task 的 Worker claim、smartvideo-worker、Outbox publish、lease/heartbeat/crash reclaim | 本地 `:3310` API + generation-worker 已采集；需完整 compose 和故障注入证据 |
| RabbitMQ / Redis / MinIO dependencies | `AVAILABLE LOCALLY` | 依赖容器与当前 release API/Worker 的真实连接和权限 | 当前采集：RabbitMQ ping、Redis PONG、MinIO health HTTP 200、PostgreSQL accepting connections |
| Release identity | `BLOCKED` | clean tree、推送 commit、构建物 digest 与运行实例一致 | 提交/推送当前变更，`git status --porcelain` 为空，执行 immutable release verification |

## 当前采集结果

```text
branch: main
origin/main drift: behind 5 commits
tracked env files: 0
local ignored env files: 1
tracked secret-pattern hit: .env.example only (placeholder/example values)
history regex hit: 0 by current bounded scan
PostgreSQL: accepting connections
RabbitMQ: ping succeeded
Redis: PONG
MinIO: HTTP 200
xianzhi API/generation-worker containers: NOT PRESENT
```

## 证据判定规则

- 依赖容器健康不等于业务 runtime 已运行。
- 本地 fake provider / fake object storage 测试不等于真实 Provider/远端存储证据。
- 代码测试通过不能替代支付、备份恢复、Worker crash/reclaim 的真实运行记录。
- 未完成 Secret 轮换或没有旧 Key 失效证明时，`SECRET_STATUS` 不得写成 `CLEAN`。
