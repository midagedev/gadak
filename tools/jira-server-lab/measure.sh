#!/usr/bin/env bash
# Measure the support matrix rows against a seeded Jira Server / Data Center
# lab workspace (GDK-1634). One numbered block per docs/SUPPORT_MATRIX.md row,
# in table order; each block runs the command that proves the row and writes
# `<out>/NN-<slug>.txt` holding the command, its exit code and its output.
# Reading the outputs and writing the cells is a person's job — this script
# only makes the evidence, the way tools/jira-server-lab/seed.sh makes the
# data it needs.
#
#   tools/jira-server-lab/measure.sh --workspace mtx --out /tmp/matrix [--gadak ./gadak]
#
# Write rows change the fixture (labels, a comment, a link, a created issue).
# Run seed.sh --reset before a clean re-measure.
set -uo pipefail
WS=""; OUT=""; G="${GADAK:-gadak}"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --workspace) WS="$2"; shift 2 ;;
    --out) OUT="$2"; shift 2 ;;
    --gadak) G="$2"; shift 2 ;;
    *) echo "measure: unknown flag $1" >&2; exit 64 ;;
  esac
done
[[ -n "$WS" && -n "$OUT" ]] || { echo "usage: measure.sh --workspace W --out DIR [--gadak BIN]" >&2; exit 64; }
mkdir -p "$OUT"
g() { "$G" --workspace "$WS" "$@"; }
n=0
row() { # row <slug> <<'CMD' ... CMD   (stdin is the snippet; $G/$WS are in scope)
  n=$((n+1)); local slug="$1" f
  f=$(printf '%s/%02d-%s.txt' "$OUT" "$n" "$slug")
  local snippet; snippet=$(cat)
  { printf '# row %02d %s\n$ %s\n\n' "$n" "$slug" "$snippet"; } > "$f"
  ( eval "$snippet" ) >> "$f" 2>&1; local rc=$?
  printf '\n[exit %d]\n' "$rc" >> "$f"
  printf '%02d %-34s exit %d\n' "$n" "$slug" "$rc"
}
export -f g 2>/dev/null || true

# Keys from the seed (MTX-1 epic … MTX-9 sub-task), by role, read back from the mirror.
EPIC=$(g sql --no-header "select key from issues_full where issue_type_id in (select issue_type_id from issues_full where hierarchy_level=1) and project_key='$(g sql --no-header "select project_key from issues_full limit 1")' order by key limit 1")
S1=MTX-2; S2=MTX-3; S3=MTX-4; S4=MTX-5; B1=MTX-6; B2=MTX-7; T1=MTX-8; ST1=MTX-9

row sync-status <<'CMD'
g status --json
CMD
row search <<'CMD'
g search coupon
CMD
row sql-recipes <<'CMD'
g sql "select key, status_category, priority_rank, issue_type_id, summary from issues_full where status_category != 'done' and labels != '[]' order by priority_rank, updated_at desc limit 8"
CMD
row jql <<'CMD'
g search --jql 'project = MTX AND statusCategory = "In Progress" ORDER BY key'
CMD
row comments <<'CMD'
g sql "select i.key, c.author, c.visibility, substr(c.body_text,1,50) body from comments c join issues i on i.item_id = c.item_id order by i.key"
CMD
row attachment-bytes <<'CMD'
g sql "select i.key, a.filename, a.size, a.mime_type from attachments a join issues i on i.item_id = a.item_id"
g issue MTX-2 --json | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d.get("attachments"))'
CMD
row attach-get <<'CMD'
g attach get MTX-2 repro.txt --out "$OUT" && cat "$OUT/repro.txt"
CMD
row history-derived <<'CMD'
g sql "select key, status_category, status_changed_at, reopen_count, reopen_reason, started_at, cycle_hours from issues_full where key in ('MTX-2','MTX-5','MTX-7') order by key"
g sql "select i.key, c.field, c.from_value, c.to_value, c.at from changelog c join issues i on i.item_id=c.item_id where i.key='MTX-7' order by c.at"
CMD
row issue-links <<'CMD'
g sql "select i.key, l.link_type, l.direction, l.other_key from links l join issues i on i.item_id = l.item_id order by i.key"
CMD
row ready-open-blockers <<'CMD'
g sql "select key, open_blockers from issues_full where key in ('MTX-3','MTX-6')"
g ready --limit 5
CMD
row remote-links-ref <<'CMD'
g ref MTX-2 scr/SCR-1
CMD
row dev-links <<'CMD'
g sql "select count(*) from dev_links"
g dev link MTX-2 --pr https://github.com/example/app/pull/7
CMD
row labels <<'CMD'
g sql "select key, labels from issues_full where labels != '[]' and json_array_length(labels) > 0 and key like 'MTX-%' and labels not like '%bulk%' order by key"
CMD
row components <<'CMD'
g sql "select key, components from issues_full where components != '[]' order by key"
CMD
row fix-versions-catalog <<'CMD'
g sql "select id, name, released, release_date from versions order by name"
g sql "select key, fix_versions, fix_version_ids from issues_full where fix_versions != '[]' order by key"
CMD
row sprint-columns <<'CMD'
g sql "select key, sprint_id, sprint_name, sprint_state from issues_full where sprint_id is not null order by key"
CMD
row boards-sprints-rows <<'CMD'
g sql "select id, name, type, project_key from boards order by id"
g sprint list
CMD
row sprint-writes <<'CMD'
SPD=$(g sprint create --board 2 "Sprint D" --json | python3 -c 'import sys,json; print(json.load(sys.stdin)["sprint"]["id"])')
echo "created sprint $SPD"
g sprint add "$SPD" MTX-8
g sql "select key, sprint_id, sprint_state from issues_full where key='MTX-8'"
g sprint remove MTX-8
g sql "select key, sprint_id, sprint_state from issues_full where key='MTX-8'"
g sprint list
CMD
row custom-fields <<'CMD'
g fields --apply
g sync
g sql "select key, json_extract(custom, '$.story_points') sp, json_extract(custom, '$.epic_name') epic_name from issues_full where json_extract(custom, '$.story_points') is not null or json_extract(custom, '$.epic_name') is not null order by key"
CMD
row issue-type <<'CMD'
g sql "select issue_type_id, issue_type, hierarchy_level, count(*) from issues_full group by 1,2,3 order by 3 desc"
CMD
row hierarchy <<'CMD'
g sql "select key, issue_type, hierarchy_level, parent_key, epic_key from issues_full where key in ('MTX-1','MTX-2','MTX-9','MTX-6') order by key"
CMD
row wiki-pages <<'CMD'
g page list --limit 3
g sql "select count(*) from pages"
CMD
row origin-url <<'CMD'
g sql "select key, url from issues_full where key='MTX-2'"
g issue MTX-2 --json | python3 -c 'import sys,json; print(json.load(sys.stdin)["url"])'
CMD
row view-link <<'CMD'
g views open --jql 'project = MTX AND statusCategory != Done' --no-open --json
CMD
row create-issue <<'CMD'
g create "Measured from the lab" --project MTX --type Bug --priority High --label matrix -m "Created by tools/jira-server-lab/measure.sh — *wiki* markup body." --json
CMD
row comment-visibility <<'CMD'
g comment MTX-6 -m "measure: plain comment"
g comment MTX-6 -m "measure: admins only" --visibility role=Administrators
g sql "select c.author, c.visibility, substr(c.body_text,1,40) from comments c join issues i on i.item_id=c.item_id where i.key='MTX-6'"
CMD
row transition-screen-fields <<'CMD'
g transition MTX-6
g transition MTX-6 done --resolution "Won't Do" --json
g sql "select key, status_category, resolution from issues_full where key='MTX-6'"
CMD
row assign-unassign <<'CMD'
g assign MTX-8 dana --json | python3 -c 'import sys,json; d=json.load(sys.stdin)["issue"]; print(d["assignee"], d["assignee_id"])'
g assign MTX-8 - --json | python3 -c 'import sys,json; d=json.load(sys.stdin)["issue"]; print(d["assignee"], d["assignee_id"])'
CMD
row label-edits <<'CMD'
g edit MTX-8 --label +triaged --json | python3 -c 'import sys,json; print(json.load(sys.stdin)["issue"]["labels"])'
g edit MTX-8 --label -triaged --json | python3 -c 'import sys,json; print(json.load(sys.stdin)["issue"]["labels"])'
CMD
row component-edits <<'CMD'
g edit MTX-8 --component +core --json | python3 -c 'import sys,json; print(json.load(sys.stdin)["issue"]["components"])'
g edit MTX-8 --component -core --json | python3 -c 'import sys,json; print(json.load(sys.stdin)["issue"]["components"])'
CMD
row priority-edit <<'CMD'
g edit MTX-8 --priority Low --json | python3 -c 'import sys,json; d=json.load(sys.stdin)["issue"]; print(d["priority"], d["priority_id"])'
CMD
row due-set <<'CMD'
g edit MTX-8 --due 2026-10-01 --json | python3 -c 'import sys,json; print(json.load(sys.stdin)["issue"]["duedate"])'
CMD
row due-clear <<'CMD'
g edit MTX-8 --due none --json | python3 -c 'import sys,json; print(json.load(sys.stdin)["issue"]["duedate"])'
CMD
row summary-description-edit <<'CMD'
g edit MTX-8 --summary "Rotate the payment gateway key (measured)" -m "h2. Steps
* rotate
* verify" --json | python3 -c 'import sys,json; print(json.load(sys.stdin)["issue"]["summary"])'
g issue MTX-8 | sed -n '/^description/,/^$/p'
CMD
row custom-field-edit <<'CMD'
g edit MTX-8 --field story_points=5 --json | python3 -c 'import sys,json; print(json.load(sys.stdin)["issue"]["custom"])'
CMD
row type-edit <<'CMD'
g edit MTX-8 --type Story --json | python3 -c 'import sys,json; d=json.load(sys.stdin)["issue"]; print(d["issue_type"], d["issue_type_id"])'
g edit MTX-8 --type Task --json | python3 -c 'import sys,json; d=json.load(sys.stdin)["issue"]; print(d["issue_type"], d["issue_type_id"])'
CMD
row parent-set-clear <<'CMD'
g edit MTX-8 --parent MTX-1 --json | python3 -c 'import sys,json; d=json.load(sys.stdin)["issue"]; print(d["parent_key"], d["epic_key"])'
g edit MTX-8 --parent none --json | python3 -c 'import sys,json; d=json.load(sys.stdin)["issue"]; print(d["parent_key"], d["epic_key"])'
CMD
row attachment-upload <<'CMD'
printf 'uploaded by measure.sh\n' > "$OUT/measure-upload.txt"
g attach MTX-8 "$OUT/measure-upload.txt" --json
CMD
row link-unlink <<'CMD'
g link MTX-8 MTX-4 --type blocks --json
g sql "select i.key, l.link_type, l.direction, l.other_key from links l join issues i on i.item_id = l.item_id where i.key='MTX-8'"
g unlink MTX-8 MTX-4 --type blocks --json
CMD
row wiki-write <<'CMD'
g page create --space LAB --title "measure" -m "body"
CMD
row claim <<'CMD'
g claim MTX-4 --json
g sql "select key, status_category, assignee_id from issues_full where key='MTX-4'"
CMD
row worklog-api-write <<'CMD'
g api POST /rest/api/2/issue/MTX-4/worklog --write --data '{"timeSpent":"30m","comment":"measure"}'
g api GET /rest/api/2/issue/MTX-4/worklog | python3 -c 'import sys,json; print([w["timeSpent"] for w in json.load(sys.stdin)["worklogs"]])'
CMD
row migrate-from <<'CMD'
"$G" --workspace mtxcopy migrate --from "$WS" --projects MTX --skip-attachments --json
"$G" --workspace mtxcopy sql "select count(*) from issues_full"
CMD
row migrate-to <<'CMD'
"$G" --workspace "$WS" migrate --from mtxcopy --json
CMD
row agent-surfaces <<'CMD'
g sql --json "select key from issues_full limit 1"
g doctor --json | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d["workspace"]["kind"], d.get("workspace_kind"))'
CMD
row board-layout <<'CMD'
g views save "Lab board" --jql 'project = MTX AND sprint in openSprints()' --layout board --json
g views show "Lab board"
CMD
row views-open-keys <<'CMD'
g sql --no-header "select key from issues_full where sprint_state='active' order by key" | g views open --keys - --no-open --json
CMD
row watch-feed <<'CMD'
timeout 25 "$G" --workspace "$WS" watch --interval 5s 2>&1 &
sleep 7
g comment MTX-4 -m "measure: watch tick $(date +%s)" >/dev/null
wait
CMD
row in-process-origin <<'CMD'
g doctor --json | python3 -c 'import sys,json; d=json.load(sys.stdin); print("kind:", d["workspace"]["kind"], "site:", d["workspace"].get("site"))'
CMD
echo "measure: $n rows → $OUT"
