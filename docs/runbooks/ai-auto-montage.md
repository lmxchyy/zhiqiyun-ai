# AI 自动混剪运维手册

本手册覆盖 V1 生产路径：分析 / 规划 / 渲染 Worker、事务 Outbox、个人积分冻结与作品发布。  
旧 Smoke 直入队路径已退役，请勿再按 `docs/ai-smart-video-render-worker-runbook.md` 手工 LPUSH Redis。

## 服务组成

| 组件 | 作用 |
|---|---|
| API (`xianzhi-ai`) | 项目、素材、方案、导出 HTTP；写任务 + Outbox |
| `smartvideo-worker` | 消费 Outbox → Redis 队列；跑 analysis/plan/render |
| PostgreSQL | 领域表 + `video_task_outbox` |
| Redis | analysis/plan/render 队列 |
| 对象存储 | 素材与成片 |
| FFmpeg/FFprobe | 探测与 Manifest 渲染 |
| Planner / Speech Provider | 方案生成与配音 |

## 启动（开发）

```powershell
docker compose --profile smartvideo up -d --build migrate xianzhi-ai smartvideo-worker
docker compose ps
docker compose logs -f smartvideo-worker
```

## 启动（生产）

`compose.prod.yml` 中的 `smartvideo-worker` 服务，关键环境变量：

- `SMARTVIDEO_ANALYSIS_ENABLED=true`
- `SMARTVIDEO_OUTBOX_ENABLED=true`
- `SMARTVIDEO_WORKER_CONCURRENCY` / `SMARTVIDEO_RENDER_WORKER_CONCURRENCY`
- `SMARTVIDEO_TEMP_DIR`（专属临时盘）
- `SMARTVIDEO_FFMPEG_PATH` / `SMARTVIDEO_FFPROBE_PATH`
- Planner / Speech Base URL 与 API Key

## Outbox 积压

症状：任务已创建但队列无消息；`video_task_outbox.state='pending'` 增长。

处理：

1. 确认 worker 进程存活与 `/tmp/smartvideo/worker.healthy`（或健康探针）。
2. 查看 outbox `last_error` / attempts。
3. Redis 短暂故障后 OutboxPublisher 会退避重放；勿手工双写队列。
4. 若需强制重放：将对应行 `state` 置回 `pending` 并清空 `available_at`（仅运维窗口）。

## Provider 故障

- Planner 429/5xx：plan task 标记 FAILED，可用户侧重新生成；积分未冻结。
- Speech 失败：render 阶段失败并 `Release` 已冻结积分。
- 对象存储失败：保持任务可重试，勿重复 Capture。

## 磁盘与 OOM

- 渲染并发默认 1；临时盘不足时降低 `SMARTVIDEO_RENDER_WORKER_CONCURRENCY`。
- 清理 `SMARTVIDEO_TEMP_DIR` 下超过 24h 的任务目录。

## 积分异常

- 导出走 `PersonalPointsLifecycle`（`SMART_VIDEO_RENDER`）。
- 取消/终态失败必须 Release 一次；成功 Capture 一次。
- 禁止绕过 ExportService 手工插入 QUEUED 渲染任务。

## 容量基准（上线前）

```powershell
# 冒烟（本机 ffmpeg + compose.prod worker 配置）
node scripts/smartvideo-capacity-check.mjs

# 全量（上线前必跑）：注入真实渲染探针命令
$env:SMARTVIDEO_CAPACITY_FULL="1"
$env:SMARTVIDEO_CAPACITY_PROBE_CMD="<your render probe command>"
$env:SMARTVIDEO_CAPACITY_OUTPUT_MS="60000"
node scripts/smartvideo-capacity-check.mjs
# 目标：60s 成片、8 混合素材、720p 实时系数 ≤ 5，连续 50 次成功率 ≥ 95%
```

详见 `docs/release/ai-auto-montage-v1-checklist.md`。

## 观测指标

Worker 每 30s（`SMARTVIDEO_METRICS_INTERVAL`）输出：

- `analysis_*` / `plan_*` / `render_*` 队列 pending/working/delayed/dead
- `outbox_pending`、`outbox_oldest_age_ms`
- `plan_provider` / `speech_provider` 就绪标志

健康探针：`/tmp/smartvideo/worker.healthy`（DB + Redis + ffmpeg/ffprobe 周期复检）。
