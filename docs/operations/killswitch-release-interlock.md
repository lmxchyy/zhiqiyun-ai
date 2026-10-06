# Killswitch 发布互斥（Issue #194）

## 范围

只修 `ops/auto_monitor_killswitch.py` 与现有发布锁的协作。不改 #190 的业务代码、不改 provider execution 状态、不引入 legacy-orphan / quarantine / drain 豁免，也不清理 DLQ。

生产入口目前是 root cron 每分钟调用 `/opt/zhiqiyun-ai/bin/auto_monitor_killswitch.py`；`bin/` 被 Git 忽略，现场安装版本与仓库 `ops/` 文件不同。仅修改仓库文件 **不等于** 生产入口已经更新。

## 锁与副作用

- 默认共享 `/opt/zhiqiyun-ai/.prestage/release.lock`，与 deploy / rollback / prestage 相同的原子 mkdir、`owner_token`、`owner_pid` 协议。
- 若发布使用自定义 `PRESTAGE_DIR`，monitor 必须配置为同一个目录；不得为两者配置不同锁域。相对目录从项目根解析，不从 cron 的 cwd 解析。
- 已有 release/recovery 锁（包括空锁、无效 PID、dead owner、悬空符号链接）：立即 defer，不抓取 metrics、不写 env、不调用 Compose。monitor 不负责 stale recovery。
- metrics 抓取后、任何 env/container 操作前，再原子竞争共享锁；持有到同步 Compose 返回。并发 monitor、deploy、rollback、prestage 不能同时执行变更。
- cleanup 只移除自己的 token 对应锁；不删除别人的锁，不递归清未知目录。
- 图片 DLQ（`xianzhi_async_canary_rabbitmq_dlq_depth > 0`）仅关闭 `GENERATION_ASYNC_CANARY_ENABLED`，保留视频/PPT 开关、视频用户/provider/model allowlist 和其它 env 字节，消除仓库版本比现场副本额外关闭视频 canary 的差异。
- 视频/PPT 专属及其它告警仍使用既有默认 disable 行为；不在此补丁重构其开关范围、顺序、阈值或首次触发后退出的逻辑。不能把“图片 DLQ 不关闭视频”理解成“任何告警都不得关闭视频”。
- env 改动仍使用同目录原子替换并保留权限。不输出 env 内容。
- 本次选中的开关已经 false 时不重复写文件，但仍在锁内执行 API-only reconcile：env 为 false 不能证明运行容器已读取 false。

## Stage 0A：observe-only 验收模式（Issue #205）

- 仅显式传入 `--observe-only` 才进入 observe-only；cron 不带该参数时仍为正常模式，不存在环境变量、配置文件或自动切换入口。
- observe-only 只读 release/recovery lock、DLQ metrics 和 `.env.production` 中三个 canary 开关；输出结构化拟执行动作，不调用 Docker/Compose、不写 env、不启动 migrate。
- lock 在 scrape 前存在时输出 `SUPPRESSED_BY_RELEASE_LOCK` 且跳过 metrics；scrape 期间出现 lock 时再次输出该标记并抑制拟执行结论。
- 无 lock 且 metrics 报警时只报告正常模式本次会执行的首个动作、后续待处理告警、当前 canary 值及拟关闭开关；`executed=false`、`migration=false`。正常模式每次触发后退出，因此不把后续告警误报成同一 cron 调用会执行的动作。
- 验收必须有明确起止时间与操作人； observe-only 不是长期运行配置。退出 observe-only 只允许经单独审批的显式 cron 配置变更，脚本不会自行恢复正常模式。
- 恢复 normal mode 的先决条件：对应新 Carrier 已安装到 host 控制面；至少一个真实 cron 周期在正式 release lock 下输出 `SUPPRESSED_BY_RELEASE_LOCK` 且 env/API/migrate/canary 状态无变化；当前 DLQ 处置策略已另行确认。此脚本不批准该策略，也不自动恢复 normal mode。
- 唯一允许的 Compose 操作为 `up -d --no-deps --no-build --pull never xianzhi-ai`；不拉取、不构建、不启动 migrate 或其它依赖。

## 中断与失败

INT/TERM 记录退出意图，不在 Compose 子进程仍运行时提前解锁；同步返回后退出 130/143。不能把“monitor PID 已死”当成“Compose 子进程或 Docker daemon 副作用已结束”。

调用 Compose 前写 `owner_pid=indeterminate-compose`；成功返回后才恢复数值 PID 并正常解锁。SIGKILL 或 Compose 调用失败会保留不确定锁；现有三种发布入口对非数值 PID 都 fail closed，禁止自动 stale recovery。值不是 provider execution 标记，与 9 条历史记录无关。

这种残留锁需要另行授权的只读核查（monitor/Compose 子进程、Docker lifecycle、env 与运行身份/健康），证明现场静止后才按受控运维方案处理。**禁止直接 rm 锁恢复发布。** 不确定状态可能阻塞发布是刻意的安全取舍，不是假成功。

## 受控上线边界（尚未执行）

1. 经 PR/CI 合并到 main，核验正式 Git SHA；production clean ff-only 同步，不手工补源码或临时注释 cron。
2. 安装是**宿主机控制面更新**，不是应用镜像切流。必须使用已审核 SHA 的 `ops/auto_monitor_killswitch.py`，验证字节与 Git 一致后原子更新现有 `bin/` 入口（或受控改成指向 tracked `ops/` 的稳定入口）。现有 cron 的每分钟调度可保持；不能只更新 `ops/` 却继续调用旧副本。
3. 首次转换旧脚本时，仅靠 release lock 不够：旧脚本不认锁。安装必须独占正式发布锁，原子更换入口后，核清/等待所有已加载旧代码的 monitor 及其 Compose 子进程完成，再核对 runtime/env。旧进程未排净或来源不明就维持 BLOCKED，不能宣称已完成互斥上线。新入口遇安装锁应无副作用退出。
4. 独立确认 cron 确实执行新字节，并验证有锁时 env 字节/mtime、API ID/StartedAt、migrate StartedAt 均不变。不得主动重建 API、运行 migration、调用 provider、修改任务/钱包来“验证”。
5. 同步新的宿主机 SHA 后，旧 Carrier proof 的 source/SHA 绑定不能自动继承；不要编辑 proof。最终应用发布仍需按新 Carrier 官方产物和现场门禁重新准备，且 9 条 unresolved executions 在后续独立策略完成前继续阻断应用切流。

本次兼容补丁只有隔离开发/测试，不授权生产安装或切流；Review PASS 且远端 CI 全绿后也只能重新申请生产安装。没有执行以上生产步骤，没有重新 prestage 或刷新备份。

## Stage 0B：离线审批与完整快照（Issue #205）

- `xz_assets` 是当前 schema 中的 artwork/result projection；`xz_file_objects`、storage configs、relations/jobs/multipart 表组成 DB storage family。Canonical live snapshot 将 task/core、provider attempts/correlations、完整个人账本投影与上述 DB family 一起绑定。
- “完整”只指已声明并哈希的数据库证据；远端 object 是否仍可读、历史结果是否可归属于特定 provider attempt 如仍标记 `UNPROVEN`，必须显式呈现给人工审批人，不得描述成已验证。
- Agent 只能输出 unsigned candidate 与 snapshot evidence；签名是离线人类审批动作。发布机只按固定 `authority_id=prod-quarantine-approval-v1`、Carrier-pinned 公钥指纹、release SHA、记录集合、快照/evidence hash 及有效期验签。
- 私钥不进入仓库、发布机、Pi 或 deploy tooling。公钥 pin 通过 Git review/CI/Carrier/Proof 变更；正式人类审批责任方与公钥未 provision 时，验签必须 fail closed。

## 测试

```bash
python3 tests/killswitch-image-dlq-scope.py
python3 tests/killswitch-release-interlock.py
python3 tests/prestaged-lock-signals.py
shellcheck deploy.sh rollback.sh ops/prestage-release.sh ops/verify-prestage-proof.sh
```

新测试在 Linux 隔离临时目录运行生产 monitor 及三个发布入口的真实锁函数：真实进程竞争、metrics/锁 TOCTOU、owner token、INT/TERM、SIGKILL、Compose 非零退出、自定义锁域、无效锁。metrics transport 和 Docker executable 使用夹具；不会访问生产、数据库、上游或真实应用。CI production-contract 明确执行新测试，不拿静态检查替代并发证明。

## 保护面

- [x] P1：未绕过发布/dirty/source/proof 门禁，未生产热修；正式部署/运行 SHA 尚未验收。
- [x] P2：本次相关锁回归及新增隔离测试执行；业务、migration、drain 代码未改。
- [ ] W1–W3 / M1–M8：本次未改页面或业务代码，未进行生产 UI/生成验收。
- [ ] 生产互斥安装验收：待独立授权与已合并正式提交，不能以本地测试冒称上线。
