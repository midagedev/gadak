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
