# Promises

Nine things gadak asks you not to take on trust: one sentence each, and one
command you can run in a clone of this repository — a Go toolchain for six of
them, `sqlite3` for one, `grep` for the rest. Every command was run on this tree
and produced the output shown; if one stops doing so, the promise is broken and
that is a bug worth reporting. Only what is true and checkable today, no roadmap.
The threat model and the reasoning live in [`SECURITY.md`](SECURITY.md).

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

**9. On standalone, the origin is one plaintext YAML file, readable without gadak.**
`gadak init --standalone` writes `origin/issuetap.yaml` under the profile
directory (`internal/origin/origin.go` `PersistRel`). That file is the record;
`gadak.db` remains a disposable cache. The command uses a throwaway
`GADAK_HOME` so it never touches `~/.gadak`.

```bash
tmp=$(mktemp -d)
go build -o "$tmp/gadak" ./cmd/gadak
GADAK_HOME=$tmp "$tmp/gadak" init --standalone --json >/dev/null
head -n 16 "$tmp/origin/issuetap.yaml"
# → seed: 1
#   locale: en
#   users:
#     - accountId: 5b10a2844c20165700ede21g
#       name: ada
#       key: ada
#       displayName: Ada Lovelace
#       email: you@example.com
#       active: true
#       timeZone: Asia/Seoul
#   projects:
#     - id: "10000"
#       key: STD
#       name: Standalone
#       type: software
#       style: classic
```
