# RC Exit Plan

- **制定时间**：2026-09-10
- **依据**：`docs/acceptance/RC-FREEZE-REPORT.md`
- **模式**：冻结态；本文件不授权继续开发，也不改变当前 blocker 状态。

## 1. 当前冻结状态

```text
RC_FREEZE_STATUS=PASS
RC_SCORE=NOT_COMPUTABLE_WITH_CURRENT_REPOSITORY_RULES
P0_BLOCKERS=1
P1_BLOCKERS=8
EXTERNAL_BLOCKERS=3
ENVIRONMENT_BLOCKERS=6
PRODUCTION_READY=NO
```

冻结期间禁止：

- 修改业务逻辑、增加功能或扩大测试范围；
- 用 mock、旧 release 或依赖容器健康替代生产证据；
- 删除、重置或覆盖当前 dirty/untracked 工作区；
- 在没有单一已推送 release commit 的情况下部署。

## 2. 已关闭能力（证据边界）

下列项目已达到代码级或专项验收的 `VERIFIED` 范围；“已关闭”不自动代表生产发布完成：

- `VIDEO_ASYNC` 专项能力；
- 视频资产持久化方案、Artifact → File Object → Asset、签名访问和幂等存储的代码级能力；
- 历史视频黑屏的根因定位与失效防守代码；`VIDEO-P1-001` 用户体验已由真实 H5 E2E 关闭，旧 Provider 文件本体标记为 `NOT_RECOVERABLE`；
- 图片资产的 fail-closed 代码路径与已有本地证据；
- 智能混剪代码/测试专项的既有 PASS 证据；
- Connector 重试/DLQ 代码路径；
- 虚拟支付 fulfillment projection 代码路径；
- PPTX File Object 代码路径；
- dirty/untracked 发布门禁代码路径。

仍需注意：PPTX 真实对象存储 E2E、真实图片 Provider Canary、Connector 真实 DLQ 和生产 Worker 运行证据尚未关闭；旧 Provider 历史文件本体不可恢复不作为用户体验 blocker。

## 3. Remaining blockers

### P0

| Blocker | 状态 | 负责人 | 执行条件 | 需要环境 | 完成标准 | 验证命令/步骤 |
|---|---|---|---|---|---|---|
| Secret Rotation | `BLOCKED_EXTERNAL` | 安全负责人 / IAM 管理员 | 获得生产 Secret/IAM 操作授权 | Provider、微信、对象存储、数据库、Connector 的生产密钥系统 | 所有生产凭据轮换；旧凭据失效；GitHub secret history/repository secret 检查完成；无 Secret 值进入仓库 | 由安全负责人提供脱敏轮换记录、旧 Key 失效结果和时间戳；禁止在本地输出 Secret |

### P1

| Blocker | 状态 | 负责人 | 执行条件 | 需要环境 | 完成标准 | 验证命令/步骤 |
|---|---|---|---|---|---|---|
| API runtime evidence | `BLOCKED_ENVIRONMENT` | 发布平台 / SRE | 当前 release 镜像已构建并启动 | API 容器、健康端点、配置与数据库/队列权限 | API process、port、health、release commit/digest 可核对 | `docker compose ps`；`curl -fsS http://<api>/api/v1/health`；记录镜像 digest 与 HEAD |
| generation-worker runtime evidence | `BLOCKED_ENVIRONMENT` | 发布平台 / SRE | Worker 镜像和队列配置就绪 | RabbitMQ、generation-worker、consumer、lease/heartbeat | 同一 task 完成 Outbox → RabbitMQ → claim → completion 证据 | `docker compose ps`；RabbitMQ consumer 查询；采集 task/event/consumer 日志 |
| smartvideo-worker runtime evidence | `BLOCKED_ENVIRONMENT` | 发布平台 / SRE / 混剪 owner | 智能混剪 Worker 发布并接入当前 release | smartvideo-worker、队列、对象存储、积分账务 | 同一 render task 完成 render → File Object → signed download → Capture/Release 对账 | `docker compose ps`；worker health/log；数据库、对象存储和账本查询 |
| Payment integration evidence（含 fulfillment projection） | `BLOCKED_EXTERNAL` | 支付运营 / 后端 / DBA | 获得 sandbox/白名单权限并准备匹配 migrations | 微信虚拟支付回调、查单、人工补发、独立 PostgreSQL fixture | 当前 release 完成签名/金额/商品快照、重复通知幂等、查单补偿、人工补发、paid→fulfillment→ledger 对账 | 执行最小支付生命周期集成套件；保存订单号、event ID、账本/fulfillment 结果，不保存支付密钥 |
| Connector DLQ evidence | `BLOCKED_ENVIRONMENT` | Connector owner / SRE | 可控 failure injection 与告警权限可用 | Redis、Connector worker、DLQ、告警和人工恢复工具 | 失败 1→2→3、durable DLQ、告警、人工恢复、幂等重投全链路成立 | 使用隔离 Redis 执行 failure exhaustion；查询 DLQ/attempts/failed state；保存恢复记录 |
| Backup restore evidence | `BLOCKED_EXTERNAL` | SRE / DBA / 对象存储管理员 | 获得当前生产备份和 offsite/restore 授权 | 生产备份副本、OBS/R2/COS、独立 PostgreSQL restore 库 | `OFFSITE_VERIFIED`、clean restore、关键表校验、RPO/RTO 记录 | backup → offsite verify → clean restore → schema/data/File Object/Outbox/ledger 校验 |
| Release identity | `BLOCKED_ENVIRONMENT` | Release owner | 变更获得审阅和提交/推送授权 | 单一 Git commit、CI 构建、immutable image registry、运行实例 | clean tree、已推送 commit、镜像 digest、运行实例四者一致 | `git status --porcelain`；`git rev-parse HEAD origin/main`；镜像 digest/运行实例核对 |
| PPTX storage E2E | `BLOCKED_ENVIRONMENT` | 后端 / 平台 owner | 当前 release API 和对象存储运行 | PostgreSQL、MinIO/R2、File Object、signed download | PPT 导出→File Object→storage reference→HTTP 200→重复导出幂等 | 执行最小 PPT E2E；查询 `xz_file_objects`、task raw state 和 signed download |
| History Video User Experience | `CLOSED` | 视频 owner | 隔离 EXPIRED fixture 与 H5 同源认证环境已就绪 | 真实 H5 作品中心 | 不下发第三方 URL，显示 EXPIRED 失效卡片并提供重新生成入口 | 见 `HISTORY-VIDEO-REAL-PLAYBACK-E2E.md`；API、DOM/video src、媒体请求和重新生成证据均已记录 |
| Image Provider/storage Canary | `BLOCKED_ENVIRONMENT` | Provider / 平台 owner | 白名单 Provider 与账务测试环境可用 | 真实 Provider、MinIO/R2、积分 freeze/capture/release、故障注入 | 成功和失败路径均不回退 Provider URL；Asset/File Object/HTTP 200/账务结果一致 | 执行白名单 Canary；记录 task、file、asset、HTTP 和账务查询结果 |

> P1 当前为 8 个未关闭逻辑 blocker：历史视频用户体验项已由真实 H5 E2E 关闭；Payment integration 同时覆盖 `PAY-P1-001` 与 `PAY-P1-002`。PPTX、Image 等剩余项目仍需真实环境证据，不能因为代码路径已实现就伪造关闭。

## 4. RC 恢复验收条件

只有同时满足以下条件，才允许从 Freeze 状态重新打开 RC 验收：

```text
P0_BLOCKERS=0
P1_BLOCKERS=0
PRODUCTION_EVIDENCE_COMPLETE=YES
RELEASE_IDENTITY_VERIFIED=YES
```

并且必须具备：

1. 当前 release 的单一已推送 commit、immutable image digest 和运行实例一致；
2. Secret 轮换及旧凭据失效证据；
3. API、generation-worker、smartvideo-worker 的真实运行和 consumer 证据；
4. 支付、Connector DLQ、PPTX、图片 Canary 的当前 release 证据；
5. 备份 offsite verify、独立恢复和 RPO/RTO 证据；
6. Protected Surfaces 相关回归按实际变更范围执行并留存结果；
7. RC 评分规则版本和逐项分值已被项目 owner 批准。

## 5. RC scoring rule draft

评分草案见 `docs/acceptance/RC-SCORING-RULE.md`。在该规则被批准、版本化并绑定完整 criterion inventory 之前，`RC_SCORE` 必须保持：

```text
RC_SCORE=NOT_COMPUTABLE_WITH_CURRENT_REPOSITORY_RULES
```

不得根据“关闭了几个问题”自行推导新的数值分数。

## 6. 恢复入口

收到外部凭据、真实运行环境或授权的 release identity 证据后：

1. 不直接修改冻结工作区；先由 Release owner 建立可审计的 release checkout。
2. 将外部证据绑定到该 checkout 的 commit/digest。
3. 只执行对应 blocker 的最小验证命令。
4. 更新证据矩阵、逐项状态和评分版本。
5. 仅在 P0/P1 全部关闭后重新生成 RC Final Audit；此前不生产放量。
