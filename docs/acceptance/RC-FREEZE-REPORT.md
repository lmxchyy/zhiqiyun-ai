# RC Freeze & Evidence Closure Report

- **冻结时间**：2026-09-10T02:31:41+08:00
- **模式**：RC Freeze & Evidence Closure Loop
- **本轮原则**：只读验证与证据归档；未修改业务逻辑、未新增功能、未重跑已 PASS 的 VIDEO_ASYNC / 视频资产 / 图片资产全量测试。
- **工作区基线**：冻结前 44 个 dirty/untracked 条目，详见 `RC-WORKTREE-FREEZE.md`。

## 1. 当前代码修复完成项

以下是已有工作区内容的代码级结果。本报告不把代码测试或本地 mock 证据升级为生产运行证据：

| 项目 | 当前冻结状态 | 边界 |
|---|---|---|
| 视频资产持久化、历史播放防守、签名访问、幂等 | `BLOCKED_ENVIRONMENT` | 代码级方案与 Canary 证据存在；真实用户历史视频仍黑屏/停滞，实际详情 runtime 尚未验证 |
| Connector 有界重试、失败状态、DLQ 代码路径 | `VERIFIED` | 代码与本地测试证据存在；真实 Redis failure exhaustion/DLQ/人工恢复仍为环境阻断 |
| 虚拟支付 fulfillment projection 与管理员重试分流 | `VERIFIED` | 代码级修复和编译/单测范围已记录；真实支付与匹配数据库证据仍未闭环 |
| PPTX File Object / storage reference / signed access | `VERIFIED` | 代码级实现和相关测试证据存在；真实 API + PostgreSQL + 对象存储 E2E 未闭环 |
| 图片存储 fail-closed | `VERIFIED` | 代码级 fail-closed 和本地证据存在；真实 Provider/对象存储 Canary 未闭环 |
| dirty/untracked 发布门禁 | `VERIFIED` | `deploy.sh` 已有门禁改动；当前工作区仍 dirty，不能因此宣布 release identity 已关闭 |

## 2. 运行证据检查

只读检查结果：

| 组件 | PROCESS_PRESENT | PORT_LISTENING | QUEUE_CONSUMER_PRESENT | HEALTH_STATUS |
|---|---|---|---|---|
| API (`xianzhi-ai`) | `NO` | `NO`（未发现业务 API 监听；`3310` 无监听） | `NO` | `BLOCKED_ENVIRONMENT` |
| `generation-worker` | `NO` | `NO` | `NO`（RabbitMQ `/` consumer 列表为空） | `BLOCKED_ENVIRONMENT` |
| `smartvideo-worker` | `NO` | `NO` | `NO` | `BLOCKED_ENVIRONMENT` |
| PostgreSQL / RabbitMQ / Redis / MinIO 依赖 | `VERIFIED` | 对应端口已暴露 | 仅 RabbitMQ broker 健康，不代表业务 consumer | `VERIFIED` |

Docker 只读检查显示：`ai-postgres-1`、`ai-rabbitmq-1`、`ai-redis-1`、`ai-minio-1` healthy；`ai-xianzhi-ai-1` 为 exited；未发现运行中的 API、generation-worker 或 smartvideo-worker release 实例。

## 3. 外部与环境 blocker 分类

本节只使用四类最终状态：`VERIFIED`、`BLOCKED_EXTERNAL`、`BLOCKED_ENVIRONMENT`、`NOT_APPLICABLE`。

| 项目 | 最终状态 | 阻断原因 | 需要的凭据/环境 | 执行责任 | 完成标准 |
|---|---|---|---|---|---|
| Secret Rotation | `BLOCKED_EXTERNAL` | 生产凭据轮换、旧凭据失效和历史 secret 检查需要安全/IAM 权限 | Provider、微信、对象存储、数据库、Connector 的生产 Secret/IAM | 安全负责人 / IAM 管理员 | 轮换记录、旧 Key 失效验证、GitHub history/repository secret 检查，且不暴露 Secret 值 |
| Payment Real Integration（PAY-P1-001）与 fulfillment projection（PAY-P1-002） | `BLOCKED_EXTERNAL` | 当前 release 没有真实回调、查单、人工补发、重复通知和对账证据；匹配 migration 的 PostgreSQL fixture 也未就绪 | 微信虚拟支付 sandbox/白名单账号、回调/查单/人工补发权限；匹配 migration 的独立 PostgreSQL fixture | 支付运营、后端发布负责人、DBA | 同一 release 绑定 callback → query compensation → manual grant → idempotency → fulfillment/ledger reconciliation 证据 |
| Backup Restore | `BLOCKED_EXTERNAL` | 没有可安全使用的当前生产备份、offsite 上传校验和独立恢复授权 | 生产备份副本、OBS/R2/COS offsite 权限、独立 PostgreSQL 恢复库 | SRE / DBA / 对象存储管理员 | `OFFSITE_VERIFIED`、clean restore、关键表/账本/File Object/Outbox 校验、RPO/RTO 记录 |
| Release Identity | `BLOCKED_ENVIRONMENT` | checkout 落后 `origin/main` 5 commits，且冻结前仍有 44 个 dirty/untracked 条目 | 授权的提交、审阅、推送、immutable image build 环境 | Release owner | 单一已推送 commit、clean tree、immutable image digest 与运行实例一致 |
| Connector Runtime DLQ | `BLOCKED_ENVIRONMENT` | 真实 failure exhaustion → DLQ → 告警 → 人工恢复尚无运行记录 | 可控 Redis integration、Connector worker、告警与恢复权限 | 平台/SRE/Connector owner | 真实 Redis 三次失败、durable DLQ、告警、人工恢复和幂等证据 |
| PPTX Storage E2E | `BLOCKED_ENVIRONMENT` | 当前只有代码级和相关测试证据，没有同一 release 的真实导出/存储/下载链路 | API、匹配 PostgreSQL、MinIO/R2、File Object、signed download 环境 | 后端/平台 owner | 创建 PPT → 导出 → File Object → signed download HTTP 200 → 重复导出幂等 |
| Image Storage Canary | `BLOCKED_ENVIRONMENT` | 本地 mock/兼容路径不等价于真实 Provider 白名单与对象存储 Canary | 白名单 Provider、MinIO/R2、积分结算和故障注入环境 | Provider/平台 owner | 真实 Provider → File Object → Asset → signed HTTP 200；失败时无 Provider URL 回退并完成账务结算 |
| API / Worker Runtime | `BLOCKED_ENVIRONMENT` | 当前只看到依赖容器；业务 API、generation-worker、smartvideo-worker 未运行 | 当前 release 镜像、RabbitMQ consumer、lease/heartbeat/crash-reclaim 运行环境 | 发布平台 / SRE | API → Outbox → RabbitMQ → worker claim → Provider → Artifact → Storage → Asset 的同 task 证据链 |
| History Video Real Playback | `BLOCKED_ENVIRONMENT` | `task_000284` 的第三方 URL 真实返回 403，浏览器 `stalled`/`duration=0`；当前 H5 未挂载 `UserAssetDetailPage` | 包含最新 backend 与 `AssetDetailCenterPage` 的同环境 API + 小程序/App 构建 | 视频 owner / 发布平台 | 同一历史资产真实详情页不再下发第三方 URL，显示 EXPIRED 失效卡片并完成重新生成引导 | 见 `HISTORY-VIDEO-REAL-PLAYBACK-VERIFY.md`；抓 API、DOM/video src、媒体请求和 console |

## 4. RC 状态重算

仓库现有报告只给出历史 `RC_SCORE=55`，没有保存可复算的评分公式、权重或逐项分值。当前冻结检查没有新增可关闭 P0/P1 的真实证据，因此不能诚实地生成一个新的数值分数，也不机械复制 55。

```text
RC_SCORE=NOT_COMPUTABLE_WITH_CURRENT_REPOSITORY_RULES
P0_BLOCKERS=1
P1_BLOCKERS=9
P2_ISSUES=7
EXTERNAL_BLOCKERS=3
ENVIRONMENT_BLOCKERS=6
PRODUCTION_READY=NO
```

计数口径：P0 为 Secret Rotation；P1 为当前 burn-down 的 8 个未闭环项（Release、Payment 两项、Connector、PPTX、Image、Backup、Infrastructure）加上历史视频真实播放验证，共 9 项。外部/环境分类按本报告表格的唯一最终状态计数；Payment/Backup 的数据库或备份前置条件已在阻断原因中记录，但不重复计数。

## 5. Worktree 状态

```text
branch=main
HEAD=bc00ae1f7940a12cfdc571b342b091d704d39442
origin/main=e8846b0c5f49735df09f9c1639e3dbb8ca84d6be
origin_drift=behind_5_commits
git_status_before_freeze=44_entries
git_diff_check=FAIL_WITH_EXISTING_TRAILING_WHITESPACE_IN_docs/regression/protected-surfaces.md
production_deploy=NO
```

`protected-surfaces.md` 的 trailing whitespace 属于既有问题，本轮不处理，以免制造无关 diff 和审计噪音。

## 6. 未处理既有问题

- 未处理既有 trailing whitespace。
- 未清理、删除或重置任何 dirty/untracked 文件。
- 未执行生产 Secret 轮换、支付回调、offsite backup restore 或生产发布。
- 未把依赖容器健康误判为业务 API/Worker 健康。
- 未重新执行已经有明确 PASS 的视频资产、VIDEO_ASYNC 和图片资产全量测试。

## 7. 下一次恢复执行入口

仅当对应真实外部/运行环境到位后，按以下顺序恢复：

1. 先由授权 owner 整理冻结前工作区并形成单一 release commit，不在冻结目录直接部署。
2. 安全负责人提供 Secret rotation 与旧凭据失效证据。
3. 准备匹配 migrations 的独立支付数据库和真实支付 sandbox/白名单证据。
4. 准备 offsite backup 与独立 restore 环境，采集 RPO/RTO。
5. 启动当前 release API、generation-worker、smartvideo-worker，采集同 task 的完整运行证据。
6. 仅补跑对应 blocker 的最小验证，不重跑无关 PASS 套件。

## 最终状态

```text
RC_FREEZE_STATUS=PASS
RC_SCORE=NOT_COMPUTABLE_WITH_CURRENT_REPOSITORY_RULES
P0_BLOCKERS=1
P1_BLOCKERS=9
EXTERNAL_BLOCKERS=3
ENVIRONMENT_BLOCKERS=6
PRODUCTION_READY=NO
```

`RC_FREEZE_STATUS=PASS` 仅表示冻结、分类和证据归档完成，不表示生产发布通过。
