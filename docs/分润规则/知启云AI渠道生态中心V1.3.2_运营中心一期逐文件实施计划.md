# 知启云AI渠道生态中心 V1.3.2 运营中心一期 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不启用 Canary 和真实切换的前提下，实现运营中心技术服务费支付、审核激活、推荐奖励、奖励释放、统一退款 Saga、人工退款及完整不变量测试。

**Architecture:** 运营中心状态验证放入独立 `internal/app/operationcenter` 领域包，PostgreSQL 原子写入复用现有 `httpserver` 事务和 V1.3.2 钱包方法。退款采用“第一事务原子撤权与冲正、事务外调用 `PaymentProvider.RefundPayment`、第二事务记录 Provider 结果”的 Saga；`applyCommerceOrderFulfillmentForTx` 只在最后一批接入。

**Tech Stack:** Go、PostgreSQL、`database/sql`、Gin、现有 `channelrules`、`payment`、V1.3.2 佣金钱包和 RBAC 投影。

## Global Constraints

- 不修改 `002` 至 `088` 已执行历史迁移，只新增前向迁移 `089`。
- 当前运行配置继续保持 `mode=SHADOW`、`real_switch_enabled=false`、`percentage_rollout_enabled=false`。
- 运营中心不加入 Canary 白名单，最后一批接入业务入口后也不自动启用真实切换。
- 所有金额、冻结周期和退款策略只读取订单固化的已发布规则版本，不在 Go 代码写默认商业金额。
- `CommissionEngine` 保持纯计算，不查询数据库、不解析关系、不写钱包。
- `ChannelRuleService` 负责已发布规则、套餐、关系快照、订单商业快照和金额守恒。
- 代理推荐运营中心只奖励推荐代理和事件快照所属运营中心，任何上级代理不得获奖。
- `REVERSING`、撤权、奖励与钱包冲正、`REVOKED`、`PROVIDER_PENDING` 必须在同一个 PostgreSQL 事务内完成。
- Provider 调用必须在数据库事务外执行。
- 退款稳定幂等键不得包含时间戳、随机数或重试次数。
- `UNKNOWN` 不得直接重新调用退款；先查询渠道状态，暂无查询能力时进入延迟核验或人工核验。
- 退款进入 Provider 处理阶段后，运营中心不得恢复 `ACTIVE`。
- RBAC 继续使用本地 PostgreSQL `xz_user_roles` 投影，授权和撤权必须使用调用方传入的同一个 `*sql.Tx`。
- 微信虚拟支付是首个实际退款适配渠道，但运营中心领域包不得引用微信类型。
- 测试环境与生产环境使用独立数据库和独立已发布规则版本。

---

## 1. 文件结构总览

### 1.1 新增文件

| 文件 | 职责 |
| --- | --- |
| `database/migrations/089-operation-center-lifecycle-refund-saga.sql` | 新增退款任务、人工退款、状态迁移审计，扩展服务订单、推荐奖励和钱包引用，增加权限点 |
| `backend-go/internal/app/operationcenter/types.go` | 定义运营中心、退款任务、人工退款状态和命令类型 |
| `backend-go/internal/app/operationcenter/state_machine.go` | 校验状态迁移、构造稳定退款幂等键、约束 UNKNOWN 重试 |
| `backend-go/internal/app/operationcenter/state_machine_test.go` | 状态机、幂等键和 Provider 结果单元测试 |
| `backend-go/internal/httpserver/operation_center_lifecycle_postgres.go` | 支付待审、审核通过、审核拒绝、激活后撤权的事务内 SQL |
| `backend-go/internal/httpserver/operation_center_rewards_postgres.go` | 推荐事件、冻结奖励、释放、三类冲正和 `recoverable_cents` |
| `backend-go/internal/httpserver/operation_center_refund_saga.go` | 退款准备事务、事务外 Provider 调用、结果落库、重试和人工核验 |
| `backend-go/internal/httpserver/operation_center_lifecycle_api.go` | 审核、退款、核验、人工退款提交和审批 HTTP 处理器 |
| `backend-go/internal/httpserver/wechat_virtual_refund_provider.go` | 微信虚拟支付退款适配和 Provider 结果归一化 |
| `backend-go/internal/httpserver/operation_center_lifecycle_postgres_test.go` | 正向激活、推荐奖励、冻结释放和权限不变量 PostgreSQL 测试 |
| `backend-go/internal/httpserver/operation_center_refund_saga_postgres_test.go` | 退款 Saga、四类 Provider 结果、人工退款和资金不变量 PostgreSQL 测试 |
| `backend-go/internal/httpserver/operation_center_lifecycle_api_test.go` | 审核、退款、人工登记、权限和幂等 HTTP 测试 |

### 1.2 修改文件

| 文件 | 修改内容 |
| --- | --- |
| `backend-go/internal/app/payment/provider.go` | 扩展 `RefundPaymentResult` 为四类结果，新增可选 `RefundQueryProvider` |
| `backend-go/internal/app/payment/payment_test.go` | 覆盖 Mock Provider 四类结果、查询能力和同幂等键重试 |
| `backend-go/internal/app/channelrules/types.go` | 新增运营中心已发布规则快照 DTO 和退款策略 DTO |
| `backend-go/internal/app/channelrules/transaction_store.go` | 新增只允许 `PUBLISHED` 的运营中心规则读取方法 |
| `backend-go/internal/app/channelrules/rollout_postgres_integration_test.go` | 验证运营中心历史规则固定、DRAFT 不可用于正式激活 |
| `backend-go/internal/httpserver/commerce_v132_wallet.go` | 增加推荐奖励钱包引用、三类冲正和 `recoverable_cents` 抵扣 |
| `backend-go/internal/httpserver/payment_center_api.go` | 注册退款 Provider 解析器，不改变现有支付下单路径 |
| `backend-go/internal/httpserver/server.go` | 注册运营中心审核、退款、核验和人工退款路由 |
| `backend-go/internal/httpserver/governance.go` | 将新路由映射到渠道和财务权限点 |
| `backend-go/internal/httpserver/postgres_store.go` | 最后一批在 `applyCommerceOrderFulfillmentForTx` 接入支付成功待审核流程 |
| `backend-go/internal/httpserver/wechat_virtual_entitlements.go` | 将已确认微信退款通知转交统一退款结果落库，不在文件内写运营中心状态机 |
| `backend-go/internal/httpserver/wechat_virtual_payment_postgres_test.go` | 增加微信虚拟支付运营中心退款生命周期验证 |
| `backend-go/internal/httpserver/runtime_projection_baseline_postgres_test.go` | 将 `089` 纳入全量初始化契约和关键表存在性断言 |

---

## 2. 数据库前向迁移设计

### 2.1 迁移文件

**Create:** `database/migrations/089-operation-center-lifecycle-refund-saga.sql`

迁移必须包含 `BEGIN`、全部 DDL/DML、约束校验和 `COMMIT`。不得编辑 `079` 至 `088`。

### 2.2 扩展 `xz_operation_center_service_orders`

新增字段：

```sql
ALTER TABLE xz_operation_center_service_orders
  ADD COLUMN IF NOT EXISTS commercial_rule_set_id TEXT REFERENCES xz_commercial_rule_sets(id),
  ADD COLUMN IF NOT EXISTS commercial_rule_set_version INT,
  ADD COLUMN IF NOT EXISTS plan_version_id TEXT REFERENCES xz_commercial_plan_versions(id),
  ADD COLUMN IF NOT EXISTS commercial_order_snapshot_id TEXT REFERENCES xz_commercial_order_rule_snapshots(id),
  ADD COLUMN IF NOT EXISTS relationship_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS refund_policy_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS review_idempotency_key TEXT,
  ADD COLUMN IF NOT EXISTS activation_idempotency_key TEXT,
  ADD COLUMN IF NOT EXISTS current_refund_task_id TEXT;
```

约束和索引：

- `commercial_rule_set_version > 0`。
- `relationship_snapshot` 和 `refund_policy_snapshot` 必须为 JSON object。
- `review_idempotency_key`、`activation_idempotency_key` 分别建立非空部分唯一索引。
- 替换现有状态检查约束时保留全部历史状态，并加入 `REVOKING`。
- 对 `tenant_id, status, updated_at DESC` 建立审核和退款处理索引。
- 先从 `xz_commercial_order_rule_snapshots` 回填规则和套餐引用，再验证“除 `PENDING_PAYMENT` 外规则快照引用必须完整”的 `NOT VALID` 检查约束。

业务不变量：

- 一个商业订单只能有一条运营中心服务订单，继续复用现有 `UNIQUE(order_id)`。
- `REVIEW_REQUIRED` 及后续状态必须能追溯到已发布规则版本和订单商业快照。
- `ACTIVE` 必须具有 `activated_at`；`REVOKED` 必须具有 `revoked_at`。
- 已进入退款 Provider 阶段的记录不能通过普通审核接口回到 `ACTIVE`，由应用条件更新和状态迁移测试保证。

### 2.3 新增 `xz_operation_center_refund_tasks`

```sql
CREATE TABLE IF NOT EXISTS xz_operation_center_refund_tasks (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  service_order_id TEXT NOT NULL REFERENCES xz_operation_center_service_orders(id),
  order_id TEXT NOT NULL REFERENCES xz_orders(id),
  payment_record_id TEXT NOT NULL REFERENCES xz_payment_records(id),
  commercial_rule_set_id TEXT NOT NULL REFERENCES xz_commercial_rule_sets(id),
  origin_type TEXT NOT NULL CHECK (origin_type IN ('REVIEW_REJECTION','ACTIVE_REVOCATION')),
  refund_scope TEXT NOT NULL CHECK (refund_scope = 'FULL'),
  amount_cents BIGINT NOT NULL CHECK (amount_cents > 0),
  currency TEXT NOT NULL,
  payment_provider TEXT NOT NULL,
  provider_payment_id TEXT,
  provider_refund_id TEXT,
  provider_outcome TEXT CHECK (provider_outcome IS NULL OR provider_outcome IN ('SUCCESS','TEMPORARY_FAILURE','UNSUPPORTED','UNKNOWN')),
  refund_status TEXT NOT NULL CHECK (refund_status IN ('PENDING','REVERSING','PROVIDER_PENDING','REFUND_RETRYABLE','UNKNOWN_VERIFYING','MANUAL_REQUIRED','MANUAL_SUBMITTED','SUCCEEDED','CANCELLED')),
  idempotency_key TEXT NOT NULL,
  attempt_count INT NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  next_attempt_at TIMESTAMPTZ,
  lease_owner TEXT,
  lease_expires_at TIMESTAMPTZ,
  unknown_since TIMESTAMPTZ,
  prepared_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  last_error_code TEXT,
  last_error_message TEXT,
  state_version BIGINT NOT NULL DEFAULT 0 CHECK (state_version >= 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (idempotency_key),
  UNIQUE (service_order_id, refund_scope)
);
```

索引和附加约束：

- `refund_status, next_attempt_at, created_at` 任务领取索引。
- `tenant_id, service_order_id` 查询索引。
- `payment_provider, provider_refund_id` 非空部分唯一索引。
- `UNKNOWN_VERIFYING` 必须有 `unknown_since`。
- `PROVIDER_PENDING` 及后续状态必须有 `prepared_at`。
- `SUCCEEDED` 必须有 `completed_at`。
- 创建表后为 `xz_operation_center_service_orders.current_refund_task_id` 添加外键。

业务不变量：

- 同一服务订单一期只能创建一个全额退款任务。
- `amount_cents` 必须由服务端复制原支付成功金额；跨表相等关系由事务服务和集成测试验证。
- 稳定幂等键全局唯一且创建后不可更新。
- Provider 不得在任务到达 `PROVIDER_PENDING` 前调用。

### 2.4 新增 `xz_operation_center_manual_refunds`

字段：

```sql
CREATE TABLE IF NOT EXISTS xz_operation_center_manual_refunds (
  id TEXT PRIMARY KEY,
  refund_task_id TEXT NOT NULL REFERENCES xz_operation_center_refund_tasks(id),
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  amount_cents BIGINT NOT NULL CHECK (amount_cents > 0),
  currency TEXT NOT NULL,
  provider_transaction_id TEXT NOT NULL,
  provider_refund_id TEXT,
  voucher_reference TEXT NOT NULL,
  voucher_file_hash TEXT NOT NULL CHECK (voucher_file_hash ~ '^[0-9a-f]{64}$'),
  status TEXT NOT NULL CHECK (status IN ('SUBMITTED','APPROVED','REJECTED')),
  submitted_by TEXT NOT NULL REFERENCES xz_users(id),
  submitted_at TIMESTAMPTZ NOT NULL,
  approved_by TEXT REFERENCES xz_users(id),
  approved_at TIMESTAMPTZ,
  rejection_reason TEXT,
  remark TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK ((status = 'SUBMITTED' AND approved_by IS NULL AND approved_at IS NULL) OR status <> 'SUBMITTED'),
  CHECK (approved_by IS NULL OR approved_by <> submitted_by)
);
```

索引和唯一约束：

- `refund_task_id, status, created_at DESC` 查询索引。
- 每个退款任务只允许一条 `APPROVED` 记录的部分唯一索引。
- `payment_provider + provider_transaction_id` 的唯一性由退款任务关联查询验证，避免跨 Provider 碰撞。

业务不变量：

- 提交人与审批人必须分离。
- 人工退款金额必须等于任务金额，币种必须一致。
- 人工审批只补登记支付事实，不重复撤权和冲正。

### 2.5 新增 `xz_operation_center_state_transitions`

字段：

```sql
CREATE TABLE IF NOT EXISTS xz_operation_center_state_transitions (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL REFERENCES xz_tenants(id),
  entity_type TEXT NOT NULL CHECK (entity_type IN ('SERVICE_ORDER','REFUND_TASK','REFERRAL_REWARD')),
  entity_id TEXT NOT NULL,
  from_status TEXT,
  to_status TEXT NOT NULL,
  action TEXT NOT NULL,
  actor_id TEXT,
  request_id TEXT,
  idempotency_key TEXT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (entity_type, entity_id, idempotency_key, to_status)
);
```

索引：

- `entity_type, entity_id, created_at, id` 时间线索引。
- `tenant_id, action, created_at DESC` 审计查询索引。

业务不变量：

- `ACTIVE -> REVOKING -> REVOKED` 和 `PENDING -> REVERSING -> PROVIDER_PENDING` 即使在同一事务内也必须留下连续审计记录。
- 重复请求不得插入第二条相同目标状态记录。

### 2.6 扩展推荐奖励和钱包引用

`xz_referral_rewards` 新增：

- `commercial_rule_set_id TEXT REFERENCES xz_commercial_rule_sets(id)`，从 `reward_rule_id` 回填后设为非空。
- `grant_wallet_ledger_id TEXT REFERENCES xz_commission_wallet_ledger(id)`。
- `release_wallet_ledger_id TEXT REFERENCES xz_commission_wallet_ledger(id)`。
- `original_wallet_ledger_id TEXT REFERENCES xz_commission_wallet_ledger(id)`。
- `reversal_wallet_ledger_id TEXT REFERENCES xz_commission_wallet_ledger(id)`。
- `refund_task_id TEXT REFERENCES xz_operation_center_refund_tasks(id)`。
- `recoverable_cents BIGINT NOT NULL DEFAULT 0 CHECK (recoverable_cents >= 0)`。

复用现有 `reversal_of_id` 表示原奖励；保留现有一条奖励只能有一条冲正记录的唯一索引。新增检查约束：新写入的 `REVERSAL` 必须同时具有 `reversal_of_id`、`commercial_rule_set_id`、`original_wallet_ledger_id`、`reversal_wallet_ledger_id` 和 `refund_task_id`。

`xz_commission_wallet_ledger` 新增：

- `referral_reward_id TEXT REFERENCES xz_referral_rewards(id)`。
- `original_ledger_id TEXT REFERENCES xz_commission_wallet_ledger(id)`。
- `refund_task_id TEXT REFERENCES xz_operation_center_refund_tasks(id)`。
- `recoverable_cents_delta BIGINT NOT NULL DEFAULT 0`。

新增部分唯一索引：`refund_task_id, original_ledger_id` 在两者非空时唯一。

业务不变量：

- 每笔冲正必须引用原奖励、原事件、原规则、原钱包流水和退款任务。
- `FROZEN` 冲正只扣冻结余额。
- `AVAILABLE` 冲正先扣可提现余额；已进入待提现的部分必须在同一事务锁定并冲减。
- `SETTLED` 不修改历史已提现金额，转入账户 `recoverable_cents` 并由后续收入优先抵扣。
- 原奖励和原钱包流水不可更新金额或删除。

### 2.7 权限初始化

迁移新增权限：

```text
channel:operation-center:review
channel:operation-center:refund
finance:operation-center-refund:view
finance:operation-center-refund:retry
finance:operation-center-refund:verify
finance:operation-center-refund:manual-submit
finance:operation-center-refund:manual-approve
```

写入现有 `permissions`、`role_permissions` 和 `xz_role_permissions`。`SUPER_ADMIN` 拥有全部权限；渠道审核角色获得 review；财务角色获得 view、retry、verify、manual-submit、manual-approve。迁移使用 `INSERT ... ON CONFLICT DO NOTHING`，不依赖生产环境外键 ID 常量。

---

## 3. 代码接口和事务职责

### 3.1 领域状态类型

**Create:** `backend-go/internal/app/operationcenter/types.go`

新增类型：

```go
type ServiceStatus string
type RefundStatus string
type RefundOrigin string

const (
    ServicePendingPayment ServiceStatus = "PENDING_PAYMENT"
    ServiceReviewRequired ServiceStatus = "REVIEW_REQUIRED"
    ServiceActive         ServiceStatus = "ACTIVE"
    ServiceRejected       ServiceStatus = "REJECTED"
    ServiceRevoking       ServiceStatus = "REVOKING"
    ServiceRevoked        ServiceStatus = "REVOKED"
)

const (
    RefundPending          RefundStatus = "PENDING"
    RefundReversing        RefundStatus = "REVERSING"
    RefundProviderPending  RefundStatus = "PROVIDER_PENDING"
    RefundRetryable        RefundStatus = "REFUND_RETRYABLE"
    RefundUnknownVerifying RefundStatus = "UNKNOWN_VERIFYING"
    RefundManualRequired   RefundStatus = "MANUAL_REQUIRED"
    RefundManualSubmitted  RefundStatus = "MANUAL_SUBMITTED"
    RefundSucceeded        RefundStatus = "SUCCEEDED"
)
```

命令类型：

- `PaymentSucceededCommand`
- `ReviewCommand`
- `RequestRefundCommand`
- `DispatchRefundCommand`
- `VerifyUnknownRefundCommand`
- `SubmitManualRefundCommand`
- `ApproveManualRefundCommand`
- `ReleaseRewardsCommand`

每个命令必须包含 tenant、目标 ID、actor、request ID、稳定请求幂等键和业务时间；不得包含客户端决定的奖励金额、冻结周期或退款金额。

### 3.2 状态机和稳定幂等键

**Create:** `backend-go/internal/app/operationcenter/state_machine.go`

接口：

```go
func ValidateServiceTransition(from, to ServiceStatus) error
func ValidateRefundTransition(from, to RefundStatus) error
func StableRefundIdempotencyKey(tenantID, serviceOrderID, paymentRecordID string) (string, error)
func CanDispatchRefund(status RefundStatus) bool
func CanRetryRefund(status RefundStatus, providerOutcome payment.RefundOutcome) bool
```

`StableRefundIdempotencyKey` 固定对以下字符串做 SHA-256：

```text
operation-center-refund:v1:{tenant_id}:{service_order_id}:{payment_record_id}:FULL
```

只有 `REFUND_RETRYABLE + TEMPORARY_FAILURE` 可直接回到 `PROVIDER_PENDING`。`UNKNOWN_VERIFYING` 必须经过查询结果或人工核验命令，不得由普通 retry 接口推进。

### 3.3 已发布规则读取

**Modify:** `backend-go/internal/app/channelrules/types.go`

新增：

```go
type OperationCenterRuleSnapshot struct {
    RuleSetID          string
    RuleSetVersion     int
    PlanVersionID      string
    ServiceFeeCents    int64
    Currency           string
    RefundPolicy       string
    ReferralRules      []ReferralRewardRule
}
```

**Modify:** `backend-go/internal/app/channelrules/transaction_store.go`

新增：

```go
func (s *TransactionStore) LoadPublishedOperationCenterRules(
    ctx context.Context,
    tenantID, ruleSetID string,
    ruleSetVersion int,
    planID string,
    businessTime time.Time,
) (OperationCenterRuleSnapshot, error)
```

事务内操作。SQL 必须要求 `xz_commercial_rule_sets.status='PUBLISHED'`、套餐身份为 `OPERATION_CENTER`、推荐规则状态为 `PUBLISHED`。不得复用允许 `DRAFT` 的 Shadow 查询条件。

### 3.4 PaymentProvider 四类结果

**Modify:** `backend-go/internal/app/payment/provider.go`

新增：

```go
type RefundOutcome string

const (
    RefundOutcomeSuccess          RefundOutcome = "SUCCESS"
    RefundOutcomeTemporaryFailure RefundOutcome = "TEMPORARY_FAILURE"
    RefundOutcomeUnsupported      RefundOutcome = "UNSUPPORTED"
    RefundOutcomeUnknown          RefundOutcome = "UNKNOWN"
)

type RefundPaymentResult struct {
    Outcome          RefundOutcome `json:"outcome"`
    ProviderRefundID string        `json:"providerRefundId"`
    ProviderCode     string        `json:"providerCode"`
    Message          string        `json:"message"`
    Status           PaymentStatus `json:"status"`
    CompletedAt      *time.Time    `json:"completedAt,omitempty"`
}

type QueryRefundRequest struct {
    PaymentNo       string
    RefundNo        string
    ProviderRefundID string
}

type QueryRefundResult struct {
    Outcome          RefundOutcome
    ProviderRefundID string
    Status           PaymentStatus
}

type RefundQueryProvider interface {
    QueryRefund(context.Context, QueryRefundRequest) (QueryRefundResult, error)
}
```

保持现有 `PaymentProvider.RefundPayment` 方法名和请求结构。渠道可确认的失败不使用 Go `error` 传递，统一写入 `RefundPaymentResult.Outcome`；Go `error` 仅表示调用前参数错误或适配器无法形成结果，Saga 将其保守归类为 `UNKNOWN`。

现有 Mock 兼容：

- 默认返回 `SUCCESS`。
- 按测试配置返回 `TEMPORARY_FAILURE`、`UNSUPPORTED`、`UNKNOWN`。
- 同一 `RefundNo` 保存并返回相同 Provider 退款结果，验证稳定幂等。
- Mock 实现 `RefundQueryProvider`，可查询已保存结果。

### 3.5 事务内生命周期服务

**Create:** `backend-go/internal/httpserver/operation_center_lifecycle_postgres.go`

新增方法：

```go
func markOperationCenterPaymentReviewRequiredTx(ctx context.Context, tx *sql.Tx, order *adminOrder) error
func approveOperationCenterApplicationTx(ctx context.Context, tx *sql.Tx, cmd operationcenter.ReviewCommand) error
func rejectOperationCenterApplicationTx(ctx context.Context, tx *sql.Tx, cmd operationcenter.ReviewCommand) (string, error)
func prepareActiveOperationCenterRefundTx(ctx context.Context, tx *sql.Tx, cmd operationcenter.RequestRefundCommand) (string, error)
func grantOperationCenterRBACTx(ctx context.Context, tx *sql.Tx, tenantID, userID string, now time.Time) error
func revokeOperationCenterRBACTx(ctx context.Context, tx *sql.Tx, tenantID, userID string, now time.Time) error
```

事务职责：

- `mark...ReviewRequiredTx`：锁订单和服务订单，保存已发布规则与关系快照，只写 `REVIEW_REQUIRED`，不激活、不授予 RBAC、不发奖。
- `approve...Tx`：单事务更新服务订单、用户状态、运营中心档案、商业身份、RBAC、订单履约和冻结推荐奖励。
- `reject...Tx`：更新 `REJECTED`，确认无 ACTIVE 身份/RBAC/奖励，创建统一退款任务并推进到 `PROVIDER_PENDING`。
- `prepareActive...Tx`：在同一事务记录 `REVERSING`、撤权、冲正、`REVOKED` 和 `PROVIDER_PENDING`。
- RBAC 方法只接收 `*sql.Tx`，禁止内部使用全局 `*sql.DB`。

### 3.6 推荐奖励和钱包

**Create:** `backend-go/internal/httpserver/operation_center_rewards_postgres.go`

新增方法：

```go
func createOperationCenterReferralRewardsTx(ctx context.Context, tx *sql.Tx, serviceOrderID string, now time.Time) error
func releaseDueReferralRewardsTx(ctx context.Context, tx *sql.Tx, rewardIDs []string, now time.Time) error
func reverseOperationCenterReferralRewardsTx(ctx context.Context, tx *sql.Tx, refundTaskID, serviceOrderID string, now time.Time) error
func reverseSingleReferralRewardTx(ctx context.Context, tx *sql.Tx, refundTaskID string, rewardID string, now time.Time) error
```

**Modify:** `backend-go/internal/httpserver/commerce_v132_wallet.go`

修改职责：

- `v132CommissionWalletBalances` 加入 `RecoverableCents`。
- `lockV132CommissionWalletAccountTx` 和 `updateV132CommissionWalletAccountTx` 同步读取、更新 `recoverable_cents`。
- `v132CommissionWalletLedgerEntry` 加入 reward、原流水、退款任务和 `RecoverableCentsDelta` 引用。
- `postV132CommissionRecordToWalletTx` 入账时先抵扣账户 `recoverable_cents`，仅将剩余金额进入冻结余额。
- `reverseV132CommissionWalletTx` 保持现有订单佣金退款兼容；运营中心奖励调用新的强类型辅助函数，不拼装无引用的通用负向流水。

三类奖励冲正：

- `FROZEN`：冻结余额减少原奖励金额，写 DEBIT 负向流水，`recoverable_cents=0`。
- `AVAILABLE`：先锁钱包和待提现记录，扣可提现余额；已进入待提现的部分在同一事务撤销或冲减；仍不足的差额进入 `recoverable_cents`。
- `SETTLED`：不减少历史已提现金额，将全额增加到 `recoverable_cents`，写引用原结算流水的 RECOVERY 负向流水。

`recoverable_cents` 后续抵扣公式：

```go
recovered := min(incomingAmountCents, balances.RecoverableCents)
balances.RecoverableCents -= recovered
creditable := incomingAmountCents - recovered
balances.FrozenCents += creditable
```

必须分别写 RECOVERY 流水和剩余 CREDIT 流水，二者金额之和等于新收入金额。

### 3.7 退款 Saga

**Create:** `backend-go/internal/httpserver/operation_center_refund_saga.go`

新增类型和方法：

```go
type operationCenterRefundProviderResolver interface {
    RefundProvider(name string) (payment.PaymentProvider, bool)
}

func (s *postgresStore) dispatchOperationCenterRefund(ctx context.Context, taskID, workerID string) error
func (s *postgresStore) claimOperationCenterRefundTask(ctx context.Context, taskID, workerID string, now time.Time) (operationCenterRefundTask, error)
func (s *postgresStore) recordOperationCenterRefundResult(ctx context.Context, taskID string, result payment.RefundPaymentResult, now time.Time) error
func (s *postgresStore) retryOperationCenterRefund(ctx context.Context, taskID, actorID, requestID string) error
func (s *postgresStore) verifyUnknownOperationCenterRefund(ctx context.Context, cmd operationcenter.VerifyUnknownRefundCommand) error
func (s *postgresStore) submitManualOperationCenterRefund(ctx context.Context, cmd operationcenter.SubmitManualRefundCommand) error
func (s *postgresStore) approveManualOperationCenterRefund(ctx context.Context, cmd operationcenter.ApproveManualRefundCommand) error
```

事务边界：

1. 第一事务由 reject 或 active refund 方法完成。激活后退款必须原子完成 `REVERSING`、撤权、奖励及钱包冲正、`REVOKED`、`PROVIDER_PENDING`。
2. `claimOperationCenterRefundTask` 使用 `SELECT ... FOR UPDATE SKIP LOCKED` 和租约领取任务，提交后才调用 Provider。
3. `PaymentProvider.RefundPayment` 在事务外调用，参数从任务读取，`RefundNo` 使用稳定幂等键派生的持久化退款号。
4. `recordOperationCenterRefundResult` 开启第二事务，按四类结果推进状态。

结果映射：

| Provider outcome | refund_status | 后续入口 |
| --- | --- | --- |
| `SUCCESS` | `SUCCEEDED` | 无，重复结果幂等返回 |
| `TEMPORARY_FAILURE` | `REFUND_RETRYABLE` | 财务 retry 或退避任务 |
| `UNSUPPORTED` | `MANUAL_REQUIRED` | 人工退款提交 |
| `UNKNOWN` | `UNKNOWN_VERIFYING` | QueryRefund 或人工核验 |

失败恢复：

- 第一事务失败：整体回滚，不领取任务，不调用 Provider。
- Provider 调用失败：运营中心保持 `REVOKED`；明确临时失败可复用原幂等键重试。
- 第二事务失败：任务仍可按稳定幂等键重新记录或重新查询，Provider 不生成第二笔退款。
- UNKNOWN：若 Provider 实现 `RefundQueryProvider`，只调用 `QueryRefund`；未实现时设置延迟核验时间并要求人工结论。
- 人工确认“未退款”后才允许状态回到 `PROVIDER_PENDING`；确认“已退款”时要求渠道流水和凭证后完成。

### 3.8 HTTP 和 RBAC

**Create:** `backend-go/internal/httpserver/operation_center_lifecycle_api.go`

处理器：

```go
func (a operationCenterLifecycleAPI) review(w http.ResponseWriter, r *http.Request)
func (a operationCenterLifecycleAPI) requestRefund(w http.ResponseWriter, r *http.Request)
func (a operationCenterLifecycleAPI) retryRefund(w http.ResponseWriter, r *http.Request)
func (a operationCenterLifecycleAPI) verifyUnknown(w http.ResponseWriter, r *http.Request)
func (a operationCenterLifecycleAPI) submitManualRefund(w http.ResponseWriter, r *http.Request)
func (a operationCenterLifecycleAPI) approveManualRefund(w http.ResponseWriter, r *http.Request)
```

**Modify:** `backend-go/internal/httpserver/server.go`

新增路由：

```text
POST /api/v1/admin/channel/operation-centers/applications/:id/review
POST /api/v1/admin/channel/operation-centers/:id/refunds
GET  /api/v1/admin/channel/refunds
POST /api/v1/admin/channel/refunds/:id/retry
POST /api/v1/admin/channel/refunds/:id/verify
POST /api/v1/admin/channel/refunds/:id/manual-submissions
POST /api/v1/admin/channel/refunds/:id/manual-approval
```

**Modify:** `backend-go/internal/httpserver/governance.go`

路由到权限映射：

- review -> `channel:operation-center:review`
- request refund -> `channel:operation-center:refund`
- list -> `finance:operation-center-refund:view`
- retry -> `finance:operation-center-refund:retry`
- verify -> `finance:operation-center-refund:verify`
- manual submit -> `finance:operation-center-refund:manual-submit`
- manual approval -> `finance:operation-center-refund:manual-approve`

HTTP 约束：

- 审核、退款、重试、核验、人工提交和审批要求 `Idempotency-Key`。
- 请求体不接收金额、冻结天数和规则版本。
- `UNKNOWN_VERIFYING` 调用 retry 返回 `409`，必须走 verify。
- 非法状态返回 `409`；缺权限返回 `403`；重复幂等请求返回首次结果。

---

## 4. 状态机逐流程实现

### 4.1 支付成功

事务内：

1. `applyCommerceOrderFulfillmentForTx` 识别 `OPERATION_CENTER_JOIN`。
2. 调用 `markOperationCenterPaymentReviewRequiredTx`。
3. 锁定订单、支付记录和服务订单。
4. 从订单商业快照读取已发布规则、套餐、关系和退款策略。
5. 写 `REVIEW_REQUIRED`，保持身份、档案、用户状态和 RBAC 非 ACTIVE。
6. 不创建 `xz_referral_events` 和 `xz_referral_rewards`。

重复回调：服务订单 `UNIQUE(order_id)` 加状态条件更新，只返回既有结果。

### 4.2 审核通过

单事务：

1. 锁 `REVIEW_REQUIRED` 服务订单。
2. 校验支付成功和已发布规则快照。
3. 更新服务订单 `ACTIVE`。
4. 更新用户运营中心状态、`xz_operation_centers` 档案和 `xz_user_business_identities` 为 ACTIVE。
5. 用同一 `*sql.Tx` 激活 `xz_user_roles` 运营中心角色。
6. 更新订单履约完成。
7. 按支付时关系快照创建推荐事件。
8. 按已发布规则创建冻结奖励和钱包流水。
9. 写连续状态迁移和审计记录。

任一步失败，全部回滚。

### 4.3 审核拒绝

单事务：

1. 锁 `REVIEW_REQUIRED` 服务订单。
2. 更新 `REJECTED`。
3. 断言不存在 ACTIVE 身份、RBAC 和奖励。
4. 创建 `origin_type=REVIEW_REJECTION` 的统一退款任务。
5. 记录 `PENDING -> REVERSING -> PROVIDER_PENDING`。
6. 提交后由统一调度器调用 Provider。

### 4.4 激活后退款

第一事务：

1. 锁 ACTIVE 服务订单、身份、档案、RBAC、全部推荐奖励、钱包和待提现记录。
2. 校验历史规则快照为一期全额退款。
3. 创建退款任务并记录 `REVERSING`。
4. 记录服务订单 `REVOKING`。
5. 撤销身份、档案、用户状态和本地 RBAC。
6. 分别冲正 FROZEN、AVAILABLE、SETTLED 奖励。
7. 写营销费用和钱包负向流水。
8. 记录 `REVOKED` 和 `PROVIDER_PENDING`。
9. 提交。

事务外：调用 Provider。

第二事务：记录四类 Provider 结果。

### 4.5 奖励冻结、释放和冲正

- 冻结：审核通过事务创建 `FROZEN` 奖励，`freeze_until` 取规则版本。
- 释放：任务使用 `FOR UPDATE SKIP LOCKED` 领取到期奖励，同事务从冻结移到可提现并写流水。
- 冲正：按原奖励状态执行第 3.6 节算法，每条冲正引用原奖励、事件、规则和钱包流水。

### 4.6 人工退款

- submit：仅 `MANUAL_REQUIRED` 或已核验为人工处理的 UNKNOWN 可提交；校验金额、币种、流水号、凭证引用和 SHA-256 文件哈希。
- approve：审批人与提交人不同；同事务将人工记录设为 APPROVED、退款任务设为 SUCCEEDED并写审计。
- reject：人工记录设为 REJECTED，退款任务回到 `MANUAL_REQUIRED`，不恢复权限。

---

## 5. 并发与幂等策略

| 场景 | 锁与唯一保护 | 重复行为 |
| --- | --- | --- |
| 支付回调 | 订单行锁、支付事件唯一键、服务订单 `UNIQUE(order_id)` | 返回既有 `REVIEW_REQUIRED` |
| 审核 | 服务订单 `FOR UPDATE`、review 幂等键唯一 | 返回首次审核结果 |
| 激活 | `WHERE status='REVIEW_REQUIRED'` 条件更新、activation 幂等键 | 不重复身份/RBAC/奖励 |
| 推荐发奖 | `source_order_id` 推荐事件唯一、受益人规则唯一 | 不重复冻结钱包 |
| 奖励释放 | 到期任务 `SKIP LOCKED`、原奖励释放流水唯一 | 不重复增加可提现 |
| 发起退款 | 服务订单行锁、`UNIQUE(service_order_id,refund_scope)` | 返回原退款任务 |
| 退款领取 | `FOR UPDATE SKIP LOCKED` 加 lease | 单任务同一时刻只有一个 worker |
| Provider 重试 | 持久化 `idempotency_key` 和 `RefundNo` | 渠道命中同一退款 |
| UNKNOWN | 状态机禁止 retry；只能 query/verify | 不盲目再次退款 |
| 奖励冲正 | `reversal_of_id` 唯一、钱包原流水与退款任务唯一 | 不重复扣钱包 |
| 人工审批 | 每任务一条 APPROVED 部分唯一索引 | 不重复完成退款 |

固定数据库加锁顺序：

```text
service_order -> refund_task -> business_identity -> operation_center_profile
-> user_roles -> referral_rewards(order by id) -> wallet_accounts(order by id)
-> pending_withdrawals(order by id) -> wallet_ledger
```

所有事务方法遵循同一顺序，减少死锁。

---

## 6. 测试计划

### 6.1 单元测试

**Test:** `backend-go/internal/app/operationcenter/state_machine_test.go`

测试名称：

- `TestServiceTransitionsAllowOnlyDocumentedEdges`
- `TestRefundTransitionsAllowOnlyDocumentedEdges`
- `TestStableRefundIdempotencyKeyIgnoresRetryAndTime`
- `TestUnknownCannotUseRetryTransition`
- `TestTemporaryFailureCanRetryWithSameKey`

**Modify:** `backend-go/internal/app/payment/payment_test.go`

新增：

- `TestMockRefundReturnsFourOutcomes`
- `TestMockRefundSameRefundNoIsIdempotent`
- `TestMockQueryRefundReturnsStoredOutcome`

运行：

```powershell
cd backend-go
go test ./internal/app/operationcenter ./internal/app/payment -count=1
```

期望：全部 PASS。

### 6.2 PostgreSQL 集成测试

**Test:** `backend-go/internal/httpserver/operation_center_lifecycle_postgres_test.go`

覆盖：

- 支付后 `REVIEW_REQUIRED` 且无 ACTIVE 身份、RBAC、档案和奖励。
- 运营中心推荐运营中心审核激活并产生配置化冻结奖励。
- 代理推荐运营中心产生代理和快照运营中心两笔奖励，上级代理为零。
- 重复支付、审核、激活幂等。
- 冻结到期释放和重复释放幂等。
- 审核事务任一注入失败点整体回滚。

**Test:** `backend-go/internal/httpserver/operation_center_refund_saga_postgres_test.go`

覆盖：

- 审核拒绝和激活后退款复用同一任务表和调度器。
- 第一事务内同时出现 REVERSING、撤权、冲正、REVOKED、PROVIDER_PENDING 审计记录。
- FROZEN、AVAILABLE、SETTLED 三类冲正。
- AVAILABLE 与待提现并发。
- SETTLED 增加 `recoverable_cents`，后续奖励优先抵扣。
- Mock Provider 的 SUCCESS、TEMPORARY_FAILURE、UNSUPPORTED、UNKNOWN。
- TEMPORARY_FAILURE 同键重试。
- UNKNOWN 不调用第二次 RefundPayment，QueryRefund 或人工核验后推进。
- Provider 成功但第二事务注入失败时，重新查询或同键落库不重复真实退款。
- 人工退款提交、双人审批、凭证引用和文件哈希。
- Provider 失败时运营中心保持 REVOKED。

不变量断言：

- `SUCCEEDED refund -> no ACTIVE identity/profile/RBAC`。
- `REVOKED -> every original reward has exactly one reversal`。
- `provider refund count per idempotency_key <= 1`。
- `original reward + reversal = 0`。
- 钱包各科目变化加 `recoverable_cents` 与全部流水守恒。
- 技术服务费平台收入与营销费用分账，不互相冲减。

运行：

```powershell
cd backend-go
go test ./internal/httpserver -run 'TestOperationCenter(Lifecycle|RefundSaga).*Postgres' -count=1
```

期望：测试 PostgreSQL 可用时全部 PASS；不可用时不得以单测临时建表替代迁移初始化。

### 6.3 HTTP 和权限测试

**Test:** `backend-go/internal/httpserver/operation_center_lifecycle_api_test.go`

覆盖：

- 审核接口的通过、拒绝、重复请求和非法状态。
- 发起全额退款，不接受客户端金额。
- retry 只接受 `REFUND_RETRYABLE`。
- UNKNOWN retry 返回 409，verify 可推进。
- 人工提交和审批权限分离。
- 缺少权限返回 403。
- RBAC 撤销后运营中心端接口失去访问权限。

运行：

```powershell
cd backend-go
go test ./internal/httpserver -run 'TestOperationCenterLifecycleAPI' -count=1
```

### 6.4 迁移和全量回归

迁移测试：

```powershell
cd backend-go
go test ./internal/app/channelrules -run 'Test.*Migration|TestPinnedRuleVersionAndProtectionPostgres' -count=1
go test ./internal/httpserver -run 'TestRuntimeProjectionBaselinePostgres' -count=1
```

微信虚拟支付生命周期：

```powershell
cd backend-go
go test ./internal/httpserver -run 'TestWechatVirtualPaymentPostgresLifecycle' -count=1
```

HTTP 全量回归：

```powershell
cd backend-go
go test ./internal/httpserver -count=1
```

最终 Go 回归：

```powershell
cd backend-go
go test ./... -count=1
```

迁移与测试只连接隔离测试数据库，不对生产数据库执行 `089`。

---

## 7. 分批编码与提交顺序

### Task 1: 数据库契约和领域状态机

**Files:**

- Create: `database/migrations/089-operation-center-lifecycle-refund-saga.sql`
- Create: `backend-go/internal/app/operationcenter/types.go`
- Create: `backend-go/internal/app/operationcenter/state_machine.go`
- Create: `backend-go/internal/app/operationcenter/state_machine_test.go`
- Modify: `backend-go/internal/app/channelrules/migration_contract_test.go`
- Modify: `backend-go/internal/httpserver/runtime_projection_baseline_postgres_test.go`

**Interfaces:**

- Produces: 第 3.1、3.2 节状态类型和函数；`089` 的表、列与约束。
- Consumes: 现有 `xz_operation_center_service_orders`、推荐奖励和钱包表。

- [ ] **Step 1: 写状态机和迁移契约失败测试**

断言合法边、非法回退、稳定幂等键、089 字段命名和约束存在。

- [ ] **Step 2: 运行单元测试并确认失败**

```powershell
cd backend-go
go test ./internal/app/operationcenter ./internal/app/channelrules -count=1
```

期望：因 operationcenter 包和 089 迁移尚不存在而 FAIL。

- [ ] **Step 3: 创建 089 迁移和最小状态机实现**

严格按第 2 节 DDL 和第 3.1、3.2 节签名实现。

- [ ] **Step 4: 运行状态机、迁移契约和基线测试**

```powershell
cd backend-go
go test ./internal/app/operationcenter ./internal/app/channelrules -count=1
go test ./internal/httpserver -run TestRuntimeProjectionBaselinePostgres -count=1
```

期望：PASS，测试库可从基线顺序执行到 089。

- [ ] **Step 5: 第一批验收**

验收标准：无业务入口变化；089 可重复初始化；历史迁移未修改；所有结构不变量有 SQL 约束或明确集成测试。

建议提交说明：`feat(channel): add operation center saga schema and state contracts`。

### Task 2: PaymentProvider 四类退款结果

**Files:**

- Modify: `backend-go/internal/app/payment/provider.go`
- Modify: `backend-go/internal/app/payment/payment_test.go`

**Interfaces:**

- Consumes: `PaymentProvider.RefundPayment` 现有签名。
- Produces: `RefundOutcome`、扩展 `RefundPaymentResult`、可选 `RefundQueryProvider`。

- [ ] **Step 1: 写 Mock 四结果和同键幂等失败测试**

每种 outcome 独立断言；UNKNOWN 查询断言不触发第二次退款。

- [ ] **Step 2: 运行 payment 单元测试并确认失败**

```powershell
cd backend-go
go test ./internal/app/payment -count=1
```

- [ ] **Step 3: 扩展 Provider 类型并修改 Mock**

保持现有方法名；默认成功行为兼容旧测试；业务 outcome 不通过错误字符串判断。

- [ ] **Step 4: 运行 payment 单元测试**

```powershell
cd backend-go
go test ./internal/app/payment -count=1
```

期望：PASS。

- [ ] **Step 5: 第二批验收**

验收标准：现有支付创建、查询、关闭、通知接口不变；四类退款结果可被 Saga 无歧义识别。

建议提交说明：`feat(payment): classify provider refund outcomes`。

### Task 3: 支付待审、审核激活和推荐奖励

**Files:**

- Modify: `backend-go/internal/app/channelrules/types.go`
- Modify: `backend-go/internal/app/channelrules/transaction_store.go`
- Modify: `backend-go/internal/app/channelrules/rollout_postgres_integration_test.go`
- Create: `backend-go/internal/httpserver/operation_center_lifecycle_postgres.go`
- Create: `backend-go/internal/httpserver/operation_center_rewards_postgres.go`
- Modify: `backend-go/internal/httpserver/commerce_v132_wallet.go`
- Create: `backend-go/internal/httpserver/operation_center_lifecycle_postgres_test.go`

**Interfaces:**

- Consumes: Task 1 状态类型和 089 表；Task 2 Provider 类型尚不在本任务调用。
- Produces: 第 3.3、3.5、3.6 节事务方法。

- [ ] **Step 1: 写支付待审和审核激活 PostgreSQL 失败测试**

先覆盖无提前激活、两种推荐场景、上级代理无奖励、重复审核和事务回滚。

- [ ] **Step 2: 写三类奖励钱包和 recoverable 失败测试**

分别构造 FROZEN、AVAILABLE、SETTLED，并断言引用和余额变化。

- [ ] **Step 3: 运行目标测试并确认失败**

```powershell
cd backend-go
go test ./internal/app/channelrules ./internal/httpserver -run 'Test(OperationCenterLifecycle|PublishedOperationCenterRules)' -count=1
```

- [ ] **Step 4: 实现已发布规则读取和事务内生命周期**

不得修改 Shadow 的 DRAFT 兼容读取；正式审核只接受 PUBLISHED。

- [ ] **Step 5: 实现冻结奖励、释放和钱包冲正辅助方法**

所有钱包写入使用同一事务，先锁奖励再按固定顺序锁钱包。

- [ ] **Step 6: 运行 Task 3 测试**

```powershell
cd backend-go
go test ./internal/app/channelrules -count=1
go test ./internal/httpserver -run 'TestOperationCenterLifecycle.*Postgres' -count=1
```

期望：PASS；此时方法只由测试调用，真实支付入口未接线。

- [ ] **Step 7: 第三批验收**

验收标准：审核通过原子激活；推荐奖励来源和冻结周期均为历史已发布规则；钱包引用完整；不影响现有 Legacy、Shadow、Canary。

建议提交说明：`feat(channel): add operation center activation and referral rewards`。

### Task 4: 统一退款 Saga 和人工退款

**Files:**

- Create: `backend-go/internal/httpserver/operation_center_refund_saga.go`
- Create: `backend-go/internal/httpserver/operation_center_refund_saga_postgres_test.go`

**Interfaces:**

- Consumes: Task 2 `RefundOutcome`；Task 3 撤权和奖励冲正事务方法。
- Produces: 第 3.7 节调度、结果落库、重试、核验和人工退款方法。

- [ ] **Step 1: 写第一事务原子性失败注入测试**

逐个在撤身份、撤 RBAC、冲正奖励、更新 REVOKED 前注入错误，断言全部回滚且 Mock Provider 调用次数为零。

- [ ] **Step 2: 写四类 Provider 结果和重复调度失败测试**

明确断言 SUCCESS、TEMPORARY_FAILURE、UNSUPPORTED、UNKNOWN 的目标状态和调用次数。

- [ ] **Step 3: 写人工退款双人审批失败测试**

同人审批、金额不一致、错误哈希、重复审批必须失败。

- [ ] **Step 4: 运行目标测试并确认失败**

```powershell
cd backend-go
go test ./internal/httpserver -run 'TestOperationCenterRefundSaga.*Postgres' -count=1
```

- [ ] **Step 5: 实现退款准备事务和事务外调度**

严格执行第 3.7 节边界；Provider 调用代码不得位于 `BeginTx` 与 `Commit/Rollback` 之间。

- [ ] **Step 6: 实现四类结果、UNKNOWN 核验和人工登记**

UNKNOWN 普通 retry 必须返回领域错误；QueryRefund 或人工 verify 才能推进。

- [ ] **Step 7: 运行 Task 4 测试**

```powershell
cd backend-go
go test ./internal/httpserver -run 'TestOperationCenterRefundSaga.*Postgres' -count=1
```

期望：PASS。

- [ ] **Step 8: 第四批验收**

验收标准：不存在部分撤权、部分冲正或重复 Provider 退款；失败后不恢复 ACTIVE；人工退款证据完整。

建议提交说明：`feat(channel): implement operation center refund saga`。

### Task 5: HTTP、RBAC 和后台查询入口

**Files:**

- Create: `backend-go/internal/httpserver/operation_center_lifecycle_api.go`
- Create: `backend-go/internal/httpserver/operation_center_lifecycle_api_test.go`
- Modify: `backend-go/internal/httpserver/server.go`
- Modify: `backend-go/internal/httpserver/governance.go`

**Interfaces:**

- Consumes: Task 3 生命周期方法和 Task 4 退款 Saga 方法。
- Produces: 第 3.8 节后台 API。

- [ ] **Step 1: 写路由、输入、状态冲突和权限失败测试**

覆盖全部七个路由、幂等头、403、409 和不接受客户端金额。

- [ ] **Step 2: 运行 API 测试并确认失败**

```powershell
cd backend-go
go test ./internal/httpserver -run TestOperationCenterLifecycleAPI -count=1
```

- [ ] **Step 3: 实现处理器和路由**

处理器只解析命令和输出结果；金额、冻结期和规则版本由服务端读取。

- [ ] **Step 4: 增加 governance 权限映射**

使用第 3.8 节精确权限字符串；人工提交和审批权限分开。

- [ ] **Step 5: 运行 API 与现有 RBAC 测试**

```powershell
cd backend-go
go test ./internal/httpserver -run 'Test(OperationCenterLifecycleAPI|UserRBAC)' -count=1
```

期望：PASS。

- [ ] **Step 6: 第五批验收**

验收标准：接口和权限可用，但真实支付履约仍未调用新生命周期；当前业务流不变。

建议提交说明：`feat(admin): expose operation center review and refund APIs`。

### Task 6: 最后一批接入真实履约入口和微信虚拟退款适配

**Files:**

- Modify: `backend-go/internal/httpserver/postgres_store.go`
- Modify: `backend-go/internal/httpserver/payment_center_api.go`
- Create: `backend-go/internal/httpserver/wechat_virtual_refund_provider.go`
- Modify: `backend-go/internal/httpserver/wechat_virtual_entitlements.go`
- Modify: `backend-go/internal/httpserver/wechat_virtual_payment_postgres_test.go`
- Modify: `backend-go/internal/httpserver/commerce_service_test.go`

**Interfaces:**

- Consumes: Task 2 Provider 契约、Task 3 支付待审方法、Task 4 Saga。
- Produces: 运营中心订单支付后的真实 `REVIEW_REQUIRED` 行为和微信首个实际退款渠道。

- [ ] **Step 1: 写业务入口失败测试**

将现有运营中心履约期望改为支付后 `REVIEW_REQUIRED`，明确无提前 ACTIVE、RBAC 和奖励。

- [ ] **Step 2: 写微信 Provider 四结果映射和同键幂等测试**

模拟成功、临时错误、不支持、超时未知；断言业务层只看到统一 outcome。

- [ ] **Step 3: 运行目标测试并确认失败**

```powershell
cd backend-go
go test ./internal/httpserver -run 'Test(CommerceOperationCenter|WechatVirtualPaymentPostgresLifecycle)' -count=1
```

- [ ] **Step 4: 修改唯一履约入口**

仅在 `applyCommerceOrderFulfillmentForTx` 的 `OPERATION_CENTER_JOIN` 分支调用 `markOperationCenterPaymentReviewRequiredTx`。不得新增第二个支付回调履约入口。

- [ ] **Step 5: 注册微信虚拟退款 Provider**

`payment_center_api.go` 只负责 Provider 注册和解析；`wechat_virtual_refund_provider.go` 负责渠道请求与结果归一化。

- [ ] **Step 6: 接入微信退款通知核验**

`wechat_virtual_entitlements.go` 将已确认通知按退款号交给通用 `recordOperationCenterRefundResult`；不直接恢复或撤销运营中心权限。

- [ ] **Step 7: 运行支付、退款和重复回调测试**

```powershell
cd backend-go
go test ./internal/app/payment -count=1
go test ./internal/httpserver -run 'Test(CommerceOperationCenter|WechatVirtualPaymentPostgresLifecycle|OperationCenter)' -count=1
```

期望：PASS。

- [ ] **Step 8: 执行迁移和 HTTP 全量回归**

```powershell
cd backend-go
go test ./internal/app/channelrules -count=1
go test ./internal/httpserver -count=1
go test ./... -count=1
```

期望：全部 PASS。

- [ ] **Step 9: 核对运行开关**

断言数据库和测试配置仍为 `SHADOW`、`real_switch_enabled=false`、运营中心不在 Canary 白名单；不得修改生产规则版本。

- [ ] **Step 10: 第六批验收**

验收标准：运营中心支付真实进入待审；审核、奖励、退款、人工流程端到端通过；状态、资金、权限和幂等不变量通过；未启用真实白名单或比例放量。

建议提交说明：`feat(channel): connect operation center lifecycle to payment fulfillment`。

---

## 8. 最终交付证据

编码完成后提交以下证据，不以代码存在代替测试结果：

- 修改文件清单和每批提交说明。
- `schema.sql + 002...089` 隔离测试库全量迁移结果。
- 单元测试、PostgreSQL 集成测试、微信虚拟支付生命周期和 HTTP 全量测试结果。
- Mock Provider 四类结果及稳定幂等重试证据。
- 审核拒绝和激活后退款复用同一任务的数据库记录样例。
- FROZEN、AVAILABLE、SETTLED 三类冲正和 `recoverable_cents` 样例。
- 状态、资金、权限和重复退款不变量断言结果。
- 当前 Shadow/Canary 开关和运营中心未加入白名单的确认。
- 尚存风险、生产迁移前置条件和回滚只影响新请求的说明。

## 9. 停止点

本实施计划提交后停止。只有用户明确确认计划并要求进入编码，才从 Task 1 开始；不得跳过批次验收，不得提前修改 `applyCommerceOrderFulfillmentForTx`。
