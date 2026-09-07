# tools

## `seed-demo`

Populates a throwaway Jira Cloud site with a realistic backlog: releases,
components, issues, real transition history, comments, and issue links.

Used to produce the data behind gadak's screenshots and `examples/demo.db`, and
reusable by anyone who wants a demo Jira to point gadak at.

```bash
export JIRA_SITE=https://your-site.atlassian.net
export JIRA_EMAIL=you@example.com
export JIRA_TOKEN=...    # id.atlassian.com/manage-profile/security/api-tokens

# Project it from the committed dataset (reproducible)
go run ./tools/seed-demo --data examples/demo-seed.json --projects NMB,NMA,NMS

# Or generate content procedurally (quick, more repetitive)
go run ./tools/seed-demo --projects NMB,NMA,NMS --issues 300
```

Flags: `--dry-run`, `--skip-setup` (versions and components already exist),
`--no-history` (skip transitions, comments, links), `--assignees <id,id>`,
`--seed <int>`.

Redistribute assignees across a pool of accounts (dataset issues follow their
`assignee_slot`; other issues keep their assigned/unassigned status but get spread
by key hash, so repeated runs are a no-op):

```bash
go run ./tools/seed-demo --data examples/demo-seed.json \
  --repair-assignees --assignees "<accountId>,<accountId>,<accountId>"
```

Repair a site whose workflow states do not match the dataset — issues, comments,
and links are left alone and only statuses are re-driven, matched by summary:

```bash
go run ./tools/seed-demo --data examples/demo-seed.json --repair-states
```

### Requirements

- Projects must exist and be **company-managed**. Team-managed projects do not
  expose `priority`, `components`, or `fixVersions`, which leaves most of the
  UI's filter axes empty.
- A user API token. Organization API keys from `admin.atlassian.com` (prefix
  `ATCTT`) authenticate only against organization admin APIs and will 401 against
  every product endpoint.

### Gotchas encoded in the script

- `issue/createmeta` translates issue-type names into the account's display
  language and ignores `Accept-Language`, so name matching breaks on non-English
  accounts. The script reads `project/{key}/statuses`, which is not localized.
- Jira assigns `created` at insert time with no way to backdate, so seeded issues
  share roughly one creation time. Realistic time spread is applied by
  `gadak snapshot`, not here.
- Deleting issues needs the "Delete Issues" permission, which the default
  company-managed scheme does not grant. Plan runs so you do not need to undo them.
- Default workflows offer a direct `Backlog -> Done` edge. Taking it leaves a
  single changelog entry, which makes derived fields correct but the history
  timeline empty — so the script walks the status *category* ladder one rung at a
  time (`new -> indeterminate -> done`) instead, and only falls back to a direct
  jump when no stepwise path exists.
- An admin cannot set a user's display name. `POST /rest/api/3/user` accepts
  `displayName` and ignores it for accounts outside a verified domain — Jira uses
  the email local part until the account holder accepts the invitation and sets
  their own name. Demo personas created this way read as `you+alice` until then.
- A `reopened` issue is driven to done and then back, because that is the only
  way to get a real done-to-not-done transition into the changelog. History
  cannot be backfilled after the fact: pushing an already-done issue backwards
  later would register as a reopen it was never supposed to have.

## `hosted-demo`

Builds the zero-install hosted demo into `dist/hosted/` (Vite with
`VITE_HOSTED_DEMO=1` + `gadak export-static` over `examples/demo.db`). See
`make hosted-demo` and ADR 0004 addendum.

```bash
node tools/hosted-demo/build.mjs
# or: make hosted-demo && make hosted-demo-test
```

## `backlog-snapshot.sh` / `backlog-scrub-check.sh`

Freeze the maintainer's GDK mirror into `examples/backlog-snapshot/` — the
committed, whitelist-scrubbed data behind the public backlog page
(`/gadak/backlog/`, GDK-389). The snapshot script runs locally (CI never
holds Jira credentials); the check script asserts the scrub invariants (no
members, no people fields, empty descriptions/comments/attachments/history,
no emails, no concrete site URLs) and runs again in `pages.yml` on the
built artifact. Refresh is manual, release-time by default.

```bash
tools/backlog-snapshot.sh            # default mirror: ~/.gadak/profiles/gdk/gadak.db
tools/backlog-scrub-check.sh examples/backlog-snapshot
```

## `bench-fixture`

Builds a deterministic synthetic `gadak.db` for latency work (T6.7 / G5). No
network — only `internal/store`.

```bash
go run ./tools/bench-fixture -out /tmp/bench.db -issues 10000
make bench   # BenchmarkBootstrap10k + BenchmarkSearch10k
```

## `examples/demo-seed.json`

The dataset the seeder projects onto Jira. Every issue has a unique summary and
hand-authored body and comments; nothing is templated, and nothing derives from a
real backlog.

```json
{
  "issues": [
    {
      "project": "NMB", "type": "Bug",
      "summary": "...", "description": ["...", "..."],
      "priority": "High", "components": ["Dashboard"], "fix_version": "2026.9.0",
      "labels": ["regression"], "environment": "Chrome 141 / macOS 15.2",
      "state": "inprogress", "reopened": false, "assignee_slot": 1,
      "comments": ["..."], "links": [{"type": "Relates", "target": 42}]
    }
  ]
}
```

- `state` is one of `backlog`, `selected`, `inprogress`, `done`; the seeder walks
  the real workflow to reach it, so the changelog is genuine.
- `reopened: true` makes the seeder push the issue to done and then back, which is
  what produces a real reopen in the changelog.
- `links[].target` is an index into the same array.
- `assignee_slot` is mapped to whatever accounts `--assignees` provides.

## `release-stats`

Writes one JSON snapshot of GitHub release asset `download_count` and the
14-day traffic window (`/traffic/clones`, `/traffic/views`). Traffic is a
rolling window; a periodic snapshot is the only history. This talks to
GitHub's API from a workflow or a human — it is not product telemetry.

```bash
tools/release-stats.sh --out /tmp/stats
# writes /tmp/stats/<YYYY-MM-DD>.json (UTC stamp). Same-day reruns overwrite.

tools/release-stats.sh --out /tmp/stats --repo midagedev/gadak --stamp 2026-08-18
```

`--repo` defaults to `midagedev/gadak`. `--stamp` defaults to UTC today.
Requires `gh` and `jq`. A traffic 403 is recorded in `errors[]` and the rest
of the file is still written (exit 0); if every endpoint fails, exit 1 and
no file is written.

`.github/workflows/stats.yml` runs this weekly (Monday 06:00 UTC) and on
`workflow_dispatch`, and commits `stats/<stamp>.json` on the `stats` branch.
The files live only on that branch — they are not on `main`.

**How to read.** `downloads_total` is asset hits, not unique people.
`checksums.txt` / `SHA256SUMS` are a large share. Homebrew cask/formula
installs fetch release assets, so they are already in the count; the tap
has no analytics of its own. Clone uniques are inflated by Actions
runners. Prefer stars and 14-day view uniques as a people-shaped ceiling.
Do not put these numbers in the README lead.

```bash
git fetch origin stats
git show origin/stats:stats/2026-08-25.json \
  | jq '{stamp, downloads_total, clones:.traffic.clones|{count,uniques}, views:.traffic.views|{count,uniques}, errors}'

# per-release cumulative (includes checksums)
git show origin/stats:stats/2026-08-25.json \
  | jq -r '.releases[] | "\(.tag)\t\(.downloads_total)"'

# binaries only
git show origin/stats:stats/2026-08-25.json \
  | jq '[.releases[].assets[] | select(.name | test("checksum|SHA256SUMS") | not) | .download_count] | add'
```

The weekly job's `GITHUB_TOKEN` 403s `traffic/*`. A human `gh` with
push access still reads those endpoints — run the script locally and
commit onto `stats` so the 14-day window is not lost. Stars, referrers,
and popular paths are not in the JSON yet; `errors[]` is the signal
that a window was dropped.

## `winsmoke.ps1`

Manual / real-machine Windows startup gate (GDK-244). **Not a CI job** —
GitHub `windows-latest` has no interactive desktop, so a capture there is
black. Run it on a real Windows session (or `schtasks /create /it`).

Launches `gadak-desktop.exe` from a portable pack directory, waits for the
window, records hwnd/rect/style, counts WebView2 children, captures the
screen, asserts it is not black, closes the app, and checks `GADAK_HOME`
did not touch `%USERPROFILE%\.gadak`. Exit 0 / 1 / 64 / 69. The app has no
TCP listener by design; "no port open" is a pass.

The file is UTF-8 **with BOM** (Windows PowerShell 5.1 otherwise reads it
as the system ANSI page).

```powershell
# After desktop/build-windows.ps1 has written the portable directory:
tools/winsmoke.ps1 -BundleDir desktop\\build\\Gadak-<ver>-x64 -OutDir $env:TEMP\\gadak-winsmoke\\out
```

## `staticcheck.sh`

Runs [staticcheck](https://staticcheck.dev) over both Go modules (root and
`desktop/`, which has its own `go.mod`) under GOOS darwin, linux and windows,
and reports only what every analysed GOOS agrees on (GDK-1463).

The matrix is the point. staticcheck analyses one GOOS at a time, so a function
whose only caller sits behind a `//go:build <other-goos>` tag looks like dead
code: a darwin-only run calls `parseProcStartTime` (`internal/term/`, called
from `members_linux.go`) and `protocolDefaultIcon` (`desktop/`, called from
`protocol_windows.go`) unused, and both are false. Findings that appear under
some but not all GOOS are printed under a **platform-only (informational)**
heading with the GOOS they came from, and never fail the run.

`ST1005` is excluded: gadak's error strings are Korean sentences that start
with a Latin word, and `internal/mcp/tools.go` echoes a multi-line protocol
string on purpose.

A `(module, GOOS)` pair that the host cannot cross-compile is skipped with a
note rather than reported — `desktop/` pulls wails, whose linux backend is cgo,
so `GOOS=linux` cannot be analysed from macOS and `GOOS=darwin` cannot be from
the linux CI runner. A compile error in a path **inside** this repo is a real
break and fails; one in the module cache is the skip. Either way the run
contributes no findings, because an incomplete package set would demote real
ones by accident.

```bash
tools/staticcheck.sh               # exit 1 on cross-platform findings
tools/staticcheck.sh --warn-only   # always exit 0 — what CI runs today
tools/staticcheck.sh --goos darwin # one GOOS, to reproduce a single finding
tools/staticcheck.sh --self-test   # classifier vs fixtures; no toolchain needed
```

Requires `go install honnef.co/go/tools/cmd/staticcheck@latest` (the script
prints that line and exits 2 if the binary is missing). CI pins the version and
runs `--warn-only` in the `Staticcheck (warning-only)` job.
