param(
    [Parameter(Mandatory = $true)][string]$ConfigPath
)

$ErrorActionPreference = "Stop"
$config = Get-Content -LiteralPath $ConfigPath -Raw | ConvertFrom-Json
$readyPath = "$ConfigPath.ready"
$capturePath = "$ConfigPath.capture"

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

$listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, [int]$config.port)
$listener.Start()
Set-Content -LiteralPath $readyPath -Value "ready" -Encoding ASCII

$client = $null
try {
    $acceptTask = $listener.AcceptTcpClientAsync()
    if (-not $acceptTask.Wait(30000)) {
        throw "gateway accept timed out"
    }
    $client = $acceptTask.Result
    $stream = $client.GetStream()
    $reader = [IO.StreamReader]::new($stream, [Text.Encoding]::UTF8, $false, 4096, $true)
    $requestLine = $reader.ReadLine()
    $headers = @{}
    while ($true) {
        $line = $reader.ReadLine()
        if ($null -eq $line -or $line -eq "") {
            break
        }
        $separator = $line.IndexOf(":")
        if ($separator -gt 0) {
            $headers[$line.Substring(0, $separator).Trim().ToLowerInvariant()] = $line.Substring($separator + 1).Trim()
        }
    }

    $contentLength = 0
    if ($headers.ContainsKey("content-length")) {
        $contentLength = [int]$headers["content-length"]
    }
    $buffer = New-Object char[] ([Math]::Max($contentLength, 1))
    $read = 0
    while ($read -lt $contentLength) {
        $count = $reader.Read($buffer, $read, $contentLength - $read)
        if ($count -le 0) {
            break
        }
        $read += $count
    }
    $payload = [string]::new($buffer, 0, $read)
    $signature = $headers["x-Cloudreve-signature"]
    $expected = Get-HmacHex -Secret ([string]$config.secret) -Body $payload
    $capture = @{
        request_line       = $requestLine
        payload            = $payload
        signature          = $signature
        expected_signature = $expected
        signature_valid    = ($signature -and $signature.ToLowerInvariant() -eq $expected)
    }
    $capture | ConvertTo-Json -Depth 8 -Compress | Set-Content -LiteralPath $capturePath -Encoding UTF8

    $reply = [string]$config.reply
    $replyBytes = [Text.Encoding]::UTF8.GetBytes($reply)
    $responseHeaders = "HTTP/1.1 200 OK`r`nContent-Type: application/json`r`nContent-Length: $($replyBytes.Length)`r`nConnection: close`r`n`r`n"
    $headerBytes = [Text.Encoding]::ASCII.GetBytes($responseHeaders)
    $stream.Write($headerBytes, 0, $headerBytes.Length)
    $stream.Write($replyBytes, 0, $replyBytes.Length)
    $stream.Flush()
} finally {
    if ($client) {
        $client.Close()
    }
    $listener.Stop()
}
