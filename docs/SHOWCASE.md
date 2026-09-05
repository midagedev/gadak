# Showcase

The window, the launcher, and agents driving both — on camera. Every
recording here is generated or recorded against the demo snapshot
(`examples/demo.db`, 534 issues); the spec or tape that produced each one is
linked under it, and [`project/MEDIA.md`](project/MEDIA.md) says how to
regenerate them. The README keeps one of these; the rest live here so the
front door stays short.

## One index

⌘K is the one index — titles, bodies, comments, issues and pages. The chips
on the list do not apply. That is why a comment-only word still finds the row.

<p align="center">
  <img src="media/search.gif" alt="A Project chip is on the list; ⌘K opens the palette, a comment-only word is typed, and All search fills with rows from other projects, each labelled Comment match with a snippet" width="900">
  <br>
  <sub>Generated from <a href="../e2e/demo/search-demo.spec.ts">e2e/demo/search-demo.spec.ts</a> against the demo snapshot.</sub>
</p>

## The shell is in the window

<p align="center">
  <img src="media/terminal-demo.gif" alt="In gadak's terminal pane, gadak claim names the shell's tab after the issue and moves the row; then gadak sql piped into gadak views open --keys - turns the list above into those five keys, and gadak views open --jql lands it on project, priority and unresolved chips" width="430">
  <br>
  <sub>Typed into gadak's own pane: <code>gadak claim</code> first, so the tab carries the issue's key, then the pipe and the JQL. <code>gadak views open</code> writes a one-shot hash; the list above applies it. The recording adds a priority clause — in <code>--jql</code> a priority or status name is matched as the literal string your Jira stores, which is localized, so the README example leaves it out. Generated from <a href="../e2e/demo/terminal-demo.spec.ts">e2e/demo/terminal-demo.spec.ts</a> (<code>make media-terminal</code>).</sub>
</p>

```bash
gadak sql --no-header "select key from issues_full where status_category = 'inprogress'
                       order by status_changed_at asc limit 5" | gadak views open --keys -
```

```bash
gadak views open --jql 'project = NMA AND resolution is EMPTY'
```

`gadak views open` is the "open in gadak" verb; `gadak open KEY` leaves for
Jira. The list box takes the same JQL paste as `gadak search --jql`; clauses
gadak cannot express are listed, never dropped.

## Two takes: a wall, and a look

Two takes from the same rig, each following one job to the end. Nothing in
either is scripted but the opening sentence — the commands, the HTML and the
recovery are the model's:

<table align="center">
<tr>
<td width="50%" align="center">
  <img src="media/claude-dashboards-vertical.gif" width="430" alt="Asked for a triage dashboard, a live Claude Code session queries the mirror, writes the HTML, saves it with three datasources and opens it; the wall paints status cards, a per-month line chart and a list of the oldest open issues, then an issue key on the wall is clicked and the app opens that issue">
</td>
<td width="50%" align="center">
  <img src="media/claude-tokens-vertical.gif" width="430" alt="Asked for the team look, a live Claude Code session sets the accent, the label and issue-type colors and the row height and body size; one write saves with a warning that prints the whole type ladder, and the session reads the warning and restores the step itself">
</td>
</tr>
<tr>
<td align="center"><sub><b>It builds the wall, and the wall links back.</b> A dashboard is one HTML document plus named queries. The keys it puts on the wall are real links — the frame asks the app to navigate, so a click lands on the issue instead of leaving the page.</sub></td>
<td align="center"><sub><b>It changes the look, and keeps what you asked for.</b> Token writes apply and then say how they will read; only what the machine cannot honor is refused. Here the warning names the whole type ladder and the session repairs the step on its own.</sub></td>
</tr>
</table>

<sub>Recorded from <a href="../tools/tapes/claude-dashboards.tape">claude-dashboards.tape</a> and <a href="../tools/tapes/claude-tokens.tape">claude-tokens.tape</a> against the demo snapshot. Full-resolution MP4s: <a href="media/claude-dashboards-vertical.mp4">dashboards</a> · <a href="media/claude-tokens-vertical.mp4">tokens</a>.</sub>

## Dashboards

When the answer is a wall rather than a list, author a dashboard — one HTML
document plus registered datasources, rendered sandboxed in the web tab:
[`DASHBOARDS.md`](DASHBOARDS.md).

<p align="center">
  <img src="media/dashboards.gif" alt="A terminal saves a dashboard — one HTML file plus four datasources over the mirror — and the web tab renders the triage wall: status counters and the top open issues by priority; a second save swaps the open frame in place" width="900">
  <br>
  <sub><code>gadak dashboards save</code> registers the document and its datasources; the host runs the queries and pushes rows in, and a re-save swaps an open frame in about a second. Charts come from a locally served uPlot — no CDN, no CSP widening. Generated from <a href="../e2e/demo/dashboards-demo.spec.ts">e2e/demo/dashboards-demo.spec.ts</a>.</sub>
</p>

## Colors are config

The window keeps one paper metaphor across four palettes — `light`, a
neutral-cool `dark`, blue-black `ink`, and warm `ember`. The theme follows
the system unless you pick one, and it belongs to the workspace, not the
browser: `gadak config set appearance.theme ink`. Every field the settings
dialog edits is a CLI verb over the same validation (`gadak config list`,
`gadak config set …`), so setup is not a screen an agent has to click.

<p align="center">
  <img src="media/tokens.gif" alt="A terminal sets ui.tokens and ui.dataColors and the open tab retints live — accent, chips and breakdown colors change with no reload; a write to the locked bg-base saves with a warning that names the reason" width="900">
  <br>
  <sub><code>ui.tokens</code> / <code>ui.dataColors</code> flow from the CLI into an open tab with no reload, and the keys a palette owns refuse an override instead of silently breaking the paper. Generated from <a href="../e2e/demo/tokens-demo.spec.ts">e2e/demo/tokens-demo.spec.ts</a>.</sub>
</p>

## A launcher is a surface

Two surfaces is not a closed list. Reading the mirror is one binary call
(`gadak search --json`, ~20 ms), and opening anything in the app is one URL
(`gadak://view?issue=…` — [the scheme](DESKTOP.md)), so whatever can do
those two things becomes a surface.

<p align="center">
  <img src="media/raycast.gif" alt="Raycast searches the local gadak mirror as you type — a text query shows the matched snippet in bold with a field tag, then typing the bare issue key finds that issue, and Enter opens it in the Gadak app through a gadak:// deep link" width="800">
  <br>
  <sub>Each keystroke is one <code>gadak search --json</code>; Enter is the deep link. A saved view travels the same way — <code>gadak views open</code> prints its link.</sub>
</p>

That launcher exists: a Raycast extension that searches issues and wiki
documents as you type, [submitted to the Raycast Store](https://github.com/raycast/extensions/pull/30297).
Until the review lands, one command installs it from the binary you already
have (embedded, no checkout):

```sh
gadak raycast install
```

The macOS app has the same install as a button — **Settings → Integrations**
lists Raycast, the agent skill and MCP, shows what is already installed, and
runs the exact command it prints. Building on the extension itself:
[`../contrib/raycast/`](../contrib/raycast/). With no extension at all, a
Raycast Quicklink pointed at `gadak://view?issue={argument}` covers the
open-by-key half.

## A live MCP session

For hosts without a shell (Claude Desktop), the same mirror is an MCP
server. Ask the thing Jira cannot answer at all, because the wiki is a
second search: "what do we know about X?" One index holds both, so the
answer can put a ticket and the design doc that drove it in the same
sentence.

<p align="center">
  <img src="media/mcp.gif" alt="Claude Code registers gadak as an MCP server, is asked to search Jira and the wiki for idempotency, calls gadak, and answers with an issue and the Confluence brief that drove it" width="800">
  <br>
  <sub>Five tools; no writes to the mirror or to Jira. A host with a shell can use <code>gadak sql</code> instead. Setup: <a href="MCP.md">MCP.md</a>.</sub>
</p>
