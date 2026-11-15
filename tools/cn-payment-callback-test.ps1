param(
    [string]$BaseUrl = "http://127.0.0.1:5212",
    [string]$AdminEmail = "admin@example.com",
    [string]$AdminPassword = "$env:CN_ADMIN_PASSWORD",
    [string]$ProviderID = "cn-custom-probe",
    [string]$ProviderSecret = "{{SECRET_5bksxemr}}",
    [int]$GatewayPort = 18089
)

$ErrorActionPreference = "Stop"
$checks = [System.Collections.Generic.List[object]]::new()

function Invoke-Api {
    param([string]$Method, [string]$Path, [string]$Token, $Body, [hashtable]$Headers = @{})
    $requestHeaders = @{}
    foreach ($key in $Headers.Keys) {
        $requestHeaders[$key] = $Headers[$key]
    }
    if ($Token) {
        $requestHeaders["Authorization"] = "Bearer $Token"
    }
    $args = @{
        Method             = $Method
        Uri                = "$BaseUrl$Path"
        Headers            = $requestHeaders
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
    param([string]$Email, [string]$Password)
    $res = Invoke-Api -Method POST -Path "/api/v4/session/token" -Body @{
        email = $Email; password = $Password
    }
    Assert-Code $res 0 "login $Email"
    $res.json.data.token.access_token
}

function Get-Admin-Order {
    param([string]$Token, [string]$TradeNo, [string]$Label)
    $res = Invoke-Api -Method POST -Path "/api/v4/admin/payment" -Token $Token -Body @{
        page            = 1
        page_size       = 10
        order_by        = "id"
        order_direction = "desc"
        conditions      = @{ payment_trade_no = $TradeNo }
    }
    Assert-Code $res 0 $Label
    $match = @($res.json.data.payments) | Where-Object { $_.trade_no -eq $TradeNo } | Select-Object -First 1
    if (-not $match) {
        throw "$Label did not return order $TradeNo"
    }
    return $match
}

function Set-CustomProviderEndpoint {
    param([string]$Token, [string]$Endpoint)
    $payment = [ordered]@{
        currency_code = "CNY"
        currency_mark = "¥"
        currency_unit = 100
        providers     = @(
            [ordered]@{
                id           = $ProviderID
                type         = "custom"
                name         = "CN Probe"
                endpoint     = $Endpoint
                secret_key   = $ProviderSecret
                enabled      = $true
                description  = "Temporary fixture for CN callback security tests."
            }
        )
    }
    $res = Invoke-Api -Method PATCH -Path "/api/v4/admin/settings" -Token $Token -Body @{
        settings = @{ payment = ($payment | ConvertTo-Json -Depth 8 -Compress) }
    }
    Assert-Code $res 0 "configure custom provider endpoint"
}

function Restore-PaymentSettings {
    param([string]$Token)
    $payment = [ordered]@{
        currency_code = "CNY"
        currency_mark = "¥"
        currency_unit = 100
        providers     = @()
    }
    $res = Invoke-Api -Method PATCH -Path "/api/v4/admin/settings" -Token $Token -Body @{
        settings = @{ payment = ($payment | ConvertTo-Json -Depth 8 -Compress) }
    }
    if ($res.json.code -ne 0) {
        Write-Warning "Failed to restore payment settings: $($res.raw)"
    }
}

function Get-HmacHex {
    param([string]$Secret, [string]$Body)
    $hmac = [System.Security.Cryptography.HMACSHA256]::new([Text.Encoding]::UTF8.GetBytes($Secret))
    try {
        $bytes = [Text.Encoding]::UTF8.GetBytes($Body)
        return ([BitConverter]::ToString($hmac.ComputeHash($bytes)) -replace "-", "").ToLowerInvariant()
    } finally {
        $hmac.Dispose()
    }
}

function Send-Callback {
    param([string]$TradeNo, [hashtable]$Payload)
    $body = $Payload | ConvertTo-Json -Depth 10 -Compress
    $signature = Get-HmacHex -Secret $ProviderSecret -Body $body
    return Send-RawCallback -TradeNo $TradeNo -Body $body -Signature $signature
}

function Send-RawCallback {
    param([string]$TradeNo, [string]$Body, [string]$Signature = "")
    $headers = @{ "Content-Type" = "application/json" }
    if ($Signature) {
        $headers["X-Cloudreve-Signature"] = $Signature
    }
    $response = Invoke-WebRequest -Method POST -Uri "$BaseUrl/api/v4/vas/payment/callback/$TradeNo" `
        -Headers $headers -Body $Body -SkipHttpErrorCheck -UseBasicParsing
    [pscustomobject]@{
        status = $response.StatusCode
        json   = ($response.Content | ConvertFrom-Json)
        raw    = $response.Content
    }
}

try {
    $adminToken = Get-Token -Email $AdminEmail -Password $AdminPassword
    Set-CustomProviderEndpoint -Token $adminToken -Endpoint "http://127.0.0.1:$GatewayPort/"

    $gatewayConfigPath = Join-Path $env:TEMP "cn-payment-gateway-$GatewayPort-$([DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()).json"
    $reply = @{
        payment_url           = "http://127.0.0.1:$GatewayPort/pay"
        qr_code               = ""
        provider_reference_id = "probe-ref-pending"
        paid                   = $false
    } | ConvertTo-Json -Compress
    @{ port = $GatewayPort; secret = $ProviderSecret; reply = $reply } |
        ConvertTo-Json -Depth 5 -Compress |
        Set-Content -LiteralPath $gatewayConfigPath -Encoding UTF8
    $gateway = Start-Process -FilePath (Get-Process -Id $PID).Path `
        -ArgumentList @("-NoProfile", "-ExecutionPolicy", "Bypass", "-File", (Join-Path $PSScriptRoot "cn-payment-gateway.ps1"), "-ConfigPath", $gatewayConfigPath) `
        -PassThru -WindowStyle Hidden
    $readyPath = "$gatewayConfigPath.ready"
    for ($i = 0; $i -lt 50 -and -not (Test-Path -LiteralPath $readyPath); $i++) {
        Start-Sleep -Milliseconds 100
    }
    if (-not (Test-Path -LiteralPath $readyPath)) {
        throw "Payment gateway fixture did not become ready"
    }

    $order = Invoke-Api -Method PUT -Path "/api/v4/vas/payment" -Token $adminToken -Body @{
        product_type = 4
        provider_id  = $ProviderID
        qyt          = 1
    }
    Assert-Code $order 0 "create custom-provider order"
    $tradeNo = [string]$order.json.data.trade_no
    if ($tradeNo -eq "") {
        throw "Custom order did not return trade_no"
    }

    if (-not $gateway.WaitForExit(10000)) {
        throw "Payment gateway fixture did not finish"
    }
    $capturePath = "$gatewayConfigPath.capture"
    if (-not (Test-Path -LiteralPath $capturePath)) {
        throw "Payment gateway did not capture the create request"
    }
    $capture = Get-Content -LiteralPath $capturePath -Raw | ConvertFrom-Json
    if (-not $capture.request_line.StartsWith("POST ")) {
        throw "Custom gateway received an unexpected request: $($capture.request_line)"
    }
    if (-not $capture.signature_valid) {
        throw "Custom gateway create request signature is invalid"
    }
    $gatewayOrder = $capture.payload | ConvertFrom-Json
    if ([string]$gatewayOrder.trade_no -ne $tradeNo) {
        throw "Gateway payload trade_no mismatch"
    }
    if ([int64]$gatewayOrder.amount -ne [int64]$order.json.data.price_unit) {
        throw "Gateway payload amount mismatch"
    }
    if ([string]$gatewayOrder.currency -ne "CNY") {
        throw "Gateway payload currency mismatch: $($gatewayOrder.currency)"
    }
    $checks.Add([pscustomobject]@{ check = "signed create request carries order identity and amount"; result = "PASS"; value = $tradeNo })

    $created = Get-Admin-Order -Token $adminToken -TradeNo $tradeNo -Label "read pending custom order"
    if ($created.status -ne "created") {
        throw "Custom order expected status created, got $($created.status)"
    }

    $priceUnit = [int64]$order.json.data.price_unit
    $basePayload = @{
        trade_no              = $tradeNo
        provider_reference_id = "probe-paid-$tradeNo"
        amount                = $priceUnit
        currency              = "CNY"
        paid                  = $true
    }

    $badSignatureBody = $basePayload | ConvertTo-Json -Compress
    $badSignature = Send-RawCallback -TradeNo $tradeNo -Body $badSignatureBody -Signature ("0" * 64)
    Assert-Code $badSignature 40001 "tampered callback signature rejected"

    $underpay = $basePayload.Clone()
    $underpay.amount = $priceUnit - 1
    $underpayBody = $underpay | ConvertTo-Json -Compress
    $underpayResult = Send-RawCallback -TradeNo $tradeNo -Body $underpayBody `
        -Signature (Get-HmacHex -Secret $ProviderSecret -Body $underpayBody)
    Assert-Code $underpayResult 40001 "underpaid signed callback rejected"

    $wrongCurrency = $basePayload.Clone()
    $wrongCurrency.currency = "USD"
    $currencyBody = $wrongCurrency | ConvertTo-Json -Compress
    $currencyResult = Send-RawCallback -TradeNo $tradeNo -Body $currencyBody `
        -Signature (Get-HmacHex -Secret $ProviderSecret -Body $currencyBody)
    Assert-Code $currencyResult 40001 "wrong currency signed callback rejected"

    $wrongOrder = $basePayload.Clone()
    $wrongOrder.trade_no = "00000000000000000000000000000000"
    $orderBody = $wrongOrder | ConvertTo-Json -Compress
    $orderResult = Send-RawCallback -TradeNo $tradeNo -Body $orderBody `
        -Signature (Get-HmacHex -Secret $ProviderSecret -Body $orderBody)
    Assert-Code $orderResult 40001 "wrong order signed callback rejected"

    $stillPending = Get-Admin-Order -Token $adminToken -TradeNo $tradeNo -Label "order still readable after rejected callbacks"
    if ($stillPending.status -ne "created") {
        throw "Rejected callbacks changed order status to $($stillPending.status)"
    }
    $checks.Add([pscustomobject]@{ check = "rejected callbacks do not change order state"; result = "PASS"; value = "created" })

    $validBody = $basePayload | ConvertTo-Json -Compress
    $validResult = Send-RawCallback -TradeNo $tradeNo -Body $validBody `
        -Signature (Get-HmacHex -Secret $ProviderSecret -Body $validBody)
    Assert-Code $validResult 0 "valid signed callback accepted"

    $fulfilled = Get-Admin-Order -Token $adminToken -TradeNo $tradeNo -Label "read fulfilled order"
    if ($fulfilled.status -ne "fulfilled") {
        throw "Valid callback expected fulfilled, got $($fulfilled.status)"
    }
    $checks.Add([pscustomobject]@{ check = "valid callback fulfills order"; result = "PASS"; value = "fulfilled" })

    $repeatResult = Send-RawCallback -TradeNo $tradeNo -Body $validBody `
        -Signature (Get-HmacHex -Secret $ProviderSecret -Body $validBody)
    Assert-Code $repeatResult 0 "repeated valid callback is idempotent"
    $repeat = Get-Admin-Order -Token $adminToken -TradeNo $tradeNo -Label "read order after repeated callback"
    if ($repeat.status -ne "fulfilled") {
        throw "Repeated callback changed status to $($repeat.status)"
    }
    $checks.Add([pscustomobject]@{ check = "repeated callback stays fulfilled"; result = "PASS"; value = "fulfilled" })

    [pscustomobject]@{
        checks         = $checks.Count
        trade_no       = $tradeNo
        provider       = $ProviderID
        rejected       = "bad signature, underpayment, wrong currency, wrong order number"
        accepted       = "exact signed callback"
        final_status   = $repeat.status
    } | Format-List
} finally {
    if ($adminToken) {
        Restore-PaymentSettings -Token $adminToken
    }
    if ($gateway -and -not $gateway.HasExited) {
        try { Stop-Process -Id $gateway.Id -Force } catch {}
    }
}
