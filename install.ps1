#!/usr/bin/env pwsh
<#
.SYNOPSIS
    figma-map installer for Windows.

.DESCRIPTION
    Downloads the latest (or a pinned) figma-map release for windows/amd64,
    verifies its SHA-256 checksum, and installs the binary.

.EXAMPLE
    irm https://raw.githubusercontent.com/KirillBaranov/figma-map/main/install.ps1 | iex

.PARAMETER Version
    Install a specific tag (e.g. v0.6.0). Defaults to the latest release.

.PARAMETER InstallDir
    Directory to install figma-map.exe into. Defaults to
    "$env:LOCALAPPDATA\figma-map\bin".
#>
param(
    [string]$Version = $env:FIGMA_MAP_VERSION,
    [string]$InstallDir = $env:FIGMA_MAP_INSTALL_DIR
)

$ErrorActionPreference = "Stop"

$Repo   = "kirillbaranov/figma-map"
$Binary = "figma-map.exe"

function Write-Info($msg)  { Write-Host "›  $msg" -ForegroundColor Cyan }
function Write-Ok($msg)    { Write-Host "✓  $msg" -ForegroundColor Green }
function Write-Warn($msg)  { Write-Host "!  $msg" -ForegroundColor Yellow }
function Die($msg)         { Write-Host "✗  $msg" -ForegroundColor Red; exit 1 }

# ---------------------------------------------------------------------------
# Detect platform
# ---------------------------------------------------------------------------
$Arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { "amd64" }
    "ARM64" { Die "figma-map does not currently publish a windows/arm64 build — build from source with 'go build' instead, or run under x64 emulation." }
    default { Die "unsupported architecture: $($env:PROCESSOR_ARCHITECTURE)" }
}
Write-Info "Platform    windows/$Arch"

# ---------------------------------------------------------------------------
# Resolve version
# ---------------------------------------------------------------------------
if (-not $Version) {
    Write-Info "Resolving   latest release"
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -UseBasicParsing
    $Version = $release.tag_name
    if (-not $Version) { Die "could not resolve latest version (set -Version or `$env:FIGMA_MAP_VERSION)" }
}
Write-Info "Version     $Version"

$VersionNoV = $Version.TrimStart("v")
$Archive    = "figma-map_${VersionNoV}_windows_${Arch}.zip"
$BaseUrl    = "https://github.com/$Repo/releases/download/$Version"

$Tmp = Join-Path ([System.IO.Path]::GetTempPath()) ([System.IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $Tmp | Out-Null
try {
    $ArchivePath = Join-Path $Tmp $Archive
    Write-Info "Downloading $Archive"
    try {
        Invoke-WebRequest -Uri "$BaseUrl/$Archive" -OutFile $ArchivePath -UseBasicParsing
    } catch {
        Die "download failed — no release asset for windows/$Arch at ${Version}?"
    }

    Write-Info "Verifying   checksum"
    $ChecksumsPath = Join-Path $Tmp "checksums.txt"
    try {
        Invoke-WebRequest -Uri "$BaseUrl/checksums.txt" -OutFile $ChecksumsPath -UseBasicParsing
    } catch {
        Die "could not download checksums.txt"
    }
    $expectedLine = Select-String -Path $ChecksumsPath -Pattern " $Archive$" | Select-Object -First 1
    if (-not $expectedLine) { Die "no checksum entry for $Archive" }
    $expected = ($expectedLine.Line -split '\s+')[0]
    $actual = (Get-FileHash -Path $ArchivePath -Algorithm SHA256).Hash.ToLower()
    if ($expected -ne $actual) {
        Die "checksum mismatch!`n    expected $expected`n    actual   $actual"
    }
    Write-Ok "checksum verified (sha256 $($actual.Substring(0,12))...)"

    Expand-Archive -Path $ArchivePath -DestinationPath $Tmp -Force
    $BinaryPath = Join-Path $Tmp $Binary
    if (-not (Test-Path $BinaryPath)) { Die "binary $Binary not found in archive" }

    if (-not $InstallDir) { $InstallDir = Join-Path $env:LOCALAPPDATA "figma-map\bin" }
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null

    $Dest = Join-Path $InstallDir $Binary
    Copy-Item -Path $BinaryPath -Destination $Dest -Force
    Write-Ok "installed to $Dest"

    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath -notlike "*$InstallDir*") {
        [Environment]::SetEnvironmentVariable("Path", "$userPath;$InstallDir", "User")
        $env:Path = "$env:Path;$InstallDir"
        Write-Warn "$InstallDir was not on PATH — added it (open a new terminal for it to take effect)"
    }

    Write-Host ""
    try {
        & $Dest --version
    } catch {
        Write-Ok "$Binary $Version"
    }

    Write-Host ""
    Write-Host "Next:" -NoNewline -ForegroundColor White
    Write-Host " run " -NoNewline
    Write-Host "figma-map doctor" -NoNewline -ForegroundColor Cyan
    Write-Host " to verify your setup."
    Write-Host "Then:" -NoNewline -ForegroundColor White
    Write-Host " run " -NoNewline
    Write-Host "figma-map init <path>" -NoNewline -ForegroundColor Cyan
    Write-Host " to add the Claude Code skill + config to your project."
}
finally {
    Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue
}
