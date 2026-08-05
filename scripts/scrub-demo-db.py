#!/usr/bin/env python3
"""Scrub a demo-profile mirror into a committable examples/demo.db.

The live demo profile syncs from a real Atlassian site, so its database holds
the real account email, display name, and site host — including inside raw
API payloads kept for field re-discovery. The committed snapshot must carry
fictional values only (tasks T6.5/T6.8). This script re-applies that scrub to
a fresh copy, so regenerating the snapshot is one command instead of a
remembered ritual:

    python3 scripts/scrub-demo-db.py ~/.scry/profiles/demo/scry.db examples/demo.db
    ./scripts/scan-internal.sh   # must pass before committing

Replacements (matching the original T6.5 scrub of the committed snapshot):
  - real account emails            -> demo@example.com
  - the owner's real display name  -> Alex Kim
  - the real site host             -> nimbus.example.com
Gravatar URLs are left as-is, same as the original scrub (opaque hashes,
already in the committed snapshot).
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
    con.execute("VACUUM")

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
