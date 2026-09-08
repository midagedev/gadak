#!/usr/bin/env bash
# Seed a Jira Software (Server / Data Center) lab instance with the data the
# support matrix needs to be measured row by row (GDK-1634, GDK-1641).
#
# One scrum project — key MTX by default — carrying every shape a matrix row
# asks about: components, versions, labels, priorities, due dates, story
# points, an epic with epic links, a sub-task, a blocking link, a remote
# link, an attachment, a role-restricted comment, a worklog, transitions
# through done and back (a reopen), and three sprints in the three states.
# `--bulk N` adds N plain issues on top, 50 per request, for the rate-limit
# measurement (GDK-1646).
#
# Runs against the instance docs/runbooks/jira-server-lab.md brings up.
# Credentials never enter the repo: the PAT comes from the environment.
#
#   JIRA_LAB_URL=http://localhost:8088 JIRA_LAB_PAT=… tools/jira-server-lab/seed.sh [--key MTX] [--bulk 400]
#
# Idempotency is deliberate and blunt: the script refuses when the project
# exists. Delete it in Jira (or pick another --key) and run again — a lab
# fixture is rebuilt, not patched.
set -euo pipefail

URL="${JIRA_LAB_URL:-http://localhost:8088}"
PAT="${JIRA_LAB_PAT:-}"
KEY="MTX"
BULK=0
RESET=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --key) KEY="$2"; shift 2 ;;
    --bulk) BULK="$2"; shift 2 ;;
    --reset) RESET=1; shift ;;   # delete the project first (lab only — every issue in it goes)
    -h|--help) sed -n '2,20p' "$0"; exit 0 ;;
    *) echo "seed: unknown flag $1" >&2; exit 64 ;;
  esac
done
if [[ -z "$PAT" ]]; then
  echo "seed: JIRA_LAB_PAT is required (a Personal Access Token of a Jira admin; see the runbook)" >&2
  exit 64
fi

api() { # api METHOD PATH [JSON]
  local m="$1" p="$2" d="${3:-}"
  if [[ -n "$d" ]]; then
    curl -sS -H "Authorization: Bearer $PAT" -H 'Content-Type: application/json' -X "$m" "$URL$p" -d "$d"
  else
    curl -sS -H "Authorization: Bearer $PAT" -H 'Content-Type: application/json' -X "$m" "$URL$p"
  fi
}
jget() { python3 -c 'import sys,json; d=json.load(sys.stdin); print(eval(sys.argv[1]))' "$1"; }
say() { printf '%s\n' "$*" >&2; }

# ── preconditions ────────────────────────────────────────────────────────
info=$(api GET /rest/api/2/serverInfo)
dep=$(printf '%s' "$info" | jget 'd.get("deploymentType","")') || { say "seed: $URL did not answer serverInfo"; exit 1; }
say "seed: $URL is Jira ${dep} $(printf '%s' "$info" | jget 'd["version"]')"
if [[ "$dep" != "Server" ]]; then
  say "seed: this script seeds a Server / Data Center instance, not $dep"; exit 1
fi
if [[ "$RESET" == 1 ]] && api GET "/rest/api/2/project/$KEY" | grep -q '"key"'; then
  api DELETE "/rest/api/2/project/$KEY" >/dev/null; say "seed: --reset deleted project $KEY"
fi
if api GET "/rest/api/2/project/$KEY" | grep -q '"key"'; then
  say "seed: project $KEY already exists — delete it in Jira or pass --key OTHER; a lab fixture is rebuilt, not patched"
  exit 1
fi
me=$(api GET /rest/api/2/myself | jget 'd["name"]')

# Field ids are per instance: resolve the ones we set by their schema key.
fields=$(api GET /rest/api/2/field)
fid() { printf '%s' "$fields" | jget "next(f['id'] for f in d if f.get('schema',{}).get('custom','').endswith('$1'))"; }
EPIC_NAME=$(fid gh-epic-label)
EPIC_LINK=$(fid gh-epic-link)
STORY_POINTS=$(printf '%s' "$fields" | jget "next((f['id'] for f in d if f['name']=='Story Points'),'')")
say "seed: Epic Name=$EPIC_NAME Epic Link=$EPIC_LINK Story Points=${STORY_POINTS:-none}"

# ── project (scrum template → a board comes with it) ─────────────────────
api POST /rest/api/2/project "{\"key\":\"$KEY\",\"name\":\"Matrix Lab\",\"projectTypeKey\":\"software\",
  \"projectTemplateKey\":\"com.pyxis.greenhopper.jira:gh-scrum-template\",\"lead\":\"$me\",
  \"description\":\"Seeded by tools/jira-server-lab/seed.sh for the support matrix\"}" >/dev/null
say "seed: project $KEY created"
proj=$(api GET "/rest/api/2/project/$KEY")
tid() { printf '%s' "$proj" | jget "next(t['id'] for t in d['issueTypes'] if t['name']=='$1')"; }
T_EPIC=$(tid Epic); T_STORY=$(tid Story); T_BUG=$(tid Bug); T_TASK=$(tid Task); T_SUB=$(tid Sub-task)

# ── screens: the scrum template's screens carry neither Due Date nor Story
# Points, and Server refuses a field that is not on the screen ("cannot be
# set. It is not on the appropriate screen") — so put both on every screen
# the template made for this project. This is also what makes the matrix's
# custom-field and due-date rows measurable here.
screens=$(api GET /rest/api/2/screens)
for sid in $(printf '%s' "$screens" | jget "' '.join(str(x['id']) for x in (d if isinstance(d,list) else d['values']) if x['name'].startswith('$KEY:'))"); do
  tab=$(api GET "/rest/api/2/screens/$sid/tabs" | jget 'd[0]["id"]')
  for f in duedate ${STORY_POINTS:-}; do
    api POST "/rest/api/2/screens/$sid/tabs/$tab/fields" "{\"fieldId\":\"$f\"}" >/dev/null || true
  done
done
say "seed: duedate${STORY_POINTS:+ and $STORY_POINTS} added to the $KEY screens"

# ── a second person, for assign / reporter / a foreign comment author ────
if ! api GET '/rest/api/2/user?username=dana' | grep -q '"name"'; then
  api POST /rest/api/2/user '{"name":"dana","password":"dana-lab-2026","emailAddress":"dana@example.com","displayName":"Dana Kim"}' >/dev/null
  say "seed: user dana created"
fi

# ── catalogs: components, versions ──────────────────────────────────────
for c in core ui; do
  api POST /rest/api/2/component "{\"name\":\"$c\",\"project\":\"$KEY\"}" >/dev/null
done
V10=$(api POST /rest/api/2/version "{\"name\":\"v1.0\",\"project\":\"$KEY\",\"released\":true,\"releaseDate\":\"2026-08-01\"}" | jget 'd["id"]')
V11=$(api POST /rest/api/2/version "{\"name\":\"v1.1\",\"project\":\"$KEY\",\"released\":false}" | jget 'd["id"]')
say "seed: components core, ui; versions v1.0 ($V10, released), v1.1 ($V11)"

# ── issues ───────────────────────────────────────────────────────────────
create() { # create JSON-fields → key; the origin's answer on stderr when it is not a key
  local out
  out=$(api POST /rest/api/2/issue "{\"fields\":$1}")
  if ! printf '%s' "$out" | grep -q '"key"'; then
    say "seed: create failed — fields: $1"; say "seed: origin answered: ${out:-<empty>}"; exit 1
  fi
  printf '%s' "$out" | jget 'd["key"]'
}
sp() { [[ -n "$STORY_POINTS" ]] && printf ',"%s":%s' "$STORY_POINTS" "$1" || true; }

EPIC=$(create "{\"project\":{\"key\":\"$KEY\"},\"issuetype\":{\"id\":\"$T_EPIC\"},\"summary\":\"Checkout flow\",\"$EPIC_NAME\":\"Checkout\",\"description\":\"h2. Goal\\n\\nThe *checkout* epic — wiki markup, byte for byte.\",\"labels\":[\"epic\"]}")
S1=$(create "{\"project\":{\"key\":\"$KEY\"},\"issuetype\":{\"id\":\"$T_STORY\"},\"summary\":\"Cart survives a reload\",\"$EPIC_LINK\":\"$EPIC\",\"priority\":{\"name\":\"High\"},\"labels\":[\"cart\",\"frontend\"],\"components\":[{\"name\":\"ui\"}],\"fixVersions\":[{\"id\":\"$V11\"}],\"duedate\":\"2026-09-30\",\"assignee\":{\"name\":\"$me\"},\"description\":\"As a shopper I want my cart to survive a reload.\\n\\n* localStorage first\\n* server cart when signed in\"$(sp 3)}")
S2=$(create "{\"project\":{\"key\":\"$KEY\"},\"issuetype\":{\"id\":\"$T_STORY\"},\"summary\":\"Apply a coupon at checkout\",\"$EPIC_LINK\":\"$EPIC\",\"priority\":{\"name\":\"Medium\"},\"labels\":[\"checkout\"],\"components\":[{\"name\":\"core\"},{\"name\":\"ui\"}],\"assignee\":{\"name\":\"dana\"}$(sp 5)}")
S3=$(create "{\"project\":{\"key\":\"$KEY\"},\"issuetype\":{\"id\":\"$T_STORY\"},\"summary\":\"Order confirmation email\",\"$EPIC_LINK\":\"$EPIC\",\"priority\":{\"name\":\"Low\"},\"components\":[{\"name\":\"core\"}],\"fixVersions\":[{\"id\":\"$V11\"}]$(sp 2)}")
S4=$(create "{\"project\":{\"key\":\"$KEY\"},\"issuetype\":{\"id\":\"$T_STORY\"},\"summary\":\"Address autocomplete\",\"$EPIC_LINK\":\"$EPIC\",\"fixVersions\":[{\"id\":\"$V10\"}],\"assignee\":{\"name\":\"dana\"}$(sp 8)}")
B1=$(create "{\"project\":{\"key\":\"$KEY\"},\"issuetype\":{\"id\":\"$T_BUG\"},\"summary\":\"Coupon code field accepts whitespace\",\"priority\":{\"name\":\"Highest\"},\"labels\":[\"checkout\",\"regression\"],\"components\":[{\"name\":\"ui\"}],\"duedate\":\"2026-09-15\"}")
B2=$(create "{\"project\":{\"key\":\"$KEY\"},\"issuetype\":{\"id\":\"$T_BUG\"},\"summary\":\"Confirmation email sent twice\",\"priority\":{\"name\":\"High\"},\"components\":[{\"name\":\"core\"}],\"fixVersions\":[{\"id\":\"$V10\"}]}")
T1=$(create "{\"project\":{\"key\":\"$KEY\"},\"issuetype\":{\"id\":\"$T_TASK\"},\"summary\":\"Rotate the payment gateway key\",\"labels\":[\"ops\"]}")
ST1=$(create "{\"project\":{\"key\":\"$KEY\"},\"issuetype\":{\"id\":\"$T_SUB\"},\"parent\":{\"key\":\"$S1\"},\"summary\":\"Write the localStorage adapter\",\"assignee\":{\"name\":\"$me\"}}")
say "seed: epic $EPIC; stories $S1 $S2 $S3 $S4; bugs $B1 $B2; task $T1; sub-task $ST1"

# ── relations: a blocking link, a remote link ────────────────────────────
# Measured on 11.3.11: the issue named inwardIssue is the one whose view says
# "<outward> <the other>" — inwardIssue=B1, outwardIssue=S2 reads "B1 blocks S2".
api POST /rest/api/2/issueLink "{\"type\":{\"name\":\"Blocks\"},\"inwardIssue\":{\"key\":\"$B1\"},\"outwardIssue\":{\"key\":\"$S2\"}}" >/dev/null
api POST "/rest/api/2/issue/$S1/remotelink" '{"object":{"url":"https://example.com/design/cart","title":"Cart design notes"}}' >/dev/null
say "seed: $B1 blocks $S2; remote link on $S1"

# ── attachment (Server wants the no-check header) ────────────────────────
tmp=$(mktemp); printf 'cart reload — repro steps\n1. add item\n2. reload\n' > "$tmp"
curl -sS -H "Authorization: Bearer $PAT" -H 'X-Atlassian-Token: no-check' -F "file=@$tmp;filename=repro.txt" "$URL/rest/api/2/issue/$S1/attachments" >/dev/null
rm -f "$tmp"
say "seed: attachment repro.txt on $S1"

# ── comments: plain, and one only Administrators can read ───────────────
api POST "/rest/api/2/issue/$S1/comment" '{"body":"Reproduced on staging — the cart is empty after F5."}' >/dev/null
api POST "/rest/api/2/issue/$S2/comment" '{"body":"Internal: the coupon service key is in the vault.","visibility":{"type":"role","value":"Administrators"}}' >/dev/null
api POST "/rest/api/2/issue/$S4/comment" '{"body":"done — shipped in v1.0"}' >/dev/null
say "seed: comments on $S1 (plain), $S2 (role Administrators), $S4 (done-word)"

# ── worklog ──────────────────────────────────────────────────────────────
api POST "/rest/api/2/issue/$S3/worklog" '{"timeSpent":"1h 30m","comment":"template pass","started":"2026-09-08T10:00:00.000+0000"}' >/dev/null
say "seed: worklog 1h 30m on $S3"

# ── transitions: through done (with a resolution), and one reopen ────────
move() { # move KEY category(new|indeterminate|done)
  # No resolution in the body: the scrum template's Done transition has no
  # screen, so Server 400s a resolution field ("not on the appropriate
  # screen") and sets Resolution=Done itself by post-function.
  local key="$1" cat="$2" tid out
  tid=$(api GET "/rest/api/2/issue/$key/transitions" | jget "next(t['id'] for t in d['transitions'] if t['to']['statusCategory']['key']=='$cat')")
  out=$(api POST "/rest/api/2/issue/$key/transitions" "{\"transition\":{\"id\":\"$tid\"}}")
  [[ -z "$out" ]] || { say "seed: transition $key → $cat failed: $out"; exit 1; }
}
move "$S1" indeterminate
move "$S4" indeterminate; move "$S4" done
move "$B2" indeterminate; move "$B2" done; move "$B2" new    # a reopen
move "$ST1" indeterminate
say "seed: $S1 in progress; $S4 done; $B2 done then reopened; $ST1 in progress"

# ── sprints: closed / active / future on the board the template made ────
BOARD=$(api GET "/rest/agile/1.0/board?projectKeyOrId=$KEY" | jget 'd["values"][0]["id"]')
sprint() { api POST /rest/agile/1.0/sprint "{\"name\":\"$1\",\"originBoardId\":$BOARD,\"goal\":\"$2\"}" | jget 'd["id"]'; }
SPA=$(sprint "Sprint A" "Ship v1.0")
SPB=$(sprint "Sprint B" "Cart and coupons")
SPC=$(sprint "Sprint C" "Confirmation")
api POST "/rest/agile/1.0/sprint/$SPA/issue" "{\"issues\":[\"$S4\",\"$B2\"]}" >/dev/null
api POST "/rest/agile/1.0/sprint/$SPB/issue" "{\"issues\":[\"$S1\",\"$S2\",\"$B1\"]}" >/dev/null
api POST "/rest/agile/1.0/sprint/$SPC/issue" "{\"issues\":[\"$S3\",\"$T1\"]}" >/dev/null
now=$(date -u +%Y-%m-%dT%H:%M:%S.000Z)
in14=$(date -u -v+14d +%Y-%m-%dT%H:%M:%S.000Z 2>/dev/null || date -u -d '+14 days' +%Y-%m-%dT%H:%M:%S.000Z)
api POST "/rest/agile/1.0/sprint/$SPA" "{\"state\":\"active\",\"startDate\":\"$now\",\"endDate\":\"$in14\"}" >/dev/null
api POST "/rest/agile/1.0/sprint/$SPA" '{"state":"closed"}' >/dev/null
api POST "/rest/agile/1.0/sprint/$SPB" "{\"state\":\"active\",\"startDate\":\"$now\",\"endDate\":\"$in14\"}" >/dev/null
say "seed: board $BOARD — Sprint A ($SPA) closed, Sprint B ($SPB) active, Sprint C ($SPC) future"

# ── bulk: plain issues for the rate-limit measurement ────────────────────
if [[ "$BULK" -gt 0 ]]; then
  made=0
  while [[ $made -lt $BULK ]]; do
    n=$(( BULK - made )); [[ $n -gt 50 ]] && n=50
    body=$(python3 -c "import json,sys; k,t,s,n=sys.argv[1:]; s=int(s); print(json.dumps({'issueUpdates':[{'fields':{'project':{'key':k},'issuetype':{'id':t},'summary':f'Bulk issue {s+i+1}','labels':['bulk']}} for i in range(int(n))]}))" "$KEY" "$T_TASK" "$made" "$n")
    api POST /rest/api/2/issue/bulk "$body" >/dev/null
    made=$(( made + n ))
  done
  say "seed: $BULK bulk issues"
fi

printf 'project\t%s\nboard\t%s\nepic\t%s\nstories\t%s %s %s %s\nbugs\t%s %s\ntask\t%s\nsubtask\t%s\nsprints\t%s(closed) %s(active) %s(future)\n' \
  "$KEY" "$BOARD" "$EPIC" "$S1" "$S2" "$S3" "$S4" "$B1" "$B2" "$T1" "$ST1" "$SPA" "$SPB" "$SPC"
