#!/usr/bin/env python3
"""Find issue keys in GitHub PRs and write kind='prs' enrichments.

Scans pull-request title, body, and head branch for keys matching
PROJECT-123 (e.g. NMB-42) and upserts a LinkedPr[] payload per issue.

Offline verification: pass --from-json with a GitHub-shaped PR list instead of
calling the network. Live mode needs GH_TOKEN and a repo argument (owner/name).
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sqlite3
import sys
import tempfile
import urllib.error
import urllib.request
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

SOURCE = "github-prs"
ISSUE_KEY_RE = re.compile(r"\b([A-Z][A-Z0-9]+-\d+)\b")
GITHUB_API = "https://api.github.com"


def default_db_path(profile: str | None = None) -> Path:
    home = os.environ.get("SCRY_HOME")
    if not home:
        home = str(Path.home() / ".scry")
    base = Path(home)
    if profile and profile != "default":
        return base / "profiles" / profile / "scry.db"
    return base / "scry.db"


def utc_now() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%f")[:-3] + "Z"


def extract_keys(*texts: str | None) -> set[str]:
    found: set[str] = set()
    for t in texts:
        if not t:
            continue
        found.update(ISSUE_KEY_RE.findall(t))
    return found


def pr_state(pr: dict[str, Any]) -> str:
    if pr.get("merged_at") or pr.get("merged"):
        return "merged"
    state = (pr.get("state") or "").lower()
    if state == "open":
        return "open"
    return "closed"


def linked_pr(pr: dict[str, Any], default_repo: str) -> dict[str, Any]:
    """Build one LinkedPr object (web/src/lib/types.ts)."""
    repo = default_repo
    base = pr.get("base") or {}
    if isinstance(base, dict):
        base_repo = base.get("repo") or {}
        if isinstance(base_repo, dict) and base_repo.get("full_name"):
            repo = base_repo["full_name"]
    author = None
    user = pr.get("user")
    if isinstance(user, dict):
        author = user.get("login")
    elif isinstance(pr.get("author"), str):
        author = pr["author"]
    return {
        "number": int(pr["number"]),
        "title": pr.get("title") or "",
        "url": pr.get("html_url") or pr.get("url") or "",
        "state": pr_state(pr),
        "repo": repo,
        "author": author,
    }


def head_ref(pr: dict[str, Any]) -> str | None:
    head = pr.get("head")
    if isinstance(head, dict):
        return head.get("ref")
    if isinstance(pr.get("branch"), str):
        return pr["branch"]
    return None


def group_prs_by_issue(prs: list[dict[str, Any]], default_repo: str) -> dict[str, list[dict[str, Any]]]:
    by_key: dict[str, list[dict[str, Any]]] = {}
    for pr in prs:
        keys = extract_keys(pr.get("title"), pr.get("body"), head_ref(pr))
        if not keys:
            continue
        linked = linked_pr(pr, default_repo)
        for key in keys:
            bucket = by_key.setdefault(key, [])
            # de-dupe by number within an issue
            if any(p["number"] == linked["number"] for p in bucket):
                continue
            bucket.append(linked)
    for key in by_key:
        by_key[key].sort(key=lambda p: p["number"], reverse=True)
    return by_key


def fetch_github_prs(repo: str, token: str, state: str = "all") -> list[dict[str, Any]]:
    """Paginate GET /repos/{repo}/pulls (GitHub REST)."""
    headers = {
        "Accept": "application/vnd.github+json",
        "User-Agent": "scry-github-prs-plugin",
        "X-GitHub-Api-Version": "2022-11-28",
        "Authorization": f"Bearer {token}",
    }
    out: list[dict[str, Any]] = []
    page = 1
    while True:
        url = f"{GITHUB_API}/repos/{repo}/pulls?state={state}&per_page=100&page={page}"
        req = urllib.request.Request(url, headers=headers)
        try:
            with urllib.request.urlopen(req, timeout=60) as resp:
                chunk = json.loads(resp.read().decode("utf-8"))
        except urllib.error.HTTPError as e:
            body = e.read().decode("utf-8", errors="replace")
            raise SystemExit(f"GitHub API error {e.code} for {repo}: {body[:400]}") from e
        if not isinstance(chunk, list):
            raise SystemExit(f"unexpected GitHub response: {chunk!r:.200}")
        if not chunk:
            break
        out.extend(chunk)
        if len(chunk) < 100:
            break
        page += 1
    return out


def load_prs_from_json(path: Path) -> list[dict[str, Any]]:
    data = json.loads(path.read_text(encoding="utf-8"))
    if isinstance(data, dict) and "pulls" in data:
        data = data["pulls"]
    if not isinstance(data, list):
        raise SystemExit(f"--from-json must be a JSON array of PRs, got {type(data).__name__}")
    return data


def open_db(path: Path) -> sqlite3.Connection:
    conn = sqlite3.connect(str(path), timeout=5.0)
    conn.execute("PRAGMA busy_timeout=5000")
    conn.execute("PRAGMA journal_mode=WAL")
    return conn


def ensure_plugin_tables(conn: sqlite3.Connection) -> None:
    """Minimal schema for self-tests (not a full scry mirror)."""
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


def upsert_prs(
    conn: sqlite3.Connection | None,
    by_key: dict[str, list[dict[str, Any]]],
    *,
    dry_run: bool,
) -> int:
    if not by_key:
        return 0
    now = utc_now()
    if dry_run:
        for key, prs in sorted(by_key.items()):
            print(f"[dry-run] {key}\tprs\t{json.dumps(prs, ensure_ascii=False)}")
        return len(by_key)
    assert conn is not None

    with conn:  # short transaction
        for key, prs in by_key.items():
            payload = json.dumps(prs, ensure_ascii=False, separators=(",", ":"))
            conn.execute(
                """
                INSERT INTO enrichments (key, kind, payload, source, updated_at)
                VALUES (?, 'prs', ?, ?, ?)
                ON CONFLICT(key, kind) DO UPDATE SET
                  payload = excluded.payload,
                  source = excluded.source,
                  updated_at = excluded.updated_at
                """,
                (key, payload, SOURCE, now),
            )
        conn.execute("UPDATE sync_state SET version = version + 1")
    return len(by_key)


def run_self_test() -> None:
    sample = [
        {
            "number": 7,
            "title": "fix the drop NMB-1",
            "body": "Closes NMB-1 and mentions NMB-2 for follow-up.",
            "html_url": "https://github.com/example/app/pull/7",
            "state": "closed",
            "merged_at": "2026-01-15T12:00:00Z",
            "head": {"ref": "fix/NMB-1-drop"},
            "user": {"login": "alice"},
            "base": {"repo": {"full_name": "example/app"}},
        },
        {
            "number": 8,
            "title": "docs only",
            "body": "no issue key here",
            "html_url": "https://github.com/example/app/pull/8",
            "state": "open",
            "merged_at": None,
            "head": {"ref": "docs/readme"},
            "user": {"login": "bob"},
            "base": {"repo": {"full_name": "example/app"}},
        },
        {
            "number": 9,
            "title": "another fix",
            "body": "",
            "html_url": "https://github.com/example/app/pull/9",
            "state": "open",
            "merged_at": None,
            "head": {"ref": "bugfix/NMB-1-retry"},
            "user": {"login": "carol"},
            "base": {"repo": {"full_name": "example/app"}},
        },
    ]
    by_key = group_prs_by_issue(sample, "example/app")
    assert "NMB-1" in by_key, by_key
    assert "NMB-2" in by_key, by_key
    assert len(by_key["NMB-1"]) == 2  # PR 7 and 9
    assert by_key["NMB-1"][0]["state"] in ("merged", "open")
    assert all(p["repo"] == "example/app" for p in by_key["NMB-1"])

    with tempfile.TemporaryDirectory() as td:
        db = Path(td) / "test.db"
        conn = open_db(db)
        ensure_plugin_tables(conn)
        n = upsert_prs(conn, by_key, dry_run=False)
        assert n == 2
        # idempotent second run
        n2 = upsert_prs(conn, by_key, dry_run=False)
        assert n2 == 2
        rows = conn.execute(
            "SELECT key, kind, payload FROM enrichments ORDER BY key"
        ).fetchall()
        assert len(rows) == 2
        version = conn.execute("SELECT version FROM sync_state").fetchone()[0]
        assert version == 2, version
        payload = json.loads(rows[0][2])
        assert isinstance(payload, list)
        assert payload[0]["number"] in (7, 9)
        conn.close()
    print("self-test ok")


def build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(
        prog="github_prs.py",
        description="Write kind=prs enrichments from GitHub pull requests.",
    )
    p.add_argument(
        "repo",
        nargs="?",
        help="GitHub repo as owner/name (required unless --from-json or --self-test)",
    )
    p.add_argument(
        "--db",
        default=None,
        help="Path to scry.db (default: $SCRY_HOME/scry.db or ~/.scry/scry.db)",
    )
    p.add_argument("--profile", default=None, help="scry profile name (ignored if --db set)")
    p.add_argument(
        "--from-json",
        metavar="FILE",
        help="Offline: read a JSON array of PRs instead of calling GitHub",
    )
    p.add_argument(
        "--state",
        default="all",
        choices=("all", "open", "closed"),
        help="GitHub PR state filter (live mode only, default: all)",
    )
    p.add_argument("--dry-run", action="store_true", help="Print planned rows; do not write")
    p.add_argument("--self-test", action="store_true", help="Run built-in checks and exit")
    return p


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    if args.self_test:
        run_self_test()
        return 0

    if not args.from_json and not args.repo:
        print("error: provide REPO (owner/name) or --from-json FILE", file=sys.stderr)
        return 2

    default_repo = args.repo or "example/app"
    if args.from_json:
        prs = load_prs_from_json(Path(args.from_json))
    else:
        token = os.environ.get("GH_TOKEN") or os.environ.get("GITHUB_TOKEN")
        if not token:
            print("error: set GH_TOKEN (or GITHUB_TOKEN) for live GitHub access", file=sys.stderr)
            return 2
        print(f"fetching pulls from {args.repo}…", file=sys.stderr)
        prs = fetch_github_prs(args.repo, token, state=args.state)
        print(f"fetched {len(prs)} pull request(s)", file=sys.stderr)

    by_key = group_prs_by_issue(prs, default_repo)
    print(f"matched {len(by_key)} issue key(s)", file=sys.stderr)

    if args.dry_run:
        upsert_prs(None, by_key, dry_run=True)
        return 0

    db_path = Path(args.db) if args.db else default_db_path(args.profile)
    if not db_path.exists():
        print(f"error: database not found: {db_path}", file=sys.stderr)
        return 1
    conn = open_db(db_path)
    try:
        n = upsert_prs(conn, by_key, dry_run=False)
    finally:
        conn.close()
    print(f"upserted {n} enrichment row(s) into {db_path}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
