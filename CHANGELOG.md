# Changelog

## Unreleased

- Extracted the web application from an internal deployment into this
  repository, replacing every hardcoded internal value with a runtime
  configuration document (`config.json`) fetched before mount.
- Generalized built-in views to axes that mean the same thing on every Jira site
  (`status_category`, `unassigned`, `reopened`, `stale`, `updated_from`),
  replacing presets that referenced internal project keys, status names, and team
  groupings.
- Replaced name-matching rules for resolution and reopen detection with status
  *category* rules, which are stable across sites and account languages. Dropped
  the internal `working_hours_in_status` field, which no code ever populated.
- Added `scry serve`: serves the built UI, the runtime config document, and
  `/healthz`. Refuses to bind a non-loopback address without `--allow-remote`,
  because the mirror has no authentication.
- Added `tools/seed-demo-jira.py` for populating a throwaway Jira site with
  releases, components, issues, transition history, comments, and links — either
  generated or projected from a dataset file.
- Specified the storage schema as a public contract, plus the HTTP, sync, and
  agent contracts under `specs/000-product/`.
