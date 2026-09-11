# Promises

Eight claims about gadak, each with one command that checks it in a clone of
this repository. Run them: that is the point of the file. Most need a Go
toolchain, the rest `sqlite3` or `grep`. Every block was run on this tree and
produced the output shown, and `tools/check-promises.sh` re-runs them all in
CI — a block that stops passing is a broken promise and a bug worth reporting.

Only what is true and checkable today. No roadmap, and nothing here about what
gadak intends to do. The threat model and the reasoning behind these claims
are in [`SECURITY.md`](../SECURITY.md); this file is the evidence.

They answer four questions, in the order people actually ask them: does it
report on me, can I get my data back out, what can reach it while it runs, and
when does it go to the network at all.

## Does it report on me

**1. There is no telemetry and no gadak account, and outbound traffic is the
five destinations SECURITY.md names.**
<!-- outbound: Your own Atlassian site | Linear | Pairing home serve | User-invoked gh | User-invoked library download -->
Nothing anywhere reports what you searched, opened, or synced — there is no
service to report it to. What gadak does connect to is your own Atlassian
site; Linear (`api.linear.app` GraphQL and a signed PUT to
`uploads.linear.app`); a paired home `serve`; `gh`, and a
`gadak dashboards lib add <url>` download, each only when you run it.

The first command below counts the usual reporting SDKs. The second lists
every `https://` literal in the Go source, which is more than five, because a
literal is not a call: the `atlassian.net` names are placeholders in help
text, `cdn.jsdelivr.net` is the example URL in that command's own help,
`developer.microsoft.com`, `x.com` and `github.com` are Help-menu strings, and
`https://github` is the regex `https://github\.com` cut off by this grep.

```bash
grep -rniE 'telemetry|analytics|mixpanel|amplitude|posthog|sentry\.io|google-analytics' \
  --include='*.go' --include='*.ts' --include='*.svelte' internal cmd desktop web/src | wc -l
grep -rhoE 'https://[a-z0-9.-]+' --include='*.go' --exclude='*_test.go' internal cmd desktop | sort -u
# → 0
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

**2. A snapshot you hand to someone else cannot carry your API token.**
`gadak snapshot` scans every text column of the finished file before publishing it
and refuses on a hit; `--force` overwrites the destination, it does not skip the scan.

```bash
go test ./internal/snapshot/ -run 'TestCredentialRejected|TestCredentialInPageRejected' -count=1
# → ok  github.com/midagedev/gadak/internal/snapshot
```

## Can I get my data back out

**3. Every file gadak keeps is ordinary SQLite, readable without gadak.**
No custom container and no lock-in. The cache opens in any SQLite client, and
on the built-in tracker so does the record itself: `gadak init --local` writes
`origin/issuetap.db` under the profile directory
(`internal/origin/origin.go` `PersistRel`) — that file is what you own, and
`gadak.db` beside it is a disposable cache sync refills. The second half uses
a throwaway `GADAK_HOME`, so it never touches `~/.gadak`.

```bash
sqlite3 examples/demo.db 'select count(*) from issues'
tmp=$(mktemp -d)
go build -o "$tmp/gadak" ./cmd/gadak
GADAK_HOME=$tmp "$tmp/gadak" init --local --json >/dev/null
GADAK_HOME=$tmp "$tmp/gadak" create "written through gadak" --project STD --json >/dev/null 2>&1
sqlite3 "$tmp/origin/issuetap.db" "select key, json_extract(blob, '$.summary') from issues"
# → 534
# → STD-1|written through gadak
```

**4. Deleting the cache loses nothing you made.**
Saved views, visits, and search history live in `local.db`, a separate file
beside the mirror — the mirror is a cache, and your own state is not in it.
Delete `gadak.db` — corrupted mirror, full disk, fresh re-sync — and the
views you saved, the issues you opened, and what you searched for are still
there. On a Jira workspace, what the mirror held is still on your Atlassian
site.

```bash
tmp=$(mktemp -d)
go build -o "$tmp/gadak" ./cmd/gadak
GADAK_HOME=$tmp "$tmp/gadak" init --local --json >/dev/null
GADAK_HOME=$tmp "$tmp/gadak" views save "Night triage" --jql 'project = STD'
rm "$tmp/gadak.db"
GADAK_HOME=$tmp "$tmp/gadak" views | grep -c 'Night triage'
# → 1
```

## What can reach it while it runs

**5. Nothing off this machine reaches gadak unless you asked for it, and no web
page reaches it at all.**
`gadak serve` refuses a non-loopback bind unless you pass `--allow-remote`,
which is never a default. State-changing requests reject a foreign `Origin` —
no comments or transitions by CSRF — and every request rejects a `Host` that
is neither localhost nor an IP literal, which is what closes DNS rebinding.

```bash
go test ./cmd/gadak/ -run 'TestCheckServeAddr|TestIsLoopback' -count=1
go test ./internal/server/ -run TestBrowserGuard -count=1
# → ok  github.com/midagedev/gadak/cmd/gadak
# → ok  github.com/midagedev/gadak/internal/server
```

**6. The paths an agent reads through cannot write.**
`gadak sql` and the MCP `gadak_query` tool open the mirror with SQLite `mode=ro`,
and MCP additionally refuses anything that is not a single SELECT or WITH.

```bash
go test ./internal/mcp/ -run 'TestWriteSQLRejected|TestRejectNonSelectUnit' -count=1
# → ok  github.com/midagedev/gadak/internal/mcp
```

**7. The shell is fenced twice: only a terminal-scoped token opens one, and
what you type in it never touches disk.**
A `serve` token reaches the mirror over REST and a paired phone's issues; it
opens no shell. Neither does an `origin` token, and neither does a token
minted before the terminal existed — an empty scope does not silently acquire
one. One rule decides it, and the server asks that rule on every terminal
route. Loopback is the exception in the other direction: on your own machine
there is no token at all. And the shell that opens is a real PTY whose
scrollback lives in one fixed byte slice in memory — a ring, 256 KiB,
overwritten as it fills. The package that owns those sessions writes no file
at all: not a log, not a transcript, not a crash dump. Close the session and
the bytes are gone with the process.

```bash
grep -A 1 'func AdmitsTerminal' internal/pairing/store.go | tail -1
grep -rn 'AdmitsTerminal' internal --include='*.go' | grep -v _test \
  | grep -c 'internal/server/'
printf 'ring in memory: %s\nfiles the package writes: %s\n' \
  "$(grep -c 'buf *\[\]byte' internal/term/ring.go)" \
  "$(grep -rlE 'os\.(Create|WriteFile|OpenFile)' internal/term --include='*.go' \
     | grep -v _test | wc -l | tr -d ' ')"
# → 	return scope == ScopeTerminal
# → 3
# → ring in memory: 1
# → files the package writes: 0
```

## When does it go to the network

**8. A read verb answers from the disk and opens no socket.**
`gadak sql`, `issue`, `search`, `list` and the rest of the read section of
`gadak --help` read the cache and nothing else: no request, not even a name
lookup, and no child process started to make one behind your back. The network
moves when you run `sync`, or while `serve`, `mcp` or the app is up — never as
a side effect of a question. If the cache is behind, they say so in one line on
stderr and teach the command that fixes it (`warnIfStale`, the preamble every
read verb shares), but they do not fix it themselves. Unplug the cable and
every one of them answers the same.

The test below runs each of those verbs in-process against the bundled demo
cache with the HTTP transport and the DNS resolver replaced by hooks that fail
on use, and with this package's child-process ledger armed — a detached sync
would slip past the first hook, having a transport of its own. The list of
verbs is read out of the `Reading the mirror` section of the usage string, so a
read verb added later is covered or named as an exception; it cannot arrive
uncounted.

```bash
go test ./cmd/gadak/ -run 'TestReadVerbsAnswerWithoutNetwork|TestPromiseReadVerbsAreClassified|TestSpawnHasOneOwner|TestOutboundHTTPHasNoPrivateTransport' -count=1
# → ok  github.com/midagedev/gadak/cmd/gadak
```
