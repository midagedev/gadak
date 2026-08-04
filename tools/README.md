# tools

## `seed-demo-jira.py`

Populates a throwaway Jira Cloud site with a realistic backlog: releases,
components, issues, real transition history, comments, and issue links.

Used to produce the data behind scry's screenshots and `examples/demo.db`, and
reusable by anyone who wants a demo Jira to point scry at.

```bash
export JIRA_SITE=https://your-site.atlassian.net
export JIRA_EMAIL=you@example.com
export JIRA_TOKEN=...    # id.atlassian.com/manage-profile/security/api-tokens

# Project it from the committed dataset (reproducible)
python3 tools/seed-demo-jira.py --data examples/demo-seed.json --projects NMB,NMA,NMS

# Or generate content procedurally (quick, more repetitive)
python3 tools/seed-demo-jira.py --projects NMB,NMA,NMS --issues 300
```

Flags: `--dry-run`, `--skip-setup` (versions and components already exist),
`--no-history` (skip transitions, comments, links), `--assignees <id,id>`,
`--seed <int>`.

Repair a site whose workflow states do not match the dataset — issues, comments,
and links are left alone and only statuses are re-driven, matched by summary:

```bash
python3 tools/seed-demo-jira.py --data examples/demo-seed.json --repair-states
```

### Requirements

- Projects must exist and be **company-managed**. Team-managed projects do not
  expose `priority`, `components`, or `fixVersions`, which leaves most of the
  UI's filter axes empty.
- A user API token. Organization API keys from `admin.atlassian.com` (prefix
  `ATCTT`) authenticate only against organization admin APIs and will 401 against
  every product endpoint.

### Gotchas encoded in the script

- `issue/createmeta` translates issue-type names into the account's display
  language and ignores `Accept-Language`, so name matching breaks on non-English
  accounts. The script reads `project/{key}/statuses`, which is not localized.
- Jira assigns `created` at insert time with no way to backdate, so seeded issues
  share roughly one creation time. Realistic time spread is applied by
  `scry snapshot`, not here.
- Deleting issues needs the "Delete Issues" permission, which the default
  company-managed scheme does not grant. Plan runs so you do not need to undo them.
- Default workflows offer a direct `Backlog -> Done` edge. Taking it leaves a
  single changelog entry, which makes derived fields correct but the history
  timeline empty — so the script walks the status *category* ladder one rung at a
  time (`new -> indeterminate -> done`) instead, and only falls back to a direct
  jump when no stepwise path exists.
- A `reopened` issue is driven to done and then back, because that is the only
  way to get a real done-to-not-done transition into the changelog. History
  cannot be backfilled after the fact: pushing an already-done issue backwards
  later would register as a reopen it was never supposed to have.

## `examples/demo-seed.json`

The dataset the seeder projects onto Jira. Every issue has a unique summary and
hand-authored body and comments; nothing is templated, and nothing derives from a
real backlog.

```json
{
  "issues": [
    {
      "project": "NMB", "type": "Bug",
      "summary": "...", "description": ["...", "..."],
      "priority": "High", "components": ["Dashboard"], "fix_version": "2026.9.0",
      "labels": ["regression"], "environment": "Chrome 141 / macOS 15.2",
      "state": "inprogress", "reopened": false, "assignee_slot": 1,
      "comments": ["..."], "links": [{"type": "Relates", "target": 42}]
    }
  ]
}
```

- `state` is one of `backlog`, `selected`, `inprogress`, `done`; the seeder walks
  the real workflow to reach it, so the changelog is genuine.
- `reopened: true` makes the seeder push the issue to done and then back, which is
  what produces a real reopen in the changelog.
- `links[].target` is an index into the same array.
- `assignee_slot` is mapped to whatever accounts `--assignees` provides.
