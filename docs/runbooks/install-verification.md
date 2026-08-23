# Install verification — the shipped bundle, on each OS

`release-audit.md` says to verify the shipped bundle rather than the script that
writes it. This is how. One page per release, three platforms, and an honest
statement of what a headless check cannot see.

The rule this exists for: **a green pack job is not an install.** Everything
here has been wrong at least once while CI was green — a bundle that ran from a
randomized path, a scheme that bound to a dev copy, a CLI on `PATH` a version
behind the app that shipped it.

## Two moments, not one

| When | Artifact | What it proves |
| --- | --- | --- |
| **Before the tag** | the *previous* release's artifacts | the procedure works and the install path is unbroken — this is the pre-tag gate (`release-audit.md` step 4) |
| **After the release publishes, before announcing** | the *new* release's artifacts | the thing users will actually download |

The second one is the real check and it cannot be moved earlier: the artifacts
do not exist until the tag builds them, and signing/notarization happens in CI,
not locally. A locally built bundle is unsigned and un-notarized, so it proves
bundle mechanics and nothing about Gatekeeper.

Do not skip the second moment because the first was green. They test different
things.

## Hosts

Verification needs one machine per OS, and their names are not in this repo —
they are personal machines. The inventory (hostnames, how to reach them, VM
launch commands) lives in the **Confluence `GDK` space**, page *Install
verification hosts*. Everything below refers to them by role:

- **the macOS host** — Apple Silicon, a logged-in GUI session
- **the Windows host** — Windows 11 x64, reachable over ssh (PowerShell)
- **the Linux VM** — an Arch/Omarchy guest on the Windows host under QEMU, with
  ssh forwarded from the host loopback

A contributor with their own three machines can run every step here; nothing
below depends on which machines they are.

## What a delegated round may and may not do

These rounds are safe to delegate (see `~/.claude` operating rules for the
general contract) with three additions, because they touch machines that are not
the repo:

- **No `rm -rf` outside the round's own scratch directory** on any remote host.
  Replacing `/Applications/Gadak.app` or an unpacked test directory is in scope;
  anything else is not.
- **Back up before replacing.** Record the version being replaced and keep a
  copy, so the host goes back to where it started.
- **Never touch `~/.gadak` on a verification host.** These are real installs
  with real mirrors. The install being tested is the app, not the data.

The lead owns: the tag, the release, and every write to the tracker.

---

## macOS — the dmg

```bash
# 1. Fetch the shipped artifact and give it what a browser gives it.
curl -fsSL -o Gadak-<ver>-arm64.dmg \
  https://github.com/midagedev/gadak/releases/download/v<ver>/Gadak-<ver>-arm64.dmg
xattr -w com.apple.quarantine "0083;$(printf %x $(date +%s));Safari;" Gadak-<ver>-arm64.dmg

# 2. Gatekeeper must accept it as notarized, not merely signed.
spctl -a -vvv -t open --context context:primary-signature Gadak-<ver>-arm64.dmg
#   want: accepted / source=Notarized Developer ID / origin=Developer ID Application: …

# 3. Install the way the docs tell a user to: drag in Finder.
hdiutil attach Gadak-<ver>-arm64.dmg -nobrowse
spctl -a -vvv /Volumes/Gadak/Gadak.app
codesign -dv --verbose=4 /Volumes/Gadak/Gadak.app   # want flags=…(runtime), Timestamp, TeamIdentifier
```

**The trap, and why the install method is part of the test.** Finder's drag
*strips* `com.apple.quarantine` from the copy it makes. A shell `cp -R` or
`ditto` does not — and a quarantined app bundle launches under **App
Translocation**, from a read-only randomized path:

```
/private/var/folders/…/AppTranslocation/<uuid>/d/Gadak.app/Contents/MacOS/gadak-desktop
```

Measured 2026-08-23 on 0.16.1. Everything path-dependent breaks there:
`install-cli` writes a path that vanishes on the next launch, and the bundle the
scheme is bound to is not the bundle that runs. So:

- verifying by hand: drag it in Finder, like the docs say
- verifying over ssh (no Finder): `cp -R` **and then**
  `xattr -dr com.apple.quarantine /Applications/Gadak.app`, which is what Finder
  would have done. Say in the report which path was used.
- if `pgrep -fl gadak-desktop` ever shows `AppTranslocation`, the install is not
  the install under test — fix it and start over.

```bash
# 4. The scheme binds to the copy in /Applications, and to nothing else.
LSR=/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister
$LSR -dump | grep -B20 'claimed schemes: *gadak:' | grep -E '^\s*(path|bundle id):'
#   want exactly one bundle, at /Applications/Gadak.app
```

A second claimant is the failure this step exists for — a mounted dmg or a
`desktop/build/Gadak.app` from a local build can hold the binding. Unmount and
unregister the dev copy first (`$LSR -u <path>`), then re-dump.

```bash
# 5. Cold: the app is not running, and the link starts it.
pkill -f 'Gadak.app/Contents/MacOS/gadak-desktop'; sleep 2
pgrep -fl gadak-desktop           # want: nothing
open 'gadak://view?issue=<KEY>'
sleep 10
pgrep -fl gadak-desktop           # want: exactly one, path under /Applications

# 6. Warm: the app is running, and the link does not start a rival.
PID0=$(pgrep -f 'Gadak.app/Contents/MacOS/gadak-desktop' | head -1)
open 'gadak://view?issue=<OTHER-KEY>'
sleep 6
[ "$PID0" = "$(pgrep -f 'Gadak.app/Contents/MacOS/gadak-desktop' | head -1)" ]
test "$(pgrep -f gadak-desktop | wc -l)" -eq 1
```

```bash
# 7. The CLI the bundle ships is the same release as the bundle.
/Applications/Gadak.app/Contents/Resources/bin/gadak --version
gadak --version                   # the one on PATH, if install-cli ever ran
```

A `PATH` copy older than the bundle is a real finding: an app upgrade does not
re-run `install-cli`, so the two disagree until the user notices.

### What this does not verify on macOS, headlessly

**That the window navigated to the issue in the link.** Steps 5 and 6 prove the
process lifecycle, not the destination. Three routes are closed over ssh on a
locked or unattended session, all measured 2026-08-23:

- `screencapture -x` → `could not create image from display`
- System Events / accessibility scripting → error `-1712` / `-609`
- the app's own `log.Printf("deep link → …")` goes to stderr and does **not**
  reach unified logging, so `log show --predicate 'process == "gadak-desktop"'`
  returns nothing

So the destination needs either a human at the screen or an unlocked session
with Screen Recording granted to the ssh client. **Report it as unverified
rather than implying it passed.** (Closing this gap — making the deeplink
destination observable without a screen — is worth its own item.)

---

## Windows — the portable zip

0.16+ ships `Gadak-<ver>-windows-x64.zip` (and `-arm64`): a directory with the
two exes at the root, not an installer. It is **unsigned** — that is a recorded
decision, not an oversight ([GDK-211]), and `docs/INSTALL.md` says so.

Reachable over ssh with PowerShell. Set the code page first or Korean-locale
error text arrives as mojibake:

```powershell
chcp 65001 > $null
```

```powershell
# 1. Fetch and verify what can be verified. The desktop zip is deliberately NOT
#    in checksums.txt (docs/INSTALL.md says so) — only the CLI archives are.
#    So there is no published hash for this artifact. Record that as a gap, do
#    not invent a check.
Invoke-WebRequest -Uri https://github.com/midagedev/gadak/releases/download/v<ver>/Gadak-<ver>-windows-x64.zip -OutFile gadak.zip
Get-FileHash gadak.zip -Algorithm SHA256

# 2. Unpack the way a user does, into a fresh directory.
Expand-Archive gadak.zip -DestinationPath $env:USERPROFILE\gdk-verify\<ver> -Force
Get-ChildItem $env:USERPROFILE\gdk-verify\<ver>

# 3. Authenticode state — expect "NotSigned". If it ever says Valid, the
#    unsigned decision changed and the docs are stale.
Get-AuthenticodeSignature $env:USERPROFILE\gdk-verify\<ver>\gadak-desktop.exe | Format-List Status,SignerCertificate

# 4. Mark-of-the-web: a downloaded zip taints what it unpacks.
Get-Item $env:USERPROFILE\gdk-verify\<ver>\gadak-desktop.exe -Stream Zone.Identifier -ErrorAction SilentlyContinue
```

Then the three things that have actually been broken on Windows:

```powershell
# 5. The app menu. GDK-700: without UseApplicationMenu the window gets an empty
#    HMENU, so File/Edit/Window are missing while paste still works through
#    WebView2 accelerators — which is why nobody noticed.
#    Headless proof is limited; see the boundary note below.

# 6. gadak:// registration. The portable zip has no installer, so the scheme is
#    registered at runtime by the app itself.
Start-Process $env:USERPROFILE\gdk-verify\<ver>\gadak-desktop.exe
Start-Sleep 12
Get-ItemProperty 'HKCU:\Software\Classes\gadak\shell\open\command' -ErrorAction SilentlyContinue
#   want: a command pointing at THIS unpacked copy, not an older path

# 7. Cold and warm, the same shape as macOS.
Get-Process gadak-desktop -ErrorAction SilentlyContinue | Stop-Process
Start-Sleep 2
Start-Process 'gadak://view?issue=<KEY>'
Start-Sleep 12
Get-Process gadak-desktop | Select-Object Id,Path       # want exactly one
$pid0 = (Get-Process gadak-desktop | Select-Object -First 1).Id
Start-Process 'gadak://view?issue=<OTHER-KEY>'
Start-Sleep 6
Get-Process gadak-desktop | Select-Object Id,Path       # want the same single Id
```

```powershell
# 8. The bundled CLI answers, and agrees with the app.
& $env:USERPROFILE\gdk-verify\<ver>\gadak.exe --version
```

### If the exe is blocked, measure the path a user takes

`Start-Process` and `Invoke-Item` are **not** the user's launch path, and on a
host with Smart App Control enforcing they answer a different question. SAC
refuses the `CreateProcess` and the *calling* process gets
`An Application Control policy has blocked this file` as an exception — so a
scripted probe sees no dialog, because the shell that draws the dialog was
never involved. Measured 2026-08-23: a round that only launched
programmatically concluded "no dialog appears" and that went into a Highest
bug and a docs correction before it was caught ([GDK-745]).

The instrument that settles it is the Code Integrity log's own **caller**
field:

```powershell
Get-WinEvent -LogName 'Microsoft-Windows-CodeIntegrity/Operational' -MaxEvents 2000 |
  Where-Object { $_.Id -in 3033,3077,3118 } |
  ForEach-Object { if ($_.Message -match 'process \(([^)]+)\)') { Split-Path $matches[1] -Leaf } } |
  Group-Object | Select-Object Count,Name
```

If `explorer.exe` is absent from that table, the user's path has not been
tested at all, whatever else the round measured. To test it, hand the exe to
Explorer so Explorer performs the shell verb, from a task running in the
**interactive** session (`-LogonType Interactive`, `-UserId <bare-name>`; on a
workgroup machine `$env:USERDOMAIN\$env:USERNAME` fails to map to a SID):

```powershell
Start-Process explorer.exe -ArgumentList "`"$exe`""
```

Read the dialog's words instead of screenshotting it — cheaper, and it gives
text you can paste into a doc. `GetWindowTextW` over top-level windows catches
the SAC notice (`Shell_SystemDialogProxy`); the message box is XAML-hosted, so
its body needs UI Automation (`UIAutomationClient`, walk
`ControlType.Text` descendants). Close what you opened: a modal left on the
host's desktop blocks the next round.

### What this does not verify on Windows, headlessly

The **menu bar** and anything else that is pixels. A window's menu is not
readable from PowerShell; `Get-Process` says the app is up, not what it drew.
Options, in order of preference:

1. a screenshot from the host's own session, judged by a vision round (this is
   the cheap one and it settles GDK-700 either way)
2. UI Automation (`[System.Windows.Automation]`) to enumerate the window's menu
   — heavier, and it needs the session to be interactive
3. a human at the screen

Whichever is used, name it. "The app started" is not "the menu is there".

---

## Linux — the tarball, in the VM

The Omarchy guest runs under QEMU on the Windows host with ssh forwarded from
the host's loopback, so this is fully scriptable — no VNC and no vision round
needed for the install itself. The chain is: your machine → ssh to the Windows
host → WSL → ssh to the forwarded guest port. The launcher scripts and the exact
port are in the Confluence page named above.

Start the guest, then:

```bash
# 1. Fetch and verify. Unlike the desktop bundles, the CLI archives ARE in
#    checksums.txt, so this one has a real integrity check — use it.
curl -fsSLO https://github.com/midagedev/gadak/releases/download/v<ver>/gadak_<ver>_linux_amd64.tar.gz
curl -fsSLO https://github.com/midagedev/gadak/releases/download/v<ver>/checksums.txt
sha256sum -c --ignore-missing checksums.txt      # must PASS, not "no file matched"

# 2. Install the way docs/INSTALL.md says: unpack, put it on PATH.
tar xzf gadak_<ver>_linux_amd64.tar.gz
./gadak --version                                 # want: <ver>, and note whether it prints a leading v

# 3. It runs with no config and says something useful rather than crashing.
./gadak status --json ; echo "rc=$?"
./gadak doctor
```

Then the two Linux-specific surfaces:

```bash
# 4. gadak:// via xdg-mime — the Linux half of the deeplink story.
#    GDK-207/GDK-708 track this; if registration is still unimplemented, the
#    honest result is "not registered", recorded as such.
xdg-mime query default x-scheme-handler/gadak
ls ~/.local/share/applications/ | grep -i gadak

# 5. `gadak serve` binds loopback only, and refuses a non-loopback bind without
#    --allow-remote. This is a SECURITY.md promise, so it is not optional.
./gadak serve --addr 0.0.0.0:7777 ; echo "rc=$? (want non-zero: refused)"
```

### What this does not verify on Linux

The **desktop app**. There is no Linux desktop artifact in the release; the
Linux story is the CLI plus `gadak serve` in a browser (`contrib/omarchy/` is
the webapp recipe). If a Linux desktop build ever ships, this section grows a
GTK4/WebKitGTK half — until then, do not report the CLI check as an app check.

---

## Recording the result

One comment on the release-audit parent issue per platform, each stating:

1. the artifact (file name and the hash you measured)
2. every step above with its actual output — not "ok"
3. the install method used, explicitly (Finder drag vs `cp` + `xattr -dr`)
4. **what was not verified and why** — the boundaries above are not disclaimers,
   they are the part of the report that keeps the next person honest
5. anything the host was left holding, and how to put it back

A finding goes to the tracker the same day it is measured, at Highest if it is a
defect. Friction that is not a defect still gets filed: a verification host is
one of the few places where the install is observed at all.

[GDK-211]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-211
[GDK-700]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-700
[GDK-745]: https://midagedev.github.io/gadak/backlog/#/?ks=GDK-745
