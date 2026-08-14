# Confluence space scope: residue and backfill

Symptom, seen on a real work mirror 2026-08-14: the DOCS view shows dozens of
spaces the user never selected, and the one space they did select shows only a
handful of documents.

This runbook is the manual fix. The underlying bugs are tracked in
`specs/001-dedicated-browser/audit-findings.md` (Confluence scope-prune and
scope-widen backfill) and are being closed in the sync layer; until that ships,
this is how you repair a mirror by hand.

## Why it happens

Three separate gaps stack up:

1. **Narrowing scope does not prune.** Confluence sync has no delete-reconcile
   (`internal/sync/confluence.go`, "no reconcile pass yet"). Once a space's
   pages are mirrored, narrowing `config.confluence.spaces` stops *fetching*
   them but never *deletes* them. The issue axis prunes out-of-scope keys
   (`store.DeleteItems`); Confluence does not.
2. **Widening scope does not backfill.** Incremental sync only pulls pages with
   `lastModified >= watermark`. The watermark is global and already recent, so a
   newly-selected space yields only its recently-edited pages — its older
   documents never come down.
3. **The watermark is shared across spaces**, so a space added today inherits a
   watermark from yesterday's broad sync.

Net effect: old broad-sync pages linger (gap 1) and the newly-scoped space is
half-empty (gap 2).

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
