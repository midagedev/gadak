# Agents and the network

gadak's answer to "what does this tool do with my network" is short: **mirror
reads do not use the network, writes go to the origin you configured, and the
only scheduled outbound besides sync is an optional anonymous version
check.** This page walks that answer end to end — what connects where and
when, how the mirror stays fresh without an agent managing it, and how to put
the network to work on purpose when several machines or a whole team share
one workspace.

[`SECURITY.md`](../SECURITY.md) is the enforcement record for the destination
list and the pairing gate, with the `file:line` for each claim. This page is
the operating manual: freshness, and sharing a workspace deliberately.

## The shape of the traffic

Mirror reads are answered from a local SQLite file and open no connection:
`gadak sql`, `gadak search`, `gadak issue`, the MCP tools, the web UI's list
and detail. That is why they are fast, free of rate limits, and work on a
plane. A handful of verbs are *not* mirror reads and say so — `gadak issue
--editmeta` and `gadak fields` ask the origin what is editable, `gadak api`
is a passthrough by definition, and viewing an attachment fetches its bytes
on demand.

The traffic that does exist:

1. **Sync** — pulling your tracker's changes into the mirror.
2. **Writes** — a comment, transition, or edit goes to the origin first,
   then the mirror is re-read. gadak never queues a write in the mirror: if
   the origin is remote and unreachable the write fails, and if the origin
   is the in-process standalone tracker no network is involved at all.
3. **The version check** — one anonymous GitHub GET per day, at most.
4. **Deliberate sharing** — a paired machine reaching a home serve you set
   up yourself (below).

## Every outbound connection

The complete destination list — enforced in
[`SECURITY.md`](../SECURITY.md#data-flow), which also carries the one-line
grep that proves it stays complete:

| Destination | When | Carries | Off switch |
| --- | --- | --- | --- |
| **Your tracker** — the Atlassian site, Linear, or a paired home serve you configured | sync, write-through, and on-demand attachment fetch | to Atlassian and Linear's GraphQL: your API token; to Linear's signed upload PUT: no token, only Linear's signed headers; to a paired home serve: that device's bearer token. Never to any other host — cross-host redirects drop the Authorization header | remove the source; an unpaired standalone workspace has no remote at all |
| **GitHub Releases** | at most one anonymous version-check `GET` per day | nothing — no identifier, no local data | `updateCheck: false`; dev builds never check |
| **`gh` (your own CLI)** | only when you run `gadak dev scan` | whatever `gh` is already configured to send — gadak execs it, it does not call GitHub itself | don't run `dev scan` |

That table is exhaustive. There is no telemetry, no crash reporting, no
gadak-operated server, no account, and nothing that phones home on install,
on error, or on schedule. On a standalone workspace with `updateCheck:
false`, the gadak process itself opens no outbound connections — the one
thing that can still reach out is `gadak dev scan`, because it execs your
own `gh`.

Two boundaries worth knowing because agents run near them:

- **The credential cannot be aimed off-site.** `gadak api` refuses absolute
  URLs, so a prompt-injected path in issue text cannot walk your token to
  another host. Anything past `GET`/`HEAD` needs an explicit `--write`.
- **MCP has no raw proxy.** The MCP surface is five read/present tools; a
  shell-less host never gets a full-credential REST tunnel.

## Staying fresh without managing sync

An agent should not have to think about sync, and mostly it doesn't — four
surfaces keep the mirror current, and the verbs that matter warn when none
of them has run:

- **`gadak serve` and the desktop app** run a watch loop while open:
  incremental sync every `syncIntervalSec` (default 60s), a reconcile pass
  hourly.
- **`gadak mcp` runs the same loop** for hosts like Claude Desktop, so an
  MCP-only machine is as fresh as one with the app open. The loop does not
  start when `--no-sync` is set, when the workspace has no credential, or
  when it is frozen; it logs the reason to stderr so stdout stays pure
  JSON-RPC.
- **`gadak sync --if-stale 1h`** is the idempotent freshness verb for
  CLI-only agents and scripts: it syncs a source only when its last sync is
  older than the threshold or failed, prints one `fresh` line and exits 0
  otherwise — cheap enough to run before anything that depends on current
  data, quiet enough to run always. A failed last sync always retries, so a
  broken state heals instead of being reported as fresh forever.
- **Plain `gadak sync`** when you want it now, unconditionally.

The backstop: `sql`, `issue`, `search`, `fields`, and the write verbs
(`comment`, `transition`, `assign`, and the rest of the write session) print
a one-line stderr warning when the last sync failed or is over an hour old.
stdout stays clean and pipeable — which also means a pipe that discards
stderr sees no warning, so a script that must be sure checks `gadak status
--json` (`last_error`, watermark) or simply runs `sync --if-stale` first.

## Using the network on purpose: several hosts, one workspace

Everything above is gadak minimizing traffic. This section is the opposite
direction — you *want* machines talking, because one workspace should serve
more than one host, or more than one person.

### The model

A workspace is bound to **one origin**, and the origin owns the durable
record. For a connected workspace that origin is your Atlassian site or
Linear — so multi-host is trivial: run `gadak init` on each machine and every
mirror syncs from the same site independently. Nothing to pair, nothing new
on the network.

A **standalone** workspace carries its own tracker in-process; the durable
record is its persist file (that file, not any mirror, is what you back up).
To share it, one machine runs `gadak serve` as the **home serve** — the HTTP
face of that tracker — and other machines join by **pairing**:

```bash
# home machine — the origin
gadak pairing mint --label laptop            # prints one offer line

# the other machine
gadak --workspace team init --pairing-code-stdin   # paste the offer
gadak --workspace team status                       # "paired with …"
```

The paired machine gets a full local mirror (reads stay local and fast) and
routes every write through the home serve with a per-device bearer token.
Revocation is per device: `gadak pairing revoke laptop` cuts one machine off
without touching the others.

A **phone companion** (GDK-797) is not a second workspace — it is a REST
client of a home serve's mirror. `gadak pairing mint --label phone --scope
serve` works on any workspace kind (a connected home included, GDK-798; its
origin passthrough stays closed regardless): the token opens the mirror
REST allowlist — the reads a board needs plus the comment/transition
writes, which ride the same write-through path as the web UI — and is
refused on the origin passthrough, exactly as an origin token is refused on
the allowlist.

### Tailscale is the intended transport

`gadak serve` binds loopback and refuses anything else unless you pass
`--allow-remote`. The intended way to give a serve reach is a tailnet:

```bash
tailscale serve --bg 7777      # on the home machine
```

Tailscale owns reach, encryption, and node identity: only machines on your
tailnet can connect at all. gadak's pairing gate owns application-level
authorization on top: once any active pairing token exists, every request on
the origin passthrough (`/api/v1/origin`) must carry a valid
`Authorization: Bearer` token — loopback included, because a tunnel arrives
as loopback — and the serve stores only SHA-256 hashes of tokens.

What a tailnet device can and cannot reach through that tunnel is worth
stating precisely:

- **Through `tailscale serve`, a DNS hostname reaches exactly the
  token-gated surfaces.** Requests arrive with the tailnet hostname, and the
  serve's host guard rejects DNS hostnames that are not `localhost` unless
  a later gate authenticates the request. There are two such gates: the
  origin passthrough (an origin-scope token, for paired gadak machines) and
  the mirror REST allowlist (a serve-scope token, for a phone companion).
  A tailnet device with no token gets nothing; a paired laptop gets the
  passthrough, not the mirror; a phone token gets the allowlist, not the
  passthrough. The web UI and every other route stay closed behind the
  host guard.
- **`--allow-remote` is the sharp edge.** Binding a non-loopback address
  opens the mirror's read and write API to whoever can reach the port, with
  no login in front of it — gadak deliberately has no account model
  ([`SECURITY.md`](../SECURITY.md#data-flow)). Reach for it only when the
  network boundary itself (a firewalled host, a tailnet ACL) is the access
  control you mean.

### A team on one standalone workspace

The same pieces compose into a small team tracker with no vendor and no
server bill:

- One machine (or an always-on box on the tailnet) runs the home serve —
  the HTTP face of the origin, whose persist file is the single thing to
  back up.
- Each teammate pairs their own machine and gets their own revocable token.
  Everyone reads at local-mirror speed; every write converges on the origin.
- Each teammate's agents get the same treatment as everywhere else in
  gadak: `gadak sql` locally, writes through the paired origin, freshness
  from the watch loop or `sync --if-stale`.
- Concurrent agents don't trample each other's claims: `gadak claim` is one
  atomic call on the standalone origin, and a claim someone already holds
  is refused with the holder's name.

What this deliberately is not: a hosted product. The traffic stays inside
your tailnet, the data stays in files you own, and offboarding a machine is
one `revoke` — offboarding entirely is deleting a directory.

## Where to go next

- [`SECURITY.md`](../SECURITY.md) — the threat model and where each claim is
  enforced in code.
- [`AGENT_ACCESS.md`](AGENT_ACCESS.md) — the three access layers an agent
  chooses between, and the `gadak api` escape hatch.
- [`AGENT_SETUP.md`](AGENT_SETUP.md) — one paste per host to onboard an
  agent.
