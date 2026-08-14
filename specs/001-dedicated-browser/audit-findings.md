# Bug audit — 2026-08-14

Five read-only grok tracks (store/sync, server, web, cli/desktop, and a
cross-cutting identity sweep) against `610e350`, triggered by the account-id
person-filter bug that reached us as PR #1. Every finding below carries a
file:line and a FAIL-first test sketch in the track reports (session
scratchpad `audit/t1–t5`). The lead re-verified each P0/P1 from source
before listing it here. `[verified]` = lead read the cited lines.

Severity: **P0** data loss / credential exposure / security · **P1** wrong
answer or silent omission · **P2** edge UX / staleness.

## The headline

PR #1 closed the account-id bug on **one** surface (the web issue filter).
The same class — *an optional Jira field used as identity/key* — is still
open on at least seven other surfaces (T5). Fixing them as a set is the
structural close; patching one more surface at a time is how this recurs.

## P0 / security

- **S1 [P0][verified] `--profile ../..` escapes the home dir.**
  `config.DirFor` joins the raw profile name with no allowlist
  (`internal/config/config.go:222-232`), and `store.Open` / `Config.Save`
  `chmod 0700`/`0600` the target. `GADAK_PROFILE=../../.ssh gadak init`
  writes a token-bearing `config.json` into `~/.ssh` and chmods it; `../..`
  chmods `$HOME`. Env alone triggers it. (T4)
- **S2 [P1→P0 posture][verified] browserGuard misses `/config.json` and
  `/api/v1/workspaces`.** The Host/Origin guard runs only inside
  `server.Handler.ServeHTTP` (`internal/server/server.go:88-97`); the
  serve mux registers `/config.json`, `/healthz`, `GET /api/v1/workspaces`,
  and `/w/*/config.json` outside it (`cmd/gadak/workspaces.go:17-44`). A
  DNS-rebind page reads the site URL, project keys, group labels, and every
  profile's name+site — contradicting SECURITY.md's rebind promise. Issue
  bodies stay protected. (T2)

## P1 — the identity class (T5 + siblings in T1/T2/T3)

These are one bug wearing several hats: email/display-name used where an
account id belongs. Group them into one Go round + one web round.

- **I1. JQL people resolution is email-bodied.** `assignee = currentUser()`,
  imported "My open issues" filters, and `assignee = <accountId>` all
  resolve through email and drop roster rows without one
  (`internal/jql/resolve.go`, `match.go`, `types.go` has no `ReporterID`;
  `internal/server/jql.go:54-95`). On an email-hidden site the pasted/saved
  clause returns **0**. Web chips (id-first) and CLI/JQL (email) give
  different answers for the same input. (T5, T2-P2, T1)
- **I2. `assignee is EMPTY` counts email-less assignees as unassigned**
  (`internal/jql/match.go:17-18`). (T5)
- **I3. `gadak views save --jql 'assignee = currentUser()'`** stores the
  literal token; the saved view is empty in the web
  (`cmd/gadak/views.go:216-223`, the help example itself). (T5)
- **I4. Saved-filter import** compiles people to emails on the sync side too
  (`internal/sync/filters.go:22-89`) — the store/sync half of I1. (T1)
- **I5. Member directory refuses email-less people**
  (`internal/server/read.go:841-844`); ⌘K search, the person panel, avatars,
  and group mapping lose them even though the row carries `assignee_id`.
  `addMember` is the single owner to fix. (T5, T3, T2-P2)
- **I6. `gadak init` never stores `account_id`/`tokenOwner`/`tokenVerifiedAt`**
  (`cmd/gadak/main.go:293-306`) though the web onboarding path does. CLI-only
  installs get empty "my issues", disabled mentions, and self-events leaking
  into the feed. Small, self-contained — good first fix. (T4, T5)
- **I7. changelog/attachment authors store display-name only** (no
  `author_id` column, `internal/store/schema.go:93-115`,
  `internal/sync/sync.go:631-657`); feed self-exclusion is byte-equality on
  the name → same-name people collide, renames leak. Needs schema **v19**. (T5, T1)
- **I8. Doc By-author groups on display name** though `items.author_id`
  exists and is dropped from the PageLite payload
  (`internal/store/read.go:361-419`, `web/.../pages.svelte.ts`). (T5, T3)
- **I9. JQL/`create` numeric ids and localized names for
  status/type/priority.** `status = 3` matches the stored name, not
  `status_id` (`internal/jql/compile.go:244-266`, `match.go`); web
  `filterIssues` keys on the localized name too though `status_id` is on the
  wire (`web/.../filters.svelte.ts:617-626`, client `IssueLite` omits
  `status_id`); `POST create/` sends `priority:{name}` not `{id}`
  (`internal/server/write.go:607-626`). An English-saved team view is empty
  for a Korean teammate. (T5, T3, T2)

## P1 — sync / cache correctness

- **C1. Discovery reingest doesn't bump `sync_state.version` or
  `items.synced_at`** (`internal/store/fields.go:66-196`), so an open client
  304s and never sees custom fields until another write. (T1)
- **C2. Confluence comment-only edits are invisible** — the comments-only
  pass decision 0006 required was never implemented
  (`internal/sync/confluence.go:197-221`). New comments never reach the
  mirror until the page body changes. (T1)
- **C3. Confluence incremental commits the watermark per batch** across
  space chunks (`confluence.go:121-149`); a mid-pass failure skips later
  spaces' in-window history forever. (T1)
- **C4. UpsertPages always bumps version** (`internal/store/write.go:259-296`)
  + the 5-min overlap re-fetches the newest page every Watch cycle → a quiet
  wiki invalidates the bootstrap ETag every 60s. (T1)
- **C5. Profile switch on one origin mixes mirrors.** IndexedDB + localStorage
  on `/` are keyed by workspace path, not site/profile; a delta after
  re-init upserts site B into site A's cached pool
  (`web/src/lib/db.ts:16-22`, `stores/issues.svelte.ts:158-200`,
  `internal/server/read.go:261-275` ETag). (T3)
- **C6. Comment drafts ignore the workspace prefix**
  (`web/.../CommentComposer.svelte:23-38`) → a draft can post to the wrong
  site. (T3)
- **C7. Delta / docs reload never invalidate detail caches**
  (`web/.../issues.svelte.ts:274-302`, `detail-cache.svelte.ts` claims an
  invalidation with no caller) → an open panel stays stale. (T3)
- **C8. Issue→page `item_refs` scans flattened text**, dropping ADF link
  marks / inlineCards (`internal/store/refs.go:69-97` via `PlainText`); the
  page→issue direction scans raw ADF and works. (T1)
- **C9. `SyncIssue`/`SyncPage` don't tombstone a vanished key**
  (`internal/sync/one.go:43-96`); a deleted issue lingers until the hourly
  reconcile. (T1)
- **C10. changelog `FieldID` fallback lowercases a localized field name**
  (`internal/sync/sync.go:643-657`); a Korean account with no `fieldId`
  stores `상태` and Derive misses the reopen → `reopen_count=0`. (T1)

## P1 — CLI / desktop robustness

- **D1. `~/.scry` + existing `~/.gadak` silently abandons the old mirror**
  (`internal/config/config.go:200-214`); looks like the mirror vanished. (T4)
- **D2. Empty `GADAK_*` blocks a real `SCRY_*`** (`identity.go:23-28`); a
  blank export hides the legacy token. (T4)
- **D3. Unknown `--profile` creates an empty home** instead of erroring
  (`config.go:302-312`, `cmd/gadak/main.go:81-87`); a typo reads as a valid
  empty profile. (T4)
- **D4. `install-service` is one global unit** (`cmd/gadak/service.go:14`);
  a second profile's install silently replaces the first. systemd
  `enable --now` failure still exits 0. (T4)
- **D5. `views open` focuses whichever app is running** (`open -a`, no `-n`,
  `views.go:410-425`); the `--env GADAK_PROFILE=` window never starts, so a
  different profile's focus never lands. (T4)
- **D6. `uifocus.Write` doesn't create the profile dir**
  (`internal/uifocus/uifocus.go:37-47`); first `views open` on a fresh
  profile ENOENTs. (T4)
- **D7. ui-focus not profile-scoped on `/w/*`** — the handler reads the
  process profile's file (`internal/server/focus.go:11-21` → `config.Dir()`);
  workspace tabs can't be focused and can steal the primary's file. This is
  the same defect the wave's Track G G5 already targets. (T2, T4, census)
- **D8. Workspace Watch loops start only at serve boot**
  (`internal/workspace/workspace.go:246-298`); a profile credentialed later
  never incrementally syncs. (T2)
- **D9. Attachment cache keyed by attachment id only**
  (`internal/server/read.go:553-559`); a same-profile site switch serves the
  wrong bytes, and any issue key can fetch any cached id. (T2)
- **D10. POST attachments/ swallows a failed mirror re-read** and returns 200
  (`internal/server/write.go:355-375`) instead of the contract's 502. (T2)
- **D11. `--spaces all` omits personal spaces** while help says "everything";
  `--spaces` upper-cases `~accountId` personal keys so they 404
  (`cmd/gadak/main.go:283-346`, `133`). (T4)
- **D12. `team import` isn't atomic** — views land, then a `Save` failure
  leaves a partial import that a retry skips (`cmd/gadak/team.go:222-234`). (T4)

## P1 — sort / list

- **L1. `priority_rank == 0` (unset) sorts as most-urgent**
  (`web/.../filters.svelte.ts:774-782`); untriaged issues float above
  Highest. The comment says "null sorts last"; the wire value is 0. (T3)

## P2 — recorded, not this wave's core

Paging on missing `total` stops early (T1-P2, T2-P2); search stops on an
empty `nextPageToken` with `isLast:false` (T2-P2); MediaRef filename hits a
non-REST path (T2-P2); Origin case-sensitivity false-rejects mixed-case
localhost (T2-P2); `users/` emits `email:""` (T2-P2); desktop updater is
digest-only + delete-then-rename (T4-P2); `uifocus.Take` deletes a
concurrently-rewritten file (T4-P2); custom user-field + QA editor keyed on
display name (T5-P2); `cloned_from` needs an English link-type name (T5-P2);
`gadak status --json` emits `profile:""` vs `"default"` elsewhere (T4-P2);
docs-index network error looks like "zero documents" (T3-P2); in-flight
delta can briefly revert an optimistic write (T3-P2); `me.group`
case-sensitivity misses the first-run preset (T3-P2). Full detail per track.

## Proposed fix rounds (file-boundary disjoint; grok parallel)

1. **B-security** (`internal/config` + `cmd/gadak` + `internal/server` mux):
   S1 profile-name allowlist, S2 guard the top-level routes. Highest
   priority; small, well-scoped, FAIL-first each.
2. **B-identity-go** (`internal/jql`, `internal/sync/filters.go`,
   `cmd/gadak/{main,views}.go`, schema v19): I1–I4, I6, I9(server/JQL), I7.
   Sequence **after** Track G merges (same files).
3. **B-identity-web** (`web/src/`): I5(client side), I8, I9(web), L1, C5, C6,
   C7. Sequence after Track W merges.
4. **B-sync** (`internal/store`, `internal/sync`): C1–C4, C8–C10.
5. **B-cli** (`cmd/gadak`, `internal/config`, `internal/uifocus`,
   `internal/workspace`, `internal/server/{focus,write,read}.go`): D1–D12
   (D7 folds into Track G's G5).

Each round carries the three layers: structural close (single owner, not a
param patch), a FAIL-first numeric/behavioral gate, and a debug surface.
