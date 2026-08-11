$ErrorActionPreference = 'Stop'

$container = 'gift-points-103-gates'
$repoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$migrationPath = Join-Path $repoRoot 'database/migrations/103-personal-gift-point-expiry.sql'
$rollbackPath = Join-Path $repoRoot 'database/rollbacks/103-personal-gift-point-expiry.down.sql'
$fixturePath = Join-Path $repoRoot 'database/tests/103-personal-gift-point-expiry.sql'
$oldMigrationCommit = '1ccb1dca'
$oldMigrationPath = 'database/migrations/103-personal-gift-point-expiry.sql'

$baseSchema = @'
CREATE TABLE xz_users (id text PRIMARY KEY);
CREATE TABLE xz_point_accounts (
  id text PRIMARY KEY,
  user_id text NOT NULL,
  available bigint NOT NULL DEFAULT 0,
  frozen bigint NOT NULL DEFAULT 0,
  raw jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE TABLE xz_wallet_ledger (
  id bigserial PRIMARY KEY,
  account_id text NOT NULL,
  entry_type text NOT NULL,
  points bigint NOT NULL
);
'@

function Remove-TestContainer {
  $previousPreference = $ErrorActionPreference
  $ErrorActionPreference = 'SilentlyContinue'
  docker rm -f $container 2>&1 | Out-Null
  $ErrorActionPreference = $previousPreference
}

function Invoke-PsqlSql {
  param(
    [Parameter(Mandatory = $true)][string]$Sql,
    [int]$ExpectedExitCode = 0
  )

  $Sql | docker exec -i $container psql -U verify -d verify -v ON_ERROR_STOP=1
  $exitCode = $LASTEXITCODE
  if ($exitCode -ne $ExpectedExitCode) {
    throw "psql exit code $exitCode, expected $ExpectedExitCode"
  }
  return $exitCode
}

function Invoke-PsqlFileRaw {
  param([Parameter(Mandatory = $true)][string]$Path)

  Get-Content -LiteralPath $Path -Raw -Encoding utf8 |
    docker exec -i $container psql -U verify -d verify -v ON_ERROR_STOP=1 |
    Out-Host
  return $LASTEXITCODE
}

function Invoke-OldMigration {
  git show "${oldMigrationCommit}:$oldMigrationPath" |
    docker exec -i $container psql -U verify -d verify -v ON_ERROR_STOP=1
  if ($LASTEXITCODE -ne 0) {
    throw "old migration $oldMigrationCommit failed"
  }
}

function Query-Scalar {
  param([Parameter(Mandatory = $true)][string]$Sql)

  $value = docker exec $container psql -U verify -d verify -tAc $Sql
  if ($LASTEXITCODE -ne 0) {
    throw "query failed: $Sql"
  }
  return ($value | Out-String).Trim()
}

function Reset-Database {
  Invoke-PsqlSql 'DROP SCHEMA public CASCADE; CREATE SCHEMA public;'
  Invoke-PsqlSql $baseSchema
}

function Assert-Equal {
  param(
    [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Actual,
    [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Expected,
    [Parameter(Mandatory = $true)][string]$Label
  )

  if ($Actual -ne $Expected) {
    throw "${Label}: expected [$Expected], got [$Actual]"
  }
}

function Run-InvalidGiftUpgradeCase {
  param(
    [Parameter(Mandatory = $true)][string]$Name,
    [Parameter(Mandatory = $true)][string]$SetupSql
  )

  Reset-Database
  Invoke-OldMigration
  Invoke-PsqlSql @"
INSERT INTO xz_point_accounts(id, user_id, available, frozen)
VALUES ('gift_account', 'gift_user', 0, 0);
$SetupSql
"@

  $exitCode = Invoke-PsqlFileRaw $migrationPath
  if ($exitCode -eq 0) {
    throw "RED: invalid gift upgrade case [$Name] was accepted"
  }

  Assert-Equal (Query-Scalar "SELECT count(*) FROM xz_personal_point_lots WHERE id LIKE 'invalid_gift_%'") '1' "$Name legacy row preserved after rollback"
  Assert-Equal (Query-Scalar "SELECT count(*) FROM information_schema.columns WHERE table_name='xz_personal_point_reservation_allocations' AND column_name='account_id'") '0' "$Name upgrade rolled back"
  Write-Output "PASS invalid-gift upgrade fail-closed: $Name"
}

function Run-ExpiredBalanceProbe {
  Reset-Database
  $exitCode = Invoke-PsqlFileRaw $migrationPath
  if ($exitCode -ne 0) {
    throw 'current migration failed before EXPIRED status probe'
  }
  $exitCode = Invoke-PsqlFileRaw $fixturePath
  if ($exitCode -ne 0) {
    throw 'EXPIRED status/amount fixture failed'
  }
  Write-Output 'PASS EXPIRED lot requires zero reserved balance'
}

function Run-ExpiredUpgradeCase {
  Reset-Database
  Invoke-OldMigration
  Invoke-PsqlSql @'
INSERT INTO xz_point_accounts(id, user_id, available, frozen)
VALUES ('expired_account', 'expired_user', 0, 0);
INSERT INTO xz_personal_point_lots(
  id, account_id, user_id, source_type, original_points, available_points,
  reserved_points, expired_points, granted_at, idempotency_key, status
) VALUES (
  'invalid_gift_expired_reserved', 'expired_account', 'expired_user', 'MANUAL',
  2, 0, 1, 1, now(), 'invalid_gift_expired_reserved', 'EXPIRED'
);
'@
  Assert-Equal (Query-Scalar "SELECT count(*) FROM xz_personal_point_lots WHERE id='invalid_gift_expired_reserved'") '1' 'old migration accepted EXPIRED with reserved points'
  $exitCode = Invoke-PsqlFileRaw $migrationPath
  if ($exitCode -eq 0) {
    throw 'RED: old EXPIRED+reserved lot was accepted by the upgrade'
  }
  Assert-Equal (Query-Scalar "SELECT count(*) FROM xz_personal_point_lots WHERE id='invalid_gift_expired_reserved'") '1' 'EXPIRED upgrade rollback preserves source row'
  Write-Output 'PASS old EXPIRED+reserved upgrade is rejected'
}

function Run-FrozenGate {
  Reset-Database
  Invoke-PsqlSql "INSERT INTO xz_point_accounts(id, user_id, available, frozen) VALUES ('frozen_account', 'frozen_user', 0, 3);"
  $exitCode = Invoke-PsqlFileRaw $migrationPath
  if ($exitCode -eq 0) {
    throw 'RED: frozen balance without ledger evidence was accepted'
  }
  Assert-Equal (Query-Scalar "SELECT coalesce(to_regclass('public.xz_personal_point_lots')::text, '')") '' 'frozen gate transaction rollback'
  Write-Output 'PASS frozen evidence fail-closed'
}

function Run-RollbackLiveGate {
  Reset-Database
  Invoke-PsqlSql "INSERT INTO xz_point_accounts(id, user_id, available, frozen) VALUES ('live_account', 'live_user', 5, 0);"
  $exitCode = Invoke-PsqlFileRaw $migrationPath
  if ($exitCode -ne 0) {
    throw 'current migration failed before live rollback gate'
  }
  $exitCode = Invoke-PsqlFileRaw $rollbackPath
  if ($exitCode -eq 0) {
    throw 'RED: live economic history rollback unexpectedly succeeded'
  }
  Assert-Equal (Query-Scalar 'SELECT count(*) FROM xz_personal_point_lots') '1' 'live rollback preserves lot history'
  Write-Output 'PASS rollback refuses live economic history'
}

function Run-RollbackEmptyAndIndexGate {
  Reset-Database
  Invoke-PsqlSql 'CREATE UNIQUE INDEX ux_xz_point_accounts_id_user ON xz_point_accounts(id, user_id);'
  $exitCode = Invoke-PsqlFileRaw $migrationPath
  if ($exitCode -ne 0) {
    throw 'current migration failed before empty rollback gate'
  }
  $exitCode = Invoke-PsqlFileRaw $rollbackPath
  if ($exitCode -ne 0) {
    throw 'empty rollback failed'
  }
  Assert-Equal (Query-Scalar "SELECT coalesce(to_regclass('public.ux_xz_point_accounts_id_user')::text, '')") 'ux_xz_point_accounts_id_user' 'pre-existing same-name index preserved'
  Assert-Equal (Query-Scalar "SELECT count(*) FROM pg_class WHERE relname IN ('xz_point_expiry_policy_versions','xz_personal_point_lots','xz_personal_point_reservations','xz_personal_point_reservation_allocations','xz_personal_point_lot_movements') AND relkind='r'") '0' 'empty rollback removes migration tables'
  Write-Output 'PASS empty rollback and same-name index preservation'
}

Remove-TestContainer
docker run --rm -d --name $container -e POSTGRES_PASSWORD=verify -e POSTGRES_USER=verify -e POSTGRES_DB=verify postgres:16-alpine | Out-Null

try {
  for ($attempt = 0; $attempt -lt 60; $attempt++) {
    docker exec $container pg_isready -U verify -d verify 2>$null | Out-Null
    if ($LASTEXITCODE -eq 0) {
      break
    }
    Start-Sleep -Seconds 1
  }
  if ($LASTEXITCODE -ne 0) {
    throw 'postgres container did not become ready'
  }

  Run-InvalidGiftUpgradeCase 'policy-null' @'
INSERT INTO xz_personal_point_lots(
  id, account_id, user_id, source_type, original_points, available_points,
  granted_at, idempotency_key, status
) VALUES (
  'invalid_gift_policy_null', 'gift_account', 'gift_user', 'ADMIN_GIFT', 1, 1,
  now(), 'invalid_gift_policy_null', 'ACTIVE'
);
'@

  Run-InvalidGiftUpgradeCase 'snapshot-drift' @'
INSERT INTO xz_personal_point_lots(
  id, account_id, user_id, source_type, original_points, available_points,
  granted_at, expires_at, policy_version_id, policy_snapshot, idempotency_key, status
) VALUES (
  'invalid_gift_snapshot_drift', 'gift_account', 'gift_user', 'ADMIN_GIFT', 1, 1,
  now(), now() + interval '3 months', 'point_expiry_policy_v1',
  '{"version":99,"enabled":true,"duration_value":3,"duration_unit":"CALENDAR_MONTH","time_zone":"Asia/Shanghai"}'::jsonb,
  'invalid_gift_snapshot_drift', 'ACTIVE'
);
'@

  Run-InvalidGiftUpgradeCase 'enabled-without-expiry' @'
INSERT INTO xz_personal_point_lots(
  id, account_id, user_id, source_type, original_points, available_points,
  granted_at, policy_version_id, policy_snapshot, idempotency_key, status
) VALUES (
  'invalid_gift_enabled_no_expiry', 'gift_account', 'gift_user', 'ADMIN_GIFT', 1, 1,
  now(), 'point_expiry_policy_v1',
  '{"version":1,"enabled":true,"duration_value":3,"duration_unit":"CALENDAR_MONTH","time_zone":"Asia/Shanghai"}'::jsonb,
  'invalid_gift_enabled_no_expiry', 'ACTIVE'
);
'@

  Run-InvalidGiftUpgradeCase 'disabled-with-expiry' @'
INSERT INTO xz_point_expiry_policy_versions(
  id, version, revision, enabled, duration_value, duration_unit, time_zone,
  source_types, effective_from, status, created_by, change_reason, metadata
) VALUES (
  'point_expiry_policy_v2_disabled', 2, 1, FALSE, 0, 'CALENDAR_MONTH', 'Asia/Shanghai',
  '["REGISTRATION_GIFT","ACTIVITY_GIFT","ADMIN_GIFT"]'::jsonb, now(), 'PUBLISHED',
  'gate-test', 'disabled policy', '{}'::jsonb
);
INSERT INTO xz_personal_point_lots(
  id, account_id, user_id, source_type, original_points, available_points,
  granted_at, expires_at, policy_version_id, policy_snapshot, idempotency_key, status
) VALUES (
  'invalid_gift_disabled_expiry', 'gift_account', 'gift_user', 'ADMIN_GIFT', 1, 1,
  now(), now() + interval '3 months', 'point_expiry_policy_v2_disabled',
  '{"version":2,"enabled":false,"duration_value":0,"duration_unit":"CALENDAR_MONTH","time_zone":"Asia/Shanghai"}'::jsonb,
  'invalid_gift_disabled_expiry', 'ACTIVE'
);
'@

  Run-ExpiredBalanceProbe
  Run-ExpiredUpgradeCase
  Run-FrozenGate
  Run-RollbackLiveGate
  Run-RollbackEmptyAndIndexGate
  Write-Output 'PASS migration 103 gate suite'
}
finally {
  Remove-TestContainer
}
