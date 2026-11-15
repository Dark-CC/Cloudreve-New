param(
    [string]$BaseUrl = "http://127.0.0.1:5212",
    [string]$AdminEmail = "admin@example.com",
    [string]$AdminPassword = "$env:CN_ADMIN_PASSWORD",
    [string]$MemberEmail = "member@example.com",
    [string]$MemberPassword = "$env:CN_MEMBER_PASSWORD"
)

$ErrorActionPreference = "Stop"

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
        Method      = $Method
        Uri         = "$BaseUrl$Path"
        Headers     = $headers
        ContentType = "application/json"
    }
    if ($null -ne $Body) {
        $args["Body"] = ($Body | ConvertTo-Json -Depth 20 -Compress)
    }
    try {
        Invoke-RestMethod @args
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

function Get-Token {
    param([string]$Email, [string]$Password)
    $res = Invoke-Api -Method POST -Path "/api/v4/session/token" -Body @{
        email    = $Email
        password = $Password
    }
    if ($res.code -ne 0) {
        throw "Login failed for ${Email}: $($res.code) $($res.msg)"
    }
    $res.data.token.access_token
}

function Assert-Code {
    param($Response, [int]$Expected, [string]$Label)
    if ($Response.code -ne $Expected) {
        throw "$Label expected code $Expected, got $($Response.code): $($Response.msg)"
    }
    $Response
}

$memberToken = Get-Token -Email $MemberEmail -Password $MemberPassword
$adminToken = Get-Token -Email $AdminEmail -Password $AdminPassword

$stamp = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
$folder = "cloudreve://my/acl-$stamp"
Assert-Code (Invoke-Api -Method POST -Path "/api/v4/file/create" -Token $memberToken -Body @{
    uri  = $folder
    type = "folder"
}) 0 "create folder" | Out-Null

$filePath = "$folder/acl-target-$stamp.txt"
$chunk = [System.Text.Encoding]::UTF8.GetBytes("acl test payload $stamp")
$upload = Invoke-Api -Method PUT -Path "/api/v4/file/upload" -Token $memberToken -Body @{
    uri           = $filePath
    size          = $chunk.Length
    last_modified = 0
    mime_type     = "text/plain"
}
Assert-Code $upload 0 "create upload session" | Out-Null
$uploadSession = $upload.data.session_id
$chunkRes = Invoke-WebRequest -Method POST -Uri "$BaseUrl/api/v4/file/upload/$uploadSession/0" `
    -Headers @{ Authorization = "Bearer $memberToken" } `
    -ContentType "application/octet-stream" -Body $chunk
$chunkJson = $chunkRes.Content | ConvertFrom-Json
Assert-Code $chunkJson 0 "upload chunk" | Out-Null

$shareRes = Invoke-Api -Method PUT -Path "/api/v4/share" -Token $memberToken -Body @{
    uri           = $filePath
    is_private    = $false
    share_view    = $true
    show_readme   = $false
    downloads     = 0
    expire        = 0
    price         = 0
}
Assert-Code $shareRes 0 "create share" | Out-Null
$shareUrl = [string]$shareRes.data
$shareId = ($shareUrl -split "/")[-1]

  $info = Assert-Code (Invoke-Api -Method GET -Path "/api/v4/share/info/$shareId") 0 "share info"
  $fileUri = $filePath
  $fileInfo = Assert-Code (Invoke-Api -Method GET -Path ("/api/v4/file/info?uri=" + [uri]::EscapeDataString($fileUri)) -Token $memberToken) 0 "file info"

$emptyAcl = Assert-Code (Invoke-Api -Method GET -Path "/api/v4/share/$shareId/acl" -Token $memberToken) 0 "get empty acl"
$emptyList = Assert-Code (Invoke-Api -Method GET -Path ("/api/v4/file?uri=" + [uri]::EscapeDataString($folder)) -Token $memberToken) 0 "member list folder"

$denyAll = @{
    builtin = @(
        @{ audience = "anonymous"; permission = @{ can_list = $false; can_read = $false; can_update = $false; can_create = $false; can_delete = $false; can_share = $false; discoverable = $false } },
        @{ audience = "logged_in"; permission = @{ can_list = $false; can_read = $false; can_update = $false; can_create = $false; can_delete = $false; can_share = $false; discoverable = $false } },
        @{ audience = "same_group"; permission = @{ can_list = $false; can_read = $false; can_update = $false; can_create = $false; can_delete = $false; can_share = $false; discoverable = $false } },
        @{ audience = "other_group"; permission = @{ can_list = $false; can_read = $false; can_update = $false; can_create = $false; can_delete = $false; can_share = $false; discoverable = $false } }
    )
    group = @()
    user  = @()
}
Assert-Code (Invoke-Api -Method PUT -Path "/api/v4/share/$shareId/acl" -Token $memberToken -Body @{ acl = $denyAll }) 0 "save deny-all acl" | Out-Null

  $shareUri = "cloudreve://$shareId@share"
$deniedList = Invoke-Api -Method GET -Path ("/api/v4/file?uri=" + [uri]::EscapeDataString($shareUri)) -Token $adminToken
$deniedInfo = Assert-Code (Invoke-Api -Method GET -Path "/api/v4/share/info/$shareId" -Token $adminToken) 0 "share info with denied acl"
if ($deniedInfo.data.name) {
    throw "Denied share info leaked the shared file name: $($deniedInfo.data.name)"
}
if ($deniedInfo.data.size -ne 0) {
    throw "Denied share info leaked the shared file size: $($deniedInfo.data.size)"
}

$allowRead = $denyAll.Clone()
$allowRead.builtin = @(
    @{ audience = "anonymous"; permission = @{ can_list = $true; can_read = $true; can_update = $false; can_create = $false; can_delete = $false; can_share = $false; discoverable = $false } },
    @{ audience = "logged_in"; permission = @{ can_list = $true; can_read = $false; can_update = $false; can_create = $false; can_delete = $false; can_share = $false; discoverable = $false } },
    @{ audience = "same_group"; permission = @{ can_list = $false; can_read = $false; can_update = $false; can_create = $false; can_delete = $false; can_share = $false; discoverable = $false } },
    @{ audience = "other_group"; permission = @{ can_list = $false; can_read = $false; can_update = $false; can_create = $false; can_delete = $false; can_share = $false; discoverable = $false } }
)
Assert-Code (Invoke-Api -Method PUT -Path "/api/v4/share/$shareId/acl" -Token $memberToken -Body @{ acl = $allowRead }) 0 "save read acl" | Out-Null

$listOnly = Invoke-Api -Method GET -Path ("/api/v4/file?uri=" + [uri]::EscapeDataString($shareUri)) -Token $adminToken
$listOnlyInfo = Assert-Code (Invoke-Api -Method GET -Path "/api/v4/share/info/$shareId" -Token $adminToken) 0 "share info with list-only acl"
if ($listOnlyInfo.data.name -ne [IO.Path]::GetFileName($filePath)) {
    throw "List-only share info should expose the file name, got '$($listOnlyInfo.data.name)'"
}
if ($listOnlyInfo.data.size -le 0) {
    throw "List-only share info should expose the file size, got $($listOnlyInfo.data.size)"
}
if ($listOnlyInfo.data.source_type) {
    throw "List-only share info should not expose the source type without read permission"
}
  $shareFileUri = "$shareUri/$([uri]::EscapeDataString([IO.Path]::GetFileName($filePath)))"
  $readOnly = Invoke-Api -Method GET -Path ("/api/v4/file/info?uri=" + [uri]::EscapeDataString($shareFileUri)) -Token $adminToken

$allowRead.builtin[1].permission.can_read = $true
Assert-Code (Invoke-Api -Method PUT -Path "/api/v4/share/$shareId/acl" -Token $memberToken -Body @{ acl = $allowRead }) 0 "save read acl" | Out-Null
  $readAllowed = Invoke-Api -Method GET -Path ("/api/v4/file/info?uri=" + [uri]::EscapeDataString($shareFileUri)) -Token $adminToken

$restore = @{ builtin = @(); group = @(); user = @() }
Assert-Code (Invoke-Api -Method PUT -Path "/api/v4/share/$shareId/acl" -Token $memberToken -Body @{ acl = $restore }) 0 "clear acl" | Out-Null

[pscustomobject]@{
    share_id          = $shareId
    folder            = $folder
    file              = $filePath
    file_id           = if ($fileInfo.data) { $fileInfo.data.id } else { $null }
    empty_acl_rows    = ($emptyAcl.data.builtin.Count + $emptyAcl.data.group.Count + $emptyAcl.data.user.Count)
    empty_list_code   = $emptyList.code
    deny_list_code    = $deniedList.code
    list_only_code    = $listOnly.code
    list_only_name    = if ($listOnly.data.files) { $listOnly.data.files[0].name } else { $null }
    denied_info_name  = $deniedInfo.data.name
    denied_info_size  = $deniedInfo.data.size
    list_info_name    = $listOnlyInfo.data.name
    list_info_size    = $listOnlyInfo.data.size
    read_without_list = $readOnly.code
    read_allowed_code = $readAllowed.code
} | Format-List
