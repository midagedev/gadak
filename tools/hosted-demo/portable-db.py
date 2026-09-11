#!/usr/bin/env python3
"""Publish-side Datasette Lite strip (GDK-101 / GDK-1756).

The committed examples/demo.db keeps the store's canonical items_fts DDL —
contentless_delete=1 included — because every store.Open of a stripped copy
paid a full index rebuild before doing anything (GDK-1756). pyodide's SQLite
still predates the option (3.43), so the file Datasette Lite actually
downloads must not carry it: this script copies the mirror to the publish
target and rebuilds items_fts there without the option, reusing the scrub
script's reload and MATCH-probe parity (scripts/scrub-demo-db.py). The strip
is never allowed back onto the committed fixture.

Called from tools/hosted-demo/build.mjs, which asserts the source carries the
canonical DDL first, so a stale fixture fails the build instead of letting
this strip silently no-op.

    python3 tools/hosted-demo/portable-db.py examples/demo.db dist/hosted/demo/gadak-demo.db
"""

import importlib.util
import re
import shutil
import sqlite3
import sys
from pathlib import Path


def load_scrub_module():
    """The reload + probe helpers live in scripts/scrub-demo-db.py; import it
    by path (its name has hyphens) instead of growing a second copy of the
    FTS reload — a diverging copy is how the fixture DDL drifted (GDK-1756)."""
    root = Path(__file__).resolve().parents[2]
    spec = importlib.util.spec_from_file_location(
        "scrub_demo_db", root / "scripts" / "scrub-demo-db.py"
    )
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)  # module main() is __main__-guarded
    return module


def main() -> int:
    if len(sys.argv) != 3:
        print(__doc__.strip().splitlines()[0])
        print("usage: portable-db.py <source.db> <target.db>")
        return 2
    src, dst = sys.argv[1], sys.argv[2]
    scrub = load_scrub_module()

    con = sqlite3.connect(f"file:{src}?mode=ro", uri=True)
    row = con.execute(
        "SELECT sql FROM sqlite_master WHERE name = 'items_fts'"
    ).fetchone()
    con.close()
    if not row:
        print(f"{src} has no items_fts — not a gadak mirror", file=sys.stderr)
        return 1
    ddl = row[0] or ""
    if "contentless_delete" not in ddl:
        print(f"{src} items_fts lacks contentless_delete — the committed "
              "fixture must be canonical (GDK-1756); regenerate it with "
              "`make demo-fixture`", file=sys.stderr)
        return 1

    # Drop the option and its leading comma wherever it sits in the option
    # list; the lookahead swallows neither the next option's comma nor the
    # closing paren.
    stripped = re.sub(r",\s*contentless_delete\s*=\s*1(?=\s*,|\s*\))", "", ddl, count=1)
    if "contentless_delete" in stripped:
        print(f"could not strip contentless_delete from: {ddl!r}", file=sys.stderr)
        return 1

    shutil.copyfile(src, dst)
    con = sqlite3.connect(dst)
    scrub.rebuild_fts(con, stripped)
    con.execute("VACUUM")
    # The published file is served as bare bytes (raw.githubusercontent) and
    # opened without being able to create -shm/-wal siblings.
    mode = con.execute("PRAGMA journal_mode=DELETE").fetchone()[0]
    if mode.lower() != "delete":
        print(f"could not leave {dst} in rollback-journal mode (got {mode})",
              file=sys.stderr)
        return 1
    final = con.execute(
        "SELECT sql FROM sqlite_master WHERE name = 'items_fts'"
    ).fetchone()[0] or ""
    con.close()
    if "contentless_delete" in final:
        print(f"{dst} still carries contentless_delete after the strip", file=sys.stderr)
        return 1
    print(f"published {dst} with a portable items_fts (no contentless_delete)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
