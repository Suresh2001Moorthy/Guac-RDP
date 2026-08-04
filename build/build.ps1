$ErrorActionPreference = "Stop"

Write-Host "Building Web-RDP Gateway..."

# Ensure we are at the project root
$ProjectRoot = Split-Path -Parent $PSScriptRoot
Set-Location $ProjectRoot

# Clean previous builds
.\build\clean.ps1

Write-Host "Compiling Go binary..."
$OutputPath = Join-Path $ProjectRoot "build\Release\gateway.exe"

# We use -ldflags="-s -w" to strip debugging information, reducing binary size
go build -ldflags="-s -w" -o $OutputPath .\cmd\gateway\

if ($LASTEXITCODE -ne 0) {
    Write-Error "Go build failed."
    exit 1
}

Write-Host "Build complete: $OutputPath"
