# Confluence space scope: residue and backfill

Symptom, seen on a real work mirror 2026-08-14: the DOCS view shows dozens of
spaces the user never selected, and the one space they did select shows only a
handful of documents.

**The sync layer now closes this class on its own** (v0.13, schema v19):
every successful Confluence pass prunes pages whose space left the config
scope, and each space carries its own watermark — a newly-selected (or
snapshot-restored) space has none, so its next pass is a floor-less full
backfill. Diagnose with `select key, watermark from spaces`. This runbook
remains for older builds and for verifying that the automation did its job.

## Why it happened (pre-v0.13)

Three separate gaps stacked up. v0.13 (schema v19) closed them in the sync
layer — this section is the older-build diagnosis; the sqlite3 steps below
remain the fallback when you are on a pre-v0.13 binary or want to verify
the automation.

1. **Narrowing scope did not prune.** Confluence sync had no delete-reconcile
   (`internal/sync/confluence.go`, before v0.13). Once a space's
   pages were mirrored, narrowing `config.confluence.spaces` stopped *fetching*
   them but never *deleted* them. The issue axis pruned out-of-scope keys
   (`store.DeleteItems`); Confluence did not.
2. **Widening scope did not backfill.** Incremental sync only pulled pages with
   `lastModified >= watermark`. The watermark was global and already recent, so a
   newly-selected space yielded only its recently-edited pages — its older
   documents never came down.
3. **The watermark was shared across spaces**, so a space added today inherited a
   watermark from yesterday's broad sync.

Net effect on those builds: old broad-sync pages lingered (gap 1) and the
newly-scoped space was half-empty (gap 2).

## Diagnose

```bash
# What the user selected:
python3 -c 'import json,os; c=json.load(open(os.path.expanduser("~/.gadak/config.json"))); print(c.get("confluence"))'

# What the mirror actually holds, per space:
gadak sql --csv "select space_key, count(*) pages from pages group by space_key order by pages desc"

# Confluence watermark / last full sync:
gadak sql --csv "select source_id, watermark, last_full_sync_at, last_error from sync_state where source_id='confluence'"
```

If the per-space list has spaces not in `config.confluence.spaces`, and the
selected space has far fewer pages than it should, this is the case.

## The other shape: a scope the origin does not have

A workspace whose origin was replaced — `gadak migrate` seeding a new
built-in tracker, or a pairing built before v0.20.0 — can keep a
`confluence.spaces` list from the origin it no longer talks to. The classic
residue is the local-origin default `["LOC"]`. Every pass then takes the
explicit path, the lookup 404s, and the mirror stays at zero pages while the
sync reports success — 81 runs in a row on the host this was measured on.

The pass now refuses to be quiet about it: when *every* configured
key fails its lookup it fails with the count and the keys, `gadak status`
prints `wiki on · configured spaces: LOC`, and `gadak doctor` carries a
`confluence_spaces:` row. The repair is one command — an empty list means
every space the origin has:

```bash
gadak config set confluence.spaces "[]"
gadak sync --full --source confluence
```

Narrow it again afterwards with the keys the origin really lists
(`gadak sql --csv "select key, name from spaces"`).

## Fix

Let `KEEP` be the space key the user wants (from the config). Everything else is
residue to remove; then a full Confluence sync backfills `KEEP`.

```bash
# 1. Quit the app so the watch loop does not fight the edit, and so the client
#    re-bootstraps a clean list when it reopens.
osascript -e 'quit app "Gadak"'        # macOS desktop; or stop `gadak serve`

# 2. Back up (the mirror is disposable, but this is cheap insurance).
cp ~/.gadak/gadak.db ~/.gadak/gadak.db.prebak

# 3. Prune every space except KEEP. FTS rows go first (rowid, while the items
#    still exist), then the items (foreign_keys=ON cascades to pages, comments,
#    attachments, item_refs), then the orphan space rows. items_fts is
#    contentless with contentless_delete=1, so DELETE ... WHERE rowid works.
sqlite3 ~/.gadak/gadak.db <<'SQL'
PRAGMA foreign_keys=ON;
BEGIN;
DELETE FROM items_fts WHERE rowid IN (
  SELECT i.rowid FROM items i JOIN pages p ON p.item_id=i.id
  WHERE p.space_key <> 'KEEP'
);
DELETE FROM items WHERE id IN (
  SELECT item_id FROM pages WHERE space_key <> 'KEEP'
);
DELETE FROM spaces WHERE key <> 'KEEP';
-- Reset the Confluence watermark so the next sync is a full backfill.
UPDATE sync_state SET watermark=NULL, last_full_sync_at=NULL WHERE source_id='confluence';
COMMIT;
SQL

# 4. Backfill KEEP in full (Jira is left untouched by --source confluence).
gadak sync --full --source confluence

# 5. Verify: only KEEP remains, with a full page count and no orphans.
gadak sql --csv "select space_key, count(*) from pages group by space_key"
gadak sql "select count(*) from items where source_id=(select id from sources where kind='confluence') and id not in (select item_id from pages)"   # want 0

# 6. Reopen the app.
```

Verified run: a mirror with 13,868 residue pages across 40+ spaces was pruned to
one space, and `gadak sync --full --source confluence` backfilled the kept space
from 3 pages to 438 in ~6m40s, 0 orphans.

## Notes

- `foreign_keys=ON` is required in the `sqlite3` CLI (it defaults off); without
  it the cascade does not fire and you orphan pages/comments.
- Delete the FTS rows **before** the items, while you can still read the rowids.
  Skipping this leaves orphan search-index rows.
- `gadak sql` is read-only, which is why the prune uses `sqlite3` directly. Do it
  with the app stopped so no writer holds the database.
- The backup at `~/.gadak/gadak.db.prebak` can be restored by copying it back
  over `~/.gadak/gadak.db` while the app is stopped.
