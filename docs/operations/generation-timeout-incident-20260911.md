# 生图任务超时事故复盘（2026-09-11）

## 影响

网页端 `IMAGE_TO_IMAGE / gpt-image-2` 任务在上游执行结果不明确时长期保持 `PROCESSING`，占用套餐并发并保留点数预留。受影响任务：`task_000221`、`task_000232`、`task_000242`。

## 根因

任务 context 超时后，上游执行被标记为 `UNKNOWN / possibly_submitted`，但 `runGenerationTask` 没有保证任务进入终态；启动时 stale repair 又会跳过 UNKNOWN，且原流程没有周期性 watchdog。

## 应急处理

- 通过 `deploy.sh` 发布超时回收修复。
- 服务启动后安全扫描并收敛 3 个无 provider request ID、无结果且超过宽限期的 UNKNOWN 任务。
- 任务均标记为 `FAILED`，通过事务释放预扣点数：12、12、18。
- 生产复查：主服务和 worker healthy，active 任务数为 0。

## 永久修复

- 增加可停止的周期性 stale watchdog。
- 超时使用独立 context 执行 durable 收敛。
- UNKNOWN 任务仅在无 request ID、无结果且超过显式宽限期后才允许终结。
- 普通 UNKNOWN 仍禁止盲目重提和直接退款。
- 视频归档失败统一走 durable 失败流程。
- 增加超时、watchdog、UNKNOWN grace 测试。

## 验证

- `go test ./internal/httpserver -run 'TestGeneration(StaleWatchdog|UnknownGrace)|TestRepairStaleGenerationTasks'`
- 生产发布使用 `deploy.sh` 完成构建、迁移、重启和健康检查。

## 后续行动

1. PR/CI 通过后合并正式分支。
2. 增加 PostgreSQL 集成 CI，覆盖 provider execution 和账单释放。
3. 网页与小程序统一参考图能力校验，并记录请求体大小与上传耗时。
4. 轮换本次应急操作中暴露过的生产 SSH 凭据。
