<#
.SYNOPSIS
    Commit local files through the GitHub Git Data API.

.DESCRIPTION
    The git wire protocol to github.com is intermittently reset on this network,
    so this helper uploads blobs over the REST API instead. It reads the current
    master tip, builds a tree on top of it, creates a commit, and advances the ref.

.PARAMETER Files
    Repository-relative paths to commit. Working tree contents are used.

.PARAMETER Message
    Commit message.

.EXAMPLE
    $env:GITHUB_TOKEN = "ghp_xxx"
    .\tools\api_commit.ps1 -Files Dockerfile,src/.goreleaser.yaml -Message "fix: x"
#>
param(
    [Parameter(Mandatory = $true)][string[]]$Files,
    [Parameter(Mandatory = $true)][string]$Message,
    [string]$Token = $env:GITHUB_TOKEN,
    [string]$Repository = "Dark-CC/Cloudreve-New",
    [string]$Branch = "master"
)

$ErrorActionPreference = "Stop"

if (-not $Token) { throw "No token. Set `$env:GITHUB_TOKEN or pass -Token." }

$headers = @{
    Authorization = "token $Token"
    "User-Agent"  = "codex"
    Accept        = "application/vnd.github+json"
}
$api = "https://api.github.com/repos/$Repository"
$root = Split-Path $PSScriptRoot -Parent

$tip = (Invoke-RestMethod -Uri "$api/git/ref/heads/$Branch" -Headers $headers -TimeoutSec 20).object.sha
$baseTree = (Invoke-RestMethod -Uri "$api/git/commits/$tip" -Headers $headers -TimeoutSec 20).tree.sha
Write-Host "base $($tip.Substring(0,7)) tree $($baseTree.Substring(0,7))"

$entries = @()
foreach ($file in $Files) {
    $full = Join-Path $root $file
    if (-not (Test-Path -LiteralPath $full)) { throw "missing file: $file" }

    $blob = Invoke-RestMethod -Uri "$api/git/blobs" -Headers $headers -Method Post `
        -ContentType "application/json" -TimeoutSec 60 `
        -Body (@{
            content  = [Convert]::ToBase64String([IO.File]::ReadAllBytes($full))
            encoding = "base64"
        } | ConvertTo-Json -Compress)

    $mode = if ($file -like "*.sh") { "100755" } else { "100644" }
    $entries += @{ path = $file; mode = $mode; type = "blob"; sha = $blob.sha }
    Write-Host "  blob $file ($mode)"
}

$tree = (Invoke-RestMethod -Uri "$api/git/trees" -Headers $headers -Method Post `
    -ContentType "application/json" -TimeoutSec 60 `
    -Body (@{ base_tree = $baseTree; tree = $entries } | ConvertTo-Json -Depth 6 -Compress)).sha

$commit = (Invoke-RestMethod -Uri "$api/git/commits" -Headers $headers -Method Post `
    -ContentType "application/json" -TimeoutSec 60 `
    -Body (@{ message = $Message; tree = $tree; parents = @($tip) } | ConvertTo-Json -Depth 5 -Compress)).sha

Invoke-RestMethod -Uri "$api/git/refs/heads/$Branch" -Headers $headers -Method Patch `
    -ContentType "application/json" -TimeoutSec 60 `
    -Body (@{ sha = $commit; force = $false } | ConvertTo-Json -Compress) | Out-Null

Write-Host "commit $($commit.Substring(0,7)) pushed to $Branch" -ForegroundColor Green
