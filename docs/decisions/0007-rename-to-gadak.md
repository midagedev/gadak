# 0007 — Rename to gadak

Date: 2026-08-13

## Decision

The product, binary, home directory, env prefix, MCP tools, and Go module
are named **gadak** (가닥 — a strand you can follow).

## Why

`scry` collided with Scry AI (scryai.com, 2014–) and a crowded search
space (Scryfall, MTG, other CLIs). The products do not overlap; the name
did. gadak names the thing the tool does: pick up a thread across issues
and wiki pages.

## Compatibility

- `~/.scry` and `scry.db` are renamed on first launch.
- `SCRY_*` is read when the matching `GADAK_*` variable is unset.
- Team-share files still accept `scry_team_config` as the version key.
- GitHub (`midagedev/scry` → `midagedev/gadak`) and the Homebrew formula
  rename with the public repo.

## Consequences

Docs, the desktop bundle id (`com.midagedev.gadak`), and agent skills
follow the new name. There is no hosted account to migrate.

## Addendum (2026-09-10) — the compatibility has a sunset

**Status:** accepted (sunset audit, 2026-09-10).

The Compatibility section above shipped without an end date, and the
rename compatibility was still fully resident four releases and a month
later. That is now closed: **the legacy names are dropped in v0.22.0.**
(`internal/config.LegacySunsetRelease` is the constant; the enforcing
test `TestScryCompatSunsetEnforced` fails from the v0.22.0 changelog
heading until the drop lands.)

The audit that flagged this proposed "drop in 0.19 or 0.20"; both
shipped with the compat still in, so the sunset is the next minor after
v0.21. What goes when it fires (~120 lines):

- `internal/config/identity.go` — the `Legacy*` constants and
  `legacyEnv`.
- `internal/config/config.go` — the `SCRY_*` reads (`SCRY_PROFILE`,
  `SCRY_HOME`) and the `~/.scry` tree migration.
- `cmd/gadak/main.go` — the binary-name hint.
- `web/src/lib/storage.ts` — the `scry:` localStorage migrations
  (the web-side migration the release audit also flagged).

Two things deliberately stay: doctor's `home_leftover` (it reports that a
`~/.scry` directory exists on disk — true after the drop, and it reads
nothing), and team-share files accepting `scry_team_config` as a version
key (a stored-data format, not a code path — it costs one map entry and
breaking it would strand shared files).
