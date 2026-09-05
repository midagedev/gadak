# Build a Windows portable directory from the desktop module.
#
# Prerequisites: `npm run build` at the repo root (the app embeds dist/app),
# Go. No wails3 CLI: plain `go build`, same contract as desktop/build-app.sh
# and desktop/build-linux.sh. Compiling is a cross-compile (CGO_ENABLED=0,
# GOOS=windows) and does not need a Windows host — measured from macOS.
# What still needs Windows is Authenticode signing (GDK-211) and any
# WebView2 runtime/bootstrap installer work; this script does neither.
#
# Usage: desktop/build-windows.ps1 [--arch x64|arm64] [--archive] [--msix]
#
# Exit 64 = usage / unknown argument
#      69 = a required tool is missing (go; makeappx.exe for --msix)
#       1 = dist/app missing or a build step failed
#       0 = portable directory written (and the zip / msix, if asked)
#
# --msix (GDK-1380) additionally packs the same directory into an UNSIGNED
# Gadak-<ver>-windows-<x64|arm64>.msix for Microsoft Store submission, from
# desktop/msix/AppxManifest.xml and the committed desktop/msix/Assets/. It
# needs makeappx.exe (Windows SDK), so it is a Windows-host output — the
# portable directory and zip still cross-compile from anywhere. The Store
# re-signs the package with a Microsoft certificate after certification;
# the file this script writes is not installable as-is and is never a
# GitHub Release asset.
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
#   Missing runtime (from wails v3.0.0-beta.12 source, not launched here):
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
    [Console]::Error.WriteLine('usage: desktop/build-windows.ps1 [--arch x64|arm64] [--archive] [--msix]')
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
$wantMsix = $false
$archArg = $null
for ($i = 0; $i -lt $args.Count; $i++) {
    $arg = [string]$args[$i]
    switch ($arg) {
        { $_ -in @('-h', '--help') } { Usage }
        { $_ -in @('--archive', '-Archive', '-archive') } { $wantArchive = $true }
        { $_ -in @('--msix', '-Msix', '-msix') } { $wantMsix = $true }
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

# -H windowsgui marks the PE as a GUI-subsystem image. Without it Windows
# treats the app as a console program and opens an empty console window
# beside every launch — Start menu, taskbar pin, gadak:// link, the Store
# tile alike. Every zip from 0.16 through 0.20.1 shipped that way, and the
# 0.20.1 Store package was verified on a Windows 11 machine with the black
# window on screen (2026-09-05). The subsystem gate below keeps it from
# coming back. Logs are unaffected: the app writes them under the gadak
# home (applog.Install in desktop/main.go), not to stderr.
$desktopOut = Join-Path $bundle 'gadak-desktop.exe'
Push-Location (Join-Path $repo 'desktop')
try {
    & go build -tags 'desktop,production' -trimpath `
        -ldflags "-s -w -H windowsgui -X main.appVersion=$verStamp" `
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

# PE subsystem gate. The app must be a GUI image (2) — the console
# subsystem (3) is the black-window defect above — and the CLI must stay a
# console image, or `gadak sql` in a terminal would print nothing. Read
# from the PE optional header directly (offset e_lfanew → 'PE\0\0', then
# +24 into the optional header, +68 to Subsystem; the field sits at the
# same place for PE32 and PE32+), so the gate needs no SDK tool and runs
# on the cross-compile host too.
function Get-PESubsystem {
    param([string]$Path)
    $fs = [System.IO.File]::OpenRead($Path)
    try {
        $head = New-Object byte[] 4096
        $n = $fs.Read($head, 0, $head.Length)
        if ($n -lt 0x40) { throw "$Path is too short to be a PE image" }
        $peOff = [BitConverter]::ToInt32($head, 0x3c)
        if ($peOff + 24 + 70 -gt $n) { throw "$Path PE header lies beyond the first 4 KiB" }
        $sig = [System.Text.Encoding]::ASCII.GetString($head, $peOff, 2)
        if ($sig -ne 'PE' -or $head[$peOff + 2] -ne 0 -or $head[$peOff + 3] -ne 0) {
            throw "$Path has no PE signature at e_lfanew"
        }
        return [BitConverter]::ToUInt16($head, $peOff + 24 + 68)
    }
    finally {
        $fs.Dispose()
    }
}

$subsystemNames = @{ 2 = 'WINDOWS_GUI'; 3 = 'WINDOWS_CUI' }
$subsystemWant = @{ 'gadak-desktop.exe' = 2; 'gadak.exe' = 3 }
$subsystemBad = @()
foreach ($exe in $subsystemWant.Keys | Sort-Object) {
    $got = Get-PESubsystem (Join-Path $bundle $exe)
    $gotName = if ($subsystemNames.ContainsKey([int]$got)) { $subsystemNames[[int]$got] } else { 'unknown' }
    Write-Host ("  {0}: subsystem={1} ({2})" -f $exe, $got, $gotName)
    if ([int]$got -ne $subsystemWant[$exe]) {
        $subsystemBad += ("{0} is subsystem {1} ({2}), want {3} ({4})" -f $exe, $got, $gotName, $subsystemWant[$exe], $subsystemNames[$subsystemWant[$exe]])
    }
}
if ($subsystemBad.Count -gt 0) {
    foreach ($line in $subsystemBad) {
        [Console]::Error.WriteLine("build-windows: PE subsystem gate: $line")
    }
    [Console]::Error.WriteLine('build-windows: a console-subsystem gadak-desktop.exe opens an empty console window beside the app; keep -H windowsgui in its ldflags')
    exit 1
}

Write-Host "built $bundle ($version, $fileArch)"

if ($wantMsix) {
    # Store package version: four numeric parts, the fourth reserved (0) and
    # the first non-zero — Partner Center refuses a 0.x major. gadak is 0.x,
    # so the manifest carries (major+1).minor.patch.0: 0.20.0 → 1.20.0.0,
    # and a future 1.0.0 → 2.0.0.0 stays monotonic, which matters because
    # the Store rejects any upload lower than one it has accepted. A
    # git-describe suffix (0.20.0-33-gabc) is dropped. A stamp with no
    # semver at all — 0.0.0-dev, or the bare hash a shallow CI checkout
    # without tags produces — yields 1.0.0.0 so the pack can still be built
    # and install-tested; the Store never sees one of those, the release
    # job checks out with tags.
    # ponytail: the +1 is a one-way door once the first package is accepted;
    # never "fix" it back to the semver major.
    function Get-MsixVersion {
        param([string]$Stamp)
        if ($Stamp -notmatch '^(\d+)\.(\d+)\.(\d+)') {
            [Console]::Error.WriteLine("build-windows: no semver in stamp '$Stamp'; manifest version 1.0.0.0 (untagged build, not for the Store)")
            return '1.0.0.0'
        }
        return ('{0}.{1}.{2}.0' -f ([int]$Matches[1] + 1), [int]$Matches[2], [int]$Matches[3])
    }

    # makeappx.exe ships in the Windows SDK, not on PATH; take the newest kit.
    $makeappx = Get-Command makeappx.exe -ErrorAction SilentlyContinue
    if ($null -eq $makeappx) {
        $kits = ${env:ProgramFiles(x86)}
        if ([string]::IsNullOrEmpty($kits)) { $kits = $env:ProgramFiles }
        $candidates = @()
        if (-not [string]::IsNullOrEmpty($kits)) {
            $candidates = @(Get-ChildItem -Path (Join-Path $kits 'Windows Kits\10\bin') -Directory -Filter '10.*' -ErrorAction SilentlyContinue |
                Sort-Object { [version]$_.Name } -Descending |
                ForEach-Object { Join-Path $_.FullName 'x64\makeappx.exe' } |
                Where-Object { Test-Path -LiteralPath $_ })
        }
        if ($candidates.Count -eq 0) {
            [Console]::Error.WriteLine('build-windows: missing makeappx.exe (Windows SDK) — --msix needs a Windows host')
            exit 69
        }
        $makeappx = Get-Command $candidates[0]
    }

    $msixDir = Join-Path $here 'msix'
    $manifestSrc = Join-Path $msixDir 'AppxManifest.xml'
    $assetsSrc = Join-Path $msixDir 'Assets'
    foreach ($p in @($manifestSrc, $assetsSrc)) {
        if (-not (Test-Path -LiteralPath $p)) {
            [Console]::Error.WriteLine("build-windows: missing $p")
            exit 1
        }
    }

    # Stage = the portable directory + Assets + the substituted manifest.
    # makeappx wants the manifest named exactly AppxManifest.xml at the root.
    $stage = Join-Path $out ("msix-stage-{0}" -f $fileArch)
    if (Test-Path -LiteralPath $stage) {
        Remove-Item -LiteralPath $stage -Recurse -Force
    }
    New-Item -ItemType Directory -Path $stage | Out-Null
    Copy-Item -Path (Join-Path $bundle '*') -Destination $stage -Recurse
    Copy-Item -Path $assetsSrc -Destination (Join-Path $stage 'Assets') -Recurse
    Remove-Item -LiteralPath (Join-Path $stage 'Assets\SOURCE.sha256') -Force -ErrorAction SilentlyContinue
    $msixVersion = Get-MsixVersion -Stamp $verStamp
    $manifest = Get-Content -LiteralPath $manifestSrc -Raw
    $manifest = $manifest.Replace('@VERSION@', $msixVersion).Replace('@ARCH@', $fileArch)
    if ($manifest -match '@[A-Z]+@') {
        [Console]::Error.WriteLine("build-windows: unsubstituted placeholder in AppxManifest.xml: $($Matches[0])")
        exit 1
    }
    # UTF-8 without BOM: makeappx reads the declaration, not a BOM.
    [System.IO.File]::WriteAllText((Join-Path $stage 'AppxManifest.xml'), $manifest, (New-Object System.Text.UTF8Encoding $false))

    $msixPath = Join-Path $out ('Gadak-{0}-windows-{1}.msix' -f $verStamp, $fileArch)
    if (Test-Path -LiteralPath $msixPath) {
        Remove-Item -LiteralPath $msixPath -Force
    }
    & $makeappx.Source pack /d $stage /p $msixPath /o
    if ($LASTEXITCODE -ne 0) {
        [Console]::Error.WriteLine("build-windows: makeappx pack exited $LASTEXITCODE")
        exit 1
    }
    if (-not (Test-Path -LiteralPath $msixPath) -or ((Get-Item -LiteralPath $msixPath).Length -le 0)) {
        [Console]::Error.WriteLine("build-windows: msix was not written: $msixPath")
        exit 1
    }
    Write-Host "packed $msixPath (manifest version $msixVersion, unsigned — Store re-signs)"
}

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
