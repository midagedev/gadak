#!/usr/bin/env python3
"""Extract every translatable string in the demo fixture as id → text (GDK-1556).

Output shape (examples/demo-i18n/en.json):
  {"version": 1, "source": "examples/demo.db", "strings": {"<id>": "<text>", ...}}

Ids are stable paths into the fixture, so a translation file keyed the same
way applies without any English-string matching:
  issue:<KEY>:title                    items.title of an issue
  issue:<KEY>:desc:<n>                 nth text node (document order) of issues_raw.description_adf
  comment:<KEY>:<comment_id>:<n>       nth text node of comments.body_adf
  page:<item_key>:title                items.title of a wiki page
  page:<item_key>:body:<n>             nth text node of pages.body_adf
  catalog:status:<status_id>           issues_raw.status display name
  catalog:priority:<name>              issues_raw.priority display name (the fixture has no priority ids)
  catalog:type:<issue_type_id>         issues_raw.issue_type display name
  catalog:component:<name>             a components[] entry

Text nodes are the unit because ADF marks (links, code) split a sentence into
nodes; a translator sees the nodes of one document together, in order, so the
sentence is still readable. apply.py writes the translation back into the same
node and regenerates body_text / excerpt / FTS from the translated tree.
"""
import json, sqlite3, sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]

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
    comps = set()
    for (c,) in con.execute("SELECT components FROM issues_raw WHERE components IS NOT NULL"):
        for name in json.loads(c or "[]"): comps.add(name)
    for name in sorted(comps):
        s[f"catalog:component:{name}"] = name
    return {"version": 1, "source": "examples/demo.db", "strings": s}

if __name__ == "__main__":
    db = Path(sys.argv[1]) if len(sys.argv) > 1 else ROOT / "examples/demo.db"
    out = Path(sys.argv[2]) if len(sys.argv) > 2 else ROOT / "examples/demo-i18n/en.json"
    data = extract(db)
    out.write_text(json.dumps(data, ensure_ascii=False, indent=1) + "\n")
    print(f"{out}: {len(data['strings'])} strings")
