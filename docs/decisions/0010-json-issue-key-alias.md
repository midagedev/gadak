# 0010 — JSON surfaces carry both issue_key and key

Status: accepted
Date: 2026-08-22

## Context

Agents read a field from JSON and paste that name into SQL. The same value is
`issue_key` on JSON surfaces (`gadak search --json`, `gadak issue --json`,
`gadak create --json`'s `issue` object, the HTTP IssueLite / detail / feed,
MCP `gadak_issue`, static export) and `key` on `issues_full`. Measured
2026-08-18: `SELECT issue_key FROM issues_full` returns
`no such column: issue_key`, and a create `--json` document already mixed
`created.key` with `issue.issue_key`.

## Decision

JSON objects that emit `issue_key` also emit `key`, equal to `issue_key`.
`issue_key` stays (compat). `issues_full` does not grow an `issue_key`
column. `gadak sql` appends `did you mean "key"?` to a `no such column`
error when the unknown name is close enough; the suggestion is on the
error (stderr via main), never on stdout.

The alias is derived at marshal time (`store.MarshalWithIssueKeyAlias` /
`store.AliasIssueKey`) so a constructor cannot emit one name without the
other. `store.Detail` does not implement MarshalJSON: it is embedded
anonymously in the CLI `issueDoc`, and encoding/json then emits only the
embedded Marshaler and drops `issue` / `linked_prs`. That document uses
`issueDoc.MarshalJSON` instead. New JSON surfaces that carry the tracker
key must emit both.

## Rejected

Adding `issue_key` to `issues_full` would be a 0.x contract expansion
(data-model.md: the view plus RECIPES queries are one of the three
promised surfaces). Renaming or dropping `issue_key` on JSON would break
the web client, which stores IssueLite rows verbatim in IndexedDB.

## Consequences

- Adding a JSON field is backward compatible for the web client (contracts
  already say adding is safe; renaming is not).
- A future JSON surface that emits only `issue_key` is a defect; the helper
  and `TestIssueKeyStructTagsUseHelper` are the seal.
