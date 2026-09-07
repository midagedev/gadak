#!/usr/bin/env python3
"""Gate for a demo-fixture translation file (GDK-1556).

    tools/demo-i18n/check.py <locale> [translation.json] [--source en.json] [--partial]

Fails (exit 1) when:
  - an id in the source has no translation (unless --partial: then only ids present are checked)
  - a translation has an id the source does not (a typo that would silently never apply)
  - a translation is still the English text, when the source has letters to translate
    (pure keys / URLs / numbers / code are allowed to stay)
  - a translation carries no script of its locale (Hangul for ko; kana or kanji for ja)
    when the source has letters
  - issue keys (NMB-12), URLs, backtick spans, or digit runs present in the source are
    missing from the translation — those are wire facts, not prose
Each failing id is printed with its reason; counts at the end.
"""
import json, re, sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
KEY = re.compile(r"\b[A-Z][A-Z0-9]+-\d+\b")
URL = re.compile(r"https?://\S+")
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
    return (sorted(KEY.findall(s)), sorted(URL.findall(s)), sorted(CODE.findall(s)), sorted(DIGITS.findall(s)))

def needs_translation(src: str) -> bool:
    if not LETTERS.search(src):
        return False
    stripped = KEY.sub("", URL.sub("", CODE.sub("", src)))
    words = [w for w in re.findall(r"[A-Za-z][A-Za-z0-9.+/-]*", stripped)]
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
        if facts(s) != facts(t):
            bad.append((k, f"facts changed: {facts(s)} -> {facts(t)}")); continue
        if needs_translation(s):
            if t.strip() == s.strip():
                bad.append((k, "still English")); continue
            if not SCRIPT[locale].search(t):
                bad.append((k, f"no {locale} script")); continue
    for k, why in bad:
        print(f"FAIL {k}: {why}")
    checked = len(list(ids))
    print(f"{tr_path.name}: {checked} ids checked, {len(bad)} failing")
    return 1 if bad else 0

if __name__ == "__main__":
    sys.exit(main())
