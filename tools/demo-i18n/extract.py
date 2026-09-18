#!/usr/bin/env python3
"""Extract every translatable string in the demo fixture as id → text (GDK-1556).

Output shape (examples/demo-i18n/en.json):
  {"version": 1, "source": "examples/demo.db", "strings": {"<id>": "<text>", ...}}

Ids are stable paths into the fixture, so a translation file keyed the same
way applies without any English-string matching:
  issue:<KEY>:title                    items.title of an issue
  issue:<KEY>:desc:<n>                 nth text node (document order) of issues_raw.description_adf
  issue:<KEY>:reopen_reason            issues_raw.reopen_reason (a column, not ADF)
  comment:<KEY>:<comment_id>:<n>       nth text node of comments.body_adf
  page:<item_key>:title                items.title of a wiki page
  page:<item_key>:body:<n>             nth text node of pages.body_adf
  catalog:status:<status_id>           issues_raw.status display name
  catalog:priority:<name>              issues_raw.priority display name (the fixture has no priority ids)
  catalog:type:<issue_type_id>         issues_raw.issue_type display name
  catalog:component:<name>             a components[] entry
  catalog:label:<name>                 a labels[] entry, on issues and pages
  catalog:resolution:<name>            issues_raw.resolution display name
  catalog:version:<name>               a fix_versions[] entry (and the versions row)
  catalog:board:<board_id>             boards.name
  issue:<KEY>:environment              issues_raw.environment_text
  catalog:sprint:<sprint_id>           sprints.name (and the issues_raw.sprint_name copy)
  catalog:linktype:<name>              links.type (and the link_types.name copy)
  changelog:link:<template>            a changelog link phrase, the issue key as {key} (GDK-1944)

Text nodes are the unit because ADF marks (links, code) split a sentence into
nodes; a translator sees the nodes of one document together, in order, so the
sentence is still readable. apply.py writes the translation back into the same
node and regenerates body_text / excerpt / FTS from the translated tree.
"""
import importlib.util, json, sqlite3, sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def load_check():
    """check.py owns the wire-fact detectors, key shape among them — one
    definition of "this token is an issue key", not a restated regex."""
    spec = importlib.util.spec_from_file_location("dcheck", Path(__file__).with_name("check.py"))
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


CHECK = load_check()

def text_nodes(adf: str):
    if not adf:
        return []
    out = []
    def walk(node):
        if isinstance(node, dict):
            if node.get("type") == "text" and isinstance(node.get("text"), str):
                out.append(node["text"])
            for c in node.get("content") or []:
                walk(c)
        elif isinstance(node, list):
            for c in node: walk(c)
    walk(json.loads(adf))
    return out

def extract(db: Path) -> dict:
    con = sqlite3.connect(f"file:{db}?mode=ro", uri=True)
    s = {}
    # Document order groups a document's nodes under adjacent ids.
    for key, title, desc in con.execute(
        "SELECT i.key, it.title, i.description_adf FROM issues_raw i JOIN items it ON it.id = i.item_id ORDER BY i.key"):
        s[f"issue:{key}:title"] = title or ""
        for n, t in enumerate(text_nodes(desc)):
            s[f"issue:{key}:desc:{n}"] = t
    # The reopen reason is a derived column, not a document — and the retro
    # prints it beside a translated title, so an untranslated one puts an
    # English sentence in the middle of a ko/ja report (GDK, 2026-09-13:
    # three vision judges called the ja take a blocker for exactly this).
    # A column is its own id family: every other issue: id ends in a node
    # index, and apply.py splits on that.
    for key, reason in con.execute(
        "SELECT key, reopen_reason FROM issues_raw WHERE reopen_reason IS NOT NULL AND reopen_reason != '' ORDER BY key"):
        s[f"issue:{key}:reopen_reason"] = reason
    for key, cid, adf in con.execute(
        "SELECT it.key, c.id, c.body_adf FROM comments c JOIN items it ON it.id = c.item_id ORDER BY it.key, c.created_at, c.id"):
        for n, t in enumerate(text_nodes(adf)):
            s[f"comment:{key}:{cid}:{n}"] = t
    for key, title, adf in con.execute(
        "SELECT it.key, it.title, p.body_adf FROM pages p JOIN items it ON it.id = p.item_id ORDER BY it.key"):
        s[f"page:{key}:title"] = title or ""
        for n, t in enumerate(text_nodes(adf)):
            s[f"page:{key}:body:{n}"] = t
    for sid, name in con.execute("SELECT DISTINCT status_id, status FROM issues_raw WHERE status_id != '' ORDER BY 1"):
        s[f"catalog:status:{sid}"] = name
    # The fixture carries no priority ids (a pre-GDK-1491 scrub), so the display
    # name is the only handle; apply.py rewrites by that name.
    for (name,) in con.execute("SELECT DISTINCT priority FROM issues_raw WHERE priority IS NOT NULL AND priority != '' ORDER BY 1"):
        s[f"catalog:priority:{name}"] = name
    for tid, name in con.execute("SELECT DISTINCT issue_type_id, issue_type FROM issues_raw WHERE issue_type_id != '' ORDER BY 1"):
        s[f"catalog:type:{tid}"] = name
    # Sprint names live in two places and neither was translated, so a ko
    # frame read "범위 Sprint 42 백로그 전체" (GDK-1686). Keyed by the sprint
    # id, never the name: the name is the display string being replaced.
    for sid, name in con.execute(
        "SELECT id, name FROM sprints WHERE name IS NOT NULL AND name != '' ORDER BY 1"):
        s[f"catalog:sprint:{sid}"] = name
    # The goal is the line the sprint strip draws under the name (GDK-1717).
    # Its own namespace, not "catalog:sprint:<id>:goal": apply.py splits an id
    # into three parts, so a fourth segment would land inside the sprint id.
    for sid, goal in con.execute(
        "SELECT id, goal FROM sprints WHERE goal IS NOT NULL AND goal != '' ORDER BY 1"):
        s[f"catalog:sprintgoal:{sid}"] = goal
    # The four families the applied-fixture census found on 2026-09-13, after
    # reopen_reason had already shipped English into a Korean retro. Each is a
    # display name with no id family: complete translation files, and a ko take
    # still showing "Done", "Team board", "Sprint 42" as a fix version, and an
    # English environment line on every bug detail (GDK-1847).
    for (name,) in con.execute(
        "SELECT DISTINCT resolution FROM issues_raw WHERE resolution IS NOT NULL AND resolution != '' ORDER BY 1"):
        s[f"catalog:resolution:{name}"] = name
    vers = set()
    for (fv,) in con.execute("SELECT fix_versions FROM issues_raw WHERE fix_versions IS NOT NULL"):
        for name in json.loads(fv or "[]"):
            vers.add(name)
    for (name,) in con.execute("SELECT name FROM versions WHERE name IS NOT NULL AND name != ''"):
        vers.add(name)
    for name in sorted(vers):
        s[f"catalog:version:{name}"] = name
    for bid, name in con.execute(
        "SELECT id, name FROM boards WHERE name IS NOT NULL AND name != '' ORDER BY 1"):
        s[f"catalog:board:{bid}"] = name
    # The environment line is per-issue prose, not a catalog: it names a
    # browser, a region, a deploy. It sits in the issue: family beside the
    # description, keyed by a word rather than a node index (the desc ids all
    # end in one, and apply.py splits on that).
    for key, env in con.execute(
        "SELECT key, environment_text FROM issues_raw WHERE environment_text IS NOT NULL AND environment_text != '' ORDER BY key"):
        s[f"issue:{key}:environment"] = env
    comps = set()
    for (c,) in con.execute("SELECT components FROM issues_raw WHERE components IS NOT NULL"):
        for name in json.loads(c or "[]"): comps.add(name)
    for name in sorted(comps):
        s[f"catalog:component:{name}"] = name
    # Labels, same array shape as components and — since 2026-09-18 — the same
    # answer. They were left English on the reasoning that a label is a slug a
    # real Korean or Japanese team would type in ASCII anyway (GDK-1944). The
    # ja clip settled it the other way (GDK-1976): on one detail screen
    # `customer-reported` sat directly under コンポーネント ダッシュボード and
    # 優先度 最高, and the asymmetry is what reads as unfinished — a component
    # is no less "data" than a label, and it was translated. Issues and pages
    # both carry labels, so the family is per-item like the labels column.
    labels = set()
    for (lb,) in con.execute("SELECT labels FROM issues_raw WHERE labels IS NOT NULL"):
        for name in json.loads(lb or "[]"): labels.add(name)
    for (lb,) in con.execute("SELECT labels FROM pages WHERE labels IS NOT NULL"):
        for name in json.loads(lb or "[]"): labels.add(name)
    for name in sorted(labels):
        s[f"catalog:label:{name}"] = name
    # The link type name is the phone's own label for a linked-issue row: the
    # mobile detail prints links.type raw, so a ko/ja still read "Blocks"
    # beside a translated summary while the applied census — table-blind
    # until that day — said 0 untranslated (GDK-1937). Keyed by the name
    # itself like priority/resolution: links carries no type id, only the
    # display string. link_types.name is the same name in the web's phrase
    # catalog; the demo fixture has no rows there, a real origin's does (v43).
    for (name,) in con.execute("SELECT DISTINCT type FROM links WHERE type IS NOT NULL AND type != '' ORDER BY 1"):
        s[f"catalog:linktype:{name}"] = name
    # The changelog's link rows are rendered sentences over one issue key —
    # Jira's own wording for a link's direction, measured on a live site
    # (internal/origin/linkresolve.go). One id per distinct template with the
    # key as a {key} placeholder, not one per row: 122 rows share five
    # sentences, and apply.py substitutes the row's key back after
    # translation. Every other changelog family needs no id at all — its
    # values are copies of catalog names apply.py derives (GDK-1944).
    for (val,) in con.execute(
        "SELECT to_value FROM changelog WHERE field = 'link' AND to_value IS NOT NULL AND to_value != '' "
        "UNION SELECT from_value FROM changelog WHERE field = 'link' AND from_value IS NOT NULL AND from_value != '' "
        "ORDER BY 1"):
        tpl = CHECK.KEY.sub("{key}", val)
        s[f"changelog:link:{tpl}"] = tpl
    return {"version": 1, "source": "examples/demo.db", "strings": s}

if __name__ == "__main__":
    db = Path(sys.argv[1]) if len(sys.argv) > 1 else ROOT / "examples/demo.db"
    out = Path(sys.argv[2]) if len(sys.argv) > 2 else ROOT / "examples/demo-i18n/en.json"
    data = extract(db)
    out.write_text(json.dumps(data, ensure_ascii=False, indent=1) + "\n")
    print(f"{out}: {len(data['strings'])} strings")
