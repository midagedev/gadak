# Omarchy QEMU guest (operator VM)

How to reach the Arch + Hyprland guest used to verify Omarchy install
and menu-extension work. Facts below were checked from this Mac on
2026-08-18. Do not treat an unverified step as done.

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
| Do not install gadak | AUR packaging is a different track. This guest was not given a gadak binary. |

## Do not

- Read or write `~/.gadak/` or `~/.config/gadak-dev/` on this Mac.
- Talk to a real Atlassian site from this path.
- Put the VNC password in source, docs, logs, or process arguments.
- `tailscale serve … off`, or turn on funnel.
- Install software on the Windows host or in WSL.
- Commit PNGs.
