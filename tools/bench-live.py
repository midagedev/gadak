#!/usr/bin/env python3
"""Live benchmark: the Jira REST API vs the gadak CLI, on YOUR mirror.

Usage:  python3 tools/bench-live.py [profile]     (default: the default profile)
Env:    GADAK_BIN=/path/to/gadak                  (default: gadak on PATH)

Reads site/email/token from the profile's config.json to call the REST API
directly; never prints them. Results (docs/BENCHMARKS.md) mask the project
key. Run on a quiet machine; medians of 5 (3 for paged scenarios)."""
import json, os, subprocess, time, statistics, base64, urllib.request, urllib.parse, ssl, sys, atexit

PROFILE = sys.argv[1] if len(sys.argv) > 1 else ""
HOME = os.path.expanduser(f"~/.gadak/profiles/{PROFILE}") if PROFILE else os.path.expanduser("~/.gadak")
cfg = json.load(open(f"{HOME}/config.json"))
SITE = cfg["site"].rstrip("/")
EMAIL = cfg.get("email")
TOKEN = cfg.get("apiToken") or cfg.get("token")
AUTH = "Basic " + base64.b64encode(f"{EMAIL}:{TOKEN}".encode()).decode()
PROJ = cfg["projects"][0]
GADAK = os.environ.get("GADAK_BIN", "gadak")
ENVP = {**os.environ}

# The GitHub release check is an outbound network call that must not sit
# inside timed numbers. config.json owns the switch, so flip it for this run
# and restore the file byte-for-byte on the way out (GDK-94).
_cfg_path = f"{HOME}/config.json"
_cfg_orig = open(_cfg_path).read()
atexit.register(lambda: open(_cfg_path, "w").write(_cfg_orig))
if json.loads(_cfg_orig).get("updateCheck", True):
    _cfg = json.loads(_cfg_orig)
    _cfg["updateCheck"] = False
    open(_cfg_path, "w").write(json.dumps(_cfg))
    print("# update check disabled for this run (config restored on exit)\n")

def rest(path, params=None, method="GET", body=None):
    url = SITE + path + (("?" + urllib.parse.urlencode(params)) if params else "")
    req = urllib.request.Request(url, method=method,
        headers={"Authorization": AUTH, "Accept": "application/json",
                 "Content-Type": "application/json"},
        data=json.dumps(body).encode() if body else None)
    t0 = time.perf_counter()
    with urllib.request.urlopen(req, timeout=60) as r:
        data = r.read()
    return time.perf_counter() - t0, json.loads(data)

def cli(args):
    t0 = time.perf_counter()
    r = subprocess.run([GADAK] + (["--profile", PROFILE] if PROFILE else []) + args,
                       capture_output=True, text=True, env=ENVP)
    dt = time.perf_counter() - t0
    if r.returncode != 0:
        raise RuntimeError(f"cli fail {args[:2]}: {r.stderr[:200]}")
    return dt, r.stdout

def med(runs): return statistics.median(runs)

def bench(name, fn, reps=5):
    runs = []
    out_len = 0
    for _ in range(reps):
        dt, out = fn()
        runs.append(dt * 1000)
        out_len = len(out) if isinstance(out, str) else len(json.dumps(out))
    print(f"{name:46s} median {med(runs):9.1f} ms   min {min(runs):8.1f}  max {max(runs):8.1f}  (n={reps}, out≈{out_len}B)")
    return med(runs)

R = {}
ISSUES = subprocess.run([GADAK] + (["--profile", PROFILE] if PROFILE else []) + ["sql", "--no-header", "select count(*) from issues"], capture_output=True, text=True).stdout.strip()
print(f"# corpus: profile={PROFILE or 'default'}, project ***, issues={ISSUES} (mirror)\n")

# ── 1. 단순 필터 100건 ─────────────────────────────
jql1 = f'project = {PROJ} AND statusCategory != Done ORDER BY updated DESC'
def rest_search(jql, maxr=100, fields="key,status,summary"):
    try:
        return rest("/rest/api/3/search/jql", {"jql": jql, "maxResults": maxr, "fields": fields})
    except Exception:
        return rest("/rest/api/3/search", {"jql": jql, "maxResults": maxr, "fields": fields})
R['rest_filter'] = bench("1a REST search 100 (simple filter)", lambda: rest_search(jql1))
R['sql_filter'] = bench("1b gadak sql 100 (same filter)", lambda: cli(["sql", "--no-header",
    "select key,status,summary from issues_full where status_category != 'done' order by updated_at desc limit 100"]))
R['jql_filter'] = bench("1c gadak search --jql (same filter)", lambda: cli(["search", "--jql", jql1, "--limit", "100"]))

# ── 2. 단건 상세(+changelog) ───────────────────────
_, top = cli(["sql", "--no-header",
    "select i.key from issues i join items it on it.id=i.item_id where i.comment_count>3 order by i.comment_count desc limit 1"])
KEY = top.strip().splitlines()[0]
print("\n# detail key: *** (comment-heavy)")
R['rest_issue'] = bench("2a REST issue + changelog", lambda: rest(f"/rest/api/3/issue/{KEY}", {"expand": "changelog"}))
R['cli_issue'] = bench("2b gadak issue (full detail)", lambda: cli(["issue", KEY]))

# ── 3. 텍스트 검색 ────────────────────────────────
_, w = cli(["sql", "--no-header",
    "select lower(title) from items where kind='issue' order by length(title) desc limit 1"])
word = next((t for t in w.split() if len(t) >= 4 and t.isalpha()), "test")
print(f"\n# text term: (masked, {len(word)} chars)")
R['rest_text'] = bench("3a REST jql text ~ term", lambda: rest_search(f'text ~ "{word}"', 50, "key,summary"))
R['cli_text'] = bench("3b gadak search term (FTS)", lambda: cli(["search", word, "--limit", "50"]))

# ── 4. 에픽 GROUP BY: REST의 정직한 총비용 ─────────
def rest_groupby():
    t0 = time.perf_counter(); total_fetched = 0; token = None; pages = 0
    agg = {}
    while True:
        params = {"jql": f"project = {PROJ} AND resolution is EMPTY", "maxResults": 100, "fields": "parent"}
        if token: params["nextPageToken"] = token
        dt, data = rest("/rest/api/3/search/jql", params)
        pages += 1
        for it in data.get("issues", []):
            p = (it.get("fields") or {}).get("parent") or {}
            agg[p.get("key", "")] = agg.get(p.get("key", ""), 0) + 1
        token = data.get("nextPageToken")
        total_fetched += len(data.get("issues", []))
        if not token: break
    return time.perf_counter() - t0, {"pages": pages, "rows": total_fetched}
def one_groupby():
    dt, meta = rest_groupby()
    one_groupby.meta = meta
    return dt, meta
R['rest_group'] = bench("4a REST GROUP BY epic (paged + client agg)", one_groupby, reps=3)
print(f"    └ pages per run: {one_groupby.meta['pages']}, rows aggregated: {one_groupby.meta['rows']}")
R['sql_group'] = bench("4b gadak sql GROUP BY epic_key (one shot)", lambda: cli(["sql", "--no-header",
    "select epic_key, count(*) from issues_full where resolved_at is null and epic_key <> '' group by epic_key order by 2 desc"]))

# ── 5. reopen: REST로는 표현 불가 ──────────────────
R['sql_reopen'] = bench("5a gadak sql reopen_count aggregate", lambda: cli(["sql", "--no-header",
    "select count(*) from issues where reopen_count > 0"]))
_, keys5 = cli(["sql", "--no-header", "select key from issues order by comment_count desc limit 5"])
K5 = keys5.strip().splitlines()
def changelog_sample():
    t0 = time.perf_counter()
    for k in K5:
        rest(f"/rest/api/3/issue/{k}", {"expand": "changelog", "fields": "status"})
    return time.perf_counter() - t0, {"issues": len(K5)}
R['rest_changelog5'] = bench("5b REST changelog x5 issues (sample)", changelog_sample, reps=3)
per_issue = R['rest_changelog5'] / 5
print(f"    └ per-issue changelog ≈ {per_issue:.0f} ms → {ISSUES} issues ≈ {per_issue*int(ISSUES)/1000:.0f}s (REST가 reopen을 재려면 전 이슈 순회)")

# ── 6. gadak이 지는 행 ─────────────────────────────
R['cli_startup'] = bench("6a gadak CLI startup (select 1)", lambda: cli(["sql", "--no-header", "select 1"]))

def sync_run(extra):
    t0 = time.perf_counter()
    r = subprocess.run([GADAK] + (["--profile", PROFILE] if PROFILE else []) + ["sync"] + extra,
                       capture_output=True, text=True, env=ENVP)
    dt = time.perf_counter() - t0
    if r.returncode != 0:
        raise RuntimeError(f"sync fail {extra}: {r.stderr[-200:]}")
    return dt, r.stdout

def sync_counts(out):
    """fetched/changed lines, whatever the sync flavor printed."""
    lines = [l.strip() for l in out.splitlines() if "fetched" in l]
    return " | ".join(lines) or "(no counts)"

# Where a tick's time actually goes — the tax split by source. On a quiet
# site these are the numbers behind the "watch tick" row in BENCHMARKS.md.
for src in ("jira", "confluence", "all"):
    runs = []
    note = ""
    for _ in range(2):
        dt, out = sync_run(["--source", src])
        runs.append(dt)
        note = sync_counts(out)
    R[f'sync_{src}'] = statistics.median(runs)
    print(f"{'6b gadak sync --source ' + src:46s} median {R[f'sync_{src}']*1000:9.0f} ms   "
          f"min {min(runs)*1000:8.0f}  max {max(runs)*1000:8.0f}  (n=2)  [{note}]")

# First full sync, wall clock, n=1 by definition. This is the row that used
# to read "minutes, size-dependent" — run it on a throwaway profile or one
# you are willing to re-fetch (GDK-94).
dt, out = sync_run(["--full"])
R['sync_full'] = dt
print(f"{'6c gadak sync --full (wall clock)':46s} {'':18s}{dt*1000:9.0f} ms   [{sync_counts(out)}]")

print("\n# summary ratios")
print(f"simple filter : REST {R['rest_filter']:.0f}ms vs sql {R['sql_filter']:.0f}ms  ({R['rest_filter']/R['sql_filter']:.0f}x)")
print(f"issue detail  : REST {R['rest_issue']:.0f}ms vs cli {R['cli_issue']:.0f}ms  ({R['rest_issue']/R['cli_issue']:.0f}x)")
print(f"text search   : REST {R['rest_text']:.0f}ms vs cli {R['cli_text']:.0f}ms  ({R['rest_text']/R['cli_text']:.0f}x)")
print(f"epic group by : REST {R['rest_group']:.0f}ms vs sql {R['sql_group']:.0f}ms  ({R['rest_group']/R['sql_group']:.0f}x)")
print(f"losing rows   : tick all={R['sync_all']*1000:.0f}ms (jira {R['sync_jira']*1000:.0f} / confluence {R['sync_confluence']*1000:.0f}); "
      f"first full sync {R['sync_full']/60:.1f} min ({ISSUES} issues); CLI startup {R['cli_startup']:.0f}ms per invocation")
