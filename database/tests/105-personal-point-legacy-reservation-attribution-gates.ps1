$ErrorActionPreference = 'Stop'

$container = "gift-points-105-attribution-gates-$PID"
$repoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$migration103Path = Join-Path $repoRoot 'database/migrations/103-personal-gift-point-expiry.sql'
$migration104Path = Join-Path $repoRoot 'database/migrations/104-personal-gift-policy-versioning.sql'
$migration105Path = Join-Path $repoRoot 'database/migrations/105-personal-point-legacy-reservation-attribution.sql'
$rollback105Path = Join-Path $repoRoot 'database/rollbacks/105-personal-point-legacy-reservation-attribution.down.sql'
$fixture105Path = Join-Path $repoRoot 'database/tests/105-personal-point-legacy-reservation-attribution.sql'

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
  id text PRIMARY KEY,
  account_id text NOT NULL,
  user_id text,
  tenant_id text,
  task_id text,
  billing_event_id text,
  entry_type text NOT NULL,
  points numeric(18,6) NOT NULL,
  available_before numeric(18,6) NOT NULL,
  available_after numeric(18,6) NOT NULL,
  frozen_before numeric(18,6) NOT NULL,
  frozen_after numeric(18,6) NOT NULL,
  idempotency_key text NOT NULL UNIQUE,
  reference_type text NOT NULL DEFAULT '',
  reference_id text NOT NULL DEFAULT '',
  remark text NOT NULL DEFAULT '',
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (entry_type IN ('RECHARGE','GRANT','RESERVE','CAPTURE','RELEASE','REFUND','ADJUSTMENT','EXPIRE')),
  CHECK (points >= 0),
  CHECK (available_before >= 0 AND available_after >= 0),
  CHECK (frozen_before >= 0 AND frozen_after >= 0),
  CHECK (
    (entry_type = 'RESERVE' AND available_after = available_before - points AND frozen_after = frozen_before + points) OR
    (entry_type = 'CAPTURE' AND available_after = available_before AND frozen_after = frozen_before - points) OR
    (entry_type = 'RELEASE' AND available_after = available_before + points AND frozen_after = frozen_before - points) OR
    (entry_type = 'REFUND' AND available_after = available_before + points AND frozen_after = frozen_before) OR
    (entry_type IN ('RECHARGE','GRANT') AND available_after = available_before + points AND frozen_after = frozen_before) OR
    (entry_type = 'ADJUSTMENT' AND available_after IN (available_before - points, available_before + points) AND frozen_after = frozen_before) OR
    (entry_type = 'EXPIRE' AND available_after = available_before - points AND frozen_after = frozen_before)
  )
);
CREATE TABLE xz_generation_tasks (
  id text PRIMARY KEY,
  user_id text,
  tenant_id text,
  organization_id text,
  billing_account_type text NOT NULL DEFAULT 'PERSONAL',
  billing_account_id text,
  module_code text,
  type text,
  model text,
  billing_type text,
  status text,
  progress integer NOT NULL DEFAULT 0,
  point_cost bigint NOT NULL DEFAULT 0,
  prompt text,
  params jsonb NOT NULL DEFAULT '{}'::jsonb,
  result_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
  error jsonb NOT NULL DEFAULT 'null'::jsonb,
  created_at text,
  updated_at text,
  worker_finished_at text,
  raw jsonb NOT NULL DEFAULT '{}'::jsonb,
  client_request_id text,
  task_status text NOT NULL DEFAULT 'CREATED',
  billing_status text NOT NULL DEFAULT 'UNQUOTED',
  billing_rule_version_id text,
  quoted_points numeric(18,6) NOT NULL DEFAULT 0,
  reserved_points numeric(18,6) NOT NULL DEFAULT 0,
  captured_points numeric(18,6) NOT NULL DEFAULT 0,
  released_points numeric(18,6) NOT NULL DEFAULT 0,
  refunded_points numeric(18,6) NOT NULL DEFAULT 0,
  supplier_cost numeric(18,6),
  estimated_margin numeric(18,6),
  provider_channel text
);
'@

$validSeed = @'
INSERT INTO xz_users(id) VALUES ('user_valid');
INSERT INTO xz_point_accounts(id,user_id,available,frozen,raw)
VALUES ('acct_valid','user_valid',7,5,'{"available":7,"frozen":5}'::jsonb);

INSERT INTO xz_generation_tasks(
  id,user_id,billing_account_type,billing_account_id,type,model,status,progress,
  point_cost,params,raw,task_status,billing_status,quoted_points,reserved_points,
  captured_points,released_points,refunded_points,created_at,updated_at
) VALUES
(
  'task_valid_a','user_valid','PERSONAL','user_valid','TEXT_TO_IMAGE','legacy-model',
  'PENDING',5,2,
  '{"billingReserved":true,"billingRefunded":false,"billingReservationPointCost":2,"billing_scope":"PERSONAL","billing_account_id":"user_valid"}'::jsonb,
  '{"id":"task_valid_a","userId":"user_valid","billingAccountType":"PERSONAL","billingAccountId":"user_valid","type":"TEXT_TO_IMAGE","model":"legacy-model","status":"PENDING","taskStatus":"QUEUED","billingStatus":"RESERVED","pointCost":2,"quotedPoints":2,"reservedPoints":2,"capturedPoints":0,"releasedPoints":0,"refundedPoints":0,"params":{"billingReserved":true,"billingRefunded":false,"billingReservationPointCost":2,"billing_scope":"PERSONAL","billing_account_id":"user_valid"}}'::jsonb,
  'QUEUED','RESERVED',2,2,0,0,0,'2026-08-04T00:00:00Z','2026-08-04T00:00:00Z'
),
(
  'task_valid_b','user_valid','PERSONAL','user_valid','TEXT_TO_VIDEO','legacy-model',
  'PROCESSING',25,3,
  '{"billingReserved":true,"billingRefunded":false,"billingReservationPointCost":3,"billing_scope":"PERSONAL","billing_account_id":"user_valid"}'::jsonb,
  '{"id":"task_valid_b","userId":"user_valid","billingAccountType":"PERSONAL","billingAccountId":"user_valid","type":"TEXT_TO_VIDEO","model":"legacy-model","status":"PROCESSING","taskStatus":"RUNNING","billingStatus":"RESERVED","pointCost":3,"quotedPoints":3,"reservedPoints":3,"capturedPoints":0,"releasedPoints":0,"refundedPoints":0,"params":{"billingReserved":true,"billingRefunded":false,"billingReservationPointCost":3,"billing_scope":"PERSONAL","billing_account_id":"user_valid"}}'::jsonb,
  'RUNNING','RESERVED',3,3,0,0,0,'2026-08-04T00:01:00Z','2026-08-04T00:01:00Z'
);

INSERT INTO xz_wallet_ledger(
  id,account_id,user_id,task_id,entry_type,points,
  available_before,available_after,frozen_before,frozen_after,
  idempotency_key,reference_type,reference_id,created_at
) VALUES
  ('ledger_valid_a','acct_valid','user_valid','task_valid_a','RESERVE',2,12,10,0,2,
   'task_valid_a:RESERVE','GENERATION_TASK','task_valid_a','2026-08-04T00:00:00Z'),
  ('ledger_valid_b','acct_valid','user_valid','task_valid_b','RESERVE',3,10,7,2,5,
   'task_valid_b:RESERVE','GENERATION_TASK','task_valid_b','2026-08-04T00:01:00Z');
'@

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
  param(
    [Parameter(Mandatory = $true)][string]$Path,
    [int]$ExpectedExitCode = 0
  )

  $old = $ErrorActionPreference
  $ErrorActionPreference = 'Continue'
  Get-Content -LiteralPath $Path -Raw -Encoding utf8 |
    docker exec -i $container psql -q -U verify -d verify -v ON_ERROR_STOP=1 2>&1 |
    Out-Host
  $exitCode = $LASTEXITCODE
  $ErrorActionPreference = $old
  if ($exitCode -ne $ExpectedExitCode) {
    throw "psql file exit code $exitCode, expected $ExpectedExitCode`: $Path"
  }
  return $exitCode
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

function Reset-Database {
  Invoke-PsqlText 'DROP SCHEMA public CASCADE; CREATE SCHEMA public;' | Out-Null
  Invoke-PsqlText $baseSchema | Out-Null
}

function Apply-Foundations {
  Invoke-PsqlFile $migration103Path | Out-Null
  Invoke-PsqlFile $migration104Path | Out-Null
}

function Apply-105OrLeaveRed {
  if (Test-Path -LiteralPath $migration105Path) {
    Invoke-PsqlFile $migration105Path | Out-Null
  } else {
    Write-Output 'RED setup: migration 105 is absent; postcondition fixture must fail'
  }
}

function Seed-ValidCase {
  Invoke-PsqlText $validSeed | Out-Null
}

function Run-ValidAndRollbackGates {
  Reset-Database
  Seed-ValidCase
  Apply-Foundations
  $openingMovementCount = Query-Scalar "SELECT count(*) FROM xz_personal_point_lot_movements WHERE account_id='acct_valid'"
  $walletCount = Query-Scalar "SELECT count(*) FROM xz_wallet_ledger WHERE account_id='acct_valid'"

  Apply-105OrLeaveRed
  Invoke-PsqlFile $fixture105Path | Out-Null
  Write-Output 'PASS valid legacy frozen attribution'

  Invoke-PsqlFile $migration105Path | Out-Null
  Invoke-PsqlFile $fixture105Path | Out-Null
  Assert-Equal (Query-Scalar "SELECT count(*) FROM xz_personal_point_reservations WHERE metadata->>'migration'='105-personal-point-legacy-reservation-attribution'") '2' 'rerun reservation count'
  Assert-Equal (Query-Scalar "SELECT count(*) FROM xz_personal_point_reservation_allocations WHERE metadata->>'migration'='105-personal-point-legacy-reservation-attribution'") '2' 'rerun allocation count'
  Assert-Equal (Query-Scalar "SELECT count(*) FROM xz_personal_point_lot_movements WHERE account_id='acct_valid'") $openingMovementCount 'rerun movement count'
  Assert-Equal (Query-Scalar "SELECT count(*) FROM xz_wallet_ledger WHERE account_id='acct_valid'") $walletCount 'rerun wallet count'
  Write-Output 'PASS migration 105 rerun is idempotent'

  Invoke-PsqlFile $rollback105Path | Out-Null
  Assert-Equal (Query-Scalar "SELECT count(*) FROM xz_personal_point_reservations WHERE metadata->>'migration'='105-personal-point-legacy-reservation-attribution'") '0' 'unsettled rollback reservation cleanup'
  Assert-Equal (Query-Scalar "SELECT count(*) FROM xz_personal_point_reservation_allocations WHERE metadata->>'migration'='105-personal-point-legacy-reservation-attribution'") '0' 'unsettled rollback allocation cleanup'
  Assert-Equal (Query-Scalar "SELECT count(*) FROM xz_generation_tasks WHERE id IN ('task_valid_a','task_valid_b') AND (raw ? 'billingEngine' OR raw ? 'personalPointAccountId' OR raw ? 'personalPointReservationId')") '0' 'unsettled rollback marker cleanup'
  Assert-Equal (Query-Scalar "SELECT available || ':' || frozen FROM xz_point_accounts WHERE id='acct_valid'") '7:5' 'unsettled rollback account balance'
  Assert-Equal (Query-Scalar "SELECT available_points || ':' || reserved_points FROM xz_personal_point_lots WHERE account_id='acct_valid'") '7:5' 'unsettled rollback lot balance'
  Assert-Equal (Query-Scalar "SELECT count(*) FROM xz_personal_point_lot_movements WHERE account_id='acct_valid'") $openingMovementCount 'unsettled rollback movement count'
  Assert-Equal (Query-Scalar "SELECT count(*) FROM xz_wallet_ledger WHERE account_id='acct_valid'") $walletCount 'unsettled rollback wallet count'
  Write-Output 'PASS unsettled rollback restores pre-105 attribution state'

  Invoke-PsqlFile $migration105Path | Out-Null
  Invoke-PsqlFile $fixture105Path | Out-Null
  Write-Output 'PASS migration 105 reapplies after clean rollback'

  Invoke-PsqlText @'
UPDATE xz_personal_point_reservation_allocations
SET reserved_points=0,captured_points=2,status='CAPTURED',updated_at=now()
WHERE id='personal_point_allocation_legacy_d62bb9c78f2e0b6e1923ca55';
UPDATE xz_personal_point_reservations
SET reserved_points=0,captured_points=2,status='CAPTURED',updated_at=now()
WHERE id='personal_point_reservation_legacy_d62bb9c78f2e0b6e1923ca55';
UPDATE xz_personal_point_lots
SET reserved_points=3,consumed_points=2,updated_at=now()
WHERE account_id='acct_valid';
UPDATE xz_point_accounts
SET frozen=3,raw=jsonb_set(raw,'{frozen}','3'::jsonb,true)
WHERE id='acct_valid';
INSERT INTO xz_personal_point_lot_movements(
  id,lot_id,account_id,user_id,movement_type,points,
  available_before,available_after,reserved_before,reserved_after,
  consumed_before,consumed_after,expired_before,expired_after,
  reversed_before,reversed_after,reference_type,reference_id,
  reservation_id,idempotency_key,metadata
) VALUES (
  'movement_terminal_capture_a',
  'personal_point_lot_legacy_' || substr(md5('acct_valid'),1,24),
  'acct_valid','user_valid','CAPTURE',2,7,7,5,3,0,2,0,0,0,0,
  'GENERATION_TASK','task_valid_a',
  'personal_point_reservation_legacy_d62bb9c78f2e0b6e1923ca55',
  'generation:capture:task_valid_a','{"fixture":"105"}'::jsonb
);
'@ | Out-Null

  Invoke-PsqlFile $rollback105Path 3 | Out-Null
  Assert-Equal (Query-Scalar "SELECT status FROM xz_personal_point_reservations WHERE id='personal_point_reservation_legacy_d62bb9c78f2e0b6e1923ca55'") 'CAPTURED' 'settled rollback preserves reservation'
  Assert-Equal (Query-Scalar "SELECT raw->>'billingEngine' FROM xz_generation_tasks WHERE id='task_valid_a'") 'PERSONAL_LOT_V1' 'settled rollback preserves marker'
  Write-Output 'PASS settled rollback fails closed'
}

function Run-AmbiguousDuplicateLedgerGate {
  Reset-Database
  Seed-ValidCase
  Invoke-PsqlText @'
UPDATE xz_point_accounts SET frozen=7,raw=jsonb_set(raw,'{frozen}','7'::jsonb,true) WHERE id='acct_valid';
INSERT INTO xz_wallet_ledger(
  id,account_id,user_id,task_id,entry_type,points,
  available_before,available_after,frozen_before,frozen_after,
  idempotency_key,reference_type,reference_id,created_at
) VALUES (
  'ledger_valid_a_duplicate','acct_valid','user_valid','task_valid_a','RESERVE',2,
  7,5,5,7,'task_valid_a:RESERVE:DUPLICATE','GENERATION_TASK','task_valid_a',
  '2026-08-04T00:02:00Z'
);
'@ | Out-Null
  Apply-Foundations
  Invoke-PsqlFile $migration105Path 3 | Out-Null
  Assert-Equal (Query-Scalar "SELECT count(*) FROM xz_personal_point_reservations") '0' 'duplicate ledger migration rollback reservations'
  Assert-Equal (Query-Scalar "SELECT count(*) FROM xz_generation_tasks WHERE raw ? 'billingEngine'") '0' 'duplicate ledger migration rollback markers'
  Write-Output 'PASS duplicate RESERVE ledger fails closed'
}

function Run-FrozenClosureGate {
  Reset-Database
  Seed-ValidCase
  Invoke-PsqlText @'
UPDATE xz_point_accounts SET frozen=6,raw=jsonb_set(raw,'{frozen}','6'::jsonb,true) WHERE id='acct_valid';
INSERT INTO xz_wallet_ledger(
  id,account_id,user_id,task_id,entry_type,points,
  available_before,available_after,frozen_before,frozen_after,
  idempotency_key,reference_type,reference_id,created_at
) VALUES (
  'ledger_orphan','acct_valid','user_valid','orphan_task','RESERVE',1,
  7,6,5,6,'orphan_task:RESERVE','GENERATION_TASK','orphan_task',
  '2026-08-04T00:02:00Z'
);
'@ | Out-Null
  Apply-Foundations
  Invoke-PsqlFile $migration105Path 3 | Out-Null
  Assert-Equal (Query-Scalar "SELECT count(*) FROM xz_personal_point_reservations") '0' 'frozen closure migration rollback reservations'
  Write-Output 'PASS account frozen/task attribution mismatch fails closed'
}

function Run-PartialMarkerGate {
  Reset-Database
  Seed-ValidCase
  Invoke-PsqlText "UPDATE xz_generation_tasks SET raw=jsonb_set(raw,'{billingEngine}',to_jsonb('PERSONAL_LOT_V1'::text),true) WHERE id='task_valid_a';" | Out-Null
  Apply-Foundations
  Invoke-PsqlFile $migration105Path 3 | Out-Null
  Assert-Equal (Query-Scalar "SELECT count(*) FROM xz_personal_point_reservations") '0' 'partial marker migration rollback reservations'
  Write-Output 'PASS partial marker attribution fails closed'
}

function Run-RawDriftGate {
  Reset-Database
  Seed-ValidCase
  Invoke-PsqlText "UPDATE xz_generation_tasks SET raw=jsonb_set(raw,'{pointCost}','99'::jsonb,true) WHERE id='task_valid_a';" | Out-Null
  Apply-Foundations
  Invoke-PsqlFile $migration105Path 3 | Out-Null
  Assert-Equal (Query-Scalar "SELECT count(*) FROM xz_personal_point_reservations") '0' 'raw drift migration rollback reservations'
  Write-Output 'PASS raw/column economic drift fails closed'
}

function Run-UnknownScopeGate {
  Reset-Database
  Seed-ValidCase
  Invoke-PsqlText @'
UPDATE xz_generation_tasks
SET billing_account_type='FOO',
    params=jsonb_set(params,'{billing_scope}',to_jsonb('FOO'::text),true),
    raw=jsonb_set(
      jsonb_set(raw,'{billingAccountType}',to_jsonb('FOO'::text),true),
      '{params,billing_scope}',to_jsonb('FOO'::text),true
    )
WHERE id='task_valid_a';
'@ | Out-Null
  Apply-Foundations
  Invoke-PsqlFile $migration105Path 3 | Out-Null
  Assert-Equal (Query-Scalar "SELECT count(*) FROM xz_personal_point_reservations") '0' 'unknown scope migration rollback reservations'
  Write-Output 'PASS unknown billing scope fails closed'
}

function Run-AvailableProjectionGate {
  Reset-Database
  Seed-ValidCase
  Apply-Foundations
  Invoke-PsqlText "UPDATE xz_point_accounts SET available=6,raw=jsonb_set(raw,'{available}','6'::jsonb,true) WHERE id='acct_valid';" | Out-Null
  Invoke-PsqlFile $migration105Path 3 | Out-Null
  Assert-Equal (Query-Scalar "SELECT count(*) FROM xz_personal_point_reservations") '0' 'available projection migration rollback reservations'
  Write-Output 'PASS account available/lot projection mismatch fails closed'
}

function Run-TerminalTaskRollbackGate {
  Reset-Database
  Seed-ValidCase
  Apply-Foundations
  Invoke-PsqlFile $migration105Path | Out-Null
  Invoke-PsqlText @'
UPDATE xz_generation_tasks
SET status='SUCCEEDED',billing_status='CAPTURED',reserved_points=0,captured_points=2,
    raw=jsonb_set(
      jsonb_set(
        jsonb_set(
          jsonb_set(raw,'{status}',to_jsonb('SUCCEEDED'::text),true),
          '{billingStatus}',to_jsonb('CAPTURED'::text),true
        ),
        '{reservedPoints}','0'::jsonb,true
      ),
      '{capturedPoints}','2'::jsonb,true
    )
WHERE id='task_valid_a';
'@ | Out-Null
  Invoke-PsqlFile $rollback105Path 3 | Out-Null
  Assert-Equal (Query-Scalar "SELECT count(*) FROM xz_personal_point_reservations WHERE metadata->>'migration'='105-personal-point-legacy-reservation-attribution'") '2' 'terminal task rollback preserves reservations'
  Write-Output 'PASS terminal task state blocks rollback even before reservation mutation'
}

function Run-UnattributableActivePersonalGate {
  Reset-Database
  Invoke-PsqlText @'
INSERT INTO xz_users(id) VALUES ('user_unattributable');
INSERT INTO xz_point_accounts(id,user_id,available,frozen,raw)
VALUES ('acct_unattributable','user_unattributable',1,0,'{"available":1,"frozen":0}'::jsonb);
INSERT INTO xz_generation_tasks(
  id,user_id,billing_account_type,billing_account_id,type,model,status,point_cost,
  params,raw,task_status,billing_status,quoted_points,reserved_points,
  captured_points,released_points,refunded_points
) VALUES (
  'task_unattributable','user_unattributable','PERSONAL','user_unattributable',
  'TEXT_TO_IMAGE','legacy-model','PENDING',1,'{"billing_scope":"PERSONAL"}'::jsonb,
  '{"id":"task_unattributable","userId":"user_unattributable","billingAccountType":"PERSONAL","status":"PENDING","pointCost":1,"params":{"billing_scope":"PERSONAL"}}'::jsonb,
  'QUEUED','UNQUOTED',1,0,0,0,0
);
'@ | Out-Null
  Apply-Foundations
  Invoke-PsqlFile $migration105Path 3 | Out-Null
  Assert-Equal (Query-Scalar "SELECT count(*) FROM xz_personal_point_reservations") '0' 'unattributable active task migration rollback reservations'
  Write-Output 'PASS active personal task without LOT_V1 or exact reserve evidence fails closed'
}

function Run-RawEconomicColumnDriftGate {
  Reset-Database
  Seed-ValidCase
  Invoke-PsqlText "UPDATE xz_generation_tasks SET raw=jsonb_set(raw,'{reservedPoints}','99'::jsonb,true) WHERE id='task_valid_a';" | Out-Null
  Apply-Foundations
  Invoke-PsqlFile $migration105Path 3 | Out-Null
  Assert-Equal (Query-Scalar "SELECT count(*) FROM xz_personal_point_reservations") '0' 'raw economic column drift migration rollback reservations'
  Write-Output 'PASS optional raw economic field drift fails closed'
}

Remove-TestContainer
$old = $ErrorActionPreference
$ErrorActionPreference = 'Continue'
docker run --rm -d --name $container -e POSTGRES_PASSWORD=verify -e POSTGRES_USER=verify -e POSTGRES_DB=verify postgres:16-alpine 2>&1 | Out-Host
$runCode = $LASTEXITCODE
$ErrorActionPreference = $old
if ($runCode -ne 0) {
  throw 'postgres container failed to start'
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
    throw 'postgres container did not become ready'
  }
  Start-Sleep -Seconds 1

  Run-ValidAndRollbackGates
  Run-AmbiguousDuplicateLedgerGate
  Run-FrozenClosureGate
  Run-PartialMarkerGate
  Run-RawDriftGate
  Run-UnknownScopeGate
  Run-AvailableProjectionGate
  Run-TerminalTaskRollbackGate
  Run-UnattributableActivePersonalGate
  Run-RawEconomicColumnDriftGate
  Write-Output 'PASS migration 105 gate suite'
}
finally {
  Remove-TestContainer
}
