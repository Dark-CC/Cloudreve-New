param(
    [string]$BaseUrl = "http://127.0.0.1:5212",
    [string]$AdminEmail = "admin@example.com",
    [string]$AdminPassword = "$env:CN_ADMIN_PASSWORD",
    [string]$Password = "{{SECRET_4bd4dxw5}}"
)

$ErrorActionPreference = "Stop"
$checks = [System.Collections.Generic.List[object]]::new()
$stamp = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()

function Invoke-Api {
    param(
        [string]$Method,
        [string]$Path,
        [string]$Token,
        $Body
    )

    $headers = @{}
    if ($Token) {
        $headers["Authorization"] = "Bearer $Token"
    }
    $args = @{
        Method             = $Method
        Uri                = "$BaseUrl$Path"
        Headers            = $headers
        ContentType        = "application/json"
        TimeoutSec         = 60
        SkipHttpErrorCheck = $true
        UseBasicParsing    = $true
    }
    if ($null -ne $Body) {
        $args["Body"] = ($Body | ConvertTo-Json -Depth 20 -Compress)
    }
    $response = Invoke-WebRequest @args
    $json = $null
    try { $json = $response.Content | ConvertFrom-Json } catch {}
    [pscustomobject]@{ status = $response.StatusCode; json = $json; raw = $response.Content; headers = $response.Headers }
}

function Assert-Code {
    param($Response, [int]$Expected, [string]$Label)
    if ($null -eq $Response.json -or $Response.json.code -ne $Expected) {
        throw "$Label expected code $Expected, got $($Response.json.code): $($Response.json.msg) ($($Response.raw))"
    }
    $checks.Add([pscustomobject]@{ check = $Label; result = "PASS"; detail = $Response.json.code })
}

$login = Invoke-Api -Method POST -Path "/api/v4/session/token" -Body @{
    email    = $AdminEmail
    password = $AdminPassword
}
Assert-Code $login 0 "administrator login"
$adminToken = $login.json.data.token.access_token

$email = "audit-archive-$stamp@test.local"
$created = Invoke-Api -Method POST -Path "/api/v4/user" -Body @{
    email    = $email
    password = $Password
    language = "zh-CN"
}
Assert-Code $created 0 "create audit test user"

$userLookup = Invoke-Api -Method POST -Path "/api/v4/admin/user" -Token $adminToken -Body @{
    page        = 1
    page_size   = 50
    order_by    = "id"
    order_direction = "desc"
    conditions  = @{ user_email = $email }
}
Assert-Code $userLookup 0 "lookup audit test user"
$matchedUser = @($userLookup.json.data.users) | Where-Object { $_.email -eq $email } | Select-Object -First 1
if (-not $matchedUser) {
    throw "Audit test user was not found via admin API: $($userLookup.raw)"
}
$userId = [int64]$matchedUser.id

$userLogin = Invoke-Api -Method POST -Path "/api/v4/session/token" -Body @{
    email    = $email
    password = $Password
}
Assert-Code $userLogin 0 "audit test user login"
$userToken = $userLogin.json.data.token.access_token

$folder = "/audit-archive-$stamp"
$mkdir = Invoke-Api -Method POST -Path "/api/v4/file/create" -Token $userToken -Body @{
    uri  = "cloudreve://my$folder"
    type = "folder"
}
Assert-Code $mkdir 0 "create audit fixture folder"

$logs = Invoke-Api -Method POST -Path "/api/v4/admin/event" -Token $adminToken -Body @{
    page        = 1
    page_size   = 100
    order_by    = "id"
    order_direction = "desc"
    conditions  = @{ audit_user_id = "$userId" }
}
Assert-Code $logs 0 "list test user's audit logs"
$testLogIDs = @($logs.json.data.logs | ForEach-Object { [int]$_.id })
if ($testLogIDs.Count -eq 0) {
    throw "No audit logs were recorded for the test user"
}
$fixtureLog = @($logs.json.data.logs | Where-Object { [int]$_.type -eq 7 } | Select-Object -First 1)
if (-not $fixtureLog) {
    throw "No file-created audit event was recorded for the test fixture: $($logs.raw)"
}
$fixtureLogID = [int]$fixtureLog.id
$fixtureTime = [DateTimeOffset]::Parse($fixtureLog.created_at)
$notBefore = $fixtureTime.AddSeconds(-2).ToUniversalTime().ToString("o")
$notAfter = $fixtureTime.AddSeconds(2).ToUniversalTime().ToString("o")
$checks.Add([pscustomobject]@{ check = "audit events recorded"; result = "PASS"; detail = $testLogIDs.Count })

$archive = Invoke-Api -Method POST -Path "/api/v4/admin/event/archive" -Token $adminToken -Body @{
    not_before = $notBefore
    not_after  = $notAfter
    categories = @(7)
}
Assert-Code $archive 0 "create audit archive"
$archiveID = $archive.json.data.id

for ($i = 0; $i -lt 60; $i++) {
    $status = Invoke-Api -Method GET -Path "/api/v4/admin/event/archive/$archiveID/status" -Token $adminToken
    Assert-Code $status 0 "poll audit archive status"
    if ($status.json.data.status -in @("completed", "failed")) {
        break
    }
    Start-Sleep -Milliseconds 500
}
if ($status.json.data.status -ne "completed") {
    throw "Audit archive did not complete: $($status.raw)"
}
$checks.Add([pscustomobject]@{ check = "audit archive completed"; result = "PASS"; detail = $status.json.data.size })

if (-not $status.json.data.url) {
    throw "Audit archive download URL is missing"
}
$download = Invoke-WebRequest -Uri $status.json.data.url -UseBasicParsing -SkipHttpErrorCheck
if ($download.StatusCode -ne 200 -or $download.RawContentLength -le 0) {
    throw "Audit archive download failed: $($download.StatusCode)"
}
$checks.Add([pscustomobject]@{ check = "audit archive downloadable"; result = "PASS"; detail = $download.RawContentLength })

$temp = Join-Path $env:TEMP "audit-archive-$stamp.zip"
[IO.File]::WriteAllBytes($temp, $download.Content)
Add-Type -AssemblyName System.IO.Compression.FileSystem
$zip = [IO.Compression.ZipFile]::OpenRead($temp)
$names = @($zip.Entries | ForEach-Object FullName)
if (-not ($names -contains "events.jsonl") -or -not ($names -contains "manifest.json")) {
    throw "Archive is missing expected entries: $($names -join ",")"
}
$manifestEntry = $zip.GetEntry("manifest.json")
$manifestReader = New-Object System.IO.StreamReader($manifestEntry.Open())
$manifestRaw = $manifestReader.ReadToEnd()
$manifestReader.Dispose()
$manifest = $manifestRaw | ConvertFrom-Json
if ([int]$manifest.count -lt 1) {
    throw "Archive manifest reports no events: $manifestRaw"
}
$eventsEntry = $zip.GetEntry("events.jsonl")
$eventsReader = New-Object System.IO.StreamReader($eventsEntry.Open())
$eventsRaw = $eventsReader.ReadToEnd()
$eventsReader.Dispose()
$archivedIDs = @()
foreach ($line in ($eventsRaw -split "`r?`n" | Where-Object { $_.Trim() -ne "" })) {
    $item = $line | ConvertFrom-Json
    $archivedIDs += [int]$item.id
}
if (-not ($archivedIDs -contains $fixtureLogID)) {
    throw "Archived events do not contain fixture log $fixtureLogID"
}
$zip.Dispose()
Remove-Item -LiteralPath $temp -Force
$checks.Add([pscustomobject]@{ check = "audit archive contents valid"; result = "PASS"; detail = "count=$($manifest.count), ids=$($archivedIDs -join ',')" })

$deleted = Invoke-Api -Method DELETE -Path "/api/v4/admin/event/archive/$archiveID" -Token $adminToken
Assert-Code $deleted 0 "delete archived audit logs"

$remaining = Invoke-Api -Method POST -Path "/api/v4/admin/event" -Token $adminToken -Body @{
    page        = 1
    page_size   = 100
    order_by    = "id"
    order_direction = "desc"
    conditions  = @{ audit_user_id = "$userId" }
}
Assert-Code $remaining 0 "verify archived audit logs removed"
$remainingCategory2 = @($remaining.json.data.logs | Where-Object { $_.content.category -eq 2 })
if ($remainingCategory2.Count -ne 0) {
    throw "Archived file events were not deleted: $($remaining.raw)"
}
$checks.Add([pscustomobject]@{ check = "archive delete removes only range"; result = "PASS"; detail = "0 category 2 remaining" })

$checks | Format-Table -AutoSize
"PASSED: $($checks.Count)"
