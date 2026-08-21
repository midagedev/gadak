# github-prs

Attach GitHub pull requests to mirrored issues as `kind='prs'` enrichments.

The gadak core never talks to GitHub. This script is a separate process: it
reads PRs (live API or a JSON fixture), finds issue keys like `NMB-42` in the
branch name, title, and body, and upserts rows into `enrichments`.

## Why

When someone opens an issue in the gadak UI, they want the related PRs next to
the description — without waiting for a Jira development panel, and without
baking a GitHub client into the gadak binary.

## Payload contract

`payload` is a JSON **array** of `LinkedPr` objects (see `docs/PLUGINS.md` and
`web/src/lib/types.ts`):

```json
[
  {
    "number": 42,
    "title": "Fix session timeout on NMB-110",
    "url": "https://github.com/example/app/pull/42",
    "state": "merged",
    "repo": "example/app",
    "author": "alice"
  }
]
```

`state` is `open`, `merged`, or `closed` (merged wins over GitHub's `closed`
when `merged_at` is set). The detail API exposes this as `linked_prs`. The
web PR list renders whenever `linked_prs` is non-empty
(`web/src/components/detail/DetailPanel.svelte`); it is not gated on
`features.deploy`. That flag still gates the deploy-status timeline.

## Requirements

- Python 3.9+ (stdlib only)
- Live mode: `GH_TOKEN` (or `GITHUB_TOKEN`) with `repo` read access
- A gadak mirror SQLite file (`gadak.db`)

## Quick start (offline, no token)

```sh
# from the gadak repo root
cp examples/demo.db /tmp/gadak-plugin.db

python3 examples/plugins/github-prs/github_prs.py example/app \
  --db /tmp/gadak-plugin.db \
  --from-json examples/plugins/github-prs/sample-prs.json

sqlite3 /tmp/gadak-plugin.db \
  "SELECT key, kind, payload FROM enrichments WHERE kind='prs' ORDER BY key;"
```

Expected rows (keys from the sample fixture):

```
NMB-110|prs|[{"number":61,...},{"number":42,...}]
NMB-111|prs|[{"number":42,...}]
```

## Live mode

```sh
export GH_TOKEN=ghp_...   # fine-grained or classic, read-only is enough
python3 examples/plugins/github-prs/github_prs.py owner/repo --db ~/.gadak/gadak.db
```

## Flags

| Flag | Meaning |
| --- | --- |
| `REPO` | `owner/name` (required unless `--from-json`) |
| `--db PATH` | Mirror path (default `$GADAK_HOME/gadak.db` or `~/.gadak/gadak.db`) |
| `--profile NAME` | Use `…/profiles/NAME/gadak.db` when `--db` is omitted |
| `--from-json FILE` | Offline PR list (GitHub REST shape or `{ "pulls": [...] }`) |
| `--state all\|open\|closed` | Live API filter (default `all`) |
| `--dry-run` | Print planned rows; no write, no version bump |
| `--self-test` | Built-in checks on a temp DB |

## Idempotency

Re-running the same input upserts with `ON CONFLICT(key, kind) DO UPDATE` and
bumps `sync_state.version` once per successful write. Row count stays stable;
version increases by 1 each run.

## Verify after a write

```sql
SELECT key, json_array_length(payload) AS prs, source, updated_at
FROM enrichments WHERE kind = 'prs' ORDER BY updated_at DESC LIMIT 20;

SELECT version FROM sync_state;
```
