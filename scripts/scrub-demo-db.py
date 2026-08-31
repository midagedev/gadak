#!/usr/bin/env python3
"""Scrub a demo-profile mirror into a committable examples/demo.db.

The live demo profile syncs from a real Atlassian site, so its database holds
the real account email, display name, and site host — including inside raw
API payloads kept for field re-discovery. The committed snapshot must carry
fictional values only (tasks T6.5/T6.8). This script re-applies that scrub to
a fresh copy, so regenerating the snapshot is one command instead of a
remembered ritual:

    python3 scripts/scrub-demo-db.py ~/.gadak/profiles/demo/gadak.db examples/demo.db
    ./scripts/scan-internal.sh   # must pass before committing

Replacements (matching the original T6.5 scrub of the committed snapshot):
  - real account emails            -> demo@example.com
  - the owner's real display name  -> Alex Kim
  - the real site host             -> nimbus.example.com
Gravatar URLs are left as-is, same as the original scrub (opaque hashes,
already in the committed snapshot).

The snapshot is also served raw and opened in the reader's browser by
Datasette Lite (GDK-101), whose SQLite (pyodide) is older than the mirror's
build target: `contentless_delete=1` (SQLite 3.43+, internal/store/schema.go)
makes every Lite page fail with `unrecognized option`. The mirror keeps the
option — its engine is modern and needs row replacement — but the snapshot is
read-only, so this script rebuilds its FTS without it (same tokenizer, same
content, verified by MATCH-count probes). CI rejects a snapshot that regresses
on this (see the "snapshot portability" step in ci.yml).
"""

import re
import shutil
import sqlite3
import sys

REPLACEMENTS = [
    (re.compile(r"midagedev[+A-Za-z0-9._-]*@gmail\.com"), "demo@example.com"),
    (re.compile(r"김현철"), "Alex Kim"),
    (re.compile(r"midagedev\.atlassian\.net"), "nimbus.example.com"),
    # Invited teammate accounts whose display name is still the email local
    # part; the committed snapshot maps them to the T6.8 personas.
    (re.compile(r"midagedev\+dana"), "Dana Whitfield"),
    (re.compile(r"midagedev\+marco"), "Marco Reyes"),
    (re.compile(r"midagedev\+priya"), "Priya Sharma"),
]


def scrub_text(value: str) -> str:
    for pat, repl in REPLACEMENTS:
        value = pat.sub(repl, value)
    return value


# CJK rune ranges for the cjk_bigram column — must stay in lockstep with
# internal/store/cjk.go (cjkRanges): if the two disagree, the portable
# snapshot's cjk_bigram silently diverges from what a local mirror writes.
CJK_RANGES = [
    (0x1100, 0x11FF),   # Hangul Jamo
    (0x3041, 0x30FF),   # Hiragana + Katakana
    (0x3130, 0x318F),   # Hangul Compatibility Jamo
    (0x31F0, 0x31FF),   # Katakana Phonetic Extensions
    (0x3400, 0x4DBF),   # CJK Extension A
    (0x4E00, 0x9FFF),   # CJK Unified Ideographs
    (0xA960, 0xA97F),   # Hangul Jamo Extended-A
    (0xAC00, 0xD7A3),   # Hangul Syllables
    (0xD7B0, 0xD7FF),   # Hangul Jamo Extended-B
    (0xF900, 0xFAFF),   # CJK Compatibility Ideographs
    (0x20000, 0x2FFFD),  # CJK Extensions (Plane 2)
    (0x30000, 0x3FFFD),  # CJK Extension G+ (Plane 3)
]


def cjk_bigrams(text: str) -> list[str]:
    """Overlapping 2-grams of CJK runs; 1-rune runs emit nothing (0009)."""
    grams: list[str] = []
    run = ""
    for ch in text:
        if any(lo <= ord(ch) <= hi for lo, hi in CJK_RANGES):
            run += ch
            continue
        grams.extend(run[i : i + 2] for i in range(len(run) - 1))
        run = ""
    grams.extend(run[i : i + 2] for i in range(len(run) - 1))
    return grams


def cjk_bigram_column(title: str, body: str, comments: str) -> str:
    """Mirror of store.FTSCJKBigramColumn — the items_fts.cjk_bigram value."""
    parts = []
    for text in (title, body, comments):
        grams = cjk_bigrams(text)
        if grams:
            parts.append(" ".join(grams))
    return " ".join(parts)


def rebuild_portable_fts(con: sqlite3.Connection) -> None:
    """Rebuild items_fts without options Datasette Lite's SQLite cannot parse.

    Contentless tables return no stored text, so parity is checked with MATCH
    counts over a fixed probe set (plus total row count) before and after.
    Comment text is re-concatenated in insertion order, matching writeFTS in
    internal/store/write.go. The cjk_bigram fourth column is computed here in
    Python (SQL cannot emit overlapping 2-grams) and must match
    store.FTSCJKBigramColumn, or the hosted snapshot silently loses CJK
    mid-compound search (GDK-259 / docs/decisions/0009).
    """
    fts_sql = con.execute(
        "SELECT sql FROM sqlite_master WHERE name = 'items_fts'"
    ).fetchone()
    if not fts_sql:
        return
    ddl = fts_sql[0] or ""
    if "contentless_delete" not in ddl and "cjk_bigram" in ddl:
        return  # already the portable shape this build produces

    probes = ["upload", "retri*", "webhook AND retry", "로그인"]
    before = {"rows": con.execute("SELECT count(*) FROM items_fts").fetchone()[0]}
    before.update(
        {p: con.execute(
            "SELECT count(*) FROM items_fts WHERE items_fts MATCH ?", (p,)
        ).fetchone()[0] for p in probes}
    )

    con.execute("DROP TABLE items_fts")
    con.execute(
        "CREATE VIRTUAL TABLE items_fts USING fts5("
        "title, body_text, comments_text, cjk_bigram, content='', "
        "tokenize='unicode61 remove_diacritics 2')"
    )
    rows = con.execute(
        """
        SELECT i.rowid, i.title, COALESCE(i.body_text, ''),
               COALESCE((SELECT group_concat(body_text, char(10))
                         FROM (SELECT body_text FROM comments
                               WHERE item_id = i.id AND body_text <> ''
                               ORDER BY rowid)), '')
        FROM items i
        """
    ).fetchall()
    con.executemany(
        "INSERT INTO items_fts (rowid, title, body_text, comments_text, cjk_bigram) "
        "VALUES (?, ?, ?, ?, ?)",
        [
            (rowid, title, body, comments, cjk_bigram_column(title, body, comments))
            for rowid, title, body, comments in rows
        ],
    )

    after = {"rows": con.execute("SELECT count(*) FROM items_fts").fetchone()[0]}
    after.update(
        {p: con.execute(
            "SELECT count(*) FROM items_fts WHERE items_fts MATCH ?", (p,)
        ).fetchone()[0] for p in probes}
    )
    if before != after:
        raise SystemExit(
            f"FTS rebuild changed search behavior: {before} -> {after}"
        )
    con.commit()
    print(f"rebuilt items_fts without contentless_delete ({after['rows']} rows)")


def main() -> int:
    if len(sys.argv) != 3:
        print(__doc__.strip().splitlines()[0])
        print("usage: scrub-demo-db.py <source.db> <target.db>")
        return 2
    src, dst = sys.argv[1], sys.argv[2]

    # Checkpoint the source so the copy is self-contained, then copy bytes.
    con = sqlite3.connect(src)
    con.execute("PRAGMA wal_checkpoint(TRUNCATE)")
    con.close()
    shutil.copyfile(src, dst)

    con = sqlite3.connect(dst)
    con.row_factory = sqlite3.Row
    tables = [
        r[0]
        for r in con.execute(
            "SELECT name FROM sqlite_master WHERE type='table'"
            " AND name NOT LIKE 'sqlite_%' AND name NOT LIKE 'items_fts%'"
        )
    ]
    total = 0
    for table in tables:
        cols = con.execute(f"PRAGMA table_info({table})").fetchall()
        text_cols = [c["name"] for c in cols if (c["type"] or "").upper().startswith("TEXT")]
        if not text_cols:
            continue
        pks = [c["name"] for c in cols if c["pk"]] or ["rowid"]
        pk_expr = ", ".join(pks)
        rows = con.execute(f"SELECT {pk_expr}, {', '.join(text_cols)} FROM {table}").fetchall()
        for row in rows:
            updates = {}
            for col in text_cols:
                v = row[col]
                if isinstance(v, str):
                    s = scrub_text(v)
                    if s != v:
                        updates[col] = s
            if updates:
                set_expr = ", ".join(f"{c} = ?" for c in updates)
                where = " AND ".join(f"{k} = ?" for k in pks)
                con.execute(
                    f"UPDATE {table} SET {set_expr} WHERE {where}",
                    [*updates.values(), *[row[k] for k in pks]],
                )
                total += 1
    con.commit()

    rebuild_portable_fts(con)

    con.execute("VACUUM")
    # The snapshot is served as bare bytes (raw.githubusercontent) and opened
    # by tools that may not be able to create -shm/-wal siblings; leave it as
    # a plain rollback-journal file.
    mode = con.execute("PRAGMA journal_mode=DELETE").fetchone()[0]
    if mode.lower() != "delete":
        print(f"could not leave the snapshot in rollback-journal mode (got {mode})",
              file=sys.stderr)
        return 1

    fts_sql = con.execute(
        "SELECT sql FROM sqlite_master WHERE name = 'items_fts'"
    ).fetchone()
    if fts_sql and "contentless_delete" in (fts_sql[0] or ""):
        print("items_fts still carries contentless_delete after rebuild", file=sys.stderr)
        return 1
    if fts_sql and "cjk_bigram" not in (fts_sql[0] or ""):
        print("items_fts lost the cjk_bigram column in the portable rebuild", file=sys.stderr)
        return 1

    # e2e/person.spec.ts opens both kinds of comment from one person's panel,
    # and the panel loads the newest 50 (CommentsByAuthorDefaultLimit). The
    # snapshot re-dates comments, so whether a page comment survives inside
    # that window is regen luck — one regen pushed Alex Kim's first page
    # comment from rank 46 to 88 and only CI noticed. Pin the premise instead:
    # his newest page comment gets re-dated next to his newest comment.
    newest = con.execute(
        "SELECT MAX(c.created_at) FROM comments c WHERE c.author = 'Alex Kim'"
    ).fetchone()[0]
    if newest:
        con.execute(
            """
            UPDATE comments SET created_at = datetime(?, '-1 minute')
            WHERE rowid = (
              SELECT c.rowid FROM comments c
              JOIN items it ON it.id = c.item_id
              WHERE c.author = 'Alex Kim' AND it.kind = 'page'
              ORDER BY c.created_at DESC LIMIT 1
            )
            """,
            (newest,),
        )
        con.commit()

    leftovers = 0
    for table in tables:
        cols = con.execute(f"PRAGMA table_info({table})").fetchall()
        for c in cols:
            if not (c["type"] or "").upper().startswith("TEXT"):
                continue
            name = c["name"]
            n = con.execute(
                f"SELECT count(*) FROM {table} WHERE {name} LIKE '%midagedev%'"
                f" OR {name} LIKE '%김현철%'"
            ).fetchone()[0]
            leftovers += n
    con.close()

    print(f"scrubbed {total} rows across {len(tables)} tables; leftovers: {leftovers}")
    return 0 if leftovers == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
