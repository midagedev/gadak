# Tasks

Current state of every piece of v0.1. This file is the honest inventory — if
something is marked done here, it works; if it is not, no amount of README
enthusiasm changes that.

Legend: **done** / **partial** / **todo**

## T0 Extraction and repository shape

| # | Task | State | Notes |
| --- | --- | --- | --- |
| T0.1 | Move the web application into `web/` | done | 73 source files, Svelte 5 + Vite + Tailwind 4 |
| T0.2 | Runtime config layer replacing hardcoded internals | done | `web/src/lib/config.ts`, fetched before mount |
| T0.3 | Strip every internal string from the app source | done | Verified: no site URL, company name, project key, team label, or internal path in `web/` |
| T0.4 | Generalize built-in views to tenant-neutral axes | done | Presets now use only `status_category`, `unassigned`, `reopened`, `stale`, `updated_from` |
| T0.5 | Generalize the media-URL allowlist to the configured API base | done | Security-relevant: still an exact-shape whitelist |
| T0.6 | Build config at repo root (`package.json`, `vite.config.ts`, `tsconfig.json`) | done | Vite root is `web/`, output `dist/app` |
| T0.7 | Governance and license files | done | Apache-2.0 |
| T0.8 | Specs, contracts, and architecture docs | done | This directory plus `docs/` |
| T0.9 | Remove or plugin-ize the cut internal surfaces still present in the UI | todo | `PrList`, `DeployTimeline`, `QaImpact` render nothing without data; they should move behind a plugin boundary or be deleted |
| T0.10 | Translate the UI to English with the current copy as a locale | todo | **Release blocker for a public launch.** The UI is Korean-only, which caps adoption regardless of everything else |

## T1 Storage

| # | Task | State |
| --- | --- | --- |
| T1.1 | Schema and migrations per `data-model.md` | todo |
| T1.2 | WAL mode, sane pragmas, single-writer discipline | todo |
| T1.3 | FTS5 table and rebuild-on-sync path | todo |
| T1.4 | `saved_views` / `watches` / `favorites` plus `scry export` | todo |
| T1.5 | Schema-version check on open, refusing a newer database | todo |

## T2 Jira connector

| # | Task | State |
| --- | --- | --- |
| T2.1 | REST client with Basic auth, retry, and backoff | todo |
| T2.2 | Full sync with token pagination and explicit field lists | todo |
| T2.3 | Incremental sync with watermark and overlap window | todo |
| T2.4 | Changelog paging for long histories | todo |
| T2.5 | Comment paging | todo |
| T2.6 | Reconcile pass for deletion detection | todo |
| T2.7 | Derived field computation | todo |
| T2.8 | Configurable field mapping and body fields | todo |

## T3 Server

| # | Task | State | Notes |
| --- | --- | --- | --- |
| T3.1 | `scry serve` with static UI, `config.json`, `/healthz` | done | `cmd/scry/main.go` |
| T3.2 | Loopback-only bind with an explicit `--allow-remote` escape | done | The mirror has no auth; this is the only thing protecting it |
| T3.3 | `bootstrap` and `delta` from SQLite, with ETag | todo |
| T3.4 | `detail` assembly | todo |
| T3.5 | `search` over FTS5 | todo |
| T3.6 | Attachment content proxy | todo |
| T3.7 | Saved views and watches endpoints | todo |
| T3.8 | Deferred endpoints returning a clean `404` | todo |

## T4 Write-through

| # | Task | State |
| --- | --- | --- |
| T4.1 | Credential store at `0600`, verified against `/myself` | todo |
| T4.2 | Transitions (list and execute) | todo |
| T4.3 | Comment with mentions and attachments | todo |
| T4.4 | Attachment upload | todo |
| T4.5 | Assignee change | todo |
| T4.6 | Field edit against a configured allowlist | todo |
| T4.7 | Issue creation and create-meta | todo |
| T4.8 | Re-read and mirror refresh after every write | todo |

## T5 Agent access

| # | Task | State |
| --- | --- | --- |
| T5.1 | Documented schema | done (`data-model.md`) |
| T5.2 | `scry sql` read-only query path | todo |
| T5.3 | `scry status --json` | todo |
| T5.4 | MCP server | todo (post-v0.1) |

## T6 Demo and fixtures

| # | Task | State | Notes |
| --- | --- | --- | --- |
| T6.1 | Jira seeding tool | done | `tools/seed-demo-jira.py`: dataset-driven or generated, plus `--repair-states` |
| T6.2 | Public demo site populated | done | 519 issues across three fictional products. Categories: 209 todo / 144 in progress / 166 done. Status-change depth 0–7 per issue. 95 reopen transitions, 339 issues with comments, 264 assigned, 61 link edges |
| T6.3 | Authored (non-templated) issue bodies | done | 210 of the 519 are hand-authored: 210/210 unique summaries, 642/642 unique paragraphs, 339/339 unique comments. The other 309 are procedurally generated and visibly more repetitive in the detail panel |
| T6.8 | Demo assignee display names | blocked | Four accounts are assigned across the site, but three show an email local part. Jira Cloud refuses to let an admin set `displayName` for a non-managed account, so each invitation has to be accepted and the name set by its holder. Blocks public screenshots |
| T6.4 | `scry snapshot` with timestamp spreading and volume scaling | todo |
| T6.5 | `examples/demo.db` committed, credential-scanned | todo |
| T6.6 | `scry demo` serving the bundled snapshot | todo |
| T6.7 | 10k-issue benchmark fixture for the latency target | todo |

## T7 CI and release

| # | Task | State |
| --- | --- | --- |
| T7.1 | CI: Go build and vet, frontend typecheck and build | done |
| T7.2 | Go tests once there is Go logic to test | todo |
| T7.3 | Browser smoke test against the demo snapshot | todo |
| T7.4 | Secret and internal-string scan in CI | todo |
| T7.5 | Dockerfile and container build | todo |
| T7.6 | Release process and signed binaries | todo |

## Critical path to something usable

T1.1 → T1.2 → T2.1 → T2.2 → T2.7 → T3.3 → T3.4. At that point the UI reads real
mirrored data and the tool is worth installing. Writes (T4) and search (T1.3,
T3.5) follow. T0.10 gates any public launch.
