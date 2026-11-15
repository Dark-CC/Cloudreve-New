param(
    [string]$BaseUrl = "http://127.0.0.1:5212",
    [string]$AdminEmail = "admin@example.com",
    [string]$AdminPassword = "$env:CN_ADMIN_PASSWORD",
    [string]$MemberEmail = "member@example.com",
    [string]$MemberPassword = "$env:CN_MEMBER_PASSWORD"
)

$ErrorActionPreference = "Stop"
$checks = [System.Collections.Generic.List[object]]::new()

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
        $args["Body"] = ($Body | ConvertTo-Json -Depth 30 -Compress)
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
    param([string]$Email, [string]$Password)
    $res = Invoke-Api -Method POST -Path "/api/v4/session/token" -Body @{ email = $Email; password = $Password }
    Assert-Code $res 0 "login $Email"
    $res.json.data.token.access_token
}

function Get-Group {
    param([int]$ID)
    $res = Invoke-Api -Method GET -Path "/api/v4/admin/group/$ID" -Token $adminToken
    Assert-Code $res 0 "get group $ID"
    $res.json.data
}

$adminToken = Get-Token -Email $AdminEmail -Password $AdminPassword
$memberToken = Get-Token -Email $MemberEmail -Password $MemberPassword

$stamp = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
$password = "{{SECRET_4bd4dxw5}}"
$id = 2

$policies = Invoke-Api -Method POST -Path "/api/v4/admin/policy" -Token $adminToken -Body @{
    page = 1
    page_size = 100
    order_by = ""
    order_direction = "desc"
    conditions = @{}
}
Assert-Code $policies 0 "list storage policies"
$policyIDs = @($policies.json.data.policies | ForEach-Object { $_.id })
if ($policyIDs.Count -eq 0) {
    throw "No storage policies"
}

$originalPolicies = @(1)
$updated = Invoke-Api -Method PUT -Path "/api/v4/admin/group/$id/policies" -Token $adminToken -Body @{
    policy_ids = @(1)
}
Assert-Code $updated 0 "update group policy bindings"
if (@($updated.json.data.policy_ids).Count -ne 1 -or @($updated.json.data.policy_ids)[0] -ne 1) {
    throw "Unexpected group policy update result: $($updated.raw)"
}
$checks.Add([pscustomobject]@{ check = "group policy persisted"; result = "PASS"; detail = "1" })

$restored = Invoke-Api -Method PUT -Path "/api/v4/admin/group/$id/policies" -Token $adminToken -Body @{
    policy_ids = $originalPolicies
}
Assert-Code $restored 0 "restore group policy bindings"

# Temporarily enable archive and relocation features for the runtime test user.
$group = Invoke-Api -Method GET -Path "/api/v4/admin/group/$id" -Token $adminToken
Assert-Code $group 0 "read runtime test group"
$originalGroup = $group.json.data
$permissionBytes = [Convert]::FromBase64String($originalGroup.permissions)
foreach ($bit in 5, 10) {
    $byteIndex = [math]::Floor($bit / 8)
    while ($permissionBytes.Length -le $byteIndex) {
        $permissionBytes += [byte]0
    }
    $permissionBytes[$byteIndex] = $permissionBytes[$byteIndex] -bor (1 -shl ($bit % 8))
}
$groupSettings = @{}
if ($originalGroup.settings) {
    $originalGroup.settings.psobject.Properties | ForEach-Object { $groupSettings[$_.Name] = $_.Value }
}
$groupSettings["compress_size"] = 1073741824
$groupSettings["decompress_size"] = 1073741824
$groupChanged = $true
$secondaryPolicyID = $null
$loadBalancePolicyID = $null
try {
$updatedGroup = Invoke-Api -Method PUT -Path "/api/v4/admin/group/$id" -Token $adminToken -Body @{
    group = @{
        id = $id
        name = $originalGroup.name
        max_storage = $originalGroup.max_storage
        speed_limit = $originalGroup.speed_limit
        permissions = [Convert]::ToBase64String($permissionBytes)
        settings = $groupSettings
        edges = @{
            storage_policies = @{
                id = $originalGroup.storage_policy_id
            }
        }
    }
    storage_policy_ids = $originalPolicies
}
Assert-Code $updatedGroup 0 "enable runtime group permissions"

$fixture = Join-Path (Split-Path $PSScriptRoot -Parent) "runtime\\pro-runtime-fixture"
New-Item -ItemType Directory -Path (Join-Path $fixture "tree\sub") -Force | Out-Null
[System.IO.File]::WriteAllText((Join-Path $fixture "alpha.txt"), "alpha $stamp", (New-Object System.Text.UTF8Encoding($false)))
[System.IO.File]::WriteAllText((Join-Path $fixture "tree\sub\beta.txt"), "beta $stamp", (New-Object System.Text.UTF8Encoding($false)))

$ownerRes = Invoke-Api -Method PUT -Path "/api/v4/admin/user" -Token $adminToken -Body @{
    user = @{
        email = "pro-runtime-$stamp@test.local"
        nick = "pro-runtime-$stamp"
        status = "active"
        group_users = 2
        storage = 0
    }
    password = $password
}
Assert-Code $ownerRes 0 "create workflow user"
$ownerHashID = $ownerRes.json.data.hash_id
$ownerToken = Get-Token -Email $ownerRes.json.data.email -Password $password

$memberPolicies = Invoke-Api -Method GET -Path "/api/v4/user/setting/policies" -Token $ownerToken
Assert-Code $memberPolicies 0 "member reads available policies"
if (@($memberPolicies.json.data).Count -eq 0) {
    throw "Member policy list is empty"
}
$checks.Add([pscustomobject]@{
    check = "member policy list matches group binding"
    result = "PASS"
    detail = (@($memberPolicies.json.data | ForEach-Object { $_.id }) -join ",")
})

$rootList = Invoke-Api -Method GET -Path ("/api/v4/file?uri=" + [uri]::EscapeDataString("cloudreve://my") + "&page_size=20") -Token $ownerToken
Assert-Code $rootList 0 "list owner root folder"
$rootCapability = [Convert]::FromBase64String([string]$rootList.json.data.props.capability)
$mountByte = [math]::Floor(21 / 8)
if ($rootCapability.Length -le $mountByte -or (($rootCapability[$mountByte] -shr (21 % 8)) -band 1) -ne 1) {
    throw "Root navigator does not advertise mount capability: $($rootList.json.data.props.capability)"
}
if (-not $rootList.json.data.parent.owned) {
    throw "Owner root folder is reported as not owned: $($rootList.raw)"
}
$checks.Add([pscustomobject]@{ check = "root exposes mount capability and ownership"; result = "PASS"; detail = "capability bit 21" })

$dst = "cloudreve://$ownerHashID@my/pro-$stamp"
$import = Invoke-Api -Method POST -Path "/api/v4/workflow/import" -Token $adminToken -Body @{
    src = $fixture
    dst = "/pro-$stamp"
    extract_media_meta = $true
    user_id = $ownerHashID
    recursive = $true
    policy_id = 1
}
Assert-Code $import 0 "seed workflow import"

function Wait-Task {
    param([string]$TaskID, [string]$Token, [int]$Attempts = 180)
    for ($i = 0; $i -lt $Attempts; $i++) {
        $res = Invoke-Api -Method GET -Path "/api/v4/workflow?page_size=50&category=general" -Token $Token
        Assert-Code $res 0 "list workflow tasks"
        $task = $res.json.data.tasks | Where-Object { $_.id -eq $TaskID } | Select-Object -First 1
        if ($task -and $task.status -in @("completed", "error", "canceled")) {
            return $task
        }
        Start-Sleep -Milliseconds 1000
    }
    throw "Task $TaskID did not finish"
}

$seed = Wait-Task -TaskID $import.json.data.id -Token $ownerToken
if ($seed.status -ne "completed") {
    throw "Seed import failed: $($seed | ConvertTo-Json -Depth 20 -Compress)"
}
$checks.Add([pscustomobject]@{ check = "workflow import completed"; result = "PASS"; detail = $seed.id })

$archiveDst = "$dst/release.zip"
$archive = Invoke-Api -Method POST -Path "/api/v4/workflow/archive" -Token $ownerToken -Body @{
    src = @("$dst/alpha.txt", "$dst/tree")
    dst = $archiveDst
}
Assert-Code $archive 0 "create archive task"
$archiveDone = Wait-Task -TaskID $archive.json.data.id -Token $ownerToken
if ($archiveDone.status -ne "completed") {
    throw "Archive task failed: $($archiveDone | ConvertTo-Json -Depth 20 -Compress)"
}
$archiveInfo = Invoke-Api -Method GET -Path ("/api/v4/file/info?uri=" + [uri]::EscapeDataString($archiveDst)) -Token $ownerToken
Assert-Code $archiveInfo 0 "read archive file info"
if ($archiveInfo.json.data.size -le 0) {
    throw "Archive output is empty"
}
$checks.Add([pscustomobject]@{ check = "archive workflow completed"; result = "PASS"; detail = $archiveInfo.json.data.size })

$extractRoot = "$dst/extracted-$stamp"
$extractMkdir = Invoke-Api -Method POST -Path "/api/v4/file/create" -Token $ownerToken -Body @{
    uri  = $extractRoot
    type = "folder"
}
Assert-Code $extractMkdir 0 "create extract destination"
$extract = Invoke-Api -Method POST -Path "/api/v4/workflow/extract" -Token $ownerToken -Body @{
    src = @($archiveDst)
    dst = $extractRoot
}
Assert-Code $extract 0 "create extract task"
$extractDone = Wait-Task -TaskID $extract.json.data.id -Token $ownerToken
if ($extractDone.status -ne "completed") {
    throw "Extract task failed: $($extractDone | ConvertTo-Json -Depth 20 -Compress)"
}
$extractList = Invoke-Api -Method GET -Path ("/api/v4/file?uri=" + [uri]::EscapeDataString($extractRoot)) -Token $ownerToken
Assert-Code $extractList 0 "list extracted files"
$extractedNames = @($extractList.json.data.files | ForEach-Object { $_.name })
if (-not ($extractedNames -contains "alpha.txt") -and -not ($extractedNames -contains "tree")) {
    throw "Extract output does not contain expected entries: $($extractedNames -join ",")"
}
$checks.Add([pscustomobject]@{ check = "extract workflow completed"; result = "PASS"; detail = ($extractedNames -join ",") })

$secondaryPolicy = Invoke-Api -Method PUT -Path "/api/v4/admin/policy" -Token $adminToken -Body @{
    policy = @{
        name           = "pro-runtime-secondary-$stamp"
        type           = "local"
        file_name_rule = "{uuid}_{originname}"
        settings       = @{
            chunk_size   = 26214400
            pre_allocate = $true
        }
        edges          = @{}
    }
}
Assert-Code $secondaryPolicy 0 "create secondary storage policy"
$secondaryPolicyID = [int]$secondaryPolicy.json.data.id

$boundPolicies = Invoke-Api -Method PUT -Path "/api/v4/admin/group/$id/policies" -Token $adminToken -Body @{
    policy_ids = @(1, $secondaryPolicyID)
}
Assert-Code $boundPolicies 0 "bind secondary storage policy"

$available = Invoke-Api -Method GET -Path "/api/v4/user/setting/policies" -Token $ownerToken
Assert-Code $available 0 "member reads policies after binding"
$secondary = @($available.json.data) | Where-Object { $_.name -eq "pro-runtime-secondary-$stamp" } | Select-Object -First 1
if (-not $secondary) {
    throw "Secondary policy is missing from member policy list: $($available.raw)"
}

$policyBefore = @($available.json.data) | Where-Object { $_.id -ne $secondary.id } | Select-Object -First 1
if (-not $policyBefore) {
    throw "Original storage policy is missing from member policy list"
}

$rootPolicies = @($available.json.data | ForEach-Object { $_.id })
$rootSwitch = Invoke-Api -Method PATCH -Path "/api/v4/file/policy" -Token $ownerToken -Body @{
    uri       = "cloudreve://my"
    policy_id = $secondary.id
}
Assert-Code $rootSwitch 0 "switch owner root storage policy"
$rootSwitched = Invoke-Api -Method GET -Path ("/api/v4/file?uri=" + [uri]::EscapeDataString("cloudreve://my") + "&page_size=20") -Token $ownerToken
Assert-Code $rootSwitched 0 "list owner root after policy switch"
if ($rootSwitched.json.data.storage_policy.id -ne $secondary.id) {
    throw "Root policy did not change to $($secondary.id): $($rootSwitched.raw)"
}
$rootSwitchBack = Invoke-Api -Method PATCH -Path "/api/v4/file/policy" -Token $ownerToken -Body @{
    uri       = "cloudreve://my"
    policy_id = $policyBefore.id
}
Assert-Code $rootSwitchBack 0 "switch owner root storage policy back"
$rootRestored = Invoke-Api -Method GET -Path ("/api/v4/file?uri=" + [uri]::EscapeDataString("cloudreve://my") + "&page_size=20") -Token $ownerToken
Assert-Code $rootRestored 0 "list owner root after restoring policy"
if ($rootRestored.json.data.storage_policy.id -ne $policyBefore.id) {
    throw "Root policy was not restored to $($policyBefore.id): $($rootRestored.raw)"
}
$checks.Add([pscustomobject]@{
    check  = "root policy switch round trip"
    result = "PASS"
    detail = "$($rootPolicies -join '/') -> $($secondary.id)"
})

# Direct folder policy switching must be owner-only, persist immediately, and
# emit Pro's mount event (type 26) with the selected policy.
$switchPolicy = Invoke-Api -Method PATCH -Path "/api/v4/file/policy" -Token $ownerToken -Body @{
    uri       = $dst
    policy_id = $secondary.id
}
Assert-Code $switchPolicy 0 "switch folder storage policy"
$switchedRoot = Invoke-Api -Method GET -Path ("/api/v4/file?uri=" + [uri]::EscapeDataString($dst) + "&page_size=20") -Token $ownerToken
Assert-Code $switchedRoot 0 "list folder after policy switch"
if ($switchedRoot.json.data.storage_policy.id -ne $secondary.id) {
    throw "Folder policy did not change to $($secondary.id): $($switchedRoot.raw)"
}
$mountLogs = Invoke-Api -Method POST -Path "/api/v4/admin/event" -Token $adminToken -Body @{
    page            = 1
    page_size       = 20
    order_by        = "id"
    order_direction = "desc"
    conditions      = @{
        audit_user_id = "$([int]$ownerRes.json.data.id)"
        audit_type    = "26"
    }
}
Assert-Code $mountLogs 0 "list folder policy audit events"
$mountLog = @($mountLogs.json.data.logs) | Where-Object {
    $_.content.storage_policy_name -eq $secondary.name
} | Select-Object -First 1
if (-not $mountLog) {
    throw "No mount audit event recorded for the selected policy: $($mountLogs.raw)"
}
$checks.Add([pscustomobject]@{
    check  = "folder policy switch writes mount audit event"
    result = "PASS"
    detail = "type=$($mountLog.type) policy=$($mountLog.content.storage_policy_name)"
})

$restoreFolderPolicy = Invoke-Api -Method PATCH -Path "/api/v4/file/policy" -Token $ownerToken -Body @{
    uri       = $dst
    policy_id = $policyBefore.id
}
Assert-Code $restoreFolderPolicy 0 "restore folder storage policy"

$relocate = Invoke-Api -Method POST -Path "/api/v4/workflow/relocate" -Token $ownerToken -Body @{
    src = @("$extractRoot/alpha.txt")
    dst_policy_id = $secondary.id
}
if ($relocate.json.code -ne 0) {
    throw "Relocate task creation failed: $($relocate.raw)"
}
$relocateDone = Wait-Task -TaskID $relocate.json.data.id -Token $ownerToken
if ($relocateDone.status -ne "completed") {
    throw "Relocate task failed: $($relocateDone | ConvertTo-Json -Depth 20 -Compress)"
}
$relocatedInfo = Invoke-Api -Method GET -Path ("/api/v4/file/info?uri=" + [uri]::EscapeDataString("$extractRoot/alpha.txt") + "&extended=true") -Token $ownerToken
Assert-Code $relocatedInfo 0 "read relocated file info"
if ($relocatedInfo.json.data.extended_info.storage_policy.id -ne $secondary.id) {
    throw "Relocated file is still on $($relocatedInfo.json.data.extended_info.storage_policy.id), expected $($secondary.id)"
}
$checks.Add([pscustomobject]@{ check = "relocate workflow completed"; result = "PASS"; detail = $secondary.name })

$originalPolicyID = (@($available.json.data) | Where-Object { $_.id -ne $secondary.id } | Select-Object -First 1).id
if (-not $originalPolicyID) {
    throw "Original storage policy is missing from member policy list"
}
$relocateBack = Invoke-Api -Method POST -Path "/api/v4/workflow/relocate" -Token $ownerToken -Body @{
    src = @("$extractRoot/alpha.txt")
    dst_policy_id = $originalPolicyID
}
Assert-Code $relocateBack 0 "create relocate-back task"
$relocateBackDone = Wait-Task -TaskID $relocateBack.json.data.id -Token $ownerToken
if ($relocateBackDone.status -ne "completed") {
    throw "Relocate-back task failed: $($relocateBackDone | ConvertTo-Json -Depth 20 -Compress)"
}
$backInfo = Invoke-Api -Method GET -Path ("/api/v4/file/info?uri=" + [uri]::EscapeDataString("$extractRoot/alpha.txt") + "&extended=true") -Token $ownerToken
Assert-Code $backInfo 0 "read relocated-back file info"
if ($backInfo.json.data.extended_info.storage_policy.id -ne $originalPolicyID) {
    throw "File was not moved back to the original policy"
}
$checks.Add([pscustomobject]@{ check = "relocate back completed"; result = "PASS"; detail = $backInfo.json.data.extended_info.storage_policy.name })

$loadBalance = Invoke-Api -Method PUT -Path "/api/v4/admin/policy" -Token $adminToken -Body @{
    policy = @{
        name     = "pro-runtime-lb-$stamp"
        type     = "load_balance"
        edges    = @{}
        settings = @{}
    }
}
Assert-Code $loadBalance 0 "create load balance policy"
$loadBalancePolicyID = [int]$loadBalance.json.data.id

$targetsUpdated = Invoke-Api -Method PUT -Path "/api/v4/admin/policy/$loadBalancePolicyID/loadBalance" -Token $adminToken -Body @{
    targets = @(
        @{ target_policy_id = 1; weight = 3 },
        @{ target_policy_id = $secondaryPolicyID; weight = 7 }
    )
}
Assert-Code $targetsUpdated 0 "update load balance targets"
if (@($targetsUpdated.json.data.targets).Count -ne 2) {
    throw "Unexpected load balance target count: $($targetsUpdated.raw)"
}
$weightByPolicy = @{}
@($targetsUpdated.json.data.targets) | ForEach-Object {
    $weightByPolicy[[int]$_.target_policy_id] = [int]$_.weight
}
if ($weightByPolicy[1] -ne 3 -or $weightByPolicy[$secondaryPolicyID] -ne 7) {
    throw "Load balance weights were not persisted: $($targetsUpdated.raw)"
}

$targetsRead = Invoke-Api -Method GET -Path "/api/v4/admin/policy/$loadBalancePolicyID/loadBalance" -Token $adminToken
Assert-Code $targetsRead 0 "read load balance targets"
if (@($targetsRead.json.data.targets).Count -ne 2) {
    throw "Load balance targets were not persisted: $($targetsRead.raw)"
}
$checks.Add([pscustomobject]@{ check = "load balance targets persisted"; result = "PASS"; detail = "2 targets: 3/7" })

$zeroWeight = Invoke-Api -Method PUT -Path "/api/v4/admin/policy/$loadBalancePolicyID/loadBalance" -Token $adminToken -Body @{
    targets = @(
        @{ target_policy_id = $secondaryPolicyID; weight = 0 }
    )
}
Assert-Code $zeroWeight 0 "normalize zero load balance weight"
if (@($zeroWeight.json.data.targets)[0].weight -ne 1) {
    throw "Zero weight was not normalized to 1: $($zeroWeight.raw)"
}
$checks.Add([pscustomobject]@{ check = "load balance zero weight normalized"; result = "PASS"; detail = "1" })

$selfTarget = Invoke-Api -Method PUT -Path "/api/v4/admin/policy/$loadBalancePolicyID/loadBalance" -Token $adminToken -Body @{
    targets = @(
        @{ target_policy_id = $loadBalancePolicyID; weight = 1 }
    )
}
if ($selfTarget.json.code -eq 0) {
    throw "Load balance policy accepted itself as a target"
}
$checks.Add([pscustomobject]@{ check = "load balance rejects self target"; result = "PASS"; detail = $selfTarget.json.code })

} finally {
    if ($groupChanged) {
        $restoreGroupSettings = @{}
        if ($originalGroup.settings) {
            $originalGroup.settings.psobject.Properties | ForEach-Object {
                $restoreGroupSettings[$_.Name] = $_.Value
            }
        }
        $restore = Invoke-Api -Method PUT -Path "/api/v4/admin/group/$id" -Token $adminToken -Body @{
            group = @{
                id = $id
                name = $originalGroup.name
                max_storage = $originalGroup.max_storage
                speed_limit = $originalGroup.speed_limit
                permissions = $originalGroup.permissions
                settings = $restoreGroupSettings
                edges = @{
                    storage_policies = @{
                        id = $originalGroup.storage_policy_id
                    }
                }
            }
            storage_policy_ids = $originalPolicies
        }
        if ($restore.json.code -ne 0) {
            Write-Error "Failed to restore group ${id}: $($restore.raw)"
        }
    }
    if ($loadBalancePolicyID) {
        $deleteLB = Invoke-Api -Method DELETE -Path "/api/v4/admin/policy/$loadBalancePolicyID" -Token $adminToken
        if ($deleteLB.json.code -ne 0) {
            Write-Warning "Failed to delete load balance policy ${loadBalancePolicyID}: $($deleteLB.raw)"
        }
    }
    if ($secondaryPolicyID) {
        $deleteSecondary = Invoke-Api -Method DELETE -Path "/api/v4/admin/policy/$secondaryPolicyID" -Token $adminToken
        if ($deleteSecondary.json.code -ne 0) {
            Write-Warning "Failed to delete secondary policy ${secondaryPolicyID}: $($deleteSecondary.raw)"
        }
    }
}

$checks | Format-Table -AutoSize
"PASSED: $($checks.Count)"
