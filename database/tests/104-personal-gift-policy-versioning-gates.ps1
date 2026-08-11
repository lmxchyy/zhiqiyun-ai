$ErrorActionPreference = 'Stop'

$container = "gift-points-104-policy-gates-$PID"
$repoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$migrationDir = Join-Path $repoRoot 'database/migrations'
$schemaPath = Join-Path $repoRoot 'database/schema.sql'
$migrationPath = Join-Path $migrationDir '104-personal-gift-policy-versioning.sql'
$rollbackPath = Join-Path $repoRoot 'database/rollbacks/104-personal-gift-policy-versioning.down.sql'
$fixturePath = Join-Path $repoRoot 'database/tests/104-personal-gift-policy-versioning.sql'

function Remove-TestContainer {
  $old = $ErrorActionPreference
  $ErrorActionPreference = 'SilentlyContinue'
  docker rm -f $container 2>&1 | Out-Null
  $ErrorActionPreference = $old
}

function Invoke-PsqlText {
  param(
    [Parameter(Mandatory = $true)][string]$Sql,
    [int]$ExpectedExitCode = 0
  )

  $old = $ErrorActionPreference
  $ErrorActionPreference = 'Continue'
  $Sql | docker exec -i $container psql -q -U verify -d verify -v ON_ERROR_STOP=1 2>&1 | Out-Host
  $exitCode = $LASTEXITCODE
  $ErrorActionPreference = $old
  if ($exitCode -ne $ExpectedExitCode) {
    throw "psql exit code $exitCode, expected $ExpectedExitCode"
  }
  return $exitCode
}

function Invoke-PsqlFile {
  param([Parameter(Mandatory = $true)][string]$Path)

  $old = $ErrorActionPreference
  $ErrorActionPreference = 'Continue'
  Get-Content -LiteralPath $Path -Raw -Encoding utf8 |
    docker exec -i $container psql -q -U verify -d verify -v ON_ERROR_STOP=1 2>&1 |
    Out-Host
  $exitCode = $LASTEXITCODE
  $ErrorActionPreference = $old
  if ($exitCode -ne 0) {
    throw "psql file failed: $Path (exit $exitCode)"
  }
}

function Query-Scalar {
  param([Parameter(Mandatory = $true)][string]$Sql)

  $old = $ErrorActionPreference
  $ErrorActionPreference = 'Continue'
  $value = docker exec $container psql -q -U verify -d verify -tAc $Sql
  $exitCode = $LASTEXITCODE
  $ErrorActionPreference = $old
  if ($exitCode -ne 0) {
    throw "query failed: $Sql"
  }
  return ($value | Out-String).Trim()
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

function Start-PublisherJob {
  param([Parameter(Mandatory = $true)][string]$Sql)

  return Start-Job -ScriptBlock {
    param($ContainerName, $CommandText)
    $ErrorActionPreference = 'Continue'
    $CommandText | docker exec -i $ContainerName psql -q -U verify -d verify -v ON_ERROR_STOP=1 2>&1
    $code = $LASTEXITCODE
    Write-Output ("EXIT=" + $code)
  } -ArgumentList $container, $Sql
}

Remove-TestContainer
$old = $ErrorActionPreference
$ErrorActionPreference = 'Continue'
docker run --rm -d --name $container -e POSTGRES_PASSWORD=verify -e POSTGRES_USER=verify -e POSTGRES_DB=verify pgvector/pgvector:pg16 2>&1 | Out-Host
$runCode = $LASTEXITCODE
$ErrorActionPreference = $old
if ($runCode -ne 0) {
  throw 'pgvector container failed to start'
}

try {
  $ready = $false
  $readyChecks = 0
  for ($attempt = 0; $attempt -lt 120; $attempt++) {
    docker exec $container pg_isready -U verify -d verify 2>$null | Out-Null
    if ($LASTEXITCODE -eq 0) {
      $readyChecks++
      if ($readyChecks -ge 3) {
        $ready = $true
        break
      }
    } else {
      $readyChecks = 0
    }
    Start-Sleep -Milliseconds 500
  }
  if (-not $ready) {
    throw 'pgvector container did not become ready'
  }
  # pgvector's entrypoint briefly starts a bootstrap server and shuts it down
  # before starting the final postmaster; the consecutive checks above avoid
  # racing that handoff.
  Start-Sleep -Seconds 1

  Invoke-PsqlFile $schemaPath
  $migrations = Get-ChildItem -LiteralPath $migrationDir -Filter '*.sql' | Sort-Object Name
  foreach ($migration in $migrations) {
    Invoke-PsqlFile $migration.FullName
  }
  Assert-Equal (Query-Scalar "SELECT count(*) FROM xz_point_expiry_policy_versions WHERE status='PUBLISHED'") '1' 'full rehearsal initial published policy'
  Write-Output ("PASS full rehearsal through migration 104 (files=$($migrations.Count))")

  Invoke-PsqlFile $fixturePath
  Write-Output 'PASS 104 policy mutation fixture'

  # A closure without a replacement must fail at transaction commit and leave
  # the current PUBLISHED version intact. Migration 103/104 before the fix
  # accepts this transaction, so this probe is intentionally RED for TDD.
  Invoke-PsqlText @'
BEGIN;
UPDATE xz_point_expiry_policy_versions
SET status = 'ARCHIVED', effective_to = effective_from + interval '1 hour', updated_at = now()
WHERE id = 'point_expiry_policy_v1';
COMMIT;
'@ 3
  Assert-Equal (Query-Scalar "SELECT status FROM xz_point_expiry_policy_versions WHERE id='point_expiry_policy_v1'") 'PUBLISHED' 'closure-only commit rolls back'
  Assert-Equal (Query-Scalar "SELECT count(*) FROM xz_point_expiry_policy_versions WHERE status='PUBLISHED'") '1' 'closure-only commit leaves one PUBLISHED'
  Write-Output 'PASS deferred current-PUBLISHED closure gate'

  # Empty rollback restores the strict 103 trigger while v1 is untouched.
  Invoke-PsqlFile $rollbackPath
  Assert-Equal (Query-Scalar "SELECT count(*) FROM pg_trigger WHERE tgrelid='xz_point_expiry_policy_versions'::regclass AND tgname='trg_xz_point_expiry_policy_versions_immutable' AND NOT tgisinternal") '1' 'empty rollback trigger exists'
  Invoke-PsqlText @'
BEGIN;
UPDATE xz_point_expiry_policy_versions
SET status = 'ARCHIVED', effective_to = effective_from + interval '1 hour', updated_at = now()
WHERE id = 'point_expiry_policy_v1';
'@ 3
  Assert-Equal (Query-Scalar "SELECT status FROM xz_point_expiry_policy_versions WHERE id='point_expiry_policy_v1'") 'PUBLISHED' 'empty rollback preserves v1'
  Write-Output 'PASS empty rollback restores migration 103 strict trigger'

  # Reapply 104 before the concurrency and live-history gates.
  Invoke-PsqlFile $migrationPath

  # Create two DRAFT candidates while v1 remains current. Two separate
  # transactions then close v1 and race to promote them; the advisory lock
  # permits at most one publisher while the deferred trigger requires each
  # successful transaction to finish with one current PUBLISHED row.
  Invoke-PsqlText @'
BEGIN;
INSERT INTO xz_point_expiry_policy_versions(
  id, version, revision, enabled, duration_value, duration_unit, time_zone,
  source_types, effective_from, status, created_by, change_reason, metadata
) VALUES
  ('concurrent_policy_a', 2, 2, TRUE, 3, 'CALENDAR_MONTH', 'Asia/Shanghai',
   '["REGISTRATION_GIFT"]'::jsonb, now(), 'DRAFT', 'verify:104', 'concurrency A', '{}'::jsonb),
  ('concurrent_policy_b', 3, 3, TRUE, 3, 'CALENDAR_MONTH', 'Asia/Shanghai',
   '["REGISTRATION_GIFT"]'::jsonb, now(), 'DRAFT', 'verify:104', 'concurrency B', '{}'::jsonb);
COMMIT;
'@

  $publisherSql = @'
BEGIN;
SELECT pg_advisory_xact_lock(hashtextextended('xz_point_expiry_policy_versions:published', 0));
SELECT pg_sleep(1);
UPDATE xz_point_expiry_policy_versions
SET status = 'ARCHIVED', effective_to = effective_from + interval '1 hour', updated_at = now()
WHERE id = 'point_expiry_policy_v1';
UPDATE xz_point_expiry_policy_versions
SET status = 'PUBLISHED', updated_at = now()
WHERE id = '__POLICY_ID__';
COMMIT;
'@
  $jobA = Start-PublisherJob ($publisherSql.Replace('__POLICY_ID__', 'concurrent_policy_a'))
  $jobB = Start-PublisherJob ($publisherSql.Replace('__POLICY_ID__', 'concurrent_policy_b'))
  Wait-Job -Job $jobA, $jobB -Timeout 45 | Out-Null
  if ($jobA.State -ne 'Completed' -or $jobB.State -ne 'Completed') {
    Stop-Job -Job $jobA, $jobB -ErrorAction SilentlyContinue
    throw 'concurrent publisher jobs did not complete'
  }
  $outA = @(Receive-Job -Job $jobA)
  $outB = @(Receive-Job -Job $jobB)
  $exitA = [int]((@($outA | Where-Object { "$_" -match '^EXIT=' }) | Select-Object -Last 1) -replace '^EXIT=', '')
  $exitB = [int]((@($outB | Where-Object { "$_" -match '^EXIT=' }) | Select-Object -Last 1) -replace '^EXIT=', '')
  Remove-Job -Job $jobA, $jobB -Force
  if (($exitA -eq 0) -eq ($exitB -eq 0)) {
    throw "concurrent publisher result invalid: exitA=$exitA exitB=$exitB"
  }
  Assert-Equal (Query-Scalar "SELECT count(*) FROM xz_point_expiry_policy_versions WHERE status='PUBLISHED' AND id LIKE 'concurrent_policy_%'") '1' 'concurrent publishers max one PUBLISHED'
  Assert-Equal (Query-Scalar "SELECT count(*) FROM xz_point_expiry_policy_versions WHERE status='DRAFT' AND id LIKE 'concurrent_policy_%'") '1' 'losing publisher remains DRAFT'
  Write-Output "PASS concurrent publishers serialized (exitA=$exitA exitB=$exitB)"

  # New archived/published history makes rollback fail closed, and the 104
  # close path remains active after the refused rollback.
  Invoke-PsqlText (Get-Content -LiteralPath $rollbackPath -Raw -Encoding utf8) 3
  $triggerDef = Query-Scalar "SELECT pg_get_triggerdef(oid) FROM pg_trigger WHERE tgrelid='xz_point_expiry_policy_versions'::regclass AND tgname='trg_xz_point_expiry_policy_versions_immutable'"
  if ($triggerDef -notmatch 'INSERT') {
    throw 'live rollback gate changed the trigger unexpectedly'
  }
  Invoke-PsqlText @'
BEGIN;
UPDATE xz_point_expiry_policy_versions
SET status = 'ARCHIVED', effective_to = effective_from + interval '1 hour', updated_at = now()
WHERE id = (SELECT id FROM xz_point_expiry_policy_versions WHERE status='PUBLISHED' ORDER BY version DESC LIMIT 1);
ROLLBACK;
'@
  Write-Output 'PASS live-history rollback gate preserves 104 close behavior'

  # A close and replacement committed in one transaction is the supported
  # version transition and must leave exactly one current PUBLISHED row.
  Invoke-PsqlText @'
BEGIN;
UPDATE xz_point_expiry_policy_versions
SET status = 'ARCHIVED', effective_to = effective_from + interval '1 hour', updated_at = now()
WHERE status = 'PUBLISHED';
INSERT INTO xz_point_expiry_policy_versions(
  id, version, revision, enabled, duration_value, duration_unit, time_zone,
  source_types, effective_from, status, created_by, change_reason, metadata
)
SELECT
  'verify_policy_commit_v4', max(version) + 1, max(revision) + 1, TRUE, 3,
  'CALENDAR_MONTH', 'Asia/Shanghai', '["REGISTRATION_GIFT"]'::jsonb,
  now() + interval '1 hour', 'PUBLISHED', 'verify:104',
  'committed same-transaction replacement', '{}'::jsonb
FROM xz_point_expiry_policy_versions;
COMMIT;
'@
  Assert-Equal (Query-Scalar "SELECT count(*) FROM xz_point_expiry_policy_versions WHERE status='PUBLISHED'") '1' 'committed replacement leaves one PUBLISHED'
  Assert-Equal (Query-Scalar "SELECT status FROM xz_point_expiry_policy_versions WHERE id='verify_policy_commit_v4'") 'PUBLISHED' 'committed replacement exists'
  Write-Output 'PASS committed same-transaction closure and replacement'
  Write-Output 'PASS migration 104 gate suite'
}
finally {
  Remove-TestContainer
}
