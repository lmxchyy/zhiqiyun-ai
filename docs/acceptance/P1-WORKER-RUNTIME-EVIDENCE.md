# P1 Worker Runtime Evidence

- **采集时间**：2026-09-09
- **环境**：本地隔离 API `:3310` + generation-worker；依赖使用本地 PostgreSQL/RabbitMQ/Redis，文件服务实际解析到数据库中的 R2 storage config
- **结论**：`WORKER_CLAIM_EVIDENCE=PASS / FULL_RUNTIME=PARTIAL`

## 已验证

| 链路 | 证据 | 结果 |
|---|---|---|
| API process | API 日志：`xianzhi-ai go gin api listening on :3310` | PASS |
| API health | `GET /api/v1/health` HTTP 200 | PASS |
| RabbitMQ connection | API/worker 日志：`[messaging] connected to rabbitmq` | PASS |
| generation-worker process | `cmd/generation-worker` 在 `ASYNC_MESSAGING_ENABLED=true`、`PROVIDER_EXECUTION_SAFETY_ENABLED=true` 下保持运行 | PASS |
| API admission | `task_000296`, client request `rc-evidence-20260909-005` | PASS |
| PostgreSQL task state | `xz_generation_tasks`: `SUCCEEDED`, `billing_status=CAPTURED` | PASS |
| File Object | `file_b0b9d6e3c326991bcee9da07`, 1,795,927 bytes, `ACTIVE`（验证时） | PASS |
| Asset | `asset_000223`, `storageManaged=true`, `fileId` 绑定 | PASS |
| Download | asset download HTTP 200, `image/png`, 1,795,927 bytes | PASS |

## Worker Claim 专项证据

为排除 API 内部 canary consumer 干扰，本次单独启动了 API publisher-only（`PROVIDER_EXECUTION_SAFETY_ENABLED=false`，API consumer 不注册）和 generation-worker。随后创建独立 terminal canary task，并通过以下链路验证：

```text
outbox_events.status=pending
  -> outbox_events.status=published
  -> RabbitMQ x.ai.generation.image.canary
  -> generation-worker consumer
  -> consumer_inbox.consumer_name=generation-image-canary-worker
  -> consumer_inbox.result=completed
```

证据记录：

```text
event_id: generation.image.requested:rc-worker-chain-20260909
outbox: published
rabbit consumer count: 1
action: consumer_inbox claim + complete
```

该专项任务使用 terminal 状态以隔离 Worker claim，不执行 Provider 或账务副作用。因此它关闭的是 **Worker claim / Outbox publisher 证据缺口**，不是完整 Provider processing Canary。

## 重要边界

- 本次 Provider 使用本地配置的可运行 mock/兼容路径，不是生产 Provider credential。
- task 日志由 API 内部 generation runtime 完成；generation-worker 日志只证明进程已连接 RabbitMQ，未出现该 task 的 Worker claim/consume 记录。
- 该 task 的 File Object storage config 解析为 R2 endpoint/bucket，而不是本地 MinIO；因此本次不能标记“MinIO Worker Canary PASS”。
- 测试资产已通过 API soft delete + permanent delete 清理；File Object 当前保留 `DELETED` recycle 记录，成功 task 仍保留，因为成功任务不能通过用户 API 删除。

## 未验证

```text
API -> Outbox -> RabbitMQ -> generation-worker claim -> Provider -> Artifact -> Storage -> Asset
```

其中 Worker claim、真实 Provider、真实白名单账号、失败重试、积分 release/capture 对账未形成同一 task 的完整证据链。

## 状态

`WORKER_CLAIM_EVIDENCE=PASS`

`FULL_RUNTIME_PROVIDER_AND_SMARTVIDEO_WORKER=NOT_VERIFIED`
