param(
    [string]$BaseUrl = "http://127.0.0.1:5212",
    [string]$AdminEmail = "admin@example.com",
    [string]$AdminPassword = "$env:CN_ADMIN_PASSWORD",
    [int]$TargetUserID = 0,
    [string]$ExpectedUserHashID = ""
)

$ErrorActionPreference = "Stop"
$checks = [System.Collections.Generic.List[object]]::new()
$stamp = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
$password = "{{SECRET_4bd4dxw5}}"

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
        TimeoutSec         = 20
        SkipHttpErrorCheck = $true
    }
    if ($null -ne $Body) {
        $args["Body"] = ($Body | ConvertTo-Json -Depth 20 -Compress)
    }
    $response = Invoke-WebRequest @args -UseBasicParsing
    try {
        $json = $response.Content | ConvertFrom-Json
    } catch {
        $json = $null
    }
    return [pscustomobject]@{
        status = $response.StatusCode
        json   = $json
        raw    = $response.Content
    }
}

function Assert-Code {
    param($Response, [int]$Expected, [string]$Label)
    if ($Response.json.code -ne $Expected) {
        throw "$Label expected code $Expected, got $($Response.json.code): $($Response.json.msg) ($($Response.raw))"
    }
    $checks.Add([pscustomobject]@{ check = $Label; result = "PASS"; value = $Response.json.code })
}

function Assert-Denied {
    param($Response, [string]$Label)
    if ($Response.json.code -eq 0) {
        throw "$Label was unexpectedly allowed: $($Response.raw)"
    }
    if ($Response.raw -notmatch "Only owner or administrator") {
        throw "$Label was rejected for an unexpected reason: $($Response.raw)"
    }
    $checks.Add([pscustomobject]@{ check = $Label; result = "PASS"; value = $Response.json.code })
}

function Get-Token {
    param([string]$Email, [string]$LoginPassword)
    $res = Invoke-Api -Method POST -Path "/api/v4/session/token" -Body @{
        email    = $Email
        password = $LoginPassword
    }
    Assert-Code $res 0 "login $Email"
    return $res.json.data.token.access_token
}

function New-TestUser {
    param([string]$Token, [string]$Email, [string]$Nick)
    $res = Invoke-Api -Method PUT -Path "/api/v4/admin/user" -Token $Token -Body @{
        user = @{
            email       = $Email
            nick        = $Nick
            status      = "active"
            group_users = 2
            storage     = 0
        }
        password = $password
    }
    Assert-Code $res 0 "create user $Email"
    return $res.json.data
}

$adminToken = Get-Token -Email $AdminEmail -LoginPassword $AdminPassword
$owner = $null
if ($TargetUserID -gt 0 -or $ExpectedUserHashID) {
    $lookup = Invoke-Api -Method POST -Path "/api/v4/admin/user" -Token $adminToken -Body @{
        page            = 1
        page_size       = 50
        order_by        = "id"
        order_direction = "desc"
        conditions      = @{}
    }
    Assert-Code $lookup 0 "find fixture user"
    $owner = $lookup.json.data.users | Where-Object {
        ($TargetUserID -gt 0 -and [int]$_.id -eq $TargetUserID) -or
        ($ExpectedUserHashID -and $_.hash_id -eq $ExpectedUserHashID)
    } | Select-Object -First 1
    if (-not $owner) {
        throw "Fixture user $TargetUserID/$ExpectedUserHashID was not found"
    }
} else {
    $owner = New-TestUser -Token $adminToken -Email "admin-my-owner-$stamp@test.local" -Nick "admin-my-owner-$stamp"
}

$ownerToken = Get-Token -Email $owner.email -LoginPassword $password
$folderName = "admin-my-$stamp"
$ownerRoot = "cloudreve://my"
Assert-Code (Invoke-Api -Method POST -Path "/api/v4/file/create" -Token $ownerToken -Body @{
    uri  = "$ownerRoot/$folderName"
    type = "folder"
}) 0 "owner creates fixture folder"

$fileName = "admin-my-$stamp.txt"
$filePath = "$ownerRoot/$folderName/$fileName"
$chunk = [Text.Encoding]::UTF8.GetBytes("admin my access $stamp")
$upload = Invoke-Api -Method PUT -Path "/api/v4/file/upload" -Token $ownerToken -Body @{
    uri           = $filePath
    size          = $chunk.Length
    last_modified = 0
    mime_type     = "text/plain"
}
Assert-Code $upload 0 "owner prepares upload"
$chunkResponse = Invoke-WebRequest -Method POST `
    -Uri "$BaseUrl/api/v4/file/upload/$($upload.json.data.session_id)/0" `
    -Headers @{ Authorization = "Bearer $ownerToken" } `
    -ContentType "application/octet-stream" `
    -Body $chunk `
    -UseBasicParsing
Assert-Code ([pscustomobject]@{
    status = $chunkResponse.StatusCode
    json   = ($chunkResponse.Content | ConvertFrom-Json)
    raw    = $chunkResponse.Content
}) 0 "owner uploads fixture file"

$adminRoot = "cloudreve://$($owner.hash_id)@my"
$adminList = Invoke-Api -Method GET -Path ("/api/v4/file?uri=" + [uri]::EscapeDataString($adminRoot)) -Token $adminToken
Assert-Code $adminList 0 "administrator lists target user's root"
$rootFolder = @($adminList.json.data.files) | Where-Object { $_.name -eq $folderName } | Select-Object -First 1
if (-not $rootFolder) {
    throw "Administrator did not see the target user's fixture folder"
}
$checks.Add([pscustomobject]@{ check = "administrator sees target root entry"; result = "PASS"; value = $rootFolder.id })

$adminFolder = "$adminRoot/$folderName"
$folderList = Invoke-Api -Method GET -Path ("/api/v4/file?uri=" + [uri]::EscapeDataString($adminFolder)) -Token $adminToken
Assert-Code $folderList 0 "administrator lists target subfolder"
$targetFile = @($folderList.json.data.files) | Where-Object { $_.name -eq $fileName } | Select-Object -First 1
if (-not $targetFile) {
    throw "Administrator did not see the target user's fixture file"
}
$checks.Add([pscustomobject]@{ check = "administrator sees target file"; result = "PASS"; value = $targetFile.id })

$adminFile = "$adminFolder/$fileName"
$fileInfo = Invoke-Api -Method GET -Path ("/api/v4/file/info?uri=" + [uri]::EscapeDataString($adminFile)) -Token $adminToken
Assert-Code $fileInfo 0 "administrator reads target file info"
if ($fileInfo.json.data.size -ne $chunk.Length) {
    throw "Administrator file info size $($fileInfo.json.data.size) != $($chunk.Length)"
}

$adminCapacity = Invoke-Api -Method GET -Path "/api/v4/user/capacity" -Token $adminToken
Assert-Code $adminCapacity 0 "administrator session remains healthy"

$createDenied = Invoke-Api -Method POST -Path "/api/v4/file/create" -Token $adminToken -Body @{
    uri  = "$adminRoot/admin-write-$stamp"
    type = "folder"
}
Assert-Code $createDenied 40082 "administrator cannot create in target my"

$renameDenied = Invoke-Api -Method POST -Path "/api/v4/file/rename" -Token $adminToken -Body @{
    uri      = $adminFile
    new_name = "renamed-$stamp.txt"
}
Assert-Code $renameDenied 40082 "administrator cannot rename in target my"

$deleteDenied = Invoke-Api -Method DELETE -Path "/api/v4/file" -Token $adminToken -Body @{
    uris = @($adminFile)
}
Assert-Denied $deleteDenied "administrator cannot delete in target my"

$memberToken = Get-Token -Email $owner.email -LoginPassword $password
$memberOtherRoot = "cloudreve://$($owner.hash_id)@my"
$selfList = Invoke-Api -Method GET -Path ("/api/v4/file?uri=" + [uri]::EscapeDataString($memberOtherRoot)) -Token $memberToken
Assert-Code $selfList 0 "owner can still list own my"

$other = New-TestUser -Token $adminToken -Email "admin-my-other-$stamp@test.local" -Nick "admin-my-other-$stamp"
$otherToken = Get-Token -Email $other.email -LoginPassword $password
$foreign = Invoke-Api -Method GET -Path ("/api/v4/file?uri=" + [uri]::EscapeDataString($memberOtherRoot)) -Token $otherToken
if ($foreign.json.code -eq 0) {
    throw "Non-admin user was able to list another user's my root"
}
$checks.Add([pscustomobject]@{ check = "non-admin cannot list another my"; result = "PASS"; value = $foreign.json.code })

$auditCases = @(
    @{ label = "numeric audit user filter"; conditions = @{ audit_log_user = [string]$owner.id } },
    @{ label = "hashid audit user filter"; conditions = @{ audit_log_user = [string]$owner.hash_id } }
)
foreach ($case in $auditCases) {
    $res = Invoke-Api -Method POST -Path "/api/v4/admin/event" -Token $adminToken -Body @{
        page            = 1
        page_size       = 50
        order_by        = "id"
        order_direction = "desc"
        conditions      = $case.conditions
    }
    Assert-Code $res 0 $case.label
    $match = @($res.json.data.logs) | Where-Object {
        [string]$_.user_hash_id -eq [string]$owner.hash_id
    } | Select-Object -First 1
    if (-not $match) {
        throw "$($case.label) returned no audit entries for the target user"
    }
}

$invalid = Invoke-Api -Method POST -Path "/api/v4/admin/event" -Token $adminToken -Body @{
    page            = 1
    page_size       = 50
    order_by        = "id"
    order_direction = "desc"
    conditions      = @{ audit_log_user = "not-a-hashid" }
}
Assert-Code $invalid 40001 "invalid audit user hash rejected"

[pscustomobject]@{
    checks              = $checks.Count
    target_user_id      = $owner.id
    target_user_hash_id = $owner.hash_id
    fixture_file        = $adminFile
    allowed             = "list root, list subfolder, read file info"
    denied              = "create, rename, delete"
} | Format-List
