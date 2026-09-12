# RC Scoring Rule Draft

- **版本**：Draft v0.1
- **状态**：`NOT_APPLICABLE` 作为正式评分规则（待项目 owner / release owner 批准）
- **用途**：定义未来 RC 分数如何计算；不回填当前 `RC_SCORE`，不改变当前 blocker 状态。
- **当前分数**：`NOT_COMPUTABLE_WITH_CURRENT_REPOSITORY_RULES`

## 1. 评分原则

1. RC 分数只对已登记的 criterion inventory 计算，不对任意代码行数、测试数量或主观完成度计算。
2. 每个 criterion 必须绑定当前 release 的 commit、运行环境或外部证据。
3. 代码实现、单元测试、mock 和依赖容器健康只能证明对应范围；不能自动关闭生产证据 criterion。
4. `BLOCKED_EXTERNAL` 和 `BLOCKED_ENVIRONMENT` 的得分均为 0，并保留阻断原因。
5. `NOT_APPLICABLE` 只有在 release owner 书面批准后才从分母排除；不能用来隐藏缺失证据。
6. 未登记权重、版本或 criterion inventory 不完整时，分数必须输出 `NOT_COMPUTABLE`，禁止人工猜测数值。

## 2. 最终状态定义

| 状态 | 定义 | 计分 | 是否允许 Production Ready |
|---|---|---:|---|
| `VERIFIED` | 当前 release 的要求证据完整、可复核、无越界推断 | 该 criterion 全权重 | 只有 P0/P1 全部为 `VERIFIED` 才允许 |
| `BLOCKED_EXTERNAL` | 需要生产凭据、第三方平台、审批或外部 owner 操作 | 0 | 不允许 |
| `BLOCKED_ENVIRONMENT` | 需要的运行服务、fixture、队列、存储或恢复环境不存在/不可核对 | 0 | 不允许 |
| `NOT_APPLICABLE` | 经批准确认该 criterion 不属于本 release 范围 | 不计入分母 | 不得用于规避 P0/P1 |

## 3. 建议权重模型

总分固定为 100，避免不同审计轮次随意改变比例：

- P0 criteria：总权重 40；同一优先级内平均分配。
- P1 criteria：总权重 50；同一优先级内平均分配。
- P2 criteria：总权重 10；同一优先级内平均分配。

某一优先级没有登记 criterion 时，该优先级权重应转入“未定义”，不能静默重新分配。正式规则批准前，必须先提交完整的 criterion inventory 和每项权重。

## 4. 计算公式

```text
active_weight = Σ(weight of VERIFIED + BLOCKED criteria)
verified_weight = Σ(weight of VERIFIED criteria)

RC_SCORE = 100 * verified_weight / active_weight
```

如果存在 `NOT_APPLICABLE`：

```text
active_weight = Σ(weight of VERIFIED + BLOCKED criteria)
```

`NOT_APPLICABLE` 不进入分母，但必须有批准记录和理由。

## 5. Release gate 覆盖规则

数值分数不能覆盖硬门禁：

```text
if any P0 != VERIFIED: PRODUCTION_READY=NO
if any P1 != VERIFIED: PRODUCTION_READY=NO
if RELEASE_IDENTITY != VERIFIED: PRODUCTION_READY=NO
if PRODUCTION_EVIDENCE_COMPLETE != VERIFIED: PRODUCTION_READY=NO
```

因此高分也不能绕过 Secret、支付、备份、Worker 或 release identity 的阻断。

## 6. 当前冻结态映射

当前冻结报告可以确定状态和 blocker 数，但仓库没有本规则版本批准记录，也没有完整 criterion inventory/权重。因此当前只能输出：

```text
RC_SCORE=NOT_COMPUTABLE_WITH_CURRENT_REPOSITORY_RULES
P0_BLOCKERS=1
P1_BLOCKERS=9
PRODUCTION_READY=NO
```

不能把历史 `RC_SCORE=55` 当作本规则计算结果，也不能在没有新增 VERIFIED 证据时推导 72、80 或其他数字。

## 7. 正式启用前清单

- [ ] 项目 owner 批准 Draft v0.1 或提交修订版。
- [ ] 建立带 ID、优先级、权重、证据路径和 release commit 的 criterion inventory。
- [ ] 明确 P1 中 Payment 双项、PPTX、Image、API/Worker 等是否为独立 criterion。
- [ ] 为 `NOT_APPLICABLE` 建立批准人和审计附件字段。
- [ ] 在下一次 RC 审计中按版本化规则计算一次，并保留中间明细。
- [ ] 批准前继续使用 `NOT_COMPUTABLE_WITH_CURRENT_REPOSITORY_RULES`，不输出未经批准的数值分数。
