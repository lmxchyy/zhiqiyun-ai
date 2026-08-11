# AI 自动混剪 V1 发布清单

分阶段判定：`GO` / `CONDITIONAL` / `NO-GO`。未满足强制项一律 `NO-GO`。

## Phase A — 生产闭环（强制）

| 检查项 | 证据 | 判定 |
|---|---|---|
| PersonalPointService 已接线 PointsLifecycle | `smartvideo_points_bridge.go` + billing 集成测试 | |
| Analysis/Plan/Render 均事务 Outbox | `Enqueue*WithOutbox` / `Create*WithOutbox` | |
| Smoke 直入队已关死 | `Service.CreateRenderTask` → `ErrExportNotReady` | |
| `/points/account` total 语义未改（W1/M5） | `TestPointAccountTotalIncludesConsumedPoints` | |

## Phase B — 客户端主路径（强制）

| 检查项 | 证据 | 判定 |
|---|---|---|
| Web `userSmartVideo` 工作台可创建→方案→导出 | `tests/ai-auto-montage-web.test.mjs` + 手工冒烟 | |
| 小程序 `packageSmartVideo` 可创建→方案→导出 | `tests/ai-auto-montage-mini.test.mjs` | |
| M6 featured 仍为「AI设计」「自由P图」 | `tests/user-mini-smart-video-montage.test.mjs` | |
| 无页面 `uni.request` / Axios | mini/web 门禁测试 | |

## Phase C — Worker / 运维（强制）

| 检查项 | 证据 | 判定 |
|---|---|---|
| `compose.prod.yml` 含 `smartvideo-worker` | `docker compose -f compose.prod.yml config` | |
| Outbox / Analysis 默认开启 | `SMARTVIDEO_OUTBOX_ENABLED` / `SMARTVIDEO_ANALYSIS_ENABLED` | |
| 运维手册就绪 | `docs/runbooks/ai-auto-montage.md` | |
| 容量脚本可跑冒烟 | `node scripts/smartvideo-capacity-check.mjs` | |
| 全量容量（上线前） | `SMARTVIDEO_CAPACITY_FULL=1` + 真实探针；60s / 8 素材 / RT≤5 / 50 次 ≥95% | |

## Phase D — 安全与回归（强制）

| 检查项 | 证据 | 判定 |
|---|---|---|
| 租户隔离 / 幂等 / 配额 / 状态机 | Go smartvideo + httpserver 测试 | |
| 拒绝任意 URL/本地 path 入素材 | `TestSmartVideoAssetPayloadRejectsRemoteURLAndLocalPath` | |
| FFmpeg argv 无 shell 注入 | `manifest_renderer_test.go` | |
| 重复扣费/重试不双扣 | `billing_publish_integration_test.go` | |
| 保护面 W1–W3 / M1 / M2 / M6 / M7 绿 | 见 `docs/regression/protected-surfaces.md` | |
| API/Web/Mini 门禁 | `node --test tests/ai-auto-montage-*.test.mjs` | |

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

## 最终签发

- 总判定：`GO` / `CONDITIONAL` / `NO-GO`
- 条件说明：
- 签发人 / 日期：
