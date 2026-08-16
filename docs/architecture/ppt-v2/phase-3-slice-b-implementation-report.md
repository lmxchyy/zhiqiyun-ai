# PPT Generation V2 Phase 3 Slice B 实施报告

- 日期：2026-08-16
- 工作区：E:\code\work\ppt-v2
- 分支：codex/ppt-v2
- Pull Request：[#6](https://github.com/lmxchyy/zhiqiyun-ai/pull/6)
- Slice A.1 基线：a0f9db70c fix(ppt-v2): research English industry requests
- Slice B 实现：97f0d58d1 feat(ppt-v2): generate durable multi-page decks
- 功能验证运行：[GitHub Actions user-core #76](https://github.com/lmxchyy/zhiqiyun-ai/actions/runs/31933646139)
- Slice B Technical Gates：READY
- Documentation Closeout：READY（本文）
- Slice B Status：READY

本文记录 Phase 3 Slice B「Multi-page Deck Generation」的最终实现状态。Phase 1、Phase 2、Slice A 与 Slice A.1 报告继续作为历史阶段快照保留，不删除、不覆盖。

Slice B 从不可变的 Approved OutlinePlan 开始，最终交付完整的 6～12 页 Professional PPTX：

    Approved OutlinePlan
      -> Multi-page SlideIR
      -> Private Image Assets
      -> LayoutResult
      -> Quality Gate
      -> PptxGenJS Renderer
      -> Private PPTX Artifact
      -> Signed Download URL

本阶段没有进入 Slice C 或 Slice D：没有 PreviewRenderer、会话式局部修改、EditCommand、Revision UI 或 Undo。

## 1. Architecture

Slice B 复用同一 /api/v1/ppt Application Boundary、同一 GenerationJob、Phase 2 lease/fencing/retry、Slice A/A.1 的规划状态以及现有 SlideIR/LayoutResult/PptxGenJS 内核，没有创建第三套 PPT API、Document Model 或存储系统。

用户批准大纲后，Approval 事务保存不可变的 Approved Outline revision，并为该 revision 初始化 DeckJob 与一页对应一个的 SlideJob。后台 Worker 随后从 PostgreSQL claim 可执行 Job，按 checkpoint 推进：

    OUTLINE_APPROVED
      -> per-slide SlideContent PlanningPort
      -> CONTENT_READY
      -> per-intent Image AssetPort
      -> ASSETS_READY
      -> Professional Deck + LayoutResult compilation
      -> LAYOUT_COMPILED
      -> Quality Gate
      -> QUALITY_CHECKED
      -> PptxGenJS render
      -> RENDERED
      -> private file storage
      -> FILE_STORED
      -> Work Center Asset
      -> ASSET_CREATED
      -> atomic Task relation
      -> TASK_RELATED
      -> COMPLETED

未批准的大纲没有 ApprovedOutline，Worker 会拒绝进入 Deck generation。上游 Intent、Research、Storyline 或 Outline 数据不完整时 fail closed，不在 Slice B 重新 Research 或重新规划。

## 2. Changed Files

主实现提交 97f0d58d1 共修改 45 个文件，新增 3,546 行、删除 66 行。

### Domain、Worker 与 PostgreSQL

- backend-go/internal/app/ppt/agent_deck.go
- backend-go/internal/app/ppt/agent_deck_store.go
- backend-go/internal/app/ppt/agent_deck_postgres.go
- backend-go/internal/app/ppt/agent_planning_worker.go
- backend-go/internal/app/ppt/agent_planning_service.go
- backend-go/internal/app/ppt/agent_planning_postgres.go
- backend-go/internal/app/ppt/generation_job.go
- backend-go/internal/app/ppt/generation_job_postgres.go

### Production adapters 与 HTTP boundary

- backend-go/internal/provider/pptplanning/client.go
- backend-go/internal/httpserver/ppt_v2_agent_deck_compiler.go
- backend-go/internal/httpserver/ppt_v2_agent_image_asset.go
- backend-go/internal/httpserver/ppt_v2_agent_artifacts.go
- backend-go/internal/httpserver/ppt_v2_durable_artifact_store.go
- backend-go/internal/httpserver/ppt_agent_slice_a.go
- backend-go/internal/httpserver/api.go
- backend-go/internal/httpserver/server.go

### Contract、Layout 与 Renderer

- contracts/ppt-v2/common.schema.json
- contracts/ppt-v2/deck.schema.json
- contracts/ppt-v2/slide-ir.schema.json
- packages/ppt-v2/src/contract.mjs
- packages/ppt-v2/src/design-system.mjs
- packages/ppt-v2/src/layout-compiler.mjs
- packages/ppt-v2/src/professional-deck.mjs
- packages/ppt-v2/src/professional-cli.mjs
- packages/ppt-v2/src/pptxgenjs-renderer.mjs

### UI、Migration、Tests 与 CI

- admin-vue/src/api/ppt.ts
- admin-vue/src/types/pptAgent.ts
- admin-vue/src/stores/pptAgent.ts
- admin-vue/src/components/ppt/PptAgentPlanningWorkspace.vue
- database/migrations/111-ppt-v2-agent-deck-generation.sql
- backend-go/internal/app/ppt/agent_deck_test.go
- backend-go/internal/app/ppt/agent_deck_worker_test.go
- backend-go/internal/app/ppt/generation_job_postgres_integration_test.go
- backend-go/internal/httpserver/ppt_agent_slice_b_test.go
- backend-go/internal/httpserver/ppt_v2_agent_image_asset_test.go
- backend-go/internal/provider/pptplanning/client_test.go
- tests/ppt-v2-phase3-slice-b.test.mjs
- admin-vue/tests/pptAgentPlanning.spec.ts
- .github/workflows/user-core.yml

## 3. Multi-page SlideIR

Professional Deck contract 版本为 2.1，明确接受 6～12 页。构建器同时校验 approvedOutline.pageCount 与实际 SlideObjective 数量相等，并要求每个 SlideObjective 恰好有一个 SlideContent 结果。

页面身份直接继承 Approved Outline 的稳定 slideId，顺序由 Approved Outline 决定；每个页面元素通过 slideId + semantic slot 生成稳定 elementId。6、8、10、12 页 contract tests 均验证：

- Outline 页数与 SlideIR 页数完全一致；
- Slide ID 与 Approved Outline 完全一致；
- 同样输入得到相同 SlideIR、LayoutResult 与 RenderInput；
- SlideIR 不包含 x/y/width/height；
- geometry 只存在于 LayoutResult。

SlideIR 保存 semantic content、key message、evidence/citation refs、speaker notes 与稳定 assetRef。Layout Compiler 继续是 geometry 的唯一权威，Renderer 不重新排版。

## 4. SlideContent PlanningPort

新增生产端口：

    IntentSpec
    + ResearchPack
    + Storyline
    + Approved OutlinePlan
    + current SlideObjective
    -> SlideContent PlanningPort
    -> SlideContentDraft + PlanningProvenance

输出包含 title、可选 subtitle、body blocks、bullets、supporting text、speaker notes、asset intents、citation refs 与 professional layout hint。

Production Adapter 复用现有 OpenAI-compatible Chat Provider 和既有 PPT text model，不新增模型 SDK。每个 SlideObjective 独立生成内容；Prompt 携带完整 Approved Outline、当前页面目标与真实 ResearchPack，并要求输出与 Intent language 一致。

生产路径 fail closed，没有 deterministic silent fallback。结构化错误包括：

- content_provider_unavailable
- content_timeout
- content_invalid_output
- content_contract_validation_failed
- content_evidence_mapping_invalid

Provider 输出必须通过严格 JSON 与领域校验；未知结构、非法 layout、语言缺失、事实页 citation 缺失、citation 与 Approved evidence 不一致、图片目标与 layout/asset intent 不一致都会被拒绝。

## 5. Citation / Evidence Provenance

Slice A/A.1 建立的 provenance 已进入最终页面层：

    SlideIR / image element
      -> claimId in citationRefs
      -> Claim.citationRefs
      -> Citation.sourceId
      -> Source

Deck contract 保留完整 sources、citations 与 claims。事实型 SlideObjective 必须携带 Claim 引用；SlideContent 的 citationRefs 必须与 Approved Outline evidenceRefs 等价，且每个 Claim 必须能闭合到同一 Source 的 Citation。

PPTX speaker notes 会附加简洁 Sources 列表，包含 Claim ID、Source title 和 Citation locator。图片元素也保留该页 citation refs。第一版没有复杂可视脚注系统，但 provenance 不会在 SlideIR、RenderInput 或 PPTX notes 中丢失。

## 6. Image Pipeline

Image 是 Slice B 的正式 P0 能力，生产链路为：

    SlideAssetIntent
      -> existing billed generation.Service
      -> idempotent generation task
      -> validated PNG/JPEG bytes
      -> Private Storage
      -> ResolvedDeckAsset
      -> stable asset:// identity in SlideIR
      -> renderer-time private byte resolution

Renderer 不调用图片 Provider，也不下载任意 URL。Provider 结果只在 Image Adapter 内获取、校验并写入私有存储；SlideIR 只保存稳定 asset identity、SHA-256、MIME、alt text 与 private file identity，不保存短期 signed URL。

渲染前，Go adapter 通过 Tenant/User scope 从 Private Storage 读取字节并核对 SHA-256，然后以进程内 data URI 交给 Node renderer。缺失资产、跨租户资产、checksum 不匹配或无法从 private storage resolve 都会阻断导出。

## 7. Asset Durability

每个图片意图使用稳定业务键 jobId + stable AssetIntent ID。

既有图片生成任务使用稳定 client_request_id，私有文件使用 business_type = ppt_v2_image_asset 与同一 business identity。Migration 111 增加按 tenant/user/business identity 的 active unique index。

Worker 每成功解析一张图片立即保存 deck_state checkpoint。Retry 或 restart 会先读取已持久化 ResolvedDeckAsset，已成功图片不重新生成、不重复计费。并发写入命中唯一约束后重新读取权威对象；旧 worker 的 checkpoint 仍受 lease/fencing 拒绝，不能覆盖新结果。

图片相关结构化错误包括：

- image_provider_unavailable
- image_timeout
- image_invalid_result
- image_storage_failed

## 8. Professional Layouts

Slice B 只扩展 Professional layouts：

- Cover
- Section
- Title + Body
- Title + Bullets
- Two-column
- Text + Image
- Image + Text
- Key Metric / Highlight
- Closing / Action

所有布局基于 960×540 pt wide canvas 和统一 safe area。每个 semantic slot 在 Layout Compiler 中获得确定 geometry、resolved style 与 z-index；相同输入的 geometry 可重复。

Approved SlideObjective 声明 IMAGE 时只允许 text-image 或 image-text，且必须恰好有一个有效图片意图。该约束防止图片需求被文本-only layout 静默丢弃。

## 9. Quality Gate

导出前执行 fail-closed Professional Quality Gate，覆盖：

- MISSING_TITLE
- MISSING_KEY_MESSAGE
- TEXT_OVERFLOW
- ELEMENT_OVERLAP
- ILLEGAL_BOUNDS
- MISSING_ASSET
- BROKEN_ASSET_REFERENCE
- MISSING_CITATION
- DUPLICATE_SLIDE_IDENTITY
- INVALID_PAGE_ORDER
- PAGE_COUNT_MISMATCH

Contract validation 还会拒绝非法页数、非法 layout、缺失页面内容、证据链断裂和 IMAGE objective/layout mismatch。Quality Gate 失败写入 quality_gate_failed durable error，不调用 Renderer，不创建 PPTX、Work Center Asset 或 Task relation。

## 10. PPTX Renderer

继续使用现有 PptxGenJS Adapter；没有引入 HTML/screenshot PPT pipeline。

多页 Renderer 保持：

- native editable text 与 bullets；
- native editable shapes；
- 图片作为 image element；
- elementId 映射为 Open XML object name；
- LayoutResult z-order；
- speaker notes；
- theme heading/body fonts 与 language；
- deterministic core timestamps、ZIP entry timestamps 与压缩输出。

Renderer 只消费 SlideIR + LayoutResult + resolved private asset bytes。它不修改页面结构、不重新计算 geometry，也不进行 Provider 调用。

## 11. Durable Stages 与 Recovery

Migration 111 将 Agent workflow 的生成阶段扩展为：

    OUTLINE_APPROVED
      -> CONTENT_READY
      -> ASSETS_READY
      -> LAYOUT_COMPILED
      -> QUALITY_CHECKED
      -> RENDERED
      -> FILE_STORED
      -> ASSET_CREATED
      -> TASK_RELATED
      -> COMPLETED

内容生成期间，Job 保持在 OUTLINE_APPROVED，每完成一页即原 stage checkpoint；图片期间保持在 CONTENT_READY，每成功一张即 checkpoint。这让页面关闭、断线和服务重启不会丢失已完成 work。

Worker 继续使用 Phase 2 ready scan、claim、lease、heartbeat 与 fencing token。Restart 从 PostgreSQL 的最新 stage/deck_state 恢复；stale worker 无权提交 checkpoint。Retry 清除当前结构化 error 后从失败 stage 继续，不重复已完成页面、图片、Layout、Render 或 Artifact side effect。

## 12. Real Progress

Progress 由 durable work units 计算，不使用 elapsed time：

    totalWorkUnits = 8 + 2 * approvedSlideCount + imageIntentCount

计数包含已完成 planning 基线、逐页 content、逐图 asset、逐页 layout，以及 quality、render、file storage、artifact/relation completion。Worker 每次 checkpoint 更新 completed_work_units，UI 直接读取服务端 Job Stage/Progress。

## 13. Artifact、Task Relation 与 Download

最终 PPTX 继续复用 Phase 2 链路：

- EnsureTask 使用稳定 client_request_id；
- StorePPTX 写入 Tenant/User scoped Private Storage；
- 同一 Job 通过 business identity 复用已保存文件；
- EnsureArtifact 必须走 fenced durable artifact transaction；
- RelateTask 原子写入 Work Center Asset 与既有 PPT Task relation；
- retry 后仍只有一个有效 PPTX artifact relation。

新增现有 API family 内的下载操作：

GET /api/v1/ppt/agent/jobs/:jobId/download

只有 Job 为 SUCCEEDED / COMPLETED 且存在 private fileId 时才返回短期 signed download URL。读取与下载均使用认证用户的 Tenant/Owner scope。

## 14. Migration 111

最终 migration：

database/migrations/111-ppt-v2-agent-deck-generation.sql

它完成：

- 在 xz_ppt_v2_agent_plans 增加 deck_state jsonb；
- 扩展 GenerationJob stage constraint；
- 允许 AGENT_OUTLINE workflow 在批准后进入 Deck generation；
- 保持 RENDER 与 AGENT_OUTLINE 各自合法 stage/status 约束；
- 为 active ppt_v2_image_asset 增加 tenant/user/business identity unique index。

实现前后已检查 migration 目录，编号 111 无碰撞。Migration 是可前向应用的 schema 扩展，不删除既有 Phase 1/2/A/A.1 数据。

## 15. API 与 UI

Slice B 沿用统一 /api/v1/ppt/agent workflow，没有新增 /api/v2/ppt 或前端 /advance 调度 API。

用户批准大纲后，统一 PPT Agent 工作区轮询权威 Job State，并显示真实产品阶段：

- 正在生成内容；
- 正在准备图片；
- 正在排版；
- 正在检查；
- 正在生成 PPTX；
- 正在保存文件/创建作品/关联项目；
- 演示文稿已完成。

完成后 UI 提供“下载 PPTX”。关闭页面只停止 polling，不停止后台 Worker；重新进入后使用保存的 job identity 恢复状态。Slice B 没有伪装 Preview 已可用，也没有进入拖拽编辑或会话式修改。

## 16. Tests

本地可执行验证在提交前通过：

- go test ./internal/app/ppt ./internal/provider/pptplanning -count=1：PASS；
- Slice B HTTP targeted tests：PASS；
- npm run test:ppt-v2：25/25 PASS；
- Admin PPT Agent planning tests：PASS；
- Admin build：PASS；
- API boundary verification：PASS；
- go vet ./internal/app/ppt ./internal/provider/pptplanning：PASS；
- git diff --check：PASS。

核心新增测试包括：

- 6/8/10/12 页一对一 Multi-page SlideIR；
- 中英文 language preservation；
- Claim → Citation → Source provenance；
- Production SlideContent Provider strict output 与 fail-closed；
- 图片任务幂等、私有存储与稳定 asset identity；
- retry/restart 不重复页面、图片、render 与 artifact；
- stale fencing rejection；
- cross-tenant asset rejection；
- 9 种 Professional layout legal geometry；
- Quality Gate 各阻断诊断；
- HTTP 从 Approved Outline 到 private PPTX download；
- Go → Node compiler 使用 private image；
- deterministic renderer 与 OfficeCLI integrity。

## 17. PostgreSQL Integration Gate

Slice B 新增真实 gate：

TestPostgresGenerationJobAgentDeckGenerationRecoveryFencingAndArtifact

该测试在 PostgreSQL 中验证：

- 8 页 Approved Outline 初始化 DeckJob 与 8 个 SlideJob；
- 第一页 content checkpoint 后模拟失败；
- 重建 Store/Service 模拟服务重启；
- Retry 不重复已 checkpoint 页面；
- 图片、layout、render、artifact 与 relation 各自 effectively-once；
- 最终 Job 为 SUCCEEDED / COMPLETED 且 progress 100%；
- DeckJob/SlideJob 全部成功；
- Task relation 保存最终 deckId/PPTX assetId；
- cross-tenant read 被拒绝。

GitHub Actions #76 的 PostgreSQL 16 service 中，以下六项均真实 PASS、无 Skip：

- TestPostgresGenerationJobLeaseFencingRestartCancelAndIsolation
- TestPostgresGenerationJobArtifactConstraintsAndTransactionRollback
- TestPostgresGenerationJobRetryAndAtomicTaskRelation
- TestPostgresGenerationJobAgentOutlineApprovalRecoveryAndIsolation
- TestPostgresGenerationJobAgentPlanningWorkerRecoveryRetryAndFencing
- TestPostgresGenerationJobAgentDeckGenerationRecoveryFencingAndArtifact

## 18. Golden 2 — Professional Research Deck

新增 8 页 Golden 2 fixture，包含 Cover、文本/要点、Text + Image、Image + Text、事实页来源与 Closing。它验证：

- semantic parity 与稳定 slide/element identities；
- deterministic LayoutResult；
- legal bounds 与无 overlap；
- 至少两页带图片；
- 多个事实页保留 Claim/Citation/Source；
- speaker notes 来源文本；
- 相同输入的 PPTX bytes 可重复；
- Open XML 中存在 8 个 slides、notes 与 image media。

Golden 2 使用冻结 fixture，不调用真实模型或产生外部计费；生产 Image Adapter 的真实任务/私有存储链路由 Go integration/HTTP tests 覆盖。

## 19. OfficeCLI 与 Regression

GitHub Actions #76 的 Windows job 按锁定配置安装 OfficeCLI 1.0.144，校验固定 SHA-256 与版本后真实执行完整 Node regression。

CI 日志确认：

- Golden 1 Professional Business Deck semantic and geometry parity：PASS；
- Golden 1 frozen renderer repeatability：PASS；
- Golden 1 OfficeCLI no-repair validation：PASS、未 Skip；
- 6/8/10/12 页 Multi-page tests：PASS；
- Golden 2 OfficeCLI no-repair validation：PASS、未 Skip；
- Node regression：310 tests、310 PASS、0 FAIL、0 SKIP。

因此 Phase 1 Golden 1/PptxGenJS、Phase 2 PostgreSQL、Slice A approval、Slice A.1 planning worker、existing PPT/Connector 与 protected frontend surfaces 均保持绿色。

## 20. GitHub Actions Evidence

实现提交 97f0d58d1 对应 [user-core #76](https://github.com/lmxchyy/zhiqiyun-ai/actions/runs/31933646139)。Actions 在 PR merge commit 45ed73a09 上验证当前 main 基线与 Slice B head 的组合：

- user-core / backend-go：GREEN；
- user-core / user-core：GREEN；
- PostgreSQL 六个 GenerationJob gates：PASS、无 Skip；
- 完整 go test ./...：PASS；
- PPT V2 Node/Golden/OfficeCLI tests：PASS；
- Admin unit tests：PASS；
- user-core typecheck：PASS；
- API client boundaries：PASS；
- H5/Admin/微信小程序/App Plus/Harmony App/Harmony 小程序 builds：PASS；
- H5 smoke tests：7/7 PASS。

本次 run 没有 NEW_FAILURE。

## 21. Known Limitations

- Slice B 只生成 Professional Deck；Creative Mode、动画、音视频页不在范围内。
- Chart/Table/Diagram/Timeline/Gantt/Organization Chart 尚未实现；Chart 作为 P1 没有阻塞主闭环。
- 当前生产图片链路接入既有 AI image generation；Web/stock 搜索和用户上传图片选择尚未接入 Agent generation UI。
- 页面内容按 SlideObjective 顺序逐页调用 Provider；大页数时总耗时仍受模型 latency 影响，但每页均有 durable checkpoint。
- 文本 overflow 使用布局 slot 的字符容量阈值，不是 PowerPoint 字体渲染引擎的逐字测量。
- Citation 第一版以数据 provenance 和 speaker notes Sources 为主，没有复杂可视脚注/参考文献编辑器。
- UI 使用 polling，没有 SSE；但进度来自 durable server state，不是 elapsed-time fake progress。
- Slice C Preview Workspace 未实现，用户完成前只能看到真实阶段，完成后下载 PPTX。
- Slice D conversational edit、局部 regenerate、Revision/Undo 尚未实现。
- 当前没有 Google Slides 或 PDF 导出。

## 22. Slice B Exit Gate

| Exit Gate | 最终证据 | 结果 |
| --- | --- | --- |
| Approved Outline 生成 6～12 页完整 SlideIR | 6/8/10/12 页 contract tests | PASS |
| 一页 Outline 对应一页 SlideIR | page count、stable slide IDs、content cardinality validation | PASS |
| SlideIR/LayoutResult separation | SlideIR 无 geometry；Layout Compiler 唯一 geometry authority | PASS |
| Production SlideContent PlanningPort | OpenAI-compatible provider；strict contract；fail closed | PASS |
| factual slides provenance 保留 | Slide/element → Claim → Citation → Source + speaker notes | PASS |
| Image 进入统一 Asset pipeline | billed task → private file → stable assetRef | PASS |
| 至少部分页面包含 Image | Golden 2 与 Go→Node HTTP test | PASS |
| 图片 retry/restart/effectively-once | stable task/business IDs、per-asset checkpoint、unique index | PASS |
| stale worker 不能覆盖结果 | lease/fencing unit 与 PostgreSQL gate | PASS |
| Professional layouts 支持多页 | 9 种布局 geometry test | PASS |
| Quality Gate fail closed | citation/asset/order/count/overlap/bounds/overflow tests | PASS |
| PPTX 可下载 | authenticated private signed URL HTTP test | PASS |
| PPTX 可正常打开 | Golden 1 + Golden 2 OfficeCLI real validation | PASS |
| Artifact effectively-once | render/store/artifact/relation restart tests | PASS |
| Tenant/Owner isolation | Job read、private image、download scope tests | PASS |
| Golden 1 | CI #76 semantic/geometry 与 repeatability PASS | PASS |
| Golden 2 | 8 页 Professional Research Deck PASS | PASS |
| OfficeCLI | 1.0.144 checksum/version + 两项真实 validate PASS | PASS |
| Phase 2 PostgreSQL gates | 三项 PASS、无 Skip | PASS |
| Slice A PostgreSQL gate | approval/recovery/isolation PASS、无 Skip | PASS |
| Slice A.1 PostgreSQL gate | planning recovery/retry/fencing PASS、无 Skip | PASS |
| Slice B PostgreSQL gate | deck recovery/fencing/artifact PASS、无 Skip | PASS |
| backend-go | CI #76 GREEN | PASS |
| user-core | CI #76 GREEN | PASS |
| 无 NEW_FAILURE | CI #76 两个 checks 全绿 | PASS |

Slice B 的全部 Exit Gate 已满足。

本结论不授权合并 PR，不进入 Slice C，也不扩展 conversational editing 或 Preview。

## 最终结论

SLICE B STATUS: READY
