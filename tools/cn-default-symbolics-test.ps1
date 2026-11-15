param(
    [string]$BaseUrl = "http://127.0.0.1:5212",
    [string]$AdminEmail = "admin@example.com",
    [string]$AdminPassword = "$env:CN_ADMIN_PASSWORD"
)

$ErrorActionPreference = "Stop"
$checks = [System.Collections.Generic.List[object]]::new()
$stamp = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
$password = "{{SECRET_4bd4dxw5}}"
$originalSetting = $null

function Invoke-Api {
    param([string]$Method, [string]$Path, [string]$Token, $Body)

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
    $json = $null
    try { $json = $response.Content | ConvertFrom-Json } catch {}
    [pscustomobject]@{ status = $response.StatusCode; json = $json; raw = $response.Content }
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
    $res.json.data.token.access_token
}

$adminToken = Get-Token -Email $AdminEmail -LoginPassword $AdminPassword

try {
    $current = Invoke-Api -Method POST -Path "/api/v4/admin/settings" -Token $adminToken -Body @{ keys = @("default_symbolics") }
    Assert-Code $current 0 "read default_symbolics"
    $originalSetting = [string]$current.json.data.default_symbolics

    $ownerEmail = "default-symbol-owner-$stamp@test.local"
    $owner = Invoke-Api -Method PUT -Path "/api/v4/admin/user" -Token $adminToken -Body @{
        user = @{
            email       = $ownerEmail
            nick        = "default-symbol-owner-$stamp"
            status      = "active"
            group_users = 2
            storage     = 0
        }
        password = $password
    }
    Assert-Code $owner 0 "create shortcut source user"
    $ownerToken = Get-Token -Email $ownerEmail -LoginPassword $password

    $root = Invoke-Api -Method GET -Path ("/api/v4/file?uri=" + [uri]::EscapeDataString("cloudreve://my") + "&page_size=20") -Token $ownerToken
    Assert-Code $root 0 "initialize shortcut source root"

    $folderName = "default-symbol-target-$stamp"
    $folder = Invoke-Api -Method POST -Path "/api/v4/file/create" -Token $ownerToken -Body @{
        uri  = "cloudreve://my/$folderName"
        type = "folder"
    }
    Assert-Code $folder 0 "create shortcut source folder"

    $share = Invoke-Api -Method PUT -Path "/api/v4/share" -Token $ownerToken -Body @{
        uri         = "cloudreve://my/$folderName"
        is_private  = $false
        expire      = 0
        downloads   = 0
        share_view  = $true
        show_readme = $false
        price       = 0
    }
    Assert-Code $share 0 "create share for default shortcut"
    $shareUrl = [string]$share.json.data
    if ($shareUrl -notmatch "/s/([^/?#]+)") {
        throw "Unexpected share URL: $shareUrl"
    }
    $shareHashID = $Matches[1]

    $adminShares = Invoke-Api -Method POST -Path "/api/v4/admin/share" -Token $adminToken -Body @{
        page      = 1
        page_size = 1000
        order_by  = "id"
        order_direction = "desc"
    }
    Assert-Code $adminShares 0 "resolve share id for default shortcut"
    $shareEntry = @($adminShares.json.data.shares) |
        Where-Object { [string]$_.share_link -match [regex]::Escape($shareHashID) } |
        Select-Object -First 1
    if (-not $shareEntry) {
        throw "Share $shareHashID was not found in the admin share list"
    }
    $shareNumericID = [int]$shareEntry.id
    if ($shareNumericID -le 0) {
        throw "Invalid numeric share ID: $($shareEntry.id)"
    }

    $set = Invoke-Api -Method PATCH -Path "/api/v4/admin/settings" -Token $adminToken -Body @{
        settings = @{ default_symbolics = (ConvertTo-Json -InputObject @($shareNumericID) -Compress) }
    }
    Assert-Code $set 0 "set default_symbolics"

    $newEmail = "default-symbol-new-$stamp@test.local"
    $newUser = Invoke-Api -Method PUT -Path "/api/v4/admin/user" -Token $adminToken -Body @{
        user = @{
            email       = $newEmail
            nick        = "default-symbol-new-$stamp"
            status      = "active"
            group_users = 2
            storage     = 0
        }
        password = $password
    }
    Assert-Code $newUser 0 "create new shortcut recipient"
    $newToken = Get-Token -Email $newEmail -LoginPassword $password

    $listed = Invoke-Api -Method GET -Path ("/api/v4/file?uri=" + [uri]::EscapeDataString("cloudreve://my") + "&page_size=50") -Token $newToken
    Assert-Code $listed 0 "initialize new user root with shortcuts"
    $shortcut = @($listed.json.data.files) | Where-Object { $_.name -eq $folderName } | Select-Object -First 1
    if (-not $shortcut) {
        throw "Default share shortcut was not created: $($listed.raw)"
    }
    if (-not $shortcut.metadata.'sys:shared_redirect' -or -not $shortcut.metadata.'sys:shared_owner') {
        throw "Shortcut metadata is incomplete: $($shortcut | ConvertTo-Json -Depth 10 -Compress)"
    }

    $redirect = [string]$shortcut.metadata.'sys:shared_redirect'
    if ($redirect -notmatch "/$shareHashID(?:@|$)") {
        throw "Shortcut points to unexpected share: $redirect"
    }
    if ($shortcut.type -ne $folder.json.data.type) {
        throw "Shortcut type does not match the shared source: $($shortcut.type) != $($folder.json.data.type)"
    }
    $checks.Add([pscustomobject]@{
        check  = "new user root contains configured share shortcut"
        result = "PASS"
        detail = "$folderName -> $redirect"
    })
} finally {
    if ($null -ne $originalSetting) {
        $restore = Invoke-Api -Method PATCH -Path "/api/v4/admin/settings" -Token $adminToken -Body @{
            settings = @{ default_symbolics = $originalSetting }
        }
        if ($restore.json.code -ne 0) {
            Write-Warning "Failed to restore default_symbolics: $($restore.raw)"
        }
    }
}

$checks | Format-Table -AutoSize
"TOTAL=$($checks.Count)"
