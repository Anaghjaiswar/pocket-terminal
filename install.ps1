# PocketTerminal Windows Installer
# Usage: irm https://raw.githubusercontent.com/Anaghjaiswar/pocket-terminal/main/install.ps1 | iex

$ErrorActionPreference = 'Stop'

Write-Host "=========================================" -ForegroundColor Cyan
Write-Host "   Installing PocketTerminal for Windows  " -ForegroundColor Cyan
Write-Host "=========================================" -ForegroundColor Cyan

$Repo = "Anaghjaiswar/pocket-terminal"
$InstallDir = Join-Path $env:LOCALAPPDATA "Programs\pocket-terminal"
$ZipPath = Join-Path $env:TEMP "pocket-terminal-windows-amd64.zip"

# Fetch latest release tag
Write-Host "Fetching latest release from GitHub..." -ForegroundColor Yellow
try {
    $ReleaseInfo = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers @{ "User-Agent" = "PowerShell" }
    $Tag = $ReleaseInfo.tag_name
    Write-Host "Latest release: $Tag" -ForegroundColor Green
} catch {
    $Tag = "v1.0.0"
    Write-Host "Could not auto-fetch release tag, falling back to $Tag" -ForegroundColor Yellow
}

$DownloadUrl = "https://github.com/$Repo/releases/download/$Tag/pocket-terminal-windows-amd64.zip"

Write-Host "Downloading $DownloadUrl..." -ForegroundColor Yellow
Invoke-WebRequest -Uri $DownloadUrl -OutFile $ZipPath -UseBasicParsing

if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

Write-Host "Extracting to $InstallDir..." -ForegroundColor Yellow
Expand-Archive -Path $ZipPath -DestinationPath $InstallDir -Force
Remove-Item -Path $ZipPath -Force -ErrorAction SilentlyContinue

# Ensure directory is in user PATH
$UserPath = [Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::User)
if ($UserPath -notlike "*$InstallDir*") {
    Write-Host "Adding $InstallDir to user PATH..." -ForegroundColor Yellow
    [Environment]::SetEnvironmentVariable("Path", "$UserPath;$InstallDir", [EnvironmentVariableTarget]::User)
    $env:Path = "$env:Path;$InstallDir"
}

Write-Host ""
Write-Host "PocketTerminal installed successfully!" -ForegroundColor Green
Write-Host "Run it anytime from your terminal by typing:" -ForegroundColor Cyan
Write-Host "    pocket-terminal" -ForegroundColor White
Write-Host ""
