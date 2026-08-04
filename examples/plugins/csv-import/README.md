# csv-import

The generic escape hatch: load enrichments from a spreadsheet.

Teams that already track “which issues are on staging”, “QA blocking list”, or
“manual PR links” in Google Sheets / Excel can export CSV and run this script
on a schedule — no API client required.

## CSV format

Header row required:

```csv
issue_key,kind,payload_json
NMB-110,prs,"[{""number"":42,""title"":""Fix"",""url"":""https://github.com/example/app/pull/42"",""state"":""merged"",""repo"":""example/app"",""author"":""alice""}]"
NMB-110,opinion,"""Needs a staging re-check after the deploy."""
```

| Column | Required | Notes |
| --- | --- | --- |
| `issue_key` | yes | Tracker key, e.g. `NMB-110` |
| `kind` | yes | `deploy`, `qa`, `prs`, or `opinion` (others need `--allow-unknown-kind`) |
| `payload_json` | yes | Valid JSON for that kind — see `docs/PLUGINS.md` |
| `source` | no | Defaults to `csv-import` |

UTF-8 with optional BOM is accepted. Invalid JSON fails the whole run and
names the **row number** so a sheet is easy to fix.

## Quick start

```sh
cp examples/demo.db /tmp/scry-plugin.db

python3 examples/plugins/csv-import/csv_import.py \
  examples/plugins/csv-import/sample.csv \
  --db /tmp/scry-plugin.db

sqlite3 /tmp/scry-plugin.db <<'SQL'
SELECT key, kind, substr(payload, 1, 80) FROM enrichments
WHERE source = 'spreadsheet' OR source = 'csv-import'
ORDER BY key, kind;
SELECT version FROM sync_state;
SQL
```

## Flags

| Flag | Meaning |
| --- | --- |
| `CSV` | Path to the file |
| `--db PATH` | Mirror path (default `$SCRY_HOME/scry.db` or `~/.scry/scry.db`) |
| `--profile NAME` | Profile when `--db` is omitted |
| `--allow-unknown-kind` | Accept kinds the UI does not render yet |
| `--dry-run` | Print planned rows; no write, no version bump |
| `--self-test` | Built-in checks (valid + invalid JSON + idempotency) |

## Idempotency

Same `(key, kind)` rows overwrite payload/source/updated_at. A second import of
the same file keeps the row count stable and bumps `sync_state.version` by 1.

## Verify after a write

```sql
SELECT kind, COUNT(*) FROM enrichments GROUP BY kind;
SELECT key, kind, source, updated_at FROM enrichments ORDER BY updated_at DESC LIMIT 20;
SELECT version FROM sync_state;
```
