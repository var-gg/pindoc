param(
  [Parameter(Mandatory = $true, Position = 0)]
  [string]$Path,

  [string]$Container = "pindoc-server-daemon",
  [string]$ContainerDir = "/tmp/pindoc-asset-upload",
  [string]$ProjectSlug = ""
)

$ErrorActionPreference = "Stop"

$resolved = Resolve-Path -LiteralPath $Path
$item = Get-Item -LiteralPath $resolved.Path
if ($item.PSIsContainer) {
  throw "Path must be a file, got directory: $($resolved.Path)"
}

$fileName = Split-Path -Leaf $resolved.Path
$stamp = Get-Date -Format "yyyyMMdd-HHmmss"
$suffix = [Guid]::NewGuid().ToString("N").Substring(0, 8)
$remoteDir = "$ContainerDir/$stamp-$suffix"
$remotePath = "$remoteDir/$fileName"

& docker exec $Container mkdir -p $remoteDir | Out-Null
if ($LASTEXITCODE -ne 0) {
  throw "docker exec failed for container '$Container'"
}

& docker cp $resolved.Path "${Container}:$remotePath" | Out-Null
if ($LASTEXITCODE -ne 0) {
  throw "docker cp failed for '$($resolved.Path)' -> ${Container}:$remotePath"
}

$input = [ordered]@{
  local_path = $remotePath
  filename = $fileName
}
if ($ProjectSlug.Trim() -ne "") {
  $input = [ordered]@{
    project_slug = $ProjectSlug
    local_path = $remotePath
    filename = $fileName
  }
}

Write-Output "Copied to ${Container}:$remotePath"
Write-Output "Use this MCP input for pindoc.asset.upload:"
($input | ConvertTo-Json -Compress)
