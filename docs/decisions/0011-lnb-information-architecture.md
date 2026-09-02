# 0011 — The left navigation is a return path, not a table of contents

Status: accepted 2026-09-02 (GDK-1335, parent GDK-1334)

## Context

The sidebar accreted one section per feature. On the demo fixture at
1440×900 it rendered six section headers of equal weight (My issues ·
Recently viewed · Built-in views · Jira filters · Documents · Workspaces),
a four-row footer (Create a built-in tracker workspace · Settings · account ·
Terminal), a full-width accent "New issue" block, and a count line — and the
last section scrolled off the bottom. Every row carried an icon. One count
was a pill, the rest were plain numbers.

`docs/project/UX_PRINCIPLES.md` §5 (density stays, chrome recedes) and §6
(recency beats hierarchy; sidebars must not grow with content volume) were
already the brief. The sidebar violated both.

## Decision

The sidebar answers one question — *how do I get back to what I was doing?* —
and is laid out top to bottom in the order that question is usually asked:

```
where am I         workspace switcher row (28px; the mirror list lives in its menu)
do something       [ + New issue   c ] [ ⌕ ]   quiet, bordered; kbd hints in place
how fresh          534 issues · ● Sync history   (the one freshness sentence)
MY ISSUES          Assigned to me · Reported by me · Feed        (identity rows)
RECENT      History  one line per visit: key/space · title · seen  (cap 12, yields)
FAVORITES          only when there are any
VIEWS              the seven built-ins, with their icons
SAVED VIEWS        only when there are any
JIRA FILTERS       only when there are any
DOCUMENTS          Documents · ▸ Spaces
DASHBOARDS         only when there are any
──────────────────
[⚙] dana@example.com                         [>_]   one footer row
```

Rules that decide what earns a place:

1. **A top-level section is a kind of destination, not a feature.** Views,
   documents, dashboards are kinds. "Workspaces" is a scope, so it became the
   switcher row at the top; "Create a built-in tracker workspace" is an
   errand inside that scope, so it lives in the switcher's menu.
2. **Rows are 28px, headers 24px, sections 8px apart.** The default fixture
   fits at 900px without scrolling the sidebar. Recent rows are one line
   (lead · title · time); the two-line 48px card was the single largest
   spend of vertical space.
3. **Icons only where they discriminate.** The three identity rows and the
   seven built-in views keep theirs (each icon is a different meaning). Saved
   views and Jira filters are text; the workspace row and the footer use
   glyphs because there is no room for words.
4. **One loud element at most, and it is content.** The accent-filled New
   issue block became a bordered row; the Feed unread pill stays the one
   accent chip because unread is the one signal that asks for attention.
5. **Shortcuts are shown where the action is** (§3): `c` on New issue,
   `⌘K` on the palette button's title, `Ctrl+`` on the terminal's.

## What did not change (wire)

- Section ids `builtin · jira · personal · team · dashboards · docs ·
  workspaces` stay in `SECTION_IDS`; collapse and order persist as before.
  `workspaces` is parsed from stored order and never rendered, as `team`
  already was.
- Every `data-testid` survives (`workspace-link`, `workspace-new`,
  `workspace-kind`, `local-origin-create` moved inside `workspace-menu`;
  `sidebar-terminal` moved into SidebarNav's footer).
- i18n keys are unchanged; three labels changed value (Built-in views →
  Views, My views → Saved views, Recently viewed → Recent) and two were
  added (`sidebar.workspaceSwitch`, `sidebar.searchEverything`).

## Consequences

- The recency list's yield rule (GDK-1081) holds with a 28px floor instead
  of 48px; twelve seeded visits still mount, and more of them are visible.
- Two e2e specs open the switcher before reaching `workspace-new` and
  `local-origin-create`.
- Later rounds (GDK-1336…1339) take the same rules to the list chrome, the
  detail properties, the board card and the column headers.

## Addendum 2026-09-02 (GDK-1336)

The `[ ⌕ ]` palette button beside New issue, and the `sidebar.searchEverything`
key it used, were removed one round later. The list toolbar's `palette-open`
button is the one visible door to the palette; two doors two inches apart
read as two different features. The action row is New issue alone.
