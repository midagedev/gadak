# Handoff — state as of 2026-08-05

Written for resuming work from another machine. `docs/STATE_OF_PLAY.md` is the
verified feature inventory; `docs/ROADMAP.md` is the ordering; this page is
only what a fresh session needs to continue.

## Where things stand

Everything below is merged to `main`, pushed, CI green (build + secret scan +
Playwright 18/18):

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
- **Docs repositioned** after an external-mentor-style review: README leads
  with the agent angle and the no-token demo; Rovo MCP addressed in the
  comparison; roadmap reordered around retention.

No unmerged worktrees or branches remain; `git worktree list` shows only main.

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

- TUI follow-ups deliberately parked: feed focus tabs, saved-view sort/group.
- `scry snapshot` (specified, unimplemented; usage says so).
- Windows: OS notifications and install-service are explicit no-ops.
- Parked-by-design: workspaces UI (v0.4), JQL→SQL bridge, Confluence.
