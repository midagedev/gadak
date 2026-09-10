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
clig.dev. §11–§13 added 2026-08-14; §14 added 2026-09-06 from the
literature review in `THEORY.md`.)

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
  warm boot, search, palette. It is an opt-in local suite (`GADAK_PERF=1`);
  `e2e/playwright.config.ts` ignores `perf/` and CI does not run it.
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
  triage keys, sync-now, view switches, settings. The registry is
  `web/src/lib/commands.ts`; a row's action lives in
  `web/src/lib/command-palette.ts`, which is why an action a component keeps
  as a private closure is invisible to the palette by construction. When a
  button and a palette row do the same thing, the thing itself gets a module
  and both call it (`copy-view-link.ts`, `copy-issue-link.ts`,
  `save-current-view.ts`).
- **...and audited by a gate, not by a promise.** The "diff the action list
  against the registry" step was a habit, and a habit measures nothing: every
  column view had a row to enter it and none had a row back to the list.
  `web/src/lib/palette-coverage.test.ts` now measures the part that can be
  measured mechanically: every destination the main column can show
  (`COLUMN_KINDS` in `lib/column-view.ts`, checked against the `ColumnView`
  union by the compiler) has a palette row declaring `opens`, or an
  allowlist entry stating why it does not. Both sides are read from their own
  owner; the test keeps no list of its own.
- **Except what should not be one press away.** Deleting a shared view is
  irreversible and lands on everyone, and a palette row is one Enter — it
  stays a deliberate act with a target you have to point at (§7:
  confirmation is rarity × irreversibility).
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

**Revision 2026-08-24 (GDK-786/791, user color overrides).** Colors tested
this gate and passed it — not as a settings surface, but as an escape hatch
with the opinion kept. The defaults still have opinions: the four shipped
palettes are the design, grounds and shell structure are locked outright,
and validated inks must clear the same contrast and color-vision floors the
palettes themselves are held to (a user override that fails them is
refused with the measured number). What a user may move is the accent, the
free decorative inks, and per-data tints — through a catalog
(`ui.tokens.catalog`) plus a write gate, never free CSS. The principle
gains its enforcement clause: *a setting is admissible when the default
stays opinionated and the override is validated against the same contract
the default had to meet.*

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

**First-screen precedence (0.21).** "Viewed wins the default" above and
THEORY.md G4 ("the first screen is my work") name different defaults, and
the shipped order in `web/src/lib/startup-view.ts` is the resolution: URL view
params → the last-used view → the team-group preset → **my work**, when an
identity exists and it has open assigned issues → all open. Recently viewed
is not a landing view in that order; it stays the return path (the Feed and
`gadak recents`), and the resume card carries the per-issue half. The
anonymous / zero-assigned fallback was the Epic breakdown (GDK-100) until
2026-09-07, when that built-in left the sidebar (GDK-1493: a layout of the
open pool that only means something on a site with a hierarchy); the hosted
demo alone keeps an epic-grouped landing as its own config. Open decision,
registered as a GDK draft on 2026-09-06: whether that fallback should become
recently viewed, as this section would have it, or stay the open pool.
"Popular", "following", "shared with me" have no meaning here and are
deliberately absent. Tree hierarchy is kept for *orientation once a document
is open* (breadcrumbs), not as the primary way to *find* one. The local
mirror lets us do one thing the originals cannot do cheaply: an unread
highlight computed as `updated_at > last local visit`, with zero server
involvement.

## 7. Confirmation is a function of rarity × irreversibility

(Revised 2026-08-21, GDK-134. The original text promised confirmation for
every write that leaves the machine; in practice high-frequency triage
writes — transitions, assigns, comments — are correct without one, and
that was the §1 reading all along.)

clig.dev: confirmation strength proportional to danger. Two axes decide it:
how often the action happens, and whether anyone can undo it. Frequent
actions get no dialog even when they write to Jira — a confirm on every
transition would train reflexive clicking, which protects nothing. What
earns friction is the rare action nobody can take back: deleting a team
view removes it for every teammate with no undo, so its × arms on the
first click and deletes on the second. That two-step arm is the house
confirm — proportional strength, no modal. Whether friction exists is
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

## 14. Coaching grammar: facts about the work, never advice about the person

Owner direction, 2026-09-06: gadak should help the user get work done, not
only find issues — and never overtly. `THEORY.md` carries the evidence; the
short form is that more than a third of measured feedback interventions
*lowered* performance, and the effect worsens as attention moves from the
task to the self (Kluger & DeNisi 1996), while arrangement — defaults, order,
weight — changes behaviour without provoking reactance, even when disclosed
(Thaler & Sunstein 2008; Loewenstein et al. 2015). Every signal, card, or
meta line a UI round adds is measured against these ten rules:

- **G1** The subject is an issue, a queue, an epic — never the user.
  "In progress for 12 days", not "you have too much in progress".
- **G2** State the fact; put the verb on a button beside it, never in the
  sentence.
- **G3** Speak only at boundaries — session start, opening an issue, just
  before a transition, choosing a priority. Never mid-edit, mid-read, or
  in the terminal.
- **G4** Try order, density and weight before adding a sentence.
- **G5** Every signal has a dim peripheral form and a central form with a
  numeric promotion condition. Colour is the last resort; red is not used.
- **G6** Friction, never a block: a count, not a confirm dialog.
- **G7** A one-line basis on hover for every signal; dismissal is remembered
  per signal and scope and never re-asked.
- **G8** Count unprompted handling; past a threshold the signal stays
  peripheral. Coaching succeeds by disappearing.
- **G9** Progress is visible where the user goes on purpose. No push, no
  streaks, no scores, no rankings, no praise.
- **G10** When the user asks the agent, the answer carries age, blockers and
  who is waiting — asked-for is an answer, not an interruption. Coaching
  content lives in `skills/gadak/SKILL.md`.

There is no coaching-intensity setting (§4): if the grammar is right, none is
needed. The seven boundaries where gadak is allowed to speak are tabulated in
`THEORY.md`; outside them it says nothing.

## 15. Every overlay closes the same way, and says so

(Added 2026-09-10, GDK-138.) An overlay that covers what someone was reading
owes them one obvious way out. Which way it was had been a per-file decision:
four panels drew the same × from four hand-rolled copies of one SVG path, and
whether a dialog also offered a secondary button depended on who wrote it.
Nothing about that variance was a design; it was the absence of one.

Overlays fall into two shapes, and the shape decides the chrome:

- **Modal** — a backdrop, `aria-modal`, focus trapped inside. It carries a
  header × at the top right, closes on Esc, and closes on a backdrop click.
  `ui/DialogShell.svelte` owns all three; a modal that hand-rolls them is the
  defect (GDK-316's claim, measured by `DialogShell.test.ts`). The full-bleed
  media lightbox and the command palette are the two sanctioned strays, and
  the palette has no × because running the thing you came for is the exit.
- **Anchored** — a popover or menu attached to the control that opened it.
  No ×: it has no header to put one in, and outside-click is the dismissal a
  person already reaches for (`lib/dom-actions.ts` owns Esc and outside-click
  for both shapes, in one tier order).

Three rules hold across both:

1. **One name.** The dismiss control is `common.closeEsc` on `aria-label` and
   on `title`, so the tooltip and the screen reader say the same words and
   name the keyboard route while they are at it. A second phrasing is a
   second vocabulary for one action (§8).
2. **One glyph.** The mark is `<Icon name="x" />`. Never a literal ✕ and
   never a local `<svg>` — an icon set is only a set if every member shares a
   grid, a weight and a colour (`ui/Icon.svelte`).
3. **A secondary button is for abandoning edits, not for closing.** A dialog
   holding a pending draft pairs Save with Cancel, because discarding a draft
   is a different act from dismissing a panel. A read-only overlay gets no
   second button: it would be a synonym for the × beside it.

`ui/close-chrome.test.ts` measures all three. The same reasoning covers the
roles: a row of tabs is a `tablist` with `aria-selected`, not nine buttons
that happen to be painted differently — the mark and the semantic ride the
same condition, exactly as §6's `aria-current` does.

## 16. A number says what it counted; an empty state says what to do next

(Added 2026-09-10, GDK-1091, GDK-1092.) Two findings from the R2 UI audit
turned out to be one shape: a screen showing a reader a number, or a
sentence, that is true and still misread because nothing on the screen says
what it is about.

**Two totals of different scope may not sit side by side unlabelled.**
The Documents header said `Documents 0` on an account whose wiki held 71
pages. The badge counted the active tab — Viewed, the pages this account has
opened — and it sat beside the word Documents, so it read as the library. The
denominator was worse than the numerator: it was
`tab === 'viewed' ? recentlyViewed.length : index.length`, so the fraction a
filter drew meant *of the library* on two tabs and *of what you have opened*
on the third. The rule now:

> The denominator beside a screen's name is the **library** total — one owner,
> the same meaning on every tab and under every filter. A number that has been
> narrowed is written as a fraction of it; a number that has not is written
> alone.

`0` becomes `0 / 71`. The narrowing is what the reader could not see, so the
narrowing is what the badge shows. The owner is
`components/docs/docs-count.ts`, and `docs-count-scope.test.ts` fails on a
per-tab denominator coming back. The same rule is what makes the list
toolbar's count legible beside the sidebar's built-in view counts: each of
those sits inside the row that names its scope, and the toolbar's sits inside
the column whose filters produced it. A count that can be read as belonging
to neither is the defect this rule names.

**An empty state's hint is the only line with room for a next move.** The
0-hit search screen read `No issues match` over `No issues match this
search.` — two lines that are one sentence, told to a reader already looking
at an empty list. A hint may repeat a word from the title; it may not repeat
the sentence. `components/list/empty-state-copy.test.ts` discovers every
`<EmptyState>` pair from the source and fails on containment or near-total
word overlap, so a pair written next year is gated the day it is written.

**And a settled empty state centres in the surface it owns.** The issue
panel's "not found" rendered at `py-16` — about 8% down an 830px scroller,
the rest blank — while the same kind of message in the list column sat at its
middle, because the panels had hand-laid copies of a block `EmptyState`
already owns. Both panels render `EmptyState` now
(`detail/panel-empty-centering.test.ts`). Where a sentence lands is not a
per-file padding decision.


---

## 16. A piece of chrome looks like what it is

The visual audit (GDK-142) did not find five bad looks. It found the same
meaning drawn differently in each file that draws it: a status dot at three
sizes, two progress bars with two different tracks, a border that means
"different window" wearing the token for "same surface", and an action wearing
the costume of an empty value. Each one is small; together they are the reason
a reader has to re-learn the interface on every screen.

So the rule is not a look. It is that each piece of vocabulary has one owner,
and the owner's numbers are gated
(`web/src/lib/chrome-vocabulary.test.ts`, measured in all four palettes).

**A window seam is not a divider.** Two regions that must read as separate
windows — the terminal against the list, the sidebar against the main column,
the detail panel against the main column — are separated by
`border-border-strong`. A rule *inside* one surface — a dialog header, a row
under a row, a card edge — stays `border-border-subtle`. Measured 2026-09-10:
strong is 1.77–2.31:1 against the ground it sits on, subtle is 1.26–1.43:1.
The audit read the dark terminal as one body with the list because its seam
was subtle; brightening `border-subtle` itself would have thickened every rule
in the app, which is why the two are two words and not one dial.

**A meter is a track plus a fill,** and both belong to `ui/MeterBar.svelte`.
A bar whose empty half is invisible is not a proportion, it is an underline —
the epic progress bar's `bg-bg-elevated` track measured 1.00:1 against its own
ground. The track is `bg-bg-active` (1.28–1.76:1 against base, panel and
elevated in every palette), and the fill carries the meaning: category ink
when the bar is about status, muted when it is about volume.

**A status dot is `ui/StatusDot.svelte`,** at one of two sizes: `md` for a row
lead you aim at, `sm` for a dot that rides beside the text it annotates. The
dot carries the category and nothing else carries it again — the word next to
it is the site's own status name in muted ink, never re-tinted. Colour and word
are two facts, not one fact twice. The single exception is `IssueRow`'s lead
dot, which is a filter button rather than a chip; it owns its hover affordance
and imports `DOT_CLASS` for the size.

**A value and an action are not the same costume.** An absent value —
`Unassigned`, `None`, an empty body — is `EMPTY_VALUE` from `lib/chrome.ts`:
muted, italic. Italic is this app's "there is nothing here" mark and never
appears on something you can press. An inline affordance — `Add a label` — is
`INLINE_ACTION`: the 쪽빛 accent thread, upright, the same ink as an issue key,
which is the other thing in a row you can click. The two share no class.

**A section label is `.section-label`, and its signal is weight, not
case.** Section headings through ~20 files wore a copied utility dialect —
micro, medium, uppercase, tracked, muted — whose label-ness lived in the
uppercase. Hangul has no case: once the Korean line-breaking work turned
the uppercase utility off per language, a Korean section label was small
muted medium text — the same costume as metadata, and the hierarchy the
audit named collapsed (GDK-141). The class owns the whole recipe (11px /
600 / 0.025em / muted, in `app.css`'s components layer), and 600 is the
one signal every script in the product renders — the ko and ja system
stacks ship a real SemiBold — so the label tier is the same weight in
every language, with uppercase kept as a Latin-only bonus that the same
owner switches off per language. Rounded badge chips (the doc badge, the
palette badge) keep the utility dialect: their shape already says what
they are. Gated in `chrome-vocabulary.test.ts`: the utility dialect is
banned everywhere a rounded chip does not justify it.

**A focus trap agrees with the browser about what is focusable.** A roving
tabindex parks `tabindex="-1"` on real controls; `lib/focus-trap.ts` excludes
that on every clause, so Tab inside a dialog stops where the browser would.

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
[`specs/001-dedicated-browser/spec.md`](../../specs/001-dedicated-browser/spec.md) (2026-08-14)

Coaching grammar (§14): the sources are listed in [`THEORY.md`](THEORY.md).
