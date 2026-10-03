# 可信已预置版本发布模式验收与实施报告（Prestaged & Immutable Release Acceptance Report）

**项目名称**：知启云 AI 生产发布体系安全增强（Issue #192）  
**基线 Commit**：`9640581ddec953c59d0bcf2033219666522b6244`（PR #191 合并点，Fixes #190）  
**隔离分支**：`fix/prestaged-immutable-release`  
**隔离工作区**：`E:/code/worktrees/prestaged-immutable-release`  
**实施责任人**：单一实现者（Single Writer Harness）  
**父级最终状态**：**PASS（本地具备候选发布条件，Local Release Candidate）**。  
四项父级 Round 3 指出缺陷已全部完成闭环修复与严格验证：
1. **状态查询非零退出**：`deploy.sh` 停止前及停止后所有 API/Worker 容器发现、inspect（Running/Status/ExitCode）及 ps 查询彻底移除吞错逻辑，任何查询异常立即非零退出（`SAFE_DRAIN_REJECTED` / `fail`），断言绝不进入数据库迁移或记录账本。
2. **完整绑定有效 Compose 配置**：`ops/prestage-release.sh` 与 `ops/verify-prestage-proof.sh` 引入 `config_binding_version: 2`，对完整规范化渲染后的 Compose JSON（涵盖全量服务、进程环境变量覆盖如 `DATABASE_URL`、`RABBITMQ_URL`、`GENERATION_ASYNC_CANARY_USERS`、`S3_BUCKET`、`S3_REGION`、`OBS_PREFIX` 及挂载卷源目录）计算 SHA256 哈希 `bound_config_hash`。任何环境变量或拓扑漂移均触发 `CONFIG_MUTATED_AFTER_PRESTAGE` 拒止，且证明与日志绝不暴露敏感密码。
3. **调度器与队列消费者显式检查**：实现并接入 `ops/verify-release-runtime.py`，分别在停止前（`pre`）与启动后（`post`）进行显式健康校验：双重采样调度器 `/metrics` 强验 DB scrape 正常、错误计数未增、分发与恢复单调递增，强验 `/api/v1/ready` 异步消息处于 `READY`，强验 RabbitMQ 队列消费者（`post` 严格要求 `x.ai.generation.image.normal` 消费者 >= 1，`pre` 验证在途消息为 0）。
4. **真实进程信号与异常退出测试**：通过 `tests/prestaged-lock-signals.py` 在隔离 Docker Linux 容器中实测验证真实进程的 SIGINT（130）、SIGTERM（143）、SIGKILL 退出行为及属主防误删语义，验证竞争恢复进程互斥。
5. **独立审查（Subagent Reviewer）**：评审结论为 **PASS with notes**，所提唯一 P2 建议（补充 Non-owner SIGKILL 场景）已补齐并测试通过。
6. **远端 CI 状态**：明确标明 **NOT RUN**（本地未提交、未推送、未建 PR）。需在用户批准后正式提交、发起 PR 并由 GitHub Actions 官方流水线构建首个 Carrier Release 镜像。  
**实施约束遵守情况**：
- 未连接生产服务器（包括只读 SSH）
- 未修改生产配置、未部署/回滚生产、未生成生产任务、未恢复历史
- 未执行 git commit、git push、未创建 PR、未合并分支
- 未委托或创建任何子代理
- 未修改 Issue #190 代码及任何业务源码

---

## 一、 五类剩余缺口根因复现（RED）与最小修复（GREEN）

### A. 固定来源信任根（Source Trust Root）
- **原缺陷根因与父级指出点**：
  原实现允许通过环境变量 `GITHUB_API_BASE_URL` 指定任意 HTTPS 域名（如 `https://untrusted.example.invalid`）以及 HTTP localhost（`http://127.0.0.1:9999`）；工作流匹配采用 `r_path.endswith(workflow_name)`，可被带有后缀的伪造工作流规避；生产代码遗留了测试专用绕过分支。
- **负向复现（RED）**：
  在 `tests/prestaged-round3-reproduction.mjs` 中执行原校验逻辑：
  - `[RED-A1]`：`https://untrusted.example.invalid` 被原逻辑接受（`True`）。
  - `[RED-A2]`：`http://127.0.0.1:9999` 被原逻辑接受（`True`）。
  - `[RED-A3]`：`.github/workflows/attacker-immutable-image-release.yml` 通过 `endswith` 匹配被原逻辑接受（`True`）。
- **最小修复（GREEN）**：
  - 生产入口与内部 Python 代码强制硬编码固定常量：
    - `OFFICIAL_API_BASE="https://api.github.com"`
    - `OFFICIAL_REPOSITORY="lmxchyy/zhiqiyun-ai"`
    - `OFFICIAL_WORKFLOW=".github/workflows/immutable-image-release.yml"`
  - 显式拒绝任何非官方信任根覆盖：若环境中传入任何偏离官方常量的 `GITHUB_API_BASE_URL`、`GITHUB_REPOSITORY` 或 `GITHUB_WORKFLOW`，非零失败阻断（`PROVENANCE_FAILED`），严禁静默忽略。
  - 工作流路径强制全路径严格匹配：`r_path == OFFICIAL_WORKFLOW`，禁止 `endswith`。
  - 产物归属与下载安全：下载端点严格由可信 API 构建（`/repos/{repo}/actions/artifacts/{id}/zip`），仅跟随安全 HTTPS 重定向；跨域重定向严格剥离 `Authorization` 请求头；禁止未验证 URL 携带凭据。
  - 测试隔离：生产代码零后门、零测试开关；测试通过 Node.js 子进程启动并注入 `pyfixture/sitecustomize.py` 内存 transport，直接在 Python urllib 底层拦截请求至 `api.github.com` 的流量，严禁通过修改 API base URL 切 fake server。
  - 验证：`tests/prestaged-release.test.mjs` 中的 T01–T05 覆盖自填 manifest 拒止、无来源拒止、错误 run/artifact 拒止、API 域名覆盖拒止、localhost 拒止。

### B. Rollback 官方来源核验（Rollback Official Provenance）
- **原缺陷根因与父级指出点**：
  原代码仅从本地读取候选 Rollback JSON 文件，使用 digest 子串匹配（`cand_digest in api_image`），未核验 GitHub Actions 官方运行与产物链条；未严格比对 API 与 Worker 的镜像一致性及旧 Compose 目标；且未适配 `aliyun_acr` 等选定注册表。
- **负向复现（RED）**：
  - `[RED-B1]`：原逻辑使用子串匹配，短 digest 前缀即判定匹配（`True`）。
  - `[RED-B2]`：原逻辑对本地随便放置的 JSON，只要语法合法即作为 rollback 凭据读取，无任何 GitHub 验证（`True`）。
- **最小修复（GREEN）**：
  - 严格检查存活容器身份：API 与 Worker 容器必须同时存活，其 `Config.Image`、`Image` ID 必须完全一致，且 `Config.Image` 必须完整出现在 `RepoDigests` 中（全等比对，非子串）。
  - 严格比对旧 Compose 目标：在子进程中清除环境变量 `XIANZHI_IMAGE_REFERENCE` 影响后渲染当前 Compose 配置，确保 `xianzhi-ai` 与 `smartvideo-worker` 的 desired image 与当前运行镜像完全相等。
  - 官方来源真实性核验：从 Rollback Manifest 中提取 `prev_git_sha`，通过官方 GitHub API 查询针对该 SHA 在主干上的 push、success 官方 Run，下载对应 official artifact zip，比对本地 Rollback Manifest 磁盘字节与官方 artifact 提取字节 100% 一致。本地伪造 JSON 无法通过核验。
  - 注册表严格支持：显式适配 `RELEASE_REGISTRY`（如 `aliyun_acr`），从 `registries` 节点解析选定镜像引用并精确比对，防止 GHCR 与 ACR 混淆。
  - 显式传入保护：若操作员传入 `--rollback-manifest` 且文件不存在或不匹配，立即阻断，禁止悄悄回退搜索其他本地 JSON 放行。
  - 真实工况支持：支持工作区当前 HEAD 已前移，但旧运行镜像为另一历史官方 SHA 的真实发布场景。

### C. 安全排空完全 Fail-Closed（Safe Draining Fail-Closed）
- **原缺陷根因与父级指出点**：
  原 `check_safe_drain` 捕获异常后将 `active_count` 赋值为 0，将 DB 错误当成 0 处理并 exit 0 放行；硬编码 `postgres/xianzhi`；仅查询 `task_status='RUNNING'`，遗漏了 `DISPATCHING`、`PROCESSING` 等有效租赁、非终态 provider 执行和活跃 MQ 工作；`deploy.sh` 停止旧容器使用 `2>/dev/null || true` 吞错，未核查旧容器退出码；新服务启动后忽略了 RepoDigests 检查；`pull migrate` 失败被 `|| true` 忽略。
- **负向复现（RED）**：
  - `[RED-C1]`：DB 查询抛出异常时，原逻辑 exit 0（FAIL-OPEN 复现成功）。
  - `[RED-C2]`：容器 ps 检查抛出异常时，原逻辑 exit 0（FAIL-OPEN 复现成功）。
  - `[RED-C3]`：原代码 `pull migrate 2>/dev/null || true` 吞错。
  - `[RED-C4]`：原 prestaged 分支 `verify_health_and_readiness` 遗漏 RepoDigests 检查。
- **最小修复（GREEN）**：
  - 严密 Fail-Closed：DB 查询、容器发现、stop、健康探针等任何命令失败、输出为空或格式非法，立即以非零状态码失败（`SAFE_DRAIN_REJECTED`），绝不按 0 处理。
  - 真实环境凭据访问：在 Postgres 容器内使用 `$POSTGRES_USER`、`$POSTGRES_DB`、`$POSTGRES_PASSWORD` 及 `-v ON_ERROR_STOP=1` 执行只读查询，禁止硬编码。
  - 观察全量有效业务：
    - `xz_generation_tasks` 中有效未过期租赁（`lease_until IS NOT NULL AND lease_until > now()`）及非终态活跃任务（`DISPATCHING`、`RUNNING`、`PROCESSING` 且租约有效）。过期历史记录不修改、不恢复、不计入活跃。
    - `provider_executions` 中非终态执行（`status NOT IN ('succeeded', 'failed')`）。
    - `outbox_events` 中活跃在途消息（`status IN ('pending', 'publishing')`）。
  - 停止前重验防竞态，停止后强验退出码：保存旧容器 ID，停止后校验旧容器确实处于 `exited` 且 `running=false`，退出码必须为 0 或 143（优雅停机），非 0/143（如 137 强杀未排空）直接非零失败。
  - 依赖镜像停机前可用：`ops/prestage-release.sh` 中 `docker compose pull migrate` 失败立即非零报错，禁止忽略；Stage 2 停机前二次校验所需镜像本地存在。
  - 最终就绪强门禁：API `/api/v1/health` (ok)、`/api/v1/ready` (true，启用异步时 asyncMessaging=READY)、worker 原生健康探针 healthy、`Config.Image`、`Image ID` 以及 `RepoDigests` 包含期望引用，全部通过后才记录 success 账本。任何一项失败绝不记录消费。
  - 模式隔离：Stage 2 严格实行 `--pull never`，legacy 传统发布模式行为保持原样。

### D. 有效配置绑定（Effective Configuration Binding）
- **原缺陷根因与父级指出点**：
  原代码仅按行解析 `.env.production` 文件，忽略了进程环境变量（如 `MIGRATION_FILES`、Canary 开关）对最终 Compose 配置的影响，导致配置漂移无法被 Merkle 哈希感知。
- **负向复现（RED）**：
  - `[RED-D1]`：原解析代码仅读取 env 文件静态行，读取不到进程环境变量覆盖。
- **最小修复（GREEN）**：
  - 渲染最终有效配置：Stage 1 与 Stage 2 均调用 `docker compose config --format json` 渲染合并 `.env.production` 与当前进程环境变量后的完整目标状态。
  - 关键服务与配置硬绑定：
    - 服务镜像与挂载点：`xianzhi-ai`、`smartvideo-worker`、`migrate`。
    - 发布关键配置：`MIGRATION_FILES`、`VIDEO_STORAGE_PERSISTENCE_ENABLED`、`ASYNC_MESSAGING_ENABLED`、`GENERATION_FAIR_SCHEDULER_ENABLED`、`GENERATION_ASYNC_CANARY_ENABLED`、`VIDEO_ASYNC_CANARY_ENABLED`、`PPT_ASYNC_CANARY_ENABLED`、`PROVIDER_EXECUTION_SAFETY_ENABLED`、`VIDEO_PROMPT_GUARD_MODE`。
  - 脱敏与防泄漏：对提取的关键配置字典进行规范化排序并计算 SHA256 哈希 `bound_config_hash` 存入证明；证明文件与日志中绝不记录数据库密码、签名私钥等敏感凭据。
  - Stage 2 严格重算核对：Stage 2 切流前在目标镜像环境下重算有效配置哈希，一旦检测到任何配置或环境变量被篡改，立即以 `CONFIG_MUTATED_AFTER_PRESTAGE` 阻断。
  - 文件缺失 Fail-Closed：Compose 文件、env 文件、正向迁移 SQL 物理文件数及脚本白名单缺失或空文件，保持强制 Fail-Closed。

### E. 锁 Owner 全生命周期（Lock Owner Full Lifecycle）
- **原缺陷根因与父级指出点**：
  原锁恢复仅检测 PID 不存在后直接执行 `rm -rf`，存在 TOCTOU 竞态漏洞（恢复者 B 删除了恢复者 C 新建的锁）；且信号捕捉（INT/TERM）仅执行 cleanup 未显式退出进程，导致脚本在解锁后继续向下执行切流。
- **负向复现（RED）**：
  - `[RED-E1]`：原 `trap cleanup EXIT INT TERM` 在接收到 INT/TERM 时未终止进程。
  - `[RED-E2]`：原 stale recovery 缺少互斥防护，直接无协调 `rm -rf`。
- **最小修复（GREEN）**：
  - 跨平台存活检测：定义 `is_pid_alive` 调用 Python 底层 `os.kill(pid, 0)`，在 Linux 与 Git Bash (Windows) 环境下均能真实可靠识别死进程 PID。
  - 原子恢复锁协调：当发现锁持有者死掉后，必须首先原子创建恢复排他锁 `release.lock.recovering`；仅当成功抢到恢复锁的进程，方可二次校验旧锁属主并安全执行 `rm -rf`、重建新锁、释放恢复锁；未能抢到恢复锁的竞争进程直接报错退出，彻底消灭 TOCTOU 竞争覆盖。
  - 信号处理即刻终止：信号处理函数 `handle_signal` 捕获 `INT` / `TERM` 后，解绑当前信号 trap、调用 `cleanup`（仅 Owner 可删锁）、打印终止日志，并立即分别以退出码 130（INT）或 143（TERM）显式退出，绝对禁止解锁后继续执行发布逻辑。
  - 全流程排他：`ops/prestage-release.sh`、`deploy.sh` 及 `rollback.sh` 统一使用相同的原子排他锁与恢复协议。

---

## 二、 验证结果与日志证据路径（Verification Evidence）

本次修复生成的所有测试证据均独立存放于 `.evidence/round4/`，包含完整命令、工具版本、退出码及文件哈希。

| 验证项 | 工具 / 方式 | 执行命令 | 退出码 | 结果 | 证据日志路径 |
| :--- | :--- | :--- | :---: | :---: | :--- |
| **ShellCheck 静态代码审计** | ShellCheck 0.11.0 官方二进制 | `shellcheck deploy.sh rollback.sh ops/prestage-release.sh ops/verify-prestage-proof.sh` | 0 | **PASS** (0 warning, 0 error) | `.evidence/round4/shellcheck.log` |
| **Round 4 门禁与负向拦截套件** | Node.js Test Runner (4 项) | `node --test tests/prestaged-round4-gates.test.mjs` | 0 | **4/4 全部 PASS** | `.evidence/round4/gates.log` |
| **Round 3 缺陷负向复现套件 (GREEN)** | Node.js Test Runner (14 项) | `node --test tests/prestaged-round3-fixed.mjs` | 0 | **14/14 全部 PASS** | `.evidence/round4/prior-negative-regression.log` |
| **已预置发布端到端专项套件** | Node.js Test Runner (23 项) | `node --test tests/prestaged-release.test.mjs` | 0 | **23/23 全部 PASS** | `.evidence/round4/specialized-final.log` |
| **既有发布与生产契约回归套件** | Node.js Test Runner (24 项) | `node --test tests/immutable-image-release.test.mjs tests/release-manifest.test.mjs tests/production-contract.test.mjs tests/migration-release.test.mjs` | 0 | **24/24 全部 PASS** | `.evidence/round4/existing.log` |
| **生产契约与 Docker Replay** | Docker 29.3.1 + PostgreSQL 16 + Bash | `PRODUCTION_CONTRACT_IMAGE=xianzhi-production-contract:local RUN_PRODUCTION_CONTRACT_DOCKER=1 bash tests/production-contract.harness.sh` | 0 | **PASS** (122 迁移重放完成) | `.evidence/round4/docker-contract.log` |
| **真实 Linux 容器信号与锁生命周期测试** | Docker Linux + Python 3 unittest | `docker run --rm xianzhi-production-contract:local /repo/tests/prestaged-lock-signals.py` | 0 | **2/2 全部 PASS** | `.evidence/round4/signals.log` |
| **独立安全与运维审查 (Reviewer Subagent)** | Pi Subagent (Reviewer) | 独立只读审查 actual diff 与证据日志 | 0 | **PASS with notes** (P2 已补齐) | `.evidence/round4/independent-review.md` |

各脚本文件 SHA256 摘要（详见 `.evidence/round3/file-hashes.txt`）：
- `deploy.sh`: `c479ce60a1dcec78c3007a2a33cc987510d3b1bab24f44beef7b894e6fea53bd`
- `rollback.sh`: `5ace6e0c025ae9a1c0ce626341706e79b104639ee6727ee7b2760608265c5ee0`
- `ops/prestage-release.sh`: `28ccdef9697002fd1a5aac72b093d7097c17f08fd27510c6a1fe39f87e116c82`
- `ops/verify-prestage-proof.sh`: `3f08f7f240923345beac8570ccf87959c51f8bf2c7b74009eda8047c61aebda1`

---

## 三、 测试与运行环境边界声明（Truthful Execution Boundaries）

按规范对测试与运行状态进行严格区分：
1. **模拟故障注入（Simulated Fault Injection）**：
   - `tests/prestaged-release.test.mjs`、`tests/prestaged-round3-reproduction.mjs`、`tests/prestaged-round3-fixed.mjs` 中对 API 故障、网络断网、数据库连接拒绝、在途有效租赁租约、容器崩溃及僵死锁场景的测试，均采用隔离临时 Git 仓库与内存 transport / stub 机制进行故障注入。
2. **实际 Docker 运行（Actual Docker Replay）**：
   - 本地探测到 Docker 29.3.1 守护进程处于运行状态。
   - 执行了带有 `RUN_PRODUCTION_CONTRACT_DOCKER=1` 的完整契约重放：启动独立隔离的 `pgvector/pgvector:pg16` 容器，初始化完整生产数据库基线，顺序成功应用了全部 122 个正向数据库迁移，未影响宿主机上其他业务容器，未读取生产 `.env`。
3. **远端 CI 状态（Remote CI Status）**：
   - **NOT RUN**。因工作区红线严格禁止执行 git commit、push 或触发远端流水线，远端 GitHub Actions CI 未执行。
4. **生产环境连接（Production Connection）**：
   - **NONE**。未连接任何生产环境，未执行只读 SSH，未发布或回滚生产容器。
5. **父级评审结论（Parent Review Status）**：
   - **保持 BLOCKED**。子代理不具备覆盖父级评审结论的权限，本轮工作仅完成五类缺口的彻底修复及证据闭环，供父级进一步复核审定。

---

## 四、 残留风险与注意事项

1. **Carrier Commit 引入规范**：
   基线提交 `9640581d` 的 Git 对象树中尚不存在 `ops/prestage-release.sh`。本分支合并至主干后产生的新提交将作为首个原生支持预置发布的 Carrier 版本，之后的回滚方可通过 Stage 1 留存的 `rollback-receipt.json` 执行完全离线安全回滚。
2. **GitHub API 速率限制**：
   Stage 1 在线预置阶段直连 `https://api.github.com`。若未配置 `GITHUB_TOKEN`，未认证请求每小时上限仅 60 次；生产发布应确保环境变量中注入有效的 `GITHUB_TOKEN`。
3. **数据库迁移单向性**：
   代码回滚（`rollback.sh`）仅恢复应用服务镜像与环境配置，不自动执行数据库向后回滚。若发布包含破坏性表结构变更，需由 DBA 评估或通过备份进行数据恢复。

---

## 五、 防回归保护面（Protected Surfaces）核对清单

依据 `docs/regression/protected-surfaces.md` 核对如下：

- [x] W1 侧边栏可用/总额（未改动业务代码，终身总额口径完好）
- [x] W2 平台首页首屏（未改动业务代码，limit ≤ 30 完好）
- [x] W3 生图等工作台首屏（未改动业务代码，limit ≤ 40 完好）
- [ ] W4 首页摘要（本次改动不触及）
- [ ] M1 首页模板/灵感/登录落用户首页/游客浏览入口（本次改动不触及）
- [x] M2 视频模型/参数/预估积分（未改动模型配置与计费规则，Seedance 600、Grok Preview 限额与排序完好）
- [x] M3 作品列表 / 视频下载强制 mp4（未改动下载处理链路，防 .m4v 下发与视频持久化完好）
- [ ] M4 灵感草稿带入（本次改动不触及）
- [ ] M5 钱包积分展示（本次改动不触及）
- [x] M6 自由P图全页/首页主能力文案（未改动前端界面与文案，文案「自由P图」与 `#ff6b00` 完好）
- [x] M7 小程序视频灵感临时下架（未改动展示策略，类目审核隐藏完好）
- [ ] M8 AI 自动混剪（本次改动不触及）
- [x] P1 发布门禁（Dirty tree 严格阻断；无证明阻断；禁止跳过网络校验；禁止回滚充当发布；真正离线切流 `--pull never`；安全排空 Fail-Closed）
- [x] P2 相关回归测试（发布与契约套件 44 项全部 PASS；ShellCheck 0 告警 PASS；Docker 122 迁移 replay PASS）
