<#
.SYNOPSIS
    Build Cloudreve-New locally the same way the release workflow does.

.DESCRIPTION
    1. Builds the frontend in web/ with pnpm
    2. Packs web/build into src/application/statics/assets.zip (assets/build prefix)
    3. Runs goreleaser for every released platform

    Runs the full 12-platform matrix by default, exactly like
    .github/workflows/release.yml. Pass -Fast to build only the host platform.

.PARAMETER Version
    Version stamped into the binary and version.json. Defaults to web/package.json.

.PARAMETER Fast
    Build only the current OS/arch instead of the full matrix.

.PARAMETER SkipFrontend
    Reuse the existing assets.zip and skip the pnpm build.

.EXAMPLE
    .\tools\build-release.ps1

.EXAMPLE
    .\tools\build-release.ps1 -Version 4.19.1 -Fast
#>
param(
    [string]$Version,
    [switch]$Fast,
    [switch]$SkipFrontend
)

$ErrorActionPreference = "Stop"
$root = Split-Path $PSScriptRoot -Parent
$web = Join-Path $root "web"
$src = Join-Path $root "src"
$statics = Join-Path $src "application/statics"
$assetsZip = Join-Path $statics "assets.zip"

function Resolve-Tool([string]$Name, [string[]]$Candidates) {
    $cmd = Get-Command $Name -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    foreach ($c in $Candidates) {
        if ($c -and (Test-Path -LiteralPath $c)) { return $c }
    }
    throw "$Name not found. Install it or pass its path explicitly."
}

if (-not $Version) {
    $Version = (Get-Content (Join-Path $web "package.json") -Raw | ConvertFrom-Json).version
}
Write-Host "Cloudreve-New $Version" -ForegroundColor Cyan

$pnpm = Resolve-Tool "pnpm" @(
    (Join-Path $env:LOCALAPPDATA "pnpm/pnpm.exe")
)
$go = Resolve-Tool "go" @(
    (Join-Path $root "tools/go/bin/go.exe")
)
$goreleaser = Resolve-Tool "goreleaser" @(
    (Join-Path $root ".tmp/bin/goreleaser.exe")
)

$env:GOPATH = Join-Path $root ".gopath"
$env:GOCACHE = Join-Path $root ".gocache"
$env:GOFLAGS = "-mod=mod"
$env:CGO_ENABLED = "0"

if (-not $SkipFrontend) {
    Write-Host "`n[1/3] Building frontend" -ForegroundColor Yellow
    Push-Location $web
    try {
        & $pnpm install --frozen-lockfile --config.dangerouslyAllowAllBuilds=true
        if ($LASTEXITCODE -ne 0) { throw "pnpm install failed" }

        $env:npm_package_name = "cloudreve-frontend"
        $env:npm_package_version = $Version
        $env:NODE_OPTIONS = "--max-old-space-size=8192"
        & $pnpm build
        if ($LASTEXITCODE -ne 0) { throw "pnpm build failed" }
    }
    finally {
        Pop-Location
        Remove-Item Env:npm_package_name, Env:npm_package_version -ErrorAction SilentlyContinue
    }

    $built = Join-Path $web "build/index.html"
    if (-not (Test-Path -LiteralPath $built)) { throw "frontend output missing: $built" }

    Write-Host "`n[2/3] Packing assets.zip" -ForegroundColor Yellow
    New-Item -ItemType Directory -Force -Path $statics | Out-Null
    Remove-Item -LiteralPath $assetsZip -Force -ErrorAction SilentlyContinue

    # fs.Sub(statics, "assets/build") requires the assets/build prefix.
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $zip = [System.IO.Compression.ZipFile]::Open($assetsZip, "Create")
    try {
        Get-ChildItem -LiteralPath (Join-Path $web "build") -Recurse -File | ForEach-Object {
            $rel = $_.FullName.Substring((Join-Path $web "build").Length).TrimStart("\", "/")
            $entry = "assets/build/" + ($rel -replace "\\", "/")
            [void][System.IO.Compression.ZipFileExtensions]::CreateEntryFromFile(
                $zip, $_.FullName, $entry, [System.IO.Compression.CompressionLevel]::Optimal)
        }
    }
    finally { $zip.Dispose() }
    Write-Host "  assets.zip $((Get-Item $assetsZip).Length) bytes"
}
else {
    if (-not (Test-Path -LiteralPath $assetsZip)) { throw "-SkipFrontend needs an existing assets.zip" }
    Write-Host "`n[1/3] Skipping frontend build" -ForegroundColor Yellow
    Write-Host "`n[2/3] Reusing assets.zip" -ForegroundColor Yellow
}

Write-Host "`n[3/3] Compiling backend" -ForegroundColor Yellow
Push-Location $src
try {
    & $go mod download
    if ($LASTEXITCODE -ne 0) { throw "go mod download failed" }

    if ($Fast) {
        $ldflags = "-s -w " +
            "-X 'github.com/cloudreve/Cloudreve/v4/application/constants.BackendVersion=$Version' " +
            "-X 'github.com/cloudreve/Cloudreve/v4/application/constants.LastCommit=local' " +
            "-X 'github.com/cloudreve/Cloudreve/v4/application/constants.IsPro=false'"
        & $go build -trimpath -ldflags $ldflags -o (Join-Path $root "Cloudreve-New.exe") .
        if ($LASTEXITCODE -ne 0) { throw "go build failed" }
        Write-Host "`nDone: $root\Cloudreve-New.exe" -ForegroundColor Green
    }
    else {
        & $goreleaser build --snapshot --clean
        if ($LASTEXITCODE -ne 0) { throw "goreleaser failed" }
        Write-Host "`nDone: $src\dist" -ForegroundColor Green
    }
}
finally {
    Pop-Location
}
