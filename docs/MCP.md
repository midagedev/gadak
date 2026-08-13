# MCP server

`gadak mcp` speaks the [Model Context Protocol](https://modelcontextprotocol.io/)
over **stdio JSON-RPC 2.0**. It is a thin, read-only wrapper around the same
local SQLite mirror that `gadak sql`, `gadak issue`, and `gadak search` already
expose.

The agent contract is `specs/000-product/contracts/agent.md`. This page is the
setup and troubleshooting guide.

## When not to use it

If the agent has a shell, **prefer the CLI and SQL**:

| Reach for | When |
| --- | --- |
| `gadak sql` / `sqlite3 ~/.gadak/gadak.db` | Relational, aggregated, or historical questions |
| `gadak issue` / `gadak search` | One key, or free-text recall |
| `gadak comment` / `transition` / `assign` | Writes (MCP has **no** write tools) |
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
# or a named profile:
gadak --profile demo mcp
```

- **stdin / stdout**: JSON-RPC frames only (one JSON object per line).
- **stderr**: diagnostics. Clients may ignore them; never mix logs into stdout.
- **State**: none beyond the mirror file. Each request is independent.

The process exits when stdin closes.

## Install (pin the profile)

MCP hosts do **not** inherit your shell environment. A bare `gadak mcp` in a
client config therefore resolves the **default** profile even if you always
`export GADAK_PROFILE=work` in the terminal. `gadak mcp install` bakes the
current profile (global `--profile` / `GADAK_PROFILE`) and this binary's absolute
path into the registration.

```bash
gadak mcp install claude              # exec: claude mcp add gadak -- <abs> mcp
gadak --profile demo mcp install claude
gadak mcp install claude --dry-run    # print the command only
gadak mcp install cursor              # paste block for .cursor/mcp.json
gadak mcp install codex               # paste block for ~/.codex/config.toml
gadak mcp install json                # mcpServers JSON snippet
gadak mcp install                     # list clients
```

`claude` runs `claude mcp add` when the binary is on `PATH`; if it is missing,
the error prints the manual command. If the server name is already registered,
gadak shows claude's message, prints `already registered`, and exits 0.
`cursor` / `codex` / `json` never exec — they only print.

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

### Profile

```json
{
  "mcpServers": {
    "gadak-demo": {
      "command": "gadak",
      "args": ["--profile", "demo", "mcp"]
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

## Tools

Exactly four tools. There is no plan to add one tool per question — `gadak_query`
plus the schema in `specs/000-product/data-model.md` subsumes pre-baked queries.

| Tool | Arguments | Returns |
| --- | --- | --- |
| `gadak_query` | `{sql: string, limit?: number}` | `{columns, rows, count, truncated?, …}` — **SELECT/WITH only**, default limit 200, hard max 1000, byte-capped |
| `gadak_search` | `{text: string, limit?: number}` | `{total, issues: [{key, summary, status}], pages, matches}` via FTS; `matches` is key → `{field, snippet}` (plain text) |
| `gadak_issue` | `{key: string}` | Full detail (comments, history, links) plus list fields |
| `gadak_status` | `{}` | Watermark, version, last_error, row counts |

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
| Tools error with “no mirror” | `gadak status --json` or `gadak init && gadak sync`. Confirm `GADAK_HOME` / `--profile` match the config. |
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
