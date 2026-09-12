# 先知 AI 生产环境最新进展与审计报告（09-05 上午版）

**更新时间**：2026-09-05 08:30 (CST)  
**环境**：生产环境（Production）  

---

## 核心进展速览

### 1. 视频异步（Video Async）：全用户生产上线完成 ✅
* **配置已生效**：
  * `VIDEO_ASYNC_CANARY_ENABLED=true`
  * `VIDEO_ASYNC_CANARY_USERS=*`
  * 供应商与模型白名单保持严格：`configured` / `grok-imagine-1.5-video`
* **真实普通用户端到端验收通过**：
  * 账号：`user_000002`（`demo@xianzhi.ai`，普通会员）；
  * 任务：`task_000238`（`TEXT_TO_VIDEO`，6s，480p）；
  * 结果：**`SUCCEEDED`**（耗时约 70s，进度 100%）；
  * 执行凭据：单次提交（`provider_executions attempt=1`），无盲目重试，Inbox 成功幂等去重；
  * 计费与资产：预扣 90 积分并单次 Capture，无重复结算（余额正常扣减至 55,309）；资产 `asset_000191`（4.26MB MP4）经 HTTP 200 验证可正常播放；
  * 运行时健康：最终视频 queue、retry、dlq、stuck、unsettled 全部清零，consumer=1。

### 2. PR #98 部署情况：代码已入库生效 ✅
* 生产代码已对齐最新 main 提交 `0e6f883df`；
* 运行镜像对齐 Immutable Digest：`sha256:8d25295827a06c6c472e2570217ba600b6080153447cfa982e590456c15ca95b`；
* 脚本已具备结构化错误识别逻辑。

### 3. 生产异地备份（Offsite Backup）只读最终验证：发现关键细节 ⏳
* **两次自然调度实测**：定时器分别在 `02:23:42 CST` 和 `08:23:44 CST` 准时自动触发（未进行任何手工干预）；
* **错误识别验证通过**：首个历史无元数据备份 `db_2026-08-21_195734.sql` 成功被标记为 `INVALID=1`，未报未知失败；
* **发现新阻塞点**：
  * **现象**：脚本在处理完首个 candidate 后未继续扫描后续的正常备份，整个批次直接提前退出；
  * **根因定位**：在 Shell 的 `while read` 循环中，子进程 `docker compose run` 默认继承了父进程的 stdin，将用于传递待处理文件名的管道缓冲区数据一次性吸空，导致第二次迭代直接读到了 EOF；另外 `found=1` 未做数字累加。
* **判定**：异地备份自动化暂不能标记 `VERIFIED`，待对输入流进行隔绝修复。

### 4. 历史受保护对象复核 ✅
* `task_000232`：保持 `IMAGE_TO_IMAGE | QUEUED | RESERVED`（**UNTOUCHED**）；
* `task_000234`：保持 `TEXT_TO_VIDEO | FAILED | RELEASED`（**CLOSED / UNRESTORED**）。

---

## 下一步工作建议

1. 对 `ops/backup-offsite-upload-pending.sh` 进行轻量修复：
   * 在 `docker compose run` 命令中增加 `-T` 并显式重定向输入 `< /dev/null`（或循环改用独占文件描述符 `read -u 9`），彻底隔绝 stdin；
   * 修正 `found=$((found + 1))` 计数逻辑；
2. 在 CI 测试中补充 stdin 消耗隔离测试，提 PR 合并；
3. 发布后再次观察下一个自然周期，完成异地备份自动化的最终封板；
4. 进入下一阶段：PPT / Agent 耗时任务接入统一异步任务 Runtime。
