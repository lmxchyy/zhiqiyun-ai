param(
  [Parameter(Mandatory = $true)]
  [string]$InputFile,
  [string]$Container = "ai-postgres-1",
  [string]$Database = "xianzhi",
  [string]$User = "xianzhi"
)

$ErrorActionPreference = "Stop"
$resolvedInput = Resolve-Path $InputFile
Get-Content -Raw -Encoding UTF8 $resolvedInput | docker exec -i $Container psql -U $User -d $Database
Write-Host "Restore completed from $resolvedInput"
