param(
    [string]$BaseUrl = "http://127.0.0.1:5212",
    [string]$AdminEmail = "admin@example.com",
    [string]$AdminPassword = "$env:CN_ADMIN_PASSWORD"
)

$ErrorActionPreference = "Stop"
$checks = [System.Collections.Generic.List[object]]::new()

function Invoke-Api {
    param(
        [string]$Method,
        [string]$Path,
        [string]$Token,
        $Body,
        [hashtable]$Headers = @{},
        [switch]$Raw
    )

    $requestHeaders = @{}
    foreach ($key in $Headers.Keys) {
        $requestHeaders[$key] = $Headers[$key]
    }
    if ($Token) {
        $requestHeaders["Authorization"] = "Bearer $Token"
    }

    $args = @{
        Method      = $Method
        Uri         = "$BaseUrl$Path"
        Headers     = $requestHeaders
        ContentType = "application/json"
        TimeoutSec  = 20
        SkipHttpErrorCheck = $true
    }
    if ($null -ne $Body) {
        $args["Body"] = ($Body | ConvertTo-Json -Depth 30 -Compress)
    }

    if ($Raw) {
        return Invoke-WebRequest @args -UseBasicParsing
    }

    $response = Invoke-WebRequest @args
    try {
        return $response.Content | ConvertFrom-Json
    } catch {
        throw "HTTP $($response.StatusCode) for $Method $Path`: $($response.Content)"
    }
}

function Assert-True {
    param([bool]$Condition, [string]$Label, $Value = "")
    if (-not $Condition) {
        throw "$Label failed (value: $Value)"
    }
    $checks.Add([pscustomobject]@{ check = $Label; result = "PASS"; value = $Value })
}

function Assert-Code {
    param($Response, [int]$Expected, [string]$Label)
    if ($Response.code -ne $Expected) {
        throw "$Label expected code $Expected, got $($Response.code): $($Response.msg)"
    }
    $checks.Add([pscustomobject]@{ check = $Label; result = "PASS"; value = $Response.code })
}

function Get-Token {
    param([string]$Email, [string]$Password)
    $res = Invoke-Api -Method POST -Path "/api/v4/session/token" -Body @{
        email    = $Email
        password = $Password
    }
    Assert-Code $res 0 "login $Email"
    return $res.data.token.access_token
}

function Get-Credit {
    param([string]$Token)
    $res = Invoke-Api -Method GET -Path "/api/v4/user/credit" -Token $Token
    Assert-Code $res 0 "read credit"
    return [int64]$res.data.credit
}

function Get-Settings {
    param([string]$Token, [string[]]$Keys)
    $res = Invoke-Api -Method POST -Path "/api/v4/admin/settings" -Token $Token -Body @{ keys = $Keys }
    Assert-Code $res 0 "read settings"
    return $res.data
}

$adminToken = Get-Token -Email $AdminEmail -Password $AdminPassword

$vas = Get-Settings -Token $adminToken -Keys @("vas_credit_enabled", "vas_local_payment_enabled", "vas_credit_per_unit")
Assert-True ($vas.vas_credit_enabled -eq "1") "VAS enabled" $vas.vas_credit_enabled
Assert-True ($vas.vas_local_payment_enabled -eq "1") "local payment enabled" $vas.vas_local_payment_enabled

$creditBefore = Get-Credit -Token $adminToken
$local = Invoke-Api -Method PUT -Path "/api/v4/vas/payment" -Token $adminToken -Body @{
    product_type = 4
    provider_id  = "local"
    qyt          = 3
}
Assert-Code $local 0 "create local credit order"
Assert-True ($local.data.status -eq "fulfilled") "local order fulfilled" $local.data.status
Assert-True ($local.data.price_unit -eq ($local.data.price_one_unit * 3)) "local order total" $local.data.price_unit
$creditAfter = Get-Credit -Token $adminToken
Assert-True ($creditAfter -eq ($creditBefore + 3)) "local order credited" "$creditBefore -> $creditAfter"

$orders = Invoke-Api -Method POST -Path "/api/v4/admin/payment" -Token $adminToken -Body @{
    page            = 1
    page_size       = 10
    order_by        = "id"
    order_direction = "desc"
    conditions      = @{ payment_trade_no = $local.data.trade_no }
}
Assert-Code $orders 0 "list admin payments"
$orderMatches = @($orders.data.payments | Where-Object { $_.trade_no -eq $local.data.trade_no })
Assert-True ($orderMatches.Count -eq 1) "order appears in admin list" $orderMatches.Count
Assert-True ($orderMatches[0].creator_hash_id -ne "") "order has creator hash id" $orderMatches[0].creator_hash_id

$repeat = Invoke-Api -Method POST -Path "/api/v4/admin/payment/$($local.data.id)/fulfill" -Token $adminToken
Assert-Code $repeat 0 "repeat fulfillment is idempotent"
$creditRepeat = Get-Credit -Token $adminToken
Assert-True ($creditRepeat -eq $creditAfter) "repeat fulfillment does not double credit" "$creditAfter -> $creditRepeat"

$pointsBefore = Get-Credit -Token $adminToken
$points = Invoke-Api -Method PUT -Path "/api/v4/vas/payment" -Token $adminToken -Body @{
    product_type  = 4
    provider_id   = "points"
    qyt           = 2
    payment_method = "points"
}
if ($points.code -eq 0) {
    Assert-True ($points.data.status -eq "fulfilled") "points order fulfilled" $points.data.status
    $pointsAfter = Get-Credit -Token $adminToken
    Assert-True ($pointsAfter -eq ($pointsBefore + 2)) "points order credited" "$pointsBefore -> $pointsAfter"
} else {
    Assert-True ($points.code -eq 40001 -or $points.code -eq 40002) "points order rejected without points price" "$($points.code) $($points.msg)"
}

$tradeNo = $local.data.trade_no
$status = Invoke-Api -Method GET -Path "/api/v4/vas/payment/status/$tradeNo/bad-sign"
Assert-Code $status 403 "invalid status signature rejected"

$createForCallback = Invoke-Api -Method PUT -Path "/api/v4/vas/payment" -Token $adminToken -Body @{
    product_type = 4
    provider_id  = "local"
    qyt          = 1
}
Assert-Code $createForCallback 0 "callback fixture order created"

$record = Invoke-Api -Method POST -Path "/api/v4/admin/event" -Token $adminToken -Body @{
    page            = 1
    page_size       = 1
    order_by        = "id"
    order_direction = "desc"
    conditions      = @{}
}
Assert-Code $record 0 "list events"
Assert-True (@($record.data.logs).Count -ge 1) "events are readable" @($record.data.logs).Count
$eventID = [int]$record.data.logs[0].id
$event = Invoke-Api -Method GET -Path "/api/v4/admin/event/$eventID" -Token $adminToken
Assert-Code $event 0 "event detail"
Assert-True ($event.data.id -eq $eventID) "event detail id" $event.data.id

$archive = Invoke-Api -Method POST -Path "/api/v4/admin/event/archive" -Token $adminToken -Body @{
    not_before = (Get-Date).AddDays(-2).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    not_after  = (Get-Date).AddDays(1).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    categories = @()
}
Assert-Code $archive 0 "create event archive"
$archiveID = [string]$archive.data.id
$archiveStatus = $archive.data
for ($i = 0; $i -lt 40; $i++) {
    if ($archiveStatus.status -eq "completed" -or $archiveStatus.status -eq "failed") {
        break
    }
    Start-Sleep -Milliseconds 250
    $archiveStatus = (Invoke-Api -Method GET -Path "/api/v4/admin/event/archive/$archiveID/status" -Token $adminToken).data
}
Assert-True ($archiveStatus.status -eq "completed") "event archive completed" $archiveStatus.status
Assert-True ([string]$archiveStatus.url -ne "") "event archive signed url" $archiveStatus.url
Assert-True ([int64]$archiveStatus.size -gt 0) "event archive non-empty" $archiveStatus.size

$download = Invoke-Api -Method GET -Path ([uri]$archiveStatus.url).PathAndQuery -Raw
Assert-True ($download.StatusCode -eq 200) "event archive download" $download.StatusCode
Assert-True ([string]$download.Headers["Content-Type"] -match "zip") "event archive content type" ([string]$download.Headers["Content-Type"])

$badRange = Invoke-Api -Method POST -Path "/api/v4/admin/event/archive" -Token $adminToken -Body @{
    not_before = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    not_after  = (Get-Date).AddDays(-1).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
}
Assert-Code $badRange 40001 "invalid archive range rejected"

$delete = Invoke-Api -Method DELETE -Path "/api/v4/admin/event/archive/$archiveID" -Token $adminToken
Assert-Code $delete 0 "delete event archive"
$deletedStatus = Invoke-Api -Method GET -Path "/api/v4/admin/event/archive/$archiveID/status" -Token $adminToken
Assert-Code $deletedStatus 404 "deleted archive no longer found"

$cleanup = Invoke-Api -Method POST -Path "/api/v4/admin/event/cleanup" -Token $adminToken -Body @{
    not_after  = (Get-Date).AddDays(-30).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    categories = @()
}
Assert-Code $cleanup 0 "cleanup old events"

[pscustomobject]@{
    checks         = $checks.Count
    local_trade_no = $local.data.trade_no
    local_credit    = "$creditBefore -> $creditAfter"
    archive_id      = $archiveID
    archive_size    = $archiveStatus.size
} | Format-List

