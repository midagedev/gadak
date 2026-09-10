#!/usr/bin/env python3
"""Census of every items_fts writer in the tree (GDK-1021).

Why this exists as a standing tool rather than a one-off grep: items_fts is a
contentless FTS5 table, so a writer that omits a column does not fail. It
produces an index that is simply empty on that axis, and search quietly stops
returning a whole class of hit. docs/decisions/0009 names this as the design's
trap, and it has now cost two rounds -- cjk_bigram (GDK-259) and labels
(GDK-1021). The second time, the writer that lagged behind the schema was
tools/demo-i18n/apply.py: it drops and recreates items_fts with its own DDL to
re-tokenise the translated ko/ja recording fixture, and no Go test compiles it,
so every Go gate stayed green while the localized fixture lost label search.

Run it directly to see the census (every writer and the columns it names):

    python3 tools/fts-writer-census.py --list

tools/doc-checks.sh check 49 runs it in assert mode, which is the gate.

The canonical column list and tokenizer are read from internal/store/schema.go
rather than duplicated here, so a future column lands in one place and this
census follows it.
"""

import argparse
import re
import subprocess
import sys

SCHEMA = "internal/store/schema.go"
# Files that can hold SQL against items_fts. Go and Python are the writers
# today; the others are here so a new surface is covered the day it appears.
GLOBS = ["*.go", "*.py", "*.sql", "*.ts", "*.mjs"]

INSERT_RE = re.compile(r"INSERT INTO items_fts\s*\(([^)]*)\)")
DDL_RE = re.compile(r"CREATE VIRTUAL TABLE items_fts USING fts5\(([^)]*)\)")
TOKENIZE_RE = re.compile(r"tokenize\s*=\s*[\"']([^\"']+)[\"']")


def canonical():
    """Read the column list and tokenizer from the canonical DDL."""
    body = open(SCHEMA, encoding="utf-8").read()
    m = DDL_RE.search(" ".join(body.split()))
    if not m:
        sys.exit(f"{SCHEMA}: the canonical items_fts DDL did not parse -- this census has drifted")
    spec = m.group(1)
    tok = TOKENIZE_RE.search(spec)
    if not tok:
        sys.exit(f"{SCHEMA}: the canonical DDL names no tokenizer -- this census has drifted")
    return columns_of(spec), tok.group(1)


def columns_of(spec):
    """The bare column names in an fts5() argument list: no options, no rowid."""
    out = []
    for part in spec.split(","):
        part = part.strip()
        if not part or "=" in part or part.startswith("tokenize"):
            continue
        part = part.strip("\"'` ")
        if part and part != "rowid":
            out.append(part)
    return out


def tracked_files():
    out = subprocess.run(
        ["git", "ls-files", "-z", *GLOBS],
        capture_output=True, text=True, check=True,
    ).stdout
    return [p for p in out.split("\0") if p]


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--list", action="store_true",
                    help="print every writer and the columns it names, then exit 0")
    args = ap.parse_args()

    cols, tokenizer = canonical()
    findings, rows = [], []
    n_insert = n_ddl = 0

    for path in tracked_files():
        try:
            body = open(path, encoding="utf-8").read()
        except (OSError, UnicodeDecodeError):
            continue
        if "items_fts" not in body:
            continue
        flat = " ".join(body.split())

        for m in INSERT_RE.finditer(flat):
            n_insert += 1
            named = columns_of(m.group(1))
            rows.append((path, "INSERT", named, ""))
            if named != cols:
                findings.append(f"{path}: INSERT INTO items_fts names {named}, want {cols}")

        for m in DDL_RE.finditer(flat):
            n_ddl += 1
            spec = m.group(1)
            named = columns_of(spec)
            tok = TOKENIZE_RE.search(spec)
            rows.append((path, "CREATE", named, tok.group(1) if tok else ""))
            if named != cols:
                findings.append(f"{path}: items_fts DDL declares {named}, want {cols}")
            if not tok or tok.group(1) != tokenizer:
                got = tok.group(1) if tok else "(none)"
                findings.append(f"{path}: items_fts DDL tokenizer is {got!r}, want {tokenizer!r}")

    if args.list:
        print(f"canonical ({SCHEMA}): {cols}  tokenize={tokenizer!r}")
        for path, kind, named, tok in rows:
            extra = f"  tokenize={tok!r}" if tok else ""
            print(f"  {kind:6} {path}: {named}{extra}")
        return 0

    # A census that matches nothing is a broken census, not a clean tree.
    if n_insert == 0 or n_ddl == 0:
        print(f"  items_fts census matched nothing (inserts={n_insert}, ddl={n_ddl})"
              " -- the patterns have drifted from the source")
        return 1

    for f in findings:
        print("  " + f)
    if findings:
        print("  contentless FTS5 fails silent here: fix the writer, do not narrow"
              " this check (docs/decisions/0009)")
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
