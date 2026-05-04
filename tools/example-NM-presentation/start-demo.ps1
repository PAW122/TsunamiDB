param(
  [switch]$NoBrowser
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = (Resolve-Path (Join-Path $scriptRoot "..\..")).Path
$runtimeRoot = Join-Path $scriptRoot "runtime"
$sessionName = "session-" + (Get-Date -Format "yyyyMMdd-HHmmss")
$sessionRoot = Join-Path $runtimeRoot $sessionName
$logsRoot = Join-Path $sessionRoot "logs"
$nodeADir = Join-Path $sessionRoot "node-a"
$nodeBDir = Join-Path $sessionRoot "node-b"
$nodeARun = Join-Path $runtimeRoot "node-a.pid"
$nodeBRun = Join-Path $runtimeRoot "node-b.pid"
$trafficRun = Join-Path $runtimeRoot "traffic.pid"

$nodeA = @{
  Name = "node-a"
  Api = 6044
  Sub = 6045
  NM = 6055
  Advertise = "127.0.0.1:6055"
  Peer = ""
  Dir = $nodeADir
  Pid = $nodeARun
  StdOut = Join-Path $logsRoot "node-a.stdout.log"
  StdErr = Join-Path $logsRoot "node-a.stderr.log"
}

$nodeB = @{
  Name = "node-b"
  Api = 6144
  Sub = 6145
  NM = 6155
  Advertise = "127.0.0.1:6155"
  Peer = "127.0.0.1:6055"
  Dir = $nodeBDir
  Pid = $nodeBRun
  StdOut = Join-Path $logsRoot "node-b.stdout.log"
  StdErr = Join-Path $logsRoot "node-b.stderr.log"
}

$trafficOut = Join-Path $logsRoot "traffic.stdout.log"
$trafficErr = Join-Path $logsRoot "traffic.stderr.log"
$demoPorts = @(6044, 6045, 6055, 6144, 6145, 6155)

function Stop-TrackedProcess {
  param([string]$PidFile)

  if (-not (Test-Path $PidFile)) {
    return
  }

  $raw = (Get-Content $PidFile -Raw).Trim()
  if ($raw) {
    $pidValue = [int]$raw
    $proc = Get-Process -Id $pidValue -ErrorAction SilentlyContinue
    if ($null -ne $proc) {
      Stop-Process -Id $pidValue -Force
      Start-Sleep -Milliseconds 200
    }
  }

  Remove-Item $PidFile -Force -ErrorAction SilentlyContinue
}

function Stop-OrphanDemoProcesses {
  $orphanProcs = Get-CimInstance Win32_Process | Where-Object {
    $_.Name -in @("go.exe", "powershell.exe") -and (
      $_.CommandLine -like "*traffic-loop.ps1*" -or
      $_.CommandLine -like "*node-runner*"
    )
  }

  foreach ($proc in $orphanProcs) {
    Stop-Process -Id $proc.ProcessId -Force -ErrorAction SilentlyContinue
  }
}

function Stop-ProcessesByPort {
  param([int[]]$Ports)

  foreach ($port in $Ports) {
    $connections = Get-NetTCPConnection -LocalPort $port -ErrorAction SilentlyContinue
    foreach ($connection in $connections) {
      Stop-Process -Id $connection.OwningProcess -Force -ErrorAction SilentlyContinue
    }
  }
}

function Start-BackgroundShell {
  param(
    [string]$WorkingDirectory,
    [string]$Command,
    [string]$StdOut,
    [string]$StdErr,
    [string]$PidFile
  )

  $proc = Start-Process `
    -FilePath "powershell.exe" `
    -ArgumentList @("-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", $Command) `
    -WorkingDirectory $WorkingDirectory `
    -RedirectStandardOutput $StdOut `
    -RedirectStandardError $StdErr `
    -PassThru

  Set-Content -Path $PidFile -Value $proc.Id -NoNewline
  return $proc
}

function Wait-Health {
  param(
    [string]$Url,
    [int]$TimeoutSec = 30
  )

  $deadline = (Get-Date).AddSeconds($TimeoutSec)
  while ((Get-Date) -lt $deadline) {
    try {
      return Invoke-RestMethod -Uri $Url -TimeoutSec 2
    } catch {
      Start-Sleep -Milliseconds 400
    }
  }

  throw "Timeout waiting for $Url"
}

function Wait-ConnectedPeer {
  param(
    [string]$Url,
    [int]$TimeoutSec = 30
  )

  $deadline = (Get-Date).AddSeconds($TimeoutSec)
  while ((Get-Date) -lt $deadline) {
    try {
      $health = Invoke-RestMethod -Uri $Url -TimeoutSec 2
      if ($health.network.connected_peers -ge 1) {
        return $health
      }
    } catch {
    }

    Start-Sleep -Milliseconds 500
  }

  throw "Timeout waiting for connected peer on $Url"
}

function Save-Json {
  param(
    [string]$BaseUrl,
    [string]$Table,
    [string]$Key,
    [hashtable]$Payload
  )

  $body = $Payload | ConvertTo-Json -Depth 8 -Compress
  Invoke-RestMethod `
    -Method Post `
    -Uri "$BaseUrl/save/$Table/$Key" `
    -ContentType "application/json" `
    -Body $body | Out-Null
}

Stop-TrackedProcess -PidFile $trafficRun
Stop-TrackedProcess -PidFile $nodeARun
Stop-TrackedProcess -PidFile $nodeBRun
Stop-OrphanDemoProcesses
Stop-ProcessesByPort -Ports $demoPorts
Start-Sleep -Milliseconds 500

New-Item -ItemType Directory -Force -Path $runtimeRoot, $sessionRoot, $logsRoot, $nodeADir, $nodeBDir | Out-Null
Set-Content -Path (Join-Path $runtimeRoot "latest-session.txt") -Value $sessionName

$nodeACommand = "go run ..\..\..\node-runner -api-port $($nodeA.Api) -sub-port $($nodeA.Sub) -nm-port $($nodeA.NM) -advertise-addr $($nodeA.Advertise) -network-secret demo-cluster-secret"
$nodeBCommand = "go run ..\..\..\node-runner -api-port $($nodeB.Api) -sub-port $($nodeB.Sub) -nm-port $($nodeB.NM) -known-peers $($nodeB.Peer) -advertise-addr $($nodeB.Advertise) -network-secret demo-cluster-secret"

Start-BackgroundShell -WorkingDirectory $nodeA.Dir -Command $nodeACommand -StdOut $nodeA.StdOut -StdErr $nodeA.StdErr -PidFile $nodeA.Pid | Out-Null

$apiA = "http://127.0.0.1:$($nodeA.Api)"
$apiB = "http://127.0.0.1:$($nodeB.Api)"

Wait-Health -Url "$apiA/health" | Out-Null
Start-Sleep -Milliseconds 800
Start-BackgroundShell -WorkingDirectory $nodeB.Dir -Command $nodeBCommand -StdOut $nodeB.StdOut -StdErr $nodeB.StdErr -PidFile $nodeB.Pid | Out-Null
Wait-Health -Url "$apiB/health" | Out-Null
Wait-ConnectedPeer -Url "$apiA/health" | Out-Null
Wait-ConnectedPeer -Url "$apiB/health" | Out-Null

Save-Json -BaseUrl $apiA -Table "users" -Key "user-1" -Payload @{
  customer_id = "cust-1001"
  name = "Anna Kowalska"
  tier = "pro"
  region = "eu-central"
  last_seen = (Get-Date).ToUniversalTime().ToString("o")
}

Save-Json -BaseUrl $apiB -Table "orders" -Key "order-1" -Payload @{
  order_id = "ord-2026-0001"
  customer_id = "cust-1001"
  status = "created"
  warehouse = "wroclaw-01"
  amount_cents = 129900
  updated_at = (Get-Date).ToUniversalTime().ToString("o")
}

Invoke-RestMethod -Uri "$apiA/read/orders/order-1" -TimeoutSec 3 | Out-Null
Invoke-RestMethod -Uri "$apiB/read/users/user-1" -TimeoutSec 3 | Out-Null

$trafficScript = Join-Path $scriptRoot "traffic-loop.ps1"
$trafficCommand = "& '$trafficScript' -ApiA '$apiA' -ApiB '$apiB' -IntervalMs 1500"
Start-BackgroundShell -WorkingDirectory $scriptRoot -Command $trafficCommand -StdOut $trafficOut -StdErr $trafficErr -PidFile $trafficRun | Out-Null

$endpoints = @(
  "$apiA/health"
  "$apiB/health"
)

$endpointsText = ($endpoints -join [Environment]::NewLine)
Set-Content -Path (Join-Path $runtimeRoot "topology-endpoints.txt") -Value $endpointsText
Set-Content -Path (Join-Path $sessionRoot "topology-endpoints.txt") -Value $endpointsText

if (-not $NoBrowser) {
  $topologyPath = [System.IO.Path]::GetFullPath((Join-Path $repoRoot "tools\network-topology.html"))
  $topologyUri = "file:///" + ($topologyPath -replace "\\", "/")
  $query = "endpoints=" + [System.Uri]::EscapeDataString($endpointsText) + "&intervalMs=1500"
  Start-Process ($topologyUri + "?" + $query) | Out-Null
}

Write-Host "Demo network-manager is running."
Write-Host "Node A API: $apiA"
Write-Host "Node B API: $apiB"
Write-Host "Health endpoints:"
$endpoints | ForEach-Object { Write-Host "  $_" }
Write-Host "Topology helper: $(Join-Path $runtimeRoot 'topology-endpoints.txt')"
Write-Host "Session: $sessionRoot"
Write-Host "Logs: $logsRoot"
Write-Host "Stop command: powershell -ExecutionPolicy Bypass -File .\tools\example-NM-presentation\stop-demo.ps1"
