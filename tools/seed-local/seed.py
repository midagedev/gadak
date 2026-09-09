#!/usr/bin/env python3
"""Seed examples/local.db with a demo browsing history (GDK-1720).

`gadak retro` answers "what did I look at, and what moved after" from
local.db visits — sessions, resume, seen-vs-touched. local.db is personal
state, so the committed fixture carried an empty one and e2e/serve.sh deleted
whatever a previous run had left. Those rows were therefore empty in the
demo, in every recording and in every e2e run: the one surface built on them
had nothing to show and no test could tell a working retro from a broken one.

This writes a plausible history instead of leaving a blank one. It is
deterministic (seeded PRNG, no clock) and it reads the mirror it accompanies,
so every key it names exists and the reads sit inside the mirror's own window.

Usage: seed.py <mirror.db> <local.db>   (local.db is rewritten in place;
its schema and user_version are the committed file's, untouched.)
"""

import random
import sqlite3
import sys
from datetime import datetime, timedelta

# Sessions split on a gap larger than this (internal/retro: the default is
# 30 minutes), so reads inside a session stay well under it and the sessions
# themselves are hours or days apart.
SESSION_GAP_MIN = 30
WINDOW_DAYS = 28
SESSIONS = 16
# The demo mirror's own user (examples/demo.db comments.author_id), which is
# also the account e2e/serve.sh configures.
SELF_ACCOUNT = "demo-dana"

ISO = "%Y-%m-%dT%H:%M:%S.%f"


def stamp(t: datetime) -> str:
    return t.strftime(ISO)[:-3] + "Z"


def parse(s: str) -> datetime:
    return datetime.strptime(s.replace("Z", ""), ISO)


def main() -> int:
    if len(sys.argv) != 3:
        print(__doc__, file=sys.stderr)
        return 2
    mirror_path, local_path = sys.argv[1], sys.argv[2]

    mirror = sqlite3.connect(f"file:{mirror_path}?mode=ro", uri=True)
    # The mirror's own "now": the snapshot spreads created_at up to it.
    anchor = parse(mirror.execute("SELECT MAX(created_at) FROM issues_raw").fetchone()[0])
    start = anchor - timedelta(days=WINDOW_DAYS)

    issue_keys = [r[0] for r in mirror.execute(
        "SELECT key FROM issues_raw WHERE key != '' ORDER BY key")]
    page_keys = [r[0] for r in mirror.execute(
        "SELECT COALESCE(key,'') FROM items WHERE kind = 'page' AND COALESCE(key,'') != '' ORDER BY key")]
    if not issue_keys:
        print("seed-local: mirror has no issues", file=sys.stderr)
        return 1

    # (when, key, author_id) for every comment in the window. The retro's
    # resume row is "how long from opening the app to your first write", and
    # the rule keys on the actor — so a session has to sit in front of a write
    # by the configured account for that row to have anything to say. In this
    # fixture the changelog carries no author id and the comments do, so the
    # comments are the anchors.
    activity = []
    for at, key, author in mirror.execute("""
            SELECT c.created_at, i.key, COALESCE(c.author_id,'')
            FROM comments c JOIN issues_raw i ON i.item_id = c.item_id
            WHERE c.created_at >= ? AND c.created_at <= ?
            ORDER BY c.created_at, i.key""", (stamp(start), stamp(anchor))):
        activity.append((parse(at), key, author))
    mirror.close()

    rnd = random.Random(20260909)

    # Half the sessions are placed just before a write by the demo's own user,
    # so resume resolves; the rest are reading-only sessions on working days.
    mine = [(at, key) for (at, key, author) in activity if author == SELF_ACCOUNT]
    anchors = mine[:: max(1, len(mine) // (SESSIONS // 2))][: SESSIONS // 2]
    seeded = [(at - timedelta(minutes=rnd.randint(15, 110)), key) for (at, key) in anchors]
    while len(seeded) < SESSIONS:
        base = start + timedelta(days=rnd.randrange(WINDOW_DAYS))
        seeded.append((base.replace(hour=rnd.randint(9, 18), minute=rnd.randint(0, 59),
                                    second=rnd.randint(0, 59), microsecond=0), None))
    seeded.sort(key=lambda p: p[0])

    rows = []
    sessions = 0
    last_read = None
    for s, anchor_key in seeded:
        # A session is only a session when the gap in front of it is bigger
        # than the split; anything closer would merge into the one before.
        if last_read is not None and s - last_read <= timedelta(minutes=SESSION_GAP_MIN):
            continue
        picks = []
        if anchor_key:
            picks.append(anchor_key)
        for (at, key, _) in activity:
            if len(picks) >= 2:
                break
            if s < at <= s + timedelta(hours=6) and key not in picks:
                picks.append(key)
        want = rnd.randint(3, 8)
        while len(picks) < want:
            k = rnd.choice(issue_keys)
            if k not in picks:
                picks.append(k)
        # One session in four also opens a wiki page.
        if page_keys and rnd.random() < 0.25:
            picks.insert(rnd.randrange(len(picks) + 1), page_keys[rnd.randrange(len(page_keys))])

        at = s
        for i, key in enumerate(picks):
            if i:
                at = at + timedelta(minutes=rnd.randint(2, SESSION_GAP_MIN - 8),
                                    seconds=rnd.randint(0, 59))
            kind = "page" if key in page_keys else "issue"
            # Most reads are the window; a few are `gadak issue` in a terminal.
            source = "cli" if rnd.random() < 0.15 else "ui"
            rows.append((kind, key, stamp(at), 0, source))
        last_read = at
        sessions += 1

    local = sqlite3.connect(local_path)
    local.execute("DELETE FROM visits")
    local.executemany(
        "INSERT INTO visits (kind, key, viewed_at, origin_epoch, source) VALUES (?,?,?,?,?)",
        rows)
    local.commit()
    n = local.execute("SELECT COUNT(*) FROM visits").fetchone()[0]
    first, last = local.execute("SELECT MIN(viewed_at), MAX(viewed_at) FROM visits").fetchone()
    local.execute("VACUUM")
    local.close()
    print(f"seed-local: {n} visits across {sessions} sessions, {first} … {last}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
