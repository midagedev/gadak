# gadak mobile — design

The phone companion to a desktop `gadak serve`. One user: the developer who
runs gadak, away from the desk. The job is triage-glance, not project
management: **read fast, write one line, put the phone away.** Every decision
below answers to that sentence.

Design language in one line: **a pocket ledger** — the paper-and-indigo
identity of gadak (web tokens), carried by density, a serif ledger heading,
mono issue keys, and one signature mark: the *ink spine*, a status-colored
edge that follows an issue from its row into its detail page.

---

## 1. Users and jobs

| Job | Screen | Budget |
|---|---|---|
| "What's on my plate?" | Issues (default tab), scope **Assigned to me** | glance — no taps |
| "What moved while I was away?" | Issues, glance strip above the queue (GDK-871) | 0 taps |
| "Show me the view I named at the desk" | Issues, scope picker on the heading | 1 tap |
| "What's in this space / what changed in the wiki?" | Issues, Documents section of the same picker | 1 tap |
| "Where was that issue or page about X?" | Search | 2 taps + typing |
| "What happened here?" | Detail (comments-first) | 1 tap from any row |
| "What does this page say?" | Page detail (body, then comments) | 1 tap from a doc row or a search hit |
| "One-line reply / push the status" | Detail composer · transition sheet | thumb only |
| "Is this thing still connected?" | Pairing tab | rarely visited, always honest |

Non-jobs (deliberately absent in v1): creating issues, editing fields,
attachments, **wiki writes** (create / edit / comment on a page),
a feed *tab* or notification center (the glance strip above the queue is
the phone's whole feed surface — GDK-871; a fourth tab would be a desktop
column transplanted), JQL. The desktop owns those. History timeline and plugin
enrichments are read on the desktop too — the phone shows description,
comments, linked issues, and a page's plain-text body. **Authoring** a view is a
non-job in the same sense: the phone consumes the names the desk wrote and
never `POST`s one (iOS Mail does not author smart mailboxes either).

## 2. Screen map & navigation model

Three tabs + one push layer. No hamburger, no drawer, no nested stacks.

> **This section is being replaced — GDK-902.** The tab model below is what
> the code does today and is described here honestly for that reason, but it
> has been decided against: tabs are a *fixed* set of column owners, and
> gadak's owners are ones the user makes (saved views, Jira filters, spaces,
> dashboards, a shell). The replacement is the desktop's own model — one
> column, one owner at a time, the palette changes the owner
> (`web/src/lib/commands.ts:140`) — with the palette as the list's head
> rather than a separate screen. Do not extend the tab model; do not write
> new copy against it.

**The tab is the object, the heading is the scope.** The desktop has no name
for its list screen — its main column is titled by the current view's name.
The phone adopts that model exactly: the tab says *Issues*, and the `<h1>`
says whichever scope is showing (Assigned to me, All open, or a name the
developer typed at the desk). Linear, Gmail and Apple Mail all title the list
with the current scope; none of them names it after a metaphor. That is why
there is no "Queue" and no Mine/All toggle: both were words the phone
invented, and the heading tells the truth without either.

```
┌────────────────────────────┐
│ PairGate (only when        │  not a tab — replaces the app
│ unpaired / token rejected) │  until pairing succeeds
└────────────────────────────┘
┌─────────┬─────────┬────────┬────────┐
│ Issues  │ Search  │ Shell  │ Pairing│  tab bar, bottom, always visible
└────┬────┴────┬────┴────────┴────────┘  (Shell only once paired, §10)
     ├─► Scope picker (bottom sheet, opened by the heading)
     │     My issues · Built-in · My views · Jira filters · Documents
     └────► Detail (push, slides over tabs) — issue or page
                └─► Transition sheet (issues only)
                └─► linked issue / mentioned issue (replaces, back → list)
```

**Entry / exit contract** (dead-end rule: every screen has an explicit way
out; system back = the same edge):

| Screen | Enter from | Exit |
|---|---|---|
| PairGate | boot (no pairing) · token rejected | successful pair → Issues |
| Issues | tab · boot default | tab bar |
| Search | tab | tab bar |
| Pairing | tab | tab bar |
| Shell | tab (present only once a terminal pairing is stored, §10) | tab bar |
| Scope picker | the Issues heading (44pt) | scrim tap · Cancel · picking a name |
| Detail | issue-row tap (Issues/Search) · linked-issue tap | ← back button (top-left, 44pt) → the tab that opened it |
| Page detail | doc-row tap (Issues/Search) · search page hit | ← back button (top-left, 44pt) → the tab that opened it |
| Transition sheet | status chip in Detail | scrim tap · Cancel · apply |

The picker's sections are the desk's own, in the desk's order: My issues
(Assigned to me) · Built-in views (All open) · My views · Jira filters ·
Documents. Documents is a section in this same sheet, never a fourth tab:
the whole-mirror plate is named **Updated** (`docs.tabUpdated`) — the desk's
all-documents surface is tabbed Viewed / Updated / Authors, and the phone
plate is `updated_at` desc, so that word is the honest name, not a second
"Documents" under the Documents heading. Then one row per `space_key`
(`space_name`, falling back to the key).

Tabs keep their scroll/query state across switches. Detail is a single layer:
opening a linked issue swaps the key in place (back still returns to the
originating tab — one tap out from anywhere, matching "put the phone away").

## 3. Design language

### 3.1 Color — web tokens are the single owner
`mobile/src/app.css` imports `web/src/app.css` and consumes only
`var(--color-*)`. No mobile hex. Dark mode is therefore free and identical in
temperament to desktop (paper → ink-dark, same 쪽빛 accent).

60/30/10: paper grounds (`bg-base/panel/elevated`) ~60, ink text ~30, one
accent thread (active tab, links, send button) + status inks ≤10. Status
color appears **only** where it is data: the ink spine, the status chip dot,
the transition sheet dots. Never as decoration.

### 3.2 Type — 4 sizes, 3 faces, each earning its place
Dimensions are mobile-owned (`@theme` override after the import):

| Token | Size | Use |
|---|---|---|
| `--text-micro` | 12px | metadata line, section labels, chips |
| `--text-body` | 16px | row summaries, paragraphs, **every input** |
| `--text-title` | 19px | issue title in Detail |
| `--text-heading` | 26px | ledger heading of each tab |

- Body = 16px is a **structural close for iOS input zoom**: no text style an
  input could inherit sits under 16px, so the zoom trigger cannot exist.
- Faces: `--font-display` (New York serif — already the product's display
  face) for the ledger headings and issue titles via the shared
  `.type-subject`; `--font-mono` for issue keys and counts (identifiers look
  like identifiers); `--font-sans` for everything else.
- The aesthetic risk, named: a serif ledger heading with a mono folio count
  ("Assigned to me ·42") on a *phone tool*. It is what makes a capture of this
  app unmistakably gadak and not a Tailwind template; if it reads as a
  newspaper gimmick in captures, the fallback is `.type-subject` on Detail
  titles only. The heading is also a control, so it carries a muted chevron
  after the count and measures 44pt — the padding that used to give the header
  its height now lives inside the button, and the row count below is unchanged.

### 3.3 Space & density
4pt grid (4/8/12/16/24/32), 16px screen gutter. Issue rows are **56px**
(≥44pt target, two lines: summary + meta). On the iPhone 17 Pro viewport that
yields 12–13 rows per screen — a two-digit ledger, not a card feed. No
cards, no shadows in lists; hairline `border-subtle` separators only. The
scope picker is the same dialect: section labels in the list's micro caps,
44pt rows, counts right-aligned in mono, no panel fills.

### 3.4 The signature: ink spine
Every issue row carries a 3px left spine in its `status_category` color
(new / inprogress / done tokens; reopened rows use the reopen token when
`reopen_count > 0`). On cream, `new` uses `--color-accent` (same 쪽빛
thread, darker than `--color-status-new`) via `--color-spine-new` so the
mark photographs as a column, not a whisper; dark mode keeps
`--color-status-new`. In Detail the same spine runs down the left of the
header block — the row visually *is* the page you landed on. This is the one
memorable mark; everything else stays quiet.

### 3.5 Motion
Two moves, both under 250ms, both disabled under `prefers-reduced-motion`:
Detail slides in from the right (200ms ease-out), sheets rise from the bottom
(240ms cubic). Nothing on the scroll path animates. No spinners — loading is
skeleton rows (paper rectangles), writes show state in the pressed control
itself ("Sending…" in the send button, progress dot in the transition row).

### 3.6 Vocabulary — the desktop catalog is the single owner
The same rule as §3.1, applied to words. `mobile/src/lib/i18n.ts` re-exports
`web/src/lib/i18n` and is the **only** file in the phone that reaches across
the tree; every screen calls `t(key)`. If the desk already has a word for a
thing, the phone does not author a second one — a different word for the same
thing *is* the defect, not a style preference.

The consequence is concrete: the tab is `doc.issues`, the default heading is
`personal.myAssignee`, the fallback heading is `view.allOpen.name`, the picker
sections are `personal.myIssues` / `sidebar.builtinViews` / `sidebar.myViews` /
`sidebar.jiraFilters` / `sidebar.docs`, the whole-mirror documents plate is
`docs.tabUpdated` (desk structure → phone row: Updated, not an invented "All
documents"), sync is `sidebar.syncNow`, and every sheet's dismiss is
`common.cancel`. Korean and Japanese therefore come free, exactly as dark mode
comes free from §3.1.

Strings with **no** catalog equivalent stay phone-authored and are listed as
such: the Search and Pairing tab labels, the offline and fallback notes, the
empty state, the picker's "Open on the desktop" refusal, and the pairing copy.
Adding a key is the desktop's edit, not the phone's — the phone never writes
into `web/src/lib/i18n/messages/`.

## 4. iOS contract (the §4 requirements, structurally closed)

1. **Safe area — single owner.** Only two files may mention
   `env(safe-area-inset-*)`: `app.css` (utility classes `.safe-top`,
   `.safe-bottom`) and nothing else. `Screen.svelte` applies `.safe-top` to
   every header; `TabBar.svelte` / Detail composer / sheets apply
   `.safe-bottom`. Screens receive an already-inset frame, so a touch target
   under the status bar cannot be authored.
2. **Vertical geometry.** No `vh`/`dvh` anywhere (lint-able: `grep -r "dvh\|100vh" src` is empty).
   The root is `#app { position: fixed; inset: 0 }`; the tab bar is a normal
   flex child of that fixed frame, so it sits on the physical bottom and
   cannot move when the keyboard animates. The Detail composer lifts with the
   keyboard via the VisualViewport API (measured, not assumed — see capture
   log).
3. **Horizontal overflow 0.** `#app { overflow: hidden }` backstop + every
   flex row uses `min-width: 0` + `truncate`. Meta lines are single-line
   ellipsis; chips never wrap out of the gutter.
4. **Density.** 56px rows → 12+ visible on one screen (measured in capture).
5. **No dead ends.** Table in §2; back button is 44pt and top-left per HIG.
6. **Every state designed.** Unpaired (PairGate with paste-first flow),
   syncing-first-time (skeleton rows), offline (banner with last-sync age,
   cached data stays usable), token-rejected (PairGate with explanation),
   empty plate (falls back to All open and says why, one tap to any other
   scope), a view the phone cannot honor (offered disabled, never silently
   painted as the full list), empty search (recent searches, never blank),
   write failure (inline in the control that acted, cached read state
   untouched).
7. **Dark mode** is the same code path — token flip only, verified with
   light/dark captures of every screen.
8. **44pt tap floor (GDK-867).** Every control — not only the back button —
   measures at least 44pt on its bounding box. `--spacing-control` (44px) is
   the single owner (`button { min-height }` in `app.css`). `--spacing-control-sm`
   (32px) is the visual chip only and must not set a button's tap size.
   Growing the hit area without growing a filled chip is the move that keeps
   §3.3 (12+ rows) and this floor both true.

Thumb zone: the three write actions (compose, send, transition) and the tab
bar all live in the bottom third. The top third holds only reading and the
back button. Status is *visible* in the header as data (spine + chip dot);
the transition *action* lives with compose and send.

## 5. Data & protocol decisions

- **Snapshot model.** `GET issues/bootstrap/` with `If-None-Match` is the
  read path; the full lite snapshot is the phone's mirror. Cached in
  `localStorage` (guarded try/catch — over-quota mirrors simply skip the
  cache and refetch). Sync on boot, on foreground (`visibilitychange`), and
  on freshness-chip tap. The view list is cached the same way and under the
  same guard, so a restored scope paints its own name on the first frame
  instead of flashing the default. `issues/delta/` is a v2 optimization once a mirror
  large enough to hurt is measured.
- **Issues** is one list under one scope, sorted `priority_rank` asc (unset
  ranks last), then `updated_at` desc, grouped by priority with the display
  name as the section label (display-only; logic never keys on it). Scopes:
  *Assigned to me* (assignee_id = me.account_id, else email match, open only)
  · *All open* · the desk's saved views · the desk's imported Jira filters,
  read from `GET issues/views/` alongside bootstrap. The last-used scope id
  lives in `localStorage`; a scope that has since been deleted falls back
  silently, and an empty *Assigned to me* — or no identity at all — falls back
  to *All open* **and says why**.
- **Applying a view is in-memory over the snapshot.** The phone honors only
  the axes an `IssueLite` can answer: `status_category` (+`_not`),
  `assignee_email` (+`_not`, account id first then email), `unassigned`,
  `issue_type`, `priority`, `jira_project` (+`_not`). `issue_type` and
  `priority` compare against the stored id first and the stored name second —
  the desk's own `matchesIdFirst` contract, which is consuming a stored value,
  not keying logic on a display name. Any other axis a view sets — labels,
  actor, reporter, components, fix_versions, team_group, severity, qa_*,
  deploy_*, source_project, date ranges, the text query, dynamic fields — and
  any Jira clause the desk's importer left in `unsupported[]` means the phone
  **cannot** honor that view: the row is offered disabled with a reason,
  never painted as the full list under someone else's name (decision 0007).
  Sort and grouping stay the phone's priority sections in this round; a view's
  `display` block is deliberately ignored.
- **Picker counts** (GDK-886) are one in-memory pass per row, taken when the
  sheet opens — never on the list's scroll path. A view matching zero issues
  shows `0` and stays selectable; a disabled row shows no count. The endpoint
  returns no counts and must not be extended to.
- **Pages.** `GET issues/pages/` runs in `sync()` beside the view list,
  cached as `gadak.pages`, dropped on unpair. No `If-None-Match` (the desk's
  `getPages()` does not send one). An empty list or a fetch error hides the
  Documents section — no error chrome, no empty section. Applying a documents
  scope is in-memory over that list, sorted `updated_at` desc. Page detail is
  `GET issues/pages/{key}/`; the phone paints `body_text` (plain text, filled
  from ADF by the same walker FTS indexes) and never ADF. A page is read for
  its body, so comments follow the body here (issue Detail stays comments-first).
  No composer, no page writes.
- **Search** is local-first: instant key/summary substring over the snapshot
  while typing, merged with debounced server `issues/search/` results (body
  and comment matches the snapshot cannot see). Page hits arrive with that
  reply in `pages[]` and paint a Documents section below the issue rows;
  they are not merged into one undifferentiated list.
- **Writes** (`comment`, `transition`) go through origin and re-sync after —
  the app never mutates the snapshot except as an optimistic overlay that a
  failed write rolls back. Frequent writes get no confirm dialog (§7 of
  UX_PRINCIPLES); the one destructive rarity, Unpair, uses the house two-step
  arm (tap → armed red "Tap again" → 3s auto-disarm).
- **Serve-scope pairing.** A serve-scope token opens the whole mirror REST
  (everything the local web UI can call); only the origin passthrough stays
  origin-scope, and non-API paths stay behind the host guard. A leaked
  serve token cannot reach raw REST; a paired laptop cannot dump the mirror.
- **Transitions with required fields** are listed but disabled, labeled
  "needs fields — use desktop": honest states over silent omission.
- **Errors** are `{"error": code}` → mapped copy; the body is never echoed.
  Token/offer values never appear in UI, logs, or errors (offer decode
  errors describe, never quote).
- **Token** lives in the Keychain via the Rust `token_get/set/del` commands;
  endpoint/label/expiry metadata in `localStorage`. Transport is split in one
  module (`lib/api.ts`): packaged app → `@tauri-apps/plugin-http` fetch
  against the paired endpoint; dev webview → same-origin `/api` through the
  vite proxy.

## 6. Measured on the dev shell (2026-08-25, iPhone 17 Pro simulator)

**The numbers in this first block are the pre-fix ones**, kept because they
name the failure mode; the fix and its 2026-08-26 re-measurement are below.
Before it, the native shell (mobile/src-tauri build installed as
dev.gadak.mobile) laid the WKWebView's layout viewport out INSIDE the safe
area while still reporting env() insets — both were measured with the
dev viewport probe on the Pairing tab:

    inner 402x778 · screen 402x874 · env safe-t 62px · env safe-b 34px
    (874 − 778 = 96 = 62 + 34: the insets are subtracted AND reported)

Consequences and the division of labor:

- The app honors env() unconditionally (§4.1). On this shell that pays the
  top inset twice (62pt of cream above the header) and leaves a ~96pt
  native band under the tab bar. Cosmetic waste, dev shell only — but a
  touch target can never land under the status bar or home indicator on
  any shell, which is the §4.1 guarantee.
- The structural fix is native and one line
  (`webView.scrollView.contentInsetAdjustmentBehavior = .never`): the layout
  viewport becomes 402x874, env() stays 62/34, and this CSS lands the tab bar
  on the physical bottom with no code change.

  **Landed** in `src-tauri/src/lib.rs` — an `objc2` `msg_send` pair in the
  Tauri `setup` hook, behind `#[cfg(target_os = "ios")]`, so it cannot reach
  any other target.

  **Re-measured 2026-08-26** on a shell built from the fixed source
  (`tauri ios dev "iPhone 17 Pro"`), from a `simctl io … screenshot` of the
  running app rather than the probe — the installed bundle at the time of
  the note above predated the fix by five hours, and measuring that one
  would have reported the bug as unfixed. Ink coverage per row on the
  1206×2622 PNG (402×874 @3×) puts the three bands where a correct inset
  puts them:

  | y (px) | y (pt) | what |
  | ---: | ---: | --- |
  | 78–114 | 26–38 | iOS status bar (clock, indicators) |
  | 120–198 | 40–66 | empty — zero ink |
  | 204 → | 68 → | first app content |

  Top inset is 186px/62pt; the app's first pixel is at 204px/68pt, so the
  header pays the inset **once** and adds 6pt of its own. Neither 0
  (`env()` ignored) nor ~372px (paid twice). The 90px of empty space
  between the status-bar band and the app band is why no touch target can
  land under the status bar here. Bottom: lowest app content sits
  122–130px (41–43pt) above the edge, clear of the 102px/34pt home
  indicator — not the ~96pt native band described above. A blind vision
  verdict on the same capture independently read the first app content at
  204px, which is the cross-check for the ink measurement.

  Still not measured: what the tab bar does when the keyboard is dismissed
  in Search. That needs driving, not a screenshot — GDK-838.

  Building the shell needs **rustup's** toolchain, not the one first on
  `PATH`: this machine has Homebrew `rustc` ahead of it, and Homebrew's Rust
  ships no iOS `core`, so `cargo check --target aarch64-apple-ios-sim` fails
  with `can't find crate for core` while `rustup target list --installed`
  swears the target is there. Prefix the toolchain bin dir:

      PATH="$HOME/.rustup/toolchains/stable-aarch64-apple-darwin/bin:$PATH" \
        cargo check --target aarch64-apple-ios-sim
- The same scrollview auto-inset also makes the shell's outer scroll
  offset drift, which shifted synthetic-touch coordinates ±62pt during
  simulator driving. Real fingers and a fixed shell do not have this
  problem; it is why some flows below carry a "not driven live" boundary.

## 7. Decisions that changed during build (measured, not planned)

- **Paste & pair.** The long-press paste callout is hostile on a first-run
  screen. When the offer field is empty the primary button reads
  `navigator.clipboard.readText()` (the OS shows its own paste-permission
  pill) and submits in one tap: `gadak pairing mint | pbcopy` on the
  desktop, one thumb on the phone.
- **Dev adoption.** In dev the vite proxy is the trust boundary (it only
  exists because the developer pointed it at their own loopback serve,
  which decision 0003 keeps unauthenticated). First boot with no stored
  meta probes `auth/me/` through the proxy and adopts it on success. Dev
  builds never mutate the device Keychain (reads fall back to it;
  writes go to a dev-only localStorage slot).
- **credential_required.** A serve without an origin credential (the demo
  workspace) refuses writes with 409; the copy says so instead of a
  generic failure, and reads stay fully usable.
- **Debuggability kept.** The phone has no console: dev builds ship an
  on-screen error overlay (main.ts) and a viewport telemetry line on the
  Pairing tab. Both are `import.meta.env.DEV`-gated. The probe is prefixed
  `DEV` and sits under a hairline so it reads as instrumentation, not as
  product chrome (GDK-879).
- **Pairing is a ledger, not grouped fills (GDK-879).** Same hairline
  separators and section labels as Issues/Search; no panel-fill cards.
  Unpair keeps the two-step arm and expresses danger with ink weight/shape
  (`--color-text-primary`), never a status token.

## 8. Copy

Product voice, sentence case, verbs on buttons ("Pair", "Send", "Unpair").
Jira vocabulary only (§8 UX_PRINCIPLES): status, priority, comment,
transition — no invented nouns. **Names come from the catalog, not from
here** (§3.6): "Issues", "Assigned to me", "All open", "Sync now", "Cancel"
and the picker's section labels are `t()` calls and are Korean and Japanese
without further work. What is still authored here is the connective prose,
in English: the Search and Pairing tab labels, "Offline — showing the last
synced snapshot.", "Nothing open is assigned to you.", "This serve has no
identity to filter by.", "Nothing here" / "No issues on this mirror match
this scope.", "Open on the desktop", "Show all N", and the pairing screen.
Those are the candidates for the next catalog keys — "Open on the desktop"
first, since it is the one refusal a Korean reader meets in English. Errors
say what to do next ("Pairing was refused — mint a new offer on your desktop
and pair again."), never apologize, never quote server internals.

## 9. Gates

- **Dev demo tour** is opt-in. Open the DEV origin with `?demo-tour` (any
  value). A file probe at `/__demo-tour__` was abandoned: `mobile/public/`
  does not exist, and vite's SPA fallback answers every unmatched path with
  `index.html` and 200, so the tour ran on every `npm run dev` boot. Packaged
  builds never run it (`import.meta.env.DEV` in `lib/demo-tour.ts`). `npm test`
  covers the disarmed path; the viewport gate walks a boot with nothing armed.
- **iOS contract (§4.1–4.2)** — `npm run lint:ios`. Only `src/app.css` may
  mention `env(safe-area-inset-*)`. No CSS `vh` / `dvh` / `svh` / `lvh` units
  under `src/` (a comment that names the ban is not a unit).
- **Viewport geometry** — `npm run viewport-gate` (from `mobile/`; also
  `bash mobile/scripts/viewport-gate.sh` from the repo root). Playwright at
  402×874 against `gadak demo --addr 127.0.0.1:7899` (vite's `/api` proxy —
  target port `GADAK_SERVE_PORT`, default 7899) and vite on
  `127.0.0.1:5182`. Not `e2e/`'s `127.0.0.1:7877` and not `e2e/.tmp/home` —
  demo makes its own temp home. Asserts horizontal overflow
  0, `nav.safe-bottom` flush to the viewport bottom, no input/textarea under
  16px, ≥12 issue rows per screen (before **and** after the scope picker has
  been opened), a visible escape (tab bar, `button.back`, or sheet Cancel),
  and no visible button under 44pt (GDK-867). Sheets are measured after their
  rise finishes: a rect read off a mid-transform composited layer at DPR 3
  comes back a hair under the laid-out size (a 44px control read 43.99994),
  which is an artifact of when we looked, not of what shipped.
- **Vocabulary (§3.6)** — `src/lib/vocabulary.test.ts` in `npm test`. Fails if
  "Queue" / "Mine" / "All" reappear in shipped source, if any file other than
  `lib/i18n.ts` reaches into `web/src/lib/i18n`, if the picker stops using the
  `sidebar.*` section keys, if a `POST`/`DELETE` is aimed at `issues/views/`,
  or — the one that catches a hardcoded English string an English eye would
  pass — if the scope names are not 내 담당 / 전체 미해결 under `gadak_locale=ko`.
  A parity case also re-reads the desktop's `effectiveCategory` and fails if
  the phone's status-category folding drifts from it. The by-hand form of the
  same check must exclude that file, which necessarily spells the banned words:
  `grep -rn --exclude=vocabulary.test.ts "Queue\|'Mine'\|>Mine<\|>All<" src e2e`
  → no hits.

- **Webview CSP `connect-src` (GDK-1137)** — `src/lib/csp.test.ts` in
  `npm test`. The packaged webview may only `connect` to its own origin:
  `'self'` and nothing else. Every network path is either same-origin
  (the demo bundle's `window.fetch`, the dev `/api` proxy) or native and
  outside CSP entirely (`plugin-http`, `plugin-websocket`); tauri's own iOS
  IPC tries an `ipc://localhost` fetch, is refused by this same policy, and
  falls back to `window.ipc.postMessage` — measured in the tauri 2.11.5
  crate's `scripts/ipc-protocol.js`, so `'self'` costs nothing there. No dev
  exception exists because none is needed: on iOS dev,
  `PROXY_DEV_SERVER` (`all(dev, mobile)`) routes the page through
  `tauri://localhost` and hands vite's responses to the webview verbatim —
  dev HTML carries no CSP header at all, so HMR's `ws://localhost:5180` and
  the dev terminal socket never consult this value. The capability file
  (`http:default` → ts.net + loopback) was already narrow; this closes the
  gap where the CSP alone would have let the webview fetch any origin.

## 10. Shell — the terminal tab (GDK-865)

The phone attaches to a PTY session running on the paired `gadak serve`.
Not a new capability on the phone: the same shell the desktop pane opens
(`internal/term`), reached from the couch. There is no relay and never will
be — the phone dials the endpoint it paired with, directly, over whatever
network already carries the mirror (a tailnet, usually).

**Where it lives is being decided — GDK-902.** It ships today as a tab that
is **absent until a terminal pairing is stored** (absence, not a greyed-out
tab, matching how PairGate replaces the app rather than disabling it). That
was reasoned from §2's "the tab is the object", and a shell is an object.
The tab model itself is what is being replaced: the shell becomes one of the
things that can *own the column*, entered from the palette, full-screen —
which is what the keyboard forces anyway, and therefore not a compromise but
the model. Everything below this line is independent of that choice and
survives it.

### 10.1 Its own token, and why

The terminal gate (`internal/server/terminal.go`) admits a Bearer whose
pairing scope is exactly `terminal`. The phone's existing token is `serve`
scope, and reusing it was explicitly ruled out: a token that reads the
mirror must not also be able to open a shell on the machine.

So the phone holds **two** tokens in the Keychain, in two slots. Pairing the
shell is its own scan of its own QR, minted at the desk with
`gadak pairing mint --scope terminal --label "<phone>"`.

An offer does not carry its scope, so a mis-scanned QR is indistinguishable
until it is used. The phone therefore **probes before storing**:
`GET /api/v1/terminal/sessions/` with the scanned token. The gate answers
`scope_rejected` for a serve token, `pairing_rejected` for a revoked one —
both honest, both shown as copy. A token is written to the Keychain only
after the probe passes.

### 10.2 Transport — two branches, one seam

| Build | Socket | Why |
|---|---|---|
| dev | plain `WebSocket` through vite's `/api` proxy | the proxy makes the serve loopback and same-origin, and a loopback peer needs no Bearer at all (`terminalGate`) |
| packaged | native WebSocket (`tauri-plugin-websocket`) | a webview `WebSocket` cannot set `Authorization`, and its `tauri://localhost` origin is not the serve's — `browser_guard.go` origin-checks upgrades. A native client sends no `Origin`, which the guard allows, and can set the header. |

The same split, for the same reason, as `lib/api.ts`'s `fetch`. The wire
vocabulary is not re-spelled here: `web/src/lib/terminal/protocol.ts` is
imported, the way `app.css` imports the web tokens.

**The endpoint check is ours, not the platform's.** `http:default` in
`capabilities/default.json` is URL-scoped to `https://*.ts.net` and
loopback; the websocket permission has no such allowlist. The replacement is
`assertPairedWsUrl(endpoint, url)` in `lib/terminal/transport.ts` — scheme,
host and port must equal the stored pairing endpoint, so a shell can be
opened only on the machine this phone paired with. It is a tested pure
function rather than an inline `if` because it is the only thing standing
where a platform allowlist would otherwise stand.

### 10.3 Input — the part that is phone-only

- **IME.** Nothing reaches the PTY between `compositionstart` and
  `compositionend`; the composed string is sent once, as UTF-8, on
  `compositionend`. A shell that receives half-assembled 자모 is receiving
  garbage, and no amount of terminal-side cleverness fixes it afterwards.
  The gate is a pure state machine with a Korean event sequence in its tests
  — not a screenshot.
- **The phone keyboard has no Esc, no Ctrl, no arrows.** A key bar sits
  above the keyboard carrying them, riding the existing `keyboardInset`
  action (§4.2) — the VisualViewport measurement is already solved here and
  is not solved twice. Ctrl is *sticky*: tap it, then a letter, and the
  control byte goes out (`Ctrl`+`c` → `0x03`).
- **A sticky modifier has three states, not two** (GDK-898). idle, *armed*
  for the next key, and *locked* until you tap it again — a second tap
  within 400 ms promotes armed to locked. A toggle cannot hold Ctrl across
  several keys, and it cannot tell "armed for one" apart from "held". The
  bar shows all three with shape as well as colour, because a modifier whose
  state you cannot see is worse than no modifier at all.
- **The decisions are shared; the bytes are ours.** Sticky slots, the
  composition gate, the key-repeat cadence and the flush barrier live in
  `glasskeys`, which naru-remote runs the same golden vectors
  against. What stays here is the encoder — control bytes, CSI sequences,
  UTF-8 — because naru wraps the same decision in an X11 keysym over VNC and
  there is no shared encoding of "Ctrl-C". `src/lib/terminal/keys.ts` is the
  adapter between the two, and it is still a pure function over plain data.
  New input *behaviour* belongs in the library, where both apps get it; new
  *encoding* belongs here.
- **Autocorrect is off** on whatever element takes the keystrokes.
  Smart quotes in a shell command are a defect, not a nicety.

### 10.4 Reconnect is the normal case here

On the desktop a dropped socket is an event; on a phone it is what happens
every time the screen locks. iOS freezes the webview when the app
backgrounds, so the socket dies and the 60 s server-side grace starts. The
phone therefore reattaches on `visibilitychange → visible` **immediately**,
without waiting out the backoff schedule, and the ring replay is the first
binary frame. Past the grace the session is gone and the phone says so and
offers a new one — the same ended-state contract the desktop pane has, just
reached far more often.
