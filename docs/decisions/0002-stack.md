# 0002 — Stack: Go + SQLite, Svelte client

Status: accepted
Date: 2026-08-04

## Decision

Go for the binary, SQLite (FTS5) for the mirror, Svelte 5 + Vite + Tailwind for
the client — the last inherited from the extraction.

## Why Go

Single static binary, no runtime to install, trivial cross-compilation. The
alternative that fit the ecosystem better was Python (the Jira tooling is there),
but shipping a Python tool means shipping an environment problem, and the whole
premise is "download and run".

## Why SQLite

It is simultaneously the storage engine and the agent interface. Any other store
would need an API layer in front of it for agents to use, which is precisely the
thing this design avoids (`specs/000-product/contracts/agent.md`). FTS5 covers
full-text search with no extra dependency.

Pure-Go drivers (`modernc.org/sqlite`) keep `CGO_ENABLED=0`, preserving the
single-binary property. If FTS5 or performance forces `mattn/go-sqlite3`, the
cross-compilation cost gets paid then, deliberately.

## Why keep Svelte

It came with the client. Rewriting a working 12.5k-line application to satisfy a
framework preference would be pure loss.
