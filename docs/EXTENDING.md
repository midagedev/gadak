# Extending gadak for your environment

gadak is a local-first mirror of an issue tracker. The binary stays free of your
company’s CD pipeline, test manager, and org chart. You extend it in three
places, each with a different cost and power trade-off.

If you only need “how do I attach GitHub PRs?”, jump to
[Five-minute quick start](#3-five-minute-quick-start). For the enrichment table
contract itself, see [`PLUGINS.md`](PLUGINS.md).

---

## 1. Three extension axes

| Axis | Where | When to use | Network on keystroke path? |
| --- | --- | --- | --- |
| **Config** | `~/.gadak/config.json` (or Settings UI) | Map custom fields, define team buckets, turn surfaces on/off, allowlist editable fields | No — applied in-process |
| **Enrichments** | External process writes SQLite `enrichments` | Attach data from systems gadak must not import (GitHub, CD, QA tools, spreadsheets) | No — plugin runs on its own schedule |
| **Direct SQL** | `gadak sql`, agents, reports | Ad-hoc questions, weekly digests, automations that *read* the mirror | No — reads the local file |

### Config (`config.json`)

Full reference (defaults, floors, apply timing, hand-edit-only list):
[`CONFIGURATION.md`](CONFIGURATION.md).

| Key | Purpose |
| --- | --- |
| `fields` | FieldSpec[] (`alias`, `ids`, `role`, `kind`). Synced into list rows under the alias. Empty `kind` is display-only; a set `kind` enters the inline-edit allowlist. Auto-filled by `gadak fields --apply` / first full sync. |
| `bodyFields` | Extra ADF custom field ids folded into FTS body text. |
| `groupRules` | Ordered rules (`projects` / `labels` / `components`) → group id for team views. |
| `groupQuery` | Optional `SELECT`/`WITH` returning `(issue key, group)` for classification that does not fit those three lists. Runs when the derived view is rebuilt, not on a keystroke. Empty group = unclassified; NULL / missing key falls through to `groupRules`, then the assignee's member group. Site-specific `CASE` belongs here. |
| `groupLabels` / `groupColors` / `productByGroup` | Display names and colors for those groups. |
| `members` | Static member directory (avatar, group, account id) merged into bootstrap. |
| `features` | Feature flags: `feed`, `push`, `deploy`, `qa`, `teamGroups` (`feed` defaults on when omitted; others off). |
| `qaDashboardUrl` | Optional link shown next to QA surfaces. |
| `staleThresholdHours` | Hours in status before an open issue counts as stale (0 → UI default 72). |
| `syncIntervalSec` | Incremental sync period in seconds (0 → 60). Min 15 when set. Restart `serve`. |
| `reconcileIntervalSec` | Deletion reconcile period in seconds (0 → 3600). Min 300 when set. Restart `serve`. |
| `notify` | OS desktop notifications from the watch loop (default true). |
| `confluence.spaces` | Space allowlist. The list *is* the scope: drop a space and the next Confluence pass removes it; add one and that space is fetched from the start. Empty means every *global* space; personal spaces only if named. |

Nothing installation-specific belongs in the gadak **source tree**. Put it here.

### Enrichments (plugins)

A separate process upserts:

```sql
INSERT INTO enrichments (key, kind, payload, source, updated_at) VALUES (…)
ON CONFLICT(key, kind) DO UPDATE SET …;
UPDATE sync_state SET version = version + 1;  -- required for UI refresh
```

Known kinds the UI merges today: `deploy`, `qa`, `prs`, `opinion`. Details and
payload tables: [`PLUGINS.md`](PLUGINS.md). Runnable examples:
[`examples/plugins/`](../examples/plugins/).

### Direct SQL

The mirror is a normal SQLite file. Agents and humans query it with
`gadak sql` (read-only). Cookbook queries live in
[`docs/MIRROR.md`](MIRROR.md) and [`docs/AGENT_ACCESS.md`](AGENT_ACCESS.md).

**Never write mirrored issue fields by hand** — the next sync overwrites them.
The only supported external write table is `enrichments` (plus personal tables
like watches/favorites via the API).

---

## 2. Recipe table — “we want to see X”

| We want… | Axis | How |
| --- | --- | --- |
| A custom field on the list / filters | **Config** | Pin it in `fields` (Settings → Fields) or run `gadak fields --apply`. |
| “My team’s board” without Jira boards | **Config** | `groupRules` + `features.teamGroups`. |
| Classification that needs regex, exclusions, custom fields | **Config** | `groupQuery` — one SQL statement over `issues_full` / `json_each`. Do not grow `groupRules`. |
| Inline edit of a few fields | **Config** | Set `kind` on the spec in `fields` + stored credential. Empty `kind` is display-only. |
| Deploy badge / “is it in prod?” | **Enrichment** | `kind=deploy` — start from [deploy-status](../examples/plugins/deploy-status/). |
| PR links on the issue | **Core / Enrichment** | Core: `dev_links` / `gadak dev`. Plugin `kind=prs` is extra metadata — [github-prs](../examples/plugins/github-prs/). |
| QA impact column | **Enrichment** | `kind=qa` + `features.qa` (shape in [`PLUGINS.md`](PLUGINS.md)). |
| A free-text review note | **Enrichment** | `kind=opinion` (JSON string payload). |
| Spreadsheet-managed extras | **Enrichment** | [csv-import](../examples/plugins/csv-import/). |
| Weekly reopen / load report | **SQL** | `gadak sql` + cron; see AGENTS.md cookbook. |
| “Has anyone hit this before?” | **SQL** | FTS via `items_fts` / `gadak search`. |
| Agent that comments or transitions | **CLI / REST** | `gadak comment` / `gadak transition` (writes go through the origin, then re-mirror). |

---

## 3. Five-minute quick start

Prerequisites: Python 3.9+, `sqlite3` CLI, a copy of the demo mirror (or your
own `gadak.db`).

```sh
# 0. Disposable DB
cp examples/demo.db /tmp/gadak-plugin.db
sqlite3 /tmp/gadak-plugin.db "SELECT version FROM sync_state;"

# 1. PRs from a fixture (no GitHub token)
python3 examples/plugins/github-prs/github_prs.py example/app \
  --db /tmp/gadak-plugin.db \
  --from-json examples/plugins/github-prs/sample-prs.json

# 2. Mixed rows from a spreadsheet
python3 examples/plugins/csv-import/csv_import.py \
  examples/plugins/csv-import/sample.csv \
  --db /tmp/gadak-plugin.db

# 3. Deploy stages from git tags (use your product repo, or --self-test)
python3 examples/plugins/deploy-status/deploy_status.py --self-test

# 4. Confirm
sqlite3 /tmp/gadak-plugin.db <<'SQL'
SELECT kind, COUNT(*) AS n FROM enrichments GROUP BY kind;
SELECT key, kind, substr(payload,1,100) FROM enrichments ORDER BY key, kind;
SELECT version FROM sync_state;
SQL
```

Turn on surfaces in config when serving:

```json
{ "features": { "deploy": true, "qa": true } }
```

Live GitHub:

```sh
export GH_TOKEN=…   # read-only is enough
python3 examples/plugins/github-prs/github_prs.py owner/repo --db ~/.gadak/gadak.db
```

More detail per plugin: [`examples/plugins/README.md`](../examples/plugins/README.md).

---

## 4. Scheduling and diagnosis

### cron

```cron
# Every 15 minutes — PRs
*/15 * * * * GH_TOKEN=… /usr/bin/python3 /opt/gadak-plugins/github_prs.py myorg/app --db /home/you/.gadak/gadak.db >>/var/log/gadak-prs.log 2>&1

# Hourly — deploy from a maintained mirror clone
0 * * * * /usr/bin/python3 /opt/gadak-plugins/deploy_status.py /srv/product.git --db /home/you/.gadak/gadak.db >>/var/log/gadak-deploy.log 2>&1
```

### launchd (macOS) sketch

`~/Library/LaunchAgents/com.example.gadak-prs.plist` → `StartInterval` 900,
`ProgramArguments` pointing at the same python invocation, `EnvironmentVariables`
for `GH_TOKEN` / `GADAK_HOME`.

### systemd timer sketch

`gadak-prs.service` (`Type=oneshot`) + `gadak-prs.timer` (`OnUnitActiveSec=15m`).

### When nothing shows up in the UI

1. **Did the plugin write?**

   ```sh
   gadak status --json
   gadak sql "SELECT kind, COUNT(*) FROM enrichments GROUP BY kind"
   gadak sql "SELECT key, kind, source, updated_at FROM enrichments ORDER BY updated_at DESC LIMIT 10"
   ```

2. **Was `sync_state.version` bumped?** If the row is on disk but the client
   still shows old data, the plugin forgot the version bump. Re-run a correct
   writer, or once:

   ```sql
   UPDATE sync_state SET version = version + 1;
   ```

3. **Is the feature flag on?** `features.deploy` / `features.qa` default to off.
   Check Settings or `config.json`.

4. **Is the JSON valid?** Invalid payloads are **dropped at read time** with no
   error (so a bad plugin cannot corrupt the API). Validate before insert;
   `csv-import` rejects bad JSON with a row number.

5. **Is the key present in the mirror?** Enrichments for unknown keys are
   stored but never joined into list/detail until that issue is mirrored.

6. **Stale watermark?** `gadak status --json` → `last_error` means the last
   issue-source sync failed (Jira, or Linear when that is the only issue
   source that has run; per-source rows are `sources.jira` / `sources.linear`);
   `watermark` only moves when the tracker changes.

---

## 5. Limits (read before inventing a new kind)

1. **Enrichments cannot overwrite mirrored fields.** The server serializes plugin
   keys *before* issue fields; `status`, `summary`, and friends always win.
2. **Payload is opaque JSON per kind.** The server does not validate beyond
   `json.Valid`. Shape is a contract with the UI (`web/src/lib/types.ts`).
3. **New kinds need a core PR.** Adding `kind=security` (or any surface not in
   the merge switch) requires server + UI work. Until then the row sits unused
   (or use `csv-import --allow-unknown-kind` only as a private holding area).
4. **Enrichments are disposable.** Deleting the DB and re-syncing drops them.
   The source system remains the record; re-run the plugin.
5. **Zero external integration code in this repository.** Do not propose a
   GitHub/Slack/Jenkins client inside `internal/`. Ship a plugin process
   instead — like the examples under `examples/plugins/`. Verified 2026-08-06:
   `grep -R --include='*.go' -E 'go-github|slack-go|andygrunwald|xanzy/go-gitlab'
   internal/ cmd/` → empty.
6. **Credentials stay out of SQLite.** Tokens belong in the environment or a
   secret store the plugin reads; never in `enrichments.payload`.

---

## Related docs

- [`PLUGINS.md`](PLUGINS.md) — table schema, kind payload tables, example SQL
- [`AGENT_ACCESS.md`](AGENT_ACCESS.md) — querying the mirror from agents
- [`../specs/000-product/data-model.md`](../specs/000-product/data-model.md) — full schema contract
- [`../examples/plugins/`](../examples/plugins/) — runnable reference implementations
