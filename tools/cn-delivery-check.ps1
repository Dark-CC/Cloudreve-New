param(
    [string]$BaseUrl = "http://127.0.0.1:5212",
    [string]$AdminEmail = "admin@example.com",
    [string]$AdminPassword = "$env:CN_ADMIN_PASSWORD"
)

$ErrorActionPreference = "Stop"

$login = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v4/session/token" `
    -ContentType "application/json" `
    -Body (@{ email = $AdminEmail; password = $AdminPassword } | ConvertTo-Json)
$token = $login.data.token.access_token
$group = Invoke-RestMethod -Uri "$BaseUrl/api/v4/admin/group/2" `
    -Headers @{ Authorization = "Bearer $token" }

"GROUP"
[pscustomobject]@{
    permissions     = $group.data.permissions
    storage_policies = (@($group.data.storage_policy_ids) -join ",")
    compress_size   = $group.data.settings.compress_size
    decompress_size = $group.data.settings.decompress_size
} | Format-List

"PAGES"
$pages = @("/", "/admin/home", "/admin/abuse", "/admin/event", "/admin/payment", "/admin/group", "/admin/policy")
$results = foreach ($page in $pages) {
    try {
        $res = Invoke-WebRequest -Uri "$BaseUrl$page" -UseBasicParsing -TimeoutSec 20
        [pscustomobject]@{
            path   = $page
            status = $res.StatusCode
            length = $res.RawContentLength
            type   = $res.Headers["Content-Type"]
        }
    } catch {
        [pscustomobject]@{
            path   = $page
            status = "ERROR"
            length = 0
            type   = $_.Exception.Message
        }
    }
}
$results | Format-Table -AutoSize

"PWA"
$manifest = Invoke-WebRequest -Uri "$BaseUrl/manifest.json" -UseBasicParsing -TimeoutSec 20
[pscustomobject]@{
    status = $manifest.StatusCode
    type   = $manifest.Headers["Content-Type"]
    bytes  = $manifest.RawContentLength
} | Format-List
