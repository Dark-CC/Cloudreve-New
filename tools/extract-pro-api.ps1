$files = Get-ChildItem -LiteralPath (Join-Path (Split-Path $PSScriptRoot -Parent) ".pro-web") -Filter "*.js" -File
$matches = foreach ($file in $files) {
    $text = Get-Content -LiteralPath $file.FullName -Raw
    [regex]::Matches($text, "api/v4/[A-Za-z0-9_/`${}.:-]+") | ForEach-Object { $_.Value }
}
$matches | Sort-Object -Unique
