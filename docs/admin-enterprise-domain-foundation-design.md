# 主控 SaaS 企业管理底层规范（技术方案草案）

> 状态：待评审；本轮不执行生产迁移，不切换现有运行时读写链路。
>
> 配套草案：
>
> - `database/drafts/043-enterprise-domain-foundation.draft.sql`
> - `backend-go/docs/openapi-admin-enterprise-domain-draft.yaml`

## 1. 结论

本方案不创建新的企业主表。企业身份继续以 `xz_tenants` 中 `tenant_type = 'ENTERPRISE'` 的记录为唯一企业根，组织、用户、成员、角色和权限继续复用：

- `xz_tenants`
- `xz_organizations`
- `xz_users`
- `xz_tenant_members`
- `xz_user_roles` / `xz_user_role_context`
- `roles` / `permissions` / `role_permissions`
- `xz_role_permissions`

底层改造采用“兼容层 + 版本化配置 + 领域命令”方式：

1. `xz_plans` 保留为现有套餐兼容主表，新增不可变套餐版本、商品绑定、权益值、赠送规则和模型计费策略；不再新增一套平行套餐主表。
2. `products`、`product_entitlements` 继续作为商品与权益目录；补齐版本、发布范围和审计字段。
3. `xz_tenant_certifications`、`xz_tenant_subscriptions` 继续承载认证与套餐状态；服务状态和风险状态分别建立一对一状态投影，避免继续复用 `xz_tenants.status` 表达多个业务含义。
4. 所有关键状态只能经领域服务执行状态转换。控制器只做鉴权、校验命令和返回结果，不允许直接 `UPDATE ... SET status = ...`。
5. 平台后台继续使用 `/api/v1/admin/*` 和 `PLATFORM` 权限域；企业成员端继续使用 `/api/v1/enterprise/*` 和 `TENANT` 权限域。即使角色代码同名，也不得跨域授权。
6. 金额字段统一为 `*_cents BIGINT`；算力统一为 `*_compute_units BIGINT`；Token 字段只出现在模型原始用量记录中，不作为余额、赠送物或兑换单位。

## 2. 仓库现状与差距

### 2.1 已有能力

| 能力 | 已有实现 | 复用结论 |
| --- | --- | --- |
| 企业根与隔离 | `xz_tenants`、`xz_organizations`、`xz_tenant_members` | 直接复用；所有企业查询必须带 `tenant_id` / `enterpriseId` |
| 企业认证 | `xz_tenant_certifications` | 复用记录表，补状态版本与转换日志 |
| 企业套餐 | `xz_tenant_subscriptions` -> `xz_plans` | 复用订阅和套餐，增加套餐版本快照 |
| 企业算力 | `xz_tenant_wallets`、`xz_tenant_point_transactions` | 复用账本，API 统一对外称 `computeUnit`；禁止 Token 余额别名 |
| 商品/权益 | `products`、`product_entitlements`、`plan_products` | 复用前两者；`plan_products` 仍关联旧 `membership_plans`，不可直接作为 `xz_plans` 运行时绑定 |
| 模型计费 | `model_pricing_rules`、`billing_rules`、`api_model_catalog` | 只做兼容输入；新增精确整数、可版本化的模型计费策略 |
| 用量与结算 | `xz_billing_events` | 继续作为算力/金额结算事件；新增独立原始模型用量记录 |
| 平台 RBAC | `roles`、`permissions`、`role_permissions`、`xz_role_permissions`、后台中间件 | 复用；权限增加显式 `PLATFORM` / `TENANT` 域 |
| 企业端 RBAC | `xz_user_roles`、企业上下文、`enterprise.*` 权限 | 复用；禁止后台中间件读取企业成员上下文作为平台授权 |
| 幂等 | `xz_admin_enterprise_requests.request_id`、支付事件唯一键 | 有局部实现；需要统一到带请求摘要和响应回放的幂等表 |
| 审计 | `xz_audit_logs`、`xz_tenant_audit_logs` | 复用；状态转换需同时写平台审计和企业范围审计 |

### 2.2 需要修正的现状

1. `canonicalBillingPlans()` 在代码中硬编码价格、赠送算力、有效期和权益，`ensureCanonicalBillingPlans()` 启动时会回写 `xz_plans`。这会覆盖后台配置，必须在切换阶段改成“仅首次种子”，数据库发布版本才是运行时事实源。
2. `adminPlan.TokenAmount`、订单 `token_grant_amount` 等字段把 Token 与算力/点数混用。新契约不再产生 Token 赠送；旧字段只保留兼容读取，迁移后由 `compute_unit_grant` 映射并逐步废弃。
3. `billing_rules.base_price/cost_price` 使用 `numeric`，业务结构又使用 `float64`，不满足金额精确到分。新策略使用整数金额和整数比率。
4. `plan_products` 外键指向旧 `membership_plans`，而当前企业订阅使用 `xz_plans`。新增 `xz_plan_product_bindings` 连接运行时套餐版本与已有商品，不修改旧表语义。
5. 认证、套餐和风险操作目前在 `MutateAdminEnterprise` 内直接更新状态；`risk-disable` 还直接改 `xz_tenants.status`。这些写入需迁移到领域服务。
6. 平台权限码使用 `enterprise:*`，企业端使用 `enterprise.*`，但数据库没有权限域字段；角色 `FINANCE` 等还可能同名。必须用请求域 + 授权上下文双重隔离，不能只依赖字符串格式。
7. `xz_admin_enterprise_requests` 只能拒绝重复 `request_id`，不能安全回放同请求结果，也没有请求摘要、有效期、失败状态和作用域。

## 3. 统一业务单位

| 数据 | 存储规范 | API 示例 | 禁止事项 |
| --- | --- | --- | --- |
| 金额 | `BIGINT`，字段后缀 `_cents`，人民币 1 元 = 100 分 | `priceCents: 29900` | `float`、`numeric` 参与业务结算、前端自行乘除 100 |
| 算力 | `BIGINT`，字段后缀 `_compute_units`，平台定义的整数最小单位 | `computeUnits: 40000` | 前端推算人民币/Token 与算力兑换关系 |
| Token | `BIGINT` 原始计数：输入、输出、缓存输入、推理、总量 | `inputTokens: 1350` | Token 余额、Token 赠送、把 Token 当算力或金额 |
| 倍率 | 两个正整数 `numerator/denominator` | `3/2` 表示 1.5 倍 | 浮点倍率参与结算 |
| 时间 | `TIMESTAMPTZ`，API 使用 RFC 3339 UTC | `2026-07-14T08:00:00Z` | 仅在前端计算有效期或到期状态 |

算力扣减公式由后端计费领域执行：

```text
计费数量 = 按策略单位归一后的原始用量
基础算力 = ceil(计费数量 / unit_size) * compute_units_per_unit
最终算力 = 按优先级应用命中的整数倍率与阶梯规则
```

所有舍入策略必须存储在策略版本中，默认 `CEILING`，不能由调用端选择。

## 4. 可配置商品、套餐、权益和模型计费

### 4.1 聚合关系

```text
products（已有商品目录）
  └─ product_entitlements（已有权益定义）
       └─ xz_plan_entitlement_values（套餐版本的权益值）

xz_plans（已有套餐兼容主表）
  └─ xz_plan_versions（不可变发布版本）
       ├─ xz_plan_product_bindings（商品绑定）
       ├─ xz_plan_entitlement_values（权益值、阈值、有效期）
       └─ xz_plan_grant_rules（算力赠送、周期、有效期）

api_model_catalog / ai_models（已有模型目录）
  └─ xz_model_billing_policies（计费策略版本）
       ├─ xz_model_billing_tiers（阶梯/阈值）
       └─ xz_model_billing_multipliers（参数倍率）
```

### 4.2 数据表定义

| 表 | 类型 | 关键字段 | 说明 |
| --- | --- | --- | --- |
| `products` | 复用并扩展 | `code`、`type`、`status`、`version`、`sale_scope`、`config` | 商品目录；`config` 只存非结算展示配置 |
| `product_entitlements` | 复用并扩展 | `product_id`、`code`、`unit`、`value_type`、`aggregation` | 权益定义；配额值不写在定义表 |
| `xz_plans` | 复用并扩展 | `code`、`catalog_version`、`currency_code`、`billing_cycle` | 当前套餐兼容投影，不承担历史版本追溯 |
| `xz_plan_versions` | 新增 | `plan_id`、`version`、`status`、`price_cents`、`validity_policy`、快照 | 发布后不可修改；修改必须生成下一版本 |
| `xz_plan_product_bindings` | 新增 | `plan_version_id`、`product_id`、`product_snapshot` | 连接 `xz_plans` 与已有 `products`，不复用旧 `plan_products` 外键 |
| `xz_plan_entitlement_values` | 新增 | `entitlement_id`、`quantity`、`value_json`、`threshold_config`、`validity_config` | 可配置配额、布尔/枚举权益、阈值和有效期 |
| `xz_plan_grant_rules` | 新增 | `resource_code`、`base_amount`、`bonus_amount`、`trigger`、`recurrence`、`validity_seconds` | 赠送规则；资源首期仅允许 `COMPUTE_UNIT` |
| `xz_model_billing_policies` | 新增 | 模型、能力、计费单位、单位大小、算力、金额、版本、有效期 | 计费主策略，所有金额和算力为整数 |
| `xz_model_billing_tiers` | 新增 | 数量区间、单位价格、单位算力 | 阶梯和阈值；区间左闭右开，`to_quantity` 为空表示无上限 |
| `xz_model_billing_multipliers` | 新增 | 维度、操作符、条件、整数倍率、优先级 | 可配置尺寸、质量、时长等倍率 |
| `xz_model_usage_records` | 新增 | 原始 Token 字段、模型、供应商请求、策略快照、结算结果 | Token 仅在此类原始用量表出现 |

### 4.3 发布与生效规则

- 商品、权益、套餐版本、计费策略均有 `DRAFT -> PUBLISHED -> RETIRED` 生命周期。
- 发布版本不可原地修改；修订必须复制为新版本。
- 订单和订阅必须保存 `plan_version_id`、价格、权益、赠送和有效期快照。
- 用量记录必须保存命中的 `billing_policy_id` 与策略快照；历史重放不得读取当前策略。
- `valid_from` / `valid_to` 由后端按数据库时间判断，前端只展示。
- 同一套餐只允许一个未退役的发布版本；同一模型、能力、计费单位在同一时间范围不得有重叠发布策略。
- 商品配置和模型策略发布属于关键写操作，必须填写原因、使用幂等键并写审计。

### 4.4 索引规范

| 索引 | 目的 |
| --- | --- |
| `ux_xz_plan_versions_current_published(plan_id) WHERE PUBLISHED AND retired_at IS NULL` | 一个套餐只能有一个当前发布版本 |
| `idx_xz_plan_versions_lookup(plan_id, status, version DESC)` | 套餐版本查询与回溯 |
| `idx_xz_plan_product_bindings_product(product_id, status, plan_version_id)` | 从商品反查套餐版本 |
| `idx_xz_plan_entitlement_values_entitlement(entitlement_id, status, plan_version_id)` | 权益影响范围查询 |
| `idx_xz_plan_grant_rules_effective(plan_version_id, resource_code, status, valid_from, valid_to)` | 赠送规则生效判断 |
| `idx_xz_model_billing_policies_effective(model_code, capability, billing_unit, status, valid_from, valid_to)` | 模型策略匹配 |
| `idx_xz_model_billing_tiers_range(policy_id, from_quantity, to_quantity)` | 阶梯阈值匹配 |
| `idx_xz_model_billing_multipliers_match(policy_id, status, priority, valid_from, valid_to)` | 倍率规则按优先级匹配 |
| `ux_xz_model_usage_provider_request(provider_code, provider_request_id, model_code)` | 上游请求用量防重 |
| `idx_xz_model_usage_tenant_time(tenant_id, occurred_at DESC, id DESC)` | 企业用量隔离与分页 |
| `idx_xz_tenant_state_transitions_scope(tenant_id, aggregate_type, created_at DESC, id DESC)` | 状态历史与审计查询 |
| `UNIQUE(permission_domain, actor_id, method, canonical_path, idempotency_key)` | 写操作幂等作用域 |

时间区间重叠（计费策略、赠送规则）不能只靠普通唯一索引完整表达，发布领域服务必须在 `SERIALIZABLE` 事务或模型级 advisory lock 中检查重叠；不在控制器中检查。

## 5. 四套状态机

### 5.1 通用执行规范

控制器调用命令服务，状态转换只能在同一数据库事务内完成：

```text
Controller
  -> 鉴权（权限域 + 权限码）
  -> IdempotencyService.Begin
  -> DomainService.Execute(command, expectedVersion)
  -> 状态行 FOR UPDATE
  -> 校验允许的 from -> to、业务守卫和 tenant_id
  -> 更新状态与 state_version
  -> 写 xz_tenant_state_transitions
  -> 写 xz_tenant_audit_logs + xz_audit_logs
  -> IdempotencyService.Complete（保存可回放响应）
```

控制器、通用 Store CRUD 和前端均不得直接传入任意 `status` 更新关键状态字段。数据库账号可进一步通过列级权限或触发器阻止绕过，但第一落地点是删除控制器中的直接状态 SQL。

### 5.2 企业认证状态机

当前状态保存在最新 `xz_tenant_certifications.status`，没有申请记录时由查询投影为 `UNVERIFIED`。

| 当前状态 | 命令 | 下一状态 | 守卫 | 副作用 |
| --- | --- | --- | --- | --- |
| `UNVERIFIED` | `SUBMIT` | `PENDING` | 企业管理员；资料完整；同企业无待审记录 | 创建申请、审计 |
| `REJECTED` | `RESUBMIT` | `PENDING` | 新版本资料；驳回原因已确认 | 创建新申请，旧记录不覆盖 |
| `APPROVED` | `SUBMIT_MATERIAL_CHANGE` | `PENDING` | 统一信用代码、主体名称等关键字段变化 | 保留原认证快照直到审核结束 |
| `PENDING` | `APPROVE` | `APPROVED` | 平台认证审核权限；不能审核本人提交 | 记录审核人与时间 |
| `PENDING` | `REJECT` | `REJECTED` | 驳回原因必填 | 记录原因并通知企业 |
| `APPROVED` | `REVOKE` | `REVOKED` | 超级管理员或风控审批；原因必填 | 触发服务策略重算 |
| `REVOKED` | `RESUBMIT` | `PENDING` | 企业重新提交完整资料 | 新申请版本 |

终态不是删除。认证资料只能追加版本，不能覆盖历史审核证据。

### 5.3 企业套餐状态机

持久状态保存在 `xz_tenant_subscriptions.status`。`EXPIRING_SOON` 是基于 `current_period_end` 和可配置阈值计算的展示状态，不写入数据库。

| 当前状态 | 命令 | 下一状态 | 守卫/说明 |
| --- | --- | --- | --- |
| 无订阅 | `START_TRIAL` | `TRIALING` | 套餐允许试用，企业未消费过同类试用 |
| 无订阅 / `EXPIRED` / `CANCELED` | `ACTIVATE` | `ACTIVE` | 已支付或人工审批通过；必须绑定已发布套餐版本 |
| `TRIALING` | `ACTIVATE` | `ACTIVE` | 购买、转正或人工审批 |
| `TRIALING` | `EXPIRE` | `EXPIRED` | 后端定时命令，数据库时间已到期 |
| `ACTIVE` | `CHANGE_PLAN` | `ACTIVE` | 生成新订阅版本/周期；保存前后快照，不原地改历史套餐版本 |
| `ACTIVE` | `ENTER_GRACE` | `GRACE` | 续费失败或合同宽限；宽限天数来自配置 |
| `GRACE` | `RENEW` | `ACTIVE` | 支付成功或审批通过 |
| `GRACE` | `EXPIRE` | `EXPIRED` | 宽限期结束 |
| `ACTIVE` / `GRACE` | `SUSPEND` | `SUSPENDED` | 套餐层暂停，不等同风险封禁 |
| `SUSPENDED` | `RESUME` | `ACTIVE` 或 `GRACE` | 恢复目标由暂停前状态快照决定 |
| `TRIALING` / `ACTIVE` / `GRACE` / `SUSPENDED` | `CANCEL` | `CANCELED` | 立即或周期末取消由命令参数决定 |

### 5.4 企业服务状态机

服务状态存放在 `xz_tenant_service_states`，不再使用 `xz_tenants.status` 同时表达企业档案和服务开关。

| 当前状态 | 命令 | 下一状态 | 守卫/说明 |
| --- | --- | --- | --- |
| 无记录 | `PROVISION` | `PROVISIONING` | 企业创建后由后端触发 |
| `PROVISIONING` | `ACTIVATE` | `ACTIVE` | 基础资源初始化完成 |
| `ACTIVE` | `PAUSE` | `PAUSED` | 平台服务权限；原因必填；可配置恢复时间 |
| `PAUSED` | `RESUME` | `ACTIVE` | 套餐有效且风险状态非 `BLOCKED` |
| `ACTIVE` / `PAUSED` | `DISABLE` | `DISABLED` | 高风险操作，二次确认/审批、原因必填 |
| `DISABLED` | `RESTORE` | `PAUSED` 或 `ACTIVE` | 根据套餐与风险重新计算目标状态 |
| `DISABLED` | `TERMINATE` | `TERMINATED` | 仅超级管理员；不可逆；不物理删除企业 |

有效服务状态由后端策略统一计算：

```text
effectiveService = serviceState + subscriptionState + riskState
```

只要风险为 `BLOCKED` 或套餐已失效，接口就不能仅因 `serviceState = ACTIVE` 放行。

### 5.5 企业风控状态机

当前状态存放在 `xz_tenant_risk_states`；`xz_admin_enterprise_risk_records` 继续作为兼容事件来源，后续迁移为风险案例/证据记录。

| 当前状态 | 命令 | 下一状态 | 守卫/说明 |
| --- | --- | --- | --- |
| 无记录 / `NORMAL` | `START_MONITORING` | `MONITORING` | 风险信号或人工观察；原因和证据引用必填 |
| `MONITORING` | `RESTRICT` | `RESTRICTED` | 指定限制范围，例如充值、模型、并发 |
| `NORMAL` / `MONITORING` / `RESTRICTED` | `BLOCK` | `BLOCKED` | 风控权限；高风险级别；自动触发服务 `DISABLE` 命令 |
| `RESTRICTED` | `RELAX` | `MONITORING` | 风险下降，保留观察 |
| `MONITORING` / `RESTRICTED` | `CLEAR` | `NORMAL` | 处置完成；结论必填 |
| `BLOCKED` | `UNBLOCK` | `MONITORING` | 双人审批或超级管理员；不会直接恢复服务 |

风控解除和服务恢复是两条独立命令。解除风险后必须再次执行服务恢复并重新校验套餐，禁止一个接口静默完成两种状态变更。

## 6. 权限域与矩阵

### 6.1 域隔离规则

| 项目 | 平台后台 | 企业成员端 |
| --- | --- | --- |
| 路由前缀 | `/api/v1/admin/*` | `/api/v1/enterprise/*` |
| 权限域 | `PLATFORM` | `TENANT` |
| 身份来源 | 平台会话 + 平台角色（当前兼容 `xz_users.role` / `user_roles`） | 企业上下文 + `xz_user_roles` / `xz_tenant_members` |
| 数据范围 | 按权限访问多个企业，但每次资源查询仍校验 `tenant_type='ENTERPRISE'` | 只能访问当前上下文 `tenant_id`，客户端不能指定其他企业 |
| 权限码风格 | 保留现有 `enterprise:*` | 保留现有 `enterprise.*` |

鉴权函数签名必须显式包含域：

```go
Authorize(actor, PermissionDomainPlatform, "enterprise:package:adjust", ResourceTenant(enterpriseID))
Authorize(actor, PermissionDomainTenant, "enterprise.billing.read", CurrentTenant())
```

仅检查 `role == FINANCE` 或仅检查权限字符串均不充分。平台 `FINANCE` 与企业 `FINANCE` 即使代码相同，也因授权上下文和权限域不同而不能互通。

### 6.2 平台权限码

现有企业管理权限码全部保留，并新增底层规范所需权限：

| 范围 | 权限码 |
| --- | --- |
| 企业 | `enterprise:list`、`enterprise:detail`、`enterprise:create`、`enterprise:update`、`enterprise:export` |
| 认证 | `enterprise:certification:review`、`enterprise:certification:revoke` |
| 成员 | `enterprise:member:view` |
| 套餐 | `enterprise:package:view`、`enterprise:package:adjust`、`enterprise:subscription:transition`、`enterprise:seat:adjust` |
| 算力财务 | `enterprise:compute:view`、`enterprise:compute:adjust`、`enterprise:transaction:view`、`enterprise:order:view` |
| AI | `enterprise:ai:view`、`enterprise:ai:configure`、`enterprise:employee:view`、`enterprise:knowledge:view` |
| 归属 | `enterprise:attribution:view`、`enterprise:attribution:change` |
| 服务/风控 | `enterprise:service:view`、`enterprise:service:transition`、`enterprise:risk:view`、`enterprise:risk:transition` |
| 审计 | `enterprise:audit:view` |
| 商品计费 | `billing:product:view`、`billing:product:manage`、`billing:plan:view`、`billing:plan:manage`、`billing:plan:publish`、`billing:model-policy:view`、`billing:model-policy:manage`、`billing:model-policy:publish` |

`enterprise:risk:disable` / `enterprise:risk:restore` 作为兼容别名保留一个版本，但新接口分别要求 `enterprise:risk:transition` 和 `enterprise:service:transition`。

### 6.3 平台六角色矩阵

图例：`R` 查看，`W` 编辑/发起，`A` 审核/发布，`-` 无权限。超级管理员仍需经过状态机、幂等和审计，不能绕过业务守卫。

| 能力 | 超级管理员 | 企业运营 | 认证审核员 | 财务 | 风控 | 客服 |
| --- | --- | --- | --- | --- | --- | --- |
| 企业列表/详情 | R/W | R/W | R | R | R | R |
| 创建企业 | W | W | - | - | - | - |
| 认证审核/撤销 | A | - | A（不可审本人） | - | R | R |
| 成员与组织统计 | R | R | - | R | R | R |
| 套餐/席位 | A | W | - | R | R | R |
| 算力调整/充值 | A | W（需审批阈值） | - | W/A | R | R |
| 订单/流水 | R | R | - | R | R | R |
| AI 能力配置 | A | W | - | - | R | R |
| 商品/套餐草稿 | A | W | - | R | - | R |
| 商品/套餐发布 | A | - | - | A（价格复核） | - | - |
| 模型计费策略 | A | W | - | R/A（金额复核） | - | R |
| 归属变更 | A | W（进入审批） | - | - | R | R |
| 服务暂停/恢复 | A | W（非风控原因） | - | - | A | R |
| 风险转换 | A | - | - | - | A | R |
| 审计日志 | R | R | R | R | R | 仅业务相关 R |

### 6.4 企业端权限码

企业端不开放平台审核、发布、跨企业、人工调账和风控处置权限：

- 已有并复用：`enterprise.overview.read`、`enterprise.organization.*`、`enterprise.member.*`、`enterprise.role.*`、`enterprise.billing.read`、`enterprise.audit.read`、`enterprise.settings.*`、`enterprise.certification.submit`。
- 新增只读细分：`enterprise.package.read`、`enterprise.compute.read`、`enterprise.usage.read`、`enterprise.service.read`、`enterprise.certification.read`。

| 企业端角色 | 概览 | 组织/成员 | 套餐/算力/用量 | 提交认证 | 企业审计 | 设置 |
| --- | --- | --- | --- | --- | --- | --- |
| `ENTERPRISE_ADMIN` | R | R/W | R | W | R | R/W |
| `AI_ADMIN` | R | R | R（不含金额明细） | - | - | R |
| `FINANCE` | R | R | R | - | R | - |
| `CUSTOMER_SERVICE` | R | R | R（汇总） | - | - | - |
| `ENTERPRISE_MEMBER` | R | R | 仅本人/汇总策略允许 | - | - | R |

## 7. API 契约规范

完整结构见 OpenAPI 草案。关键约束如下。

### 7.1 路由边界

平台后台：

- `/api/v1/admin/enterprises`
- `/api/v1/admin/enterprises/{enterpriseId}`
- `/api/v1/admin/enterprises/{enterpriseId}/certification-transitions`
- `/api/v1/admin/enterprises/{enterpriseId}/subscription-transitions`
- `/api/v1/admin/enterprises/{enterpriseId}/service-transitions`
- `/api/v1/admin/enterprises/{enterpriseId}/risk-transitions`
- `/api/v1/admin/billing/products`
- `/api/v1/admin/billing/plans`
- `/api/v1/admin/billing/model-policies`

企业成员端：

- `/api/v1/enterprise/overview`
- `/api/v1/enterprise/package`
- `/api/v1/enterprise/usage`
- `/api/v1/enterprise/certifications`
- `/api/v1/enterprise/certification-submissions`

主控企业管理契约同时保留以下资源能力：

| 能力 | 平台路由 |
| --- | --- |
| 导出 | `POST /api/v1/admin/enterprises/exports` |
| 成员/组织 | `GET /{enterpriseId}/members`、`GET /{enterpriseId}/organizations` |
| 席位 | `POST /{enterpriseId}/seat-adjustments` |
| 算力/充值/流水 | `GET /{enterpriseId}/compute`、`POST /{enterpriseId}/compute-adjustments`、`POST /{enterpriseId}/recharges`、`GET /{enterpriseId}/transactions` |
| 订单 | `GET /{enterpriseId}/orders` |
| AI 能力 | `GET /{enterpriseId}/ai-capabilities`、`PUT /{enterpriseId}/ai-capabilities/{capabilityCode}/configuration` |
| AI 员工/知识库统计 | `GET /{enterpriseId}/ai-employees`、`GET /{enterpriseId}/knowledge-bases` |
| 归属/关系 | `GET /{enterpriseId}/attribution`、`POST /{enterpriseId}/attribution-changes`、`GET /{enterpriseId}/relationships` |
| 风险/审计 | `GET /{enterpriseId}/risk-records`、`GET /{enterpriseId}/audit-logs` |

充值请求只提交已发布充值商品版本、支付金额（分）与支付引用；具体赠送算力由后端商品快照计算。客户端不得直接提交兑换倍率或到账算力。

企业成员端的 `tenant_id` 只能从认证上下文解析，禁止请求参数传入。平台端路径中的 `enterpriseId` 必须在 Repository 层同时校验 `xz_tenants.id = ? AND tenant_type = 'ENTERPRISE'`。

### 7.2 写操作通则

所有 `POST` / `PUT` / `PATCH` / `DELETE`：

- 必须带 `Idempotency-Key`，建议 UUID/ULID，最大 128 字符。
- 后端以 `permission_domain + actor_id + method + canonical_path + key` 建立唯一作用域。
- 同键同请求摘要：返回首次响应与 `Idempotent-Replayed: true`。
- 同键不同请求摘要：返回 `409 IDEMPOTENCY_KEY_REUSED`。
- 状态命令必须带 `expectedVersion`；版本冲突返回 `409 STATE_VERSION_CONFLICT`。
- 关键操作必须带 `reason`；发布、封禁、归属、套餐和算力调整不得为空。
- 正在处理返回 `202` 或 `409 IDEMPOTENCY_IN_PROGRESS`，不得重复执行副作用。

### 7.3 状态命令响应

```json
{
  "operationId": "op_01J...",
  "enterpriseId": "tenant_enterprise_001",
  "aggregateType": "SERVICE",
  "command": "PAUSE",
  "fromState": "ACTIVE",
  "toState": "PAUSED",
  "stateVersion": 8,
  "status": "SUCCEEDED",
  "auditId": "audit_01J...",
  "processedAt": "2026-07-14T08:00:00Z"
}
```

### 7.4 错误码

| HTTP | 业务码 | 场景 |
| --- | --- | --- |
| 400 | `INVALID_ARGUMENT` | 格式、单位或必填项错误 |
| 401 | `UNAUTHENTICATED` | 未登录或会话失效 |
| 403 | `PERMISSION_DENIED` | 权限域、权限码或数据范围不允许 |
| 404 | `ENTERPRISE_NOT_FOUND` | 企业不存在或不在允许范围；避免泄露跨租户资源 |
| 409 | `STATE_VERSION_CONFLICT` | 乐观锁版本冲突 |
| 409 | `INVALID_STATE_TRANSITION` | 当前状态不能执行命令 |
| 409 | `IDEMPOTENCY_KEY_REUSED` | 同键不同请求 |
| 422 | `BUSINESS_RULE_VIOLATION` | 守卫失败，例如套餐无效、风险未解除 |
| 429 | `RATE_LIMITED` | 频率限制 |

## 8. 幂等、审计和隔离

### 8.1 幂等

新增 `xz_idempotency_records`，保存请求摘要、处理状态、响应码、响应体、资源和过期时间。`xz_admin_enterprise_requests` 在兼容期双写/回填，待全部入口切换后只读归档。

幂等记录与业务写必须处于同一事务；异步命令先持久化 `PROCESSING`，消费者以相同记录防重。

### 8.2 审计

下列操作至少双写 `xz_audit_logs` 和 `xz_tenant_audit_logs`：

- 认证通过、驳回、撤销
- 套餐启用、变更、暂停、恢复、取消
- 算力赠送、扣减、充值、人工调整
- 商品、套餐版本和模型计费策略发布/退役
- 服务暂停、禁用、恢复、终止
- 风险监控、限制、封禁、解除
- 客户、代理商或运营中心归属变更

审计元数据必须包含：`requestId`、`idempotencyKey`、权限域、权限码、操作人、企业、原因、前后快照、状态版本、审批单（如有）、结果和失败码。敏感认证文件只记录对象键/摘要，不记录正文或临时访问 URL。

### 8.3 租户隔离

- Repository 方法必须接收 `enterpriseID`，禁止先按资源 ID 查询后再在内存判断企业。
- 企业级表使用组合唯一键/索引 `(tenant_id, id)`；可关联资源优先使用组合外键。
- 企业端接口永远忽略或拒绝请求中的 `tenantId`。
- 平台接口不能因拥有列表权限而自动拥有详情或写权限。
- 后续可在 PostgreSQL 启用 RLS；在此之前仍需服务端 SQL 条件和测试双重保证。

## 9. Migration 草案说明

草案位于 `database/drafts`，不会被现有 `database/migrations/*.sql` 自动迁移流程匹配。本轮禁止复制到正式迁移目录或在生产执行。

正式迁移建议拆成四个可回滚阶段：

1. **Additive DDL**：新增列、版本表、状态投影、幂等和转换日志，不改变现有读写。
2. **Backfill**：把 `xz_plans`、硬编码套餐、现有认证/订阅/风险记录生成版本快照和初始状态；先做数据质量报告。
3. **Dual write / shadow read**：领域服务双写旧投影和新表，对比状态与账本；禁止控制器直写。
4. **Cutover**：关闭 `ensureCanonicalBillingPlans()` 的启动覆盖，启用数据库发布版本；移除旧 Token 赠送语义和旧 action 路由写入。

上线前必须完成：

- 备份与恢复演练；
- 当前脏数据、重复发布版本、无企业归属订单和非法状态预检；
- 回滚 SQL 与开关；
- 领域服务单测、并发幂等测试、跨租户负向测试；
- OpenAPI 兼容检查和后台/企业端权限矩阵测试。

## 10. 后续实现边界

本轮只提交规范和草案，没有执行迁移，也没有修改控制器或运行时状态。下一实现轮按以下顺序进行：

1. 增加领域服务接口和状态机纯函数测试。
2. 增加统一幂等服务、权限域类型和审计写入器。
3. 把认证、套餐、服务、风控新命令路由接到领域服务，保留旧路由兼容适配器。
4. 实施并验证 additive migration；完成回填脚本的 dry-run 报告后再申请执行。
5. 切换商品/套餐/计费策略的数据库发布版本，并将硬编码目录降级为种子数据。
