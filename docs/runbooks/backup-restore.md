# Backing up the built-in tracker — and putting it back

On a workspace whose origin is the built-in tracker, the record is one file:
`origin/issuetap.db` in the workspace directory (`~/.gadak/` or
`$GADAK_HOME`, or `~/.gadak/profiles/<name>/`). `gadak.db` next to it is a
cache — the next sync rebuilds it — so it is not what you back up. Lose the
persist file and everything written since the workspace was created is gone;
a connected workspace has no such file, because Atlassian or Linear holds the
record.

## Take a backup

```bash
gadak --workspace <name> backup --to /srv/backups
```

Prints the path of one self-contained SQLite file
(`issuetap-<workspace>-<UTC stamp>.db`). It is an online copy (`VACUUM INTO`
under a read transaction), so leave `gadak serve` running — the copy includes
what is still in the `-wal` sidecar and has no sidecar of its own. `--to` may
also name a file; an existing file is refused, never overwritten. `--json`
adds the issue count and byte size, which is the cheapest "is this copy
sane" check to keep in a log.

The verb refuses two workspaces on purpose: a Jira or Linear origin (nothing
to save here — the tracker holds the record) and a paired workspace (the
persist lives on the home machine; run `gadak backup` there).

## Make it periodic, and off the machine

A copy on the same disk as the original is half a backup. Copy the file to
another device — a machine on the same tailnet is the natural target, since
nothing has to be opened to the internet.

cron on the machine that runs the serve (`crontab -e`):

```cron
15 3 * * * cd /srv/backups && gadak --workspace gdk backup >/dev/null && rsync -a --remove-source-files ./ backup-host.example:/srv/gadak-backups/
```

The same as a systemd user timer: an `OnCalendar=daily` timer whose service
runs that one line with `Type=oneshot`. Prune old copies with
`find /srv/gadak-backups -name 'issuetap-*.db' -mtime +30 -delete` on the
receiving side.

Verify a copy from any machine — no gadak needed:

```bash
sqlite3 issuetap-gdk-20260902T031500Z.db 'PRAGMA integrity_check; SELECT count(*) FROM issues;'
```

## Restore

1. Stop the serve on the home machine. `install-service` names the unit
   `gadak.service` (`gadak-<workspace>.service` for a named workspace), so
   `systemctl --user stop gadak-<workspace>`; on macOS the launchd label is
   `dev.midagedev.gadak[.<workspace>]`. Otherwise end the process. Nothing
   else may hold the persist open — a running `gadak sync` or CLI write counts.
2. In the workspace's `origin/` directory, move the damaged `issuetap.db`
   aside **together with its `issuetap.db-wal` and `issuetap.db-shm`
   sidecars**. A stale `-wal` left next to a restored file is replayed onto it
   on the next open — the restore would be silently partly undone.
3. Copy the backup in as `origin/issuetap.db`.
4. Delete the mirror — `gadak.db` and its `-wal`/`-shm` in the workspace
   directory. It is a cache of the origin, and a cache that outlives its
   origin keeps answering with issues the backup never had.
5. Start the serve, then fill the mirror from the restored origin:

   ```bash
   gadak --workspace <name> sync --full
   ```

   Paired machines keep their own mirrors; run the same `sync --full` there
   so they stop serving rows the home origin no longer has.

Out of scope, deliberately: a scheduler inside gadak (cron and systemd already
exist), uploading anywhere (gadak makes no outbound requests beyond its
origin; `SECURITY.md`), and encryption (the file is yours to wrap).
