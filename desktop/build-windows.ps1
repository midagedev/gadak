# Build a Windows portable directory from the desktop module.
#
# Prerequisites: `npm run build` at the repo root (the app embeds dist/app),
# Go. No wails3 CLI: plain `go build`, same contract as desktop/build-app.sh
# and desktop/build-linux.sh. Compiling is a cross-compile (CGO_ENABLED=0,
# GOOS=windows) and does not need a Windows host — measured from macOS.
# What still needs Windows is Authenticode signing (GDK-211) and any
# WebView2 runtime/bootstrap installer work; this script does neither.
#
# Usage: desktop/build-windows.ps1 [--arch x64|arm64] [--archive]
#
# Exit 64 = usage / unknown argument
#      69 = a required tool is missing
#       1 = dist/app missing or a build step failed
#       0 = portable directory written (and the zip, if --archive)
#
# This script must not emit gadak-desktop-<os>-<arch>.zip — v0.14.0 apps
# match that name exactly and would self-swap (same rule as
# desktop/build-app.sh --dmg and desktop/build-linux.sh).
# --archive writes Gadak-<ver>-windows-<x64|arm64>.zip instead: a portable
# directory zip, not an installer. An unsigned installer is more Windows
# friction than a zip, and 0.16 has no Authenticode certificate (GDK-211).
# That name also cannot collide with the goreleaser CLI zip
# (gadak_<ver>_windows_<amd64|arm64>.zip).
#
# WebView2 (decision, 2026-08-18):
#   Evergreen runtime, not a Fixed Version bundle. A Fixed Version tree is
#   hundreds of MB and this pack is a directory, not an installer that
#   could update it. Windows 11 ships the Evergreen runtime; many Windows 10
#   machines already have it via Edge.
#   Missing runtime (from wails v3.0.0-beta.9 source, not launched here):
#   webviewloader returns "no webview2 found"; Chromium.Embed waits at most
#   30s for the controller, then Chromium.errorCallback logs and os.Exit(1).
#   gadak-desktop sets ErrorHandler to handleDesktopFatal
#   (desktop/main.go), which shows a MessageBoxW (fatal_windows.go) and
#   writes stderr; wails still os.Exit(1) after the handler returns, so
#   there is no download dialog — the process exits. Unverified on a real
#   Windows machine. Runtime: https://developer.microsoft.com/en-us/microsoft-edge/webview2/
#
# Signing is out of scope (GDK-211). The unsigned exe is expected to hit
# SmartScreen on first download; that is documented in desktop/README.md,
# not hidden.

$ErrorActionPreference = 'Stop'

function Usage {
    [Console]::Error.WriteLine('usage: desktop/build-windows.ps1 [--arch x64|arm64] [--archive]')
    exit 64
}

function Need {
    param([string]$Name)
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        [Console]::Error.WriteLine("build-windows: missing $Name")
        exit 69
    }
}

$wantArchive = $false
$archArg = $null
for ($i = 0; $i -lt $args.Count; $i++) {
    $arg = [string]$args[$i]
    switch ($arg) {
        { $_ -in @('-h', '--help') } { Usage }
        { $_ -in @('--archive', '-Archive', '-archive') } { $wantArchive = $true }
        { $_ -in @('--arch', '-Arch', '-arch') } {
            if ($i + 1 -ge $args.Count) {
                [Console]::Error.WriteLine('build-windows: --arch needs x64 or arm64')
                Usage
            }
            $i++
            $archArg = [string]$args[$i]
        }
        default {
            [Console]::Error.WriteLine("build-windows: unknown argument: $arg")
            Usage
        }
    }
}

$here = $PSScriptRoot
if ([string]::IsNullOrEmpty($here)) {
    $here = Split-Path -Parent $MyInvocation.MyCommand.Path
}
$repo = (Resolve-Path (Join-Path $here '..')).Path
$out = Join-Path $repo 'desktop/build'

# Same formula as desktop/build-app.sh — read that file's assignment so the
# packs cannot drift. A shared helper is not in this round's file list.
function Get-GadakVersion {
    param([string]$Repo)
    $buildApp = Join-Path $Repo 'desktop/build-app.sh'
    if (-not (Test-Path -LiteralPath $buildApp)) {
        [Console]::Error.WriteLine("build-windows: missing $buildApp")
        exit 1
    }
    $versionLine = Select-String -LiteralPath $buildApp -Pattern '^version=' | Select-Object -First 1
    if ($null -eq $versionLine) {
        [Console]::Error.WriteLine('build-windows: cannot find version= in desktop/build-app.sh')
        exit 1
    }
    $line = $versionLine.Line
    $bash = Get-Command bash -ErrorAction SilentlyContinue
    if ($null -ne $bash) {
        $got = & $bash.Source -c 'repo="$1"; eval "$(grep -E "^version=" "$repo/desktop/build-app.sh")"; printf %s "$version"' _ $Repo
        if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrEmpty($got)) {
            [Console]::Error.WriteLine('build-windows: version stamp from desktop/build-app.sh was empty')
            exit 1
        }
        return $got
    }
    if ($line -notmatch 'git describe --tags --always') {
        [Console]::Error.WriteLine('build-windows: version= line in desktop/build-app.sh no longer uses git describe --tags --always; bash is missing so the assignment cannot be evaled')
        exit 1
    }
    Need git
    Push-Location $Repo
    try {
        $desc = & git describe --tags --always 2>$null
        if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrEmpty($desc)) {
            return '0.0.0-dev'
        }
        return ([string]$desc).Trim()
    }
    finally {
        Pop-Location
    }
}

# Compiling the unsigned portable pack is CGO_ENABLED=0 GOOS=windows and
# works off Windows (measured from macOS). Signing and WebView2 bootstrap
# still need a Windows host — this script does not do those.

Need go

$version = Get-GadakVersion -Repo $repo
if ([string]::IsNullOrEmpty($version)) {
    [Console]::Error.WriteLine('build-windows: version stamp from desktop/build-app.sh was empty')
    exit 1
}
$verStamp = $version -replace '^v', ''

# Windows pack labels (x64, arm64), not the macOS dmg labels (amd64, arm64)
# or the AppImage / uname labels (x86_64, aarch64).
# Directory: Gadak-<ver>-<this>/
# Archive (optional): Gadak-<ver>-windows-<this>.zip
$fileArch = $null
$goarch = $null
if (-not [string]::IsNullOrEmpty($archArg)) {
    switch ($archArg.ToLowerInvariant()) {
        'x64' { $fileArch = 'x64'; $goarch = 'amd64' }
        'arm64' { $fileArch = 'arm64'; $goarch = 'arm64' }
        default {
            [Console]::Error.WriteLine("build-windows: --arch must be x64 or arm64 (got $archArg)")
            exit 64
        }
    }
} else {
    $rawArch = $env:PROCESSOR_ARCHITECTURE
    if (-not [string]::IsNullOrEmpty($env:PROCESSOR_ARCHITEW6432)) {
        $rawArch = $env:PROCESSOR_ARCHITEW6432
    }
    if ([string]::IsNullOrEmpty($rawArch)) {
        $unameCmd = Get-Command uname -ErrorAction SilentlyContinue
        if ($null -ne $unameCmd) {
            $rawArch = (& $unameCmd.Source -m 2>$null)
        }
    }
    if ([string]::IsNullOrEmpty($rawArch)) {
        [Console]::Error.WriteLine('build-windows: cannot detect architecture; pass --arch x64 or --arch arm64')
        exit 1
    }
    switch ($rawArch.ToUpperInvariant()) {
        'AMD64' { $fileArch = 'x64'; $goarch = 'amd64' }
        'X86_64' { $fileArch = 'x64'; $goarch = 'amd64' }
        'ARM64' { $fileArch = 'arm64'; $goarch = 'arm64' }
        'AARCH64' { $fileArch = 'arm64'; $goarch = 'arm64' }
        default {
            [Console]::Error.WriteLine("build-windows: unsupported architecture: $rawArch")
            exit 1
        }
    }
}

$index = Join-Path $repo 'dist/app/index.html'
if (-not (Test-Path -LiteralPath $index)) {
    [Console]::Error.WriteLine('dist/app missing — run `npm run build` at the repo root first')
    exit 1
}

$bundle = Join-Path $out ("Gadak-{0}-{1}" -f $verStamp, $fileArch)
if (Test-Path -LiteralPath $bundle) {
    Remove-Item -LiteralPath $bundle -Recurse -Force
}
New-Item -ItemType Directory -Path $bundle | Out-Null

# wails v3 on Windows uses the Evergreen WebView2 runtime (syscall/COM, no
# CGO). CGO_ENABLED=0 matches the CLI pack and avoids a gcc requirement.
# appVersion is what the sidebar banner compares (desktop/main.go copies it
# onto server.Version). Same ldflags as desktop/build-app.sh.
$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = $goarch

$desktopOut = Join-Path $bundle 'gadak-desktop.exe'
Push-Location (Join-Path $repo 'desktop')
try {
    & go build -tags 'desktop,production' -trimpath `
        -ldflags "-s -w -X main.appVersion=$verStamp" `
        -o $desktopOut .
    if ($LASTEXITCODE -ne 0) {
        exit 1
    }
}
finally {
    Pop-Location
}

# CLI for agent wiring, same stamp as the standalone goreleaser binary.
$cliOut = Join-Path $bundle 'gadak.exe'
Push-Location $repo
try {
    & go build -trimpath `
        -ldflags "-s -w -X main.version=$verStamp" `
        -o $cliOut ./cmd/gadak
    if ($LASTEXITCODE -ne 0) {
        exit 1
    }
}
finally {
    Pop-Location
}

Write-Host "built $bundle ($version, $fileArch)"

if (-not $wantArchive) {
    exit 0
}

# Asset name is Gadak-<ver>-windows-<x64|arm64>.zip on purpose:
# v0.14.0 apps match gadak-desktop-<os>-<arch>.zip exactly (see
# desktop/build-app.sh --dmg). A zip of that pattern would be treated
# as an update. This name also cannot collide with the CLI archive
# gadak_<ver>_windows_<amd64|arm64>.zip from .goreleaser.yaml.
$zipName = 'Gadak-{0}-windows-{1}.zip' -f $verStamp, $fileArch
if ($zipName -like 'gadak-desktop-*-*.zip') {
    [Console]::Error.WriteLine("build-windows: internal error: archive name $zipName matches the updater namespace")
    exit 1
}
$updaterZips = @(Get-ChildItem -LiteralPath $out -File -Filter 'gadak-desktop-*.zip' -ErrorAction SilentlyContinue)
if ($updaterZips.Count -ne 0) {
    [Console]::Error.WriteLine('build-windows: desktop/build contains gadak-desktop-*.zip; notify-only releases must not ship that name')
    foreach ($z in $updaterZips) {
        [Console]::Error.WriteLine($z.FullName)
    }
    exit 1
}
if (-not (Test-Path -LiteralPath $out)) {
    New-Item -ItemType Directory -Path $out | Out-Null
}
$zipPath = Join-Path $out $zipName
if (Test-Path -LiteralPath $zipPath) {
    Remove-Item -LiteralPath $zipPath -Force
}
$compress = Get-Command Compress-Archive -ErrorAction SilentlyContinue
if ($null -ne $compress) {
    Compress-Archive -Path $bundle -DestinationPath $zipPath
} else {
    $zipCmd = Get-Command zip -ErrorAction SilentlyContinue
    if ($null -eq $zipCmd) {
        [Console]::Error.WriteLine('build-windows: missing Compress-Archive and zip')
        exit 69
    }
    Push-Location $out
    try {
        & $zipCmd.Source -r -q $zipName (Split-Path -Leaf $bundle)
        if ($LASTEXITCODE -ne 0) {
            exit 1
        }
    }
    finally {
        Pop-Location
    }
}
if (-not (Test-Path -LiteralPath $zipPath) -or ((Get-Item -LiteralPath $zipPath).Length -le 0)) {
    [Console]::Error.WriteLine("build-windows: archive was not written: $zipPath")
    exit 1
}
Write-Host "archived $zipPath"
