#!/usr/bin/env python3
"""Gate for a demo-fixture translation file (GDK-1556).

    tools/demo-i18n/check.py <locale> [translation.json] [--source en.json] [--partial]

Fails (exit 1) when:
  - an id in the source has no translation (unless --partial: then only ids present are checked)
  - one English string is rendered more than one way (catalog: ids excluded; the majority wins)
  - a translation has an id the source does not (a typo that would silently never apply)
  - a translation is still the English text, when the source has letters to translate
    (pure keys / URLs / numbers / code are allowed to stay)
  - a translation carries no script of its locale (Hangul for ko; kana or kanji for ja)
    when the source has letters
  - issue keys (NMB-12), URLs or backtick spans differ from the source, or a digit run of
    the source is missing from the translation — those are wire facts, not prose
Each failing id is printed with its reason; counts at the end.
"""
import json, re, sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
# Lookarounds, not \b: a key glued to Hangul/kana ("NMA-151을", "NMA-151が") is
# still the key — \b treats the CJK letter as a word character and misses it.
KEY = re.compile(r"(?<![A-Za-z0-9])[A-Z][A-Z0-9]+-\d+(?!\d)")
# A URL stops at whitespace or at the first CJK letter: "https://x.example/ 을"
# and "https://x.example/を" carry the same URL, the particle is prose.
URL = re.compile(r"(?:https?|wss?)://[^\s가-힣぀-ヿ一-鿿「」（）、。]+")
CODE = re.compile(r"`[^`]+`")
DIGITS = re.compile(r"\d+")
LETTERS = re.compile(r"[A-Za-z]")
SCRIPT = {
    "ko": re.compile(r"[가-힣]"),
    "ja": re.compile(r"[぀-ヿ一-鿿]"),
}
# Proper nouns / product words that legitimately stay Latin in both locales.
LATIN_OK = {"SDK", "REST", "API", "Auth", "Webhooks", "Nimbus", "NMB", "NMA", "NMS", "OK", "API", "UI", "ID", "URL", "JSON", "CSV", "SSO", "IdP", "OAuth", "SAML", "S3", "CDN", "SLA", "SLO", "P0", "P1", "P2", "QA", "CI", "PR", "Slack", "Stripe", "GitHub", "Datadog", "PagerDuty", "Sentry", "Redis", "Postgres", "Kafka", "gRPC", "GraphQL", "HTTP", "TLS", "DNS", "IP", "SQL", "iOS", "Android", "macOS", "Windows", "Linux", "Chrome", "Safari", "Firefox", "Terraform", "Kubernetes", "Docker", "AWS", "GCP", "Azure"}

def facts(s: str):
    # A sentence-final "." after a URL is punctuation, not part of the URL.
    urls = sorted(u.rstrip(".,;:!?)") for u in URL.findall(s))
    return (sorted(KEY.findall(s)), urls, sorted(CODE.findall(s)), sorted(DIGITS.findall(s)))

FILEISH = re.compile(r"^[\w-]+\.[a-z0-9]{1,5}$")  # context.svg, retention.md

def needs_translation(src: str) -> bool:
    if not LETTERS.search(src):
        return False
    # A source that is already in a CJK script (the fixture carries a Korean
    # wiki page) is not English to translate; the translator may leave it.
    if any(rx.search(src) for rx in SCRIPT.values()):
        return False
    stripped = KEY.sub("", URL.sub("", CODE.sub("", src)))
    words = [w for w in re.findall(r"[A-Za-z][A-Za-z0-9.+/-]*", stripped) if not FILEISH.match(w)]
    if not words:
        return False
    if all(w in LATIN_OK for w in words):
        return False
    return True

def main() -> int:
    args = [a for a in sys.argv[1:] if not a.startswith("--")]
    partial = "--partial" in sys.argv
    src_path = ROOT / "examples/demo-i18n/en.json"
    if "--source" in sys.argv:
        src_path = Path(sys.argv[sys.argv.index("--source") + 1])
    if not args:
        print(__doc__); return 2
    locale = args[0]
    tr_path = Path(args[1]) if len(args) > 1 else ROOT / f"examples/demo-i18n/{locale}.json"
    if locale not in SCRIPT:
        print(f"unknown locale {locale}"); return 2
    src = json.loads(src_path.read_text())["strings"]
    tr = json.loads(tr_path.read_text())["strings"]
    bad = []
    for k in tr:
        if k not in src:
            bad.append((k, "id not in source"))
    ids = tr.keys() if partial else src.keys()
    for k in ids:
        if k not in tr:
            bad.append((k, "missing")); continue
        s, t = src[k], tr[k]
        if not isinstance(t, str) or t.strip() == "":
            bad.append((k, "empty")); continue
        fs, ft = facts(s), facts(t)
        # Keys, URLs and code spans: exact multiset. Digit runs: the source's
        # must all survive; the translation may add its own (a spelled-out
        # "two" becoming "2건" is a translation, not a changed fact).
        if fs[:3] != ft[:3] or any(ft[3].count(d) < fs[3].count(d) for d in set(fs[3])):
            bad.append((k, f"facts changed: {fs} -> {ft}")); continue
        if needs_translation(s):
            if t.strip() == s.strip():
                bad.append((k, "still English")); continue
            if not SCRIPT[locale].search(t):
                bad.append((k, f"no {locale} script")); continue
    # One English string, one rendering. The fixture repeats its boilerplate
    # (the "Expected: … / Actual: … / Impact: …" description template, the
    # stock comments) across hundreds of issues, and the sharded translation
    # rendered the same sentence up to nine ways — visible on camera as two
    # neighbouring search hits quoting one comment differently (vision
    # verdict 2026-09-07, findings 10–11). Catalog names are one id each and
    # are excluded; the majority rendering is the one every id must carry.
    by_source: dict[str, dict[str, list[str]]] = {}
    for k in ids:
        if k.startswith("catalog:") or k not in tr or k not in src:
            continue
        by_source.setdefault(src[k], {}).setdefault(tr[k], []).append(k)
    for source, renderings in by_source.items():
        if len(renderings) < 2:
            continue
        majority = max(renderings, key=lambda r: (len(renderings[r]), r))
        for r, ks in renderings.items():
            if r == majority:
                continue
            for k in ks:
                bad.append((k, f"inconsistent: source rendered {len(renderings)} ways; majority is {majority[:60]!r}"))
    for k, why in bad:
        print(f"FAIL {k}: {why}")
    checked = len(list(ids))
    print(f"{tr_path.name}: {checked} ids checked, {len(bad)} failing")
    return 1 if bad else 0

if __name__ == "__main__":
    sys.exit(main())
