param(
    [string]$BaseUrl = "http://127.0.0.1:5212",
    [string]$AdminEmail = "admin@example.com",
    [string]$AdminPassword = "$env:CN_ADMIN_PASSWORD"
)

$ErrorActionPreference = "Stop"
$checks = [System.Collections.Generic.List[object]]::new()
$stamp = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
$password = "{{SECRET_4bd4dxw5}}"

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
    [pscustomobject]@{ status = $response.StatusCode; json = $json; raw = $response.Content }
}

function Assert-Code {
    param($Response, [int]$Expected, [string]$Label)
    if ($Response.json.code -ne $Expected) {
        throw "$Label expected code $Expected, got $($Response.json.code): $($Response.json.msg) ($($Response.raw))"
    }
    $checks.Add([pscustomobject]@{ check = $Label; result = "PASS"; value = $Expected })
}

function Get-Token {
    param([string]$Email, [string]$LoginPassword)
    $res = Invoke-Api -Method POST -Path "/api/v4/session/token" -Body @{
        email = $Email; password = $LoginPassword
    }
    Assert-Code $res 0 "login $Email"
    $res.json.data.token.access_token
}

$adminToken = Get-Token -Email $AdminEmail -LoginPassword $AdminPassword
$email = "membership-$stamp@test.local"
$created = Invoke-Api -Method PUT -Path "/api/v4/admin/user" -Token $adminToken -Body @{
    user = @{
        email       = $email
        nick        = "membership-$stamp"
        status      = "active"
        group_users = 2
        storage     = 0
    }
    password = $password
}
Assert-Code $created 0 "create membership fixture user"
$member = $created.json.data
$userToken = Get-Token -Email $email -LoginPassword $password

$expires = (Get-Date).AddDays(30).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$assign = Invoke-Api -Method PUT -Path "/api/v4/admin/user/$($member.id)" -Token $adminToken -Body @{
    user = @{
        id          = [int]$member.id
        email       = $member.email
        nick        = $member.nick
        status      = "active"
        group_users = 1
        storage     = [int64]$member.storage
    }
    group_expires  = $expires
    previous_group = 2
}
Assert-Code $assign 0 "assign paid group with previous group"
if ([int]$assign.json.data.group_users -ne 1) {
    throw "Assigned group expected 1, got $($assign.json.data.group_users)"
}
if ([int]$assign.json.data.previous_group -ne 2) {
    throw "Assigned previous_group expected 2, got $($assign.json.data.previous_group)"
}

$before = Invoke-Api -Method GET -Path "/api/v4/user/setting" -Token $userToken
Assert-Code $before 0 "read setting before cancel"
if ($null -eq $before.json.data.group_expires) {
    throw "Membership expiry was not exposed before cancel"
}

$cancel = Invoke-Api -Method PATCH -Path "/api/v4/user/setting" -Token $userToken -Body @{
    group_expires = $true
}
Assert-Code $cancel 0 "user cancels membership"

$after = Invoke-Api -Method GET -Path "/api/v4/user/setting" -Token $userToken -Body $null
Assert-Code $after 0 "read setting after cancel"
if ($null -ne $after.json.data.group_expires) {
    throw "Membership expiry still present after cancel"
}

$meAfter = Invoke-Api -Method GET -Path "/api/v4/user/me" -Token $userToken
Assert-Code $meAfter 0 "read user after cancel"
if ([string]$meAfter.json.data.group.id -eq "") {
    throw "Group after cancel was empty"
}
$groupList = Invoke-Api -Method GET -Path "/api/v4/group/list" -Token $adminToken
Assert-Code $groupList 0 "list group options"
$userGroupOption = @($groupList.json.data) | Where-Object { $_.name -eq "User" } | Select-Object -First 1
if (-not $userGroupOption) {
    throw "Default User group option not found"
}
if ([string]$meAfter.json.data.group.id -ne [string]$userGroupOption.id) {
    throw "Group after cancel expected $($userGroupOption.id), got $($meAfter.json.data.group.id)"
}

$again = Invoke-Api -Method PATCH -Path "/api/v4/user/setting" -Token $userToken -Body @{
    group_expires = $true
}
Assert-Code $again 40001 "repeat cancel rejected"

$numeric = Invoke-Api -Method POST -Path "/api/v4/admin/event" -Token $adminToken -Body @{
    page            = 1
    page_size       = 50
    order_by        = "id"
    order_direction = "desc"
    conditions      = @{ audit_log_user = [string]$member.id; audit_log_category = "48" }
}
Assert-Code $numeric 0 "numeric membership audit filter"
$numericMatch = @($numeric.json.data.logs) | Where-Object { [int]$_.type -eq 48 } | Select-Object -First 1
if (-not $numericMatch) {
    throw "No membership cancellation event found by numeric user id"
}

$hash = $member.hash_id
$hashRes = Invoke-Api -Method POST -Path "/api/v4/admin/event" -Token $adminToken -Body @{
    page            = 1
    page_size       = 50
    order_by        = "id"
    order_direction = "desc"
    conditions      = @{ audit_log_user = [string]$hash; audit_log_category = "48" }
}
Assert-Code $hashRes 0 "hashid membership audit filter"
$hashMatch = @($hashRes.json.data.logs) | Where-Object { [int]$_.type -eq 48 } | Select-Object -First 1
if (-not $hashMatch) {
    throw "No membership cancellation event found by hashid"
}
if ($numericMatch.id -ne $hashMatch.id) {
    throw "Numeric and hashid audit filters returned different events"
}

[pscustomobject]@{
    checks              = $checks.Count
    user_id             = $member.id
    user_hash_id        = $hash
    group_before        = 1
    group_after         = 2
    previous_group      = $assign.json.data.previous_group
    audit_event_id      = $numericMatch.id
    audit_event_type    = $numericMatch.type
} | Format-List
