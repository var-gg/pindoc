# Install the pindoc-server Windows service via NSSM.
#
# Run from an elevated PowerShell:
#   powershell -ExecutionPolicy Bypass -File scripts\install-service.ps1
#
# What it does:
#   1. Downloads NSSM 2.24 to <repo>\tools\nssm\ if not already present
#      (vendored under tools/ on purpose so the install path is
#      reproducible across machines without polluting C:\).
#   2. Stops + removes any existing pindoc-server service so re-running
#      is idempotent (config drift is the most likely failure mode for
#      hand-edited NSSM services).
#   3. Installs pindoc-server pointing at bin\pindoc-server.exe with
#      -http 127.0.0.1:5830, working dir at the repo root, env vars
#      sourced from the script parameters (DB URL, log level, locale,
#      identity dual).
#   4. Routes stdout/stderr to logs\service.{out,err}.log with 10MB
#      rotation so the disk doesn't fill on a long-running daemon.
#   5. Configures auto-start on boot + auto-restart on exit.
#   6. Starts the service and prints status.
#
# Re-run any time after rebuilding bin\pindoc-server.exe — the service
# already points at that path, so a `Restart-Service pindoc-server`
# picks the new binary up. Pass -Reinstall to also recreate the
# NSSM-registered config (use after editing this script).

[CmdletBinding()]
param(
    [string]$ServiceName = "pindoc-server",
    [string]$ListenAddr  = "127.0.0.1:5830",
    [string]$DatabaseURL = "postgres://pindoc:pindoc_dev@localhost:5432/pindoc?sslmode=disable",
    [string]$LogLevel    = "info",
    [string]$UserLanguage = "ko",
    [string]$UserName    = "curioustore",
    [string]$UserEmail   = "rhkdwls750@naver.com",
    [switch]$Reinstall
)

$ErrorActionPreference = "Stop"

# Resolve repo root from this script's location so the service binds to
# stable absolute paths regardless of the user's cwd at install time.
$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$BinaryPath = Join-Path $RepoRoot "bin\pindoc-server.exe"
$LogDir     = Join-Path $RepoRoot "logs"
$ToolsDir   = Join-Path $RepoRoot "tools\nssm"
$NssmExe    = Join-Path $ToolsDir "nssm.exe"

function Require-Admin {
    $current = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = [Security.Principal.WindowsPrincipal]::new($current)
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltinRole]::Administrator)) {
        throw "install-service.ps1 must run from an elevated PowerShell (right-click -> Run as Administrator). NSSM service registration touches the SCM."
    }
}

function Ensure-Nssm {
    if (Test-Path $NssmExe) {
        Write-Host "NSSM already vendored at $NssmExe"
        return
    }
    New-Item -ItemType Directory -Force -Path $ToolsDir | Out-Null

    # Try winget first — nssm.cc has been intermittently 503ing and
    # winget pulls from the same upstream zip but with retries + cache.
    if (Get-Command winget -ErrorAction SilentlyContinue) {
        Write-Host "Installing NSSM via winget..."
        try {
            winget install --id NSSM.NSSM --silent --accept-package-agreements --accept-source-agreements --disable-interactivity 2>&1 | Out-Host
        } catch {
            Write-Host "winget install threw: $($_.Exception.Message) — will look for any cached copy anyway."
        }
        $candidate = Get-ChildItem "$env:LOCALAPPDATA\Microsoft\WinGet\Packages" -Recurse -Filter "nssm.exe" -ErrorAction SilentlyContinue |
            Where-Object { $_.FullName -match "win64" } |
            Select-Object -First 1
        if ($candidate) {
            Copy-Item $candidate.FullName $NssmExe -Force
            Write-Host "NSSM vendored from winget cache: $NssmExe"
            return
        }
        Write-Host "winget didn't surface a nssm.exe; falling through to direct download."
    }

    # Fallback chain. nssm.cc is canonical but flakey; the GitHub mirror
    # below carries the same 2.24 zip published by the maintainer for
    # users hitting the 503.
    $zipPath = Join-Path $env:TEMP "nssm-2.24.zip"
    $extractRoot = Join-Path $env:TEMP "nssm-2.24-extract"
    if (Test-Path $extractRoot) { Remove-Item -Recurse -Force $extractRoot }
    $sources = @(
        "https://nssm.cc/release/nssm-2.24.zip",
        "https://web.archive.org/web/2024/https://nssm.cc/release/nssm-2.24.zip"
    )
    $downloaded = $false
    foreach ($src in $sources) {
        try {
            Write-Host "Trying $src ..."
            Invoke-WebRequest -Uri $src -OutFile $zipPath -UseBasicParsing -ErrorAction Stop
            $downloaded = $true
            break
        } catch {
            Write-Host "  failed: $($_.Exception.Message)"
        }
    }
    if (-not $downloaded) {
        throw "Could not download NSSM from any source. Install manually (e.g. 'winget install NSSM.NSSM' or 'choco install nssm') and re-run this script — the existing tools\nssm\nssm.exe will be picked up automatically."
    }
    Expand-Archive -Path $zipPath -DestinationPath $extractRoot -Force
    # Pick the win64 build — workstation host always 64-bit on Windows 10/11.
    $candidate = Get-ChildItem -Path $extractRoot -Recurse -Filter "nssm.exe" |
        Where-Object { $_.FullName -match "win64" } |
        Select-Object -First 1
    if (-not $candidate) {
        throw "Could not find nssm.exe in the downloaded archive ($extractRoot)."
    }
    Copy-Item $candidate.FullName $NssmExe -Force
    Remove-Item -Recurse -Force $extractRoot
    Remove-Item -Force $zipPath
    Write-Host "NSSM vendored at $NssmExe"
}

function Stop-And-Remove-Service {
    $svc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    if ($svc) {
        Write-Host "Stopping existing $ServiceName service..."
        & $NssmExe stop $ServiceName confirm | Out-Null
        Start-Sleep -Seconds 1
        Write-Host "Removing existing $ServiceName service..."
        & $NssmExe remove $ServiceName confirm | Out-Null
    }
}

function Install-Service {
    Write-Host "Installing $ServiceName -> $BinaryPath $ListenAddr"
    & $NssmExe install $ServiceName $BinaryPath "-http" $ListenAddr | Out-Null
    & $NssmExe set $ServiceName AppDirectory $RepoRoot | Out-Null
    & $NssmExe set $ServiceName DisplayName "Pindoc MCP+Reader Daemon" | Out-Null
    & $NssmExe set $ServiceName Description "pindoc-server -http $ListenAddr — serves /mcp/p/{project} (MCP streamable-HTTP), /api/... (Reader read-only API), /health (liveness)." | Out-Null
    & $NssmExe set $ServiceName Start SERVICE_AUTO_START | Out-Null

    # Env. NSSM accepts repeated K=V pairs on a single set call.
    $envPairs = @(
        "PINDOC_DATABASE_URL=$DatabaseURL",
        "PINDOC_LOG_LEVEL=$LogLevel",
        "PINDOC_USER_LANGUAGE=$UserLanguage",
        "PINDOC_USER_NAME=$UserName",
        "PINDOC_USER_EMAIL=$UserEmail"
    )
    & $NssmExe set $ServiceName AppEnvironmentExtra @envPairs | Out-Null

    # Auto-restart on exit, with a 1s throttle so a crash loop isn't
    # nuclear. NSSM's default Restart action is "Restart" but make it
    # explicit so the service config is grep-able.
    & $NssmExe set $ServiceName AppExit Default Restart | Out-Null
    & $NssmExe set $ServiceName AppRestartDelay 1000 | Out-Null
    & $NssmExe set $ServiceName AppThrottle 5000 | Out-Null

    # Logs — rotate at 10MB, keep online rotation so a long-running
    # service doesn't pin a 10GB file open.
    if (-not (Test-Path $LogDir)) {
        New-Item -ItemType Directory -Force -Path $LogDir | Out-Null
    }
    & $NssmExe set $ServiceName AppStdout (Join-Path $LogDir "service.out.log") | Out-Null
    & $NssmExe set $ServiceName AppStderr (Join-Path $LogDir "service.err.log") | Out-Null
    & $NssmExe set $ServiceName AppRotateFiles 1 | Out-Null
    & $NssmExe set $ServiceName AppRotateOnline 1 | Out-Null
    & $NssmExe set $ServiceName AppRotateBytes 10485760 | Out-Null
}

Require-Admin
Ensure-Nssm

if (-not (Test-Path $BinaryPath)) {
    throw "pindoc-server.exe not found at $BinaryPath — run 'go build -o bin/pindoc-server.exe ./cmd/pindoc-server' first."
}

$existing = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($existing -and -not $Reinstall) {
    Write-Host "$ServiceName already installed; restarting to pick up any binary refresh. Pass -Reinstall to recreate the NSSM config."
    & $NssmExe stop $ServiceName confirm | Out-Null
    Start-Sleep -Seconds 1
} else {
    Stop-And-Remove-Service
    Install-Service
}

Write-Host "Starting $ServiceName..."
& $NssmExe start $ServiceName | Out-Null
Start-Sleep -Seconds 2

$svc = Get-Service -Name $ServiceName
Write-Host ""
Write-Host "Service status:"
$svc | Format-Table Name, Status, StartType -AutoSize | Out-String | Write-Host
Write-Host "Listening on http://$ListenAddr — try:"
Write-Host "  curl http://$ListenAddr/health"
Write-Host "  curl http://$ListenAddr/api/config"
