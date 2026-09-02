# testdata — scrubbed live captures

Each file is a real GraphQL response from the Linear API (captured 2026-08-18,
personal API key, with the exact query constants this package ships), scrubbed
so it carries no person- or workspace-identifying value. Shape is real;
identity is not.

| File | Captured with | What it exercises |
| --- | --- | --- |
| `viewer.json` | `queryViewer` | user parse, credential-check reply shape |
| `teams.json` | `queryTeams` | team list, connection envelope |
| `workflowstates.json` | `queryWorkflowStates` (team filter) | the six state `type` values incl. `duplicate` |
| `issues_page1.json` | `queryIssues` (`first: 2`) | issue parse; `hasNextPage: true` + real cursor |
| `issues_page2.json` | `queryIssues` (`after:` page 1's cursor) | cursor continuation, final page |
| `issue_create.json` | — hand-built (GDK-360) | `issueCreate` success + returned issue parse |
| `issue_update.json` | — hand-built (GDK-360) | `issueUpdate` success + updated issue parse |
| `comment_create.json` | — hand-built (GDK-360) | `commentCreate` success + comment parse |
| `relation_create.json` | — hand-built (GDK-1265) | `issueRelationCreate` success |
| `label_create.json` | — hand-built (GDK-1265) | `issueLabelCreate` success + label parse |
| `labels.json` | — hand-built (GDK-1265) | `issueLabels` catalog page |

The three mutation fixtures are **not captures**: no live mutation was
allowed against the capture workspace. They are hand-built to the documented
mutation payload shape (`success` + the entity, the same node selections the
package's query documents request), reusing the scrub vocabulary above so
ids agree across files — the team uuid is `…003`, the Todo state `…002`,
In Progress `…008`, the viewer `…011`, and the new rows continue the counter
(`…012` issue, `…013`/`…014` labels, `…015` assignee, `…016` comment).

## Scrub rules

The captures were piped through a one-shot scrubber that walks the JSON and
rewrites, with a value-keyed map shared across all files (so the same original
always becomes the same fake — the team uuid in `teams.json` and inside
`issues_page*.json` agree):

- every UUID-shaped string → `00000000-0000-4000-8000-<12-digit counter>`
- user `name` / `displayName` / `email` → `Fixture User <n>` / `Fix User <n>`
  / `user-<n>@example.invalid`
- team `name` / `key` → `Fixture Team` / `FIX`
- issue `identifier` `MID-<n>` → `FIX-<n>` (numbers kept)
- issue `url` → `https://linear.app/example/issue/FIX-<n>/scrubbed`
  (the real org slug and title slug are dropped)
- issue `title` → `Fixture issue <n>`
- issue `description` and comment `body` → a synthetic markdown block with the
  same structural shape (heading, checklist, link, fenced code)
- workflow-state `name` → Linear's default vocabulary for its `type`
  (`backlog`→Backlog, `unstarted`→Todo, `started`→In Progress,
  `completed`→Done, `canceled`→Canceled, `duplicate`→Duplicate) — this
  workspace already used the defaults, so those specific strings round-trip

Kept verbatim because they identify no one and the fixtures need them real:
timestamps (ISO-8601 UTC with ms), `priority`/`priorityLabel` vocabulary,
`position`, booleans, `number`, connection cursors-as-fakes.

The scrubber refuses to write a file if any rewritten original survives in the
output text (substring scan over the final bytes, exempting the known-safe
Linear default vocabulary above); the run that produced these files ended with
`40 originals replaced and verified absent`.

No raw capture ever entered the repository — scrubbing happened outside the
tree and only the output was written here.
