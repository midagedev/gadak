# deploy-status

Answer “how far has this issue shipped?” using only a local git repository.

Every CD system is different, so this example stays generic: it finds commits
whose messages mention issue keys (`NMB-42`), checks which tags contain those
commits (`git tag --contains`), maps tag names to environments, and writes
`kind='deploy'` enrichments.

## Why

Deploy badges and the detail timeline are already in the gadak UI, gated by
`features.deploy`. What is missing from core on purpose is any knowledge of
*your* pipeline. This script is a template: swap the tag rules for your
release naming, or replace the git scan with an API to your CD tool — the
payload shape stays the same.

## Tag → channel rules

| Tag pattern (examples) | Channel | Resulting `state` |
| --- | --- | --- |
| `*-dev`, `v1.0.0-dev` | dev | `dev` |
| `*-staging`, `*-qa` | qa | `qa` |
| `*-prod`, `v1.2.3`, `1.0.0` | prod | `prod` |
| commit mentions key, no classified tag | — | `merged` |

The highest channel that contains the fix wins. Customize with `--tags`
(comma-separated `git tag -l` patterns; default
`v*,*-staging,*-prod,*-dev,*-qa`).

## Payload contract

Wrapped form so list rows and the detail panel both get data
(`docs/PLUGINS.md`, server `pick(p, "status")` / `pick(p, "detail")`):

```json
{
  "status": {
    "state": "prod",
    "merged_prs": 1,
    "total_prs": 1,
    "dev": { "tag": "v1.0.0-dev", "at": "2026-03-01T08:00:00Z" },
    "qa_release": { "tag": "1.0.0-staging", "at": "2026-03-02T09:00:00Z" },
    "qa_swapped_at": "2026-03-02T09:00:00Z",
    "prod_at": "2026-03-03T10:00:00Z"
  },
  "detail": {
    "state": "prod",
    "merged_prs": 1,
    "total_prs": 1,
    "dev": { "tag": "v1.0.0-dev", "at": "2026-03-01T08:00:00Z" },
    "qa_release": { "tag": "1.0.0-staging", "at": "2026-03-02T09:00:00Z" },
    "qa_swapped_at": "2026-03-02T09:00:00Z",
    "prod_at": "2026-03-03T10:00:00Z",
    "releases": [
      { "tag": "v1.0.0-dev", "channel": "dev", "at": "…" },
      { "tag": "1.0.0-staging", "channel": "qa", "at": "…" },
      { "tag": "v1.0.0", "channel": "prod", "at": "…" }
    ],
    "prs": []
  }
}
```

`merged_prs` / `total_prs` are set to `1` when any stage is known (this plugin
does not call GitHub). Point a PR plugin at the same keys if you want real
counts.

## Requirements

- Python 3.9+ (stdlib only)
- `git` on `PATH`
- A local clone of the product repo (any remote is fine; only local history is read)

## Quick start

```sh
cp examples/demo.db /tmp/gadak-plugin.db

# Use any local git repo whose commits mention keys that exist in the mirror.
python3 examples/plugins/deploy-status/deploy_status.py /path/to/your/repo \
  --db /tmp/gadak-plugin.db

# Or dry-run first:
python3 examples/plugins/deploy-status/deploy_status.py /path/to/your/repo --dry-run
```

## Self-test (no external repo)

```sh
python3 examples/plugins/deploy-status/deploy_status.py --self-test
# or
bash examples/plugins/deploy-status/test.sh
```

The self-test builds a temporary repo with `NMB-1` / `NMB-2` commits and
`v*`, `*-staging`, `*-dev` tags, then checks upsert + idempotency.

## Flags

| Flag | Meaning |
| --- | --- |
| `REPO` | Path to a local git repository |
| `--db PATH` | Mirror path (default `$GADAK_HOME/gadak.db` or `~/.gadak/gadak.db`) |
| `--profile NAME` | Profile under `$GADAK_HOME/profiles/` when `--db` is omitted |
| `--tags PATTERNS` | Comma-separated tag globs (default above) |
| `--dry-run` | Print planned rows; no write, no version bump |
| `--self-test` | Built-in checks on a temp git repo + temp DB |

## Enable the UI

```json
{ "features": { "deploy": true } }
```

## Verify after a write

```sql
SELECT key,
       json_extract(payload, '$.status.state') AS state,
       json_extract(payload, '$.detail.releases') AS releases
FROM enrichments
WHERE kind = 'deploy'
ORDER BY key
LIMIT 20;

SELECT version FROM sync_state;
```
