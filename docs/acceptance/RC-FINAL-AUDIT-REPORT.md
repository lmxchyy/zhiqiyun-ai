# 知启云 AI 项目级 RC Final Audit

- **审计时间**：2026-09-09
- **审计范围**：当前工作区 `main`（HEAD `bc00ae1f7`）及其未提交工作区变更
- **审计性质**：只读审计；本次未修改业务代码、配置或数据库
- **审计目标**：识别剩余 P0/P1 阻断，不把缺少真实证据的项目项伪造为 PASS
- **最终结论**：`PRODUCTION_READY=NO`

```text
RC_SCORE=55
P0_BLOCKERS=1
P1_BLOCKERS=9
P2_ISSUES=7
PRODUCTION_READY=NO
```

> 本报告不是发布批准。`PASS` 仅表示代码/测试/文档证据达到对应范围；`NOT VERIFIED` 表示缺少当前 release 的真实环境证据；`BLOCKED` 表示验证因基础设施或前置条件缺失无法完成。

---

## 1. 审计边界与重要校正

### 1.1 当前 checkout 不是最新 `origin/main`

只读命令结果：

```text
HEAD:       bc00ae1f7
origin/main: ahead by 5 commits
```

`origin/main` 的 5 个提交包含后续 PPTX durable artifact / recovery 变更，但它们**不在本次审计的当前 HEAD**。因此本报告不把远端后续实现当作当前代码能力。

### 1.2 工作区不是可发布状态

当前工作区存在多处已修改文件及未跟踪文件，包括视频资产实现、测试和验收文档。`deploy.sh` 目前使用 `git diff --quiet` 和 `git diff --cached --quiet` 检查 tracked diff，但没有用 `git status --porcelain` 阻断未跟踪文件。

结论：当前状态不能作为可复现 release candidate。

### 1.3 已关闭的视频项承接但不重复列为阻断

按本轮审计范围，以下项目承接既有验收结论，不重新列为 P0/P1：

- `VIDEO_ASYNC`（仅承接异步/Canary 证据）
- 视频资产持久化代码与专项 Canary 证据
- 历史视频播放兼容代码路径
- `VIDEO_PLATFORM_RC_STATUS=NOT_CLOSED`：真实用户历史视频播放验证失败，见 `docs/acceptance/HISTORY-VIDEO-REAL-PLAYBACK-VERIFY.md`

但必须保留证据边界：当前工作区的 `video_async_real_canary_test.go` 使用 `httptest.NewServer` 作为上游，并直接调用 `persistGeneratedVideos`；它证明的是 PostgreSQL/MinIO/签名 URL/幂等存储集成，不独立证明真实 Provider、RabbitMQ claim、Worker 进程和积分 freeze/capture/release 全链路。此前报告中的更强表述不能由该测试单独推出。本报告不把该问题重新升级为视频阻断，但将其列为证据限定。

---

## 2. P0 阻断项

### SEC-P0-001：历史敏感凭据未轮换，安全残余仍被接受

**状态：BLOCKED**

证据：

- `docs/acceptance/price-plan-v2-production-gate/evidence/20260729/P0-SECRETS-REDACTION.md` 明确记录：工作区已脱敏，但历史 Git 可能仍有明文，且**密钥未轮换**。
- 同目录 `OPERATIONAL-GO-SIGNATURE.md` 将其标为 `CLOSED-WITH-ACCEPTED-RESIDUAL`，不是“泄露窗口已关闭”。
- 当前源码扫描未发现新的 tracked 明文密钥，但没有当前 release 对历史凭据全部失效/轮换的外部证据。

**判定**：对生产 RC 而言，历史暴露凭据仍可能有效属于安全 P0。接受残余不等于证明生产安全；需要安全负责人提供轮换完成证据，或单独形成当前 release 的正式风险接受记录。

---

## 3. P1 阻断项

### REL-P1-001：release 身份不可复现，dirty tree 与远端漂移同时存在

**状态：BLOCKED**

证据：

- 当前 `main` 落后 `origin/main` 5 commits。
- 工作区包含未提交 tracked 修改和未跟踪业务/测试文件。
- `deploy.sh` 只检查 tracked diff，不阻断 untracked 文件；在构建型部署路径中，未跟踪文件可能进入 build context，造成“部署内容不等于 Git 提交”的风险。

**影响**：无法把当前目录中的测试结果、验收报告和业务代码绑定到一个已推送的 release commit。

### PAY-P1-001：支付回调、官方查单、人工补发的当前 release 真实证据不完整

**状态：NOT VERIFIED / TEST INFRA BLOCKED**

代码侧已有：

- `GrantOrderEntitlements` 统一入口。
- 回调、查单补偿和后台人工补发调用同一服务的代码路径。
- 相关 Go 单测/集成测试文件存在。

但当前审计执行 `go test ./...` 时，7 个 PostgreSQL pricing/payment 相关测试因 `127.0.0.1:55441` 不可连接而失败，不能把它们记为 PASS。仓库中的微信支付证据主要来自 2026-07-29 的旧 release 资料，未绑定当前 HEAD。

**缺口**：当前候选 release 没有一套绑定同一 commit 的真实支付回调、官方查单补偿、人工补发、重复回调和失败重试证据。

### PAY-P1-002：两套支付履约投影不一致

**状态：IMPLEMENTED LOCALLY / POSTGRES INTEGRATION BLOCKED**

证据：

- 已在 `backend-go/internal/httpserver/wechat_virtual_entitlements.go` 为 `GrantOrderEntitlements` 增加 `xz_fulfillment_records` 的 PROCESSING/SUCCESS/FAILED 投影。
- 已在 `backend-go/internal/httpserver/payment_center_api.go` 让管理员重试识别 `wechat_virtual_entitlement` 并分流回虚拟权益幂等入口。
- `docs/acceptance/fixes/P1-PAYMENT-FULFILLMENT.md` 已记录修改和测试边界。
- `go test` 编译验证通过；完整 PostgreSQL fixture 仍因 `127.0.0.1:55441` 不可连接而 BLOCKED。

**剩余影响**：代码修复已落地，但当前 release 尚无真实数据库回归证据，不能将该 P1 标记 CLOSED。

### CON-P1-001：Connector worker 对处理错误没有 durable retry/DLQ

**状态：IMPLEMENTED LOCALLY / REAL REDIS DLQ NOT VERIFIED**

证据：

- `backend-go/internal/httpserver/connector_queue.go` 已增加 Redis attempts hash、最多 3 次 retry、成功清理和 durable DLQ。
- 非法 payload 也进入 DLQ；内存降级路径执行有界重试并明确记录不能跨进程恢复。
- `docs/acceptance/fixes/P1-CONNECTOR-RETRY.md` 已记录修改和测试边界。
- 本地 Connector retry 测试通过；真实 Redis DLQ 验证仍需 `XIANZHI_CONNECTOR_TEST_REDIS_URL`。

**剩余影响**：没有真实 Redis 的 retry/DLQ 运行证据前，不能将该 P1 标记 CLOSED。

### PPT-P1-001：当前 HEAD 的普通 PPTX 导出不是持久化 File Object

**状态：IMPLEMENTED LOCALLY / REAL STORAGE E2E NOT VERIFIED**

证据：

- `exportPPT` 与 `downloadPPTExport` 现在在返回文件前调用 `persistPPTXArtifact`。
- `ppt.Service.SetPPTURL` 将 `storage://tenant/file` 持久化到任务 raw state；读取时动态签发访问 URL。
- 使用 `StoreObjectIdempotent` 和内容 hash，统一进入 `File Object -> Storage -> Signed Access`。
- `docs/acceptance/fixes/P1-PPTX-FILE-OBJECT.md` 已记录修改和测试边界。
- 编译、Service reference 和相关 HTTP 测试通过；真实 PostgreSQL + MinIO E2E 尚未执行。

**剩余影响**：真实对象存储 E2E 未验证前，不能将该 P1 标记 CLOSED。

### IMG-P1-001：图片存储不可用时仍允许生成链路保留 Provider URL

**状态：IMPLEMENTED LOCALLY / REAL IMAGE STORAGE E2E NOT VERIFIED**

证据：

- `persistGeneratedImages` 现在在没有 `fileService` 或 `StorageAvailable=false` 时 fail-closed。
- 只有 File Object 持久化完成后才继续生成资产写入；原有幂等复用和签名 URL 保留。
- `docs/acceptance/fixes/P1-IMAGE-STORAGE-FAIL-CLOSED.md` 已记录修改和测试边界。
- fail-closed、持久化和私有下载测试通过；当前 release 的真实图片 Provider + MinIO/R2 Canary 尚未执行。

**剩余影响**：真实图片对象存储 E2E 未验证前，不能将该 P1 标记 CLOSED。

### DR-P1-001：生产异地备份/恢复没有当前周期的真实闭环证据

**状态：NOT VERIFIED**

证据：

- 备份上传、批次隔离、retention 和 Python 3.6 兼容测试本次通过：`67 pass / 2 skipped`。
- 这些测试主要使用 fake provider/local harness，不能证明生产 OBS/R2/COS 当前身份、远端对象和恢复权限。
- 历史 acceptance 资料明确记录过 offsite 批次提前退出问题；当前脚本已有 stdin 隔离修复，但没有本次自然调度周期的 `OFFSITE_VERIFIED` 与独立库恢复记录绑定当前 release。

**影响**：发生 PostgreSQL 灾难时，不能仅凭本地备份脚本测试宣布 RPO/RTO 已验证。

### INFRA-P1-001：生产异步 runtime 与 worker 部署状态无法核对

**状态：NOT VERIFIED**

证据：

- `compose.prod.yml` 中 `ASYNC_MESSAGING_ENABLED` 默认值为 `false`。
- `compose.prod.yml` 未声明独立 `generation-worker` 服务；当前实现依赖 API 内部 async runtime，且只有启用开关后才启动 canary consumers。
- 当前 Docker 运行实例只有 PostgreSQL、RabbitMQ、Redis、MinIO 等依赖容器，没有可用于本次审计的 `xianzhi-ai` API 或 `generation-worker` 容器。
- `go test ./...` 的失败也显示独立 PostgreSQL test fixture `55441` 未就绪。

**影响**：不能把本地依赖容器存在等同于生产 API/Worker/Outbox/Consumer 已运行并受监控。

---

## 4. 各领域审计结论

| 领域 | 结论 | 证据状态 |
|---|---|---|
| VIDEO_ASYNC / 视频资产 | 承接既有关闭结论，不重复列阻断 | 当前测试可证明 DB/MinIO 存储子链路；真实 Provider/Worker/计费证据边界见 §1.3 |
| 支付回调与履约 | **P1** | 代码路径存在；当前 release 真实回调/查单/人工补发未验证，且双投影不一致 |
| 冻结/扣减/释放/退款账本 | **部分通过** | 代码与单测存在；当前完整 payment Postgres suite 被 `55441` 环境阻断 |
| Connector 重试/DLQ | **P1** | Redis 工作队列有恢复扫描，但 handler error 会移除任务，无 durable retry/DLQ |
| PPT 产物持久化 | **P1** | 当前 HEAD 普通导出按请求重建，不是最终 File Object |
| 智能混剪 | **代码/单测通过，真实环境未验证** | API/ExportService/Outbox/积分测试通过；未取得当前 release 真实渲染、对象存储、播放和结算证据 |
| 图片资产生命周期 | **P1 风险** | 存储不可用会回退 Provider URL；无当前 release 真实长周期证据 |
| PostgreSQL/RabbitMQ/Redis/MinIO | **本地依赖可用，生产未验证** | 本地端口/容器存在；API/Worker 生产 runtime 未在本次审计环境运行 |
| Outbox/Inbox 幂等 | **代码/测试通过** | Outbox claim/reclaim、Inbox duplicate/recovery、Rabbit redelivery 测试通过；无当前生产 backlog/alert 证据 |
| Worker lease/heartbeat/crash recovery | **部分通过** | smartvideo lease/heartbeat 代码和测试存在；Connector handler failure recovery 不完整；generation 当前仅 canary consumer 路径 |
| 备份与恢复 | **P1** | 本地/伪对象存储测试通过；生产远端与独立库恢复未绑定当前 release |
| Feature Flags | **默认值安全，但 rollout 未验证** | 生产 compose 多数 fail-closed；实际 production env 与当前 release 的旗标证明不足 |
| Protected Surfaces | **未完成总核对** | M3 已核对；其余 W1–W4、M1、M2、M4–M8、P1/P2 仍未在本次审计逐项签核 |
| Secrets / 配置 | **P0** | 工作区无新增明显明文；历史凭据未轮换的残余仍被接受 |
| RC 证据完整性 | **P1** | 旧文档日期/commit 不统一，当前 HEAD dirty 且落后远端，不能形成单一 release evidence bundle |

---

## 5. P2 问题与改进项

1. `docs/acceptance/price-plan-v2-production-gate/README.md`、`go-no-go-gate.md` 及证据目录同时保留 GO/NO-GO 历史状态，缺少当前 release 的单一真相源。
2. `MEDIA_STORAGE_PROVIDER` 在 compose 默认仍为 `local`，而生成资产链路同时使用 S3/MinIO 文件中心；需要在生产配置基线中明确两者边界。
3. 当前本地数据库存在历史 Connector `received/queued/processing` 记录和 PPT `pending` 记录；这是本地 fixture，不应当作生产指标，但应在真正生产审计中输出 backlog age、DLQ、stuck、unsettled 指标。
4. PPT PDF 导出接口仍返回 `501 Not Implemented`，若 PDF 是商业承诺，应另列产品门禁；若不是，应在 RC 范围明确排除。
5. 智能混剪真实验收证据缺少实际视频文件、对象 key、signed URL、HTTP range、Capture/Release 对账和 Worker crash/retry 记录。
6. 图片资产缺少与视频同级的真实对象存储 Canary、长期播放和故障释放积分证据。
7. Protected Surface 模板中除 M3 外多数条目仍为 `[ ]`，不能用视频专项 PASS 代替项目级回归签核。

---

## 6. 已执行验证

### 通过

```text
cd backend-go && go test -count=1 ./internal/messaging ./internal/app/payment ./internal/app/ppt ./internal/app/smartvideo
PASS

cd backend-go && go test -count=1 ./internal/httpserver -run 'Test(PPTCanary|PPTWorker|Connector|VideoCanary|GenerationRecovery|.*Payment.*|.*Entitlement.*)'
PASS

node --test tests/ai-auto-montage-api.test.mjs tests/ai-auto-montage-web.test.mjs tests/ai-auto-montage-mini.test.mjs tests/user-mini-smart-video-montage.test.mjs
12/12 PASS

node --test tests/backup-offsite-batch.test.mjs tests/backup-offsite-upload.test.mjs tests/backup-retention.test.mjs tests/backup-retention-automation.test.mjs tests/backup-script-hardening.test.mjs tests/backup-uploader-container.test.mjs tests/production-contract.test.mjs
67 PASS, 2 SKIPPED
```

### 阻断 / 未通过

```text
cd backend-go && go test ./...
FAIL
```

失败不是已确认业务代码失败，主要是 PostgreSQL test fixture `127.0.0.1:55441` 不可连接，导致 pricing/payment 相关 7 个测试无法执行；因此这些测试必须标记 `BLOCKED`，不能写成 PASS。

### 本地运行态观察

```text
docker ps
```

可见 PostgreSQL、RabbitMQ、Redis、MinIO 等依赖容器；未见本次审计可核对的 `xianzhi-ai` API 或 `generation-worker` 容器。依赖就绪不能替代业务 runtime 就绪证明。

---

## 7. Burn-down Iteration 1

本轮已自动完成以下本地修复，并生成对应 fix record：

- `CON-P1-001`：Connector retry / DLQ 代码和本地测试。
- `PAY-P1-002`：虚拟支付 fulfillment record 投影和管理员重试分流。
- `PPT-P1-001`：普通 PPTX File Object 持久化、幂等和 signed access reference。
- `IMG-P1-001`：图片存储不可用时 fail-closed。
- `REL-P1-001`：`deploy.sh` 现在阻断 tracked、staged 和 untracked 变化。

保护面回归：视频资产、智能混剪及相关 M3/M6 Node 套件 `20/20 PASS`。

本轮新增验证：使用本地 PostgreSQL `54321` 尝试运行虚拟支付生命周期时，因缺少 `xz_order_settlement_engine_decisions` 迁移表失败；这进一步证明当前数据库不是可用的 release test fixture，不能把支付集成测试写成 PASS。

这些修复尚未全部关闭对应 P1，因为真实 Redis、PostgreSQL、对象存储、生产部署和支付环境证据仍缺失。依据“不伪造 PASS”规则，本轮不降低 P0/P1 计数。

## 8. Evidence Collection Update

本轮已切换为证据收集模式，未修改业务代码：

- `docs/acceptance/RC-EVIDENCE-MATRIX.md`
- `docs/acceptance/RC-EVIDENCE-CLOSURE-REPORT.md`

依赖健康检查：PostgreSQL accepting connections、RabbitMQ ping succeeded、Redis PONG、MinIO HTTP 200。

本轮曾在隔离端口启动 API `:3310` 和 generation-worker，完成一次本地 mock/兼容图片任务 admission、PostgreSQL `SUCCEEDED/CAPTURED`、File Object、Asset 和 HTTP 200 下载验证；但该任务由 API 内部 runtime 完成，未形成 generation-worker claim 证据，且数据库 storage config 指向 R2 而非 MinIO。测试资产已清理，File Object 进入 `DELETED` recycle 状态。

当前没有可核对的 smartvideo-worker 运行证据。Secret 状态为 `ROTATION_REQUIRED`，支付独立 PostgreSQL fixture 仍缺失。

因此 RC 数值不变，不生成 Production Ready 报告。

## 9. 最终 RC 决策

```text
RC_SCORE=55
P0_BLOCKERS=1
P1_BLOCKERS=9
P2_ISSUES=7
PRODUCTION_READY=NO
```

补充校正：历史视频真实用户播放验证失败，视频资产 RC 项不能按已关闭处理；见 `docs/acceptance/HISTORY-VIDEO-REAL-PLAYBACK-VERIFY.md`。

**下一步不是继续堆视频能力**。应先完成：

1. 安全凭据轮换/正式风险接受；
2. 冻结单一 release commit，清理 dirty/untracked 并与 `origin/main` 对齐；
3. 修复或明确接受支付双履约投影差异；
4. 为 Connector 增加 durable retry/DLQ 与 delivery retry 运维路径；
5. 把当前 HEAD 的 PPTX 最终产物纳入 File Object/Storage/Signed Access；
6. 完成当前 release 绑定的支付、图片、PPT、智能混剪和备份恢复真实证据；
7. 完成 Protected Surfaces 总核对后，再重新计算 RC 分数。

在以上 P0/P1 和真实证据缺口关闭前，不应进入全量生产灰度或商业化放量。
