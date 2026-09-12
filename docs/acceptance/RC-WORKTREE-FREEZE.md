# RC Worktree Freeze

- **冻结时间**：2026-09-10T02:31:41+08:00
- **冻结前基线**：`main` / `HEAD bc00ae1f7940a12cfdc571b342b091d704d39442`
- **`origin/main`**：`e8846b0c5f49735df09f9c1639e3dbb8ca84d6be`
- **冻结前状态**：44 个 `git status --short` 条目（22 modified + 22 untracked）
- **原则**：不删除、不重置、不覆盖任何现有条目；本文件及冻结报告属于本轮新增证据文档，不计入冻结前 44 项。

## 来源分类边界

Git 状态本身不能证明每个修改由谁、在哪一轮产生。以下分类按路径和当前 RC 证据内容归类，仅用于冻结与审计，不作为删除或回滚依据。无法证明来源的条目不被擅自归入“既有修改”。

- **A — 本轮 RC burn-down 代码、配置与专项测试**：24 项
- **B — 可从当前证据确认的既有修改**：0 项
- **C — 测试、验收、设计、发布与保护面证据**：20 项
- **D — 临时文件**：0 项（ignored 文件不计入这 44 项）
- **E — 不明来源**：0 项

## A. RC burn-down 代码与配置（21）

```text
apps/user-uni/src/components/assets/AssetDetailCenterPage.vue
apps/user-uni/src/features/assets/api.ts
apps/user-uni/src/features/assets/types.ts
backend-go/internal/app/ppt/service.go
backend-go/internal/app/ppt/service_test.go
backend-go/internal/config/config.go
backend-go/internal/httpserver/api.go
backend-go/internal/httpserver/connector_queue.go
backend-go/internal/httpserver/connector_queue_test.go
backend-go/internal/httpserver/generation_storage.go
backend-go/internal/httpserver/generation_storage_test.go
backend-go/internal/httpserver/payment_center_api.go
backend-go/internal/httpserver/postgres_store.go
backend-go/internal/httpserver/ppt_api.go
backend-go/internal/httpserver/ppt_storage.go
backend-go/internal/httpserver/types.go
backend-go/internal/httpserver/wechat_virtual_entitlements.go
backend-go/internal/httpserver/wechat_virtual_payment_postgres_test.go
compose.prod.yml
compose.yml
deploy.sh
```

## A. 视频专项新增测试（3；A 合计 24）

```text
backend-go/internal/httpserver/video_async_real_canary_test.go
backend-go/internal/httpserver/video_history_defense_test.go
backend-go/internal/httpserver/video_persistence_test.go
```

## C. 保护面、验收、设计、发布与测试证据（20）

```text
docs/regression/protected-surfaces.md
docs/acceptance/P1-ASSET-LIFECYCLE-EVIDENCE.md
docs/acceptance/P1-BACKUP-RESTORE-EVIDENCE.md
docs/acceptance/P1-CONNECTOR-EVIDENCE.md
docs/acceptance/P1-PAYMENT-EVIDENCE.md
docs/acceptance/P1-WORKER-RUNTIME-EVIDENCE.md
docs/acceptance/RC-BURN-DOWN-PLAN.md
docs/acceptance/RC-EVIDENCE-CLOSURE-REPORT.md
docs/acceptance/RC-EVIDENCE-MATRIX.md
docs/acceptance/RC-FINAL-AUDIT-REPORT.md
docs/acceptance/VIDEO-ASSET-PERSISTENCE-IMPLEMENTATION-REPORT.md
docs/acceptance/VIDEO-ASSET-RC-FINAL-ACCEPTANCE.md
docs/acceptance/VIDEO-ASYNC-CANARY-REVALIDATION.md
docs/acceptance/VIDEO-ASYNC-REAL-CANARY-REPORT.md
docs/acceptance/VIDEO-HISTORY-PLAYBACK-DIAGNOSIS.md
docs/acceptance/fixes/
docs/design/
docs/release/production-progress-20260905.md
tests/video-asset-persistence.test.mjs
tests/video-playback-defense.test.mjs
```

> `git status --short` 将 `docs/acceptance/fixes/` 和 `docs/design/` 各显示为一个 untracked directory 条目；目录内文件不再重复计数。因此 C=20，A=24，合计正好 44。冻结后新生成的 `RC-WORKTREE-FREEZE.md` 与 `RC-FREEZE-REPORT.md` 不属于冻结前基线。

## 当前不能从冻结分类推出的事项

- 未发现可明确归类为 D 临时文件的 status 条目；ignored 的 `backend-go/tmp` 等不在 44 项内。
- 未发现可由当前 Git 证据确认的 B 既有修改；这不表示所有 A/C 文件都由同一人产生，只表示不能凭空断言其来源。
- 不明来源文件不删除、不重命名、不移动。

## 冻结规则

1. 后续只允许新增证据文档或修正证据报告中的事实矛盾；禁止业务逻辑、功能、配置和测试范围扩张。
2. 不删除或清理不明来源文件；不执行 reset、clean、checkout 覆盖。
3. `docs/regression/protected-surfaces.md` 的既有 trailing whitespace 不在本轮处理范围。
4. 发布前必须由授权人员将最终变更整理为单一、已推送且可复现的 release commit；当前冻结状态不能部署。
