# Promises

Eleven claims about gadak, each with one command that checks it in a clone of
this repository. Run them: that is the point of the file. Six need a Go
toolchain, the rest `sqlite3` or `grep`. Every block was run on this tree and
produced the output shown, and `tools/check-promises.sh` re-runs them all in
CI — a block that stops passing is a broken promise and a bug worth reporting.

Only what is true and checkable today. No roadmap, and nothing here about what
gadak intends to do. The threat model and the reasoning behind these claims
are in [`SECURITY.md`](../SECURITY.md); this file is the evidence.

The first three are the ones that decide whether you can install this at work.
The rest answer, in order: can you get your data back out, and what can reach
gadak once it is running.

## What leaves this machine

**1. There is no telemetry, no analytics, and no gadak account or server.**
Nothing anywhere reports what you searched, opened, or synced.

```bash
grep -rniE 'telemetry|analytics|mixpanel|amplitude|posthog|sentry\.io|google-analytics' \
  --include='*.go' --include='*.ts' --include='*.svelte' internal cmd desktop web/src | wc -l
# → 0
```

**2. Outbound traffic is the five destinations in SECURITY.md.**
<!-- outbound: Your own Atlassian site | Linear | Pairing home serve | User-invoked gh | User-invoked library download -->
Your own Atlassian site; Linear (`api.linear.app` GraphQL and a signed PUT to
`uploads.linear.app`); a paired home `serve`; `gh`, and a
`gadak dashboards lib add <url>` download, each only when you run it.

The grep below lists every `https://` literal in the Go source, which is more
than five, because a literal is not a call: the `atlassian.net` names are
placeholders in help text, `cdn.jsdelivr.net` is the example URL in that
command's own help, `developer.microsoft.com`, `x.com` and `github.com` are
Help-menu strings, and `https://github` is the regex `https://github\.com`
cut off by this grep.

```bash
grep -rhoE 'https://[a-z0-9.-]+' --include='*.go' --exclude='*_test.go' internal cmd desktop | sort -u
# → https://api.linear.app
#   https://cdn.jsdelivr.net
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

**3. A snapshot you hand to someone else cannot carry your API token.**
`gadak snapshot` scans every text column of the finished file before publishing it
and refuses on a hit; `--force` overwrites the destination, it does not skip the scan.

```bash
go test ./internal/snapshot/ -run 'TestCredentialRejected|TestCredentialInPageRejected' -count=1
# → ok  github.com/midagedev/gadak/internal/snapshot
```

## What you can take away

**4. The mirror is an ordinary SQLite file, readable without gadak.**
No custom container and no lock-in: the bundled demo mirror opens in any SQLite
client. On a Jira workspace, deleting a mirror loses nothing your
Atlassian site does not hold.

```bash
sqlite3 examples/demo.db 'select count(*) from issues'
# → 534
```

**5. Deleting `gadak.db` loses nothing you made.**
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

**6. On the built-in tracker, the origin is one ordinary SQLite file, readable without gadak.**
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

## What can reach gadak

**7. `gadak serve` refuses a non-loopback bind unless you ask for one.**
`--allow-remote` is the only way past the check, and it is never a default.

```bash
go test ./cmd/gadak/ -run 'TestCheckServeAddr|TestIsLoopback' -count=1
# → ok  github.com/midagedev/gadak/cmd/gadak
```

**8. A hostile web page cannot post through gadak, or rebind a DNS name at it.**
State-changing requests reject a foreign `Origin` — no comments or transitions by
CSRF; every request rejects a `Host` that is neither localhost nor an IP literal.

```bash
go test ./internal/server/ -run TestBrowserGuard -count=1
# → ok  github.com/midagedev/gadak/internal/server
```

**9. The mirror cannot be written through the paths an agent reads.**
`gadak sql` and the MCP `gadak_query` tool open the mirror with SQLite `mode=ro`,
and MCP additionally refuses anything that is not a single SELECT or WITH.

```bash
go test ./internal/mcp/ -run 'TestWriteSQLRejected|TestRejectNonSelectUnit' -count=1
# → ok  github.com/midagedev/gadak/internal/mcp
```

**10. Only a terminal-scoped pairing token can open a shell.**
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
