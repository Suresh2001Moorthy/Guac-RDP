$ErrorActionPreference = "Stop"

$ProjectRoot = Split-Path -Parent $PSScriptRoot
$ReleaseDir = Join-Path $ProjectRoot "build\Release"

Write-Host "Cleaning up previous build artifacts..."

if (Test-Path $ReleaseDir) {
    Remove-Item -Path $ReleaseDir -Recurse -Force
}

New-Item -ItemType Directory -Path $ReleaseDir -Force | Out-Null
Write-Host "Cleaned $ReleaseDir"
