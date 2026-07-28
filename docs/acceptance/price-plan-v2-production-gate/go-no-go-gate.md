# 会员/代理价格方案 V2 上线前 GO/NO-GO 门禁

当前总状态：`NO-GO FOR PRODUCTION ENABLEMENT`

当前只允许继续：冻结候选制品、生产只读预检、生产备份隔离演练、人工核对和沙箱验收准备。任何生产迁移、部署、微信操作或开关启用都需要新的变更批准。

准备期实际值必须保持：

```text
PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED=false
PRICE_PLAN_TEST_ENTRY_ENABLED=false
SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED=false
```

## Gate A：代码与增量 schema 候选发布，三开关关闭

| 门禁 | GO 证据 | NO-GO |
|---|---|---|
| Release commit | clean checkout、40 位 commit、审批文件清单、第三阶段文件不在范围 | 当前脏工作区直接构建；迁移仍 untracked；混入未审批文件 |
| 迁移制品 | 097–100 各一个文件；从 release `git archive` 冻结 SHA256；DBA 收到同一份文件 | 编号冲突、hash 漂移、工作树 hash 冒充 release hash |
| 镜像 | registry `repository@sha256:...`、平台、image ID、revision label、SBOM/签名 | 只有 `prod/latest` tag；生产机重新 build；revision 不匹配 |
| Compose | 三开关显式注入且默认 false；离线覆盖渲染通过 | 宿主机变量未进入容器或默认 true |
| 生产只读预检 | `dba-readonly-preflight.sql` 完整输出符合判定表 | 脚本失败、身份不明、任一硬阻断非零 |
| 隔离迁移 | 生产一致性备份上 097→100、重放、约束验证和恢复演练通过 | 未演练、锁预算未知、历史基线变化 |
| 备份 | 备份 SHA256、恢复日志、第二副本恢复成功 | 只有“备份成功”无恢复证明 |
| RBAC | `XIANZHI_ENFORCE_RBAC=true`；真实角色按审批矩阵授权 | 普通 ADMIN/客服/代理/运营中心获敏感权限 |
| 回滚制品 | 上一版兼容镜像 digest、V2 兼容后端、回滚责任人 | 只能回滚到不认识 V2 快照的后端 |
| 测试 | 测试报告绑定同一 release commit | 沿用更早工作区报告或未处理失败项 |
| Secrets | Secret 注入；仓库、镜像、日志扫描通过 | AppKey/sessionKey/数据库密码进入制品或日志 |

Gate A 通过只允许提出“部署代码/迁移且三开关保持 false”的变更单，不自动进入 Gate B。

## Gate B：会员/代理普通 V2 创建

| 门禁 | GO 证据 | NO-GO |
|---|---|---|
| Gate A | 已由应用、DBA、安全和支付负责人签字 | Gate A 任一项未完成 |
| 启用顺序 | 先开启 `SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED`，确认健康后再开 creation | creation 先开，或履约关闭 |
| Pricing health | `blockedIssueCount=0`；仅允许审批过的 DISABLED 提示 | 任一未批准 BLOCKED issue |
| V132 | affected tenant 数为 0 | 任一 V132/CANARY 实切租户 |
| Gift points | 所有候选 enabled/default 方案 `giftPoints=0` | 任一 `giftPoints>0` |
| 权益版本 | MEMBER/AGENT 各一个有效 ACTIVE 版本 | 无 ACTIVE、多个 ACTIVE 或过期 |
| 默认方案 | 每个 channel/environment/currency 仅一个有效公开默认 | 多默认、TEST/隐藏/非公开默认 |
| 微信商品 | 正常价正式/沙箱商品已发布并双人实时核对 | 未确认、过期、跨环境或 productId 不明 |
| 价值守恒 | quote、方案、绑定、本地商品、微信后台价格完全一致 | 任差 1 分 |
| 运行配置 | AppID/offerId/mode/AppKey/environment 证明属于同一套 | 仅凭本地 health 推测微信配置 |
| 沙箱真机 | MEMBER/AGENT NORMAL 成功；回调、查单、幂等和 V1 回归通过 | 只跑旧 productCode 脚本或缺真机证据 |
| 流量范围 | 有明确全局开关影响评估和批准 | 把全局开关误当租户/用户 canary |
| 第三阶段边界 | 退款、补偿、人工补发对 V2 的运行限制和人工事故 SOP 已书面批准 | 需要这些流程但仍读取当前配置或硬编码；无处置方案 |

由于 creation 是全局开关，不具备租户级 canary。没有明确全量影响评估时 Gate B 保持 `NO-GO`。

## Gate C：隐藏 TEST 入口

Gate C 必须在 Gate B 通过后单独审批，且最后开启。

| 门禁 | GO 证据 | NO-GO |
|---|---|---|
| TEST 商品 | MEMBER/AGENT 各自独立的 SANDBOX ¥1 productId，价格 100 分 | 复用正常价/996 元 productId，或正式/沙箱交叉 |
| TEST 方案 | hidden、non-default、non-PUBLIC、ACTIVE、enabled | 可见、默认或 PUBLIC |
| 白名单 | 有效期、状态、revision、操作人和审计完整 | 过期、重复有效或无审计 |
| 普通入口隔离 | 白名单用户普通入口仍返回 NORMAL | 普通入口能自动选 TEST |
| 专用入口 | 登录 + 开关 + 白名单三重校验 | 依赖前端隐藏作为授权 |
| 下单复核 | quote 后白名单失效时明确拒绝，不自动换正式价 | 资格失效仍成交或自动改价 |
| 真机 | MEMBER/AGENT TEST 均为 goodsPrice=100，代理不再比较 99600 | 未完成真机、价格或 productId 不符 |

生产环境默认不启用 TEST；沙箱通过不授权生产 TEST 商品或入口。

## 当前已确认状态

| 项目 | 当前状态 | 说明 |
|---|---|---|
| Compose 三开关注入 | PASS（本地静态/离线渲染） | 待进入冻结 release commit |
| 开关实际启用 | PASS/保持关闭 | 本轮未开启任何开关 |
| Release commit | NO-GO | 当前工作区大量 modified/untracked，097–100 未跟踪 |
| 镜像 RepoDigest | NO-GO | 本轮未构建、未 push；不得从当前工作区构建 |
| 097–100 最终 SHA | NO-GO | 必须从未来 release commit 的 git archive 冻结 |
| 生产只读预检 | NOT RUN | 本轮未连接生产数据库 |
| 隔离迁移/恢复演练 | NOT RUN | 本轮只提供命令 |
| 微信后台人工核对 | NOT RUN | 本轮未操作微信后台 |
| 沙箱真机 V2 quote | NOT RUN | 本轮未发起支付 |
| 真实角色授权 | NOT VERIFIED | 需生产只读结果和真实账号正反向验收 |
| V132/giftPoints | NO-GO UNTIL VERIFIED | V132 必须为 0；giftPoints 必须为 0 |
| 第三阶段流程 | OUT OF SCOPE | 未进入退款、补偿、人工补发统一履约 |

## 当前实现风险需修复或书面豁免

- 当前部署脚本会跟随分支并在目标机 `up -d --build`，不能证明使用冻结 commit/digest；digest 发布路径未批准前 `NO-GO`。
- 运行时没有自动证明本地 good.offerId/mode 与环境级 offer/AppKey 属于同一微信配置，必须双人核对。
- creation 关闭后提交既有 quoteId 不会创建 V2 订单，但可能落入 V1 返回通用 400，错误码不稳定。
- 2F 已知全量测试失败和导航门禁债务必须修复或由上线审批人书面接受，不能静默忽略。
- 本轮不建设第三阶段。若真实生产支持要求退款、补偿、人工补发按 V2 快照执行，则 Gate B 继续 `NO-GO`。

## 事故回退顺序

```text
PRICE_PLAN_TEST_ENTRY_ENABLED=false
→ PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED=false
→ SNAPSHOT_V2_MEMBER_AGENT_FULFILLMENT_ENABLED 保持 true
```

- 存在任何 V2 待支付、已支付未履约、补偿中订单时，不得关闭 V2 履约。
- 保持微信回调、官方查单、对应环境 AppKey 和 NotifyToken 可用。
- 已签发 quote 不改价；creation 关闭后冻结消费并等待过期。
- 后端只能回滚到仍支持 V2 快照履约的 digest。
- 097–100 无 down migration；不得现场手工 DROP。优先保留增量结构并前滚修复。
- PITR 只用于经过审批的灾难恢复，并需评估恢复点之后的其他业务数据损失。

## 最终签字

应用负责人：__________  DBA：__________  微信支付负责人：__________

安全负责人：__________  业务负责人：__________  发布负责人：__________

最终决定：`GO / NO-GO`  决定时间：__________  变更单号：__________

