# Example plugins

gadak never embeds third-party integrations. External processes write the
`enrichments` table; the server merges known kinds into list and detail
responses. These directories are **copy-pasteable starting points** — each one
runs with Python 3 stdlib only (plus `git` for deploy-status).

| Plugin | Kind | What it does |
| --- | --- | --- |
| [github-prs/](github-prs/) | `prs` | Match issue keys in GitHub PR title/body/branch |
| [deploy-status/](deploy-status/) | `deploy` | Infer env reach from git tags that contain the fix |
| [csv-import/](csv-import/) | any | Upsert rows from a spreadsheet export |

Contract and UI flags: [`docs/PLUGINS.md`](../../docs/PLUGINS.md).
How to choose an extension axis: [`docs/EXTENDING.md`](../../docs/EXTENDING.md).

## Five-minute smoke test (offline)

```sh
# from the gadak repository root
cp examples/demo.db /tmp/gadak-plugin.db
V0=$(sqlite3 /tmp/gadak-plugin.db "SELECT version FROM sync_state LIMIT 1;")

python3 examples/plugins/github-prs/github_prs.py example/app \
  --db /tmp/gadak-plugin.db \
  --from-json examples/plugins/github-prs/sample-prs.json

python3 examples/plugins/csv-import/csv_import.py \
  examples/plugins/csv-import/sample.csv \
  --db /tmp/gadak-plugin.db

# deploy-status needs a git repo; use --self-test for a synthetic one, or:
# python3 examples/plugins/deploy-status/deploy_status.py /path/to/repo --db /tmp/gadak-plugin.db

sqlite3 /tmp/gadak-plugin.db "SELECT kind, COUNT(*) FROM enrichments GROUP BY kind;"
sqlite3 /tmp/gadak-plugin.db "SELECT version FROM sync_state;"
# version should be V0+2 after the two writers above
```

## Run every self-test

```sh
make plugins-test
```
