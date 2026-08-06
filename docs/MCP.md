# MCP server

`scry mcp` speaks the [Model Context Protocol](https://modelcontextprotocol.io/)
over **stdio JSON-RPC 2.0**. It is a thin, read-only wrapper around the same
local SQLite mirror that `scry sql`, `scry issue`, and `scry search` already
expose.

The agent contract is `specs/000-product/contracts/agent.md`. This page is the
setup and troubleshooting guide.

## When not to use it

If the agent has a shell, **prefer the CLI and SQL**:

| Reach for | When |
| --- | --- |
| `scry sql` / `sqlite3 ~/.scry/scry.db` | Relational, aggregated, or historical questions |
| `scry issue` / `scry search` | One key, or free-text recall |
| `scry comment` / `transition` / `assign` | Writes (MCP has **no** write tools) |
| **`scry mcp`** | The client has **no shell** (Claude Desktop, some IDE hosts) |

MCP is deliberately not the primary interface. Every tool schema is context the
model must read before it can act; SQL is not a guess about which questions will
be asked. Use MCP only when the host cannot run `scry` as a subprocess with a
normal argv/stdio pipe for one-shot commands.

## Start

```bash
scry mcp
# or a named profile:
scry --profile demo mcp
```

- **stdin / stdout**: JSON-RPC frames only (one JSON object per line).
- **stderr**: diagnostics. Clients may ignore them; never mix logs into stdout.
- **State**: none beyond the mirror file. Each request is independent.

The process exits when stdin closes.

## Install (pin the profile)

MCP hosts do **not** inherit your shell environment. A bare `scry mcp` in a
client config therefore resolves the **default** profile even if you always
`export SCRY_PROFILE=work` in the terminal. `scry mcp install` bakes the
current profile (global `--profile` / `SCRY_PROFILE`) and this binary's absolute
path into the registration.

```bash
scry mcp install claude              # exec: claude mcp add scry -- <abs> mcp
scry --profile demo mcp install claude
scry mcp install claude --dry-run    # print the command only
scry mcp install cursor              # paste block for .cursor/mcp.json
scry mcp install codex               # paste block for ~/.codex/config.toml
scry mcp install json                # mcpServers JSON snippet
scry mcp install                     # list clients
```

`claude` runs `claude mcp add` when the binary is on `PATH`; if it is missing,
the error prints the manual command. If the server name is already registered,
scry shows claude's message, prints `already registered`, and exits 0.
`cursor` / `codex` / `json` never exec — they only print.

## Claude Desktop

Add a server entry to the Claude Desktop MCP config (path varies by OS; on macOS
it is typically `~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "scry": {
      "command": "scry",
      "args": ["mcp"]
    }
  }
}
```

### Profile

```json
{
  "mcpServers": {
    "scry-demo": {
      "command": "scry",
      "args": ["--profile", "demo", "mcp"]
    }
  }
}
```

### Custom home (SCRY_HOME)

```json
{
  "mcpServers": {
    "scry": {
      "command": "scry",
      "args": ["mcp"],
      "env": {
        "SCRY_HOME": "/path/to/your/scry-home"
      }
    }
  }
}
```

If `scry` is not on the absolute `PATH` that the desktop app inherits, use the
full path to the binary in `"command"`.

### Other clients

Any MCP host that can spawn a stdio server:

```json
{
  "command": "scry",
  "args": ["mcp"]
}
```

Protocol version spoken: `2025-03-26`. If the client requests another version,
scry answers with its own version and does not reject the session.

## Tools

Exactly four tools. There is no plan to add one tool per question — `scry_query`
plus the schema in `specs/000-product/data-model.md` subsumes pre-baked queries.

| Tool | Arguments | Returns |
| --- | --- | --- |
| `scry_query` | `{sql: string, limit?: number}` | `{columns, rows, count, truncated?, …}` — **SELECT/WITH only**, default limit 200, hard max 1000, byte-capped |
| `scry_search` | `{text: string, limit?: number}` | `{total, issues: [{key, summary, status}], pages, matches}` via FTS; `matches` is key → `{field, snippet}` (plain text) |
| `scry_issue` | `{key: string}` | Full detail (comments, history, links) plus list fields |
| `scry_status` | `{}` | Watermark, version, last_error, row counts |

### Filtering rule (same as the CLI)

```sql
WHERE status = 'In Progress'          -- WRONG: empty on a non-English account
WHERE status_category = 'inprogress'  -- RIGHT: new | inprogress | done
```

### Errors

- **Bad SQL / unknown key / missing mirror**: tool result with `isError: true` and
  a human-readable message. Fix and retry; this is not a JSON-RPC failure.
- **Unknown tool / malformed request**: JSON-RPC error (`-32601` / `-32602` / …).
- **No database**: message points at `scry init && scry sync`.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| Client fails to start the server | `which scry`; use an absolute path in config. Run `scry mcp` in a terminal and send a line of JSON. |
| Tools error with “no mirror” | `scry status --json` or `scry init && scry sync`. Confirm `SCRY_HOME` / `--profile` match the config. |
| Empty query results for status names | Filter on `status_category` / ids, not localized display names. |
| Client parses garbage | Something logged to stdout. Only `scry mcp` should own stdout; do not wrap it in a script that echoes. |
| Truncated rows | Raise `limit` (max 1000) or select fewer columns; the result body says when it was cut. |
| Want to leave a comment | Use `scry comment` or the REST write API — not MCP. |

### Manual stdio smoke

```bash
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
  | scry mcp
```

Each stdout line must be a single JSON-RPC object. Logs, if any, appear only on
stderr.

## Implementation notes

- Package: `internal/mcp` (stdlib JSON-RPC; no MCP SDK dependency).
- Entry: `scry mcp` in `cmd/scry`.
- Reads open the mirror the same way as the rest of the CLI (`config.DBPath()`).
- Query path uses SQLite `mode=ro` and rejects non-`SELECT`/`WITH` statements.
