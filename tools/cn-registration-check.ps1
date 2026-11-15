param(
    [string]$BaseUrl = "http://127.0.0.1:5212",
    [string]$Password = "{{SECRET_4bd4dxw5}}"
)

$ErrorActionPreference = "Stop"
$stamp = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
$email = "register-check-$stamp@test.local"

$payload = @{
    email    = $email
    password = $Password
    language = "zh-CN"
} | ConvertTo-Json

$res = Invoke-WebRequest -Method Post -Uri "$BaseUrl/api/v4/user" `
    -ContentType "application/json" -Body $payload -UseBasicParsing -SkipHttpErrorCheck
$body = $res.Content | ConvertFrom-Json

if ($body.code -ne 0) {
    throw "Registration failed ($($body.code)): $($body.msg)"
}

$login = Invoke-WebRequest -Method Post -Uri "$BaseUrl/api/v4/session/token" `
    -ContentType "application/json" `
    -Body (@{ email = $email; password = $Password } | ConvertTo-Json) `
    -UseBasicParsing -SkipHttpErrorCheck
$loginBody = $login.Content | ConvertFrom-Json
if ($loginBody.code -ne 0) {
    throw "New account login failed ($($loginBody.code)): $($loginBody.msg)"
}

[pscustomobject]@{
    email       = $email
    register    = $body.code
    login       = $loginBody.code
    user_hashid = $body.data.hash_id
} | Format-List
