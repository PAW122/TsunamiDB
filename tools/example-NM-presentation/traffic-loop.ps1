param(
  [Parameter(Mandatory = $true)]
  [string]$ApiA,

  [Parameter(Mandatory = $true)]
  [string]$ApiB,

  [int]$IntervalMs = 1500
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$orderStatuses = @("created", "packing", "shipped", "delivered")
$regions = @("eu-central", "eu-west", "pl-south", "de-berlin")
$warehouses = @("wroclaw-01", "poznan-02", "berlin-03")
$counter = 0

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

while ($true) {
  try {
    $timestamp = (Get-Date).ToUniversalTime().ToString("o")
    $status = $orderStatuses[$counter % $orderStatuses.Count]
    $region = $regions[$counter % $regions.Count]
    $warehouse = $warehouses[$counter % $warehouses.Count]

    Save-Json -BaseUrl $ApiA -Table "users" -Key "user-1" -Payload @{
      customer_id = "cust-1001"
      name = "Anna Kowalska"
      tier = "pro"
      region = $region
      active_order = "ord-2026-0001"
      last_seen = $timestamp
    }

    Save-Json -BaseUrl $ApiB -Table "orders" -Key "order-1" -Payload @{
      order_id = "ord-2026-0001"
      customer_id = "cust-1001"
      status = $status
      warehouse = $warehouse
      amount_cents = 129900 + ($counter * 25)
      updated_at = $timestamp
    }

    $remoteOrder = Invoke-RestMethod -Uri "$ApiA/read/orders/order-1" -TimeoutSec 3
    $remoteUser = Invoke-RestMethod -Uri "$ApiB/read/users/user-1" -TimeoutSec 3

    Write-Output ("[{0}] cross-read order.status={1} user.region={2}" -f $timestamp, $remoteOrder.status, $remoteUser.region)
    $counter++
  } catch {
    Write-Output ("[{0}] traffic loop error: {1}" -f (Get-Date).ToUniversalTime().ToString("o"), $_.Exception.Message)
  }

  Start-Sleep -Milliseconds $IntervalMs
}
