# Promises

Twelve things gadak asks you not to take on trust: one sentence each, and one
command you can run in a clone of this repository — a Go toolchain for seven
of them, `sqlite3` or `grep` for the rest. Every command was run on this tree
and produced the output shown; if one stops doing so, the promise is broken and
that is a bug worth reporting. Only what is true and checkable today, no roadmap.
The threat model and the reasoning live in [`SECURITY.md`](../SECURITY.md).

**1. There is no telemetry, no analytics, and no gadak account or server.**
Nothing anywhere reports what you searched, opened, or synced.

```bash
grep -rniE 'telemetry|analytics|mixpanel|amplitude|posthog|sentry\.io|google-analytics' \
  --include='*.go' --include='*.ts' --include='*.svelte' internal cmd desktop web/src | wc -l
# → 0
```

**2. Outbound traffic is the six destinations in SECURITY.md.**
<!-- outbound: Your own Atlassian site | GitHub Releases | Linear | Pairing home serve | User-invoked gh | User-invoked library download -->
Your own Atlassian site; GitHub's release API
(`api.github.com/repos/midagedev/gadak/releases/latest`); Linear
(`api.linear.app` GraphQL and a signed PUT to `uploads.linear.app`); a
pairing home serve; user-invoked `gh`; a one-shot, user-invoked
`gadak dashboards lib add <url>` download to the URL the user typed. The `atlassian.net` strings below
are placeholders in help text and comments. `developer.microsoft.com`,
`x.com`, and `github.com` are desktop Help-menu / help-text strings, not
hosts the HTTP client calls; `https://github` is the regex literal
`https://github\.com` truncated by this grep.

```bash
grep -rhoE 'https://[a-z0-9.-]+' --include='*.go' --exclude='*_test.go' internal cmd desktop | sort -u
# → https://api.github.com
#   https://api.linear.app
#   https://developer.microsoft.com
#   https://example.atlassian.net
#   https://github
#   https://github.com
#   https://linear.app
#   https://uploads.linear.app
#   https://x.atlassian.net
#   https://x.com
#   https://your-site.atlassian.net
```

**3. That release check is off with one config line, and never runs on a dev build.**
`updateCheck: false` in `~/.gadak/config.json`; both tests assert zero network hits.

```bash
go test ./internal/selfupdate/ -run 'TestCheck_disabled|TestCheck_devVersion' -count=1
# → ok  github.com/midagedev/gadak/internal/selfupdate
```

**4. The mirror is an ordinary SQLite file, readable without gadak.**
No custom container and no lock-in: the bundled demo mirror opens in any SQLite
client. On a connected workspace, deleting a mirror loses nothing your
Atlassian site does not hold.

```bash
sqlite3 examples/demo.db 'select count(*) from issues'
# → 534
```

**5. The mirror cannot be written through the paths an agent reads.**
`gadak sql` and the MCP `gadak_query` tool open the mirror with SQLite `mode=ro`,
and MCP additionally refuses anything that is not a single SELECT or WITH.

```bash
go test ./internal/mcp/ -run 'TestWriteSQLRejected|TestRejectNonSelectUnit' -count=1
# → ok  github.com/midagedev/gadak/internal/mcp
```

**6. `gadak serve` refuses a non-loopback bind unless you ask for one.**
`--allow-remote` is the only way past the check, and it is never a default.

```bash
go test ./cmd/gadak/ -run 'TestCheckServeAddr|TestIsLoopback' -count=1
# → ok  github.com/midagedev/gadak/cmd/gadak
```

**7. A hostile web page cannot post through gadak, or rebind a DNS name at it.**
State-changing requests reject a foreign `Origin` — no comments or transitions by
CSRF; every request rejects a `Host` that is neither localhost nor an IP literal.

```bash
go test ./internal/server/ -run TestBrowserGuard -count=1
# → ok  github.com/midagedev/gadak/internal/server
```

**8. A snapshot you hand to someone else cannot carry your API token.**
`gadak snapshot` scans every text column of the finished file before publishing it
and refuses on a hit; `--force` overwrites the destination, it does not skip the scan.

```bash
go test ./internal/snapshot/ -run 'TestCredentialRejected|TestCredentialInPageRejected' -count=1
# → ok  github.com/midagedev/gadak/internal/snapshot
```

**9. On standalone, the origin is one ordinary SQLite file, readable without gadak.**
`gadak init --local` writes `origin/issuetap.db` under the profile
directory (`internal/origin/origin.go` `PersistRel`) — plain SQLite, no
custom container. That file is the record; `gadak.db` remains a disposable
cache filled by sync. The command uses a throwaway `GADAK_HOME` so it never
touches `~/.gadak`.

```bash
tmp=$(mktemp -d)
go build -o "$tmp/gadak" ./cmd/gadak
GADAK_HOME=$tmp "$tmp/gadak" init --local --json >/dev/null
GADAK_HOME=$tmp "$tmp/gadak" create "written through gadak" --project STD --json >/dev/null 2>&1
sqlite3 "$tmp/origin/issuetap.db" "select key, json_extract(blob, '$.summary') from issues"
# → STD-1|written through gadak
```

**10. Deleting `gadak.db` loses nothing you made.**
Saved views, visits, and search history live in `local.db`, a separate file
beside the mirror — the mirror is a cache, and your own state is not in it.
Delete `gadak.db` — corrupted mirror, full disk, fresh re-sync — and the
views you saved, the issues you opened, and what you searched for are still
there.

```bash
tmp=$(mktemp -d)
go build -o "$tmp/gadak" ./cmd/gadak
GADAK_HOME=$tmp "$tmp/gadak" init --local --json >/dev/null
GADAK_HOME=$tmp "$tmp/gadak" views save "Night triage" --jql 'project = STD'
rm "$tmp/gadak.db"
GADAK_HOME=$tmp "$tmp/gadak" views | grep -c 'Night triage'
# → 1
```

**11. What you type in gadak's terminal never touches disk.**
The shell that opens inside gadak is a real PTY, and its scrollback lives in
one fixed byte slice in memory — a ring, 256 KiB, overwritten as it fills.
The package that owns those sessions writes no file at all: not a log, not a
transcript, not a crash dump. Close the session and the bytes are gone with
the process. What leaves this machine is still only promise 2's destinations,
and none of them is fed by a shell.

```bash
printf 'ring in memory: %s\nfiles the package writes: %s\n' \
  "$(grep -c 'buf *\[\]byte' internal/term/ring.go)" \
  "$(grep -rlE 'os\.(Create|WriteFile|OpenFile)' internal/term --include='*.go' \
     | grep -v _test | wc -l | tr -d ' ')"
# → ring in memory: 1
# → files the package writes: 0
```

**12. Only a terminal-scoped pairing token can open a shell.**
A `serve` token reaches the mirror over REST and a paired phone's issues; it
opens no shell. Neither does an `origin` token, and neither does a token
minted before the terminal existed — an empty scope does not silently
acquire one. One rule decides it, and the server asks that rule on every
terminal route. Loopback is the exception in the other direction: on your own
machine there is no token at all.

```bash
grep -A 1 'func AdmitsTerminal' internal/pairing/store.go | tail -1
grep -rn 'AdmitsTerminal' internal --include='*.go' | grep -v _test \
  | grep -c 'internal/server/'
# → 	return scope == ScopeTerminal
# → 3
```
