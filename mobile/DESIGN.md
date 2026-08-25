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
| "What's on my plate?" | Queue (default tab) | glance — no taps |
| "Where was that issue about X?" | Search | 2 taps + typing |
| "What happened here?" | Detail (comments-first) | 1 tap from any row |
| "One-line reply / push the status" | Detail composer · transition sheet | thumb only |
| "Is this thing still connected?" | Pairing tab | rarely visited, always honest |

Non-jobs (deliberately absent in v1): creating issues, editing fields,
attachments, wiki, feed/notifications, JQL. The desktop owns those.
History timeline and plugin enrichments are read on the desktop too — the
phone shows description, comments, linked issues.

## 2. Screen map & navigation model

Three tabs + one push layer. No hamburger, no drawer, no nested stacks.

```
┌────────────────────────────┐
│ PairGate (only when        │  not a tab — replaces the app
│ unpaired / token rejected) │  until pairing succeeds
└────────────────────────────┘
┌─────────┬─────────┬────────┐
│ Queue   │ Search  │ Pairing│  tab bar, bottom, always visible
└────┬────┴────┬────┴────────┘
     └────► Detail (push, slides over tabs)
                └─► Transition sheet (bottom sheet)
                └─► Detail of a linked issue (replaces, back returns to list)
```

**Entry / exit contract** (dead-end rule: every screen has an explicit way
out; system back = the same edge):

| Screen | Enter from | Exit |
|---|---|---|
| PairGate | boot (no pairing) · token rejected | successful pair → Queue |
| Queue | tab · boot default | tab bar |
| Search | tab | tab bar |
| Pairing | tab | tab bar |
| Detail | row tap (Queue/Search) · linked-issue tap | ← back button (top-left, 44pt) → the tab that opened it |
| Transition sheet | status chip in Detail | scrim tap · Cancel · apply |

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
  ("Queue ·23") on a *phone tool*. It is what makes a capture of this app
  unmistakably gadak and not a Tailwind template; if it reads as a newspaper
  gimmick in captures, the fallback is `.type-subject` on Detail titles only.

### 3.3 Space & density
4pt grid (4/8/12/16/24/32), 16px screen gutter. Queue rows are **56px**
(≥44pt target, two lines: summary + meta). On the iPhone 17 Pro viewport that
yields 12–13 rows per screen — a two-digit ledger, not a card feed. No
cards, no shadows in lists; hairline `border-subtle` separators only.

### 3.4 The signature: ink spine
Every issue row carries a 3px left spine in its `status_category` color
(new / inprogress / done tokens; reopened rows use the reopen token when
`reopen_count > 0`). In Detail the same spine runs down the left of the
header block — the row visually *is* the page you landed on. This is the one
memorable mark; everything else stays quiet.

### 3.5 Motion
Two moves, both under 250ms, both disabled under `prefers-reduced-motion`:
Detail slides in from the right (200ms ease-out), sheets rise from the bottom
(240ms cubic). Nothing on the scroll path animates. No spinners — loading is
skeleton rows (paper rectangles), writes show state in the pressed control
itself ("Sending…" in the send button, progress dot in the transition row).

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
   empty queue ("Nothing assigned to you" + one-tap scope switch), empty
   search (recent searches, never blank), write failure (inline in the
   control that acted, cached read state untouched).
7. **Dark mode** is the same code path — token flip only, verified with
   light/dark captures of every screen.

Thumb zone: the three write actions (compose, send, transition) and the tab
bar all live in the bottom third. The top third holds only reading and the
back button.

## 5. Data & protocol decisions

- **Snapshot model.** `GET issues/bootstrap/` with `If-None-Match` is the
  read path; the full lite snapshot is the phone's mirror. Cached in
  `localStorage` (guarded try/catch — over-quota mirrors simply skip the
  cache and refetch). Sync on boot, on foreground (`visibilitychange`), and
  on freshness-chip tap. `issues/delta/` is a v2 optimization once a mirror
  large enough to hurt is measured.
- **Queue** = `status_category !== 'done'`, sorted `priority_rank` asc
  (unset ranks last), then `updated_at` desc, grouped by priority with the
  display name as the section label (display-only; logic never keys on it).
  Scope: **Mine** (assignee_id = me.account_id, else email match) with an
  **All** toggle; when the server has no identity (standalone) or Mine is
  empty, the queue auto-falls to All and says so.
- **Search** is local-first: instant key/summary substring over the snapshot
  while typing, merged with debounced server `issues/search/` results (body
  and comment matches the snapshot cannot see).
- **Writes** (`comment`, `transition`) go through origin and re-sync after —
  the app never mutates the snapshot except as an optimistic overlay that a
  failed write rolls back. Frequent writes get no confirm dialog (§7 of
  UX_PRINCIPLES); the one destructive rarity, Unpair, uses the house two-step
  arm (tap → armed red "Tap again" → 3s auto-disarm).
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

The current native shell (mobile/src-tauri build installed as
dev.gadak.mobile) lays the WKWebView's layout viewport out INSIDE the safe
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
  any other target. Verified to compile for `aarch64-apple-ios-sim`; **not
  re-measured on the simulator yet**, so the numbers above are still the
  pre-fix ones. Re-run the viewport probe on the Pairing tab after the next
  shell build and replace them.

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
  Pairing tab. Both are `import.meta.env.DEV`-gated.

## 8. Copy

English, product voice, sentence case, verbs on buttons ("Pair", "Send",
"Sync now", "Unpair"). Jira vocabulary only (§8 UX_PRINCIPLES): status,
priority, comment, transition — no invented nouns. Errors say what to do
next ("Pairing was refused — mint a new offer on your desktop and pair
again."), never apologize, never quote server internals.

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
  402×874 against `gadak demo --addr 127.0.0.1:7899` (vite's hardcoded `/api`
  proxy) and vite on `127.0.0.1:5182`. Not `e2e/`'s `127.0.0.1:7877` and not
  `e2e/.tmp/home` — demo makes its own temp home. Asserts horizontal overflow
  0, `nav.safe-bottom` flush to the viewport bottom, no input/textarea under
  16px, ≥12 queue rows per screen, and a visible escape (tab bar, `button.back`,
  or sheet Cancel). The 44pt tap-target assertion is written at the real
  contract and skipped until GDK-867 (32pt `.fresh` / `.status` / `.cancel` /
  `.act`).
