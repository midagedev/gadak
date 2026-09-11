# MCP server

`gadak mcp` speaks the [Model Context Protocol](https://modelcontextprotocol.io/)
over **stdio JSON-RPC 2.0**. It is a thin wrapper around the same local SQLite
mirror that `gadak sql`, `gadak issue`, and `gadak search` already expose. Tools
do not write to the mirror or to Jira. Two of them write locally and nowhere
else: `gadak_show` writes a ui-focus file so the running app can present a view
(SQL answers; show presents), and `gadak_ui_set` writes this workspace's
`config.json` so a shell-less host can adjust the user's design tokens.

The agent contract is `specs/000-product/contracts/agent.md`. This page is the
setup and troubleshooting guide.

## When not to use it

If the agent has a shell, **prefer the CLI and SQL**:

| Reach for | When |
| --- | --- |
| `gadak sql` / `sqlite3 ~/.gadak/gadak.db` | Relational, aggregated, or historical questions |
| `gadak issue` / `gadak search` | One key, or free-text recall |
| `gadak comment` / `transition` / `assign` | Writes (MCP does not write to Jira or the mirror) |
| `gadak config get/set ui.tokens…` | Design tokens (MCP mirrors this pair as `gadak_ui_tokens` / `gadak_ui_set`) |
| **`gadak mcp`** | The client has **no shell** (Claude Desktop, some IDE hosts) |

MCP is deliberately not the primary interface. Every tool schema is context the
model must read before it can act; SQL is not a guess about which questions will
be asked. Use MCP only when the host cannot run `gadak` as a subprocess with a
normal argv/stdio pipe for one-shot commands. If the agent has a shell and you
want schema/query knowledge without a server process, prefer the skill —
`gadak skill install <client>`, where the client is Claude Code, Codex, Cursor,
Gemini CLI, OpenCode or grok. The same file serves all of them; only the path
differs. See [`docs/AGENT_SETUP.md`](AGENT_SETUP.md).

## Start

```bash
gadak mcp
```

For a named workspace:

```bash
gadak --workspace demo mcp
```

- **stdin / stdout**: JSON-RPC frames only (one JSON object per line).
- **stderr**: diagnostics. Clients may ignore them; never mix logs into stdout.
- **State**: none beyond the mirror file. Each request is independent.

The process exits when stdin closes.

While the server is running it keeps the mirror fresh on the same incremental
loop `gadak serve` uses (`syncIntervalSec`; `--no-sync` turns the loop off —
fixtures and demos). The loop starts only when the workspace has a credential
(the built-in tracker included) and is not frozen, and it stops when stdin closes.
A missing mirror still starts the protocol; the loop is not forced on that path.
Tools still do not write to Jira or the mirror.

## Install (pin the workspace)

MCP hosts do **not** inherit your shell environment. A bare `gadak mcp` in a
client config therefore resolves the **default** workspace even if you always
`export GADAK_WORKSPACE=work` in the terminal. `gadak mcp install` bakes the
current workspace (`--workspace` / `GADAK_WORKSPACE`; `--profile` /
`GADAK_PROFILE` still work as aliases) and this binary's absolute
path into the registration. The argv it writes still uses `--profile`.

```bash
gadak mcp install claude
```

`claude` runs `claude mcp add` when the binary is on `PATH`; if it is missing,
the error prints the manual command. If the server name is already registered,
gadak shows claude's message, prints `already registered`, and exits 0.

To pin a named workspace:

```bash
gadak --workspace demo mcp install claude
```

| Flag | Effect |
| --- | --- |
| `--dry-run` | print the command (claude), the merged config (claude-desktop), or the config without registering |

Besides claude, one client writes (claude-desktop), and the rest only print (`raycast` prints form values; see [Raycast](#raycast) below):

| Client | Command | What it does |
| --- | --- | --- |
| claude | `gadak mcp install claude` | Claude Code: runs `claude mcp add` (registers with Claude Code's own config) |
| claude-desktop | `gadak mcp install claude-desktop` | Claude Desktop: merges the gadak entry into `claude_desktop_config.json` |
| cursor | `gadak mcp install cursor` | prints Cursor MCP config to paste (`.cursor/mcp.json`) |
| codex | `gadak mcp install codex` | prints Codex MCP config to paste (`~/.codex/config.toml`) — but Codex loads skills, so `gadak skill install codex` is usually the better route |
| raycast | `gadak mcp install raycast` | prints values to fill into Raycast's Install New Server form |
| json | `gadak mcp install json` | prints a `mcpServers` JSON snippet only |

With no client, `gadak mcp install` lists those clients.

## Claude Desktop

Claude Desktop is a different app from Claude Code, with a different config —
and no shell, so nothing can run the CLI for it. The install verb writes the
file itself:

```bash
gadak mcp install claude-desktop
```

This merges a `gadak` entry into Claude Desktop's `claude_desktop_config.json`,
keeping the file's other keys and servers untouched. The entry pins this
workspace and this binary's absolute path, same as `claude` does for Claude
Code. The file's location by OS:

| OS | Path |
| --- | --- |
| macOS | `~/Library/Application Support/Claude/claude_desktop_config.json` |
| Windows | `%APPDATA%\Claude\claude_desktop_config.json` |
| Linux | `$XDG_CONFIG_HOME/Claude/claude_desktop_config.json`, else `~/.config/Claude/claude_desktop_config.json` |

If the file is missing it is created (`{}` plus the entry); if it holds
something that is not a JSON object, gadak refuses and leaves the file
untouched. An unchanged entry prints `already registered` and exits 0. Restart
Claude Desktop to load a change — it reads the file at startup. `--dry-run`
prints the merged config without writing.

To pin a named workspace:

```bash
gadak --workspace demo mcp install claude-desktop
```

The entry it writes:

```json
{
  "mcpServers": {
    "gadak": {
      "command": "gadak",
      "args": ["mcp"]
    }
  }
}
```

If you would rather edit the file by hand, that is the shape — under a named
workspace, `"args": ["--workspace", "demo", "mcp"]` (a second registration can
also use a different server name, which the install verb never does):

```json
{
  "mcpServers": {
    "gadak-demo": {
      "command": "gadak",
      "args": ["--workspace", "demo", "mcp"]
    }
  }
}
```

### Custom home (GADAK_HOME)

```json
{
  "mcpServers": {
    "gadak": {
      "command": "gadak",
      "args": ["mcp"],
      "env": {
        "GADAK_HOME": "/path/to/your/gadak-home"
      }
    }
  }
}
```

If `gadak` is not on the absolute `PATH` that the desktop app inherits, use the
full path to the binary in `"command"` (the install verb always writes the full
path). `gadak mcp install claude-desktop` does not set `env` — a custom
`GADAK_HOME` is a hand edit on top of the entry it writes.

### Other clients

Any MCP host that can spawn a stdio server:

```json
{
  "command": "gadak",
  "args": ["mcp"]
}
```

Protocol version spoken: `2025-03-26`. If the client requests another version,
gadak answers with its own version and does not reject the session.

## Raycast

Raycast (1.98+) speaks MCP over stdio but exposes **no config file**: its manual
documents no settings path, and none exists on disk. Registration is form-only:

```bash
gadak mcp install raycast
```

This prints the values for Raycast → **Manage MCP Servers** → **Install New
Server**: Name `gadak`, Transport *Standard Input/Output*, Command = this
binary's absolute path, Arguments `mcp` (or `--workspace <name> mcp` when a
workspace is set — the flag comes first, as it does on the command line).
Raycast's AI/MCP features may require a paid plan.

## Tools

Nine tools: seven reads, one presentation act, one local write. There is no plan
to add one tool per question — `gadak_query` plus the schema in
`specs/000-product/data-model.md` subsumes pre-baked queries. `gadak_show` is
presentation, not another way to answer, `gadak_retro` is the one report that
is not a query (sessions, resume medians, cycle percentiles, the reopen
surfaces — computed together), `gadak_recents` reads back the local trail the
read tools leave (the first call after a context compaction), and the `ui` pair
is the settings surface a host without a shell otherwise cannot reach.

| Tool | Arguments | Returns |
| --- | --- | --- |
| `gadak_query` | `{sql: string, limit?: number}` | `{columns, rows, count, truncated?, …}` — **SELECT/WITH only**, default limit 200, hard max 1000, byte-capped |
| `gadak_search` | `{query: string, limit?: number}` (aliases: `text`, `q`) | `{total, issues: [{key, summary, status}], pages, matches}` via FTS; `matches` is key → `{field, snippet}` (plain text) |
| `gadak_issue` | `{key: string}` \| `{keys: string[]}` (exactly one) | Full detail (`description_text`, comments, history, links, `dev_links`, wiki cross-refs) plus list fields. One key is a single document; several keys wrap as `{issues, missing?}` |
| `gadak_status` | `{}` | Watermark, version, last_error, row counts, kind, frozen |
| `gadak_show` | `{jql}` \| `{keys: string[]}` \| `{issue}` \| `{name}` (exactly one) | `{hash, applied, unsupported, file}` — writes the process workspace's ui-focus file; the running window picks it up (500 ms visible / 2 min TTL); does not open a window or return issue rows |
| `gadak_retro` | `{since?, session-gap?, by-sprint?, board?, open?, week?}` (the flags of `gadak retro`) | The document `gadak retro --json` prints — one column per ISO week or sprint with sessions, resume median, WIP age, closed, cycle p50/p85, mismatch, plus the materials, the aging tail and the definitions. The per-column event log and key lists are left out (`omitted` says so); `open` names one cell and answers its issue keys |
| `gadak_recents` | `{limit?: number}` (the flag of `gadak recents`) | `{recents: [{kind, key, viewed_at}]}` — this machine's read history, one row per distinct `(kind, key)` folded to its newest read, newest first (default 20). `gadak_issue` and `gadak_search` append to that trail as they run, with `source = 'mcp'`; keys read under a previous origin never resurface |
| `gadak_ui_tokens` | `{axis?}` | `{axes, targets, tokens, rules, catalog, warnings, config_file, config_version}` — the stored `ui.tokens` overrides plus `ui.tokensByTheme` and `ui.dataColors`. With **no** argument the read-only catalogs are left out (they are tens of KB); naming an `axis` adds that axis's catalog, from the same owners `gadak config get ui.tokens.catalog` / `ui.tokens.dim-catalog` read |
| `gadak_ui_set` | `{axis, values: {token: string \| null}, palette?}` | `{axis, path, saved, tokens, previous, warnings, …}` — the same key-wise merge as `gadak config set ui.tokens.<axis>`; `palette` merges into `ui.tokensByTheme.<palette>` and `axis: "dataColors"` into `ui.dataColors` (`<family>.<key>`). Unparseable values and the derived `layout.docked-min` are **refused** by name; locked tiers, contrast floors, ranges and relations **warn and save**. `previous` is the undo patch: pass it back as `values`. Writes `config.json` only |

### Filtering rule (same as the CLI)

```sql
WHERE status = 'In Progress'          -- WRONG: empty on a non-English account
WHERE status_category = 'inprogress'  -- RIGHT: new | inprogress | done
```

### Errors

- **Bad SQL / unknown key / missing mirror**: tool result with `isError: true` and
  a human-readable message. Fix and retry; this is not a JSON-RPC failure.
- **Unknown tool / malformed request**: JSON-RPC error (`-32601` / `-32602` / …).
- **No database**: message points at `gadak init && gadak sync`.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| Client fails to start the server | `which gadak`; use an absolute path in config. Run `gadak mcp` in a terminal and send a line of JSON. |
| Tools error with “no mirror” | `gadak status --json` or `gadak init && gadak sync`. Confirm `GADAK_HOME` / `--workspace` match the config. |
| Empty query results for status names | Filter on `status_category` / ids, not localized display names. |
| Client parses garbage | Something logged to stdout. Only `gadak mcp` should own stdout; do not wrap it in a script that echoes. |
| Truncated rows | Raise `limit` (max 1000) or select fewer columns; the result body says when it was cut. |
| Want to leave a comment | Use `gadak comment` or the REST write API — not MCP. |

### Manual stdio smoke

```bash
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
  | gadak mcp
```

Each stdout line must be a single JSON-RPC object. Logs, if any, appear only on
stderr.

## Implementation notes

- Package: `internal/mcp` (stdlib JSON-RPC; no MCP SDK dependency).
- Entry: `gadak mcp` in `cmd/gadak`.
- Reads open the mirror the same way as the rest of the CLI (`config.DBPath()`).
- Query path uses SQLite `mode=ro` and rejects non-`SELECT`/`WITH` statements.
