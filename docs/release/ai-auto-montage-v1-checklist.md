# AI 自动混剪 V1 发布清单

分阶段判定：`GO` / `CONDITIONAL` / `NO-GO`。未满足强制项一律 `NO-GO`。

> 草稿签发：2026-08-11 · commit `7b166d051` · 分支 `codex/channel-ecosystem-v132-phase3`（ahead 1，未 push）

## Phase A — 生产闭环（强制）

| 检查项 | 证据 | 判定 |
|---|---|---|
| PersonalPointService 已接线 PointsLifecycle | `smartvideo_points_bridge.go` + `personal_points_lifecycle*.go` + billing 集成测试 | GO |
| Analysis/Plan/Render 均事务 Outbox | `Enqueue*WithOutbox` / `Create*WithOutbox` + `smartvideo_outbox.go` | GO |
| Smoke 直入队已关死 | `Service.CreateRenderTask` → `ErrExportNotReady`；`TestLegacyCreateRenderTaskIsDisabled` | GO |
| `/points/account` total 语义未改（W1/M5） | 本变更未触碰 points account；保护面回归仍要求保持 | GO |

## Phase B — 客户端主路径（强制）

| 检查项 | 证据 | 判定 |
|---|---|---|
| Web `userSmartVideo` 工作台可创建→方案→导出 | `tests/ai-auto-montage-web.test.mjs` 绿；**真链路手工冒烟待做** | CONDITIONAL |
| 小程序 `packageSmartVideo` 可创建→方案→导出 | `tests/ai-auto-montage-mini.test.mjs` + `packageSmartVideo` 页面；**真链路手工冒烟待做** | CONDITIONAL |
| M6 featured 仍为「AI设计」「自由P图」 | `tests/user-mini-smart-video-montage.test.mjs`；混剪在 featured 之后 | GO |
| 无页面 `uni.request` / Axios | mini/web 门禁 + `verify:api-boundaries`（admin multipart 已改 `adminFetchResponse`） | GO |

## Phase C — Worker / 运维（强制）

| 检查项 | 证据 | 判定 |
|---|---|---|
| `compose.prod.yml` 含 `smartvideo-worker` | `compose.prod.yml` service `smartvideo-worker` | GO |
| Outbox / Analysis 默认开启 | `SMARTVIDEO_OUTBOX_ENABLED` / `SMARTVIDEO_ANALYSIS_ENABLED` 默认 `true` | GO |
| 运维手册就绪 | `docs/runbooks/ai-auto-montage.md` | GO |
| 容量脚本可跑冒烟 | `node scripts/smartvideo-capacity-check.mjs`（本地验证期已绿） | GO |
| 全量容量（上线前） | 需 `SMARTVIDEO_CAPACITY_FULL=1` + 真实探针 | **NO-GO until done** |

## Phase D — 安全与回归（强制）

| 检查项 | 证据 | 判定 |
|---|---|---|
| 租户隔离 / 幂等 / 配额 / 状态机 | Go smartvideo + httpserver 聚焦测试已绿；全量 `go test ./...` 依赖本地 test DB | CONDITIONAL |
| 拒绝任意 URL/本地 path 入素材 | `TestSmartVideoAssetPayloadRejectsRemoteURLAndLocalPath` | GO |
| FFmpeg argv 无 shell 注入 | `manifest_renderer_test.go` | GO |
| 重复扣费/重试不双扣 | `billing_publish_integration_test.go` | GO |
| 保护面 W1–W3 / M1 / M2 / M6 / M7 绿 | H5 smoke 7/7；mini click e2e 绿；M8 已写入 protected-surfaces | CONDITIONAL（缺生产侧手工确认） |
| API/Web/Mini 门禁 | `node --test tests/ai-auto-montage-*.test.mjs` | GO |

## 推荐验证命令

```bash
cd backend-go && go test ./internal/app/smartvideo ./internal/infra ./internal/provider/media ./internal/httpserver -count=1
node --test tests/ai-auto-montage-api.test.mjs tests/ai-auto-montage-web.test.mjs tests/ai-auto-montage-mini.test.mjs
node scripts/smartvideo-capacity-check.mjs
npm run typecheck:packages
npm --prefix admin-vue run typecheck
npm --prefix apps/user-uni run typecheck
npm run verify:api-boundaries
```

本地已额外确认：
- `npm run test:user-h5:smoke` → 7/7
- `npm run verify:user-mini-clicks:e2e` → pass
- `npm run test:pc-web:smoke` → 曾绿（API up）

## 最终签发

- 总判定：**CONDITIONAL**
- 条件说明：
  1. 上线前必须跑通 `SMARTVIDEO_CAPACITY_FULL=1` 全量容量探针
  2. Web / 小程序「创建 → 方案 → 导出」真链路手工冒烟各 1 次
  3. 预发执行迁移 `106-ai-auto-montage-v1`、`107-storage-multipart-upload`，并确认 `smartvideo-worker` 健康
  4. push `7b166d051` 后再走 `deploy.sh`（dirty tree 不得绕过；当前另有未提交 PPT 草稿，部署前勿混入）
- 签发人 / 日期：待人工确认 · 2026-08-11
