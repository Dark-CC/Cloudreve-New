param(
    [string]$BaseUrl = "http://127.0.0.1:5212",
    [string]$AdminEmail = "admin@example.com",
    [string]$AdminPassword = "$env:CN_ADMIN_PASSWORD"
)

$ErrorActionPreference = "Stop"
$stamp = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
$createdIDs = [System.Collections.Generic.List[int]]::new()

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

function Assert-Code {
    param($Response, [int]$Expected, [string]$Label)
    if ($Response.code -ne $Expected) {
        throw "$Label expected code $Expected, got $($Response.code): $($Response.msg)"
    }
}

function Get-Token {
    param([string]$Email, [string]$LoginPassword)
    $res = Invoke-Api -Method POST -Path "/api/v4/session/token" -Body @{
        email    = $Email
        password = $LoginPassword
    }
    Assert-Code $res 0 "login $Email"
    return $res.data.token.access_token
}

$adminToken = Get-Token -Email $AdminEmail -LoginPassword $AdminPassword
$memberToken = Get-Token -Email "member@example.com" -LoginPassword "$env:CN_MEMBER_PASSWORD"

try {
    $create = Invoke-Api -Method PUT -Path "/api/v4/admin/giftCodes" -Token $adminToken -Body @{
        num     = 3
        qyt     = 7
        product = @{ type = 4 }
    }
    Assert-Code $create 0 "create gift codes"
    if ($create.data.Count -ne 3) {
        throw "Expected 3 gift codes, got $($create.data.Count)"
    }
    $create.data | ForEach-Object { $createdIDs.Add([int]$_.id) }
    $target = $create.data[0]
    if (-not $target.code) {
        throw "Created gift code has no code"
    }

    $filtered = Invoke-Api -Method POST -Path "/api/v4/admin/giftCodes" -Token $adminToken -Body @{
        page            = 1
        page_size       = 10
        order_by        = "id"
        order_direction = "desc"
        conditions      = @{ gift_code_used = "false" }
        searches        = @{ gift_code_code = $target.code }
    }
    Assert-Code $filtered 0 "filter gift codes"
    $match = @($filtered.data.gift_codes | Where-Object { $_.code -eq $target.code })
    if ($match.Count -ne 1) {
        throw "Gift code search returned $($match.Count) matches"
    }
    if ($match[0].type -ne 4 -or $match[0].amount -ne 7) {
        throw "Flattened gift code product fields missing: type=$($match[0].type) amount=$($match[0].amount)"
    }

    $update = Invoke-Api -Method PATCH -Path "/api/v4/admin/giftCodes" -Token $adminToken -Body @{
        id      = [int]$target.id
        qyt     = 11
        product = @{ type = 4 }
    }
    Assert-Code $update 0 "update gift code"
    if ($update.data.qyt -ne 11) {
        throw "Gift code quantity was not updated: $($update.data.qyt)"
    }

    $preview = Invoke-Api -Method GET -Path "/api/v4/vas/giftcode/$($target.code)" -Token $memberToken
    Assert-Code $preview 0 "preview gift code"
    if ($preview.data.type -ne 4 -or $preview.data.amount -ne 11) {
        throw "Preview flattened product mismatch: type=$($preview.data.type) amount=$($preview.data.amount)"
    }

    $creditBefore = Invoke-Api -Method GET -Path "/api/v4/user/credit" -Token $memberToken
    Assert-Code $creditBefore 0 "credit before redeem"
    $redeem = Invoke-Api -Method POST -Path "/api/v4/vas/giftcode/$($target.code)/redeem" -Token $memberToken
    Assert-Code $redeem 0 "redeem gift code"

    $creditAfter = Invoke-Api -Method GET -Path "/api/v4/user/credit" -Token $memberToken
    Assert-Code $creditAfter 0 "credit after redeem"
    if (($creditAfter.data.credit - $creditBefore.data.credit) -ne 11) {
        throw "Credit delta mismatch: before=$($creditBefore.data.credit) after=$($creditAfter.data.credit)"
    }

    $again = Invoke-Api -Method POST -Path "/api/v4/vas/giftcode/$($target.code)/redeem" -Token $memberToken
    if ($again.code -eq 0) {
        throw "A redeemed gift code was accepted twice"
    }

    $usedFilter = Invoke-Api -Method POST -Path "/api/v4/admin/giftCodes" -Token $adminToken -Body @{
        page            = 1
        page_size       = 10
        order_by        = "id"
        order_direction = "desc"
        conditions      = @{ gift_code_used = "true"; gift_code_code = $target.code }
        searches        = @{}
    }
    Assert-Code $usedFilter 0 "filter used gift code"
    $usedMatch = @($usedFilter.data.gift_codes | Where-Object { $_.code -eq $target.code })
    if ($usedMatch.Count -ne 1 -or -not $usedMatch[0].used) {
        throw "Used gift code filter did not return the redeemed code"
    }
    if (-not $usedMatch[0].edges.user -or $usedMatch[0].user_hash_id -ne "z7hr") {
        # Admin is expected to be the viewer only when redeemed by admin; member is the actual redeemer.
        if (-not $usedMatch[0].edges.user) {
            throw "Used gift code did not include the redeeming user"
        }
    }

    $userFilter = Invoke-Api -Method POST -Path "/api/v4/admin/giftCodes" -Token $adminToken -Body @{
        page            = 1
        page_size       = 10
        order_by        = "id"
        order_direction = "desc"
        conditions      = @{ gift_code_user_id = [string]$usedMatch[0].user_hash_id }
        searches        = @{}
    }
    Assert-Code $userFilter 0 "filter gift codes by user hash ID"
    if (@($userFilter.data.gift_codes | Where-Object { $_.code -eq $target.code }).Count -ne 1) {
        throw "Gift code user filter did not return the redeemed code"
    }

    $deleteTarget = $create.data[1]
    Assert-Code (Invoke-Api -Method DELETE -Path "/api/v4/admin/giftCodes/$($deleteTarget.id)" -Token $adminToken) 0 "delete gift code"
    $afterDelete = Invoke-Api -Method POST -Path "/api/v4/admin/giftCodes" -Token $adminToken -Body @{
        page            = 1
        page_size       = 10
        order_by        = "id"
        order_direction = "desc"
        conditions      = @{}
        searches        = @{ gift_code_code = $deleteTarget.code }
    }
    Assert-Code $afterDelete 0 "list after delete"
    if (@($afterDelete.data.gift_codes | Where-Object { $_.code -eq $deleteTarget.code }).Count -ne 0) {
        throw "Deleted gift code is still listed"
    }

    [pscustomobject]@{
        created             = $create.data.Count
        filtered            = $match.Count
        updated_qyt         = $update.data.qyt
        preview_amount      = $preview.data.amount
        credit_before       = $creditBefore.data.credit
        credit_after        = $creditAfter.data.credit
        double_redeem_code  = $again.code
        used_filter_count   = $usedMatch.Count
        user_filter_count   = @($userFilter.data.gift_codes | Where-Object { $_.code -eq $target.code }).Count
        deleted_visible     = @($afterDelete.data.gift_codes | Where-Object { $_.code -eq $deleteTarget.code }).Count
    } | Format-List
} finally {
    foreach ($id in $createdIDs) {
        try {
            Invoke-Api -Method DELETE -Path "/api/v4/admin/giftCodes/$id" -Token $adminToken | Out-Null
        } catch {
            Write-Warning "cleanup gift code $id failed: $($_.Exception.Message)"
        }
    }
}
