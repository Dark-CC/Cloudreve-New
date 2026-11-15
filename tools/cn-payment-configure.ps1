param(
    [ValidateSet("probe", "restore")]
    [string]$Mode = "probe",
    [string]$BaseUrl = "http://127.0.0.1:5212",
    [string]$AdminEmail = "admin@example.com",
    [string]$AdminPassword = "$env:CN_ADMIN_PASSWORD"
)

$ErrorActionPreference = "Stop"

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
    $json = $response.Content | ConvertFrom-Json
    if ($json.code -ne 0) {
        throw "HTTP $($response.StatusCode) $Method $Path failed: $($json.code) $($json.msg)"
    }
    return $json
}

$login = Invoke-Api -Method POST -Path "/api/v4/session/token" -Body @{
    email    = $AdminEmail
    password = $AdminPassword
}
$token = $login.data.token.access_token

if ($Mode -eq "probe") {
    $payment = @{
        currency_code = "CNY"
        currency_mark = "¥"
        currency_unit = 100
        providers     = @(
            @{
                id           = "cn-custom-probe"
                type         = "custom"
                name         = "CN Probe"
                endpoint     = "http://127.0.0.1:9"
                secret_key   = "{{SECRET_5bksxemr}}"
                enabled      = $true
                description   = "Temporary fixture for CN callback security tests."
            }
        )
    }
    $json = $payment | ConvertTo-Json -Depth 8 -Compress
    Invoke-Api -Method PATCH -Path "/api/v4/admin/settings" -Token $token -Body @{
        settings = @{ payment = $json }
    } | Out-Null
    return
}

$original = '{"currency_code":"CNY","currency_mark":"¥","currency_unit":100,"providers":[]}'
Invoke-Api -Method PATCH -Path "/api/v4/admin/settings" -Token $token -Body @{
    settings = @{ payment = $original }
} | Out-Null
