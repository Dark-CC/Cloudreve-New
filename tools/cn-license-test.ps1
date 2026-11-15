param(
    [string]$BaseUrl = "http://127.0.0.1:5212",
    [string]$AdminEmail = "admin@example.com",
    [string]$AdminPassword = "$env:CN_ADMIN_PASSWORD",
    [string]$MemberEmail = "member@example.com",
    [string]$MemberPassword = "$env:CN_MEMBER_PASSWORD"
)

$ErrorActionPreference = "Stop"
$passed = 0
$failed = 0

function Check([string]$Name, [bool]$Condition, [string]$Detail = "") {
    if ($Condition) {
        $script:passed++
        Write-Host "[PASS] $Name" -ForegroundColor Green
    } else {
        $script:failed++
        Write-Host "[FAIL] $Name $Detail" -ForegroundColor Red
    }
}

$adminLogin = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v4/session/token" `
    -ContentType "application/json" `
    -Body (@{ email = $AdminEmail; password = $AdminPassword } | ConvertTo-Json)
$adminHeaders = @{ Authorization = "Bearer $($adminLogin.data.token.access_token)" }

$summary = Invoke-RestMethod -Uri "$BaseUrl/api/v4/admin/summary" -Headers $adminHeaders
Check "summary returns license" ($null -ne $summary.data.license)
Check "summary returns site URLs" (@($summary.data.site_urls).Count -gt 0)
Check "license has an offline expiration time" (-not [string]::IsNullOrWhiteSpace($summary.data.license.expired_at))
Check "license domains are populated" (@($summary.data.license.domains).Count -gt 0)
Check "license domains match configured site hosts" (
    @($summary.data.license.domains | Where-Object {
        $siteHosts = @($summary.data.site_urls | ForEach-Object { ([uri]$_).Host })
        $siteHosts -notcontains $_
    }).Count -eq 0
)

$auto = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v4/admin/license/refreshAuto" `
    -Headers $adminHeaders -ContentType "application/json" -Body "{}"
Check "auto refresh succeeds" ($auto.code -eq 0)
Check "auto refresh returns license" ($null -ne $auto.data.expired_at)
Check "auto refresh keeps configured domains" (@($auto.data.domains).Count -gt 0)

$manualPayload = @{
    expired_at = (Get-Date).ToUniversalTime().AddDays(45).ToString("o")
    domains = @($auto.data.domains)
    vol_domains = @($auto.data.vol_domains)
} | ConvertTo-Json -Compress
$expectedExpiration = ($manualPayload | ConvertFrom-Json).expired_at
$manual = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v4/admin/license/refreshManual" `
    -Headers $adminHeaders -ContentType "application/json" `
    -Body (@{ license = $manualPayload } | ConvertTo-Json)
Check "manual refresh succeeds" ($manual.code -eq 0)
Check "manual refresh applies expiration" (
    ([DateTimeOffset]$manual.data.expired_at).ToUnixTimeSeconds() -eq ([DateTimeOffset]$expectedExpiration).ToUnixTimeSeconds()
)
Check "manual refresh applies VOL domains" (@($manual.data.vol_domains).Count -gt 0)

$invalid = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v4/admin/license/refreshManual" `
    -Headers $adminHeaders -ContentType "application/json" `
    -Body (@{ license = "not-json" } | ConvertTo-Json)
Check "manual refresh rejects invalid JSON" ($invalid.code -eq 40001)

$pastPayload = @{
    expired_at = (Get-Date).ToUniversalTime().AddDays(-1).ToString("o")
    domains = @($auto.data.domains)
} | ConvertTo-Json -Compress
$expired = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v4/admin/license/refreshManual" `
    -Headers $adminHeaders -ContentType "application/json" `
    -Body (@{ license = $pastPayload } | ConvertTo-Json)
Check "manual refresh rejects expired license" ($expired.code -eq 40001)

$memberLogin = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v4/session/token" `
    -ContentType "application/json" `
    -Body (@{ email = $MemberEmail; password = $MemberPassword } | ConvertTo-Json)
$memberHeaders = @{ Authorization = "Bearer $($memberLogin.data.token.access_token)" }
$forbidden = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v4/admin/license/refreshAuto" `
    -Headers $memberHeaders -ContentType "application/json" -Body "{}"
Check "non-admin cannot refresh license" ($forbidden.code -eq 403)

$homePage = Invoke-WebRequest -Uri "$BaseUrl/admin/home" -UseBasicParsing
Check "admin home is served" ($homePage.StatusCode -eq 200)

Write-Host ""
Write-Host "LICENSE TESTS: $passed passed, $failed failed"
if ($failed -gt 0) {
    exit 1
}
