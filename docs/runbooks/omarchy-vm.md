# Omarchy QEMU guest (operator VM)

How to reach the Arch + Hyprland guest used to verify Omarchy install
and menu-extension work. Facts below were checked from this Mac on
2026-08-18; the payload route and bar-widget states were checked on
2026-08-19. Do not treat an unverified step as done.

This is not a CI path. The guest lives on another machine and is reached
over the tailnet.

**No addresses in this file.** The machine name, the tailnet hostname, the
IP, and the account names are the operator's, and this repository is public —
knowing them buys an outsider nothing here, and publishing them costs
something. The commands below read them from the environment instead:

| Variable | What |
| --- | --- |
| `OMARCHY_VM_HOST` | SSH target for the Windows host (an alias from your own `~/.ssh/config`) |
| `OMARCHY_VNC_HOST`, `OMARCHY_VNC_PORT` | where the tailnet forward answers |
| `VNC_PASSWORD` | the QEMU VNC password — env only, never a flag |
| `VMDIR` | the VM directory on WSL (`~/omarchy-vm` on this setup) |

`scripts/scan-internal.sh` refuses a tailnet hostname or a `100.64/10`
address in any tracked file, so a later edit cannot quietly put them back.

## What is running

| Layer | What it is (checked) |
| --- | --- |
| Windows host | reached by an SSH alias from the operator's own config; default shell is PowerShell |
| WSL2 | `wsl.exe -e bash`, Ubuntu 24.04.1, kernel `6.6.114.1-microsoft-standard-WSL2` |
| QEMU | `qemu-system-x86_64 -name omarchy`, 6 vCPU, 8G, KVM, virtio-vga, OVMF, disk `~/omarchy-vm/disk.qcow2` |
| VNC | QEMU `-vnc 127.0.0.1:1` (RFB 3.8, security type 2). On WSL that is `127.0.0.1:5901` |
| Tailnet forward | `tailscale serve --tcp=5901` on the Windows host → WSL `127.0.0.1:5901`. Funnel is **off**, so the port exists only inside the tailnet. |
| Guest SSH forward | QEMU user-net `hostfwd=tcp:127.0.0.1:2223-:22`. QEMU is listening. Guest `sshd` is `inactive` and `disabled`. |
| Guest OS | `ID=omarchy` `ID_LIKE=arch`. `omarchy-version` printed `4.0.0.r1772.gf32ebbd-1`. `/usr/share/omarchy/version` is `4.0.0.alpha`. `/etc/os-release` `VERSION_ID` matches the `omarchy-version` string. |
| Guest session | Hyprland, hostname `omarchy`. `systemctl --user is-system-running` → `running` (184 units, 0 failed, systemd 261.2-1-arch). Framebuffer 1280×800. RFB desktop name `QEMU (omarchy)`. |

QEMU argv (password field redacted) as seen on 2026-08-18:

```
qemu-system-x86_64 -name omarchy -machine q35,accel=kvm -cpu host -smp 6 -m 8G
  -drive if=pflash,format=raw,readonly=on,file=/usr/share/OVMF/OVMF_CODE_4M.fd
  -drive if=pflash,format=raw,file=$VMDIR/OVMF_VARS.fd
  -drive file=$VMDIR/disk.qcow2,if=virtio,format=qcow2,discard=unmap
  -device virtio-vga -display none
  -object secret,id=vncpass,data=<redacted>
  -vnc 127.0.0.1:1,password-secret=vncpass
  -netdev user,id=net0,hostfwd=tcp:127.0.0.1:2223-:22
  -device virtio-net-pci,netdev=net0 -device qemu-xhci -device usb-tablet
  -pidfile $VMDIR/qemu.pid -daemonize
  -cdrom $VMDIR/omarchy-hangul.iso -boot order=d
```

The guest desktop is a configured Omarchy install (session up since
2026-08-15), even though the QEMU line still has the hangul ISO and
`-boot order=d`. Guest `sshd` is what is down, not the desktop.

## Reach the screen from this Mac

Password is `VNC_PASSWORD` in the environment, never a flag
(`tools/vnc-snap.py` reads `os.environ.get("VNC_PASSWORD")` and the
argparse help says so). Do not put the value in the shell history, in
`ps`, or in this file.

```bash
# RFB banner check (no password):
nc -z "$OMARCHY_VNC_HOST" "$OMARCHY_VNC_PORT"

# One full-frame PNG. Requires Crypto.Cipher.DES and Pillow already on this Mac.
# Do not pip-install.
python3 tools/vnc-snap.py \
  --host "$OMARCHY_VNC_HOST" --port "$OMARCHY_VNC_PORT" \
  --out /tmp/omarchy-state.png
```

`--host`, `--port`, `--out` are required. `--timeout` defaults to 20
seconds. `--do ACTION` is optional and repeatable; it runs before the
capture. Actions implemented: `key:NAME`, `combo:Super+Return`,
`type:TEXT`, `enter`, `sleep:SEC`, `click:X,Y`.

Verified Hyprland bindings (from the on-screen keybindings overlay, then
exercised):

- `Escape` dismissed the overlay
- `Super+Return` opened a terminal (prompt `~ ❯`, window title `Work  1:<user>`)

QEMU drops the shift key if you send a shifted keysym alone. `type:`
holds `Shift_L` and sends the unshifted US key, with a 25 ms gap
between characters.

Write PNGs outside the repo. This tool is not a CI job: it talks to an
operator host.

## Reach a shell on the WSL host (read the VM, do not install packages)

```bash
ssh "$OMARCHY_VM_HOST" "wsl.exe -e bash -s" <<'EOF'
whoami
ps -ef | grep '[q]emu-system-x86_64 -name omarchy'
ss -lntp | grep -E '5901|2223'
cat -n $VMDIR/start.sh
EOF
```

Do not run `tailscale serve --tcp=5901 off`. Do not turn on funnel. Do not
install packages on Windows or WSL.

## Start / stop the guest

Scripts live in `$VMDIR/` on WSL:
`start.sh`, `stop.sh`, `qemu.pid`, `disk.qcow2`,
`omarchy-4.0.0.iso`, `omarchy-hangul.iso`.

`start.sh` (checked):

- Refuses to start a second copy if `qemu.pid` is alive (`already running` + exit 0).
- Default `ISO` is `$HOME/omarchy-vm/omarchy-4.0.0.iso`.
- If that ISO exists **and** `BOOT_DISK` is not `1`, it adds
  `-cdrom "$ISO" -boot order=d`.
- Otherwise it adds `-boot order=c` (disk).
- The running process was **not** the default ISO: it used
  `omarchy-hangul.iso`. An `ISO=...` override or a hand-built argv did that.

`stop.sh` kills the pid in `qemu.pid` (or `pkill`s the named QEMU line)
and removes the pidfile.

To boot the disk without the CD (only when you intend to restart the
guest; this tears down the Hyprland session):

```bash
ssh "$OMARCHY_VM_HOST" "wsl.exe -e bash -s" <<'EOF'
$VMDIR/stop.sh
BOOT_DISK=1 $VMDIR/start.sh
# then: nc -w 3 127.0.0.1 2223;  guest sshd may still be disabled
EOF
```

A 2026-08-18 session left the guest up (Aether open, user session since
2026-08-15) and did **not** run that restart.

## Guest SSH

From WSL, `nc` to `127.0.0.1:2223` succeeds (QEMU is listening).
`ssh -p 2223` to the WSL user, `omarchy@`, and `root@` all timed out
on the banner. Inside the guest:

```
systemctl is-active sshd    # inactive
systemctl is-enabled sshd   # disabled
```

Rebooting with `BOOT_DISK=1` does not by itself start `sshd`. Enable it
on the guest if a later round needs a shell without VNC.

A 2026-08-19 round tried exactly that over VNC. `systemctl is-active sshd`
was still `inactive` and `is-enabled` still `disabled`. `sudo -n true`
printed `sudo: a password is required` (exit 1). The round did not type or
guess a password, so sshd stayed down. The payload used the user-net
fallback below instead. The QEMU forward (`127.0.0.1:2223` on the WSL
side) is still listening; it is still the guest daemon that is off.

## What the guest already has (checked over VNC)

Commands were typed in the Hyprland terminal and read back from the
PNG. `command -v omarchy-plugin` printed nothing; the plugin CLIs are
hyphenated (`omarchy-plugin-list`, …).

| Item | Result |
| --- | --- |
| Version | `omarchy-version` → `4.0.0.r1772.gf32ebbd-1`. Also `/usr/share/omarchy/bin/omarchy`. |
| Web app install | `/usr/share/omarchy/bin/omarchy-webapp-install` exists (159 lines). Header: `omarchy:args=[name url icon-url-or-name [custom-exec] [mime-types]]`. Fewer than 3 args opens a `gum` prompt (`Name>`). It is **not** URL-only. |
| How a web app opens | Desktop `Exec` defaults to `omarchy-launch-webapp $APP_URL`. That 13-line script reads `xdg-settings get default-web-browser`, falls back to `chromium.desktop` unless the default is chrome/brave/edge/opera/vivaldi/helium, and execs `setsid uwsm-app -- <browser-exec> --app="$1"`. This guest's default is `chromium.desktop`. |
| Menu extension | `~/.config/omarchy/extensions/` exists. It contains `omarchy-menu.jsonc` (stock commented schema + examples). IDs are object keys; dotted ids nest (`personal.notes`). Fields documented in that file: `icon`, `label`, `action`, `target`, `provider`, `aliases`, `description`, `when`, `checked`. |
| Plugin CLI | No `omarchy plugin` argv. Present: `omarchy-plugin-clone`, `-enable`, `-disable`, `-list`, `-remove`, `-update`, `-validate`. `omarchy-plugin-list` prints first-party plugins (`omarchy.menu` is one). |
| Plugin manifest | Example `/usr/share/omarchy/shell/plugins/panels/clock/manifest.json`: `"schemaVersion": 1`, plus `id`, `name`, `version`, `kinds`, `entryPoints`. |
| Theme hook | `~/.config/omarchy/hooks/theme-set.d/` exists. On this guest it holds only `show-theme-notification.sample`. |
| `colors.toml` | Per-theme files under `/usr/share/omarchy/themes/<name>/colors.toml` (catppuccin starts `mode = "dark"`). |
| systemd --user | Works (`running`, 0 failed). `gadak install-service` on Linux writes a systemd user unit; this guest can host that path. |
| IME | `fcitx5` is on PATH and running (`/usr/bin/fcitx5 --disable notificationitem`). Locale `en_US.UTF-8`, X11 layout `us`. No hangul indicator was visible on the bar. `fcitx5-hangul` package query was not completed (typed command was eaten). |
| gadak binary | AUR packaging remains a different track (`docs/INSTALL.md`). A 2026-08-19 round put a `linux/amd64` binary built from this tree at `~/.local/bin/gadak` (already on the graphical-session PATH). The 2026-08-18 line "this guest was not given a gadak binary" is therefore stale — keep AUR off this guest; do not `pacman -S gadak`. |
| gadak plugin | `contrib/omarchy/install.sh` copied `io.github.midagedev.gadak` into `~/.config/omarchy/plugins/`, `omarchy-plugin-validate` printed `ok`, `omarchy-plugin-enable` enabled it, and `omarchy-webapp-install` wrote the `gadak` desktop file pointing at `http://127.0.0.1:7777`. |
| default mirror | Seeded from `examples/demo.db` (534 issues) at `~/.gadak/gadak.db`. Do not use `gadak demo` for the widget: that command sets `GADAK_HOME` to a temp dir (`cmd/gadak/demo.go`) the widget never sees. |
| GTK stack | GTK4 installed (`gtk4 1:4.22.4-1`, `libgtk-4.so.1`, `pkg-config --exists gtk4` → 0). **WebKitGTK 6.0 is absent** — only `webkit2gtk-4.1` is installed, a different SONAME; a wails v3 GTK4 binary fails at load time here. The providing package is `extra/webkitgtk-6.0` and installing it needs sudo. Chromium being the default browser implies nothing about WebKitGTK. (checked 2026-08-19) |
| glibc / Go | glibc `2.44` (newer than Ubuntu 24.04 containers — container-built binaries link fine downward). `go` is **not** on PATH. |
| Session env | Wayland (Hyprland), `WAYLAND_DISPLAY=wayland-1`, Xwayland on `:0`. `GDK_BACKEND=wayland,x11,*` is already exported — a GTK4 app needs no extra env. |
| Terminal `ls` | The interactive `ls` is a decorated listing (Permissions/Size/User headers), not coreutils plain `ls` — parse-by-eye accordingly when reading captures. |

## Payload into the guest (checked 2026-08-19)

Guest `sshd` is still down (see above), so there is no `scp` into the guest.
The route that worked, without installing anything on Windows or WSL:

1. On the Mac, in the worktree: `nvm use` (`.nvmrc` owns the Node version),
   `npm ci` if `node_modules` is absent, `npm run build` (go:embed needs
   `dist/app`). Then match the goreleaser `builds:` block
   (`.goreleaser.yaml`):
   `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath`
   `-ldflags "-s -w -X main.version=$(git describe --tags --always)"`
   `-o <scratch>/payload/gadak ./cmd/gadak`.
2. Pack that binary, `examples/demo.db`, and `contrib/omarchy/` as one
   tarball. Confirm the fixture yourself (`SELECT COUNT(*) FROM issues`
   was 534). Build the tarball with `COPYFILE_DISABLE=1` so macOS
   AppleDouble `._*` headers do not travel with it.
3. Mac → Windows host: `scp` to that host fails; `sftp put` into the
   Windows-user home directory works. From WSL that file is
   `/mnt/c/Users/<windows-user>/<name>`.
4. `ssh "$OMARCHY_VM_HOST" "wsl.exe -e bash -lc '…'"` copies it onto the
   Linux filesystem (`/tmp/gadak-payload/`) and extracts it.
5. One-shot `python3 -m http.server 8765 --bind 0.0.0.0` in that
   directory on WSL (Python is already there; this is not installing
   software). Tear it down after the guest has the tarball.
6. Inside the guest, curl the QEMU user-net gateway:
   `u=http://10.0.2.2:8765` then `curl -o p.tgz $u/p.tgz`. A HEAD of
   `$u/` returned `HTTP/1.0 200 OK` from `SimpleHTTP/0.6 Python/3.12.3`.
   The 7 MB tarball transferred at 100%.

A later round that can type a sudo password (or that finds passwordless
sudo) should still prefer enabling guest `sshd` — the QEMU forward is
already there — and then `scp -P 2223` from WSL. Until then, reuse this
user-net path.

### VNC typing (costs a round if ignored)

`tools/vnc-snap.py` `type:` holds `Shift_L` for shifted keys. Long
strings are still flaky; **read the PNG before pressing enter** on
anything that writes. Measured on 2026-08-19:

- Uppercase `I` (`Shift_L` + `i`) was dropped twice. `curl -sI http://…`
  landed as `curl -stp://…`; `curl -sI $u/` landed as `curl -s$u`.
  Use `curl --head` instead of `-sI`.
- `combo:Control+c` is rejected (`unknown key name 'Control'`). The
  keysym table names it `Control_L`. `combo:Control_L+u` kills a
  half-typed line; `combo:Control_L+c` did not always cancel.
- `export PATH=…` in the Hyprland terminal does **not** reach the
  widget. `BarWidget.qml` runs `bash -c` inside Quickshell, which has
  the graphical-session PATH. Copy the binary into `~/.local/bin/`
  (already populated on this guest).
- There is no hover action and no right-click in `vnc-snap.py`. Tooltip
  text cannot be photographed with the current wrapper. Widget refresh
  is the 60 s `Timer` (`BarWidget.qml`), not a key we can send.

### Answering a prompt that wants a secret

`type:TEXT` puts the text in `ps` and in whatever transcript recorded the
command, so it is the wrong action for a password — including the guest's
`sudo` prompt, which is the only way to reach `sudo` here (`sshd` is down and
`sudo -n` fails). Use `typeenv:VAR`, which names an environment variable and
never places the value on the command line:

```bash
GUEST_SUDO_PASSWORD="$(head -1 <a 0600 file outside this repo>)" \
  python3 tools/vnc-snap.py --host … --port … \
    --do 'type:sudo systemctl enable --now sshd' --do enter --do sleep:1 \
    --do 'typeenv:GUEST_SUDO_PASSWORD' --do enter --do sleep:2 \
    --out /tmp/omarchy-sshd.png
```

A wrong or unset variable name is reported by **name**; the value is never
echoed, logged, or included in an error. Keep the secret in a `0600` file
outside the repository and read it into the environment on the command that
needs it — never into a tracked file, and never as a flag.

## Bar widget states (checked 2026-08-19)

`contrib/omarchy/gadak/BarWidget.qml` names five `viewState` values:
`loading | ok | no-gadak | not-synced | sql-err`. The four a stranger
meets, in order, with the commands that produced them:

1. **`no-gadak`** — `command -v gadak` printed nothing (exit 1).
   `bash omarchy/install.sh` copied the plugin, printed
   `omarchy-plugin-validate: ok`, `Enabled io.github.midagedev.gadak`,
   and `web app: gadak -> http://127.0.0.1:7777`. The bar badge read
   `no gadak` (`BarWidget.qml` `displayText` for that state).
2. **`not-synced`** — `cp gadak ~/.local/bin/` then wait for the 60 s
   poll. Badge read `not synced`. In the same terminal,
   `gadak sql "select 1"` printed
   `no mirror at ~/.gadak/gadak.db — run \`gadak sync\` first`
   (`cmd/gadak/sql.go`), which is the stderr the widget keys on
   (`/no mirror/i` in `applyResult`).
3. **`ok`** — `mkdir -p ~/.gadak && cp demo.db ~/.gadak/gadak.db`.
   Next poll: badge `368·201`. Cross-check in the guest:
   `q=$(cat omarchy/gadak/query.sql)` then `gadak sql --json "$q"`
   printed `{"open":368,"stuck":201}` (plus a stale-mirror warning on
   stderr, which the widget ignores). Same query against
   `examples/demo.db` on the Mac also returned `368|201`.
4. **the click** — `gadak serve --no-sync --no-open > /tmp/gs.log 2>&1 &`
   then `curl --head http://127.0.0.1:7777/` → `HTTP/1.1 200 OK`.
   Left-click the badge (`click:1150,18` on this 1280×800 framebuffer).
   `Quickshell.execDetached(["omarchy-launch-webapp", serveUrl])` opened
   an app window of the embedded UI: sidebar title `gadak`,
   `534 issues`, `368 issues` in the filtered list.

`sql-err` and the first-poll `…` were not staged. `gadak demo` was not
used.

## Do not

- Read or write `~/.gadak/` or `~/.config/gadak-dev/` on this Mac.
- Talk to a real Atlassian site from this path.
- Put the VNC password in source, docs, logs, or process arguments.
- `tailscale serve … off`, or turn on funnel.
- Install software on the Windows host or in WSL.
- Commit PNGs.
