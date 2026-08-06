# Task 2 报告 — 受控 PPT Agent Phase 1 migration 106

## 结论

- 状态：`IMPLEMENTED_AND_VERIFIED`，但不是发布或生产上线批准。
- 已实现 migration 106 的精确 legacy→final 迁移、精确 final 重跑只读 no-op，以及仅在没有 Phase 1 新写入时允许的 down。
- 独立审查 round 1 的空 Feishu 幂等链 P1 已按 RED/GREEN 修复；当前提交等待 round 2 独立复核，不沿用 round 1 的失败结论作为批准。
- 已在本地专用 PostgreSQL 16 数据库 `ppt_agent_phase1_migration_codex_20260806` 完成真实迁移验证；全部 fixture 已清理，数据库本身按授权保留。
- 未 push、merge、deploy，未访问共享或生产数据库，未执行生产迁移、外部服务调用或流量切换。
- 预提交 HEAD 为 `1ddb727a17990b915f6ecc3008a766c7d4f3d3d6`；本层要求使用单一提交主题 `feat(db): add ppt agent phase1 migration`。

## 修改文件

1. `database/migrations/106-ppt-agent-phase1.sql`
2. `database/rollbacks/106-ppt-agent-phase1.down.sql`
3. `backend-go/internal/app/ppt/migration_106_integration_test.go`
4. `backend-go/internal/app/ppt/postgres.go`
5. `backend-go/internal/app/ppt/postgres_test.go`
6. `.superpowers/sdd/ppt-agent-phase1-regression-integration-20260806/task-2-report.md`

没有修改两个外部 102 版本、101/103–105、前端、Connector、billing/provider 生产逻辑或部署配置。

## 数据库变化与调用链

### Up

- 只接受精确 legacy 或精确 final `public.xz_ppt_tasks`；任何额外/缺失列、索引、约束或 102-like 中间态均原子失败。
- 整个过程使用单事务、事务级 advisory lock、5 秒 lock timeout、45 秒 statement timeout 和显式 `ACCESS EXCLUSIVE` 表锁。
- 精确 final 分支只校验既有数据并返回，不执行 DDL 或数据写入；真实 PostgreSQL 测试以 catalog、row `xmin` 和 `ctid` 指纹证明重跑无变化。
- legacy 仅允许 `success`、`failed`、`cancelled` 历史终态；拒绝 pending、processing、未知状态和 malformed raw。
- 每个历史任务的 tenant 只能来自以下去重证据：
  - `xz_billing_events`：`task_id + user_id` 且 `metric_code='ppt.generations'`；
  - `xz_generation_tasks`：`user_id + client_request_id`，并同时满足 `module_code='ppt_generation'`、`type='PPT_GENERATION'`、Feishu `source_type` 和 `source_task_id`。PPT `client_request_id`、generation `client_request_id`、`params.source_task_id` 三端都必须 trim 后非空，并继续使用未修剪值精确相等；empty-to-empty 永不构成证据。
- 相同 tenant 的多条证据允许；零 tenant、冲突 tenant、空 tenant、长度超过 128、tenant 不存在或投影唯一性冲突均失败。
- 新增精确列：non-default `tenant_id`、nullable `session_id`、`skill_code`、`stage`、`source_file_ids`。历史投影固定为 `session_id=NULL`、`skill_code='general'`、`source_file_ids=[]`，并只映射 `success→READY`、`failed→FAILED`、`cancelled→CANCELLED`。
- 新建三个 tenant-aware 索引后才删除三个 user-only legacy 索引；新增 tenant/session nonblank、source JSON array 和六组精确 stage/status CHECK。
- 不添加跨模块 tenant 外键，不读取 `tenant_default`、membership、请求 payload 或 raw tenant，不用 `raw.slides` 推断 READY，不改写历史 raw/status/timestamps。

### Down

- 只接受精确 final 或精确 legacy；legacy 为 no-op，中间态原子失败。
- 若出现 session、非 `general` skill、非空 source、非历史终态映射、Phase 1 raw 标记、无法用相同非空三端链重新证明的 tenant，或回退后会破坏 legacy user/client-request 唯一性，立即拒绝。
- 安全回退时先恢复三个 legacy 索引，再删除本 migration 引入的三个索引、四个 CHECK 和五列；历史七列及非 PPT 表/数据保持不变。

### Runtime readiness

- PostgreSQL 16 将正确的 varchar partial-index predicate 反解析为 `((client_request_id)::text <> ''::text)`。
- readiness 现在只把这一条精确列/操作符/空字面量的无害 cast/外层括号形式规范成既有 canonical predicate。
- 不同列、`=`、非空字面量、追加 `OR` 仍被拒绝；没有 runtime DDL、fallback 或 schema repair。

## TDD 证据

### RED

1. readiness 新测试先以 PG16 catalog predicate 运行，旧 normalizer 拒绝正确索引。
2. migration 合同测试先运行：因 `database/migrations/106-ppt-agent-phase1.sql` 尚不存在而按预期失败。
3. round 1 P1 回归测试先只增加真实 PG 场景；旧 up/down 均错误成功，两个子测试都精确失败为 `migration succeeded, want fail-closed error`。

### GREEN

- readiness 聚焦测试：PASS，`ok .../internal/app/ppt 2.727s`。
- migration 合同测试：PASS。
- 专用 PG16 migration 测试：PASS，覆盖 14 个子场景：
  - exact old→new、相同 tenant 重复证据、三个历史 stage 投影；
  - exact-final rerun no-op；
  - tenant-aware unique 与索引 key/order/predicate；
  - partial 102-like、缺失/冲突证据、pending/processing/unknown、malformed raw、tenant 不存在、tenant 超长均原子失败；
  - blank PPT request、blank generation request、blank `source_task_id` 的 Feishu empty-to-empty 链在 up 不能提供 tenant，down 也不能重新证明 tenant；两个方向都用完整 catalog/data 指纹确认失败后零变化；
  - down 在新写入前精确恢复 legacy，新写入后原子拒绝；
  - non-PPT catalog/data 指纹不变，迁移后真实 `ensurePostgresReady` 通过且无 runtime DDL。

## 最终验证

1. 带专用 migration DSN 的 migration 合同与真实 PG 聚焦测试：
   - `go test ./internal/app/ppt -run '^TestPPTAgentPhase1Migration106(Contract|Postgres)$' -count=1`
   - PASS：`ok .../internal/app/ppt 4.610s`。
2. 带同一专用 migration DSN 的完整 PPT 包：
   - `go test ./internal/app/ppt/... -count=1`
   - PASS：`ppt 4.940s`、`ppt/skills 1.075s`。
3. PPT HTTP / export / projection / visual / Connector 定向回归：
   - `go test ./internal/httpserver -run 'PPT' -count=1`
   - PASS：`ok xianzhi-ai/backend-go/internal/httpserver 18.680s`。
4. Task 0 受保护聚合回归：
   - PASS：Feishu `2.979s`、HTTP `8.095s`、video provider `2.345s`、storage `2.291s`。
5. `go vet ./internal/app/ppt/...`：PASS。
6. migration 编号检查：PASS；仓库内 `106*` 仅本任务的一份 up 与一份 down。
7. `git diff --check`：PASS。
8. 专用数据库清理检查：PASS；测试完成后 `public` schema 没有遗留 fixture relation。

本任务没有观察到基线失败。

## API、受保护面与未验证项

- 无 API 契约变化；仅数据库 migration、数据库 readiness 的精确 catalog 归一化和测试变化。
- 已确认：完整 `app/ppt`、PPT HTTP/导出/视觉/Connector 定向面，以及 Task 0 的计费、视频、存储和 Feishu 受保护回归仍通过。
- 未测：首页新旧模板、前端 UI、小程序构建、真实浏览器、真实 Provider、共享/预发/生产数据库；不得据此宣称全产品或生产验收完成。
- 生产运行前仍需对真实数据库做只读 legacy/final shape、活跃任务、证据覆盖率、tenant 冲突和 lock window 审计。任何拒绝条件命中都应保持 fail-closed，不得临时放宽 migration。

## 风险、回滚与下一步

- migration 需要短时 `ACCESS EXCLUSIVE` 锁；5 秒无法取锁会安全失败，必须在批准维护窗口重试，而不是提高风险绕过。
- 没有 tenant 外键是明确设计；tenant 存在性只在受控迁移时验证，运行时隔离仍由 owner-bound 服务链路和 tenant-aware 索引保障。
- 回滚只适用于尚无 Phase 1 新写入的窗口；一旦有新写入，down 会拒绝。此时应停止并制定保留新数据的独立恢复方案，不能强删列。
- 本地专用测试数据库可供后续复核，当前为空；不要把它当成共享或发布环境。
- 下一步应由独立 QA / security / release 角色复核本提交和真实环境只读审计结果，再决定 release GO / NO-GO。
