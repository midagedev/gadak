# Support

scry is pre-release and support is best effort.

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

## Before Opening an Issue

Include:

- scry commit or version
- OS, Go, and Node versions
- the command you ran
- expected and actual behavior
- whether your projects are team-managed or company-managed
- your Jira account display language, if the problem involves status or type
  names — these are localized per account and it is a common cause

Never paste real issue text, tokens, site URLs, or a database snapshot.
