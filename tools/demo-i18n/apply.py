#!/usr/bin/env python3
"""Apply a demo-fixture translation to a mirror copy (GDK-1556).

    tools/demo-i18n/apply.py <mirror.db> <locale> [--strings examples/demo-i18n/<locale>.json]
    tools/demo-i18n/apply.py <db> <locale> --changelog-plan

The db is edited in place — pass a copy, never examples/demo.db itself. That
file stays English so every e2e assertion keyed on fixture strings keeps
holding; the media targets copy it to e2e/.tmp/demo-<locale>.db, run this, and
seed serve.sh from the copy (GADAK_SEED_DB). --changelog-plan is the one
read-only mode: it prints what the changelog derivation below would resolve,
by which rule, and what it leaves, editing nothing — safe to point straight
at examples/demo.db.

What changes, by id family (see extract.py for the ids):
  issue:<KEY>:title        items.title (the issues view's `summary`), and the
                           summary inside issues_raw.raw so a re-ingest agrees
  issue:<KEY>:desc:<n>     the text node in issues_raw.description_adf; then
                           items.body_text is regenerated from the translated tree
  issue:<KEY>:reopen_reason  issues_raw.reopen_reason — a plain column the retro
                           prints beside a translated title
  comment:<KEY>:<id>:<n>   comments.body_adf node; comments.body_text regenerated
  page:<KEY>:title         items.title
  page:<KEY>:body:<n>      pages.body_adf node; pages.excerpt regenerated
                           (200 runes of collapsed plain text, store/excerpt.go)
  catalog:status:<id>      issues_raw.status for that status_id
  catalog:priority:<name>  issues_raw.priority for that display name
  catalog:type:<id>        issues_raw.issue_type for that issue_type_id
  catalog:component:<n>    the entry in issues_raw.components JSON arrays
  catalog:label:<n>        the entry in issues_raw.labels and pages.labels arrays
  catalog:resolution:<n>   issues_raw.resolution for that display name
  catalog:version:<n>      the entry in issues_raw.fix_versions arrays, and versions.name
  catalog:board:<id>       boards.name for that board id
  issue:<KEY>:environment  issues_raw.environment_text
  catalog:linktype:<n>     links.type for that display name, and link_types.name
  changelog:link:<tpl>     changelog.from_value/to_value of field='link' rows,
                           the row's issue key substituted for {key}

The changelog's own from_value/to_value copies are derived, not given ids row
by row (GDK-1944): every family except link repeats a display name some
catalog family above already carries, so the value is resolved through that
family and nobody translates the same word twice. CHANGELOG_FIELD_RULES is
the single table naming which field answers to which rule; check-applied.py
reads it too — exempting the never-translated value shapes per row and
failing on a field no rule accounts for — so a new field value has to argue
its way in there rather than pass silently.

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

def load_check():
    """check.py owns the wire-fact detectors, key shape among them — the
    template ids and the substitution below share its one definition."""
    spec = importlib.util.spec_from_file_location("dcheck", Path(__file__).with_name("check.py"))
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod

CHECK = load_check()

# ——— the changelog's own copies of every catalog name (GDK-1944) ———
#
# from_value/to_value repeat display names the catalog families above already
# translate, so they are derived from those families rather than given ids of
# their own. This table is the single owner of which changelog field answers
# to which rule; check-applied.py reads it too — to exempt the
# never-translated value shapes per row and to fail on a field no rule
# accounts for — so adding a catalog family covers its changelog copies by
# declaring the field here once. Kinds:
#   derived   the value is a catalog:<family> display name; resolved by the
#             row's from_id/to_id first, then by matching the English value
#             against the family's English source
#   people    a person: Latin on purpose in every locale
#   key       an issue key: never translated
#   filename  a literal file name: never translated
#   template  a rendered sentence over one issue key, translated through one
#             id per distinct template (<prefix> + the English sentence with
#             the key as {key}); the row's key is substituted back in
CHANGELOG_VALUE_COLUMNS = ("from_value", "to_value")
CHANGELOG_FIELD_RULES = {
    "status": {"kind": "derived", "family": "status"},
    "resolution": {"kind": "derived", "family": "resolution"},
    "sprint": {"kind": "derived", "family": "sprint"},
    "assignee": {"kind": "people"},
    "issueparentassociation": {"kind": "key"},
    "attachment": {"kind": "filename"},
    "link": {"kind": "template", "prefix": "changelog:link"},
}

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

def catalog_reverse(en: dict, family: str) -> dict[str, str]:
    """English display name → the catalog key of that family (its id part)."""
    prefix = f"catalog:{family}:"
    return {v: k[len(prefix):] for k, v in en.items() if k.startswith(prefix)}

def resolve_derived(tr: dict, rev: dict, family: str, row_id, row_value):
    """One changelog value copy → (translated name, how) or (None, None).

    By the row's catalog id first — the id is the stable handle; the copied
    name can drift or repeat. That path serves the families whose catalog
    is keyed by id (status, sprint). resolution's catalog is keyed by name
    and the fixture maps its changelog resolution ids nowhere
    (issues_raw.resolution_id is empty on every row and there is no
    resolutions catalog table), so for it the value is the handle that
    resolves: en.json maps the English name back to the catalog key.
    """
    if row_id and f"catalog:{family}:{row_id}" in tr:
        return tr[f"catalog:{family}:{row_id}"], "id"
    if row_value and row_value in rev and f"catalog:{family}:{rev[row_value]}" in tr:
        return tr[f"catalog:{family}:{rev[row_value]}"], "value"
    return None, None

def resolve_template(tr: dict, prefix: str, value, key_rx):
    """A rendered sentence over one issue key → (translated, "template").

    The id is the sentence with the key as a {key} placeholder; the row's
    key is substituted back after translation. A rendering without the
    placeholder would drop the issue key on camera, so it is refused rather
    than applied — the value stays English and the census flags it.
    """
    if not value:
        return None, None
    keys = key_rx.findall(value)
    if len(keys) != 1:
        return None, None
    t = tr.get(f"{prefix}:{key_rx.sub('{key}', value)}")
    if t is None or t.count("{key}") != 1:
        return None, None
    return t.replace("{key}", keys[0]), "template"

def derive_changelog(con, tr: dict, en: dict, write: bool = True):
    """Translate changelog.from_value/to_value per CHANGELOG_FIELD_RULES.

    Returns (counts, left_values, left_ids, unaccounted): counts[field][how]
    says how many values each rule resolved ("id", "value", "template") or
    exempted or left ("exempt", "left"); left_values/left_ids name what was
    left, for --changelog-plan; unaccounted lists field values no rule
    covers — those rows are left untouched for the census to fail on.
    """
    key_rx = CHECK.KEY
    rev = {r["family"]: catalog_reverse(en, r["family"])
           for r in CHANGELOG_FIELD_RULES.values() if r["kind"] == "derived"}
    counts: dict[str, dict] = {}
    left_values: dict[str, set] = {}
    left_ids: dict[str, set] = {}
    unaccounted: set[str] = set()
    rows = con.execute(
        "SELECT item_id, id, field, from_id, from_value, to_id, to_value FROM changelog").fetchall()
    for item_id, row_id, field, fid, fv, tid, tv in rows:
        rule = CHANGELOG_FIELD_RULES.get(field or "")
        c = counts.setdefault(field, {"id": 0, "value": 0, "template": 0, "exempt": 0, "left": 0})
        if rule is None:
            unaccounted.add(field or "")
            continue
        kind = rule["kind"]
        if kind in ("people", "key", "filename"):
            c["exempt"] += int(bool(fv)) + int(bool(tv))
            continue
        news = {}
        for side, rid, val in (("from", fid, fv), ("to", tid, tv)):
            if not val:
                continue
            if kind == "derived":
                new, how = resolve_derived(tr, rev[rule["family"]], rule["family"], rid, val)
            else:
                new, how = resolve_template(tr, rule["prefix"], val, key_rx)
            if how is None or new == val:
                c["left"] += 1
                left_values.setdefault(field, set()).add(val)
                if kind == "template":
                    left_ids.setdefault(field, set()).add(f"{rule['prefix']}:{key_rx.sub('{key}', val)}")
                elif val in rev[rule["family"]]:
                    left_ids.setdefault(field, set()).add(f"catalog:{rule['family']}:{rev[rule['family']][val]}")
            else:
                c[how] += 1
            if new is not None:
                news[side] = new  # a refused side keeps its original value
        new_fv, new_tv = news.get("from", fv), news.get("to", tv)
        if write and (new_fv != fv or new_tv != tv):
            con.execute("UPDATE changelog SET from_value = ?, to_value = ? WHERE item_id = ? AND id = ?",
                        (new_fv, new_tv, item_id, row_id))
    return counts, left_values, left_ids, unaccounted

def changelog_plan(con, db, locale, tr, en) -> int:
    """Print what the changelog derivation resolves and what it leaves."""
    counts, left_values, left_ids, unaccounted = derive_changelog(con, tr, en, write=False)
    print(f"changelog derivation plan — {db.name}, locale {locale}")
    for field, r in CHANGELOG_FIELD_RULES.items():
        c = counts.get(field)
        if not c:
            continue
        if r["kind"] == "derived":
            src, bits = f"catalog:{r['family']}", [f"{c['id']} by id, {c['value']} by name"]
        elif r["kind"] == "template":
            src, bits = r["prefix"], [f"{c['template']} by template"]
        else:
            src, bits = "—", [f"{c['exempt']} exempt ({r['kind']} — never translated)"]
        bits.append(f"{c['left']} left English")
        print(f"  {field:<24}{r['kind']:<10}{src:<22}{', '.join(bits)}")
        for tid in sorted(left_ids.get(field, []))[:6]:
            print(f"      needs a rendering: {tid}")
        if len(left_ids.get(field, ())) > 6:
            print(f"      … and {len(left_ids[field]) - 6} more")
        # Left values that map to no id at all (a name no catalog carries)
        # have no pointer to print — show the values themselves.
        if c["left"] and not left_ids.get(field):
            for v in sorted(left_values.get(field, ()))[:6]:
                print(f"      left: {v}")
            if len(left_values.get(field, ())) > 6:
                print(f"      … and {len(left_values[field]) - 6} more")
    if unaccounted:
        print(f"  fields no rule accounts for (rows left as-is, census will fail): {', '.join(sorted(unaccounted))}")
    return 0

def main() -> int:
    args = [a for a in sys.argv[1:] if not a.startswith("--")]
    if len(args) < 2:
        print(__doc__); return 2
    db, locale = Path(args[0]), args[1]
    strings_path = ROOT / f"examples/demo-i18n/{locale}.json"
    if "--strings" in sys.argv:
        strings_path = Path(sys.argv[sys.argv.index("--strings") + 1])
    if "--changelog-plan" in sys.argv:
        # Read-only, so the committed English fixture is a legal target.
        tr = json.loads(strings_path.read_text())["strings"]
        en = json.loads((ROOT / "examples/demo-i18n/en.json").read_text())["strings"]
        return changelog_plan(sqlite3.connect(f"file:{db}?mode=ro", uri=True), db, locale, tr, en)
    if db.resolve() == (ROOT / "examples/demo.db").resolve():
        print("refusing to edit examples/demo.db in place — pass a copy"); return 2
    tr = json.loads(strings_path.read_text())["strings"]
    # The English source beside the translation: the changelog derivation
    # matches copied display names against it (GDK-1944).
    en = json.loads((ROOT / "examples/demo-i18n/en.json").read_text())["strings"]
    con = sqlite3.connect(db)
    con.execute("PRAGMA foreign_keys=OFF")

    # Group node translations per document.
    issues: dict[str, dict] = {}; comments: dict[tuple, dict] = {}; pages: dict[str, dict] = {}
    for k, v in tr.items():
        p = k.split(":")
        if p[0] == "issue":
            d = issues.setdefault(p[1], {"title": None, "nodes": {}, "reopen_reason": None, "environment": None})
            if p[2] == "title": d["title"] = v
            elif p[2] == "reopen_reason": d["reopen_reason"] = v
            elif p[2] == "environment": d["environment"] = v
            else: d["nodes"][int(p[3])] = v
        elif p[0] == "comment":
            # The comment id itself carries a colon ("jira:10358"), so the
            # node index is the last segment and the id is everything between.
            comments.setdefault((p[1], ":".join(p[2:-1])), {})[int(p[-1])] = v
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
        if d["reopen_reason"] is not None:
            con.execute("UPDATE issues_raw SET reopen_reason = ? WHERE item_id = ?", (d["reopen_reason"], item_id))
        if d["environment"] is not None:
            con.execute("UPDATE issues_raw SET environment_text = ? WHERE item_id = ?", (d["environment"], item_id))
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

    # The changelog's value copies, derived from the catalogs (GDK-1944):
    # read the rows and the two string files only, so it shares nothing
    # with the catalog rewrites below and their order cannot matter.
    ch_counts, ch_left_values, ch_left_ids, ch_unaccounted = derive_changelog(con, tr, en)
    if ch_unaccounted:
        print(f"changelog fields no rule accounts for (rows left as-is): {', '.join(sorted(ch_unaccounted))}")

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
        elif p[1] == "sprint":
            # Two copies of one name: the sprints row the board's scope reads
            # and the projection on the issue (GDK-1686). Both, or the frame
            # disagrees with itself.
            con.execute("UPDATE sprints SET name = ? WHERE id = ?", (v, p[2]))
            con.execute("UPDATE issues_raw SET sprint_name = ? WHERE sprint_id = ?", (v, p[2]))
        elif p[1] == "resolution":
            con.execute("UPDATE issues_raw SET resolution = ? WHERE resolution = ?", (v, p[2]))
        elif p[1] == "board":
            con.execute("UPDATE boards SET name = ? WHERE id = ?", (v, p[2]))
        elif p[1] == "linktype":
            # Two copies of one name, like sprints: links.type is what the
            # phone prints raw on a linked-issue row (GDK-1937), link_types
            # is the web's phrase catalog keyed by the same name — empty in
            # the demo fixture, present once a real origin's catalog rides
            # along (v43). Rewriting the type string strands nothing the
            # viewer reads: the phrase and the label both come from these
            # two columns.
            con.execute("UPDATE links SET type = ? WHERE type = ?", (v, p[2]))
            con.execute("UPDATE link_types SET name = ? WHERE name = ?", (v, p[2]))
        elif p[1] == "sprintgoal":
            # One copy: the goal lives only on the sprints row (GDK-1717).
            con.execute("UPDATE sprints SET goal = ? WHERE id = ?", (v, p[2]))
    # Fix versions live in two places, like sprint names: the project catalog
    # row and the same-order array on every issue. Both, or the detail panel
    # and the version list disagree.
    vers = {k.split(":", 2)[2]: v for k, v in tr.items() if k.startswith("catalog:version:")}
    if vers:
        for name, tl in vers.items():
            con.execute("UPDATE versions SET name = ? WHERE name = ?", (tl, name))
        for item_id, fj in con.execute(
            "SELECT item_id, fix_versions FROM issues_raw WHERE fix_versions IS NOT NULL AND fix_versions != '[]'").fetchall():
            arr = json.loads(fj)
            new = [vers.get(x, x) for x in arr]
            if new != arr:
                con.execute("UPDATE issues_raw SET fix_versions = ? WHERE item_id = ?", (json.dumps(new, ensure_ascii=False), item_id))
    comp = {k.split(":", 2)[2]: v for k, v in tr.items() if k.startswith("catalog:component:")}
    if comp:
        for item_id, cj in con.execute("SELECT item_id, components FROM issues_raw WHERE components IS NOT NULL AND components != '[]'").fetchall():
            arr = json.loads(cj)
            new = [comp.get(c, c) for c in arr]
            if new != arr:
                con.execute("UPDATE issues_raw SET components = ? WHERE item_id = ?", (json.dumps(new, ensure_ascii=False), item_id))
    # Labels (GDK-1976), the same array rewrite as components, on both
    # projections — pages carry labels too, and the wiki list shows them.
    lab = {k.split(":", 2)[2]: v for k, v in tr.items() if k.startswith("catalog:label:")}
    if lab:
        for table in ("issues_raw", "pages"):
            for item_id, lj in con.execute(
                    f"SELECT item_id, labels FROM {table} WHERE labels IS NOT NULL AND labels != '[]'").fetchall():
                arr = json.loads(lj)
                new_arr = [lab.get(x, x) for x in arr]
                if new_arr != arr:
                    con.execute(f"UPDATE {table} SET labels = ? WHERE item_id = ?",
                                (json.dumps(new_arr, ensure_ascii=False), item_id))
    con.commit()

    scrub = load_scrub()
    # Force a rebuild regardless of DDL shape: the tokens changed, not the schema.
    con.execute("DROP TABLE IF EXISTS items_fts")
    # Same shape scripts/scrub-demo-db.py rebuilds: the labels column, the
    # porter wrapper (GDK-1021) and script_runs (GDK-1978). Recreating a
    # shorter-column DDL here would hand the ko/ja recording fixture an index
    # that silently loses label-only hits, English stem variants or the
    # CJK-glued Latin runs — the named trap of 0009 §Consequences, one writer
    # at a time.
    con.execute("CREATE VIRTUAL TABLE items_fts USING fts5(title, labels, body_text, comments_text, cjk_bigram, script_runs, content='', tokenize='porter unicode61 remove_diacritics 2')")
    rows = con.execute("""
        SELECT i.rowid, COALESCE(i.title,''), COALESCE(i.body_text, ''),
               COALESCE((SELECT group_concat(body_text, char(10)) FROM (SELECT body_text FROM comments WHERE item_id = i.id AND body_text <> '' ORDER BY rowid)), ''),
               COALESCE(ir.labels, p.labels, '[]')
        FROM items i
        LEFT JOIN issues_raw ir ON ir.item_id = i.id
        LEFT JOIN pages p ON p.item_id = i.id""").fetchall()
    con.executemany("INSERT INTO items_fts (rowid, title, labels, body_text, comments_text, cjk_bigram, script_runs) VALUES (?, ?, ?, ?, ?, ?, ?)",
        [(r, t, scrub.fts_labels_text(lb), b, c,
          scrub.cjk_bigram_column(t, scrub.fts_labels_text(lb), b, c),
          scrub.script_runs_column(t, scrub.fts_labels_text(lb), b, c)) for r, t, b, c, lb in rows])
    con.commit()
    fts = con.execute("SELECT count(*) FROM items_fts").fetchone()[0]
    ch_derived = sum(c["id"] + c["value"] + c["template"] for c in ch_counts.values())
    ch_left = sum(c["left"] for c in ch_counts.values())
    print(f"{db}: {locale} applied — issues {n_issue}, comments {n_comment}, pages {n_page}, "
          f"catalog {sum(1 for k in tr if k.startswith('catalog:'))}; "
          f"changelog {ch_derived} derived, {ch_left} left English"
          f"{'' if not ch_left else ' (--changelog-plan names them)'}; items_fts {fts} rows")
    return 0

if __name__ == "__main__":
    sys.exit(main())
