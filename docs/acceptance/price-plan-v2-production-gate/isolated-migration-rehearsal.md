# 生产备份隔离迁移演练手册

本手册中的恢复、迁移和约束验证命令只允许在隔离 PostgreSQL 16 使用。不得把生产 service/DSN 填入演练变量，不得在本轮执行。

## 1. 输入制品

- 经 DBA 校验的生产备份及 SHA256。
- 从冻结 release commit 的 `git archive` 解出的 097–100 原文件。
- `release-freeze-runbook.md` 生成的迁移 SHA256 manifest。
- 本目录的 `dba-readonly-preflight.sql`。
- 独立的 rehearsal PostgreSQL 16 主机、账号和空数据库。

## 2. 隔离身份保护

建议由 DBA 在密码不进入命令行的前提下配置：

```text
PGSERVICE=xianzhi_priceplan_rehearsal
数据库名必须匹配：*_rehearsal_YYYYMMDDHHMM
主机不得等于任何生产数据库 IP/域名
```

演练开始时执行：

```powershell
$PSNativeCommandUseErrorActionPreference = $true
$rehearsalService = 'service=xianzhi_priceplan_rehearsal'
$identity = psql $rehearsalService -X -Atc "select current_database(), inet_server_addr(), current_user, current_setting('server_version')"
if ($LASTEXITCODE -ne 0) { throw 'Cannot identify rehearsal database' }
if ($identity -notmatch '_rehearsal_[0-9]{12}\|') {
  throw "Unsafe rehearsal database identity: $identity"
}
$identity
```

目标数据库不是全新空库或身份不符合规则时，停止并让 DBA 重新提供目标；本手册不包含自动删除数据库命令。

## 3. 校验备份和 release bundle

```powershell
$backupFile = '<approved-backup-file>'
$releaseRoot = '<extracted-git-archive-root>'
$evidenceRoot = '<isolated-evidence-directory>'

Get-FileHash -Algorithm SHA256 -LiteralPath $backupFile

$migrations = @(
  '097-member-agent-price-plan-v2.sql',
  '098-price-plan-admin-governance.sql',
  '099-price-plan-default-switch.sql',
  '100-price-plan-test-whitelist-audit.sql'
)

$migrations | ForEach-Object {
  $path = Join-Path $releaseRoot "database\migrations\$_"
  if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
    throw "Missing frozen migration: $path"
  }
  Get-FileHash -Algorithm SHA256 -LiteralPath $path
}
```

所有 hash 必须与 release manifest 完全相同。

## 4. 检查备份并恢复

自定义格式备份：

```powershell
pg_restore --list $backupFile | Tee-Object (Join-Path $evidenceRoot 'backup-contents.txt')
if ($LASTEXITCODE -ne 0) { throw 'Backup listing failed' }

pg_restore `
  --exit-on-error `
  --no-owner `
  --no-privileges `
  --dbname=$rehearsalService `
  $backupFile 2>&1 |
  Tee-Object (Join-Path $evidenceRoot 'restore.log')
if ($LASTEXITCODE -ne 0) { throw 'Backup restore failed' }
```

纯 SQL 备份改用：

```powershell
psql $rehearsalService -X -v ON_ERROR_STOP=1 -f $backupFile 2>&1 |
  Tee-Object (Join-Path $evidenceRoot 'restore.log')
if ($LASTEXITCODE -ne 0) { throw 'SQL restore failed' }
```

恢复后再次打印数据库身份并保存 PostgreSQL 小版本、数据库大小、恢复耗时。

## 5. 迁移前只读预检

```powershell
$preflight = Join-Path $releaseRoot 'docs\acceptance\price-plan-v2-production-gate\dba-readonly-preflight.sql'
psql $rehearsalService -X -v ON_ERROR_STOP=1 -f $preflight 2>&1 |
  Tee-Object (Join-Path $evidenceRoot 'preflight-before.log')
if ($LASTEXITCODE -ne 0) { throw 'Pre-migration preflight failed' }
```

按照 `dba-preflight-decision-table.md` 判定。六张 097 表部分存在、重复套餐 code 或前置列缺失时停止。

精确历史基线只在隔离副本执行：

```powershell
$baselineQuery = @'
select 'xz_plans' as table_name, count(*) as row_count,
       coalesce(sum(price_cents),0) as amount_total
from xz_plans
union all
select 'xz_orders', count(*), coalesce(sum(amount_cents),0)
from xz_orders
union all
select 'xz_users', count(*), 0
from xz_users
union all
select 'xz_audit_logs', count(*), 0
from xz_audit_logs
union all
select 'xz_role_permissions', count(*), 0
from xz_role_permissions
order by table_name;
'@

psql $rehearsalService -X -v ON_ERROR_STOP=1 -c $baselineQuery 2>&1 |
  Tee-Object (Join-Path $evidenceRoot 'exact-baseline-before.txt')
if ($LASTEXITCODE -ne 0) { throw 'Exact baseline failed' }
```

## 6. 逐文件执行 097–100

```powershell
foreach ($name in $migrations) {
  $path = Join-Path $releaseRoot "database\migrations\$name"
  $log = Join-Path $evidenceRoot "$name.log"
  $startedAt = Get-Date

  psql $rehearsalService -X -v ON_ERROR_STOP=1 -f $path 2>&1 |
    Tee-Object -FilePath $log
  if ($LASTEXITCODE -ne 0) {
    throw "Migration failed and later files must not run: $name"
  }

  $elapsed = (Get-Date) - $startedAt
  "migration=$name elapsed=$elapsed" |
    Tee-Object -FilePath (Join-Path $evidenceRoot 'migration-durations.txt') -Append

  psql $rehearsalService -X -v ON_ERROR_STOP=1 -f $preflight 2>&1 |
    Tee-Object -FilePath (Join-Path $evidenceRoot "$name-postflight.log")
  if ($LASTEXITCODE -ne 0) { throw "Postflight failed: $name" }
}
```

全部迁移后再次执行 `$baselineQuery`，保存为 `exact-baseline-after.txt`。除 `xz_role_permissions` 最多新增 7 条 `SUPER_ADMIN` pricing 权限外，历史表的行数和金额必须保持一致。

不要给上述调用增加 `--single-transaction`；每个 SQL 文件已经有自己的 `BEGIN/COMMIT`。

## 7. 逐步检查

- 097：六张表、订单快照列、4 个 NOT VALID 约束存在；订单数和金额基线不变。
- 098：code 唯一、单 ACTIVE 权益版本、人工确认检查和治理触发器存在；普通角色没有自动获权。
- 099：currency/audience、默认唯一索引、价格方案和 V2 订单不可变触发器存在。
- 100：旧白名单全局唯一约束被 ACTIVE 部分唯一索引替代；identity 索引定义精确；复合 ownership FK、审计和 quote pin 触发器存在。

特别记录 100 的锁耗时：它会在 `xz_audit_logs` 创建多个普通、非 `CONCURRENTLY` 索引，锁保持到文件提交。

## 8. 重放演练

在同一个隔离副本再次按 097→100 顺序执行，验证当前 release bundle 的重放行为。重放成功不代表可以在生产重复执行；必须记录第二次各文件耗时和 catalog 是否变化。

## 9. 隔离库约束验证

以下命令会修改隔离库 catalog，只能在 rehearsal 执行：

```sql
ALTER TABLE xz_orders VALIDATE CONSTRAINT fk_xz_orders_plan_version_097;
ALTER TABLE xz_orders VALIDATE CONSTRAINT fk_xz_orders_price_plan_097;
ALTER TABLE xz_orders VALIDATE CONSTRAINT fk_xz_orders_price_quote_097;
ALTER TABLE xz_orders VALIDATE CONSTRAINT ck_xz_orders_snapshot_v2_097;

ALTER TABLE xz_price_plans VALIDATE CONSTRAINT ck_xz_price_plans_currency_099;
ALTER TABLE xz_price_plans VALIDATE CONSTRAINT ck_xz_price_plans_audience_099;
ALTER TABLE xz_price_plans VALIDATE CONSTRAINT ck_xz_price_plans_audience_rule_099;
ALTER TABLE xz_price_plans VALIDATE CONSTRAINT ck_xz_price_plans_code_format_099;
ALTER TABLE xz_price_plans VALIDATE CONSTRAINT ck_xz_price_plans_test_scope_099;
ALTER TABLE xz_price_plans VALIDATE CONSTRAINT ck_xz_price_plans_default_state_099;

ALTER TABLE xz_order_price_quotes VALIDATE CONSTRAINT ck_xz_order_price_quotes_whitelist_pin_100;
ALTER TABLE xz_price_plan_user_whitelist VALIDATE CONSTRAINT ck_xz_price_plan_whitelist_lifecycle_100;
ALTER TABLE xz_price_plan_user_whitelist VALIDATE CONSTRAINT ck_xz_price_plan_whitelist_enabled_100;
ALTER TABLE xz_order_price_quotes VALIDATE CONSTRAINT fk_xz_order_price_quotes_whitelist_100;
ALTER TABLE xz_audit_logs VALIDATE CONSTRAINT ck_xz_audit_logs_pricing_result_100;
ALTER TABLE xz_audit_logs VALIDATE CONSTRAINT ck_xz_audit_logs_pricing_required_100;
```

逐条执行并记录耗时。任何失败均为生产 `NO-GO`，不得在生产现场补数据。

## 10. 代码和 PostgreSQL 测试

在 `backend-go` 目录执行：

```powershell
go test ./internal/httpserver -run 'TestPricePlanPhase1Migration097|TestPricePlanPhase2AMigration098|TestPricePlanPhase2DMigration099|TestPricePlanPhase2EMigration100NumberIsUnique' -count=1

$env:XIANZHI_TEST_DATABASE_URL='<isolated-test-database-url>'
$env:XIANZHI_APPLY_TEST_MIGRATION_100='true'
go test ./internal/httpserver -run 'Test(PricePlanPhase2AGovernancePostgres|PricePlanPhase2DGovernancePostgres|PricePlanPhase2EMigration100PostgresGovernance|PricePlanV2Postgres|ManagedMemberAgentPlanCannotFallBack|PriceQuoteRejectsBindingIdentityDrift)' -count=1
```

测试 URL 只能指向可销毁的隔离测试库，不能指向刚恢复的证据库或生产库。

## 11. 恢复演练

由 DBA 再提供一个全新、空的第二隔离数据库，重复第 2–5 步并核对原始基线。不得通过删除第一数据库来模拟恢复。

恢复演练证据至少包含：备份 SHA256、两个隔离库身份、恢复日志、迁移日志、约束验证耗时、锁等待、前后基线和最终判定。

演练负责人：__________  DBA：__________  复核人：__________  日期：__________
