# AI 自动混剪 V1 发布清单

分阶段判定：`GO` / `CONDITIONAL` / `NO-GO`。未满足强制项一律 `NO-GO`。

> 更新：2026-08-11 · tip `4a7a5dc63` · 分支 `codex/channel-ecosystem-v132-phase3`  
> 已同步：`origin` + `gitee`  
> **生产部署**：`119.29.191.227` / `ai.zs-kjhn.cn` · HEAD=`4a7a5dc` · 迁移 `106`/`107` 已应用 · `smartvideo-worker` healthy · API health ok

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
| 容量脚本可跑冒烟 | `node scripts/smartvideo-capacity-check.mjs`（2026-08-11 再验绿） | GO |
| 全量容量（上线前） | 需 `SMARTVIDEO_CAPACITY_FULL=1` + 真实探针 | **NO-GO until done**（部署后仍待跑） |
| 生产迁移 `106`/`107` | 2026-08-11 已对生产显式 `MIGRATION_FILES` 应用；表 `video_task_outbox` / `xz_multipart_uploads` 已存在 | GO |
| 生产 `smartvideo-worker` | 容器 healthy；outbox 指标可采集；`/tmp/smartvideo/worker.healthy` 存在 | GO |
| 生产 API | HEAD=`4a7a5dc`；`/api/v1/health` ok；`/api/v1/video-projects` → 401（路由已挂） | GO |

## Phase D — 安全与回归（强制）

| 检查项 | 证据 | 判定 |
|---|---|---|
| 租户隔离 / 幂等 / 配额 / 状态机 | `go test ./internal/app/smartvideo` 绿；httpserver 混剪相关 `-run SmartVideo|...` 绿 | CONDITIONAL |
| 拒绝任意 URL/本地 path 入素材 | `TestSmartVideoAssetPayloadRejectsRemoteURLAndLocalPath` | GO |
| FFmpeg argv 无 shell 注入 | `manifest_renderer_test.go` | GO |
| 重复扣费/重试不双扣 | `billing_publish_integration_test.go` | GO |
| 保护面 W1–W3 / M1 / M2 / M6 / M7 绿 | H5 smoke 7/7；mini click e2e 绿；M8 已写入 protected-surfaces | CONDITIONAL（缺生产侧手工确认） |
| API/Web/Mini 门禁 | `node --test tests/ai-auto-montage-*.test.mjs` → 10/10（2026-08-11 再验） | GO |

## 推荐验证命令

```bash
cd backend-go && go test ./internal/app/smartvideo ./internal/infra ./internal/provider/media -count=1
cd backend-go && go test ./internal/httpserver -count=1 -run "TestLegacyCreateRenderTaskIsDisabled|TestSmartVideoAssetPayloadRejectsRemoteURLAndLocalPath|SmartVideo"
node --test tests/ai-auto-montage-api.test.mjs tests/ai-auto-montage-web.test.mjs tests/ai-auto-montage-mini.test.mjs
node scripts/smartvideo-capacity-check.mjs
```

本地已额外确认：
- `npm run test:user-h5:smoke` → 7/7
- `npm run verify:user-mini-clicks:e2e` → pass
- 跟踪工作区已干净（PPT WIP 已 `stash@{0}: wip(ppt): park before montage go-live`；未跟踪 PPT 草稿挪到 `.local-verify/parked-ppt-before-montage/`）

## 生产/预发执行清单（下一步，需服务器）

在部署机（通常 Gitee 拉取）按顺序：

```bash
cd /opt/xianzhi-ai   # 或实际目录
git fetch --prune gitee
git checkout codex/channel-ecosystem-v132-phase3
git pull --ff-only gitee codex/channel-ecosystem-v132-phase3
# 确认 tip == 4a7a5dc63（或其后 ff）
git rev-parse --short HEAD

./backup.sh
./deploy.sh
# deploy 会拉当前分支并重建 compose.prod（含 smartvideo-worker + migrate）

docker compose -f compose.prod.yml --env-file .env.production ps
docker compose -f compose.prod.yml --env-file .env.production logs --tail=100 smartvideo-worker
# 确认 migrate 已应用 106 / 107；worker healthy

# 全量容量（注入真实渲染探针后）
SMARTVIDEO_CAPACITY_FULL=1 \
SMARTVIDEO_CAPACITY_PROBE_CMD='...' \
SMARTVIDEO_CAPACITY_OUTPUT_MS=60000 \
node scripts/smartvideo-capacity-check.mjs
```

手工冒烟（各 1 次）：
1. Web：创建项目 → 上传素材 → 出方案 → 导出成片
2. 小程序：同上；确认首页 featured 仍为「AI设计」「自由P图」

## 最终签发

- 总判定：**CONDITIONAL**（**部署已完成**；仍缺全量容量探针 + Web/小程序真链路手工冒烟）
- 条件说明：
  1. 上线前必须跑通 `SMARTVIDEO_CAPACITY_FULL=1` 全量容量探针
  2. Web / 小程序「创建 → 方案 → 导出」真链路手工冒烟各 1 次
  3. ~~预发/生产执行迁移 `106`、`107`，并确认 `smartvideo-worker` 健康~~ ✅ 2026-08-11 已完成
  4. ~~干净树 `deploy.sh`~~ ✅ 已完成（HEAD=`4a7a5dc`）
- 签发人 / 日期：部署完成待产品冒烟确认 · 2026-08-11
