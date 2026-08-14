# v0.13 tasks

Track boundaries are disjoint on purpose; tracks run in parallel worktrees.
Every task lands with its evidence noted here (command + observed output),
same discipline as specs/000-product/tasks.md.

Status: `[ ]` open · `[x]` done+evidence · `[-]` dropped (say why).

## Track D — docs (docs/ + README.md + SECURITY.md only)

- [x] D1. CONCEPT.md: "The browser it replaces" section — the statement,
      the hierarchy, contain-don't-reimplement; extend the optimized loop
      with the agent handoff step. Keep the existing insight text.
      Evidence: `docs/CONCEPT.md` new section + loop step 6; insight
      paragraph unchanged.
- [x] D2. UX_PRINCIPLES.md: guardrails G-a/G-b/G-c as principles, with the
      tab-strip/pill named as the component G-c constrains.
      Evidence: §11–§13 name tab strip + re-entry pill.
- [x] D3. ROADMAP.md: v0.13 wave section; amend "Next" per spec.md
      (PR #1 = first arrival signal; parallel, not displaced).
      Evidence: `docs/ROADMAP.md` v0.13 + rewritten Next.
- [x] D4. SECURITY.md: WKWebView cookie session = second credential
      surface, desktop-only, distinct from the API token.
      Evidence: new "The in-app page session (desktop only)" section.
- [x] D5. ARCHITECTURE.md: drop the removed `presence/` line (doc rot).
      Evidence: `grep -n presence docs/ARCHITECTURE.md` → 0.
- [x] D6. AGENTS.md + docs: `gadak open` = Jira escape hatch vs
      `gadak views open` = open in gadak, said where both are introduced.
      Evidence: `AGENTS.md` CLI reference prose + comments; ROADMAP v0.13
      restates the pair.

## Track G — Go (internal/jql/, cmd/gadak/views.go + views tests,
## internal/uifocus/, internal/server/focus.go + jql/focus tests)

- [ ] G1. `keys` axis in internal/jql per pinned contract: types (Keys,
      `[]` marshal), compile (`key/issuekey/issue` = / IN → Keys; the q
      stuffing dies), emit (`key =` / `key in (…)`), match (exact,
      case-insensitive), hash (`ks=`, comma-joined, order preserved).
      FAIL-first: a test on current HEAD proving `key in (A,B)` yields an
      empty match set / lossy emit, committed with the fix flipping it.
- [ ] G2. `views open --keys 'A,B C'` (comma/whitespace), `--keys -`
      (stdin, one key per line or whitespace; pipes from `gadak sql`),
      cap 500 with a loud error; `--keys` composes with nothing else
      (exclusive with `--jql` and a positional name).
- [ ] G3. `views open NMB-140`: a positional matching `^[A-Z][A-Z0-9]*-\d+$`
      that is not a stored view name focuses detail — hash `issue=KEY`.
      Stored-view names win when both match.
- [ ] G4. Print the link: `views open` (and `--json`) always prints the
      hash and, when a serve base is found, the full URL — even under
      `--no-open`. Revive `jql.HashURL` for it or delete the dead helper.
- [ ] G5. Multi-profile focus: `GET /w/<name>/api/v1/issues/ui-focus/`
      reads that profile's file (workspace server passes its profile to
      uifocus), and `openServeOnHash` prefixes `/w/<name>/` when the
      target profile is not the serve process's primary. Test: workspace
      handler consumes the right file.
- [ ] G6. `gadak search --jql 'key in (…)'` returns those issues (falls
      out of G1 match; assert it).

## Track W — web (web/src/ + e2e/, no Go)

- [ ] W1. `keys` axis: ViewFilters.keys, `ks=` in MULTI_KEY/parse/
      serialize/emptyFilters, filterIssues membership (OR, exact key),
      keys chip ("N keys", clearable), given-order sort branch when keys
      active and sort is default — and the `q`→relevance promote must not
      fire for keys (census 03 §given-order, all five hook points).
- [ ] W2. Omnibox paste routing per pinned contract (SearchBox onPaste +
      Enter path): /browse/KEY → selection or miss-toast; filter=<id> →
      synced source_queries row or existing toast; wiki page id →
      pages.select; other same-site → desktop in-app tab / serve system
      browser. Nothing silently swallowed (the `not_jql` false return
      path dies).
- [ ] W3. Body-link click routing: ADF `/browse/KEY` and mirrored wiki
      links → native panels on both surfaces; header key unchanged
      (escape hatch); unmodeled links unchanged.
- [ ] W4. ui-focus poll pauses while `document.hidden` (keep 500 ms
      visible; visibilitychange already covers the return). One spec
      asserts no ui-focus requests while hidden.
- [ ] W5. Palette: imported Jira filters (views.source) join the view
      section, same matching as saved views.
- [ ] W6. Recents finish §6: browse-tab issue/page visits record recents;
      sidebar recents gain the doc slice (or a mixed list — pick the
      smaller diff, state which).
- [ ] W7. e2e: keys view via ui-focus lands exact list in given order;
      `#/?issue=` focuses detail; /browse/KEY paste selects natively.

## Track A — agent surface (skills/gadak/, docs/AGENT_*.md, docs/RECIPES.md,
## specs/000-product/contracts/agent.md) — after Track G flags exist

- [ ] A1. SKILL.md: "show, don't paste" — a trigger in the front-matter
      description ("user wants to *see* issues → put them on the app"),
      a decision rule up top (table only when no UI or the user asked for
      a document), `gadak sql … | gadak views open --keys -` recipe,
      `gadak open` vs `views open` disambiguation.
- [ ] A2. AGENT_SETUP.md: Cursor and Codex blocks gain the views lines
      Claude Code already has.
- [ ] A3. AGENT_ACCESS.md: presentation layer row (SQL answers; views
      present); agent.md gets the same ranking sentence.
- [ ] A4. RECIPES.md: one worked recipe ends in `views open --keys -`.
- [ ] A5. Changelog entry for the wave (Unreleased).

## Track M — MCP `gadak_show` (after Track G merges; internal/mcp/ +
## contracts/agent.md + docs/MCP.md)

- [ ] M1. Contract revision first: contracts/agent.md and docs/MCP.md
      restate the MCP rule as "no writes to the mirror or to Jira";
      presentation (ui-focus) is a permitted local act, ranked below SQL
      (SQL answers; show presents).
- [ ] M2. `gadak_show` tool: input `{jql} | {keys: [..]} | {issue} | {name}`
      (exactly one), compiles through internal/jql (keys → `ks=` hash,
      issue → `issue=` hash, name → stored view via the same lookup the
      CLI uses), writes ui-focus for the process profile, returns
      `{hash, applied, unsupported}`. Never opens windows itself —
      the running UI picks the file up (500 ms visible / 2 min TTL);
      say so in the tool description.
- [ ] M3. server_test.go tool-count and name-list assertions updated
      (5 tools); a test proves write SQL is still rejected and that
      `gadak_show` writes the focus file where the profile expects it.
- [ ] M4. Every "four tools" claim swept: AGENTS.md, docs/MCP.md,
      docs/AGENT_ACCESS.md, SKILL.md coordinate with Track A.

## Deferred, named

- fsnotify → Wails event for desktop focus latency (census 05 option E) —
  only if 2 GET/s visible-tab polling ever matters.
- Favorite docs; `gadak open <url>`; workspace-aware `gadak open` — out of
  wave, recorded here so they are not re-derived.
