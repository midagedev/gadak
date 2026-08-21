package store

// migrations are applied in order and the index+1 is the schema version. A
// released migration is never edited; a schema change is a new entry at the end
// plus a documented row in specs/000-product/data-model.md.
var migrations = []string{schemaV1, schemaV2, schemaV3, schemaV4, schemaV5, schemaV6, schemaV7, schemaV8, schemaV9, schemaV10, schemaV11, schemaV12, schemaV13, schemaV14, schemaV15, schemaV16, schemaV17, schemaV18, schemaV19, schemaV20, schemaV21, schemaV22, schemaV23, schemaV24, schemaV25, schemaV26, schemaV27, schemaV28, schemaV29}

// itemsFTSCreate is the canonical items_fts DDL, spliced into schemaV1 so a
// fresh database is born matching it (GDK-444: an inline copy in V1 lagged at
// three columns and every new mirror's first Open logged a rebuild). Editing
// V1 is safe precisely because migrations never re-run: an existing database
// keeps whatever V1 gave it and converges through repairItemsFTS, and a
// pre-v25 binary cannot open a v29 file at all (user_version gate). It is the
// single owner of the shape every writable mirror must have:
// contentless_delete=1 is what lets writeFTS replace rows with DELETE, and
// Open (fts_repair.go) checks live DDL against this statement and rebuilds
// the index when a database carries a different shape (GDK-112: the portable
// examples/demo.db snapshot deliberately drops the option for Datasette
// Lite). cjk_bigram (GDK-259) is filled by FTSCJKBigramColumn; leaving it out
// of any writer is the silent-miss trap.
const itemsFTSCreate = `CREATE VIRTUAL TABLE items_fts USING fts5(
  title, body_text, comments_text, cjk_bigram,
  content='',
  contentless_delete=1,
  tokenize='unicode61 remove_diacritics 2'
);`

const schemaV1 = `
CREATE TABLE sources (
  id        TEXT PRIMARY KEY,
  kind      TEXT NOT NULL,
  base_url  TEXT,
  synced_at TEXT
);

CREATE TABLE items (
  id          TEXT PRIMARY KEY,
  source_id   TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  kind        TEXT NOT NULL,
  external_id TEXT,
  key         TEXT,
  title       TEXT,
  body_text   TEXT,
  author      TEXT,
  author_id   TEXT,
  url         TEXT,
  created_at  TEXT,
  updated_at  TEXT,
  synced_at   TEXT
);
CREATE UNIQUE INDEX items_source_key ON items(source_id, key);
CREATE INDEX items_kind_updated ON items(kind, updated_at);
CREATE INDEX items_updated ON items(updated_at);
CREATE INDEX items_synced ON items(synced_at);

CREATE TABLE issues (
  item_id             TEXT PRIMARY KEY REFERENCES items(id) ON DELETE CASCADE,
  key                 TEXT NOT NULL,
  project_key         TEXT,
  issue_type          TEXT,
  issue_type_id       TEXT,
  status              TEXT,
  status_id           TEXT,
  status_category     TEXT,
  priority            TEXT,
  priority_rank       INTEGER NOT NULL DEFAULT 0,
  assignee            TEXT,
  assignee_id         TEXT,
  assignee_email      TEXT,
  reporter            TEXT,
  reporter_id         TEXT,
  reporter_email      TEXT,
  parent_key          TEXT,
  labels              TEXT,
  components          TEXT,
  fix_versions        TEXT,
  affects_versions    TEXT,
  environment_text    TEXT,
  duedate             TEXT,
  resolution          TEXT,
  created_at          TEXT,
  updated_at          TEXT,
  status_changed_at   TEXT,
  resolved_at         TEXT,
  reopen_count        INTEGER NOT NULL DEFAULT 0,
  reopened_at         TEXT,
  assignee_changed_at TEXT,
  comment_count       INTEGER NOT NULL DEFAULT 0,
  description_adf     TEXT,
  custom              TEXT,
  raw                 TEXT
);
CREATE INDEX issues_project_category ON issues(project_key, status_category);
CREATE INDEX issues_assignee ON issues(assignee_id);
CREATE INDEX issues_updated ON issues(updated_at);
CREATE INDEX issues_category_updated ON issues(status_category, updated_at);
CREATE INDEX issues_reopen ON issues(reopen_count);
CREATE INDEX issues_key ON issues(key);

CREATE TABLE comments (
  id          TEXT PRIMARY KEY,
  item_id     TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  external_id TEXT,
  author      TEXT,
  author_id   TEXT,
  body_adf    TEXT,
  body_text   TEXT,
  created_at  TEXT,
  updated_at  TEXT
);
CREATE INDEX comments_item_created ON comments(item_id, created_at);

CREATE TABLE attachments (
  id          TEXT PRIMARY KEY,
  item_id     TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  external_id TEXT,
  filename    TEXT,
  mime_type   TEXT,
  size        INTEGER,
  author      TEXT,
  created_at  TEXT
);
CREATE INDEX attachments_item ON attachments(item_id);

CREATE TABLE changelog (
  id         TEXT PRIMARY KEY,
  item_id    TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  at         TEXT,
  author     TEXT,
  field      TEXT,
  from_value TEXT,
  from_id    TEXT,
  to_value   TEXT,
  to_id      TEXT
);
CREATE INDEX changelog_item_at ON changelog(item_id, at);
CREATE INDEX changelog_field_at ON changelog(field, at);

CREATE TABLE links (
  item_id    TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  type       TEXT NOT NULL,
  direction  TEXT NOT NULL,
  target_key TEXT NOT NULL,
  PRIMARY KEY (item_id, type, direction, target_key)
);

` + itemsFTSCreate + `

CREATE TABLE deleted_items (
  key        TEXT PRIMARY KEY,
  source_id  TEXT NOT NULL,
  deleted_at TEXT NOT NULL
);
CREATE INDEX deleted_items_at ON deleted_items(deleted_at);

CREATE TABLE saved_views (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  config     TEXT NOT NULL,
  created_at TEXT,
  updated_at TEXT
);

CREATE TABLE watches (
  key        TEXT PRIMARY KEY,
  created_at TEXT
);

CREATE TABLE favorites (
  key        TEXT PRIMARY KEY,
  created_at TEXT
);

CREATE TABLE sync_state (
  source_id         TEXT PRIMARY KEY,
  watermark         TEXT,
  version           INTEGER NOT NULL DEFAULT 0,
  last_full_sync_at TEXT,
  last_error        TEXT,
  schema_version    INTEGER NOT NULL DEFAULT 0
);
`

// schemaV2 adds the plugin boundary. Writes come from outside this process, so
// there is no Go write path here on purpose — see data-model.md, `enrichments`.
const schemaV2 = `
CREATE TABLE enrichments (
  key        TEXT NOT NULL,
  kind       TEXT NOT NULL,
  payload    TEXT NOT NULL,
  source     TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL,
  PRIMARY KEY (key, kind)
);
CREATE INDEX enrichments_kind ON enrichments(kind);
`

// schemaV3 adds two derived columns (rules in data-model.md): reopen_reason is
// the first comment written at or after the last reopen (a heuristic, not a
// Jira field), cloned_from is the key of the issue this one was cloned from.
// Existing rows backfill on the next full sync — Derive computes both.
const schemaV3 = `
ALTER TABLE issues ADD COLUMN reopen_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE issues ADD COLUMN cloned_from TEXT NOT NULL DEFAULT '';
`

// schemaV4 records which personal-feed events the local user has already seen.
// Events themselves are never stored: the feed is computed from the mirror at
// query time (changelog / comments / attachments / issue creates).
const schemaV4 = `
CREATE TABLE feed_reads (
  event_id TEXT PRIMARY KEY,
  read_at  TEXT NOT NULL
);
`

// schemaV5 adds retention instrumentation (first_sync_at / sync_count), an OS
// notification watermark separate from feed_reads, and the agent convenience
// view issues_full (title as summary without joining items).
const schemaV5 = `
ALTER TABLE sync_state ADD COLUMN first_sync_at TEXT;
ALTER TABLE sync_state ADD COLUMN sync_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sync_state ADD COLUMN last_notified_at TEXT;
CREATE VIEW issues_full AS
  SELECT it.title AS summary, i.*
  FROM issues i JOIN items it ON it.id = i.item_id;
`

// schemaV6 records per-day outbound Jira HTTP volume for rate-limit visibility.
// This is personal operational data (our process's call counts), not mirrored
// Jira content. Counters are flushed from the client after each sync cycle.
const schemaV6 = `
CREATE TABLE api_usage (
  day               TEXT PRIMARY KEY,
  requests          INTEGER NOT NULL DEFAULT 0,
  throttled         INTEGER NOT NULL DEFAULT 0,
  server_errors     INTEGER NOT NULL DEFAULT 0,
  retries           INTEGER NOT NULL DEFAULT 0,
  wait_ms           INTEGER NOT NULL DEFAULT 0,
  last_throttled_at TEXT
);
`

// schemaV7 records per-project fill counts for discovered custom field aliases
// (issues.custom after specs applied). Recomputed after discovery or full sync.
const schemaV7 = `
CREATE TABLE field_usage (
  project_key TEXT NOT NULL,
  alias       TEXT NOT NULL,
  filled      INTEGER NOT NULL,
  total       INTEGER NOT NULL,
  PRIMARY KEY (project_key, alias)
);
`

// schemaV8 keeps a short history of meaningful sync runs (changed something,
// was a full pass, or failed) so the UI can answer "what did sync actually do".
const schemaV8 = `
CREATE TABLE sync_runs (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  source_id   TEXT NOT NULL,
  kind        TEXT NOT NULL,           -- full | incremental (+reconcile suffix)
  started_at  TEXT NOT NULL,
  finished_at TEXT NOT NULL,
  fetched     INTEGER NOT NULL,
  changed     INTEGER NOT NULL,
  deleted     INTEGER NOT NULL,
  error       TEXT                     -- NULL on success
);
CREATE INDEX idx_sync_runs_source ON sync_runs(source_id, id DESC);
`

// schemaV9 adds the document projection for wiki pages (second source). Comments
// stay on the existing comments table; the spine is still items.
const schemaV9 = `
CREATE TABLE pages (
  item_id    TEXT PRIMARY KEY REFERENCES items(id) ON DELETE CASCADE,
  space_key  TEXT NOT NULL,
  parent_id  TEXT NOT NULL DEFAULT '',
  version    INTEGER NOT NULL DEFAULT 1,
  status     TEXT NOT NULL DEFAULT 'current'
);
CREATE INDEX idx_pages_space ON pages(space_key);
`

// schemaV10 stores the original ADF body on the pages projection so detail
// rendering does not re-fetch Confluence (R2 read path).
const schemaV10 = `
ALTER TABLE pages ADD COLUMN body_adf TEXT NOT NULL DEFAULT '';
`

// schemaV11 stores issue hierarchy level and a derived epic_key. hierarchy_level
// is the source's tree rank (Jira: Epic=1, standard=0, sub-task=-1). epic_key is
// the key of the nearest hierarchy_level==1 ancestor via parent_key (NULL when
// none). Backfill walks raw JSON for level, then two-hop parent joins for epic.
// issues_full (v5) expands i.* at CREATE VIEW time and is not recreated here —
// a follow-up migration must rebuild the view if agents need the new columns
// through issues_full.
const schemaV11 = `
ALTER TABLE issues ADD COLUMN hierarchy_level INTEGER NOT NULL DEFAULT 0;
ALTER TABLE issues ADD COLUMN epic_key TEXT;
UPDATE issues SET hierarchy_level = COALESCE(json_extract(raw, '$.fields.issuetype.hierarchyLevel'), 0);
UPDATE issues SET epic_key = (
  SELECT CASE
    WHEN p.hierarchy_level = 1 THEN p.key
    WHEN gp.hierarchy_level = 1 THEN gp.key
  END
  FROM issues p
  LEFT JOIN issues gp ON gp.key = p.parent_key
  WHERE p.key = issues.parent_key
);
`

// schemaV12 rebuilds issues_full: the view expanded i.* at CREATE VIEW time
// (v5), so databases migrated past v11 were missing hierarchy_level and
// epic_key through the agent-facing view. Recreating re-expands the star.
const schemaV12 = `
DROP VIEW issues_full;
CREATE VIEW issues_full AS
  SELECT it.title AS summary, i.*
  FROM issues i JOIN items it ON it.id = i.item_id;
`

// schemaV13 adds labels on the pages projection (JSON string array, same shape
// as issues.labels). Empty is '[]', never NULL — filters use json_each.
const schemaV13 = `
ALTER TABLE pages ADD COLUMN labels TEXT NOT NULL DEFAULT '[]';
`

// schemaV14 mirrors wiki space metadata (key → human name). Cloud space keys
// are often opaque auto-generated strings; the UI displays name via a join.
// kind is the source type string ('global' | 'personal').
const schemaV14 = `
CREATE TABLE spaces (
  source_id TEXT NOT NULL,
  key       TEXT NOT NULL,
  name      TEXT NOT NULL DEFAULT '',
  kind      TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (source_id, key)
);
`

// schemaV15 adds a one-line body preview on the pages projection for document
// lists. FTS is contentless so plain text cannot be recovered from the index;
// excerpt is derived from body_adf (max 200 runes, whitespace-normalized) on
// upsert and backfilled for existing rows in the same migration transaction
// (see backfillPageExcerpts).
const schemaV15 = `
ALTER TABLE pages ADD COLUMN excerpt TEXT NOT NULL DEFAULT '';
`

// schemaV16 stores text-derived cross-references between items: page bodies
// that mention issue keys, and issue bodies/comments that mention wiki page
// URLs. Targets need not exist in the mirror (no FK on target_key). Readers
// join items and only surface live rows. Existing rows are backfilled in the
// same migration transaction (see backfillItemRefs).
const schemaV16 = `
CREATE TABLE item_refs (
  item_id     TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  target_kind TEXT NOT NULL,
  target_key  TEXT NOT NULL,
  via         TEXT NOT NULL,
  PRIMARY KEY (item_id, target_kind, target_key)
);
CREATE INDEX idx_item_refs_target ON item_refs(target_kind, target_key);
`

// schemaV17 stores each space's homepage content id (Confluence expand=homepage).
// The UI drops the trail segment whose key equals homepage_id so the space
// label is not repeated. Empty until a listing or per-space GET fills it.
const schemaV17 = `
ALTER TABLE spaces ADD COLUMN homepage_id TEXT NOT NULL DEFAULT '';
`

// schemaV18 mirrors named queries from a connector (Jira saved filters).
// Disposable: Jira is the record; a re-sync replaces the source's rows.
// Distinct from saved_views, which are authored in gadak.
const schemaV18 = `
CREATE TABLE source_queries (
  id           TEXT PRIMARY KEY,
  source_id    TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  external_id  TEXT NOT NULL,
  name         TEXT NOT NULL,
  query_text   TEXT NOT NULL,
  config       TEXT NOT NULL,
  favourite    INTEGER NOT NULL DEFAULT 0,
  owner        TEXT,
  applied      TEXT,
  unsupported  TEXT,
  updated_at   TEXT
);
CREATE UNIQUE INDEX source_queries_ext ON source_queries(source_id, external_id);
CREATE INDEX source_queries_source ON source_queries(source_id, favourite, name);
`

// schemaV19 is the per-space incremental floor for wiki sync. NULL/empty means
// the space has not been backfilled yet and the next pass must fetch it in
// full. A space added after a broad sync must not inherit sync_state.watermark.
const schemaV19 = `
ALTER TABLE spaces ADD COLUMN watermark TEXT;
`

// schemaV20 adds account ids on changelog and attachment authors. Display
// names collide across people and change on rename; feed self-exclusion and
// later readers key on author_id when present, and fall back to author for
// rows migrated from v19 (NULL).
const schemaV20 = `
ALTER TABLE changelog ADD COLUMN author_id TEXT;
ALTER TABLE attachments ADD COLUMN author_id TEXT;
`

// schemaV21 stores Confluence page version-history stamps (never bodies).
// (item_id, number) is the PK so a re-collect upserts. message is a plain
// column for gadak sql; it is not added to items_fts (contentless, per-item).
const schemaV21 = `
CREATE TABLE page_versions (
  item_id     TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  number      INTEGER NOT NULL,
  created_at  TEXT,
  author_id   TEXT NOT NULL DEFAULT '',
  author_name TEXT NOT NULL DEFAULT '',
  message     TEXT NOT NULL DEFAULT '',
  minor_edit  INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (item_id, number)
);
CREATE INDEX idx_page_versions_at ON page_versions(item_id, created_at);
`

// schemaV22 adds issues.priority_id so list filters can key on the stable
// Jira priority id (names localize per account). Existing rows keep the
// empty string until the next sync rewrites them; the web filter still falls back to
// the display name for those rows and for legacy saved views. issues_full
// is rebuilt because it expanded i.* at CREATE VIEW time (v12) and would
// otherwise hide the new column from agents.
const schemaV22 = `
ALTER TABLE issues ADD COLUMN priority_id TEXT NOT NULL DEFAULT '';
DROP VIEW issues_full;
CREATE VIEW issues_full AS
  SELECT it.title AS summary, i.*
  FROM issues i JOIN items it ON it.id = i.item_id;
`

// schemaV23 rebuilds issues_full to expose description_text (items.body_text).
// The view expanded i.* at CREATE VIEW time (v22) and cannot grow an extra
// expression without a recreate. NULL body_text becomes the empty string.
const schemaV23 = `
DROP VIEW issues_full;
CREATE VIEW issues_full AS
  SELECT it.title AS summary, i.*, COALESCE(it.body_text, '') AS description_text
  FROM issues i JOIN items it ON it.id = i.item_id;
`

// schemaV24 stores the origin content URL on attachment rows. Jira leaves it
// empty (the proxy reconstructs site+/rest/api/3/attachment/content/+id).
// Linear stores the URL the GraphQL node returned, verbatim.
const schemaV24 = `
ALTER TABLE attachments ADD COLUMN url TEXT;
`

// schemaV25 grows items_fts by the cjk_bigram column (CJK mid-compound
// search, GDK-259 / docs/decisions/0009). The DDL change itself is owned by
// itemsFTSCreate: repairItemsFTS rebuilds the index at Open when the stored
// CREATE differs, and that rebuild — not this statement — is the migration
// for databases created before the column existed. (Fresh databases get the
// four-column table straight from schemaV1, which splices itemsFTSCreate —
// GDK-444.) This entry exists so PRAGMA user_version (and with it
// sync_state.schema_version) moves to the documented level; the body is a
// no-op on purpose.
const schemaV25 = `SELECT 1`

// schemaV26 moves personal state out of the mirror (GDK-105): saved_views,
// watches, favorites and feed_reads now live in local.db (localSchemaV4), and
// every read and write goes there (write.go, feed.go). This migration copies
// whatever the mirror still holds across the file boundary. It runs on an
// Open() connection, where the driver hook has already ATTACHed local.db as
// `local` — and EnsureLocal has run before migrate() (see Open), so the
// localSchemaV4 tables exist by the time these INSERTs execute. When they do
// not (unattachable local.db), migrate() holds the mirror at
// personalStateCopyVersion - 1 rather than fail: see store.migrate.
//
// The mirror-side tables are deliberately NOT dropped here. The mirror is WAL
// and local.db is not, so a cross-file transaction has no inter-file atomicity
// to lean on: a crash between the two file commits can land user_version
// without the copy. INSERT OR IGNORE makes re-running the copy safe (and the
// upgrade test re-executes this statement to prove it — rewinding
// user_version stopped being a way to replay it once a later migration added
// a column, since ALTER TABLE ADD COLUMN is not re-runnable), and the source rows staying
// behind means that crash window loses nothing. From this version on the
// mirror-side tables are frozen leftovers that no code path reads or writes;
// dropping them belongs to a later release, once every install has migrated.
const schemaV26 = `
INSERT OR IGNORE INTO local.saved_views (id, name, config, created_at, updated_at)
  SELECT id, name, config, created_at, updated_at FROM saved_views;
INSERT OR IGNORE INTO local.watches (key, created_at) SELECT key, created_at FROM watches;
INSERT OR IGNORE INTO local.favorites (key, created_at) SELECT key, created_at FROM favorites;
INSERT OR IGNORE INTO local.feed_reads (event_id, read_at) SELECT event_id, read_at FROM feed_reads;
`

// schemaV27 adds issues.resolution_id so queries can key on the stable Jira
// resolution id (names localize per account — 완료 vs Done). Existing rows
// keep the empty string until the next sync rewrites them; the migration
// does not backfill from changelog, because the mirror is a cache and the
// origin is the record (same contract as v22 priority_id). issues_full is
// rebuilt because it expanded i.* at CREATE VIEW time (v12, v22, v23) and
// would otherwise hide the new column from agents. The SELECT keeps v23's
// description_text expression.
const schemaV27 = `
ALTER TABLE issues ADD COLUMN resolution_id TEXT NOT NULL DEFAULT '';
DROP VIEW issues_full;
CREATE VIEW issues_full AS
  SELECT it.title AS summary, i.*, COALESCE(it.body_text, '') AS description_text
  FROM issues i JOIN items it ON it.id = i.item_id;
`

// schemaV28 adds comments.visibility_type, visibility_value, and jsd_public so
// restricted Jira comments (visibility.role/group) and JSM internal comments
// (jsdPublic) are distinguishable in the mirror. Existing rows keep ” / ” /
// NULL until the next sync rewrites them; the migration does not backfill,
// because the mirror is a cache and the origin is the record (same contract
// as v22 priority_id and v27 resolution_id). comments is not selected by any
// view (issues_full is items+issues only), so no view rebuild.
const schemaV28 = `
ALTER TABLE comments ADD COLUMN visibility_type  TEXT NOT NULL DEFAULT '';
ALTER TABLE comments ADD COLUMN visibility_value TEXT NOT NULL DEFAULT '';
ALTER TABLE comments ADD COLUMN jsd_public INTEGER;
`

// schemaV29 adds dev_links (GDK-497): the development-panel pull-request
// links the origin holds for an issue — Jira's dev-status on a connected
// workspace, issuetap's on a standalone one. Rows live and die with the
// issue rewrite (same DELETE+INSERT lifetime as comments/attachments), so a
// disabled devStatus flag drains them on the next sync. url is the
// idempotent key per issue, matching both origins' upsert rule.
const schemaV29 = `
CREATE TABLE dev_links (
  item_id     TEXT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  kind        TEXT NOT NULL DEFAULT 'pullrequest',
  external_id TEXT NOT NULL DEFAULT '',
  url         TEXT NOT NULL,
  title       TEXT NOT NULL DEFAULT '',
  status      TEXT NOT NULL DEFAULT '',
  updated_at  TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (item_id, url)
);
CREATE INDEX dev_links_item ON dev_links(item_id);
`

// personalStateCopyVersion is the migration level schemaV26 lands on. migrate
// holds a mirror whose local.db is unreachable one version below it so the
// copy re-runs later instead of refusing the open.
const personalStateCopyVersion = 26
