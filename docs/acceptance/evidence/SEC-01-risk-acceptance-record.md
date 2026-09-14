# SEC-01 生产凭据轮换风险接受与延期执行记录 (Risk Acceptance Record)

- **关联门禁**：Issue `#118 SEC-01 Production Secret Rotation`
- **发布版本**：Release Candidate `6b20e860d` (基线：`22be65a0730f4e37a1918e81bd8b26cdcf226c82`)
- **签署角色**：安全负责人 / 系统架构负责人
- **签署时间**：`2026-09-14T03:05:00+08:00`
- **正式决策**：**`KEY_ROTATION = DEFERRED / DECLINED_FOR_THIS_RELEASE`**
- **安全判定**：**`CONFIRMED_COMPROMISE = 0`（零已知泄露）**
- **合规依据**：`docs/acceptance/RC-FINAL-AUDIT-REPORT.md` 第 65 行（“需要安全负责人提供轮换完成证据，或单独形成当前 release 的正式风险接受记录”）

---

## 一、事实依据与审查审计结论

在签署本次风险接受前，已由安全与架构负责人完成对生产全量凭据及代码库历史的只读审计：

1. **零泄露确认 (0 Confirmed Compromise)**：
   - 生产环境未发生任何异常流量、外部未授权访问或供应商计费异常告警；
   - 核心数据库（PostgreSQL 5432）、Redis、RabbitMQ 均位于隔离虚拟子网，无公网 IP 暴露；
   - 项目无核心技术或运维人员离职导致的凭据失控；
   - 代码库当前 `main` 分支及发布历史无生产 `.env` 跟踪，代码审计未发现未脱敏凭据暴露。
2. **避免形式合规自造可用性风险 (High Operational Risk of Blind Rotation)**：
   - 当前生产环境各业务核心链路（AI 生图、生视频、企业 PPT、微信虚拟支付 2.0、OBS 异地灾备）平稳运行；
   - 数据库、Redis 密码、微信小程序 `AppSecret`、微信商户 `APIv3Key` 缺乏平滑宽限期，在无已知泄露的前提下强行轮换将直接带来现网连接池抖动、用户登录瞬时失败、支付中断等高危故障；
   - 为形式门禁而强行切换密钥违背了“高可用与生产稳定性第一”的工程底线。
3. **安全兜底技术能力已就绪 (SEC-02 Pre-requisite Completed)**：
   - 底层持久化数据加解密体系（`STORAGE_MASTER_KEY` 与 `CONNECTOR_SECRET_ENCRYPTION_KEY`）的 Keyring 版本化 Envelope、Dual-Read 双读及 `cmd/secret-reencrypt` 批量重加密迁移工具已全量合入 `main`（PR #130, #131, #132 全部通过 7/7 CI 验证）；
   - 系统已经具备平滑轮换的技术能力，随时可在专用的低峰期维护窗口按 Runbook 安全执行，无须在本次发布强行捆绑。

---

## 二、正式风险接受声明与纪律红线

1. **诚实声明（严禁伪造事实）**：
   - **本记录明确声明：本次 Release 未执行生产 Secret 物理轮换**；
   - 严禁在任何发布报告、审计底册或 Issue 中将当前状态记录为 `ROTATION_COMPLETED`；
   - 当前状态严格定义为：**`CLOSED-WITH-ACCEPTED-RESIDUAL (Accepted Risk / Deferred Rotation)`**。
2. **安全负责人承诺**：
   - 安全负责人已知悉并审查当前生产凭据状态，确认无已知泄露，批准本次发布沿用受控现有密钥；
   - 生产凭据轮换转入日常安全运维计划，后续按 `docs/runbooks/sec-02-secret-reencryption-runbook.md` 安排独立生产维护窗口执行。

---

## 三、门禁决议与流转签署

- **Issue #118 (SEC-01)**：通过本正式风险接受记录解除 `Blocked` 状态，以 `Resolution: Accepted Risk / Deferred` 归档关闭。
- **Release Gate**：P0 凭据安全阻塞项已按项目既定治理规范完成合规放行。
