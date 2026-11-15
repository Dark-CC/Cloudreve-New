param(
    [string]$BaseUrl = "http://127.0.0.1:5212",
    [string]$AdminEmail = "admin@example.com",
    [string]$AdminPassword = "$env:CN_ADMIN_PASSWORD"
)

$ErrorActionPreference = "Stop"
$stamp = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
$password = "{{SECRET_4bd4dxw5}}"

function Invoke-Api {
    param([string]$Method, [string]$Path, [string]$Token, $Body)

    $headers = @{}
    if ($Token) {
        $headers["Authorization"] = "Bearer $Token"
    }
    $args = @{
        Method      = $Method
        Uri         = "$BaseUrl$Path"
        Headers     = $headers
        ContentType = "application/json"
    }
    if ($null -ne $Body) {
        $args["Body"] = ($Body | ConvertTo-Json -Depth 20 -Compress)
    }
    Invoke-RestMethod @args
}

function Get-Token {
    param([string]$Email, [string]$LoginPassword)
    $res = Invoke-Api -Method POST -Path "/api/v4/session/token" -Body @{
        email    = $Email
        password = $LoginPassword
    }
    if ($res.code -ne 0) {
        throw "Login failed for ${Email}: $($res.code) $($res.msg)"
    }
    return $res.data.token.access_token
}

function Assert-Code {
    param($Response, [int]$Expected, [string]$Label)
    if ($Response.code -ne $Expected) {
        throw "$Label expected code $Expected, got $($Response.code): $($Response.msg)"
    }
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
    return $res.data
}

$adminToken = Get-Token -Email $AdminEmail -LoginPassword $AdminPassword
$owner = New-TestUser -Token $adminToken -Email "acl-hash-owner-$stamp@test.local" -Nick "acl-hash-owner-$stamp"
$viewer = New-TestUser -Token $adminToken -Email "acl-hash-viewer-$stamp@test.local" -Nick "acl-hash-viewer-$stamp"
if (-not $viewer.hash_id) {
    throw "Admin user response did not include hash_id"
}

$ownerToken = Get-Token -Email $owner.email -LoginPassword $password
$viewerToken = Get-Token -Email $viewer.email -LoginPassword $password
$folder = "cloudreve://my/acl-hash-$stamp"
Assert-Code (Invoke-Api -Method POST -Path "/api/v4/file/create" -Token $ownerToken -Body @{
    uri  = $folder
    type = "folder"
}) 0 "create folder"

$fileName = "hash-target-$stamp.txt"
$filePath = "$folder/$fileName"
$chunk = [Text.Encoding]::UTF8.GetBytes("hashid acl $stamp")
$upload = Invoke-Api -Method PUT -Path "/api/v4/file/upload" -Token $ownerToken -Body @{
    uri           = $filePath
    size          = $chunk.Length
    last_modified = 0
    mime_type     = "text/plain"
}
Assert-Code $upload 0 "prepare upload"
$chunkRes = Invoke-WebRequest -Method POST `
    -Uri "$BaseUrl/api/v4/file/upload/$($upload.data.session_id)/0" `
    -Headers @{ Authorization = "Bearer $ownerToken" } `
    -ContentType "application/octet-stream" `
    -Body $chunk `
    -UseBasicParsing
Assert-Code ($chunkRes.Content | ConvertFrom-Json) 0 "upload chunk"

$shareRes = Invoke-Api -Method PUT -Path "/api/v4/share" -Token $ownerToken -Body @{
    uri         = $filePath
    is_private  = $false
    share_view  = $true
    show_readme = $false
    downloads   = 0
    expire      = 0
    price       = 0
}
Assert-Code $shareRes 0 "create share"
$shareID = ([string]$shareRes.data -split "/")[-1]
$fileURI = "cloudreve://$shareID@share/$fileName"

$read = @{
    can_list     = $true
    can_read     = $true
    can_update   = $false
    can_create   = $false
    can_delete   = $false
    can_share    = $false
    discoverable = $false
}
$deny = @{
    can_list     = $false
    can_read     = $false
    can_update   = $false
    can_create   = $false
    can_delete   = $false
    can_share    = $false
    discoverable = $false
}

$acl = @{
    builtin = @(
        @{ audience = "anonymous"; permission = $deny },
        @{ audience = "logged_in"; permission = $deny },
        @{ audience = "same_group"; permission = $deny },
        @{ audience = "other_group"; permission = $deny }
    )
    group = @()
    user  = @(
        @{
            user_id      = 0
            user_hash_id = $viewer.hash_id
            permission   = $read
        }
    )
}

$save = Invoke-Api -Method PUT -Path "/api/v4/share/$shareID/acl" -Token $ownerToken -Body @{ acl = $acl }
Assert-Code $save 0 "save ACL with user_hash_id"
$stored = $save.data.user | Where-Object { $_.user_hash_id -eq $viewer.hash_id }
if (-not $stored) {
    throw "Saved ACL did not return user_hash_id $($viewer.hash_id)"
}
if ([int]$stored.user_id -ne [int]$viewer.id) {
    throw "Saved ACL user_id $($stored.user_id) did not resolve to $($viewer.id)"
}

$lookup = Invoke-Api -Method GET -Path "/api/v4/share/$shareID/acl" -Token $ownerToken
Assert-Code $lookup 0 "read ACL"
$stored = $lookup.data.user | Where-Object { $_.user_hash_id -eq $viewer.hash_id }
if (-not $stored) {
    throw "GET ACL did not return user_hash_id $($viewer.hash_id)"
}

$viewerInfo = Invoke-Api -Method GET -Path ("/api/v4/file/info?uri=" + [uri]::EscapeDataString($fileURI)) -Token $viewerToken
Assert-Code $viewerInfo 0 "explicit HashID user rule grants access"

$mismatch = @{
    builtin = @()
    group   = @()
    user    = @(
        @{
            user_id      = [int]$owner.id
            user_hash_id = $viewer.hash_id
            permission   = $read
        }
    )
}
$mismatchRes = Invoke-Api -Method PUT -Path "/api/v4/share/$shareID/acl" -Token $ownerToken -Body @{ acl = $mismatch }
if ($mismatchRes.code -eq 0) {
    throw "ACL accepted mismatched user_id and user_hash_id"
}

$anonymousGroup = @{
    builtin = @()
    group   = @(
        @{ group_id = 3; permission = $read }
    )
    user    = @()
}
$anonymousRes = Invoke-Api -Method PUT -Path "/api/v4/share/$shareID/acl" -Token $ownerToken -Body @{ acl = $anonymousGroup }
if ($anonymousRes.code -eq 0) {
    throw "ACL accepted the anonymous group as an explicit group rule"
}

$clear = @{ builtin = @(); group = @(); user = @() }
Assert-Code (Invoke-Api -Method PUT -Path "/api/v4/share/$shareID/acl" -Token $ownerToken -Body @{ acl = $clear }) 0 "clear ACL"

[pscustomobject]@{
    share_id              = $shareID
    viewer_hash_id        = $viewer.hash_id
    viewer_numeric_id     = $viewer.id
    stored_user_hash_id   = $stored.user_hash_id
    stored_user_numeric_id = $stored.user_id
    access_granted        = $viewerInfo.code -eq 0
    mismatch_rejected     = $mismatchRes.code -ne 0
    anonymous_group_rejected = $anonymousRes.code -ne 0
} | Format-List
