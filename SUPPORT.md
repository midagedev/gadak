# Support

scry is pre-release and support is best effort.

## Before Opening an Issue

1. **Paste `scry doctor` output.** Run `scry doctor` (or
   `scry --profile <name> doctor`) and attach the full text. It is safe to put
   on a public tracker: counts and versions only — no API tokens, site
   hostnames, emails, project keys, custom-field names, or raw error strings.
2. scry commit or version (`scry version`)
3. OS and architecture
4. the command you ran, plus expected and actual behavior
5. whether your projects are team-managed or company-managed
6. your Jira account display language, if the problem involves status or type
   names — these are localized per account and it is a common cause

Never paste real issue text, tokens, site URLs, or a database snapshot.
`scry doctor` is the supported way to describe the environment without those.

## Read First

`specs/000-product/tasks.md` lists what is implemented. Most "it does not work"
reports during this phase are features that are specified but not built yet.

## Use GitHub Issues For

- reproducible bugs in sync, the API, the UI, or write-through
- documentation errors
- feature requests that fit a local, single-user mirror
- schema or contract problems

## Use the Security Process For

Anything in `SECURITY.md`: credential exposure, the media-URL allowlist, the
loopback guard, or HTML injection through issue content.

## Out of Scope

- Jira administration, workflow configuration, or permission schemes
- boards, sprints, reports, and automation
- Jira Server / Data Center (Cloud only until someone can test DC)
- multi-user or hosted deployment
- anything that would require a scry service to exist
