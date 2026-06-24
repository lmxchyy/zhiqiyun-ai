param(
  [string]$Container = "ai-postgres-1",
  [string]$Database = "xianzhi",
  [string]$User = "xianzhi",
  [string]$OutputDir = "backups"
)

$ErrorActionPreference = "Stop"
$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$resolvedOutput = Join-Path (Get-Location) $OutputDir
New-Item -ItemType Directory -Force -Path $resolvedOutput | Out-Null
$output = Join-Path $resolvedOutput "$Database-$timestamp.sql"

docker exec $Container pg_dump -U $User -d $Database --clean --if-exists | Set-Content -Encoding UTF8 $output
Write-Host "Backup written to $output"
