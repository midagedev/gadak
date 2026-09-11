# A resident `gadak serve` on a tailnet host — install to pairing

The shape this runbook produces: one machine on your tailnet (a VPS, a home
box, a NAS) runs `gadak serve` as a user service with the built-in tracker
as its origin, and every other device — laptop, phone, an agent's shell —
pairs to it. That machine's `origin/issuetap.db` becomes the record; each
device keeps only a cache it can delete. The procedure below was walked by
hand when the project's own backlog moved to such a host (GDK-1262) and is
written down so the next host takes minutes, not an afternoon.

Two documents own the facts this one arranges: [`docs/INSTALL.md`](../INSTALL.md)
(the tarball, the service unit, pairing) and [`docs/NETWORK.md`](../NETWORK.md)
(what a tailnet device can and cannot reach). When they disagree with this
page, they win.

## 0. What you need

- A Linux host on your tailnet with `tailscale` already up
  (`tailscale status` shows it). A non-root user is enough; `sudo` is only
  needed for the `tailscale serve` variant in step 4.
- Its tailnet name (`tailscale status --self` prints
  `<host>.<tailnet>.ts.net`) or tailnet IP.
- A device to pair from, also on the tailnet, with `gadak` installed.

## 1. Install the release binary (no root)

Follow the Linux tarball route in `docs/INSTALL.md`: download
`gadak_<version>_linux_amd64.tar.gz` (or `linux_arm64`) and `checksums.txt`
from the release, verify, unpack, and put `gadak` in `~/.local/bin`.
`gadak version` must print the tag you downloaded before you go on.

## 2. Create the workspace on the built-in tracker

```bash
gadak init --local
gadak sync
```

`init --local` is non-interactive and seeds project `STD` and wiki space
`LOC`. The record it creates is `~/.gadak/origin/issuetap.db`; `gadak.db`
beside it is the cache. If you are moving an existing Jira mirror onto this
host instead, do the export on the machine that has that mirror
(`gadak --workspace <new> migrate --from <jira workspace>`), then copy the
new workspace directory here — `gadak migrate --help` lists the flags.

## 3. Make it survive a reboot

```bash
gadak install-service -- --addr <tailnet-ip>:7777 --allow-remote
loginctl enable-linger "$USER"
```

The flags after `--` ride into the systemd user unit and are validated at
install time, so a unit that would crash-loop is refused up front instead.
`enable-linger` keeps the user session — and with it the `systemd --user`
unit — alive when nobody is logged in; without it the serve stops at logout
and does not come back after a reboot.

`--allow-remote` is the sharp edge `docs/NETWORK.md` describes: it opens the
mirror's read and write API to anything that can reach the port. On a
tailnet the boundary that makes it acceptable is the tailnet itself — bind
the tailnet IP, never `0.0.0.0`, and check with `tailscale status` that the
ACLs let only your devices reach this host.

## 4. Alternative: `tailscale serve` (HTTPS, no `--allow-remote`)

If `sudo` is available, keep the serve on loopback and let Tailscale carry
it:

```bash
gadak install-service                      # loopback, default port 7777
sudo tailscale serve --bg 7777
```

The serve then answers at `https://<host>.<tailnet>.ts.net` with a
certificate Tailscale issues, and the mirror API stays closed to anything
that does not carry a pairing token — the host guard rejects DNS hostnames
unless a token authenticates the request (`docs/NETWORK.md`, "Tailscale is
the intended transport"). Prefer this variant when you can.

## 5. Mint a pairing offer for each device

On the host:

```bash
gadak pairing mint --label laptop --endpoint http://<tailnet-ip>:7777
gadak pairing mint --label phone --scope serve --endpoint https://<host>.<tailnet>.ts.net
```

`--endpoint` is required on a headless host: without it `mint` takes the
serve's own address and refuses when that address is one a remote device
cannot use (GDK-1266). The default scope, `origin`, is what a paired
`gadak` on another machine needs; `serve` is the phone companion's scope.
`mint` prints the offer on **stdout** and its guidance on stderr, so a
script captures the offer with `gadak pairing mint … --no-qr 2>/dev/null`.

## 6. Pair the device

On the device, paste the offer through stdin so it never lands in `ps` or
shell history:

```bash
gadak --workspace home init --pairing-code-stdin
gadak --workspace home status
```

`status` prints `paired with "laptop"`. Do not combine
`--pairing-code-stdin` with `--local` or a site token.

## 7. Verify the round trip

```bash
gadak --workspace home create "pairing check" --project STD --type Task
gadak --workspace home issue STD-1
```

The first write on a paired workspace needs `--project` and `--type`
spelled out (the paired client has no site default yet; `create` names the
available type ids in its refusal). The issue should come back from the
host's tracker on the next read.

Two checks that look like failures and are not:

- `curl http://127.0.0.1:7777/` **on the host** returning 200 says nothing
  about remote reach — try from the device.
- Opening the serve's address in a browser from the device gives 403. That is
  the host guard doing its job: the pairing token is the credential, and the
  web UI is not one of the token-gated surfaces.

## Afterwards

- **Back up the record, not the cache**: `gadak backup --to <dir>` copies
  `origin/issuetap.db` safely while serve runs
  ([`backup-restore.md`](backup-restore.md)).
- **Upgrade the host first.** A paired client that is newer than the host's
  serve can ask for API the host does not have yet (the agile endpoints,
  GDK-1691). Replace the binary in `~/.local/bin`, then
  `systemctl --user restart` the unit `install-service` created.
- **Revoke a lost device** with `gadak pairing revoke <label>` on the host;
  `pairing list` shows what is active.
