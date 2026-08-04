package store

// migrations are applied in order and the index+1 is the schema version. A
// released migration is never edited; a schema change is a new entry at the end
// plus a documented row in specs/000-product/data-model.md.
var migrations = []string{schemaV1}

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

CREATE VIRTUAL TABLE items_fts USING fts5(
  title, body_text, comments_text,
  content='',
  contentless_delete=1,
  tokenize='unicode61 remove_diacritics 2'
);

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
