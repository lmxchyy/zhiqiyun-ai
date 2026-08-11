# 知启云AI渠道生态中心 V1.3.2 第7批生产变更审批手册

## 1. 本批边界

本批只完成生产上线准备、非生产副本迁移演练、只读配置门禁、监控门禁和首笔订单操作方案。不得执行生产迁移，不得创建真实白名单，不得开启 Canary、比例放量或生产调度器，不得发起真实退款。

## 2. 优雅关闭

API 进程监听 `SIGTERM` 和 `SIGINT`，收到信号后按以下顺序执行：

1. 停止接收新请求。
2. 触发 `http.Server.RegisterOnShutdown` 注册的 Operation Center 调度组停止函数。
3. 等待当前 HTTP 请求和已开始的单轮调度任务在 `XIANZHI_SHUTDOWN_TIMEOUT` 内结束。
4. 超时后强制关闭 HTTP 连接；未完成调度任务依赖数据库租约到期后恢复。
5. 关闭数据库、Redis 等基础设施连接。

推荐生产值：`XIANZHI_SHUTDOWN_TIMEOUT=30s`。允许范围为大于零且不超过十分钟。关闭流程幂等，panic 路径仍执行资源清理；日志不得记录 Provider 凭证、签名、完整原始响应或支付敏感字段。

## 3. 非生产迁移演练

必须使用生产结构副本或脱敏备份恢复出的隔离数据库，禁止连接生产数据库。命令要求明确环境标签、备份引用、可用磁盘字节数和固定确认词：

```powershell
$env:XIANZHI_REHEARSAL_DATABASE_URL = '<non-production-copy-dsn>'
$env:XIANZHI_REHEARSAL_ENVIRONMENT = 'production-sanitized-copy'
$env:XIANZHI_REHEARSAL_ACK = 'NON_PRODUCTION_COPY'
$env:XIANZHI_REHEARSAL_BACKUP_REFERENCE = '<verified-backup-id>'
$env:XIANZHI_REHEARSAL_AVAILABLE_DISK_BYTES = '<bytes>'
go run ./cmd/operation-center-migration-rehearsal --apply --migration-dir ../database/migrations --output ../docs/分润规则/reports/operation-center-089-096-rehearsal.json
```

工具在写入前检查当前签名版本、扩展、磁盘余量、长事务、锁等待和备份引用；只按 089 至 096 顺序执行。迁移后只读核验 CHECK、外键、触发器、运行时投影、历史订单数量、商业规则及 Legacy/Shadow/Canary 配置指纹。

每次正式审批前必须用最新脱敏副本重新生成报告。仅用空库或合成数据的结果不能替代真实数据量演练。

## 4. 生产配置 Preflight

运行前只向进程注入生产只读数据库连接和以下显式配置，不得复用任何 `TEST` 配置：

```powershell
$env:XIANZHI_ENV = 'production'
$env:XIANZHI_PRODUCTION_OPERATION_CENTER_REFUND_PROVIDER_MAPPINGS = 'WECHAT_VIRTUAL=MANUAL'
$env:XIANZHI_PRODUCTION_OPERATION_CENTER_FINANCIAL_SUBMITTER_ID = '<finance-a-uuid>'
$env:XIANZHI_PRODUCTION_OPERATION_CENTER_FINANCIAL_APPROVER_ID = '<finance-b-uuid>'
go run ./cmd/operation-center-preflight
```

任一检查失败必须返回非零退出码并阻断变更审批。门禁包括：

- 所有结算配置均为 `SHADOW`，真实切换、比例放量关闭，四类白名单为空。
- 三个 Operation Center 调度器关闭，人工退款自动审批关闭。
- 存在已发布规则集、完整运营中心套餐、显式 `rbacRole` 和 `FULL_ONLY` 退款策略。
- Provider 映射不重复，微信虚拟支付明确映射为人工退款路径。
- 财务提交人和审批人为不同 ACTIVE 账号，具备财务权限且不继承运营审核或超级管理员权限。
- 生产环境未读取任何测试配置。

## 5. 监控和初始告警阈值

| 指标 | 初始告警阈值 | 级别 |
|---|---:|---|
| REVIEW_REQUIRED 数量/最老等待时间 | 大于20单或超过30分钟 | Warning |
| 审核拒绝率 | 15分钟窗口超过20%且样本不少于10 | Warning |
| 奖励冻结金额与冻结流水差额 | 非0 | Critical |
| 奖励释放金额与桶转移流水差额 | 非0 | Critical |
| REFUND_RETRYABLE 积压 | 大于10单或最老超过30分钟 | Warning |
| UNKNOWN_VERIFYING 积压 | 大于5单或超过配置化安全等待期 | Critical |
| MANUAL_REQUIRED 积压 | 大于10单或最老超过24小时 | Warning |
| recoverable_cents 总额 | 大于0告警；持续增长升级 | Warning/Critical |
| 调度租约超时数量 | 大于0 | Warning |
| Provider 临时失败率 | 5分钟超过5%且样本不少于20 | Warning |
| Provider UNKNOWN 比例 | 5分钟超过1%或单笔超过安全等待期 | Critical |
| 幂等冲突数量 | 5分钟超过5次 | Warning |
| 状态不变量错误数量 | 大于0 | Critical |

上线前只验证指标可读性，不开启生产调度。告警需按 `tenant_id`、Provider、任务状态和规则版本切片，但日志及标签不得包含支付密钥或完整 Provider 原始响应。

## 6. 首笔订单操作方案

### 6.1 普通会员或代理首单

1. 变更审批通过后，仅使用一个内部测试账号和一个明确 `order_id`。
2. 只允许 `order_id` 白名单；禁止 `tenant_id`、`package_id` 白名单和比例放量。
3. 下单前记录规则版本、套餐版本、关系快照、Legacy 钱包和新钱包基线。
4. 支付后核对固化 `settlement_engine=V132`、商业快照、唯一佣金写入源及金额守恒。
5. 重复回调一次并确认奖励、钱包、平台收入均无重复写入。
6. 关闭该单次白名单配置；已固化订单仍按 V1.3.2 历史规则处理。

### 6.2 运营中心首单

运营中心不得混入普通 Canary。使用独立测试订单和人工审核：

1. 支付后必须为 `REVIEW_REQUIRED`，身份、档案和 RBAC 零写入，推荐奖励零写入。
2. 审核通过后，在同一事务核对 ACTIVE 身份、档案、RBAC、履约、ReferralEvent、Eligibility、FROZEN Reward、钱包流水和释放任务。
3. 微信虚拟支付退款只能进入双人财务人工流程，提交人与审批人必须不同。
4. 核对自动或人工退款证据、两名操作人审计、奖励冲正、钱包守恒和撤权后不恢复 ACTIVE。

## 7. 回滚和紧急停用

优先采用应用级停用和配置回退，不做财务数据物理回滚：

1. 保持或恢复 `mode=SHADOW`、`real_switch_enabled=false`、`percentage_rollout_enabled=false`，清空全部白名单。
2. 关闭奖励释放、退款重试和 UNKNOWN 核验三个调度器。
3. 停止新增审核和退款管理操作，但保留只读查询及审计导出。
4. 回滚 API 应用版本；已固化为 V1.3.2 的历史订单不得迁回 Legacy。
5. 已执行 089 至 096 后，不删除财务、奖励、钱包、退款或审计数据，不修改历史迁移。
6. 若迁移导致结构性故障，从迁移前备份恢复到独立环境验证修复，再通过新的前向迁移处理生产；禁止直接 DROP 新表或约束规避问题。
7. 已领取任务允许当前单轮安全结束，或等待稳定租约到期后由恢复流程接管。

089 至 096 包含新增财务及审计结构，不视为可安全物理回滚。生产迁移一旦产生真实数据，只允许前向修复。

## 8. 生产变更审批材料

审批包至少包含：

- 最新脱敏生产副本迁移报告、每个迁移耗时、锁等待和表/索引大小差异。
- Preflight 全部 PASS 的 JSON 输出。
- 全量回归、优雅关闭、Legacy/Shadow/Canary 和系统 E2E 结果。
- 监控面板及告警路由截图或导出。
- 首笔订单操作人、复核人、回滚负责人和时间窗口。
- 迁移前备份编号及恢复演练证据。

只有上述材料齐全且零 Critical 门禁时，才具备进入生产变更审批条件；审批通过也不代表已经执行生产切换。
