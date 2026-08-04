#!/usr/bin/env python3
"""Infer deploy stage from git tags that contain commits mentioning issue keys.

For each commit message matching PROJECT-123, discover which tags contain that
commit (git tag --contains) and map tag names to channels:

  *-prod, v* (semver-like)  → prod
  *-staging, *-qa           → qa
  *-dev                     → dev

Writes kind='deploy' enrichments with the wrapped {status, detail} payload the
server merges into list badges and the detail timeline (see docs/PLUGINS.md).
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sqlite3
import subprocess
import sys
import tempfile
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

SOURCE = "deploy-status"
ISSUE_KEY_RE = re.compile(r"\b([A-Z][A-Z0-9]+-\d+)\b")

# Tag → channel heuristics. First match wins (order matters).
TAG_RULES: list[tuple[re.Pattern[str], str]] = [
    (re.compile(r"(^|[-/.])prod($|[-/.])", re.I), "prod"),
    (re.compile(r"(^|[-/.])staging($|[-/.])", re.I), "qa"),
    (re.compile(r"(^|[-/.])qa($|[-/.])", re.I), "qa"),
    (re.compile(r"(^|[-/.])dev($|[-/.])", re.I), "dev"),
    # bare semver / v1.2.3 style → prod (common release tag)
    (re.compile(r"^v?\d+\.\d+(\.\d+)?([.-].*)?$", re.I), "prod"),
]

CHANNEL_RANK = {"dev": 2, "qa": 4, "prod": 5}  # DeployState ranks (types.ts)
STATE_FOR_CHANNEL = {"dev": "dev", "qa": "qa", "prod": "prod"}


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


def classify_tag(tag: str) -> str | None:
    for pattern, channel in TAG_RULES:
        if pattern.search(tag):
            return channel
    return None


def run_git(repo: Path, *args: str) -> str:
    try:
        out = subprocess.check_output(
            ["git", "-C", str(repo), *args],
            stderr=subprocess.STDOUT,
            text=True,
        )
    except subprocess.CalledProcessError as e:
        raise SystemExit(f"git {' '.join(args)} failed:\n{e.output}") from e
    except FileNotFoundError as e:
        raise SystemExit("git not found on PATH") from e
    return out


def list_tags(repo: Path, patterns: list[str]) -> list[str]:
    """Return tag names matching any of the fnmatch-style patterns (via git)."""
    tags: set[str] = set()
    if not patterns:
        raw = run_git(repo, "tag", "-l")
        tags.update(t for t in raw.splitlines() if t.strip())
    else:
        for pat in patterns:
            raw = run_git(repo, "tag", "-l", pat)
            tags.update(t for t in raw.splitlines() if t.strip())
    return sorted(tags)


def tag_timestamp(repo: Path, tag: str) -> str | None:
    """Best-effort ISO time for a tag (tagger or committer date)."""
    raw = run_git(
        repo,
        "for-each-ref",
        f"refs/tags/{tag}",
        "--format=%(creatordate:iso-strict)",
    ).strip()
    if not raw:
        return None
    # normalize space-offset "2026-01-01T00:00:00+00:00" style already iso-strict
    return raw.replace(" ", "T") if "T" not in raw else raw


def commits_with_keys(repo: Path) -> list[tuple[str, set[str]]]:
    """(sha, issue_keys) for every commit whose subject/body mentions a key."""
    # %H sha, %s subject, %b body — use a rare separator
    raw = run_git(repo, "log", "--all", "--format=%H%x1f%s%x1f%b%x1e")
    results: list[tuple[str, set[str]]] = []
    for record in raw.split("\x1e"):
        record = record.strip("\n")
        if not record:
            continue
        parts = record.split("\x1f", 2)
        if len(parts) < 2:
            continue
        sha = parts[0].strip()
        text = " ".join(parts[1:])
        keys = set(ISSUE_KEY_RE.findall(text))
        if keys:
            results.append((sha, keys))
    return results


def tags_containing(repo: Path, sha: str) -> list[str]:
    raw = run_git(repo, "tag", "--contains", sha)
    return [t.strip() for t in raw.splitlines() if t.strip()]


def build_deployments(
    repo: Path,
    tag_patterns: list[str],
) -> dict[str, dict[str, Any]]:
    """key → best-known deploy info from tag containment."""
    # Pre-index classified tags we care about.
    all_tags = list_tags(repo, tag_patterns)
    tag_channel: dict[str, str] = {}
    for tag in all_tags:
        ch = classify_tag(tag)
        if ch:
            tag_channel[tag] = ch
    if not tag_channel and all_tags:
        # fall back: classify every tag name even if pattern list filtered none
        for tag in all_tags:
            ch = classify_tag(tag)
            if ch:
                tag_channel[tag] = ch

    # Per issue: channel → (tag, at)
    per_issue: dict[str, dict[str, tuple[str, str | None]]] = {}
    # Also track "seen in any commit" for merged-only state
    seen: set[str] = set()

    for sha, keys in commits_with_keys(repo):
        for key in keys:
            seen.add(key)
        containing = tags_containing(repo, sha)
        for tag in containing:
            channel = tag_channel.get(tag) or classify_tag(tag)
            if not channel:
                continue
            at = tag_timestamp(repo, tag)
            for key in keys:
                slots = per_issue.setdefault(key, {})
                prev = slots.get(channel)
                # Keep the earliest tag for a channel (first release that included it).
                if prev is None:
                    slots[channel] = (tag, at)
                elif at and (not prev[1] or at < prev[1]):
                    slots[channel] = (tag, at)

    # Issues with commits but no classified tags still get "merged"
    for key in seen:
        per_issue.setdefault(key, {})

    out: dict[str, dict[str, Any]] = {}
    for key, channels in per_issue.items():
        out[key] = make_payload(channels)
    return out


def make_payload(channels: dict[str, tuple[str, str | None]]) -> dict[str, Any]:
    """Build wrapped deploy payload: {status, detail}."""
    best_channel: str | None = None
    best_rank = 0
    for ch in channels:
        r = CHANNEL_RANK.get(ch, 0)
        if r > best_rank:
            best_rank = r
            best_channel = ch

    if best_channel:
        state = STATE_FOR_CHANNEL[best_channel]
    elif channels is not None:
        state = "merged"
    else:
        state = "none"

    def ref(ch: str) -> dict[str, str] | None:
        if ch not in channels:
            return None
        tag, at = channels[ch]
        d: dict[str, str] = {"tag": tag}
        if at:
            d["at"] = at
        return d

    dev = ref("dev")
    qa_release = ref("qa")
    prod = ref("prod")
    prod_at = prod["at"] if prod and "at" in prod else None

    status = {
        "state": state,
        "merged_prs": 1 if state != "none" else 0,
        "total_prs": 1 if state != "none" else 0,
        "dev": dev,
        "qa_release": qa_release,
        "qa_swapped_at": qa_release["at"] if qa_release and "at" in qa_release else None,
        "prod_at": prod_at,
    }

    releases: list[dict[str, Any]] = []
    for ch, (tag, at) in sorted(channels.items(), key=lambda kv: CHANNEL_RANK.get(kv[0], 0)):
        entry: dict[str, Any] = {"tag": tag, "channel": ch}
        if at:
            entry["at"] = at
        releases.append(entry)

    detail = {
        **status,
        "releases": releases,
        "prs": [],
    }
    return {"status": status, "detail": detail}


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


def upsert_deploys(
    conn: sqlite3.Connection | None,
    by_key: dict[str, dict[str, Any]],
    *,
    dry_run: bool,
) -> int:
    if not by_key:
        return 0
    now = utc_now()
    if dry_run:
        for key, payload in sorted(by_key.items()):
            print(f"[dry-run] {key}\tdeploy\t{json.dumps(payload, ensure_ascii=False)}")
        return len(by_key)
    assert conn is not None
    with conn:
        for key, payload in by_key.items():
            body = json.dumps(payload, ensure_ascii=False, separators=(",", ":"))
            conn.execute(
                """
                INSERT INTO enrichments (key, kind, payload, source, updated_at)
                VALUES (?, 'deploy', ?, ?, ?)
                ON CONFLICT(key, kind) DO UPDATE SET
                  payload = excluded.payload,
                  source = excluded.source,
                  updated_at = excluded.updated_at
                """,
                (key, body, SOURCE, now),
            )
        conn.execute("UPDATE sync_state SET version = version + 1")
    return len(by_key)


def run_self_test() -> None:
    with tempfile.TemporaryDirectory() as td:
        repo = Path(td) / "repo"
        repo.mkdir()
        run_git(repo, "init", "-b", "main")
        run_git(repo, "config", "user.email", "dev@example.com")
        run_git(repo, "config", "user.name", "Dev Example")

        def commit(msg: str) -> None:
            # unique content so commits never collide
            (repo / "log.txt").write_text(msg + "\n", encoding="utf-8")
            run_git(repo, "add", "log.txt")
            run_git(repo, "commit", "-m", msg)

        commit("chore: scaffold")
        commit("feat: login flow NMB-1")
        run_git(repo, "tag", "v0.1.0-dev")
        commit("fix: session timeout NMB-1")
        run_git(repo, "tag", "1.0.0-staging")
        commit("feat: billing NMB-2")
        run_git(repo, "tag", "v1.0.0")  # prod-ish

        by_key = build_deployments(repo, ["v*", "*-staging", "*-dev", "*"])
        assert "NMB-1" in by_key, by_key
        assert "NMB-2" in by_key, by_key
        # NMB-1 is in dev + staging + v1.0.0 (prod) → prod
        assert by_key["NMB-1"]["status"]["state"] == "prod", by_key["NMB-1"]
        # NMB-2 only in v1.0.0 → prod
        assert by_key["NMB-2"]["status"]["state"] == "prod", by_key["NMB-2"]
        assert by_key["NMB-1"]["detail"]["releases"], by_key["NMB-1"]

        db = Path(td) / "test.db"
        conn = open_db(db)
        ensure_plugin_tables(conn)
        n = upsert_deploys(conn, by_key, dry_run=False)
        n2 = upsert_deploys(conn, by_key, dry_run=False)
        assert n == n2 == len(by_key)
        count = conn.execute("SELECT COUNT(*) FROM enrichments WHERE kind='deploy'").fetchone()[0]
        assert count == len(by_key)
        version = conn.execute("SELECT version FROM sync_state").fetchone()[0]
        assert version == 2, version
        conn.close()
    print("self-test ok")


def build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(
        prog="deploy_status.py",
        description="Write kind=deploy enrichments from git tag containment.",
    )
    p.add_argument(
        "repo",
        nargs="?",
        help="Path to a local git repository (required unless --self-test)",
    )
    p.add_argument(
        "--db",
        default=None,
        help="Path to scry.db (default: $SCRY_HOME/scry.db or ~/.scry/scry.db)",
    )
    p.add_argument("--profile", default=None, help="scry profile name (ignored if --db set)")
    p.add_argument(
        "--tags",
        default="v*,*-staging,*-prod,*-dev,*-qa",
        help="Comma-separated git tag patterns to consider (default: v*,*-staging,*-prod,*-dev,*-qa)",
    )
    p.add_argument("--dry-run", action="store_true", help="Print planned rows; do not write")
    p.add_argument("--self-test", action="store_true", help="Run built-in checks and exit")
    return p


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    if args.self_test:
        run_self_test()
        return 0

    if not args.repo:
        print("error: provide a local git repo path", file=sys.stderr)
        return 2
    repo = Path(args.repo).resolve()
    if not (repo / ".git").exists() and not (repo / "HEAD").exists():
        # bare repos have HEAD at root; normal repos have .git
        print(f"error: not a git repository: {repo}", file=sys.stderr)
        return 1

    patterns = [p.strip() for p in args.tags.split(",") if p.strip()]
    print(f"scanning {repo} (tags: {', '.join(patterns)})…", file=sys.stderr)
    by_key = build_deployments(repo, patterns)
    print(f"matched {len(by_key)} issue key(s)", file=sys.stderr)

    if args.dry_run:
        upsert_deploys(None, by_key, dry_run=True)
        return 0

    db_path = Path(args.db) if args.db else default_db_path(args.profile)
    if not db_path.exists():
        print(f"error: database not found: {db_path}", file=sys.stderr)
        return 1
    conn = open_db(db_path)
    try:
        n = upsert_deploys(conn, by_key, dry_run=False)
    finally:
        conn.close()
    print(f"upserted {n} enrichment row(s) into {db_path}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
