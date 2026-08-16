# PPT Generation V2 Phase 3 Slice A.1 实施报告

- 日期：2026-08-16
- 工作区：`E:\code\work\ppt-v2`
- 分支：`codex/ppt-v2`
- Pull Request：[#6](https://github.com/lmxchyy/zhiqiyun-ai/pull/6)
- Slice A 基线：`421609aa7 docs(ppt-v2): record phase 3 slice a readiness`
- Slice A.1 主实现：`fce6b097d feat(ppt-v2): add durable semantic planning loop`
- CI 最小修复：`a0f9db70c fix(ppt-v2): research English industry requests`
- 功能验证运行：[GitHub Actions user-core #74](https://github.com/lmxchyy/zhiqiyun-ai/actions/runs/31924312956)
- Slice A.1 状态：`READY`

本文记录 Phase 3 Slice A.1「Agent Experience & Planning Quality」的最终实现状态。Phase 1、Phase 2 与 Slice A 报告继续作为历史阶段快照保留，不删除、不覆盖。

Slice A.1 没有进入 Slice B、C 或 D。最终边界仍停在：

`Approved OutlinePlan`

本阶段没有生成最终 SlideIR、图片、Preview、LayoutResult 或 PPTX，也没有新增企业功能。

## 1. Architecture

Slice A.1 将 Slice A 的同步、固定规则规划路径替换为 PostgreSQL 权威的 Durable 后台规划链路：

```text
POST /api/v1/ppt/agent/guide
  -> 解析 Intent
  -> 创建 GenerationJob
  -> 持久化 IntentSpec / InputSnapshot
  -> 立即返回 job identity

Planning Worker
  -> ready-job scan
  -> Phase 2 lease / fencing / heartbeat
  -> Research Provider
  -> Storyline PlanningPort
  -> Outline PlanningPort
  -> contract / provenance / evidence validation
  -> durable checkpoint
  -> WAITING_FOR_OUTLINE_APPROVAL

Outline Review UI
  -> 轮询权威 Job State
  -> 查看 Research provenance
  -> 编辑 OutlinePlan
  -> 明确批准
  -> Approved OutlinePlan
```

PostgreSQL 是调度、阶段、失败、重试和规划产物的权威状态。前端不调度 worker，也不保存第二套 ResearchPack、Storyline 或 OutlinePlan 权威副本。

## 2. Changed Files

### Backend domain 与持久化

- `backend-go/internal/app/ppt/agent_planning.go`
- `backend-go/internal/app/ppt/agent_planning_ports.go`
- `backend-go/internal/app/ppt/agent_planning_worker.go`
- `backend-go/internal/app/ppt/agent_planning_service.go`
- `backend-go/internal/app/ppt/agent_planning_postgres.go`
- `backend-go/internal/app/ppt/generation_job.go`
- `backend-go/internal/app/ppt/generation_job_postgres.go`

### Production Planning Adapter

- `backend-go/internal/provider/pptplanning/client.go`
- `backend-go/internal/provider/pptplanning/client_test.go`

### HTTP application boundary

- `backend-go/internal/httpserver/api.go`
- `backend-go/internal/httpserver/server.go`
- `backend-go/internal/httpserver/ppt_agent_slice_a.go`
- `backend-go/internal/httpserver/ppt_agent_slice_a_test.go`

### Outline Review UI

- `admin-vue/src/api/ppt.ts`
- `admin-vue/src/types/pptAgent.ts`
- `admin-vue/src/stores/pptAgent.ts`
- `admin-vue/src/components/ppt/PptAgentPlanningWorkspace.vue`
- `admin-vue/src/components/ppt/PptAgentOutlineReview.vue`
- `admin-vue/src/components/ppt/PptDocumentGeneratePage.vue`
- `admin-vue/tests/pptAgentPlanning.spec.ts`

### Tests 与 CI

- `backend-go/internal/app/ppt/agent_planning_a1_test.go`
- `backend-go/internal/app/ppt/agent_planning_test.go`
- `backend-go/internal/app/ppt/agent_planning_service_test.go`
- `backend-go/internal/app/ppt/generation_job_postgres_integration_test.go`
- `.github/workflows/user-core.yml`

CI 暴露的英文 `industry analysis` Research 意图识别缺口由 `a0f9db70c` 最小修复，并增加不依赖数据库的意图回归测试。

## 3. Worker Lifecycle

`POST /guide` 只完成 Intent 解析、Job 创建、IntentSpec/InputSnapshot 持久化和 job identity 返回。它不再在 HTTP 请求内执行 Research、Storyline 或 Outline Planning。

后台 `AgentPlanningService.Start` 启动独立循环：

1. 启动时立即扫描一次 ready jobs；
2. 定时扫描 PostgreSQL；
3. 新 Job 创建后通过非阻塞 wake channel 提醒 worker；
4. 按配置的 worker limit 并发处理；
5. 每个候选 Job 必须先通过 Phase 2 `Claim` 获得 lease 与 fencing token；
6. provider 调用期间按 lease duration 的三分之一发送 heartbeat；
7. lease 丢失时取消 provider context，过期 worker 不能提交输出；
8. 每个阶段成功后立即写入 durable checkpoint；
9. 到 `OUTLINE_PLANNED` 后进入 `WAITING_FOR_OUTLINE_APPROVAL` 并停止自动推进。

页面关闭、刷新或网络断开只会停止该页面的轮询，不会停止后台 worker。

## 4. Ready-job Scan / Claim 策略

Worker 通过 `ListReadyAgentPlanning` 只扫描 `AGENT_OUTLINE` workflow 中可执行的 Job：

- Stage 为 `CREATED`、`INTENT_RESOLVED`、`RESEARCHED` 或 `STORYLINE_PLANNED`；
- Status 为到期的 `QUEUED` / `RETRY_WAIT`；
- 或 lease 已过期的 `RUNNING`。

扫描结果仍需逐项执行 `Claim`，因此 ready scan 不是执行权。真正执行权由租户/用户 scope、lease owner、lease expiry 和 fencing token 共同决定。

同一批次的 worker identity 带稳定 worker 前缀和批内序号。并发 worker 即使观察到同一个 Job，也只有获得有效 lease 的实例能够持久化 checkpoint。

## 5. Durable Stages

Agent Planning workflow 的权威阶段为：

```text
CREATED
  -> INTENT_RESOLVED
  -> RESEARCHED
  -> STORYLINE_PLANNED
  -> OUTLINE_PLANNED
  -> WAITING_FOR_OUTLINE_APPROVAL
```

用户批准后：

```text
OUTLINE_PLANNED / WAITING_FOR_OUTLINE_APPROVAL
  -> OUTLINE_APPROVED / QUEUED
```

每个成功阶段只写入一次 transition。UI 显示的是服务器返回的 Stage/Status，而不是 elapsed-time 或前端计时推测出的进度。

## 6. PlanningPort

规划被拆为两个明确的应用端口。

### Storyline PlanningPort

输入：

```text
IntentSpec + ResearchPack
```

输出：

- `StorylineDraft`
- `PlanningProvenance`

StorylineDraft 包含 language、thesis、audienceTakeaway、narrativeArc、ordered sections 和 closingAction。它回答整套演示要证明什么以及按什么顺序证明，不等于页面列表。

### Outline PlanningPort

输入：

```text
IntentSpec + ResearchPack + Storyline
```

输出：

- `OutlinePlanDraft`
- `PlanningProvenance`

每个 SlideObjectiveDraft 包含 title、purpose、keyMessage、evidenceRequired、带 rationale 的 evidence、visualIntent 和 expectedElementTypes。

Materializer 负责生成稳定 Outline/Slide ID、revision 和 createdAt，并在写库前执行领域校验。Provider 不能决定租户、Job、Revision、时间戳或持久化 identity。

## 7. Production Provider Behavior

Production Adapter 复用仓库现有 OpenAI-compatible Chat Provider：

- `chatprovider.NewOpenAICompatible(cfg)`
- 模型来自现有 `cfg.PPTTextModel`
- 没有新增模型 SDK
- 请求使用 JSON object response format
- Storyline 与 Outline 分别调用独立端口
- provider response 使用严格 JSON decoder
- 未知字段、尾随 JSON、空响应和结构不合法都会被拒绝

Provider Prompt 接收完整 IntentSpec、ResearchPack，以及 Outline 阶段的 Storyline。它被明确要求：

- 使用 Intent language；
- 尊重 6～12 页和用户明确页数；
- 基于真实 Claim 进行语义规划；
- 不按数组位置分配 Claim；
- 为每个 EvidenceAssignment 输出 `claimId + rationale`；
- 不虚构 Claim ID。

`PlanningProvenance` 保存 mode、provider、model 和可选 providerRequestId。生产规划必须标记为 `AI`。`DETERMINISTIC_TEST` 只用于 unit/integration/golden fixture，不能在 production path 静默回退。

## 8. Fail-closed 与 Durable Error Taxonomy

Production Planning Provider 未配置、不可用、超时、返回非法结构、契约校验失败或 Evidence Mapping 不完整时，不生成伪装成 AI Planning 的 OutlinePlan。

结构化错误码为：

- `planning_provider_unavailable`
- `planning_timeout`
- `planning_invalid_output`
- `planning_contract_validation_failed`
- `planning_evidence_mapping_invalid`
- `research_provider_unavailable`
- `research_timeout`
- `research_invalid_result`
- `research_contract_validation_failed`

Durable Job Error 保存 code、面向用户的安全 message、retryable、provider、providerRequestId 和 occurredAt。前端按结构化 code 映射产品文案，不依赖模型或 SDK 的原始错误字符串。

## 9. Research Provenance 与透明度

ResearchPack 继续作为后端权威对象，保存 Source、Citation、Claim、Dataset 和 verification status。A.1 没有在前端建立第二套 provenance model。

完整证据链为：

```text
SlideObjective
  -> EvidenceAssignment(claimId, rationale)
  -> Claim
  -> citationRefs
  -> Citation
  -> Source
```

校验会拒绝：

- 不存在的 Claim；
- 空 rationale；
- evidenceRefs 与 evidence 不一致；
- Claim 缺少 Citation；
- Citation 与 Claim 指向不同 Source；
- 需要证据的事实型页面没有有效 EvidenceAssignment。

Outline Review 对每条事实型 evidence 展示：

- Source title；
- source type；
- Claim 文本；
- citation locator；
- verification status；
- evidence rationale；
- 该 evidence 支持的页码。

因此用户看到的是可解释的事实依据，而不是“有 2 条证据”之类不可审计的计数。

## 10. Language 与语义规划

Intent language 统一规范为 `zh-CN` 或 `en-US`。Storyline 和 Outline contract 强制与 Intent language 一致。

测试覆盖：

- 中文 Intent 获得中文 Storyline 与 Outline；
- 英文 Intent 获得英文 Storyline 与 Outline；
- 中英文 Provider Prompt 都携带权威 language；
- Storyline 来自 Provider output，不来自固定行业模板；
- Outline Evidence 来自 Claim 的语义选择和 rationale，不来自数组轮询。

CI #73 暴露英文 `industry analysis` 未触发 Research 的实现缺口。修复后英文 `industry`、`analysis` 和 `investment` 与已有 `research`、`market`、`trend`、`data` 一样会触发真实 Research；CI #74 的 PostgreSQL worker gate 随后通过。

## 11. Retry / Resume / Recovery

用户可调用：

`POST /api/v1/ppt/agent/jobs/:jobId/retry`

Retry 只清理当前 durable error 并把 Job 放回合法队列，不删除已完成 checkpoint：

- `RESEARCHED` 后 Storyline 失败：Retry 不重复 Research；
- `STORYLINE_PLANNED` 后 Outline 失败：Retry 不重复 Research，也不重复 Storyline；
- Outline 成功后停止在 approval gate，不自动进入 Slice B；
- 重复 Retry 保持幂等，不重复推进 Stage。

服务重启后，新 worker 从 PostgreSQL 扫描 ready jobs，并从最后一个 durable Stage 恢复。InputSnapshot、IntentSpec、ResearchPack、Storyline 与 OutlinePlan 都由服务器恢复。

Stale worker 的 heartbeat 或 checkpoint 会因 fencing token 不匹配而返回 lease-lost 语义，旧输出不能覆盖新 worker 的结果。

## 12. API Boundary

继续使用单一 `/api/v1/ppt` family，没有新增 `/api/v2/ppt` 或第二套 PPT Session backend。

Slice A/A.1 相关操作为：

- `POST /api/v1/ppt/agent/guide`
- `GET /api/v1/ppt/agent/jobs/:jobId`
- `PATCH /api/v1/ppt/agent/jobs/:jobId/outline`
- `POST /api/v1/ppt/agent/jobs/:jobId/outline/approve`
- `POST /api/v1/ppt/agent/jobs/:jobId/retry`

所有读取、修改、批准和重试都使用认证用户的 Tenant/Owner scope。前端不能通过请求体指定其他 tenant 并越权访问。

## 13. Outline Review UI

统一 PPT Agent 工作区已经接入现有 `PptDocumentGeneratePage`，没有创建第二套 Agent UI。

用户能够看到真实阶段：

- 正在理解需求；
- 正在研究资料；
- 正在规划叙事；
- 正在生成大纲；
- 大纲已生成，请确认。

Outline Review 展示每页：

- title；
- purpose；
- keyMessage；
- visualIntent；
- evidence 与来源。

P0 编辑能力：

- 修改 title；
- 修改 purpose；
- 修改 keyMessage；
- add slide；
- delete slide；
- move slide。

编辑请求仍通过受控 Outline EditCommand 和 expected revision 提交。evidenceRefs 默认由 Planning Provider 决定，本阶段只读展示，没有引入复杂 Citation Editor。

Pinia Store 每 1.5 秒读取一次权威 Job State。本地只保存 `jobId + prompt` 用于刷新恢复，不保存权威规划对象。批准后的产品文案为“方案已确认，大纲已经安全保存”，并明确说明多页生成尚未启用，不再展示实现 revision 或“Slice A 到此完成”等开发语言。

配图、模板、演讲稿和最终页面生成在当前流程中明确标记为尚未启用，没有制造已经可用的承诺。

## 14. Migration

Slice A.1 没有新增 migration。

原因是 A.1 所需持久化字段、planning JSON snapshot、revision、error、lease、fencing、transition 和 retry 元数据已经由以下既有 migration 提供：

- Migration 109：Phase 2 Durable GenerationJob；
- Migration 110：Slice A Agent Outline Approval persistence。

本阶段复用既有列和表即可完整保存新的 durable stages、PlanningProvenance、ResearchPack、Storyline 和 OutlinePlan。没有 migration 编号碰撞，也没有不可逆数据变更。

## 15. Tests

本地可运行验证结果：

- `go test ./internal/app/ppt -count=1`：PASS；
- `go test ./internal/provider/pptplanning -count=1`：PASS；
- `go test ./internal/httpserver -run '^TestPPTAgentSliceA' -count=1`：PASS；
- Admin PPT Agent planning tests：PASS；
- Admin typecheck/build：PASS；
- API Client boundary verification：PASS；
- `git diff --check`：PASS。

测试覆盖：

- `/guide` 立即返回 `CREATED` Job；
- 中英文 Intent 与 Planning language；
- production provider strict JSON 与 fail-closed；
- structured error classification；
- semantic evidence mapping 与 rationale；
- invalid Claim/evidence rejection；
- Research/Storyline checkpoint resume；
- Retry 幂等；
- stale fencing rejection；
- Outline Review evidence 展示和 P0 编辑命令；
- HTTP state/read/update/approve/retry；
- Tenant/Owner isolation。

## 16. PostgreSQL Validation

GitHub Actions `user-core #74` 在真实 PostgreSQL 16 service 上执行全部 `TestPostgresGenerationJob*`，五项均 PASS、无 SKIP：

- `TestPostgresGenerationJobLeaseFencingRestartCancelAndIsolation`
- `TestPostgresGenerationJobArtifactConstraintsAndTransactionRollback`
- `TestPostgresGenerationJobRetryAndAtomicTaskRelation`
- `TestPostgresGenerationJobAgentOutlineApprovalRecoveryAndIsolation`
- `TestPostgresGenerationJobAgentPlanningWorkerRecoveryRetryAndFencing`

新的 A.1 gate 真实验证：

- `CREATED` Job 持久化；
- Research checkpoint 后 Storyline Provider 失败；
- durable structured error；
- restart 后不重复 Research；
- Storyline checkpoint 后 Outline Provider 失败；
- 再次 restart 后不重复 Research/Storyline；
- Retry 从失败 Stage 恢复；
- 最终进入 `WAITING_FOR_OUTLINE_APPROVAL`；
- 每个 durable transition 只写一次；
- stale worker 输出被 fencing 拒绝。

## 17. GitHub Actions 与回归证据

功能与 CI 修复提交 `a0f9db70c` 对应的 [user-core #74](https://github.com/lmxchyy/zhiqiyun-ai/actions/runs/31924312956) 结果：

- `user-core / backend-go`：GREEN；
- `user-core / user-core`：GREEN；
- Phase 2 三个 PostgreSQL gate：PASS、无 Skip；
- Slice A PostgreSQL approval gate：PASS、无 Skip；
- Slice A.1 PostgreSQL worker gate：PASS、无 Skip；
- 完整 backend Go tests：PASS；
- Golden 1 semantic/geometry parity：PASS；
- Golden 1 frozen renderer repeatability：PASS；
- OfficeCLI `1.0.144` checksum/version 安装：PASS；
- `OfficeCLI accepts the generated package without a repair warning`：PASS、未 Skip；
- Admin unit tests：PASS；
- user-core typecheck、API boundary、H5/Admin/小程序/App/Harmony builds：PASS。

因此 Phase 1 Renderer/Golden、Phase 2 Durable Generation、Slice A Approval 与 Slice A.1 Planning Worker 的保护面同时保持绿色。

## 18. Known Limitations

- Research MVP 当前使用 Wikipedia Adapter，不是多供应商 autonomous research agent。
- Intent 解析仍是轻量规则解析，不是另一次模型调用；它只负责足够信息提取和必要澄清。
- UI 当前使用 1.5 秒 polling，没有 SSE。
- 前端保存 job identity 以恢复页面，但跨浏览器/设备仍需通过产品任务入口发现 Job。
- Planning Provider 的实际内容质量仍受现有模型和 Research source 质量影响；contract validation 只保证结构、语言、页数和 provenance/evidence 闭合。
- Evidence 在 A.1 停止于 Outline Review；映射到最终 SlideIR/CitationElement 属于 Slice B。
- 用户当前不能编辑 evidenceRefs，也没有复杂 Citation Editor。
- 当前没有 final SlideIR、图片生成、Chart/Table/Diagram、PreviewRenderer 或 PPTX 扩展。
- 批准后只保存 Approved OutlinePlan，不会自动生成演示文稿。
- 本阶段没有进入 Slice B、C、D，也没有新增企业治理能力。

## 19. Slice A.1 Exit Gate

| Exit Gate | 最终证据 | 结果 |
| --- | --- | --- |
| Research 对用户可解释 | Outline Review 展示 Source、Claim、Citation、状态、rationale 与使用页 | PASS |
| Claim / Source provenance 闭合 | SlideObjective → Claim → Citation → Source contract validation | PASS |
| Storyline 来自 production Planning Provider | OpenAI-compatible StorylinePlanningPort + AI provenance | PASS |
| Outline 来自语义 Planning Provider | OutlinePlanningPort 接收 Intent/Research/Storyline | PASS |
| 无 production silent deterministic fallback | provider failure 写 durable error；deterministic 仅 test mode | PASS |
| evidenceRefs 有语义 rationale | EvidenceAssignment `claimId + rationale` 及完整校验 | PASS |
| 中英文 Planning 正确 | provider/unit/intent tests；英文 Research CI 修复 | PASS |
| durable stages 可观察 | PostgreSQL Stage/Transition + UI polling | PASS |
| 页面关闭不停止 Worker | 后端独立 Start/scan loop；UI 只停止 polling | PASS |
| restart recovery | 真实 PostgreSQL A.1 gate | PASS |
| Research checkpoint 后不重复 Research | PostgreSQL provider call count assertion | PASS |
| Storyline checkpoint 后不重复 Storyline | PostgreSQL planning call count assertion | PASS |
| Retry 从失败 stage 恢复 | structured error + retry endpoint + PostgreSQL gate | PASS |
| stale worker 被 fencing 拒绝 | PostgreSQL stale lease assertion | PASS |
| Outline Review UX 完成 | title/purpose/keyMessage/visualIntent/evidence + P0 edits | PASS |
| Phase 1 Golden 1 | CI #74 semantic/geometry 与 repeatability PASS | PASS |
| OfficeCLI integrity | CI #74 OfficeCLI 1.0.144 真实执行 PASS | PASS |
| Phase 2 PostgreSQL gates | 三项 PASS、无 Skip | PASS |
| Slice A PostgreSQL gate | approval/recovery/isolation PASS、无 Skip | PASS |
| Slice A.1 PostgreSQL gate | worker recovery/retry/fencing PASS、无 Skip | PASS |
| backend-go | CI #74 GREEN | PASS |
| user-core | CI #74 GREEN | PASS |

Slice A.1 的全部 Exit Gate 已满足。

本结论不授权合并 PR，不进入 Slice B，也不扩展 PPTX generation、Preview 或多页 SlideIR。

## 最终结论

`SLICE A.1 STATUS: READY`
