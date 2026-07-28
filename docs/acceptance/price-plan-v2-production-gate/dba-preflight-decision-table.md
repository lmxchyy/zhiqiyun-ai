# DBA 生产只读预检判定表

配套脚本：`dba-readonly-preflight.sql`。该脚本只能由只读账号通过 `psql` 执行，不包含 DDL/DML。

建议调用方式：

```powershell
$PSNativeCommandUseErrorActionPreference = $true
$evidence = 'price-plan-v2-production-preflight-<UTC timestamp>.log'
psql 'service=xianzhi_prod_readonly' `
  -X `
  -v ON_ERROR_STOP=1 `
  -f 'dba-readonly-preflight.sql' 2>&1 |
  Tee-Object -FilePath $evidence
if ($LASTEXITCODE -ne 0) {
  throw "Production read-only preflight failed: exit=$LASTEXITCODE"
}
```

`PGSERVICE`/`.pgpass` 由 DBA 管理；不得把生产密码放入命令行、脚本或日志。

## 判定表

| 区段 | GO 条件 | NO-GO/处理 |
|---|---|---|
| A 数据库身份 | 数据库、主机、端口、schema、账号均与审批单一致；`transaction_read_only=on` | 任一身份不明确或只读状态不是 `on`，立即停止 |
| B 长事务/锁 | 无待锁；无未评估的 5 分钟以上事务 | 清理或等待业务事务结束，重新选择迁移窗口 |
| C 前置 schema | 返回 0 行 | 任一缺表/缺列即停止；不得让 097–100 猜测修复旧 schema |
| D/D2 097 footprint | 首次迁移前六表 `0/6` 且订单字段 `0/11`；已迁移后分别 `6/0`、`11/0` | 任一部分存在表示部分迁移或同名漂移，硬阻断 |
| D3/D4 marker 冲突 | 首次迁移返回 0 行 | 097 约束名被其他表/schema 占用会导致静默跳过，硬阻断 |
| E 重复套餐 code | 返回 0 行 | 098 唯一索引会失败；只报告，不自动改数据 |
| F 历史 code 告警 | 对拟接入 V2 的会员/代理目标返回 0 行 | 历史兼容编码可保留；空值/格式错误套餐不得新接入 V2 |
| G 生产容量估算 | 记录估算行数和表大小，用于选择同规模隔离副本与锁预算 | 未取得容量证据，不能批准迁移窗口；精确 count/sum 只在隔离副本执行 |
| H/I 097 一致性 | 每个 `blocker_count=0` | 任一非零停止后续迁移或开关启用 |
| J 098 微信确认 | 每个 `blocker_count=0` | 人工确认缺字段、快照漂移、过期或商品不可用均阻断 |
| K 099 价格方案 | 每个 `blocker_count=0` | 默认、TEST、currency、audience、code、giftPoints 任一违规均阻断 |
| L V132 | 返回 0 行 | 任一 V132/CANARY affected tenant 阻断会员/代理 V2 创建 |
| M 100 白名单/quote/审计 | 每个 `blocker_count=0` | 重复有效白名单、quote pin、归属或审计违规均阻断 |
| N identity 索引 | 恰好 1 行；`xz_price_plan_user_whitelist/btree/unique/valid/ready/3/3/false/false/id,price_plan_id,user_id` | 无行、重复、列序或属性不一致均阻断 100 |
| O 真实权限 | `SUPER_ADMIN` 七项完整；其他角色与审批单逐项一致 | 普通 ADMIN/客服/代理/运营中心存在未审批 pricing 权限即阻断 |
| P/P2 约束状态 | 迁移前 marker 不存在；完整迁移后 19 个名称、表归属和定义与 release 一致，P2 返回 0 行 | 同名约束位于错误表/schema、缺失、重复或定义漂移均阻断 |
| Q V2 库存 | 首次迁移应为零；已有 V2 数据则必须形成履约/回滚清单 | 存在未完成 V2 订单时不得关闭 V2 履约或回滚不兼容后端 |
| R 表体积 | 已记录并用于同规模锁耗时演练 | 缺少体积和锁预算，不能批准迁移窗口 |

## NOT VALID 判定

迁移完成后允许下列约束暂时显示 `convalidated=false`，但开启
`PRICE_PLAN_MEMBER_AGENT_CREATION_ENABLED` 前必须：

1. 对应违规查询全部为零。
2. 在同规模隔离副本记录每个 `VALIDATE CONSTRAINT` 的耗时和锁影响。
3. 另行批准生产验证窗口。
4. 生产验证后再次只读确认 `convalidated=true`。

共 16 个：

- 097：4 个订单外键/快照检查。
- 099：6 个价格方案检查。
- 100：6 个白名单、quote ownership 和审计检查。

## 总结论

- 脚本退出码非 0：`NO-GO`。
- 任一硬阻断查询非零：`NO-GO`。
- 结果不完整、被截断或无法证明使用只读账号：`NO-GO`。
- 所有查询符合预期，只表示“只读预检通过”，不等于迁移、微信或上线 GO。

DBA：__________  复核人：__________  执行时间：__________  证据文件 SHA256：__________
