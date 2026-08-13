#!/usr/bin/env python3
"""Import enrichments from a CSV spreadsheet.

Columns: issue_key, kind, payload_json
Optional fourth column: source (defaults to 'csv-import').

Invalid JSON is rejected with a row number. Upserts use ON CONFLICT(key, kind)
so re-importing the same file is idempotent. Bumps sync_state.version once per
successful run (skipped for --dry-run).
"""

from __future__ import annotations

import argparse
import csv
import json
import os
import sqlite3
import sys
import tempfile
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

DEFAULT_SOURCE = "csv-import"
KNOWN_KINDS = frozenset({"deploy", "qa", "prs", "opinion"})


def default_db_path(profile: str | None = None) -> Path:
    home = os.environ.get("GADAK_HOME") or os.environ.get("SCRY_HOME")
    if not home:
        nxt = Path.home() / ".gadak"
        prev = Path.home() / ".scry"
        home = str(nxt if nxt.exists() or not prev.exists() else prev)
    base = Path(home)
    if profile and profile != "default":
        base = base / "profiles" / profile
    nxt = base / "gadak.db"
    prev = base / "scry.db"
    if nxt.exists() or not prev.exists():
        return nxt
    return prev


def utc_now() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%f")[:-3] + "Z"


def open_db(path: Path) -> sqlite3.Connection:
    conn = sqlite3.connect(str(path), timeout=5.0)
    conn.execute("PRAGMA busy_timeout=5000")
    conn.execute("PRAGMA journal_mode=WAL")
    return conn


def ensure_plugin_tables(conn: sqlite3.Connection) -> None:
    conn.executescript(
        """
        CREATE TABLE IF NOT EXISTS enrichments (
          key        TEXT NOT NULL,
          kind       TEXT NOT NULL,
          payload    TEXT NOT NULL,
          source     TEXT NOT NULL DEFAULT '',
          updated_at TEXT NOT NULL,
          PRIMARY KEY (key, kind)
        );
        CREATE TABLE IF NOT EXISTS sync_state (
          source_id         TEXT PRIMARY KEY,
          watermark         TEXT,
          version           INTEGER NOT NULL DEFAULT 0,
          last_full_sync_at TEXT,
          last_error        TEXT,
          schema_version    INTEGER NOT NULL DEFAULT 0
        );
        INSERT OR IGNORE INTO sync_state (source_id, version) VALUES ('jira', 0);
        """
    )


def parse_csv(path: Path, *, allow_unknown_kind: bool) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    with path.open(newline="", encoding="utf-8-sig") as f:
        reader = csv.DictReader(f)
        if not reader.fieldnames:
            raise SystemExit(f"error: empty CSV: {path}")
        fields = {h.strip().lower(): h for h in reader.fieldnames if h}
        required = ("issue_key", "kind", "payload_json")
        for r in required:
            if r not in fields:
                raise SystemExit(
                    f"error: CSV must have columns issue_key,kind,payload_json "
                    f"(got {reader.fieldnames})"
                )
        key_col = fields["issue_key"]
        kind_col = fields["kind"]
        payload_col = fields["payload_json"]
        source_col = fields.get("source")

        for i, raw in enumerate(reader, start=2):  # header is line 1
            key = (raw.get(key_col) or "").strip()
            kind = (raw.get(kind_col) or "").strip()
            payload_text = raw.get(payload_col)
            if payload_text is None:
                payload_text = ""
            payload_text = payload_text.strip()
            if not key and not kind and not payload_text:
                continue  # blank line
            if not key or not kind:
                raise SystemExit(f"error: row {i}: issue_key and kind are required")
            if not allow_unknown_kind and kind not in KNOWN_KINDS:
                raise SystemExit(
                    f"error: row {i}: unknown kind {kind!r} "
                    f"(expected one of {sorted(KNOWN_KINDS)}; pass --allow-unknown-kind to override)"
                )
            try:
                parsed = json.loads(payload_text)
            except json.JSONDecodeError as e:
                raise SystemExit(
                    f"error: row {i}: invalid JSON in payload_json: {e.msg} "
                    f"(line {e.lineno} col {e.colno})"
                ) from e
            # Re-serialize for stable, compact storage (and to reject non-JSON types that
            # json.loads accepted only because of whitespace — already fine).
            payload = json.dumps(parsed, ensure_ascii=False, separators=(",", ":"))
            source = DEFAULT_SOURCE
            if source_col and (raw.get(source_col) or "").strip():
                source = raw[source_col].strip()
            rows.append({"key": key, "kind": kind, "payload": payload, "source": source})
    return rows


def upsert_rows(
    conn: sqlite3.Connection | None,
    rows: list[dict[str, Any]],
    *,
    dry_run: bool,
) -> int:
    if not rows:
        return 0
    now = utc_now()
    if dry_run:
        for r in rows:
            print(f"[dry-run] {r['key']}\t{r['kind']}\t{r['payload']}")
        return len(rows)
    assert conn is not None
    with conn:
        for r in rows:
            conn.execute(
                """
                INSERT INTO enrichments (key, kind, payload, source, updated_at)
                VALUES (?, ?, ?, ?, ?)
                ON CONFLICT(key, kind) DO UPDATE SET
                  payload = excluded.payload,
                  source = excluded.source,
                  updated_at = excluded.updated_at
                """,
                (r["key"], r["kind"], r["payload"], r["source"], now),
            )
        conn.execute("UPDATE sync_state SET version = version + 1")
    return len(rows)


def run_self_test() -> None:
    with tempfile.TemporaryDirectory() as td:
        td_path = Path(td)
        good = td_path / "good.csv"
        good.write_text(
            "issue_key,kind,payload_json\n"
            'NMB-1,prs,"[{""number"":1,""title"":""fix"",""url"":""https://example.com/1"",""state"":""open"",""repo"":""example/app"",""author"":""alice""}]"\n'
            'NMB-1,opinion,"""looks good"""\n'
            'NMB-2,deploy,"{""status"":{""state"":""prod"",""merged_prs"":1,""total_prs"":1,""dev"":null,""qa_release"":null,""qa_swapped_at"":null,""prod_at"":""2026-01-01T00:00:00Z""},""detail"":{""state"":""prod"",""releases"":[{""tag"":""v1.0.0"",""channel"":""prod""}]}}"\n',
            encoding="utf-8",
        )
        rows = parse_csv(good, allow_unknown_kind=False)
        assert len(rows) == 3

        bad = td_path / "bad.csv"
        bad.write_text(
            "issue_key,kind,payload_json\n"
            "NMB-1,prs,not-json\n",
            encoding="utf-8",
        )
        try:
            parse_csv(bad, allow_unknown_kind=False)
            raise AssertionError("expected invalid JSON rejection")
        except SystemExit as e:
            assert "row 2" in str(e), e

        db = td_path / "test.db"
        conn = open_db(db)
        ensure_plugin_tables(conn)
        n = upsert_rows(conn, rows, dry_run=False)
        n2 = upsert_rows(conn, rows, dry_run=False)
        assert n == n2 == 3
        count = conn.execute("SELECT COUNT(*) FROM enrichments").fetchone()[0]
        assert count == 3  # two kinds for NMB-1 + one for NMB-2
        version = conn.execute("SELECT version FROM sync_state").fetchone()[0]
        assert version == 2, version
        conn.close()
    print("self-test ok")


def build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(
        prog="csv_import.py",
        description="Upsert enrichments from a CSV file (issue_key,kind,payload_json).",
    )
    p.add_argument("csv", nargs="?", help="Path to CSV file (required unless --self-test)")
    p.add_argument(
        "--db",
        default=None,
        help="Path to gadak.db (default: $GADAK_HOME/gadak.db or ~/.gadak/gadak.db)",
    )
    p.add_argument("--profile", default=None, help="gadak profile name (ignored if --db set)")
    p.add_argument(
        "--allow-unknown-kind",
        action="store_true",
        help="Accept kinds outside deploy|qa|prs|opinion (server ignores them until a core PR)",
    )
    p.add_argument("--dry-run", action="store_true", help="Print planned rows; do not write")
    p.add_argument("--self-test", action="store_true", help="Run built-in checks and exit")
    return p


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    if args.self_test:
        run_self_test()
        return 0

    if not args.csv:
        print("error: provide a CSV path", file=sys.stderr)
        return 2
    csv_path = Path(args.csv)
    if not csv_path.exists():
        print(f"error: CSV not found: {csv_path}", file=sys.stderr)
        return 1

    rows = parse_csv(csv_path, allow_unknown_kind=args.allow_unknown_kind)
    print(f"parsed {len(rows)} row(s) from {csv_path}", file=sys.stderr)

    if args.dry_run:
        upsert_rows(None, rows, dry_run=True)
        return 0

    db_path = Path(args.db) if args.db else default_db_path(args.profile)
    if not db_path.exists():
        print(f"error: database not found: {db_path}", file=sys.stderr)
        return 1
    conn = open_db(db_path)
    try:
        n = upsert_rows(conn, rows, dry_run=False)
    finally:
        conn.close()
    print(f"upserted {n} enrichment row(s) into {db_path}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
