# MCP server

`gadak mcp` speaks the [Model Context Protocol](https://modelcontextprotocol.io/)
over **stdio JSON-RPC 2.0**. It is a thin wrapper around the same local SQLite
mirror that `gadak sql`, `gadak issue`, and `gadak search` already expose. It
does not write to the mirror or to Jira. `gadak_show` writes only a local
ui-focus file so the running app can present a view (SQL answers; show presents).

The agent contract is `specs/000-product/contracts/agent.md`. This page is the
setup and troubleshooting guide.

## When not to use it

If the agent has a shell, **prefer the CLI and SQL**:

| Reach for | When |
| --- | --- |
| `gadak sql` / `sqlite3 ~/.gadak/gadak.db` | Relational, aggregated, or historical questions |
| `gadak issue` / `gadak search` | One key, or free-text recall |
| `gadak comment` / `transition` / `assign` | Writes (MCP does not write to Jira or the mirror) |
| **`gadak mcp`** | The client has **no shell** (Claude Desktop, some IDE hosts) |

MCP is deliberately not the primary interface. Every tool schema is context the
model must read before it can act; SQL is not a guess about which questions will
be asked. Use MCP only when the host cannot run `gadak` as a subprocess with a
normal argv/stdio pipe for one-shot commands. If the agent has a shell and you
want schema/query knowledge without a server process, prefer
`gadak skill install` (Claude Code skill) — see [`docs/AGENT_SETUP.md`](AGENT_SETUP.md).

## Start

```bash
gadak mcp
# or a named workspace:
gadak --workspace demo mcp
```

- **stdin / stdout**: JSON-RPC frames only (one JSON object per line).
- **stderr**: diagnostics. Clients may ignore them; never mix logs into stdout.
- **State**: none beyond the mirror file. Each request is independent.

The process exits when stdin closes.

## Install (pin the workspace)

MCP hosts do **not** inherit your shell environment. A bare `gadak mcp` in a
client config therefore resolves the **default** workspace even if you always
`export GADAK_WORKSPACE=work` in the terminal. `gadak mcp install` bakes the
current workspace (`--workspace` / `GADAK_WORKSPACE`; `--profile` /
`GADAK_PROFILE` still work as aliases) and this binary's absolute
path into the registration. The argv it writes still uses `--profile`.

```bash
gadak mcp install claude              # exec: claude mcp add gadak -- <abs> mcp
gadak --workspace demo mcp install claude
gadak mcp install claude --dry-run    # print the command only
gadak mcp install cursor              # paste block for .cursor/mcp.json
gadak mcp install codex               # paste block for ~/.codex/config.toml
gadak mcp install raycast             # form values for Raycast's Install New Server
gadak mcp install json                # mcpServers JSON snippet
gadak mcp install                     # list clients
```

`claude` runs `claude mcp add` when the binary is on `PATH`; if it is missing,
the error prints the manual command. If the server name is already registered,
gadak shows claude's message, prints `already registered`, and exits 0.
`cursor` / `codex` / `json` never exec — they only print. `raycast` prints the
values for its Install New Server form (see below).

## Claude Desktop

Add a server entry to the Claude Desktop MCP config (path varies by OS; on macOS
it is typically `~/Library/Application Support/Claude/claude_desktop_config.json`):

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

### Named workspace

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
full path to the binary in `"command"`.

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

Exactly five tools. There is no plan to add one tool per question — `gadak_query`
plus the schema in `specs/000-product/data-model.md` subsumes pre-baked queries.
`gadak_show` is presentation, not another way to answer.

| Tool | Arguments | Returns |
| --- | --- | --- |
| `gadak_query` | `{sql: string, limit?: number}` | `{columns, rows, count, truncated?, …}` — **SELECT/WITH only**, default limit 200, hard max 1000, byte-capped |
| `gadak_search` | `{query: string, limit?: number}` (aliases: `text`, `q`) | `{total, issues: [{key, summary, status}], pages, matches}` via FTS; `matches` is key → `{field, snippet}` (plain text) |
| `gadak_issue` | `{key: string}` \| `{keys: string[]}` (exactly one) | Full detail (`description_text`, comments, history, links, `dev_links`, wiki cross-refs) plus list fields. One key is a single document; several keys wrap as `{issues, missing?}` |
| `gadak_status` | `{}` | Watermark, version, last_error, row counts, kind, frozen |
| `gadak_show` | `{jql}` \| `{keys: string[]}` \| `{issue}` \| `{name}` (exactly one) | `{hash, applied, unsupported, file}` — writes the process workspace's ui-focus file; the running window picks it up (500 ms visible / 2 min TTL); does not open a window or return issue rows |

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
