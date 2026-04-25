# Register pindoc-server as a user-mode Scheduled Task.
#
# No administrator rights required. The task runs at user logon and keeps
# pindoc-server in the foreground so Task Scheduler can restart it on exit.
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File scripts\install-user-mode.ps1

[CmdletBinding()]
param(
    [string]$TaskName = "PindocDaemon",
    [string]$ListenAddr = "127.0.0.1:5830",
    [string]$DatabaseURL = "postgres://pindoc:pindoc_dev@localhost:5432/pindoc?sslmode=disable",
    [string]$LogLevel = "info",
    [string]$UserLanguage = $env:PINDOC_USER_LANGUAGE,
    [string]$UserName = $env:PINDOC_USER_NAME,
    [string]$UserEmail = $env:PINDOC_USER_EMAIL,
    [string]$ProjectSlug = $env:PINDOC_PROJECT,
    [switch]$NoStart
)

$ErrorActionPreference = "Stop"

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$BinaryPath = Join-Path $RepoRoot "bin\pindoc-server.exe"
$LogDir = Join-Path $RepoRoot "logs"
$LogPath = Join-Path $LogDir "user-daemon.log"

if (-not (Test-Path $BinaryPath)) {
    throw "pindoc-server.exe not found at $BinaryPath. Run: go build -o bin\pindoc-server.exe .\cmd\pindoc-server"
}

$legacy = Get-Service -Name "pindoc-server" -ErrorAction SilentlyContinue
if ($legacy -and $legacy.Status -ne "Stopped") {
    throw "Legacy service 'pindoc-server' is still running. Run scripts\uninstall-service.ps1 once from an elevated PowerShell, then retry."
}

if ([string]::IsNullOrWhiteSpace($UserLanguage)) { $UserLanguage = "en" }
if ([string]::IsNullOrWhiteSpace($ProjectSlug)) { $ProjectSlug = "pindoc" }
if ([string]::IsNullOrWhiteSpace($UserName)) {
    $UserName = (& git config --get user.name 2>$null)
}
if ([string]::IsNullOrWhiteSpace($UserEmail)) {
    $UserEmail = (& git config --get user.email 2>$null)
}

New-Item -ItemType Directory -Force -Path $LogDir | Out-Null

function Quote-PSLiteral([string]$Value) {
    return "'" + ($Value -replace "'", "''") + "'"
}

$envLines = @(
    "`$env:PINDOC_DATABASE_URL = $(Quote-PSLiteral $DatabaseURL)",
    "`$env:PINDOC_LOG_LEVEL = $(Quote-PSLiteral $LogLevel)",
    "`$env:PINDOC_USER_LANGUAGE = $(Quote-PSLiteral $UserLanguage)",
    "`$env:PINDOC_PROJECT = $(Quote-PSLiteral $ProjectSlug)",
    "`$env:PINDOC_REPO_ROOT = $(Quote-PSLiteral $RepoRoot)"
)
if (-not [string]::IsNullOrWhiteSpace($UserName)) {
    $envLines += "`$env:PINDOC_USER_NAME = $(Quote-PSLiteral $UserName)"
}
if (-not [string]::IsNullOrWhiteSpace($UserEmail)) {
    $envLines += "`$env:PINDOC_USER_EMAIL = $(Quote-PSLiteral $UserEmail)"
}

$command = @"
`$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $(Quote-PSLiteral $RepoRoot)
$($envLines -join "`n")
New-Item -ItemType Directory -Force -Path $(Quote-PSLiteral $LogDir) | Out-Null
& $(Quote-PSLiteral $BinaryPath) -http $(Quote-PSLiteral $ListenAddr) *>> $(Quote-PSLiteral $LogPath)
exit `$LASTEXITCODE
"@

$encoded = [Convert]::ToBase64String([System.Text.Encoding]::Unicode.GetBytes($command))
$action = New-ScheduledTaskAction `
    -Execute "powershell.exe" `
    -Argument "-NoProfile -ExecutionPolicy Bypass -EncodedCommand $encoded" `
    -WorkingDirectory $RepoRoot
$trigger = New-ScheduledTaskTrigger -AtLogOn
$settings = New-ScheduledTaskSettingsSet `
    -Hidden `
    -MultipleInstances IgnoreNew `
    -RestartCount 3 `
    -RestartInterval (New-TimeSpan -Minutes 1) `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -ExecutionTimeLimit ([TimeSpan]::Zero)
$principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -RunLevel Limited

$existing = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
if ($existing) {
    Stop-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
    Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false
}

Register-ScheduledTask `
    -TaskName $TaskName `
    -Action $action `
    -Trigger $trigger `
    -Settings $settings `
    -Principal $principal `
    -Description "Pindoc user-mode daemon: $BinaryPath -http $ListenAddr" `
    -Force | Out-Null

if (-not $NoStart) {
    Start-ScheduledTask -TaskName $TaskName
    Start-Sleep -Seconds 2
}

Write-Host "Registered Scheduled Task '$TaskName' for user '$env:USERNAME'."
Write-Host "Endpoint: http://$ListenAddr"
Write-Host "Logs: $LogPath"
Write-Host "Verify: curl http://$ListenAddr/health"
