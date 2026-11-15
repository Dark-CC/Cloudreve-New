param(
    [string]$BaseUrl = "http://127.0.0.1:5212",
    [string]$AdminEmail = "admin@example.com",
    [string]$AdminPassword = "$env:CN_ADMIN_PASSWORD",
    [string]$MemberEmail = "member@example.com",
    [string]$MemberPassword = "$env:CN_MEMBER_PASSWORD",
    [int]$PolicyID = 1
)

$ErrorActionPreference = "Stop"
$checks = [System.Collections.Generic.List[object]]::new()
$stamp = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
$fixture = Join-Path (Split-Path $PSScriptRoot -Parent) "runtime\\import-fixture"
$tmp = "admin-import-$stamp"
$password = "{{SECRET_4bd4dxw5}}"

function Invoke-Api {
    param(
        [string]$Method,
        [string]$Path,
        [string]$Token,
        $Body,
        [switch]$Raw
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
        TimeoutSec         = 30
        SkipHttpErrorCheck = $true
        UseBasicParsing    = $true
    }
    if ($null -ne $Body) {
        $args["Body"] = ($Body | ConvertTo-Json -Depth 20 -Compress)
    }
    $response = Invoke-WebRequest @args
    if ($Raw) {
        return [pscustomobject]@{ status = $response.StatusCode; raw = $response.Content; json = $null }
    }
    $json = $null
    try { $json = $response.Content | ConvertFrom-Json } catch {}
    return [pscustomobject]@{ status = $response.StatusCode; json = $json; raw = $response.Content }
}

function Assert-Code {
    param($Response, [int]$Expected, [string]$Label)
    if ($null -eq $Response.json -or $Response.json.code -ne $Expected) {
        throw "$Label expected code $Expected, got $($Response.json.code): $($Response.json.msg) ($($Response.raw))"
    }
    $checks.Add([pscustomobject]@{ check = $Label; result = "PASS"; detail = $Response.json.code })
}

function Get-Token {
    param([string]$Email, [string]$LoginPassword)
    $res = Invoke-Api -Method POST -Path "/api/v4/session/token" -Body @{ email = $Email; password = $LoginPassword }
    Assert-Code $res 0 "login $Email"
    return $res.json.data.token.access_token
}

function Wait-ImportTask {
    param([string]$Token, [string]$TaskID)
    for ($i = 0; $i -lt 30; $i++) {
        $res = Invoke-Api -Method GET -Path "/api/v4/workflow?page_size=50&category=general" -Token $Token
        Assert-Code $res 0 "list import tasks"
        $task = $res.json.data.tasks | Where-Object { $_.id -eq $TaskID } | Select-Object -First 1
        if ($task -and $task.status -in @("completed", "error", "canceled")) {
            return $task
        }
        Start-Sleep -Seconds 1
    }
    throw "Import task $TaskID did not finish"
}

New-Item -ItemType Directory -Path (Join-Path $fixture "nested") -Force | Out-Null
[System.IO.File]::WriteAllText(
    (Join-Path $fixture "hello.txt"),
    "import hello $stamp",
    (New-Object System.Text.UTF8Encoding($false))
)
[System.IO.File]::WriteAllText(
    (Join-Path $fixture "nested\deep.txt"),
    "deep hello $stamp",
    (New-Object System.Text.UTF8Encoding($false))
)

$adminToken = Get-Token -Email $AdminEmail -LoginPassword $AdminPassword
$memberToken = Get-Token -Email $MemberEmail -LoginPassword $MemberPassword

$owner = Invoke-Api -Method PUT -Path "/api/v4/admin/user" -Token $adminToken -Body @{
    user = @{
        email       = "admin-import-$stamp@test.local"
        nick        = "admin-import-$stamp"
        status      = "active"
        group_users = 2
        storage     = 0
    }
    password = $password
}
Assert-Code $owner 0 "create import target user"
$ownerID = $owner.json.data.id
$ownerHashID = $owner.json.data.hash_id
$ownerToken = Get-Token -Email $owner.json.data.email -LoginPassword $password

$denied = Invoke-Api -Method POST -Path "/api/v4/workflow/import" -Token $memberToken -Body @{
    src              = $fixture
    dst              = "/$tmp"
    extract_media_meta = $true
    user_id          = $ownerHashID
    recursive        = $true
    policy_id        = $PolicyID
}
if ($denied.json.code -eq 0) {
    throw "Non-admin import was unexpectedly allowed: $($denied.raw)"
}
if ($denied.json.code -ne 403) {
    throw "Non-admin import should be rejected with code 403, got $($denied.json.code): $($denied.raw)"
}
$checks.Add([pscustomobject]@{ check = "non-admin import rejected"; result = "PASS"; detail = $denied.json.code })

$create = Invoke-Api -Method POST -Path "/api/v4/workflow/import" -Token $adminToken -Body @{
    src              = $fixture
    dst              = "/$tmp"
    extract_media_meta = $true
    user_id          = $ownerHashID
    recursive        = $true
    policy_id        = $PolicyID
}
Assert-Code $create 0 "administrator creates import task"
if ($create.json.data.summary.props.dst -ne "cloudreve://$ownerHashID@my/$tmp") {
    throw "Unexpected import destination: $($create.json.data.summary.props.dst)"
}
$checks.Add([pscustomobject]@{
    check = "import destination normalized"
    result = "PASS"
    detail = $create.json.data.summary.props.dst
})

$finished = Wait-ImportTask -Token $ownerToken -TaskID $create.json.data.id
if ($finished.status -ne "completed" -or $finished.summary.props.failed -ne 0) {
    throw "Import task did not complete cleanly: $($finished | ConvertTo-Json -Depth 12 -Compress)"
}
$checks.Add([pscustomobject]@{ check = "import task completed"; result = "PASS"; detail = $finished.id })

$rootUri = "cloudreve://$ownerHashID@my/$tmp"
$rootList = Invoke-Api -Method GET -Path ("/api/v4/file?uri=" + [uri]::EscapeDataString($rootUri)) -Token $ownerToken
Assert-Code $rootList 0 "target user lists imported root"
$hello = $rootList.json.data.files | Where-Object name -eq "hello.txt" | Select-Object -First 1
$nested = $rootList.json.data.files | Where-Object { $_.name -eq "nested" -and $_.type -eq 1 } | Select-Object -First 1
if (-not $hello) { throw "hello.txt was not imported" }
if (-not $nested) { throw "nested folder was not imported" }
$checks.Add([pscustomobject]@{ check = "target user sees imported file"; result = "PASS"; detail = $hello.id })
$checks.Add([pscustomobject]@{ check = "target user sees imported folder"; result = "PASS"; detail = $nested.id })

$deepUri = "$rootUri/nested"
$deepList = Invoke-Api -Method GET -Path ("/api/v4/file?uri=" + [uri]::EscapeDataString($deepUri)) -Token $ownerToken
Assert-Code $deepList 0 "target user lists imported subfolder"
$deep = $deepList.json.data.files | Where-Object name -eq "deep.txt" | Select-Object -First 1
if (-not $deep) { throw "nested/deep.txt was not imported" }
$checks.Add([pscustomobject]@{ check = "recursive import preserved"; result = "PASS"; detail = $deep.id })

$helloInfo = Invoke-Api -Method GET -Path ("/api/v4/file/info?uri=" + [uri]::EscapeDataString("$rootUri/hello.txt")) -Token $ownerToken
Assert-Code $helloInfo 0 "target user reads imported file info"
if ($helloInfo.json.data.size -le 0) { throw "Imported file is empty" }
$checks.Add([pscustomobject]@{ check = "imported file readable"; result = "PASS"; detail = $helloInfo.json.data.size })

$foreignCreate = Invoke-Api -Method PUT -Path "/api/v4/admin/user" -Token $adminToken -Body @{
    user = @{
        email       = "admin-import-viewer-$stamp@test.local"
        nick        = "admin-import-viewer-$stamp"
        status      = "active"
        group_users = 2
        storage     = 0
    }
    password = $password
}
Assert-Code $foreignCreate 0 "create unrelated user"
$viewerToken = Get-Token -Email $foreignCreate.json.data.email -LoginPassword $password
$foreign = Invoke-Api -Method GET -Path ("/api/v4/file?uri=" + [uri]::EscapeDataString("cloudreve://$ownerHashID@my/$tmp")) -Token $viewerToken
if ($foreign.json.code -eq 0) {
    throw "Unrelated user can access import destination"
}
$checks.Add([pscustomobject]@{ check = "import destination remains private"; result = "PASS"; detail = $foreign.json.code })

$checks | Format-Table -AutoSize
"PASSED: $($checks.Count)"
