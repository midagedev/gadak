#!/usr/bin/env python3
"""Gate the *applied* recording fixture: no English prose left on camera.

    tools/demo-i18n/check-applied.py <mirror.db> <locale> [--limit N]
    tools/demo-i18n/check-applied.py <mirror.db> --plan

check.py gates the translation FILE — every id in en.json has a rendering.
This gates the DATABASE a ko/ja take actually records over, and the two are
not the same claim. A file can be complete and gate-green while a column
nobody put in the extraction contract still holds English, because an id
that was never extracted cannot be reported missing.

That is not hypothetical. `issues_raw.reopen_reason` had no id family until
2026-09-13: titles, descriptions, comments, wiki pages and every catalog had
one, that column did not. The 0.22 release clip put ten English sentences in
the middle of a Korean retro — "Added a regression test so this cannot come
back silently." under a Korean title — and check.py was green the whole time
(GDK-1847).

A vision pass did not save it either, and could not have: the pre-release ko
take was recorded 2026-09-09 and judged, and on that day the surprises list
held one badge-only row, so there was no English on screen to see. The
fixture was regenerated on 09-11 (reopened 87 -> 95, reasons 42 -> 49), the
list filled up, and the clip was never re-shot. A review is pinned to the
frames it saw; nothing invalidated it when the screen changed underneath.
So the check has to be a gate that runs every time, not a person.

The census shape is the point. Every TEXT column of the user-visible tables
is checked unless it is named in WIRE below, so a column added next quarter
is checked by default and has to be argued out rather than remembered in.
That is the part that closes the class — the English sentence was only the
symptom.

The same argument applies one level up, to tables, and had to be learned a
second time (GDK-1937): the loop below used to walk `WIRE.items()`, so a
table nobody enrolled was invisible — `links`, `attachments`, `changelog`
among them — and the ja fixture reported "42 viewer-visible columns checked,
0 untranslated values" while `links.type` held "Blocks", the English word
the phone prints on every linked-issue row. Tables are now enumerated from
the database itself (`sqlite_master`); a table with no WIRE entry is checked
in full, so enrolling a table is no longer something a person has to
remember either. Two escapes remain, both deliberate and both printed by
--plan: a column in WIRE (a reason string, not just a name — the reason is
data the plan shows), and a table in ALL_WIRE (machinery nothing renders;
an explicit decision, never silence).

Exit 0 when clean, 1 with the offending rows, 2 on usage.
"""
import importlib.util, json, sqlite3, sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def load_check():
    """Reuse check.py's detectors — one definition of "this is English"."""
    spec = importlib.util.spec_from_file_location("dcheck", Path(__file__).with_name("check.py"))
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


CHECK = load_check()

# Columns that are wire, not prose: ids, categories, enums, timestamps, JSON
# and ADF payloads (the ADF is checked through the plain-text column beside
# it, which is generated from the same tree). Anything not listed here is
# read as something a viewer can see. Each entry carries its reason as the
# value, so --plan can print it and the decision stays data, not memory.
#
# items.external_id, issues_raw.sprint_id/security_level_id and
# comments.external_id are not here on purpose: the pre-discovery census
# checked them, and this round does not relax what it did not tighten — they
# hold ids/empties and never trip.
WIRE = {
    "items": {
        "id": "row id",
        "source_id": "source id",
        "kind": "item kind enum (issue/page)",
        "key": "issue/page key",
        "url": "URL",
        "author_id": "account id",
        "created_at": "timestamp",
        "updated_at": "timestamp",
        "synced_at": "timestamp",
    },
    "issues_raw": {
        "item_id": "row id into items",
        "key": "issue key",
        "issue_type_id": "issue type id (the display name is the checked column)",
        "status_id": "status id (the display name is the checked column)",
        "status_category": "category enum (new/inprogress/done) — the axis code keys on",
        "priority_id": "priority id",
        "priority_rank": "sort rank",
        "assignee_id": "account id",
        "assignee_email": "email",
        "reporter_id": "account id",
        "reporter_email": "email",
        "epic_key": "issue key",
        "parent_key": "issue key",
        "resolution_id": "resolution id (the display name is the checked column)",
        "sprint_state": "state enum (future/active/closed)",
        "fix_version_ids": "id list, JSON",
        "labels": "label list, JSON",
        "description_adf": "ADF payload; body_text beside it is generated from the same tree",
        "custom": "custom field map, JSON",
        "raw": "origin JSON blob",
        "created_at": "timestamp",
        "updated_at": "timestamp",
        "resolved_at": "timestamp",
        "status_changed_at": "timestamp",
        "reopened_at": "timestamp",
        "assignee_changed_at": "timestamp",
        "started_at": "timestamp",
        "last_activity_at": "timestamp",
        "first_sprint_at": "timestamp",
        "blocked_since": "timestamp",
    },
    "comments": {
        "id": "comment id",
        "item_id": "row id into items",
        "author_id": "account id",
        "body_adf": "ADF payload; body_text beside it is generated from the same tree",
        "created_at": "timestamp",
        "updated_at": "timestamp",
    },
    # pages.status is the Confluence lifecycle enum ("current"/"draft"), not a
    # word the reader sees.
    "pages": {
        "item_id": "row id into items",
        "space_key": "space key",
        "labels": "label list, JSON",
        "body_adf": "ADF payload; excerpt beside it is generated from the same tree",
        "status": "lifecycle enum (current/draft)",
    },
    "sprints": {
        "source_id": "source id",
        "id": "sprint id (the name and goal are the checked columns)",
        "board_id": "board id",
        "state": "state enum (future/active/closed)",
        "start_at": "timestamp",
        "end_at": "timestamp",
        "complete_at": "timestamp",
        "activated_at": "timestamp",
    },
    "boards": {
        "source_id": "source id",
        "id": "board id (the name is the checked column)",
        "type": "board type enum (scrum/kanban)",
    },
    "versions": {
        "id": "version id (the name is the checked column)",
        "project_key": "project key",
        "released": "boolean",
        "archived": "boolean",
        "release_date": "date",
    },
    # ——— enrolled by table discovery, 2026-09-16 (GDK-1937) ———
    "attachments": {
        "id": "attachment id",
        "item_id": "row id into items",
        "external_id": "origin id",
        "filename": "the literal file name — a fact, not prose",
        "mime_type": "media type",
        "size": "bytes",
        "author_id": "account id",
        "url": "URL",
        "created_at": "timestamp",
    },
    "changelog": {
        "id": "changelog row id",
        "item_id": "row id into items",
        "at": "timestamp",
        "field": "machine field name; the UI maps it through its own i18n (fieldLabel)",
        "from_id": "catalog id of from_value (status id, sprint id)",
        "to_id": "catalog id of to_value (status id, sprint id)",
        "author_id": "account id",
        # from_value / to_value are deliberately absent: the web history
        # timeline and the retro's sprint log render them raw, so they are
        # prose on camera (status, sprint and assignee display names, link
        # phrases like "This work item blocks NMA-24").
    },
    "dev_links": {
        "item_id": "row id into items",
        "kind": "enum the reader filters on (pullrequest)",
        "external_id": "origin id",
        "url": "URL",
        "status": "state token; PrList keys its chip colour on it — localizing it belongs to the UI's i18n, not the fixture",
        "updated_at": "timestamp",
        "actor": "account id",
        "actor_name": "person name — Latin on purpose in every locale",
        "branch": "git ref",
        "environment": "deploy environment name, machine-read; empty in the demo fixture",
        # title is deliberately absent: the PR title renders in the detail
        # panel's PR list (ListLinkedPRs → PrList).
    },
    "links": {
        "item_id": "row id into items",
        "direction": "wire pair (inward/outward) the add form and dedupe key on",
        "target_key": "issue key of the far end",
        # type is deliberately absent: the phone's detail screen prints it
        # raw as the linked-issue label ("Blocks") — the very defect this
        # round exists for (GDK-1937).
    },
    "link_types": {
        "source_id": "source id",
        "id": "link type id",
        # name / inward / outward are deliberately absent: display name and
        # direction phrases; the demo fixture carries no rows, a real
        # origin's catalog rides along (v43).
    },
    "remote_links": {
        "item_id": "row id into items",
        "id": "remote link id",
        "global_id": "origin global id",
        "relationship": "relationship enum",
        "url": "URL",
        # title / summary are deliberately absent: curated prose
        # (`gadak link KEY <url> --title`); empty in the demo fixture.
    },
    "page_versions": {
        "item_id": "row id into items",
        "number": "version number",
        "created_at": "timestamp",
        "author_id": "account id",
        "author_name": "person name — Latin on purpose in every locale",
        "minor_edit": "boolean",
        # message is deliberately absent: the version's commit-message-like
        # prose; empty in the demo fixture.
    },
    "spaces": {
        "source_id": "source id",
        "key": "space key",
        "kind": "space kind enum",
        "homepage_id": "page id",
        "watermark": "sync watermark",
        # name is deliberately absent: the space's display name; empty in
        # the demo fixture.
    },
    "users": {
        "source_id": "source id",
        "account_id": "account id",
        "email": "email",
        "account_type": "account type enum (atlassian/customer)",
        "name": "person name — Latin on purpose in every locale",
    },
    "item_refs": {
        "item_id": "row id into items",
        "target_kind": "target kind enum (issue/page)",
        "target_key": "the referenced item's key",
        "via": "how the ref was found (text)",
    },
    "source_queries": {
        "id": "row id",
        "source_id": "source id",
        "external_id": "origin id",
        "query_text": "the query itself (JQL)",
        "config": "filter config, JSON",
        "favourite": "boolean",
        "applied": "applied view list, JSON",
        "unsupported": "unsupported field list, JSON",
        "updated_at": "timestamp",
        # name is deliberately absent: the saved search's display name;
        # empty in the demo fixture.
    },
    "status_catalog": {
        "source_id": "source id",
        "status_id": "status id",
        "category": "category enum — the axis code keys on",
    },
}

# Whole tables that carry nothing a viewer reads, ever: sync machinery,
# counters, tombstones. An explicit decision with the reason attached —
# never silence. A table here is skipped even when it grows a new column;
# if that ever stops being true the table moves up into WIRE.
ALL_WIRE = {
    "api_usage": "sync-engine throttle counters",
    "deleted_items": "reconciliation tombstones: key, source, timestamp",
    "enrichments": "plugin payload cache — payload is a JSON blob the server parses (like items.raw); the demo fixture carries no plugin rows",
    "field_usage": "internal field-fill statistics (project → alias → counts)",
    "sources": "sync source config: kind, base URL, watermark",
    "sync_progress": "sync progress counters",
    "sync_runs": "sync run log: counts and error text for diagnostics",
    "sync_state": "sync bookmarks and health: watermarks, schema version, last error",
}

# Author names and other people-shaped strings are Latin on purpose in every
# locale — a Korean team's tracker still says "Alex Kim".
PEOPLE_COLUMNS = {"author", "assignee", "reporter", "version_by", "created_by", "updated_by", "owner"}
PEOPLE_REASON = "person name — Latin on purpose in every locale"


def is_untranslated(value: str, locale: str) -> bool:
    """English prose with no letter of the target script anywhere in it."""
    if not isinstance(value, str) or not value.strip():
        return False
    if CHECK.SCRIPT[locale].search(value):
        return False  # mixed is fine: a Korean sentence may quote a flag
    return CHECK.needs_translation(value)


def columns_of(con, table: str) -> list[str]:
    return [r[1] for r in con.execute(f"PRAGMA table_info({table})")]


def index_machinery(con) -> tuple[set[str], set[str]]:
    """FTS5 virtual tables and their shadow tables — index, not data.

    items_fts is contentless: apply.py drops and rebuilds it from the
    translated text, so its columns are derived, never prose.
    """
    virtual = {r[0] for r in con.execute(
        "SELECT name FROM sqlite_master WHERE type='table' AND sql LIKE 'CREATE VIRTUAL TABLE%'")}
    shadows = {f"{v}_{s}" for v in virtual for s in ("config", "data", "docsize", "idx")}
    return virtual, shadows


def discovered_tables(con) -> list[str]:
    """Every table in the database, minus SQLite's own bookkeeping."""
    return [r[0] for r in con.execute(
        "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name")]


def row_key(table: str, cols: list[str]) -> tuple[str, str]:
    """The key expression and FROM clause that names a failing row.

    Preference order: the table's own key, the owning item's key (an issue
    key a person can open — the raw item uuid names nothing), the table's
    own id, then rowid. A failure that cannot be traced to a row is a
    failure nobody fixes.
    """
    if "key" in cols:
        return f'"{table}"."key"', f'"{table}"'
    if "item_id" in cols:
        return (f'COALESCE(it."key", "{table}"."item_id")',
                f'"{table}" LEFT JOIN items it ON it."id" = "{table}"."item_id"')
    if "id" in cols:
        return f'"{table}"."id"', f'"{table}"'
    return f'"{table}".rowid', f'"{table}"'


def plan(con, db: Path) -> int:
    """Print every table, every column, and each skip's reason; check nothing."""
    virtual, shadows = index_machinery(con)
    names = discovered_tables(con)
    print(f"census plan for {db.name} — {len(names)} tables in sqlite_master")
    n_checked = n_wire = 0
    for table in names:
        if table in virtual:
            print(f"\n{table}: ALL skipped — FTS5 virtual table, rebuilt by apply.py from the translated text")
            continue
        if table in shadows:
            print(f"\n{table}: ALL skipped — FTS5 shadow table of the index above")
            continue
        cols = columns_of(con, table)
        if table in ALL_WIRE:
            print(f"\n{table} ({len(cols)} columns): ALL skipped — {ALL_WIRE[table]}")
            for c in cols:
                print(f"  {c:<18} wire")
            continue
        wire = WIRE.get(table, {})
        print(f"\n{table} ({len(cols)} columns){'' if wire or table in WIRE else ' — no WIRE entry: checked in full'}")
        for c in cols:
            if c in wire:
                print(f"  {c:<18} wire: {wire[c]}")
                n_wire += 1
            elif c in PEOPLE_COLUMNS:
                print(f"  {c:<18} wire: {PEOPLE_REASON}")
                n_wire += 1
            else:
                print(f"  {c:<18} CHECK")
                n_checked += 1
        for c in wire:
            if c not in cols:
                print(f"  {c:<18} STALE — named in WIRE but no such column")
    absent = sorted((set(WIRE) | set(ALL_WIRE)) - set(names))
    for table in absent:
        print(f"\n{table}: enrolled but absent from this database (inert)")
    print(f"\n{n_checked} columns checked, {n_wire} skipped as wire")
    return 0


def main() -> int:
    args = [a for a in sys.argv[1:] if not a.startswith("--")]
    if len(args) < 1:
        print(__doc__)
        return 2
    db = Path(args[0])
    if not db.exists():
        print(f"mirror not found: {db}")
        return 2
    con = sqlite3.connect(f"file:{db}?mode=ro", uri=True)

    if "--plan" in sys.argv:
        return plan(con, db)

    if len(args) < 2:
        print(__doc__)
        return 2
    locale = args[1]
    limit = int(sys.argv[sys.argv.index("--limit") + 1]) if "--limit" in sys.argv else 25
    if locale not in CHECK.SCRIPT:
        print(f"unknown locale {locale}")
        return 2

    virtual, shadows = index_machinery(con)
    bad: list[tuple[str, str, str, str]] = []
    checked_cols = 0

    for table in discovered_tables(con):
        if table in virtual or table in shadows:
            continue  # index machinery, rebuilt from the checked text
        if table in ALL_WIRE:
            continue
        cols = columns_of(con, table)
        wire = WIRE.get(table, {})
        unknown = [c for c in cols if c not in wire and c not in PEOPLE_COLUMNS]
        for col in unknown:
            checked_cols += 1
            keyexpr, from_clause = row_key(table, cols)
            try:
                rows = con.execute(
                    f'SELECT {keyexpr}, "{table}"."{col}" FROM {from_clause} '
                    f'WHERE "{table}"."{col}" IS NOT NULL AND "{table}"."{col}" != ""'
                ).fetchall()
            except sqlite3.OperationalError:
                continue
            for key, value in rows:
                if not isinstance(value, str):
                    continue
                # A JSON array column (components, labels) is a list of
                # display names; check the names, not the brackets.
                if value.startswith("[") and value.endswith("]"):
                    try:
                        for entry in json.loads(value):
                            if isinstance(entry, str) and is_untranslated(entry, locale):
                                bad.append((table, col, str(key), entry))
                        continue
                    except json.JSONDecodeError:
                        pass
                if is_untranslated(value, locale):
                    bad.append((table, col, str(key), value))

    for table, col, key, value in bad[:limit]:
        one = " ".join(value.split())
        print(f"FAIL {table}.{col} [{key}]: {one[:100]}")
    if len(bad) > limit:
        print(f"  … and {len(bad) - limit} more")
    print(f"{db.name}: {checked_cols} viewer-visible columns checked, {len(bad)} untranslated values")
    if bad:
        print()
        print(f"  These strings are on camera in a {locale} take and are English.")
        print("  If the column has no id family in tools/demo-i18n/extract.py, add one —")
        print("  a complete translation file cannot cover an id that is never extracted.")
        print("  If the column is wire and not prose, name it in WIRE here with the reason;")
        print("  if the whole table is machinery, say so in ALL_WIRE. --plan shows every decision.")
    return 1 if bad else 0


if __name__ == "__main__":
    sys.exit(main())
