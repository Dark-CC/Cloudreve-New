param(
    [string]$BaseUrl = "http://127.0.0.1:5212",
    [string]$AdminEmail = "admin@example.com",
    [string]$AdminPassword = "$env:CN_ADMIN_PASSWORD",
    [switch]$KeepTestData
)

$ErrorActionPreference = "Stop"
$stamp = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
$results = [System.Collections.Generic.List[object]]::new()

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
        Method      = $Method
        Uri         = "$BaseUrl$Path"
        Headers     = $headers
        ContentType = "application/json"
    }
    if ($null -ne $Body) {
        $args["Body"] = ($Body | ConvertTo-Json -Depth 30 -Compress)
    }

    try {
        if ($Raw) {
            return Invoke-WebRequest @args -UseBasicParsing
        }
        return Invoke-RestMethod @args
    } catch {
        $response = $_.Exception.Response
        if ($null -ne $response) {
            try {
                $bodyText = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
                Write-Host "HTTP failure for $Method $Path`: $bodyText"
            } catch {
                Write-Host "HTTP failure for $Method $Path`: $($_.Exception.Message)"
            }
        }
        throw
    }
}

function Assert-Equal {
    param($Actual, $Expected, [string]$Label)
    if ($Actual -ne $Expected) {
        throw "$Label expected '$Expected', got '$Actual'"
    }
    $results.Add([pscustomobject]@{ check = $Label; result = "PASS"; value = $Actual })
}

function Assert-True {
    param([bool]$Condition, [string]$Label, $Value = "")
    if (-not $Condition) {
        throw "$Label failed (value: $Value)"
    }
    $results.Add([pscustomobject]@{ check = $Label; result = "PASS"; value = $Value })
}

function Get-Token {
    param([string]$Email, [string]$Password)
    $res = Invoke-Api -Method POST -Path "/api/v4/session/token" -Body @{
        email    = $Email
        password = $Password
    }
    Assert-Equal $res.code 0 "login $Email"
    return $res.data.token.access_token
}

function New-TestUser {
    param(
        [string]$AdminToken,
        [string]$Email,
        [string]$Nick,
        [int]$GroupID
    )
    $res = Invoke-Api -Method PUT -Path "/api/v4/admin/user" -Token $AdminToken -Body @{
        user = @{
            email       = $Email
            nick        = $Nick
            status      = "active"
            group_users = $GroupID
            storage     = 0
        }
        password = "{{SECRET_4bd4dxw5}}"
    }
    Assert-Equal $res.code 0 "create user $Email"
    return [pscustomobject]@{
        id       = $res.data.id
        hash_id  = $res.data.hash_id
        email    = $Email
        password = "{{SECRET_4bd4dxw5}}"
    }
}

function New-TestGroup {
    param(
        [string]$AdminToken,
        [string]$Name
    )
    $res = Invoke-Api -Method PUT -Path "/api/v4/admin/group" -Token $AdminToken -Body @{
        group = @{
            id          = 0
            name        = $Name
            permissions = "hAg="
            max_storage = 1073741824
            speed_limit = 0
            settings    = @{
                source_batch     = 10
                aria2_batch      = 1
                max_walked_files = 100000
                trash_retention  = 604800
                redirected_source = $true
            }
            edges = @{
                storage_policies = @{ id = 1 }
            }
        }
        storage_policy_ids = @(1)
    }
    Assert-Equal $res.code 0 "create test group $Name"
    return [pscustomobject]@{
        id   = [int]$res.data.id
        name = $Name
    }
}

function New-ParentFolder {
    param([string]$Token, [string]$Name)
    $uri = "cloudreve://my/$Name"
    $res = Invoke-Api -Method POST -Path "/api/v4/file/create" -Token $Token -Body @{
        uri  = $uri
        type = "folder"
    }
    Assert-Equal $res.code 0 "create parent $Name"
    return $uri
}

function New-ShareFile {
    param(
        [string]$Token,
        [string]$ParentURI,
        [string]$Name,
        [string]$Content,
        [string]$Password = ""
    )

    $filePath = "$ParentURI/$Name"
    $bytes = [Text.Encoding]::UTF8.GetBytes($Content)
    $upload = Invoke-Api -Method PUT -Path "/api/v4/file/upload" -Token $Token -Body @{
        uri           = $filePath
        size          = $bytes.Length
        last_modified = 0
        mime_type     = "text/plain"
    }
    Assert-Equal $upload.code 0 "prepare upload $Name"
    $chunk = Invoke-WebRequest -Method POST `
        -Uri "$BaseUrl/api/v4/file/upload/$($upload.data.session_id)/0" `
        -Headers @{ Authorization = "Bearer $Token" } `
        -ContentType "application/octet-stream" `
        -Body $bytes `
        -UseBasicParsing
    $chunkRes = $chunk.Content | ConvertFrom-Json
    Assert-Equal $chunkRes.code 0 "upload chunk $Name"

    $body = @{
        uri         = $filePath
        is_private  = [bool]$Password
        share_view  = $true
        show_readme = $false
        downloads   = 0
        expire      = 0
        price       = 0
    }
    if ($Password) {
        $body.password = $Password
    }
    $shareRes = Invoke-Api -Method PUT -Path "/api/v4/share" -Token $Token -Body $body
    Assert-Equal $shareRes.code 0 "create share $Name"
    $shareURL = [string]$shareRes.data
    if ($shareURL -notmatch "/s/([^/]+)") {
        throw "Unable to parse share ID from $shareURL"
    }
    $shareID = $Matches[1]

    return [pscustomobject]@{
        name       = $Name
        path       = $filePath
        share_id   = $shareID
        share_url  = $shareURL
        password   = $Password
    }
}

function Set-ShareACL {
    param(
        [string]$Token,
        [string]$ShareID,
        [array]$Builtin = @(),
        [array]$Group = @(),
        [array]$User = @()
    )
    $res = Invoke-Api -Method PUT -Path "/api/v4/share/$ShareID/acl" -Token $Token -Body @{
        acl = @{
            builtin = $Builtin
            group   = $Group
            user    = $User
        }
    }
    Assert-Equal $res.code 0 "save ACL $ShareID"
}

function Permission {
    param([bool]$List = $false, [bool]$Read = $false, [bool]$Discoverable = $false)
    return @{
        can_list     = $List
        can_read     = $Read
        can_update   = $false
        can_create   = $false
        can_delete   = $false
        can_share    = $false
        discoverable = $Discoverable
    }
}

function List-SharedWithMe {
    param(
        [string]$Token,
        [int]$Page = -1,
        [int]$PageSize = 0,
        [string]$Query = "",
        [string]$PageToken = ""
    )
    $encodedURI = [uri]::EscapeDataString("cloudreve://shared_with_me")
    $query = "?uri=$encodedURI"
    if ($Page -ge 0) {
        $query += "&page=$Page"
    }
    if ($PageSize -gt 0) {
        $query += "&page_size=$PageSize"
    }
    if ($Query) {
        $query += "&$Query"
    }
    if ($PageToken) {
        $query += "&next_page_token=" + [uri]::EscapeDataString($PageToken)
    }
    $res = Invoke-Api -Method GET -Path "/api/v4/file$query" -Token $Token
    Assert-Equal $res.code 0 "list shared_with_me"
    return $res.data
}

function List-Names {
    param($List)
    return @($List.files | ForEach-Object { $_.name })
}

$adminToken = Get-Token -Email $AdminEmail -Password $AdminPassword
$originalSetting = Invoke-Api -Method POST -Path "/api/v4/admin/settings" -Token $adminToken -Body @{
    keys = @("shared_with_me_acl")
}
Assert-Equal $originalSetting.code 0 "read shared_with_me_acl setting"
$originalMode = [string]$originalSetting.data.shared_with_me_acl

$createdUserIDs = [System.Collections.Generic.List[int]]::new()
$createdGroupID = 0
try {
    $testGroup = New-TestGroup -AdminToken $adminToken -Name "swm-group-$stamp"
    $createdGroupID = $testGroup.id

    $owner = New-TestUser -AdminToken $adminToken -Email "swm-owner-$stamp@test.local" -Nick "swm-owner-$stamp" -GroupID 2
    $createdUserIDs.Add([int]$owner.id)
    $viewer = New-TestUser -AdminToken $adminToken -Email "swm-viewer-$stamp@test.local" -Nick "swm-viewer-$stamp" -GroupID 2
    $createdUserIDs.Add([int]$viewer.id)
    $outsider = New-TestUser -AdminToken $adminToken -Email "swm-outsider-$stamp@test.local" -Nick "swm-outsider-$stamp" -GroupID $testGroup.id
    $createdUserIDs.Add([int]$outsider.id)

    $ownerToken = Get-Token -Email $owner.email -Password $owner.password
    $viewerToken = Get-Token -Email $viewer.email -Password $viewer.password
    $outsiderToken = Get-Token -Email $outsider.email -Password $outsider.password

    $parent = New-ParentFolder -Token $ownerToken -Name "swm-$stamp"
    $discoverable = New-ShareFile -Token $ownerToken -ParentURI $parent -Name "discoverable-$stamp.txt" -Content "discoverable $stamp"
    $linkOnly = New-ShareFile -Token $ownerToken -ParentURI $parent -Name "link-only-$stamp.txt" -Content "link only $stamp"
    $userGranted = New-ShareFile -Token $ownerToken -ParentURI $parent -Name "user-granted-$stamp.txt" -Content "user granted $stamp"
    $groupGranted = New-ShareFile -Token $ownerToken -ParentURI $parent -Name "group-granted-$stamp.txt" -Content "group granted $stamp"
    $otherGroup = New-ShareFile -Token $ownerToken -ParentURI $parent -Name "other-group-$stamp.txt" -Content "other group $stamp"
    $passwordShare = New-ShareFile -Token $ownerToken -ParentURI $parent -Name "password-$stamp.txt" -Content "password $stamp" -Password "Passw0rdTest"
    $expiredShare = New-ShareFile -Token $ownerToken -ParentURI $parent -Name "expired-$stamp.txt" -Content "expired $stamp"

    Set-ShareACL -Token $ownerToken -ShareID $discoverable.share_id -Builtin @(
        @{ audience = "logged_in"; permission = (Permission -List $true -Read $true -Discoverable $true) }
    )
    Set-ShareACL -Token $ownerToken -ShareID $linkOnly.share_id -Builtin @(
        @{ audience = "logged_in"; permission = (Permission -List $true -Read $true -Discoverable $false) }
    )
    Set-ShareACL -Token $ownerToken -ShareID $userGranted.share_id `
        -User @(
            @{ user_id = $outsider.id; permission = (Permission -List $true -Read $true -Discoverable $true) }
        )
    Set-ShareACL -Token $ownerToken -ShareID $groupGranted.share_id `
        -Group @(
            @{ group_id = $testGroup.id; permission = (Permission -List $true -Read $false -Discoverable $true) }
        )
    Set-ShareACL -Token $ownerToken -ShareID $otherGroup.share_id -Builtin @(
        @{ audience = "same_group"; permission = (Permission -List $true -Read $true -Discoverable $true) },
        @{ audience = "other_group"; permission = (Permission -List $false -Read $false -Discoverable $false) }
    )

    $passwordShareInfo = Invoke-Api -Method GET -Path "/api/v4/share/info/$($passwordShare.share_id)?password=$($passwordShare.password)"
    Assert-Equal $passwordShareInfo.code 0 "password share info"
    Assert-True ($passwordShareInfo.data.password_protected -eq $true) "password share reports protected" $passwordShareInfo.data.password_protected

    $expiredUpdate = Invoke-Api -Method PUT -Path "/api/v4/share" -Token $ownerToken -Body @{
        uri         = $expiredShare.path
        is_private  = $false
        share_view  = $true
        show_readme = $false
        downloads   = 0
        expire      = 1
        price       = 0
    }
    Assert-Equal $expiredUpdate.code 0 "set short expiry"
    Start-Sleep -Seconds 2

    Invoke-Api -Method PATCH -Path "/api/v4/admin/settings" -Token $adminToken -Body @{
        settings = @{ shared_with_me_acl = "0" }
    } | ForEach-Object { Assert-Equal $_.code 0 "switch to link mode" }
    $linkModeList = List-SharedWithMe -Token $viewerToken
    $linkModeNames = List-Names $linkModeList
    Assert-True (-not ($linkModeNames -contains $discoverable.name)) "link mode hides ACL-only shares" ($linkModeNames -join ",")

    Invoke-Api -Method PATCH -Path "/api/v4/admin/settings" -Token $adminToken -Body @{
        settings = @{ shared_with_me_acl = "1" }
    } | ForEach-Object { Assert-Equal $_.code 0 "switch to ACL mode" }

    $viewerList = List-SharedWithMe -Token $viewerToken
    $viewerNames = List-Names $viewerList
    Assert-True ($viewerNames -contains $discoverable.name) "ACL mode shows discoverable share" ($viewerNames -join ",")
    Assert-True (-not ($viewerNames -contains $linkOnly.name)) "ACL mode honors discoverable=false" ($viewerNames -join ",")
    Assert-True (-not ($viewerNames -contains $userGranted.name)) "ACL mode honors explicit user grants" ($viewerNames -join ",")
    Assert-True (-not ($viewerNames -contains $groupGranted.name)) "ACL mode honors explicit group grants" ($viewerNames -join ",")
    Assert-True ($viewerNames -contains $otherGroup.name) "ACL mode honors same-group discovery" ($viewerNames -join ",")
    Assert-True (-not ($viewerNames -contains $expiredShare.name)) "ACL mode hides expired shares" ($viewerNames -join ",")
    Assert-True (-not ($viewerNames -contains $passwordShare.name)) "ACL mode does not bypass password shares" ($viewerNames -join ",")

    $item = $viewerList.files | Where-Object { $_.name -eq $discoverable.name } | Select-Object -First 1
    Assert-True ($null -ne $item) "discoverable entry returned"
    Assert-True ($item.metadata.'sys:shared_redirect' -eq "cloudreve://$($discoverable.share_id)@share") "virtual entry redirect metadata" $item.metadata.'sys:shared_redirect'
    Assert-True ($item.metadata.'sys:shared_owner' -eq $owner.hash_id) "virtual entry owner metadata" $item.metadata.'sys:shared_owner'

    $redirectInfo = Invoke-Api -Method GET -Path ("/api/v4/file?uri=" + [uri]::EscapeDataString("cloudreve://$($discoverable.share_id)@share")) -Token $viewerToken
    Assert-Equal $redirectInfo.code 0 "open virtual share entry"
    Assert-True (@($redirectInfo.data.files | Where-Object { $_.name -eq $discoverable.name }).Count -eq 1) "opened share resolves to shared file"

    $outsiderList = List-SharedWithMe -Token $outsiderToken
    $outsiderNames = List-Names $outsiderList
    Assert-True ($outsiderNames -contains $userGranted.name) "explicit user grant is discoverable" ($outsiderNames -join ",")
    Assert-True ($outsiderNames -contains $groupGranted.name) "explicit group grant is discoverable" ($outsiderNames -join ",")
    Assert-True (-not ($outsiderNames -contains $otherGroup.name)) "other-group deny hides entry" ($outsiderNames -join ",")

    $pageOne = List-SharedWithMe -Token $viewerToken -PageSize 1
    $pageTwo = List-SharedWithMe -Token $viewerToken -PageSize 1 -PageToken $pageOne.pagination.next_token
    Assert-Equal $pageOne.pagination.page_size 1 "page size honored"
    Assert-True ($pageOne.pagination.is_cursor -eq $true) "cursor pagination enabled" $pageOne.pagination.is_cursor
    Assert-True (-not [string]::IsNullOrEmpty($pageOne.pagination.next_token)) "next page token returned" $pageOne.pagination.next_token
    Assert-True ($pageOne.files.Count -eq 1) "page one item count" $pageOne.files.Count
    Assert-True ($pageTwo.files.Count -le 1) "page two item count" $pageTwo.files.Count
    if ($pageOne.files.Count -gt 0 -and $pageTwo.files.Count -gt 0) {
        Assert-True ($pageOne.files[0].id -ne $pageTwo.files[0].id) "pagination returns distinct items"
    }

    $filtered = List-SharedWithMe -Token $viewerToken -Query ("name=" + [uri]::EscapeDataString("discoverable-$stamp"))
    $filteredNames = List-Names $filtered
    Assert-True ($filteredNames -contains $discoverable.name) "name filter finds discoverable share" ($filteredNames -join ",")
    Assert-True (-not ($filteredNames -contains $passwordShare.name)) "name filter excludes non-matching shares" ($filteredNames -join ",")

    [pscustomobject]@{
        mode              = "pass"
        checks            = $results.Count
        viewer_visible    = ($viewerNames -join ",")
        outsider_visible  = ($outsiderNames -join ",")
    } | Format-List
}
finally {
    try {
        Invoke-Api -Method PATCH -Path "/api/v4/admin/settings" -Token $adminToken -Body @{
            settings = @{ shared_with_me_acl = $originalMode }
        } | Out-Null
    } catch {
        Write-Warning "Failed to restore shared_with_me_acl: $($_.Exception.Message)"
    }

    if (-not $KeepTestData -and @($createdUserIDs).Count -gt 0) {
        try {
            Invoke-Api -Method POST -Path "/api/v4/admin/user/batch/delete" -Token $adminToken -Body @{
                ids = @($createdUserIDs)
            } | Out-Null
        } catch {
            Write-Warning "Failed to remove test users: $($_.Exception.Message)"
        }
    }

    if (-not $KeepTestData -and $createdGroupID -gt 0) {
        try {
            Invoke-Api -Method DELETE -Path "/api/v4/admin/group/$createdGroupID" -Token $adminToken | Out-Null
        } catch {
            Write-Warning "Failed to remove test group: $($_.Exception.Message)"
        }
    }
}
