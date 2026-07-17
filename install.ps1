#!/usr/bin/env pwsh
<#
.SYNOPSIS
    figma-map installer for Windows.

.DESCRIPTION
    Downloads three things from the matching GitHub release: the figma-map
    CLI binary, a standalone backend bundle (no Node install needed to run
    it), and the Figma plugin (no build step needed to load it). Run this
    yourself, in your own PowerShell window — a coding agent should tell
    you to run it, not run it for you, since piping a remote script into a
    shell is something safety-conscious agents rightly refuse to do on
    their own.

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

# figma-map's home-scoped state dir — must match internal/service's
# StateDir()/stateDir() (Go's os.UserHomeDir() resolves to $env:USERPROFILE
# on Windows) exactly, since Go code looks for what this script writes here.
$StateDir = Join-Path $env:USERPROFILE ".figma-map"

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
# A user-supplied -Version/$env:FIGMA_MAP_VERSION might omit "v" — normalize
# so cache paths always agree with Go's release.NormalizeTag.
if ($Version -notmatch '^v') { $Version = "v$Version" }
Write-Info "Version     $Version"

$VersionNoV = $Version.TrimStart("v")
# $env:FIGMA_MAP_BASE_URL overrides the release-download base URL, for e2e
# testing against local fixtures instead of real GitHub (see test/e2e/).
$BaseUrl    = if ($env:FIGMA_MAP_BASE_URL) { $env:FIGMA_MAP_BASE_URL } else { "https://github.com/$Repo/releases/download/$Version" }

if (-not $InstallDir) { $InstallDir = Join-Path $env:LOCALAPPDATA "figma-map\bin" }
$BackendDir = Join-Path $StateDir "versions\$Version\backend"
$PluginDir  = Join-Path $StateDir "plugin"

Write-Host ""
Write-Host "This will install:" -ForegroundColor White
Write-Host "  - the figma-map CLI to $(Join-Path $InstallDir $Binary)"
Write-Host "  - a backend bundle to $BackendDir\ (no Node install needed)"
Write-Host "  - the Figma plugin to $PluginDir\ (no build step needed)"
Write-Host ""

# verifies $ArchivePath against a "<name> checksums.txt"-shaped file at
# $ChecksumsPath — used for the CLI archive, which goreleaser itself builds
# and checksums.
function Test-ChecksumsTxt($ArchivePath, $ArchiveName, $ChecksumsPath) {
    $line = Select-String -Path $ChecksumsPath -Pattern " $ArchiveName$" | Select-Object -First 1
    if (-not $line) { Die "no checksum entry for $ArchiveName" }
    $expected = ($line.Line -split '\s+')[0]
    $actual = (Get-FileHash -Path $ArchivePath -Algorithm SHA256).Hash.ToLower()
    if ($expected -ne $actual) {
        Die "checksum mismatch for ${ArchiveName}!`n    expected $expected`n    actual   $actual"
    }
}

# verifies $ArchivePath against a "<archive>.sha256" sidecar (just a hex
# digest) — used for the backend/plugin archives, which are extra_files
# goreleaser's own checksums.txt doesn't cover. Returns $false (with a
# warning, not a Die) on any mismatch, since a failure here shouldn't block
# the CLI install that already succeeded.
function Test-Sidecar($ArchivePath, $SidecarPath, $ArchiveName) {
    if (-not (Test-Path $SidecarPath)) {
        Write-Warn "no checksum available for $ArchiveName — skipping"
        return $false
    }
    $expected = (Get-Content $SidecarPath -Raw).Trim().Split()[0]
    $actual = (Get-FileHash -Path $ArchivePath -Algorithm SHA256).Hash.ToLower()
    if ($expected -ne $actual) {
        Write-Warn "checksum mismatch for $ArchiveName — skipping"
        return $false
    }
    return $true
}

$Tmp = Join-Path ([System.IO.Path]::GetTempPath()) ([System.IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $Tmp | Out-Null
try {
    # --- CLI ---
    $Archive = "figma-map_${VersionNoV}_windows_${Arch}.zip"
    $ArchivePath = Join-Path $Tmp $Archive
    Write-Info "Downloading $Archive (CLI)"
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
    Test-ChecksumsTxt $ArchivePath $Archive $ChecksumsPath
    Write-Ok "CLI checksum verified"

    Expand-Archive -Path $ArchivePath -DestinationPath $Tmp -Force
    $BinaryPath = Join-Path $Tmp $Binary
    if (-not (Test-Path $BinaryPath)) { Die "binary $Binary not found in archive" }

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $Dest = Join-Path $InstallDir $Binary
    Copy-Item -Path $BinaryPath -Destination $Dest -Force
    Write-Ok "CLI installed to $Dest"

    # --- Backend (best-effort: bridge up / figma-map update fetch it later if this fails) ---
    Write-Host ""
    $BackendArchive = "figma-map-backend_${VersionNoV}_windows_${Arch}.zip"
    $BackendArchivePath = Join-Path $Tmp $BackendArchive
    Write-Info "Downloading $BackendArchive (backend)"
    $backendOk = $true
    try {
        Invoke-WebRequest -Uri "$BaseUrl/$BackendArchive" -OutFile $BackendArchivePath -UseBasicParsing
    } catch {
        Write-Warn "no backend release asset for windows/$Arch at $Version — 'bridge up' will fetch it on first use"
        $backendOk = $false
    }
    if ($backendOk) {
        $BackendSidecar = Join-Path $Tmp "$BackendArchive.sha256"
        try {
            Invoke-WebRequest -Uri "$BaseUrl/$BackendArchive.sha256" -OutFile $BackendSidecar -UseBasicParsing
        } catch {
            Write-Warn "no checksum for $BackendArchive — skipping backend install"
            $backendOk = $false
        }
    }
    if ($backendOk) {
        $backendOk = Test-Sidecar $BackendArchivePath $BackendSidecar $BackendArchive
    }
    if ($backendOk) {
        Write-Ok "backend checksum verified"
        New-Item -ItemType Directory -Path $BackendDir -Force | Out-Null
        Expand-Archive -Path $BackendArchivePath -DestinationPath $BackendDir -Force
        Write-Ok "backend installed to $BackendDir"
    }

    # --- Figma plugin (best-effort, same reasoning as backend) ---
    Write-Host ""
    $PluginArchive = "figma-map-plugin.zip"
    $PluginArchivePath = Join-Path $Tmp $PluginArchive
    Write-Info "Downloading $PluginArchive"
    $pluginOk = $true
    try {
        Invoke-WebRequest -Uri "$BaseUrl/$PluginArchive" -OutFile $PluginArchivePath -UseBasicParsing
    } catch {
        Write-Warn "no plugin release asset at $Version — download it manually from the releases page"
        $pluginOk = $false
    }
    if ($pluginOk) {
        $PluginSidecar = Join-Path $Tmp "$PluginArchive.sha256"
        try {
            Invoke-WebRequest -Uri "$BaseUrl/$PluginArchive.sha256" -OutFile $PluginSidecar -UseBasicParsing
        } catch {
            Write-Warn "no checksum for $PluginArchive — skipping plugin install"
            $pluginOk = $false
        }
    }
    if ($pluginOk) {
        $pluginOk = Test-Sidecar $PluginArchivePath $PluginSidecar $PluginArchive
    }
    if ($pluginOk) {
        Write-Ok "plugin checksum verified"
        $ExtractDir = Join-Path $Tmp "plugin-extract"
        Expand-Archive -Path $PluginArchivePath -DestinationPath $ExtractDir -Force
        $ExtractedRoot = Join-Path $ExtractDir "figma-map-plugin"
        if (Test-Path (Join-Path $ExtractedRoot "manifest.json")) {
            Set-Content -Path (Join-Path $ExtractedRoot ".version") -Value $Version -Encoding ascii -NoNewline
            if (Test-Path $PluginDir) { Remove-Item -Recurse -Force $PluginDir }
            Move-Item -Path $ExtractedRoot -Destination $PluginDir
            Write-Ok "plugin installed to $PluginDir"
        } else {
            Write-Warn "plugin archive missing manifest.json — unexpected layout, skipping"
        }
    }

    Write-Host ""
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath -notlike "*$InstallDir*") {
        [Environment]::SetEnvironmentVariable("Path", "$userPath;$InstallDir", "User")
        $env:Path = "$env:Path;$InstallDir"
        Write-Warn "$InstallDir was not on PATH — added it (open a new terminal for it to take effect)"
    }
    try {
        & $Dest --version
    } catch {
        Write-Ok "$Binary $Version"
    }

    Write-Host ""
    Write-Host "Installed:" -ForegroundColor White
    Write-Host "  CLI      $Dest"
    Write-Host "  backend  $BackendDir\"
    Write-Host "  plugin   $PluginDir\"
    Write-Host ""
    Write-Host "Update:   " -NoNewline -ForegroundColor White
    Write-Host "figma-map update" -ForegroundColor Cyan
    Write-Host "Uninstall:" -NoNewline -ForegroundColor White
    Write-Host " figma-map uninstall" -ForegroundColor Cyan

    $ManifestPath = Join-Path $PluginDir "manifest.json"
    if (Test-Path $ManifestPath) {
        Write-Host ""
        Write-Host "Load the plugin in Figma (one-time):" -ForegroundColor White
        Write-Host "  Figma -> Plugins -> Development -> Import plugin from manifest..."
        Write-Host "  select: $ManifestPath" -ForegroundColor Cyan
        try {
            Set-Clipboard -Value $PluginDir
            Write-Host "  (folder path copied to your clipboard - paste it into the dialog's" -ForegroundColor DarkGray
            Write-Host "  filename box to jump straight there - .figma-map is a hidden folder)" -ForegroundColor DarkGray
        } catch {
            Write-Host "  (.figma-map is a hidden folder - your file picker may not show it by" -ForegroundColor DarkGray
            Write-Host "  default; paste the path above directly into the dialog's filename box)" -ForegroundColor DarkGray
        }
        try {
            Start-Process explorer.exe -ArgumentList "/select,`"$ManifestPath`""
        } catch {}
    }

    Write-Host ""
    Write-Host "Next:" -NoNewline -ForegroundColor White
    Write-Host " open your coding agent and paste:"
    Write-Host ""
    Write-Host "  ""figma-map is installed - read the figma-map-setup skill and finish setting it up for this project.""" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "No agent? Run " -NoNewline
    Write-Host "figma-map doctor" -NoNewline -ForegroundColor Cyan
    Write-Host " yourself to check the bridge, then " -NoNewline
    Write-Host "figma-map init <path>" -ForegroundColor Cyan
}
finally {
    Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue
}
