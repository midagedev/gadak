# Backing up the built-in tracker — and putting it back

On a workspace whose origin is the built-in tracker, the record is two
things in the workspace directory (`~/.gadak/` or `$GADAK_HOME`, or
`~/.gadak/profiles/<name>/`):

- `origin/issuetap.db` — the issues, comments, history, wiki pages.
- `origin/blobs/` — the attachment bytes, one file per distinct attachment,
  named by its sha256.

`gadak.db` next to them is a cache — the next sync rebuilds it — so it is not
what you back up. Lose either of the two and what they held is gone; a Jira
or Linear workspace has neither, because the tracker holds the record.

Both go into one archive, and both come back together. A copy of the database
alone is a backup with every attachment missing, and nothing in it says so.

## Going back to an older gadak

The first time this build opens a built-in workspace it moves the attachment
bytes out of `issuetap.db` and leaves the pre-move copy beside it as
`issuetap.db.pre-v2.bak`. An older gadak cannot read the new file and says
so — `schema_version 2 (this build reads 1)`, with no more than that, because
that build predates the copy and does not know it exists. To go back: stop
everything holding the workspace, then

```bash
cd ~/.gadak/origin      # or $GADAK_HOME/origin, or profiles/<name>/origin
rm -f issuetap.db issuetap.db-wal issuetap.db-shm
mv issuetap.db.pre-v2.bak issuetap.db
```

and delete the mirror (`gadak.db*`) so it is rebuilt. Anything written after
the upgrade is not in that copy. Delete the `.bak` once you are sure you are
staying — it is a second full copy of the database, and `gadak doctor` will
keep pointing at it.

## Take a backup

```bash
gadak --workspace <name> backup --to /srv/backups
```

Prints the path of one tar (`issuetap-<workspace>-<UTC stamp>.tar`) holding
`issuetap.db` and `blobs/`. The database inside it is an online copy
(`VACUUM INTO` under a read transaction), so leave `gadak serve` running — it
includes what is still in the `-wal` sidecar and has no sidecar of its own.
`--to` may also name a file; an existing file is refused, never overwritten.
`--json` adds the issue count and byte size, which is the cheapest "is this
copy sane" check to keep in a log.

Before writing the archive, `backup` checks that every attachment the
database references has its bytes on disk, and refuses with the hash if one
does not. That check is the difference between a backup and a file.

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
`find /srv/gadak-backups -name 'issuetap-*.tar' -mtime +30 -delete` on the
receiving side.

Verify a copy from any machine — no gadak needed:

```bash
tar -tf issuetap-gdk-20260902T031500Z.tar | head
tar -xOf issuetap-gdk-20260902T031500Z.tar issuetap.db > /tmp/check.db
sqlite3 /tmp/check.db 'PRAGMA integrity_check; SELECT count(*) FROM issues;'
```

## Restore

1. Stop the serve on the home machine. `install-service` names the unit
   `gadak.service` (`gadak-<workspace>.service` for a named workspace), so
   `systemctl --user stop gadak-<workspace>`; on macOS the launchd label is
   `dev.midagedev.gadak[.<workspace>]`. Otherwise end the process. Nothing
   else may hold the persist open — a running `gadak sync` or CLI write counts.
2. In the workspace's `origin/` directory, move the damaged `issuetap.db`
   aside **together with its `issuetap.db-wal` and `issuetap.db-shm`
   sidecars**, and move `blobs/` aside as well. A stale `-wal` left next to a
   restored file is replayed onto it on the next open — the restore would be
   silently partly undone.
3. Unpack the archive into `origin/`:

   ```bash
   tar -xf issuetap-<workspace>-<stamp>.tar -C ~/.gadak/origin
   ```

   That writes `issuetap.db` and `blobs/`. Keeping the old `blobs/` aside
   rather than merging it is deliberate: the archive is a complete set, and a
   merge hides which files the backup actually had.
4. Delete the mirror — `gadak.db` and its `-wal`/`-shm` in the workspace
   directory. It is a cache of the origin, and a cache that outlives its
   origin keeps answering with issues the backup never had.
5. Start the serve, then fill the mirror from the restored origin:

   ```bash
   gadak --workspace <name> sync --full
   ```

   Attachment bytes need no separate step — the app and `gadak attach get`
   read them from the restored `blobs/` through the origin.

   Paired machines keep their own mirrors; run the same `sync --full` there
   so they stop serving rows the home origin no longer has.

Out of scope, deliberately: a scheduler inside gadak (cron and systemd already
exist), uploading anywhere (gadak makes no outbound requests beyond its
origin; `SECURITY.md`), and encryption (the file is yours to wrap).
