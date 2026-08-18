# gadak Windows startup smoke (GDK-244) — manual / real-machine gate.
#
# Launch gadak-desktop.exe in an *interactive* desktop session, measure the
# window, capture the screen, assert eleven conditions, write JSON.
#
# THIS IS NOT A CI GATE. GitHub's windows-latest runner has no interactive
# desktop session; CopyFromScreen there is a black frame and the harness
# would fail for the wrong reason. An earlier round had to bridge a real
# host with `schtasks /create /it` + `/run` because an ssh session is
# session 0 and cannot show this window (SAC also blocked the binary
# there — see the run's smoke.json ci_events). Do not add this script to
# .github/workflows as a required job.
#
# What it asserts (eleven; a false value is exit 1):
#   chrome_probe_native, app_started, window_appeared,
#   style_ws_caption, style_ws_thickframe, style_ws_sysmenu,
#   capture_fullscreen_png, capture_window_png, capture_not_black,
#   closed_clean, dotgadak_untouched
# It records msedgewebview2 child count and TCP listeners; it does not
# fail the run on either. A missing WebView2 runtime is being fixed in a
# parallel round — do not assert the old silent-exit behaviour here.
#
# Two facts that are easy to lose:
#   1. The app has no TCP listener by design (desktop/main.go: the Wails
#      asset server calls server.Handler in-process). "No port open" is a
#      pass, not a failure. Fetches against anything found are kept so a
#      future listener would be exercised.
#   2. GADAK_HOME is a temporary directory. This script refuses to point
#      it at the user's real ~/.gadak (never %USERPROFILE%\.gadak).
#
# Encoding: this file MUST keep a UTF-8 BOM. Windows PowerShell 5.1 reads
# a BOM-less file as the system ANSI code page (949 on a Korean host) and
# then the script is garbage.
#
# Designed to be scheduled via: schtasks /create /it + /run.
#
# Exit codes: 0 all assertions passed
#             1 assertion failure (incl. "app never started / never showed a window")
#             64 usage (bad parameter values, or GadakHome is ~/.gadak)
#             69 required tool or input missing (exe, System.Drawing)
#

param(
    [string]$BundleDir  = (Join-Path $env:TEMP 'gadak-winsmoke\bundle'),
    [string]$OutDir     = (Join-Path $env:TEMP 'gadak-winsmoke\out'),
    [string]$GadakHome  = (Join-Path $env:TEMP 'gadak-winsmoke\home'),
    [int]$WindowWaitSeconds = 60
)

$ErrorActionPreference = 'Continue'
$script:Assertions = [ordered]@{}
$script:Smoke = [ordered]@{}
$script:LogPath = $null

function Write-Log([string]$line) {
    if ($script:LogPath) {
        Add-Content -Path $script:LogPath -Value ("{0} {1}" -f (Get-Date).ToString('u'), $line) -Encoding UTF8
    }
    Write-Output $line
}

function Save-SmokeJson() {
    $script:Smoke['assertions'] = $script:Assertions
    $script:Smoke['finished_at'] = (Get-Date).ToString('u')
    $tmp = Join-Path $OutDir 'smoke.json.tmp'
    $script:Smoke | ConvertTo-Json -Depth 7 | Out-File -FilePath $tmp -Encoding utf8
    Move-Item -Path $tmp -Destination (Join-Path $OutDir 'smoke.json') -Force
}

# --- usage / tool checks -----------------------------------------------------
if ([string]::IsNullOrWhiteSpace($BundleDir) -or [string]::IsNullOrWhiteSpace($OutDir) -or
    [string]::IsNullOrWhiteSpace($GadakHome) -or $WindowWaitSeconds -le 0) {
    Write-Output 'usage: winsmoke.ps1 [-BundleDir <dir>] [-OutDir <dir>] [-GadakHome <dir>] [-WindowWaitSeconds <n>]'
    exit 64
}
# Isolation: never the user's real profile. GADAK_HOME must be a temp dir.
$realGadak = [System.IO.Path]::GetFullPath((Join-Path $env:USERPROFILE '.gadak'))
try {
    $homeFull = [System.IO.Path]::GetFullPath($GadakHome)
} catch {
    Write-Output ("usage: GadakHome is not a usable path: {0}" -f $GadakHome)
    exit 64
}
$sep = [IO.Path]::DirectorySeparatorChar
if ($homeFull -eq $realGadak -or $homeFull.StartsWith($realGadak + $sep, [System.StringComparison]::OrdinalIgnoreCase)) {
    Write-Output ("REFUSING: GadakHome must not be the real ~/.gadak ({0})" -f $realGadak)
    exit 64
}

$DesktopExe = Join-Path $BundleDir 'gadak-desktop.exe'
$CliExe     = Join-Path $BundleDir 'gadak.exe'
foreach ($f in @($DesktopExe, $CliExe)) {
    if (-not (Test-Path $f)) { Write-Output "MISSING INPUT: $f"; exit 69 }
}
try {
    Add-Type -AssemblyName System.Drawing
    Add-Type -AssemblyName System.Windows.Forms
} catch {
    Write-Output ("MISSING TOOL: System.Drawing/Forms: {0}" -f $_.Exception.Message)
    exit 69
}

if (-not (Test-Path $OutDir))  { New-Item -ItemType Directory -Path $OutDir  -Force | Out-Null }
if (-not (Test-Path $GadakHome)) { New-Item -ItemType Directory -Path $GadakHome -Force | Out-Null }
$script:LogPath = Join-Path $OutDir 'smoke.log'
if (Test-Path $script:LogPath) { Remove-Item $script:LogPath -Force }
if (Test-Path (Join-Path $OutDir 'done.flag')) { Remove-Item (Join-Path $OutDir 'done.flag') -Force }

try { Start-Transcript -Path (Join-Path $OutDir 'transcript.log') -Force | Out-Null } catch {}

Add-Type -TypeDefinition @"
using System;
using System.Runtime.InteropServices;
public struct RECT { public int Left, Top, Right, Bottom; }
public struct PT   { public int X, Y; }
public class Win32Smoke {
    [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
    [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
    [DllImport("user32.dll")] public static extern bool GetClientRect(IntPtr h, out RECT r);
    [DllImport("user32.dll")] public static extern bool ClientToScreen(IntPtr h, ref PT p);
    [DllImport("user32.dll")] public static extern int  GetWindowLong(IntPtr h, int i);
    [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
}
"@

$exitCode = 0
$proc = $null
$hwnd = [IntPtr]::Zero

try {
    Write-Log ("winsmoke start pid={0} user={1} session={2}" -f $PID, $env:USERNAME, (Get-Process -Id $PID).SessionId)
    $script:Smoke['started_at']   = (Get-Date).ToString('u')
    $script:Smoke['computer']     = $env:COMPUTERNAME
    $script:Smoke['os']           = [Environment]::OSVersion.VersionString
    $script:Smoke['psversion']    = $PSVersionTable.PSVersion.ToString()
    $script:Smoke['bundle_dir']   = $BundleDir
    $script:Smoke['gadak_home']   = $GadakHome
    $script:Smoke['desktop_exe']  = $DesktopExe
    $script:Smoke['desktop_exe_bytes'] = (Get-Item $DesktopExe).Length
    $wv2 = Get-ItemProperty -Path 'HKLM:\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}' -ErrorAction SilentlyContinue
    $script:Smoke['webview2_pv']  = $wv2.pv
    $sac = Get-ItemProperty -Path 'HKLM:\SYSTEM\CurrentControlSet\Control\CI\Policy' -ErrorAction SilentlyContinue
    $script:Smoke['sac_state']    = $sac.VerifiedAndReputablePolicyState   # 0=off 1=enforce 2=eval
    $script:Smoke['dotgadak_before'] = Test-Path $realGadak
    if ($script:Smoke['dotgadak_before']) {
        $script:Smoke['dotgadak_before_snap'] = @(Get-ChildItem $realGadak -Recurse -File -ErrorAction SilentlyContinue |
            ForEach-Object { "{0}|{1}" -f $_.FullName.Substring($realGadak.Length), $_.Length } |
            Sort-Object)
    } else {
        $script:Smoke['dotgadak_before_snap'] = @()
    }

    # existing instances must not confuse the run
    $pre = Get-Process -Name 'gadak-desktop' -ErrorAction SilentlyContinue
    $script:Smoke['preexisting_gadak_desktop'] = @($pre).Count

    # --- chrome probe (same process start path as the app itself) ------------
    Write-Log 'stage: chrome-probe'
    $env:GADAK_HOME = $GadakHome
    $env:GADAK_PROFILE = $null
    $probeOut = ''
    $probeRc = $null
    try {
        $probeOut = (& $DesktopExe '--print-window-chrome' 2>&1) -join "`n"
        $probeRc = $LASTEXITCODE
    } catch {
        $probeOut = ("EXCEPTION: {0}" -f $_.Exception.Message)
        if ($_.Exception.InnerException) { $probeOut += (" | inner: {0}" -f $_.Exception.InnerException.Message) }
    }
    $script:Smoke['chrome_probe'] = @{ stdout = $probeOut; exitcode = $probeRc }
    $script:Assertions['chrome_probe_native'] = ($probeOut -match 'window_chrome=native')
    Write-Log ("chrome probe: rc={0} out=<{1}>" -f $probeRc, $probeOut)

    # --- CLI version probe ----------------------------------------------------
    $cliOut = (& $CliExe version 2>&1) -join "`n"
    $script:Smoke['cli_version'] = @{ stdout = $cliOut; exitcode = $LASTEXITCODE }

    # --- launch the app -------------------------------------------------------
    Write-Log 'stage: launch'
    $si = New-Object System.Diagnostics.ProcessStartInfo
    $si.FileName = $DesktopExe
    $si.WorkingDirectory = $BundleDir
    $si.UseShellExecute = $false
    $si.CreateNoWindow = $true
    $si.EnvironmentVariables['GADAK_HOME'] = $GadakHome
    if ($si.EnvironmentVariables.ContainsKey('GADAK_PROFILE')) { $si.EnvironmentVariables.Remove('GADAK_PROFILE') | Out-Null }
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    try {
        $proc = [System.Diagnostics.Process]::Start($si)
        Write-Log ("app started pid={0}" -f $proc.Id)
        $script:Assertions['app_started'] = $true
    } catch {
        $msg = $_.Exception.Message
        $inner = ''
        $native = $null
        if ($_.Exception.InnerException) {
            $inner = $_.Exception.InnerException.Message
            if ($_.Exception.InnerException.NativeErrorCode) { $native = $_.Exception.InnerException.NativeErrorCode }
        }
        Write-Log ("app START FAILED: {0} | inner: {1} | native: {2}" -f $msg, $inner, $native)
        $script:Smoke['launch_failure'] = @{ message = $msg; inner = $inner; native_error = $native }
        $script:Assertions['app_started'] = $false
        $exitCode = 1
    }

    # --- wait for the window --------------------------------------------------
    if ($proc) {
        Write-Log 'stage: wait-window'
        $windowMs = $null
        while ($sw.ElapsedMilliseconds -lt ($WindowWaitSeconds * 1000)) {
            Start-Sleep -Milliseconds 500
            $proc.Refresh()
            if ($proc.HasExited) { break }
            if ($proc.MainWindowHandle -ne [IntPtr]::Zero) { $windowMs = $sw.ElapsedMilliseconds; break }
        }
        $proc.Refresh()
        if ($proc.HasExited) {
            Write-Log ("app EXITED early code={0} after {1}ms" -f $proc.ExitCode, $sw.ElapsedMilliseconds)
            $script:Smoke['early_exit'] = @{ exitcode = $proc.ExitCode; after_ms = $sw.ElapsedMilliseconds }
            $script:Assertions['window_appeared'] = $false
            $exitCode = 1
        } elseif ($null -ne $windowMs) {
            $hwnd = $proc.MainWindowHandle
            Write-Log ("window appeared after {0}ms hwnd=0x{1:X}" -f $windowMs, [int64]$hwnd)
            $script:Smoke['window_appear_ms'] = $windowMs
            $script:Assertions['window_appeared'] = $true
        } else {
            Write-Log ("window DID NOT appear within {0}s (process alive)" -f $WindowWaitSeconds)
            $script:Assertions['window_appeared'] = $false
            $exitCode = 1
        }
        $script:Smoke['process_alive_at_measure'] = -not $proc.HasExited
    }

    # --- window metrics -------------------------------------------------------
    if ($hwnd -ne [IntPtr]::Zero) {
        Write-Log 'stage: window-metrics'
        $r = New-Object RECT
        [void][Win32Smoke]::GetWindowRect($hwnd, [ref]$r)
        $c = New-Object RECT
        [void][Win32Smoke]::GetClientRect($hwnd, [ref]$c)
        $pt = New-Object PT
        $pt.X = 0; $pt.Y = 0
        [void][Win32Smoke]::ClientToScreen($hwnd, [ref]$pt)
        $style   = [Win32Smoke]::GetWindowLong($hwnd, -16)
        $exstyle = [Win32Smoke]::GetWindowLong($hwnd, -20)
        $script:Smoke['window'] = [ordered]@{
            title          = $proc.MainWindowTitle
            hwnd           = ('0x{0:X}' -f [int64]$hwnd)
            rect           = "L=$($r.Left) T=$($r.Top) R=$($r.Right) B=$($r.Bottom) W=$($r.Right-$r.Left) H=$($r.Bottom-$r.Top)"
            client         = "origin=$($pt.X),$($pt.Y) W=$($c.Right) H=$($c.Bottom)"
            style_hex      = ('0x{0:X8}' -f $style)
            exstyle_hex    = ('0x{0:X8}' -f $exstyle)
            ws_caption     = [bool]($style -band 0x00C00000)
            ws_thickframe  = [bool]($style -band 0x00040000)
            ws_sysmenu     = [bool]($style -band 0x00080000)
        }
        Write-Log ("window: {0}" -f ($script:Smoke['window'] | ConvertTo-Json -Compress))
        # contract: Windows chrome is 'native' -> standard caption bits present
        $script:Assertions['style_ws_caption']    = [bool]($style -band 0x00C00000)
        $script:Assertions['style_ws_thickframe'] = [bool]($style -band 0x00040000)
        $script:Assertions['style_ws_sysmenu']    = [bool]($style -band 0x00080000)
        if (-not ($script:Assertions['style_ws_caption'] -and $script:Assertions['style_ws_thickframe'] -and $script:Assertions['style_ws_sysmenu'])) { $exitCode = 1 }
    }

    # --- webview children + port census + (any) loopback fetch ----------------
    $appPid = 0
    if ($proc) { $appPid = $proc.Id }
    $desc = @()
    # OrderedDictionary in Windows PowerShell 5.1 has Contains, not ContainsKey.
    if ($appPid -gt 0 -and -not $script:Smoke.Contains('early_exit')) {
        Write-Log 'stage: descendants'
        $all = Get-CimInstance Win32_Process | Select-Object ProcessId, ParentProcessId, Name
        $children = @{}
        foreach ($p in $all) {
            if (-not $children.ContainsKey([uint32]$p.ParentProcessId)) { $children[[uint32]$p.ParentProcessId] = @() }
            $children[[uint32]$p.ParentProcessId] += $p
        }
        $frontier = @($appPid)
        while ($frontier.Count -gt 0) {
            $next = @()
            foreach ($f in $frontier) {
                if ($children.ContainsKey([uint32]$f)) {
                    foreach ($ch in $children[[uint32]$f]) {
                        if ($desc -notcontains $ch.ProcessId) { $desc += $ch.ProcessId; $next += $ch.ProcessId }
                    }
                }
            }
            $frontier = $next
        }
        $wvKids = @($all | Where-Object { $desc -contains $_.ProcessId -and $_.Name -eq 'msedgewebview2.exe' })
        $script:Smoke['msedgewebview2_children'] = $wvKids.Count
        Write-Log ("descendants: {0} total, {1} msedgewebview2" -f $desc.Count, $wvKids.Count)
    }

    Write-Log 'stage: port-census'
    $listeners = @()
    if ($desc.Count -gt 0 -or $appPid -gt 0) {
        $watch = @($appPid) + @($desc)
        $listeners = @(Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue | Where-Object { $watch -contains $_.OwningProcess })
    }
    $script:Smoke['tcp_listeners'] = @($listeners | ForEach-Object {
        "port=$($_.LocalPort) addr=$($_.LocalAddress) pid=$($_.OwningProcess)"
    })
    Write-Log ("listeners: {0}" -f ($script:Smoke['tcp_listeners'] -join '; '))
    $fetches = @()
    foreach ($l in ($listeners | Where-Object { $_.LocalAddress -in @('127.0.0.1','0.0.0.0','::1','::') } | Select-Object -First 5)) {
        foreach ($path in @('/config.json', '/')) {
            $rec = $null
            try {
                $resp = Invoke-WebRequest -Uri ("http://127.0.0.1:{0}{1}" -f $l.LocalPort, $path) -UseBasicParsing -TimeoutSec 3
                $body = $resp.Content
                $rec = @{ url = "/:{0}{1}" -f $l.LocalPort, $path; status = [int]$resp.StatusCode; has_windowChrome = ($body -match 'windowChrome') }
            } catch {
                $rec = @{ url = "/:{0}{1}" -f $l.LocalPort, $path; error = $_.Exception.Message }
            }
            $fetches += $rec
        }
    }
    $script:Smoke['loopback_fetch'] = @($fetches)
    $script:Smoke['loopback_note'] = 'desktop/main.go: no TCP listener by design (wails v3 serves the webview in-process). No port open is a pass. Census records the measured absence.'

    # --- captures -------------------------------------------------------------
    if ($hwnd -ne [IntPtr]::Zero) {
        Write-Log 'stage: capture'
        [void][Win32Smoke]::SetProcessDPIAware()
        [void][Win32Smoke]::SetForegroundWindow($hwnd)
        Start-Sleep -Milliseconds 1200
        $bounds = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
        $script:Smoke['screen_bounds'] = "$($bounds.Width)x$($bounds.Height)"

        function Get-BmpStats($bmp) {
            $min = 255; $max = 0; [long]$sum = 0; [long]$n = 0
            $sx = [Math]::Max(1, [int]($bmp.Width / 64)); $sy = [Math]::Max(1, [int]($bmp.Height / 64))
            for ($x = 0; $x -lt $bmp.Width; $x += $sx) {
                for ($y = 0; $y -lt $bmp.Height; $y += $sy) {
                    $px = $bmp.GetPixel($x, $y)
                    $l = [int](($px.R + $px.G + $px.B) / 3)
                    if ($l -lt $min) { $min = $l }; if ($l -gt $max) { $max = $l }
                    $sum += $l; $n++
                }
            }
            return @{ min = $min; max = $max; mean = [Math]::Round($sum / [double]$n, 2); samples = $n }
        }

        $bmpFull = New-Object System.Drawing.Bitmap $bounds.Width, $bounds.Height
        $g = [System.Drawing.Graphics]::FromImage($bmpFull)
        $g.CopyFromScreen($bounds.Location, [System.Drawing.Point]::Empty, $bounds.Size)
        $g.Dispose()
        $bmpFull.Save((Join-Path $OutDir 'fullscreen.png'), [System.Drawing.Imaging.ImageFormat]::Png)
        $script:Smoke['fullscreen_stats'] = Get-BmpStats $bmpFull
        $bmpFull.Dispose()

        $r2 = New-Object RECT
        [void][Win32Smoke]::GetWindowRect($hwnd, [ref]$r2)
        $wx = [Math]::Max(0, $r2.Left); $wy = [Math]::Max(0, $r2.Top)
        $ww = [Math]::Min($r2.Right, $bounds.Width) - $wx
        $wh = [Math]::Min($r2.Bottom, $bounds.Height) - $wy
        if ($ww -gt 0 -and $wh -gt 0) {
            $bmpWin = New-Object System.Drawing.Bitmap $ww, $wh
            $g2 = [System.Drawing.Graphics]::FromImage($bmpWin)
            $g2.CopyFromScreen($wx, $wy, 0, 0, (New-Object System.Drawing.Size $ww, $wh))
            $g2.Dispose()
            $bmpWin.Save((Join-Path $OutDir 'window.png'), [System.Drawing.Imaging.ImageFormat]::Png)
            $script:Smoke['window_capture'] = "origin=$wx,$wy size=${ww}x${wh}"
            $script:Smoke['window_stats'] = Get-BmpStats $bmpWin
            $bmpWin.Dispose()
        }
        foreach ($f in @('fullscreen.png', 'window.png')) {
            $p = Join-Path $OutDir $f
            $script:Assertions[("capture_" + $f.Replace('.', '_'))] = ((Test-Path $p) -and ((Get-Item $p).Length -gt 0))
        }
        $notBlack = ($script:Smoke['fullscreen_stats'].max -gt 0)
        $script:Assertions['capture_not_black'] = [bool]$notBlack
        if (-not $notBlack) { $exitCode = 1 }
        Write-Log ("captures: fullscreen stats={0} window stats={1}" -f ($script:Smoke['fullscreen_stats'] | ConvertTo-Json -Compress), ($script:Smoke['window_stats'] | ConvertTo-Json -Compress))
    } else {
        Write-Log 'stage: capture SKIPPED (no window handle; a desktop photo without the app has no evidentiary value)'
        $script:Assertions['capture_fullscreen_png'] = $false
        $script:Assertions['capture_window_png'] = $false
    }

    # --- shutdown -------------------------------------------------------------
    if ($proc -and -not $proc.HasExited) {
        Write-Log 'stage: shutdown'
        $closed = $proc.CloseMainWindow()
        $proc.WaitForExit(10000) | Out-Null
        $proc.Refresh()
        $neededKill = $false
        if (-not $proc.HasExited) {
            $neededKill = $true
            $proc.Kill()
            $proc.WaitForExit(5000) | Out-Null
        }
        $script:Smoke['shutdown'] = @{ close_main_window = $closed; kill_needed = $neededKill; exitcode = $proc.ExitCode }
        Write-Log ("shutdown: close={0} kill={1} exit={2}" -f $closed, $neededKill, $proc.ExitCode)
        $script:Assertions['closed_clean'] = (-not $neededKill)
        Start-Sleep -Seconds 4
        # orphaned descendants (ancestry-scoped, never by name)
        foreach ($d in $desc) {
            $dp = Get-Process -Id $d -ErrorAction SilentlyContinue
            if ($dp) { Stop-Process -Id $d -Force -ErrorAction SilentlyContinue; Write-Log ("orphan cleanup: killed pid {0}" -f $d) }
        }
    }

    # --- footprint ------------------------------------------------------------
    Write-Log 'stage: footprint'
    $script:Smoke['gadak_home_tree'] = @(Get-ChildItem $GadakHome -Recurse -File -ErrorAction SilentlyContinue | ForEach-Object { "{0} ({1} bytes)" -f $_.FullName.Replace($GadakHome, '<home>'), $_.Length })
    $script:Smoke['dotgadak_after'] = Test-Path $realGadak
    function Get-DotGadakSnapshot([string]$path) {
        if (-not (Test-Path $path)) { return @() }
        return @(Get-ChildItem $path -Recurse -File -ErrorAction SilentlyContinue |
            ForEach-Object { "{0}|{1}" -f $_.FullName.Substring($path.Length), $_.Length } |
            Sort-Object)
    }
    if (-not $script:Smoke['dotgadak_before']) {
        $script:Assertions['dotgadak_untouched'] = (-not $script:Smoke['dotgadak_after'])
    } else {
        $afterSnap = Get-DotGadakSnapshot $realGadak
        $beforeSnap = $script:Smoke['dotgadak_before_snap']
        $script:Assertions['dotgadak_untouched'] = $null -eq (Compare-Object -ReferenceObject @($beforeSnap) -DifferenceObject @($afterSnap))
    }
    $wvdir = Join-Path $env:APPDATA 'gadak-desktop.exe'
    if (Test-Path $wvdir) {
        $files = Get-ChildItem $wvdir -Recurse -File -ErrorAction SilentlyContinue
        $script:Smoke['webview2_userdata'] = @{ path = $wvdir; files = $files.Count; bytes = ($files | Measure-Object Length -Sum).Sum }
    } else { $script:Smoke['webview2_userdata'] = 'absent' }

    # --- code-integrity events of this run (SAC evidence) ---------------------
    try {
        $since = (Get-Date).AddMinutes(-5)
        $ci = Get-WinEvent -LogName 'Microsoft-Windows-CodeIntegrity/Operational' -MaxEvents 60 -ErrorAction Stop |
              Where-Object { $_.TimeCreated -ge $since -and $_.Message -match 'gadak' }
        $script:Smoke['ci_events'] = @($ci | ForEach-Object { "id=$($_.Id) time=$($_.TimeCreated.ToString('u')) :: $($_.Message.Substring(0, [Math]::Min(300, $_.Message.Length)))" })
        Write-Log ("codeintegrity gadak events in window: {0}" -f @($ci).Count)
    } catch { $script:Smoke['ci_events'] = "unavailable: $($_.Exception.Message)" }

    # --- verdict --------------------------------------------------------------
    $failed = @($script:Assertions.GetEnumerator() | Where-Object { -not $_.Value })
    if ($failed.Count -gt 0) { $exitCode = 1 }
    Write-Log ("verdict: {0}" -f (($script:Assertions.GetEnumerator() | ForEach-Object { "{0}={1}" -f $_.Key, $_.Value }) -join ' '))
} catch {
    Write-Log ("UNEXPECTED ERROR: {0}" -f $_.Exception.ToString())
    $exitCode = 1
} finally {
    Save-SmokeJson
    try { Stop-Transcript | Out-Null } catch {}
    Set-Content -Path (Join-Path $OutDir 'done.flag') -Value ("exit={0}" -f $exitCode)
    Write-Output ("WINSMOKE EXIT {0}" -f $exitCode)
}
exit $exitCode
