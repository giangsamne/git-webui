# GitWebUI 1-Line Universal Installer for Windows PowerShell
# Author: Giang3DLab / Giang Sam
$ErrorActionPreference = "Stop"

$Repo = "giangsamne/git-webui"
$AppName = "git-webui"
$InstallDir = "$env:LOCALAPPDATA\GitWebUI"

Write-Host "==========================================================" -ForegroundColor Cyan
Write-Host "   ⚡ GitWebUI - Windows 1-Line Installer                " -ForegroundColor Cyan
Write-Host "==========================================================" -ForegroundColor Cyan

$Arch = "amd64"
if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") {
    $Arch = "arm64"
}

$DownloadUrl = "https://github.com/$Repo/releases/latest/download/$AppName-windows-$Arch.zip"
$ZipFile = "$env:TEMP\$AppName-windows-$Arch.zip"

Write-Host "📥 Downloading GitWebUI from $DownloadUrl..." -ForegroundColor Green
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

try {
    Invoke-WebRequest -Uri $DownloadUrl -OutFile $ZipFile -UseBasicParsing
    Expand-Archive -Path $ZipFile -DestinationPath $env:TEMP\$AppName-extract -Force
    Move-Item -Path "$env:TEMP\$AppName-extract\$AppName-windows-$Arch.exe" -Destination "$InstallDir\$AppName.exe" -Force
    Remove-Item -Path $ZipFile -Force
    Remove-Item -Path "$env:TEMP\$AppName-extract" -Recurse -Force
} catch {
    Write-Host "❌ Failed to download binary. Please visit https://github.com/$Repo/releases" -ForegroundColor Red
    exit 1
}

# Add to User PATH if not exists
$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$UserPath;$InstallDir", "User")
    Write-Host "✅ Added $InstallDir to User PATH." -ForegroundColor Yellow
}

Write-Host "==========================================================" -ForegroundColor Cyan
Write-Host " 🎉 Installation Complete!                                " -ForegroundColor Green
Write-Host " 🚀 To start GitWebUI, run:                               " -ForegroundColor Green
Write-Host "    $AppName                                              " -ForegroundColor Yellow
Write-Host " 🌐 Web Dashboard: http://localhost:18090                 " -ForegroundColor Cyan
Write-Host "==========================================================" -ForegroundColor Cyan
