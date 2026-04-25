# Build and restart the local pindoc-server daemon without administrator rights.
#
# Agent-friendly usage:
#   powershell -ExecutionPolicy Bypass -File scripts\dev-restart.ps1

[CmdletBinding()]
param(
    [switch]$NoBuild,
    [switch]$NoRestart,
    [string]$TaskName = "PindocDaemon",
    [string]$ListenAddr = "127.0.0.1:5830",
    [string]$DatabaseURL = "postgres://pindoc:pindoc_dev@localhost:5432/pindoc?sslmode=disable",
    [string]$LogLevel = "info",
    [string]$UserLanguage = $env:PINDOC_USER_LANGUAGE,
    [string]$UserName = $env:PINDOC_USER_NAME,
    [string]$UserEmail = $env:PINDOC_USER_EMAIL,
    [string]$ProjectSlug = $env:PINDOC_PROJECT,
    [int]$VerifyTimeoutSec = 10
)

$ErrorActionPreference = "Stop"

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$BinaryPath = Join-Path $RepoRoot "bin\pindoc-server.exe"
$LogDir = Join-Path $RepoRoot "logs"

Set-Location $RepoRoot

if ([string]::IsNullOrWhiteSpace($UserLanguage)) { $UserLanguage = "en" }
if ([string]::IsNullOrWhiteSpace($ProjectSlug)) { $ProjectSlug = "pindoc" }
if ([string]::IsNullOrWhiteSpace($UserName)) {
    $UserName = (& git config --get user.name 2>$null)
}
if ([string]::IsNullOrWhiteSpace($UserEmail)) {
    $UserEmail = (& git config --get user.email 2>$null)
}

if (-not $NoBuild) {
    Write-Host "Building pindoc-server..."
    & go build -o $BinaryPath .\cmd\pindoc-server
    if ($LASTEXITCODE -ne 0) {
        throw "go build failed with exit code $LASTEXITCODE"
    }
    Write-Host "Build OK: $BinaryPath"
}

if ($NoRestart) {
    exit 0
}

New-Item -ItemType Directory -Force -Path $LogDir | Out-Null

$env:PINDOC_DATABASE_URL = $DatabaseURL
$env:PINDOC_LOG_LEVEL = $LogLevel
$env:PINDOC_USER_LANGUAGE = $UserLanguage
$env:PINDOC_PROJECT = $ProjectSlug
$env:PINDOC_REPO_ROOT = $RepoRoot
if (-not [string]::IsNullOrWhiteSpace($UserName)) {
    $env:PINDOC_USER_NAME = $UserName
}
if (-not [string]::IsNullOrWhiteSpace($UserEmail)) {
    $env:PINDOC_USER_EMAIL = $UserEmail
}

$task = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
if ($task) {
    Write-Host "Restarting Scheduled Task '$TaskName'..."
    Stop-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 1
    Start-ScheduledTask -TaskName $TaskName
} else {
    Write-Host "Stopping existing daemon processes launched from $BinaryPath..."
    Get-Process -Name "pindoc-server" -ErrorAction SilentlyContinue |
        Where-Object { $_.Path -eq $BinaryPath } |
        Stop-Process -Force
    Start-Sleep -Seconds 1

    Write-Host "Starting pindoc-server -http $ListenAddr..."
    Start-Process `
        -FilePath $BinaryPath `
        -ArgumentList @("-http", $ListenAddr) `
        -WorkingDirectory $RepoRoot `
        -WindowStyle Hidden
}

$healthURL = "http://$ListenAddr/health"
for ($i = 0; $i -lt $VerifyTimeoutSec; $i++) {
    try {
        $health = Invoke-RestMethod -Uri $healthURL -TimeoutSec 2
        Write-Host "Health OK: status=$($health.status), db=$($health.db), uptime=$($health.uptime_sec)s"
        exit 0
    } catch {
        Start-Sleep -Seconds 1
    }
}

throw "pindoc-server did not become healthy at $healthURL within $VerifyTimeoutSec seconds"
