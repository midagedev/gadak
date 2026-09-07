#!/usr/bin/env python3
"""Apply a demo-fixture translation to a mirror copy (GDK-1556).

    tools/demo-i18n/apply.py <mirror.db> <locale> [--strings examples/demo-i18n/<locale>.json]

The db is edited in place — pass a copy, never examples/demo.db itself. That
file stays English so every e2e assertion keyed on fixture strings keeps
holding; the media targets copy it to e2e/.tmp/demo-<locale>.db, run this, and
seed serve.sh from the copy (GADAK_SEED_DB).

What changes, by id family (see extract.py for the ids):
  issue:<KEY>:title        items.title (the issues view's `summary`), and the
                           summary inside issues_raw.raw so a re-ingest agrees
  issue:<KEY>:desc:<n>     the text node in issues_raw.description_adf; then
                           items.body_text is regenerated from the translated tree
  comment:<KEY>:<id>:<n>   comments.body_adf node; comments.body_text regenerated
  page:<KEY>:title         items.title
  page:<KEY>:body:<n>      pages.body_adf node; pages.excerpt regenerated
                           (200 runes of collapsed plain text, store/excerpt.go)
  catalog:status:<id>      issues_raw.status for that status_id
  catalog:priority:<name>  issues_raw.priority for that display name
  catalog:type:<id>        issues_raw.issue_type for that issue_type_id
  catalog:component:<n>    the entry in issues_raw.components JSON arrays
Finally items_fts is rebuilt (same shape as scripts/scrub-demo-db.py), because
it is contentless and holds the English tokens otherwise.

Plain text from ADF follows internal/adf PlainText closely enough for FTS and
excerpts: block nodes are joined by newlines, inline text concatenated.
"""
import importlib.util, json, sqlite3, sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
BLOCK = {"paragraph", "heading", "listItem", "tableCell", "tableHeader", "codeBlock", "blockquote", "panel", "rule"}

def load_scrub():
    spec = importlib.util.spec_from_file_location("scrub", ROOT / "scripts/scrub-demo-db.py")
    mod = importlib.util.module_from_spec(spec); spec.loader.exec_module(mod)
    return mod

def set_text_nodes(adf: str, texts: dict[int, str]) -> tuple[str, str]:
    """Replace the nth text node with texts[n]; return (new adf json, plain text)."""
    doc = json.loads(adf)
    counter = [0]
    lines: list[str] = []
    cur: list[str] = []
    def flush():
        if cur:
            lines.append("".join(cur)); cur.clear()
    def walk(node):
        if isinstance(node, dict):
            t = node.get("type")
            if t == "text" and isinstance(node.get("text"), str):
                n = counter[0]; counter[0] += 1
                if n in texts:
                    node["text"] = texts[n]
                cur.append(node["text"])
            elif t == "hardBreak":
                cur.append("\n")
            for c in node.get("content") or []:
                walk(c)
            if t in BLOCK:
                flush()
        elif isinstance(node, list):
            for c in node: walk(c)
    walk(doc); flush()
    return json.dumps(doc, ensure_ascii=False, separators=(",", ":")), "\n".join(l for l in lines if l.strip())

def excerpt(plain: str, runes: int = 200) -> str:
    s = " ".join(plain.split())
    if len(s) <= runes:
        return s
    cut = s[:runes]
    sp = cut.rfind(" ")
    return (cut[:sp] if sp > runes // 2 else cut).rstrip() + "…"

def main() -> int:
    args = [a for a in sys.argv[1:] if not a.startswith("--")]
    if len(args) < 2:
        print(__doc__); return 2
    db, locale = Path(args[0]), args[1]
    strings_path = ROOT / f"examples/demo-i18n/{locale}.json"
    if "--strings" in sys.argv:
        strings_path = Path(sys.argv[sys.argv.index("--strings") + 1])
    if db.resolve() == (ROOT / "examples/demo.db").resolve():
        print("refusing to edit examples/demo.db in place — pass a copy"); return 2
    tr = json.loads(strings_path.read_text())["strings"]
    con = sqlite3.connect(db)
    con.execute("PRAGMA foreign_keys=OFF")

    # Group node translations per document.
    issues: dict[str, dict] = {}; comments: dict[tuple, dict] = {}; pages: dict[str, dict] = {}
    for k, v in tr.items():
        p = k.split(":")
        if p[0] == "issue":
            d = issues.setdefault(p[1], {"title": None, "nodes": {}})
            if p[2] == "title": d["title"] = v
            else: d["nodes"][int(p[3])] = v
        elif p[0] == "comment":
            comments.setdefault((p[1], p[2]), {})[int(p[3])] = v
        elif p[0] == "page":
            d = pages.setdefault(p[1], {"title": None, "nodes": {}})
            if p[2] == "title": d["title"] = v
            else: d["nodes"][int(p[3])] = v

    n_issue = n_comment = n_page = 0
    for key, d in issues.items():
        row = con.execute("SELECT i.item_id, i.description_adf, i.raw FROM issues_raw i WHERE i.key = ?", (key,)).fetchone()
        if not row: continue
        item_id, adf, raw = row
        if d["title"] is not None:
            con.execute("UPDATE items SET title = ? WHERE id = ?", (d["title"], item_id))
            if raw:
                try:
                    r = json.loads(raw)
                    if isinstance(r.get("fields"), dict) and "summary" in r["fields"]:
                        r["fields"]["summary"] = d["title"]
                        con.execute("UPDATE issues_raw SET raw = ? WHERE item_id = ?", (json.dumps(r, ensure_ascii=False), item_id))
                except json.JSONDecodeError:
                    pass
        if d["nodes"] and adf:
            new_adf, plain = set_text_nodes(adf, d["nodes"])
            con.execute("UPDATE issues_raw SET description_adf = ? WHERE item_id = ?", (new_adf, item_id))
            con.execute("UPDATE items SET body_text = ? WHERE id = ?", (plain, item_id))
        n_issue += 1
    for (key, cid), nodes in comments.items():
        row = con.execute("SELECT c.item_id, c.body_adf FROM comments c JOIN items it ON it.id = c.item_id WHERE it.key = ? AND c.id = ?", (key, cid)).fetchone()
        if not row or not row[1]: continue
        new_adf, plain = set_text_nodes(row[1], nodes)
        con.execute("UPDATE comments SET body_adf = ?, body_text = ? WHERE item_id = ? AND id = ?", (new_adf, plain, row[0], cid))
        n_comment += 1
    for key, d in pages.items():
        row = con.execute("SELECT p.item_id, p.body_adf FROM pages p JOIN items it ON it.id = p.item_id WHERE it.key = ?", (key,)).fetchone()
        if not row: continue
        item_id, adf = row
        if d["title"] is not None:
            con.execute("UPDATE items SET title = ? WHERE id = ?", (d["title"], item_id))
        if d["nodes"] and adf:
            new_adf, plain = set_text_nodes(adf, d["nodes"])
            con.execute("UPDATE pages SET body_adf = ?, excerpt = ? WHERE item_id = ?", (new_adf, excerpt(plain), item_id))
            con.execute("UPDATE items SET body_text = ? WHERE id = ?", (plain, item_id))
        n_page += 1

    # Catalog display names. Never the ids/categories — those are the wire.
    for k, v in tr.items():
        p = k.split(":", 2)
        if p[0] != "catalog": continue
        if p[1] == "status":
            con.execute("UPDATE issues_raw SET status = ? WHERE status_id = ?", (v, p[2]))
        elif p[1] == "priority":
            con.execute("UPDATE issues_raw SET priority = ? WHERE priority = ?", (v, p[2]))
        elif p[1] == "type":
            con.execute("UPDATE issues_raw SET issue_type = ? WHERE issue_type_id = ?", (v, p[2]))
    comp = {k.split(":", 2)[2]: v for k, v in tr.items() if k.startswith("catalog:component:")}
    if comp:
        for item_id, cj in con.execute("SELECT item_id, components FROM issues_raw WHERE components IS NOT NULL AND components != '[]'").fetchall():
            arr = json.loads(cj)
            new = [comp.get(c, c) for c in arr]
            if new != arr:
                con.execute("UPDATE issues_raw SET components = ? WHERE item_id = ?", (json.dumps(new, ensure_ascii=False), item_id))
    con.commit()

    scrub = load_scrub()
    # Force a rebuild regardless of DDL shape: the tokens changed, not the schema.
    con.execute("DROP TABLE IF EXISTS items_fts")
    con.execute("CREATE VIRTUAL TABLE items_fts USING fts5(title, body_text, comments_text, cjk_bigram, content='', tokenize='unicode61 remove_diacritics 2')")
    rows = con.execute("""
        SELECT i.rowid, COALESCE(i.title,''), COALESCE(i.body_text, ''),
               COALESCE((SELECT group_concat(body_text, char(10)) FROM (SELECT body_text FROM comments WHERE item_id = i.id AND body_text <> '' ORDER BY rowid)), '')
        FROM items i""").fetchall()
    con.executemany("INSERT INTO items_fts (rowid, title, body_text, comments_text, cjk_bigram) VALUES (?, ?, ?, ?, ?)",
        [(r, t, b, c, scrub.cjk_bigram_column(t, b, c)) for r, t, b, c in rows])
    con.commit()
    fts = con.execute("SELECT count(*) FROM items_fts").fetchone()[0]
    print(f"{db}: {locale} applied — issues {n_issue}, comments {n_comment}, pages {n_page}, catalog {sum(1 for k in tr if k.startswith('catalog:'))}; items_fts {fts} rows")
    return 0

if __name__ == "__main__":
    sys.exit(main())
