# Remove the legacy NSSM-backed pindoc-server Windows service.
#
# Run once from an elevated PowerShell when migrating to user-mode daemon:
#   powershell -ExecutionPolicy Bypass -File scripts\uninstall-service.ps1

[CmdletBinding()]
param(
    [string]$ServiceName = "pindoc-server",
    [switch]$Force
)

$ErrorActionPreference = "Stop"

function Test-IsAdmin {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = [Security.Principal.WindowsPrincipal]::new($identity)
    return $principal.IsInRole([Security.Principal.WindowsBuiltinRole]::Administrator)
}

$svc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if (-not $svc) {
    Write-Host "Service '$ServiceName' is already absent."
    exit 0
}

if (-not (Test-IsAdmin)) {
    throw "Removing '$ServiceName' requires an elevated PowerShell. Re-run this script as Administrator."
}

if ($svc.Status -ne "Stopped") {
    Write-Host "Stopping service '$ServiceName'..."
    Stop-Service -Name $ServiceName -Force:$Force
    $svc.WaitForStatus("Stopped", [TimeSpan]::FromSeconds(20))
}

Write-Host "Disabling service '$ServiceName'..."
Set-Service -Name $ServiceName -StartupType Disabled

Write-Host "Deleting service '$ServiceName'..."
& sc.exe delete $ServiceName | Out-Host

Write-Host "Legacy service removed. Use scripts\install-user-mode.ps1 from a normal PowerShell to register the user-mode daemon."
