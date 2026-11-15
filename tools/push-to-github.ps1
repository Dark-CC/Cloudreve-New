<#
.SYNOPSIS
    Push this repository to GitHub over HTTPS.

.DESCRIPTION
    Works around two environment problems:
      1. github.com resolves to a poisoned IP on some networks, so the
         connection is reset. The real IP is pinned with curloptResolve.
      2. The token is passed through an ephemeral auth header instead of being
         written into .git/config, so it never lands on disk.

.PARAMETER Token
    A PAT with `repo` scope. Defaults to $env:GITHUB_TOKEN.

.PARAMETER Repository
    target repository, `owner/name`.

.EXAMPLE
    $env:GITHUB_TOKEN = "ghp_xxx"
    .\tools\push-to-github.ps1 -Repository "Dark-CC/Cloudreve-New"
#>
param(
    [string]$Token = $env:GITHUB_TOKEN,
    [string]$Repository = "Dark-CC/Cloudreve-New",
    [string]$Branch = "master",
    [string]$ResolveIp = "140.82.113.4"
)

$ErrorActionPreference = "Stop"

if (-not $Token) {
    throw "No token. Set `$env:GITHUB_TOKEN or pass -Token (needs 'repo' scope)."
}

$repoRoot = Split-Path $PSScriptRoot -Parent
Push-Location $repoRoot
try {
    $url = "https://github.com/$Repository.git"

    git remote remove origin 2>$null
    git remote add origin $url

    $basic = [Convert]::ToBase64String(
        [Text.Encoding]::ASCII.GetBytes("x-access-token:$Token")
    )
    $header = "Authorization: Basic $basic"

    # --force-with-lease is unsafe on a first push; plain push is correct here.
    git -c "http.curloptResolve=github.com:443:$ResolveIp" `
        -c "http.extraHeader=$header" `
        -c credential.helper= `
        push -u origin $Branch

    if ($LASTEXITCODE -ne 0) {
        throw "git push failed with exit code $LASTEXITCODE"
    }

    Write-Host "Pushed $Branch to $url" -ForegroundColor Green
}
finally {
    Pop-Location
    # Never leave the token behind in the remote URL.
    git remote set-url origin "https://github.com/$Repository.git" 2>$null
}
