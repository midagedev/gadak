# Handoff — state as of 2026-08-05

Written for resuming work from another machine. `docs/STATE_OF_PLAY.md` is the
verified feature inventory; `docs/ROADMAP.md` is the ordering; this page is
only what a fresh session needs to continue.

## Where things stand

Everything below is merged to `main`, CI green (build + secret scan +
Playwright 19/19):

- **v0.2 scope complete**: watch feed (server computes from the mirror; web,
  TUI, and in-tab notifications), attachment disk cache, onboarding, settings
  runtime panel, MCP, packaging (goreleaser + brew tap config + install.sh +
  Docker), community health files.
- **v0.3 scope complete in code**: OS desktop notifications from the sync loop,
  `scry install-service`, `serve` syncs by default (`--no-sync` opts out),
  retention counters (`first_sync_at`/`sync_count`), `issues_full` view,
  zero-install hosted demo (`make hosted-demo`, Pages workflow), query recipes
  (`docs/RECIPES.md`), agent setup doc (`docs/AGENT_SETUP.md`), newest-first
  full sync, `scry open`, browser auto-open on serve (`--no-open`).
- **v0.3 remainder closed** (2026-08-05): `scry snapshot` (T6.4 — the last
  unimplemented command in the spec), TUI feed focus tabs and saved-view
  sort/group, per-command `--help`, and the UX/quality debt (favorites moved
  into the mirror, `presence` client stack deleted, duplicate `initials`
  merged, storage keys renamed off the project's old name).
- **Research-backed Later items closed**: `scry fields` (which custom fields
  are actually populated) and call-volume instrumentation — **schema v6**,
  `api_usage`, surfaced in `scry status` and the settings runtime panel.
- **Team config sharing**: `scry team export` / `import` for the views, field
  map and group rules a team agrees on. Whitelist-only, credentials never
  travel, `--dry-run` previews the exact plan the apply path runs.
- **Docs repositioned** after an external-mentor-style review: README leads
  with the agent angle and the no-token demo; Rovo MCP addressed in the
  comparison; roadmap reordered around retention.

No unmerged worktrees or branches remain; `git worktree list` shows only main.

`main` is ahead of `origin` by the 2026-08-05 work — push when ready.

## Blocked on the repo owner (in order)

1. **Tag `v0.2.0`** — the release pipeline is verified; the only prerequisites
   are: create the empty public repo `midagedev/homebrew-tap`, add the
   `HOMEBREW_TAP_GITHUB_TOKEN` secret (contents:write on the tap), and rotate
   the demo-profile Jira token (it was exposed in a chat once). Then
   `git tag v0.2.0 && git push origin v0.2.0`.
2. **Enable GitHub Pages** (Settings → Pages → Source: GitHub Actions) — the
   hosted demo deploys itself on the next main push.
3. **Flip the repo public** when ready; upload `docs/media/og.png` as the
   social preview (Settings → Social preview; not scriptable).
4. **Install scry for five teammates by hand** and calendar a D14 check
   (`scry status --json` → `first_sync_at`, `sync_count`). This was the
   sharpest point of the mentor review: internal retention before any launch.
5. **Launch** only after the hosted demo link works: Show HN draft, expected
   questions, and an X thread live in the session scratchpad (`launch-plan.md`)
   and need approval before anything is posted.

## Conventions a resuming session must keep

- Commits authored as `midagedev <midagedev@users.noreply.github.com>`; GitHub
  API calls need `GH_TOKEN=$(gh auth token --user midagedev)`.
- `./scripts/scan-internal.sh` must pass before every push (CI enforces it).
- E2E flakes: kill leftover `scry serve` processes first — stale fixtures on
  port 7877 produce all-red runs that look like regressions.
- The e2e fixture must keep `--no-sync --no-open` (fake credential, headless).
- `e2e/helpers.ts` pins the boot assertion to one element on purpose; the bare
  text `519 issues` matches two nodes.
- Media regeneration: `make media` (needs `SCRY_FRESHEN=1`, already wired).

## Open items beyond the blockers

- **v0.4 workspaces is next in the roadmap**, and its design question is
  settled: a *single active workspace* the server switches between, not one
  process holding several mirrors open at once. The switcher enumerates
  profiles and re-points the serve target; that keeps one credential open at a
  time and leaves the API contract almost untouched. Viewing two sites side by
  side is explicitly out of scope until someone asks for it.
- `scry team` has no web surface yet — export/import is CLI-only. A settings
  panel button is the obvious follow-up.
- `scry fields` has now run against the live demo site (519 issues, 60-issue
  sample, 41 custom fields in the catalog). Two defects only that run could
  show: alias suggestions were built from localized field names — a Korean
  account proposes `순위` where an English one proposes `rank`, and a fieldMap
  is precisely what `scry team export` shares — and the table misaligned on
  CJK names. Aliases now fall back to the field id (`cf_10019`) and column
  widths are measured in terminal cells.
- Windows: OS notifications and install-service are explicit no-ops.
- Parked-by-design: JQL→SQL bridge, Confluence, offline write queue.
