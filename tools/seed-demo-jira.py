#!/usr/bin/env python3
"""Seed a Jira Cloud site with a realistic demo backlog.

Used to produce the data behind scry's screenshots and `examples/demo.db`, and
reusable by anyone who wants a throwaway Jira to point scry at.

It creates releases, components, issues, status history, comments, and issue
links across three fake products. Every value is fictional.

Usage:
    export JIRA_SITE=https://your-site.atlassian.net
    export JIRA_EMAIL=you@example.com
    export JIRA_TOKEN=<api token from id.atlassian.com>
    python3 tools/seed-demo-jira.py --projects NMB,NMA,NMS [--issues 300] [--dry-run]

Requires the projects to exist already and to be company-managed (team-managed
projects do not expose priority, components, or fix versions).

Timestamps: Jira assigns `created` itself, so every seeded issue is created
"now". Status history is real (transitions are performed), but the time between
transitions is not. Snapshot generation is where realistic time spread gets
applied.
"""

from __future__ import annotations

import argparse
import base64
import json
import os
import random
import sys
import time
import urllib.error
import urllib.request

# ── deterministic content pools ───────────────────────────────────────────────

SITE = os.environ.get("JIRA_SITE", "").rstrip("/")
EMAIL = os.environ.get("JIRA_EMAIL", "")
TOKEN = os.environ.get("JIRA_TOKEN", "")

PROJECT_PROFILES = {
    "NMB": {
        "components": ["Dashboard", "Editor", "Billing", "Auth", "Notifications"],
        "versions": ["2026.6.0", "2026.7.0", "2026.8.0", "2026.9.0"],
        "type_weights": {"Bug": 45, "Story": 30, "Task": 25},
        "areas": [
            "dashboard filter chips", "editor autosave", "billing plan switcher",
            "SSO callback", "notification digest", "keyboard shortcuts",
            "dark mode contrast", "workspace switcher", "CSV export",
            "onboarding checklist", "saved view sharing", "column resizing",
            "inline comment editor", "avatar upload", "seat management table",
            "invoice download", "trial banner", "member invite flow",
            "activity sidebar", "global search box", "bulk selection toolbar",
            "drag-and-drop reordering", "print stylesheet", "session timeout dialog",
            "empty state illustrations", "breadcrumb navigation", "tag autocomplete",
            "date range picker", "attachment preview", "undo toast",
        ],
    },
    "NMA": {
        "components": ["REST API", "Webhooks", "Workers", "SDK", "Rate Limiting"],
        "versions": ["v2.3", "v2.4", "v2.5"],
        "type_weights": {"Bug": 40, "Story": 25, "Task": 35},
        "areas": [
            "pagination cursor",
            "webhook retry backoff",
            "idempotency key handling",
            "bulk import worker",
            "rate limit headers",
            "OpenAPI schema",
            "token refresh",
            "audit log ingestion",
            "search endpoint",
            "batch delete",
        ],
    },
    "NMS": {
        "components": ["Triage", "Escalation", "Billing Questions"],
        "versions": ["Sprint 41", "Sprint 42", "Sprint 43"],
        "type_weights": {"Bug": 70, "Task": 30},
        "areas": [
            "customer cannot log in",
            "invoice shows wrong currency",
            "export never finishes",
            "duplicate notification emails",
            "workspace invite bounces",
            "attachment preview blank",
            "timezone off by one day",
            "search returns nothing",
            "mobile layout broken",
            "seat count mismatch",
        ],
    },
}

BUG_PATTERNS = [
    "{area} silently fails on first load",
    "{area} throws 500 when the workspace has no members",
    "{area} loses state after a browser refresh",
    "{area} ignores the selected timezone",
    "{area} double-fires on slow connections",
    "{area} renders stale data after an update",
    "{area} breaks for accounts with more than 1000 records",
    "{area} returns results in a nondeterministic order",
]
STORY_PATTERNS = [
    "Let users pin {area} to the sidebar",
    "Add keyboard-only navigation to {area}",
    "Show inline validation errors in {area}",
    "Support bulk actions in {area}",
    "Remember the last used {area} per workspace",
]
TASK_PATTERNS = [
    "Add integration tests for {area}",
    "Instrument {area} with structured logs",
    "Document {area} in the public reference",
    "Remove the legacy code path behind {area}",
    "Backfill missing rows for {area}",
]
PATTERNS = {"Bug": BUG_PATTERNS, "Story": STORY_PATTERNS, "Task": TASK_PATTERNS}

LABEL_POOL = [
    "regression", "customer-reported", "needs-repro", "quick-win", "flaky",
    "performance", "accessibility", "security", "tech-debt", "docs",
]
ENVIRONMENTS = [
    "Chrome 140 / macOS 15",
    "Safari 18 / iOS 18",
    "Firefox 138 / Windows 11",
    "API v2.4 / staging",
    "Production, EU region",
]
PRIORITY_WEIGHTS = {"Highest": 6, "High": 18, "Medium": 46, "Low": 22, "Lowest": 8}

COMMENT_POOL = [
    "Reproduced on staging with a fresh workspace. Attaching the request id.",
    "This started after the pagination change landed last week.",
    "Not reproducible with a single member — needs at least three.",
    "Workaround for now: reload the page after switching workspaces.",
    "Root cause is the cache key missing the workspace id.",
    "Moving to review, the fix is behind a flag.",
    "Customer confirmed the issue is gone after the deploy.",
    "Reopening — the same trace showed up again this morning.",
    "Deferring: the affected code path is being replaced next quarter.",
    "Added a regression test so this cannot come back silently.",
]

LINK_TYPES = ["Relates", "Blocks", "Duplicate"]


# ── http plumbing ─────────────────────────────────────────────────────────────


def auth_header() -> str:
    raw = f"{EMAIL}:{TOKEN}".encode()
    return "Basic " + base64.b64encode(raw).decode()


def call(method: str, path: str, body: dict | None = None, tries: int = 5):
    """One Jira REST call with 429/5xx retry. Returns parsed JSON or None."""
    url = f"{SITE}{path}"
    data = json.dumps(body).encode() if body is not None else None
    for attempt in range(tries):
        req = urllib.request.Request(url, data=data, method=method)
        req.add_header("Authorization", auth_header())
        req.add_header("Accept", "application/json")
        # English field/status names regardless of the account's display language.
        req.add_header("Accept-Language", "en")
        if data is not None:
            req.add_header("Content-Type", "application/json")
        try:
            with urllib.request.urlopen(req, timeout=60) as res:
                payload = res.read()
                return json.loads(payload) if payload else {}
        except urllib.error.HTTPError as exc:
            detail = exc.read().decode(errors="replace")[:400]
            if exc.code in (429, 500, 502, 503, 504) and attempt < tries - 1:
                wait = min(2 ** attempt, 30)
                print(f"  retry {exc.code} in {wait}s: {method} {path}", file=sys.stderr)
                time.sleep(wait)
                continue
            print(f"  ERROR {exc.code} {method} {path}: {detail}", file=sys.stderr)
            return None
        except urllib.error.URLError as exc:
            if attempt < tries - 1:
                time.sleep(2 ** attempt)
                continue
            print(f"  ERROR {method} {path}: {exc}", file=sys.stderr)
            return None
    return None


def weighted(pool: dict[str, int]) -> str:
    keys = list(pool)
    return random.choices(keys, weights=[pool[k] for k in keys], k=1)[0]


def adf(paragraphs: list[str]) -> dict:
    """Minimal ADF document from plain paragraphs."""
    return {
        "type": "doc",
        "version": 1,
        "content": [
            {"type": "paragraph", "content": [{"type": "text", "text": p}]}
            for p in paragraphs
        ],
    }


def bug_description(area: str) -> dict:
    return adf([
        f"Steps to reproduce: open {area} with a workspace that has more than one member.",
        "Expected: the view keeps the selected filters after a reload.",
        "Actual: filters reset to the default and the row count changes.",
        "Impact: reported by two customers this week; no data loss observed.",
    ])


def plain_description(summary: str) -> dict:
    return adf([
        f"Context: {summary}.",
        "Acceptance: behaviour is covered by a test and documented in the reference.",
    ])


# ── seeding steps ─────────────────────────────────────────────────────────────


def ensure_versions(project: str, names: list[str], dry: bool) -> list[str]:
    """Create fix versions, marking all but the last two as released."""
    made = []
    for index, name in enumerate(names):
        released = index < len(names) - 2
        if dry:
            made.append(name)
            continue
        res = call("POST", "/rest/api/3/version", {
            "name": name,
            "project": project,
            "released": released,
            "description": f"{project} release {name}",
        })
        if res:
            made.append(name)
    print(f"  versions: {made}")
    return made


def ensure_components(project: str, names: list[str], dry: bool) -> list[str]:
    made = []
    for name in names:
        if dry:
            made.append(name)
            continue
        res = call("POST", "/rest/api/3/component", {"name": name, "project": project})
        if res:
            made.append(name)
    print(f"  components: {made}")
    return made


WANTED_TYPES = ("Bug", "Story", "Task")


def issue_type_ids(project: str) -> dict[str, str]:
    """Map canonical English issue-type names to ids.

    `issue/createmeta` translates type names into the caller's account language
    (and ignores Accept-Language), which breaks name matching on non-English
    accounts. `project/{key}/statuses` returns the untranslated names, so use it
    as the source of truth and keep only the types we seed.
    """
    res = call("GET", f"/rest/api/3/project/{project}/statuses")
    out = {}
    for it in res or []:
        if not it.get("subtask") and it.get("name") in WANTED_TYPES:
            out[it["name"]] = it["id"]
    return out


def build_issues(project: str, profile: dict, count: int, types: dict[str, str],
                 components: list[str], versions: list[str], me: str) -> list[dict]:
    """Assemble createmeta-valid field payloads."""
    issues = []
    for _ in range(count):
        kind = weighted(profile["type_weights"])
        if kind not in types:
            # The project does not offer this type — fall back to any available one.
            kind = sorted(types)[0]
        area = random.choice(profile["areas"])
        summary = random.choice(PATTERNS.get(kind, TASK_PATTERNS)).format(area=area)
        fields: dict = {
            "project": {"key": project},
            "issuetype": {"id": types[kind]},
            "summary": summary[:250],
            "priority": {"name": weighted(PRIORITY_WEIGHTS)},
            "description": bug_description(area) if kind == "Bug" else plain_description(summary),
        }
        if random.random() < 0.7 and components:
            picks = random.sample(components, k=random.randint(1, min(2, len(components))))
            fields["components"] = [{"name": c} for c in picks]
        if random.random() < 0.55 and versions:
            fields["fixVersions"] = [{"name": random.choice(versions)}]
        if random.random() < 0.6:
            fields["labels"] = random.sample(LABEL_POOL, k=random.randint(1, 3))
        if kind == "Bug" and random.random() < 0.7:
            fields["environment"] = adf([random.choice(ENVIRONMENTS)])
        # Leave a realistic share unassigned; the rest go to the seeding account.
        if random.random() < 0.45:
            fields["assignee"] = {"id": me}
        issues.append({"fields": fields})
    return issues


def create_issues(payloads: list[dict], dry: bool) -> list[dict]:
    """Bulk create in batches of 50. Returns [{id, key}]."""
    created = []
    for start in range(0, len(payloads), 50):
        batch = payloads[start:start + 50]
        if dry:
            created.extend({"id": "0", "key": f"DRY-{start + i}"} for i in range(len(batch)))
            continue
        res = call("POST", "/rest/api/3/issue/bulk", {"issueUpdates": batch})
        if res:
            created.extend(res.get("issues", []))
            for err in res.get("errors", [])[:3]:
                print(f"  create error: {json.dumps(err)[:300]}", file=sys.stderr)
        print(f"  created {len(created)}/{len(payloads)}")
    return created


def walk_workflow(issue_key: str) -> int:
    """Move an issue a random distance along its workflow.

    A minority get pushed to a done status and then back to a todo status, which
    is what a reopen looks like in the changelog: a done -> not-done transition.
    """
    moves = 0
    steps = random.choices([0, 1, 2, 3], weights=[25, 30, 30, 15], k=1)[0]
    for _ in range(steps):
        res = call("GET", f"/rest/api/3/issue/{issue_key}/transitions")
        if not res or not res.get("transitions"):
            break
        forward = [t for t in res["transitions"]
                   if t["to"]["statusCategory"]["key"] != "new"] or res["transitions"]
        pick = random.choice(forward)
        if call("POST", f"/rest/api/3/issue/{issue_key}/transitions",
                {"transition": {"id": pick["id"]}}) is None:
            break
        moves += 1
    if moves and random.random() < 0.12:
        res = call("GET", f"/rest/api/3/issue/{issue_key}/transitions")
        back = [t for t in (res or {}).get("transitions", [])
                if t["to"]["statusCategory"]["key"] == "new"]
        if back and call("POST", f"/rest/api/3/issue/{issue_key}/transitions",
                         {"transition": {"id": back[0]["id"]}}) is not None:
            moves += 1
    return moves


def project_status_ids(project: str) -> dict[str, str]:
    """Map dataset state names to concrete status ids for a project's workflow.

    Status names cannot be used for this. `project/{key}/statuses` leaves
    workflow-local statuses untranslated ("Backlog") but returns Jira's *global*
    statuses in the account's display language, so "In Progress" arrives as
    "진행 중" on a Korean account. Resolution therefore goes through
    `statusCategory`, which is stable everywhere:

        new           -> backlog, and the second such status is "selected"
        indeterminate -> inprogress
        done          -> done

    The list is in workflow order, so "first new status" is the initial state.
    """
    res = call("GET", f"/rest/api/3/project/{project}/statuses")
    ordered: list[dict] = []
    for it in res or []:
        for status in it.get("statuses", []):
            if not any(s["id"] == status["id"] for s in ordered):
                ordered.append({
                    "id": status["id"],
                    "category": status["statusCategory"]["key"],
                })
    todo = [s["id"] for s in ordered if s["category"] == "new"]
    progress = [s["id"] for s in ordered if s["category"] == "indeterminate"]
    done = [s["id"] for s in ordered if s["category"] == "done"]

    out: dict[str, str] = {}
    if todo:
        out["backlog"] = todo[0]
        out["selected"] = todo[1] if len(todo) > 1 else todo[0]
    if progress:
        out["inprogress"] = progress[0]
    if done:
        out["done"] = done[0]
    return out


CATEGORY_LADDER = ["new", "indeterminate", "done"]


def transition_to(issue_key: str, target_id: str, target_category: str | None = None,
                  hops: int = 5) -> bool:
    """Walk the workflow to `target_id`, one category rung at a time.

    Deliberately avoids a direct edge that skips rungs. Default Jira workflows
    offer Backlog -> Done, and taking it leaves a changelog with a single entry —
    which makes derived fields technically correct but the history timeline
    useless, and gives a demo nothing to show. Stepping through the intermediate
    categories produces the multi-entry history a real issue accumulates.
    """
    if target_category is None:
        target_category = "done"
    want_rung = CATEGORY_LADDER.index(target_category)

    for _ in range(hops):
        res = call("GET", f"/rest/api/3/issue/{issue_key}/transitions")
        options = (res or {}).get("transitions", [])
        if not options:
            return False

        # Where are we now? Any transition tells us via its own source omission,
        # so ask the issue instead.
        current = call("GET", f"/rest/api/3/issue/{issue_key}?fields=status")
        if not current:
            return False
        status = current["fields"]["status"]
        if status["id"] == target_id:
            return True
        here_rung = CATEGORY_LADDER.index(status["statusCategory"]["key"])

        if here_rung == want_rung:
            # Right category, wrong status (e.g. Backlog vs Selected).
            same = next((t for t in options if t["to"]["id"] == target_id), None)
            if not same:
                return False
            return call("POST", f"/rest/api/3/issue/{issue_key}/transitions",
                        {"transition": {"id": same["id"]}}) is not None

        step = 1 if want_rung > here_rung else -1
        next_category = CATEGORY_LADDER[here_rung + step]
        candidates = [t for t in options
                      if t["to"]["statusCategory"]["key"] == next_category]
        if not candidates:
            # No rung-by-rung path exists in this workflow; accept a direct jump
            # rather than leaving the issue in the wrong state.
            direct = next((t for t in options if t["to"]["id"] == target_id), None)
            if not direct:
                return False
            return call("POST", f"/rest/api/3/issue/{issue_key}/transitions",
                        {"transition": {"id": direct["id"]}}) is not None
        # Prefer the target itself when it happens to be on the next rung.
        pick = next((t for t in candidates if t["to"]["id"] == target_id), candidates[0])
        if call("POST", f"/rest/api/3/issue/{issue_key}/transitions",
                {"transition": {"id": pick["id"]}}) is None:
            return False
    return False


def apply_states(keys: list[str], order: list[dict], per_project: dict) -> tuple[int, int]:
    """Drive each issue to its dataset `state`, producing real changelog history.

    A `reopened` issue is pushed to done and then back to its target state, which
    is what makes the changelog contain a genuine done -> not-done transition.
    """
    state_category = {
        "backlog": "new", "selected": "new",
        "inprogress": "indeterminate", "done": "done",
    }
    moved = reopened = 0
    for key, item in zip(keys, order):
        statuses = per_project[item["project"]]["statuses"]
        state = item.get("state") or "backlog"
        target = statuses.get(state)
        if item.get("reopened"):
            done_id = statuses.get("done")
            if done_id and transition_to(key, done_id, "done"):
                moved += 1
            back = target if state != "done" else statuses.get("backlog")
            if back and transition_to(key, back, state_category.get(state, "new")):
                reopened += 1
        elif target and state != "backlog":
            if transition_to(key, target, state_category.get(state)):
                moved += 1
    return moved, reopened


def repair_states(path: str, projects: list[str]) -> int:
    """Re-drive states for issues already present in Jira.

    Needed after a seeding run whose status resolution was wrong: the issues,
    comments, and links exist, but their workflow position does not match the
    dataset. Matching is by summary, which the dataset guarantees is unique.
    """
    with open(path, encoding="utf-8") as fh:
        data = json.load(fh)
    wanted = {i["summary"]: i for i in data["issues"] if i["project"] in projects}

    per_project = {p: {"statuses": project_status_ids(p)} for p in projects}
    keys, order = [], []
    for project in projects:
        token, page = None, 0
        while True:
            path_q = (f"/rest/api/3/search/jql?jql=project%3D{project}"
                      f"&maxResults=100&fields=summary,status")
            if token:
                path_q += f"&nextPageToken={token}"
            res = call("GET", path_q)
            if not res:
                break
            for issue in res.get("issues", []):
                item = wanted.get(issue["fields"]["summary"])
                if item:
                    keys.append(issue["key"])
                    order.append(item)
            token = res.get("nextPageToken")
            page += 1
            if not token or page > 40:
                break
    print(f"matched {len(keys)} issues to the dataset")
    moved, reopened = apply_states(keys, order, per_project)
    print(f"transitions: {moved}, reopened: {reopened}")
    return len(keys)


def iter_site_issues(projects: list[str], fields: str):
    """Page every issue in scope, yielding raw issue objects."""
    for project in projects:
        token, page = None, 0
        while True:
            path_q = (f"/rest/api/3/search/jql?jql=project%3D{project}"
                      f"&maxResults=100&fields={fields}")
            if token:
                path_q += f"&nextPageToken={token}"
            res = call("GET", path_q)
            if not res:
                break
            yield from res.get("issues", [])
            token = res.get("nextPageToken")
            page += 1
            if not token or res.get("isLast") or page > 40:
                break


def repair_assignees(path: str, projects: list[str], assignees: list[str]) -> int:
    """Redistribute assignees across the given accounts.

    Dataset issues follow their `assignee_slot` (null means unassigned). Issues
    that are not in the dataset keep their assigned/unassigned status but get
    spread across the account pool, so avatars vary without changing the ratio.
    Distribution is by key hash, which makes repeated runs a no-op.
    """
    with open(path, encoding="utf-8") as fh:
        data = json.load(fh)
    slots = {i["summary"]: i.get("assignee_slot") for i in data["issues"]}

    changed = skipped = 0
    for issue in iter_site_issues(projects, "summary,assignee"):
        key = issue["key"]
        summary = issue["fields"]["summary"]
        current = (issue["fields"].get("assignee") or {}).get("accountId")

        if summary in slots:
            slot = slots[summary]
            target = None if slot is None else assignees[slot % len(assignees)]
        elif current:
            # Not from the dataset but assigned: spread deterministically.
            target = assignees[sum(ord(c) for c in key) % len(assignees)]
        else:
            target = None

        if target == current:
            skipped += 1
            continue
        if call("PUT", f"/rest/api/3/issue/{key}/assignee",
                {"accountId": target}) is not None:
            changed += 1
    print(f"assignees changed: {changed}, already correct: {skipped}")
    return changed


def seed_from_data(path: str, projects: list[str], assignees: list[str],
                   dry: bool, skip_setup: bool) -> list[str]:
    """Create issues from a pre-generated dataset (see tools/README for the shape).

    Content generation is deliberately separate from API plumbing: the dataset is
    authored once (by hand or by a model) and this function only projects it onto
    Jira, so re-running against a different site produces the same backlog.
    """
    with open(path, encoding="utf-8") as fh:
        data = json.load(fh)
    items = [i for i in data["issues"] if i["project"] in projects]
    print(f"dataset: {len(items)} issues for {', '.join(projects)}")

    per_project = {}
    for project in projects:
        profile = PROJECT_PROFILES.get(project, {})
        if not skip_setup and not dry:
            ensure_versions(project, profile.get("versions", []), dry)
            ensure_components(project, profile.get("components", []), dry)
        per_project[project] = {
            "types": issue_type_ids(project),
            "statuses": project_status_ids(project),
        }
        print(f"  {project}: types={sorted(per_project[project]['types'])} "
              f"statuses={sorted(per_project[project]['statuses'])}")

    # Build payloads in dataset order so link indexes stay meaningful.
    payloads, order = [], []
    for index, item in enumerate(items):
        meta = per_project[item["project"]]
        type_id = meta["types"].get(item["type"])
        if not type_id:
            print(f"  skip #{index}: type {item['type']} unavailable in {item['project']}",
                  file=sys.stderr)
            continue
        fields: dict = {
            "project": {"key": item["project"]},
            "issuetype": {"id": type_id},
            "summary": item["summary"][:250],
            "description": adf(item.get("description") or [item["summary"]]),
        }
        if item.get("priority"):
            fields["priority"] = {"name": item["priority"]}
        if item.get("components"):
            fields["components"] = [{"name": c} for c in item["components"]]
        if item.get("fix_version"):
            fields["fixVersions"] = [{"name": item["fix_version"]}]
        if item.get("labels"):
            fields["labels"] = item["labels"]
        if item.get("environment"):
            fields["environment"] = adf([item["environment"]])
        slot = item.get("assignee_slot")
        if slot is not None and assignees:
            fields["assignee"] = {"id": assignees[slot % len(assignees)]}
        payloads.append({"fields": fields})
        order.append(item)

    created = create_issues(payloads, dry)
    if dry:
        return [c["key"] for c in created]

    keys = [c["key"] for c in created]
    if len(keys) != len(order):
        print(f"  WARNING: created {len(keys)} of {len(order)} — links may shift",
              file=sys.stderr)

    # Status, then comments, then links: comments land after the transitions that
    # they narrate, and links need every key to exist.
    moved, reopened = apply_states(keys, order, per_project)
    print(f"  transitions: {moved}, reopened: {reopened}")

    commented = 0
    for key, item in zip(keys, order):
        for text in item.get("comments") or []:
            if call("POST", f"/rest/api/3/issue/{key}/comment", {"body": adf([text])}) is not None:
                commented += 1
    print(f"  comments: {commented}")

    linked = 0
    for key, item in zip(keys, order):
        for link in item.get("links") or []:
            target = link.get("target")
            if not isinstance(target, int) or not (0 <= target < len(keys)):
                continue
            if keys[target] == key:
                continue
            if call("POST", "/rest/api/3/issueLink", {
                "type": {"name": link.get("type", "Relates")},
                "inwardIssue": {"key": key},
                "outwardIssue": {"key": keys[target]},
            }) is not None:
                linked += 1
    print(f"  links: {linked}")
    return keys


def add_comments(issue_key: str) -> int:
    count = random.choices([0, 1, 2, 3], weights=[45, 30, 17, 8], k=1)[0]
    for text in random.sample(COMMENT_POOL, k=min(count, len(COMMENT_POOL))):
        call("POST", f"/rest/api/3/issue/{issue_key}/comment", {"body": adf([text])})
    return count


def link_issues(keys: list[str], pairs: int) -> int:
    made = 0
    for _ in range(pairs):
        a, b = random.sample(keys, 2)
        ok = call("POST", "/rest/api/3/issueLink", {
            "type": {"name": random.choice(LINK_TYPES)},
            "inwardIssue": {"key": a},
            "outwardIssue": {"key": b},
        })
        if ok is not None:
            made += 1
    return made


# ── entry point ───────────────────────────────────────────────────────────────


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--projects", default="NMB,NMA,NMS")
    ap.add_argument("--issues", type=int, default=300, help="total across all projects")
    ap.add_argument("--seed", type=int, default=20260804)
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("--skip-setup", action="store_true",
                    help="do not create versions/components (they already exist)")
    ap.add_argument("--no-history", action="store_true",
                    help="create issues only; skip transitions, comments, and links")
    ap.add_argument("--data",
                    help="JSON dataset to project onto Jira instead of generating content")
    ap.add_argument("--assignees",
                    help="comma-separated accountIds for assignee slots (default: the caller)")
    ap.add_argument("--repair-states", action="store_true",
                    help="re-drive workflow states for issues already in Jira, matched by summary")
    ap.add_argument("--repair-assignees", action="store_true",
                    help="redistribute assignees across --assignees for issues already in Jira")
    args = ap.parse_args()

    if not (SITE and EMAIL and TOKEN):
        print("JIRA_SITE, JIRA_EMAIL, and JIRA_TOKEN must be set", file=sys.stderr)
        return 2

    random.seed(args.seed)
    projects = [p.strip().upper() for p in args.projects.split(",") if p.strip()]

    me = call("GET", "/rest/api/3/myself")
    if not me:
        print("authentication failed", file=sys.stderr)
        return 1
    account_id = me["accountId"]
    print(f"authenticated as {me.get('displayName')} <{me.get('emailAddress')}>")

    if args.repair_states or args.repair_assignees:
        if not args.data:
            print("--repair-* requires --data", file=sys.stderr)
            return 2
        if args.repair_states:
            repair_states(args.data, projects)
        if args.repair_assignees:
            pool = ([a.strip() for a in args.assignees.split(",") if a.strip()]
                    if args.assignees else [account_id])
            print(f"assignee pool: {len(pool)} account(s)")
            repair_assignees(args.data, projects, pool)
        return 0

    if args.data:
        assignees = ([a.strip() for a in args.assignees.split(",") if a.strip()]
                     if args.assignees else [account_id])
        keys = seed_from_data(args.data, projects, assignees, args.dry_run, args.skip_setup)
        print(f"\ndone. {len(keys)} issues from {args.data}")
        return 0

    share = max(1, args.issues // len(projects))
    all_keys: list[str] = []

    for project in projects:
        profile = PROJECT_PROFILES.get(project)
        if not profile:
            print(f"no profile for {project}, skipping", file=sys.stderr)
            continue
        print(f"\n[{project}]")
        if args.skip_setup:
            versions, components = profile["versions"], profile["components"]
        else:
            versions = ensure_versions(project, profile["versions"], args.dry_run)
            components = ensure_components(project, profile["components"], args.dry_run)

        types = {"Bug": "1", "Story": "2", "Task": "3"} if args.dry_run else issue_type_ids(project)
        if not types:
            print(f"  could not read issue types for {project}", file=sys.stderr)
            continue
        print(f"  issue types: {sorted(types)}")

        payloads = build_issues(project, profile, share, types, components, versions, account_id)
        created = create_issues(payloads, args.dry_run)
        keys = [c["key"] for c in created]
        all_keys.extend(keys)

        if args.dry_run or args.no_history:
            continue

        moved = sum(walk_workflow(k) for k in keys)
        commented = sum(add_comments(k) for k in keys)
        print(f"  transitions: {moved}, comments: {commented}")

    if not args.dry_run and not args.no_history and len(all_keys) > 4:
        links = link_issues(all_keys, pairs=max(4, len(all_keys) // 12))
        print(f"\nlinks: {links}")

    print(f"\ndone. {len(all_keys)} issues across {', '.join(projects)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
