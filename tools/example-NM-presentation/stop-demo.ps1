Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$runtimeRoot = Join-Path $scriptRoot "runtime"
$demoPorts = @(6044, 6045, 6055, 6144, 6145, 6155)
$pidFiles = @(
  (Join-Path $runtimeRoot "traffic.pid"),
  (Join-Path $runtimeRoot "node-a.pid"),
  (Join-Path $runtimeRoot "node-b.pid")
)

foreach ($pidFile in $pidFiles) {
  if (-not (Test-Path $pidFile)) {
    continue
  }

  $raw = (Get-Content $pidFile -Raw).Trim()
  if ($raw) {
    $pidValue = [int]$raw
    $proc = Get-Process -Id $pidValue -ErrorAction SilentlyContinue
    if ($null -ne $proc) {
      Stop-Process -Id $pidValue -Force
      Write-Host "Stopped PID $pidValue"
    }
  }

  Remove-Item $pidFile -Force -ErrorAction SilentlyContinue
}

Get-CimInstance Win32_Process | Where-Object {
  $_.Name -in @("go.exe", "powershell.exe") -and (
    $_.CommandLine -like "*traffic-loop.ps1*" -or
    $_.CommandLine -like "*node-runner*"
  )
} | ForEach-Object {
  Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue
}

foreach ($port in $demoPorts) {
  Get-NetTCPConnection -LocalPort $port -ErrorAction SilentlyContinue | ForEach-Object {
    Stop-Process -Id $_.OwningProcess -Force -ErrorAction SilentlyContinue
  }
}

Write-Host "Demo network-manager processes stopped."
