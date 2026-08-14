# UX principles

The standard every UI wave is measured against. Specs for UI rounds quote the
relevant sections instead of restating them; when a change and this document
disagree, either the change is wrong or this document gets a dated revision —
never a silent divergence.

Every principle here is either (a) a published rule from a product that earned
the right to be copied, with the source linked, (b) a pattern we verified
across several such products, or (c) a dated owner decision that constrains a
surface this document already owns — currently the contained browser
(§11–§13). Nothing below is folklore; where research could not find a primary
source, the claim was dropped. Owner decisions name their spec. Sources are
collected at the end. (Established 2026-08-06, from a survey of Linear,
Superhuman, Raycast, Notion, Confluence, Outline, Google Drive, Slab, and
clig.dev. §11–§13 added 2026-08-14.)

## 1. Speed is a budget, not a feature

Superhuman publishes "every interaction should be faster than 100ms" and aims
under 50ms — and then lets that number win arguments: fewer animations, a
local database, preloaded threads. Linear says the same thing in words —
"the seconds add up when you're taking the action multiple times."

gadak has no network in the read path at all, so its numbers must be more
aggressive than theirs, and they are enforced, not aspired to:

- Interactions on the hot loop (list movement, filter toggles, opening an
  issue) budget at **50ms**; view transitions at **100ms**.
- The e2e perf suite (`e2e/perf/`) pins measured p95 ceilings — cold boot,
  warm boot, search, palette — and fails the build when they regress.
- Animations are allowed only inside the budget, and never on the triage-key
  path (`j`/`k` repetition is exactly the case where "seconds add up").

**Corollary: gadak has no spinners.** The mirror is local; anything that shows
a loading state in the read path is a bug in how we use the mirror, not a
UI-polish item. (Writes that go to Jira are the exception — see §7.)

## 2. The local mirror is a dividend — spend it

Linear's CTO on their sync engine: with local data "you're effectively just
building the front end… there's nothing else you need to do" — no error
handling, no waiting, no two code paths. Superhuman keeps "a database of your
emails stored in your app or browser."

gadak already paid for this architecture. The principle is to keep collecting
the dividend: instant filters, search while typing, undoable triage, every
feature working offline. When designing a feature, the first question is
"what does this look like when data access is free?" — because for us it is.

## 3. Keyboard is architecture, and the palette teaches it

Superhuman's five command-palette rules, Raycast's action bar, and Linear's
context menus are the same design: the pointer UI is a ladder into keyboard
fluency, not an alternative to it.

- **One palette, same key, everywhere.** No second menu with its own key.
- **Omnipotent**: every action the app can do is registered in the palette —
  triage keys, sync-now, view switches, settings. This is auditable: diff the
  action list against the palette registry when a wave adds actions.
- **Forgiving**: fuzzy match, case-insensitive, aliases across vocabularies
  (Jira's terms, ours, and Korean equivalents), and show the matched alias so
  the user learns the canonical name.
- **Shortcuts are displayed where actions appear** — palette rows, bulk bar,
  context menus — not only in the shortcuts dialog. Raycast's diagnosis
  applies: help hidden behind a small button is help new users never find.

## 4. Opinionated defaults; settings are a last resort

"The best design is opinionated" (Saarinen). "A tool should work for you, not
the other way around" (Linear Method). "Make the default the right thing for
most users" (clig.dev). Raycast goes furthest: extensions that invent their
own navigation get rejected — expressiveness is traded away for coherence.

gadak is single-user, which removes the usual excuse for settings: there is no
second user to disagree with the default. The gate question for any new
option: *are we adding this because the value genuinely differs per user
(project/space scope — legitimate), or because we failed to decide (density,
sort order, theme details — decide it)?*

The same trade applies to extension surfaces (`docs/PLUGINS.md`,
`docs/EXTENDING.md`): fixed components over arbitrary UI.

## 5. Density stays; chrome recedes

Linear's design-refresh principles: "Don't compete for attention you haven't
earned" and "Structure should be felt not seen." Density of information is
gadak's advantage (issues and docs in one screen); the way to keep it readable
is not to remove data but to demote chrome — sidebar contrast below content,
fewer separators, icons only where they carry information, status chips
(freshness) quiet until they have something to say.

Audit questions for any screen: Is the sidebar visually louder than the
content? Would removing this divider lose information? Is this icon data or
decoration?

## 6. Navigation: recency beats hierarchy

This section is the applied conclusion of a dedicated survey (Confluence,
Notion, Outline, Google Drive, Slab, Linear — sources below). The patterns
were unanimous:

- **No tool lists all containers by default.** Confluence's sidebar shows
  recent + starred spaces with "view all" one level deeper; Notion lists only
  joined teamspaces; Outline puts collections third, below Starred, in the
  scrolled region; Drive demotes the tree to one of five entries.
- **The default landing view is "recently viewed", not "recently updated".**
  Outline's default tab is Viewed; Confluence's home leads with "pick up
  where you left off" (five items); Slab's home is Viewed + activity.
  Updated-feeds are someone else's activity; viewed-lists are your own
  return path. Both exist; viewed wins the default.
- **Viewed / worked-on / updated / starred are parallel tabs, never merged.**
- **Lists have hard caps** (Confluence: five; Outline: ten with "show more").
  Sidebars must not grow with content volume.
- **A document row is one sentence: title, then who-did-what-when-where** —
  "Jane updated 3 hours ago in Engineering" (Outline's `DocumentMeta`).
  The container is a suffix, not a group header, so recency order survives.
  Names, not avatars.

For gadak, single-user and fully local, the surviving axes are: recently
viewed (local), recently updated (`updated_at`), author, favorites.
"Popular", "following", "shared with me" have no meaning here and are
deliberately absent. Tree hierarchy is kept for *orientation once a document
is open* (breadcrumbs), not as the primary way to *find* one. The local
mirror lets us do one thing the originals cannot do cheaply: an unread
highlight computed as `updated_at > last local visit`, with zero server
involvement.

## 7. Confirmation is a function of reversibility

clig.dev: confirmation strength proportional to danger. Triage actions on the
local mirror are cheap to undo — they execute immediately, no dialog, undo
offered. Writes that leave the machine (comments, transitions to real Jira)
are not reversible by us — they get confirmation. Whether a dialog exists is
never a style choice.

## 8. Don't invent vocabulary

Linear Method: "Don't invent terms if possible." gadak mirrors Jira and
Confluence, so their words are our words — epic, sprint, space, page, label.
A gadak-only synonym forces users to maintain two vocabularies. The palette
accepts aliases (§3) but displays canonical names.

## 9. Quality is a routine, not a taste

Linear operationalized quality twice: a zero-bugs policy (fix within 2/7
days, or close as explicit won't-fix — no backlog limbo, because "fixing a
bug takes the same amount of work whether we do it right away or put it
off"), and Quality Wednesdays (one small imperfection per engineer per week —
born from a team mostly *failing to see* an inconsistency they were shown).

gadak's translations:

- A defect either gets fixed or gets a dated won't-fix with a reason.
  "Someday" is not a state.
- Periodic defect-hunt rounds (vision review against live screens) on a
  schedule, not only before releases.
- Specs are a floor, not a target (Saarinen) — but the inverse trap is
  real too: perceived quality is the standard and gates are its proxy;
  when a shipped screen is judged good and a self-authored gate still
  fails, re-pin the gate with evidence instead of tuning the product
  toward the proxy.

## 10. Design is understanding, not output

Linear: "the hard part of design is rarely generating the form. It is
understanding the problem well enough to know what and how something should
exist at all." In practice for this repo: UI waves start from observed usage
(real-mirror feedback, agent-onboarding friction), and a spec that says only
*what to draw* without *why this shape* produces work that "unravels the
moment you actually use it."

## 11. The shell is packaging; the mirror and the handoff are the product

Owner decision, 2026-08-14 (`specs/001-dedicated-browser/spec.md`, G-a). A
feature that grows the shell but not the mirror or the agent handoff is
default-rejected. The pitch is "where your Jira lives", not "browser" — that
word buys universality expectations (SSO, every page perfect) that an
embedded WebView cannot honor.

## 12. The in-app browser is an escape hatch, not a floor

Owner decision, 2026-08-14 (`specs/001-dedicated-browser/spec.md`, G-b). No
native surface may *require* the in-app browser. Feature requests aimed at
the browser pane (history, bookmarks, extensions, persistent tabs) are
default-rejected. The pane owns tabs, a rectangle, and post-close resync —
nothing else.

## 13. In-app tabs are session-scoped consumables

Owner decision, 2026-08-14 (`specs/001-dedicated-browser/spec.md`, G-c).
Retrieval ("I'll need this again") is the mirror's job: search, recents,
favorites (§6). The components this constrains are the browse **tab strip**
(`BrowsePane`, `data-testid="browse-tabs"`) and the **re-entry pill**
(`BrowseHost`, `data-testid="browse-reentry"`). Neither may become a second
sidebar. The success metric is retrievals that needed no tab, not tab count.

---

## What already embodies this (keep, and defend)

- Local mirror + enforced perf budgets (§1, §2) — the structural part most
  products never get.
- Keyboard triage + command palette (§3) — remaining work is coverage
  auditing and in-context shortcut display, not architecture.
- Single-user opinionation (§4) — cheapest possible conditions; use them.
- Honest states — freshness chip, explicit "unsupported" reporting — are the
  UI face of the receipts culture.
- Contained browser as an escape hatch (§11–§13) — the tab strip and the
  re-entry pill are session chrome, not a second sidebar.

## Sources

Primary (company/founder):
[Linear Method](https://linear.app/method) ·
[Zero-bugs policy](https://linear.app/now/zero-bugs-policy) ·
[Quality Wednesdays](https://linear.app/now/quality-wednesdays) ·
[Behind the latest design refresh](https://linear.app/now/behind-the-latest-design-refresh) ·
[Output isn't design](https://linear.app/now/output-isn-t-design) ·
[Invisible details](https://medium.com/linear-app/invisible-details-2ca718b41a44) ·
[Tuomas Artman on localfirst.fm](https://www.localfirst.fm/15/transcript) ·
[Superhuman is built for speed](https://blog.superhuman.com/superhuman-is-built-for-speed/) ·
[How to build a remarkable command palette](https://blog.superhuman.com/how-to-build-a-remarkable-command-palette/) ·
[Raycast: a fresh look and feel](https://www.raycast.com/blog/a-fresh-look-and-feel) ·
[How Raycast API extensions work](https://www.raycast.com/blog/how-raycast-api-extensions-work) ·
[Raycast store guidelines](https://developers.raycast.com/basics/prepare-an-extension-for-store) ·
[clig.dev](https://clig.dev/)

Navigation survey (§6):
[Confluence navigation](https://support.atlassian.com/confluence-cloud/docs/improved-confluence-navigation/) ·
[Confluence Home](https://support.atlassian.com/confluence-cloud/docs/use-home-to-jump-into-work-and-see-whats-happening/) ·
[Notion sidebar](https://www.notion.com/help/navigate-with-the-sidebar) ·
[Outline source](https://github.com/outline/outline) (`app/scenes/Home.tsx`,
`app/components/DocumentMeta.tsx`, `Sidebar/App.tsx`) ·
[Google Drive navigation](https://support.google.com/drive/answer/12169158) ·
[Slab home](https://help.slab.com/en/articles/7061771-your-slab-home-page) ·
[Linear display options](https://linear.app/docs/display-options)

Secondary (interviews; quote as spoken views, not documented rules):
[Saarinen's 10 rules](https://www.figma.com/blog/karri-saarinens-10-rules-for-crafting-products-that-stand-out/) ·
[Ivan Zhao interviews](https://nesslabs.com/notion-featured-tool)

Owner decisions (§11–§13):
[`specs/001-dedicated-browser/spec.md`](../specs/001-dedicated-browser/spec.md) (2026-08-14)
