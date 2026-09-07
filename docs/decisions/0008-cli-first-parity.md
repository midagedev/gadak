# 0008 — CLI-first parity

Status: accepted
Date: 2026-08-18

## Context

gadak has two surfaces over one store (decision 0005, reversed 2026-08-13):
the web UI and the desktop window around it for people, and CLI / SQL / MCP
for agents and scripts. The mirror is a cache; Jira is the record.

A 2026-08-17 question was whether the CLI should stay a separate tool from
the app, because the product had started to look app-centered. The concrete
worry was not a second binary. It was the skill (`gadak skill install`
embeds `skills/gadak/SKILL.md`) inserting an app hop in the middle of a
headless path.

What the skill actually does: the only place it names the app is the
presentation verb `gadak views open`. That verb writes a one-shot ui-focus
hash, prints a `gadak://` deeplink whether or not anything is listening, and
opens a `serve` tab when one is there (`cmd/gadak/views.go`). `--no-open`
stops the launch. Read, write, `gadak config`, and install verbs in the
skill never start the app.

## Decision

Three rules. A capability that breaks one of them is a defect, not a taste
call.

1. **Every capability with user value is executable headless** — CLI, SQL,
   or the loopback API. The app adds presentation. It does not own the
   capability.
2. **No skill step assumes the app is running.** A verb that opens the app
   is for showing. It always has a headless fallback (print the hash and
   the deeplink; `--no-open`; a `serve` tab instead of Gadak.app).
3. **An app button is a wrapper around a CLI verb or an API.** Logic that
   exists only in the app is a defect. Exception: surfaces that are desktop
   by nature — the in-app browser pane, the pasteboard, OS menus, and
   install streaming (`/desktop/*` in `desktop/main.go`).

The Settings dialog, `gadak config`, and `PUT /api/v1/issues/settings/`
share the validators in `internal/config/settings.go`. They must not grow
separate rules for the same field. (The PUT document is a subset of the
`gadak config` catalog: `notify`, `updateCheck`, and `attachmentCacheMB`
are CLI-only. Credentials stay on `gadak init` / `credential/`.)

This is not a second binary and not a reversal of 0005. The web UI stays
the human surface. The rule is that nothing the human can do is locked
inside that surface.

## Census (2026-08-18)

Every endpoint `web/src` calls, against a CLI verb (or SQL / API when that
is the headless path). `apiBase` is `/api/v1/issues/`
(`internal/server/server.go`, `web/src/lib/config.ts`).

| Capability | App path | Headless path | Verdict |
| --- | --- | --- | --- |
| Load the issue pool | `web/src/stores/issues.svelte.ts` (`getBootstrap` / `getDelta` → `api.ts` `bootstrap/`, `delta/`) | `gadak sql`; `gadak search`; `gadak issue` | presentation of the same store |
| One issue, hydrated | `web/src/lib/detail-cache.svelte.ts` (`getDetail` → `{key}/detail/`) | `gadak issue <KEY>` | wrapper |
| Attachment bytes | `web/src/lib/adf.ts` (`{key}/attachments/{id}/content/` as media `src`) | `gadak issue` lists them; bytes live under the profile `attachments/` dir | presentation |
| Full-text search | `web/src/stores/filters.svelte.ts` (`search/` ) | `gadak search` | wrapper |
| JQL → chips | `web/src/components/list/SearchBox.svelte` (`POST jql/`) | `gadak search --jql`; `gadak views save --jql` | wrapper (`internal/jql`) |
| Chips → JQL | `web/src/components/list/FilterBar.svelte` (`POST jql/emit/`) | the same API (`internal/jql`). `gadak search --jql --emit` emits canonical JQL from a JQL string, not from a chip document | API wrapper. CLI emit is the other direction |
| Apply a view / key list | `web/src/App.svelte` (`GET ui-focus/`) | `gadak views open` (hash + deeplink; `--no-open`; serve tab) | presentation verb, headless fallback |
| Wiki page list / body | `web/src/stores/pages.svelte.ts` (`pages/`, `pages/{id}/`) | `gadak sql` on `pages`; `gadak search` includes pages | wrapper |
| Re-read one issue / page after the in-app browser edited Jira | `web/src/lib/browse.svelte.ts` (`POST {key}/resync/`, `POST pages/{id}/resync/`) | `gadak sync` (eventual); no single-key CLI | API exists; the trigger is the desktop browse pane |
| Comments by author | `web/src/stores/person.svelte.ts` (`people/{id}/comments/`) | `gadak sql` on `comments` | wrapper |
| Personal feed + read receipts | `web/src/stores/me.svelte.ts` (`GET feed/`, `POST feed/read/`) | `GET/POST` on the API. Events are computed at query time (not a SQL table). Receipts land in `feed_reads` (`gadak sql` can read them). No `gadak feed` verb (`cmd/gadak/`: no `feed/read`) | API wrapper. No CLI verb |
| Web Push config / subscription | `web/src/stores/push.svelte.ts` (`notifications/config/`, `notifications/subscription/`) | none — server does not register these routes (`internal/server/server.go`; `TestDeferredEndpointsAre404`) | **cut surface** (404; UI hides it). Not a live gap |
| List / create team views | `web/src/stores/views.svelte.ts` (`GET/POST views/`) | `gadak views` / `gadak views save --jql` | wrapper |
| Delete a team view | `web/src/stores/views.svelte.ts` (`DELETE views/{id}/`) | the same API; `gadak views` is `list\|show\|open\|save` only (no `delete` in `cmd/gadak/views.go`) | API wrapper. No CLI verb |
| **Personal views** | `web/src/stores/views.svelte.ts` `addPersonal` / `removePersonal` (localStorage only) | **없음** — `cmd/gadak/` has no `addPersonal` / `personalViews`; no HTTP route | **gap** (see below) |
| Watches | `web/src/stores/watches.svelte.ts` (`GET/PUT/DELETE watches/`) | API; `gadak export` / `import` dump the table. No `gadak watch` verb | API wrapper. No CLI verb |
| Favorites | `web/src/stores/favorites.svelte.ts` (`GET/PUT/DELETE favorites/`) | same as watches | API wrapper. No CLI verb |
| Credential read / replace / clear | `web/src/stores/write.svelte.ts` (`credential/`) | `gadak init`; not `gadak config` | wrapper |
| First-run connect | `web/src/components/shell/Onboarding.svelte` (`PUT onboarding/connect/`) | `gadak init` | wrapper (wizard is presentation) |
| Live project list | Onboarding + Settings (`GET projects/available/`) | `gadak init --projects`; `gadak api` | API + init. Picker is presentation |
| Start sync / progress / runs | `web/src/lib/sync-now.ts`, `SidebarNav.svelte` (`POST sync/`, `GET sync/progress/`, `GET sync/runs/`) | `gadak sync`, `gadak status`, `gadak sql` on `sync_state` | wrapper |
| Transitions | `StatusTransition.svelte`, `BulkBar.svelte` (`GET {key}/transitions/`, `POST {key}/transition/`) | `gadak transition` | wrapper |
| Comment + upload | `web/src/stores/write.svelte.ts` (`POST {key}/comment/`, `POST {key}/attachments/`) | `gadak comment`, `gadak attach` | wrapper |
| Assignee / labels / priority / summary | `write.svelte.ts` | `gadak assign`, `gadak edit` | wrapper |
| QA / custom field inline edit | `write.svelte.ts` (`GET {key}/editmeta/`, `PATCH {key}/fields/`) | the same API. `gadak edit` does not send `fields/` (`cmd/gadak/`: no `setIssueField`) | API wrapper. No CLI verb |
| Create issue + create-meta | `NewIssueDialog.svelte`, `write.svelte.ts` | `gadak create` | wrapper |
| User search (assignee picker) | `AssigneePicker.svelte` (`GET users/`) | the same API; `gadak assign` resolves an email through Jira `SearchUsers` (`cmd/gadak/agent.go`) | wrapper |
| Settings read / replace | `SettingsDialog.svelte` (`GET/PUT settings/` = `/api/v1/issues/settings/`) | `gadak config list\|get\|set` | wrapper; shared validators in `internal/config/settings.go` |
| Live Confluence space list | `SettingsDialog.svelte` (`GET settings/spaces/`) | `gadak init --spaces`; `gadak api GET /wiki/api/v2/spaces` | API + init. Picker is presentation |
| Write-meta prefetch | `write.svelte.ts` (`GET meta/write/`) | `gadak transition` / `gadak create` fetch their own meta | presentation cache |
| Local history | `history.svelte.ts`, `me.svelte.ts` (`GET history/`, `POST history/visits/`, `POST/PATCH history/searches/`) | `gadak sql` on `local.visits` / `local.searches` (write is a view side-effect) | read is SQL; write is instrumentation |
| Workspace list | `SidebarNav.svelte` (`GET /api/v1/workspaces`) | `gadak profiles` | wrapper |
| Identity probe | `me.svelte.ts` (`GET /api/v1/auth/me/`) | the same API (`GET /api/v1/auth/me/`). `gadak doctor --json` only says whether email is `configured` | API wrapper |
| Runtime `config.json` | `web/src/lib/config.ts` | `gadak config list` | same file, different encoding |
| System browser | `web/src/lib/desktop-links.ts` (`POST /desktop/open`) | `gadak open <KEY>` | **justified desktop** (`desktop/main.go`) |
| Pasteboard | `web/src/lib/copy-text.ts` (`POST /desktop/clipboard`) | a terminal already has a clipboard | **justified desktop** |
| Integrations list / install stream | `web/src/lib/integrations.ts` (`GET /desktop/integrations`, `POST /desktop/integrations/{id}/install`) | `gadak skill install`, `gadak mcp install`, `gadak raycast` | **justified desktop** (streams those same verbs) |
| In-app Jira/wiki pane | `browse.svelte.ts` (`/desktop/browse`, `/state`, `/activate`, `/close`, `/frame`) | `gadak open` (system browser) | **justified desktop** |

Skill check (`skills/gadak/SKILL.md`, the bytes `gadak skill install`
embeds): reads (`status`, `sql`, `search`, `issue`), writes (`create`,
`attach`, `edit`, `comment`, `transition`, `assign`), settings
(`gadak config`), and setup (`init`, `profiles`, `doctor`) are all
process-shaped. The app is named only under "Show, don't paste" /
`gadak views open`, with `--no-open`, the printed deeplink, and a serve
tab as fallbacks (`cmd/gadak/views.go`).

### Unresolved gap

**Personal views persist only in the browser.**
`web/src/stores/views.svelte.ts` writes them to `localStorage`
(`addPersonal` / `removePersonal`). There is no HTTP route and no CLI
verb (searched `cmd/gadak/` for `addPersonal` and `personalViews`: no
hits). `gadak export` dumps server `saved_views`, not this key. Team
views on `POST/DELETE views/` are fine.

This is the view-editing surface that is not a wrapper. The onboarding
wizard is not a gap: `PUT onboarding/connect/`, `GET projects/available/`,
`PUT settings/`, and `POST sync/` are `gadak init`, `gadak config set
projects`, and `gadak sync`.

Do not treat "no CLI verb, but the API exists" as a gap under rule 1.
Those rows are recorded so a later CLI can be a thin wrapper, not because
the capability is locked in the app.

## Consequences

- A new human-facing capability lands as a CLI verb or an API first. The
  app wraps it. The skill is not updated with an app-only step.
- `gadak views open` stays the one presentation verb. Its fallbacks
  (deeplink, `--no-open`, serve tab) stay.
- Settings, `gadak config`, and `PUT /api/v1/issues/settings/` keep one
  validator table (`internal/config/settings.go`). Field-set differences
  are listed in `docs/CONFIGURATION.md`, not re-derived per surface.
- The personal-views row is a backlog item, not a silent exception.

## Addendum (2026-08-21)

GDK-513 shipped `gadak edit --field alias=value` (and `gadak create --field`).
The census row at `:83` ("`gadak edit` does not send `fields/` … No CLI verb")
is superseded: the CLI now PATCHes `{key}/fields/` through the origin. REST
still has no issue-link endpoint (`internal/server/server.go` has no
`…/link/` route); `gadak link` is CLI-only.

## Addendum (2026-09-07) — pasteboard through the wails runtime, not a route

The Pasteboard row above names `POST /desktop/clipboard`. That route is gone
(GDK-1470): the desktop branch of `web/src/lib/copy-text.ts` imports the
`/wails/runtime.js` module the app already injects into every page it serves
and calls `Clipboard.SetText`, which is the same Go call the route used to
make. The row's verdict — **justified desktop**, a terminal already has a
clipboard — is unchanged; only the transport moved from a hand-rolled HTTP
handler to the framework's own binding. One narrowing rides along and is
written at the top of `copy-text.ts`: the runtime binding discards the
pasteboard's own bool, so `false` on desktop now means the bridge refused,
not that the pasteboard did.
