# v0.13 tasks

Track boundaries are disjoint on purpose; tracks run in parallel worktrees.
Every task lands with its evidence noted here (command + observed output),
same discipline as specs/000-product/tasks.md.

Status: `[ ]` open · `[x]` done+evidence · `[-]` dropped (say why).

## Track D — docs (docs/ + README.md + SECURITY.md only)

- [ ] D1. CONCEPT.md: "The browser it replaces" section — the statement,
      the hierarchy, contain-don't-reimplement; extend the optimized loop
      with the agent handoff step. Keep the existing insight text.
- [ ] D2. UX_PRINCIPLES.md: guardrails G-a/G-b/G-c as principles, with the
      tab-strip/pill named as the component G-c constrains.
- [ ] D3. ROADMAP.md: v0.13 wave section; amend "Next" per spec.md
      (PR #1 = first arrival signal; parallel, not displaced).
- [ ] D4. SECURITY.md: WKWebView cookie session = second credential
      surface, desktop-only, distinct from the API token.
- [ ] D5. ARCHITECTURE.md: drop the removed `presence/` line (doc rot).
- [ ] D6. AGENTS.md + docs: `gadak open` = Jira escape hatch vs
      `gadak views open` = open in gadak, said where both are introduced.

## Track G — Go (internal/jql/, cmd/gadak/views.go + views tests,
## internal/uifocus/, internal/server/focus.go + jql/focus tests)

- [x] G1. `keys` axis in internal/jql per pinned contract: types (Keys,
      `[]` marshal), compile (`key/issuekey/issue` = / IN → Keys; the q
      stuffing dies), emit (`key =` / `key in (…)`), match (exact,
      case-insensitive), hash (`ks=`, comma-joined, order preserved).
      FAIL-first: `TestKeyInMatchAndEmit` on HEAD — parse `"expected ) at 11"`
      (unquoted hyphen), then after lexer-only `"key IN stuffed q: \"NMA-1 NMA-2\""`;
      now PASS (`go test ./internal/jql/ -run TestKeyInMatchAndEmit`).
- [x] G2. `views open --keys 'A,B C'` (comma/whitespace), `--keys -`
      (stdin, one key per line or whitespace; pipes from `gadak sql`),
      cap 500 with a loud error; `--keys` composes with nothing else
      (exclusive with `--jql` and a positional name).
      `TestViewsOpenKeysNoMirror` / `Stdin` / `Exclusive` / `Cap`; binary
      `{"hash":"ks=NMA-1,NMA-2",…}` and `echo NMA-1 | … --keys -` → `hash\tks=NMA-1`.
- [x] G3. `views open NMB-140`: a positional matching `^[A-Z][A-Z0-9]*-\d+$`
      that is not a stored view name focuses detail — hash `issue=KEY`.
      Stored-view names win when both match.
      `TestViewsOpenPositionalIssueKey` (`issue=NMB-140`); `TestViewsOpenStoredViewWinsOverKeyShape`.
- [x] G4. Print the link: `views open` (and `--json`) always prints the
      hash and, when a serve base is found, the full URL — even under
      `--no-open`. Revive `jql.HashURL` for it or delete the dead helper.
      `serveFocusURL` always resolves; `QueryURL`/`HashURL` revived;
      `TestComposeServeURLAndPrefix`.
- [x] G5. Multi-profile focus: `GET /w/<name>/api/v1/issues/ui-focus/`
      reads that profile's file (workspace server passes its profile to
      uifocus), and `openServeOnHash` prefixes `/w/<name>/` when the
      target profile is not the serve process's primary. Test: workspace
      handler consumes the right file.
      `TestHandlerUIFocusReadsProfileFile` + `TestWorkspaceFocusTakesProfileFile`
      + `TestTakeForUsesProfileDir`.
- [x] G6. `gadak search --jql 'key in (…)'` returns those issues (falls
      out of G1 match; assert it).
      `TestSearchJQLKeysReturnsThoseIssues`: `key in (NMB-1)` → NMB-1; `key in (NMA-999)` misses.

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

## Deferred, named

- MCP `gadak_show` — owner call needed: revises the "read-only, four
  tools" contract (census 04 lists the full touch set when approved).
- fsnotify → Wails event for desktop focus latency (census 05 option E) —
  only if 2 GET/s visible-tab polling ever matters.
- Favorite docs; `gadak open <url>`; workspace-aware `gadak open` — out of
  wave, recorded here so they are not re-derived.
