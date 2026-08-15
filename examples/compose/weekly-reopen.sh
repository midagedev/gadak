#!/usr/bin/env bash
# What keeps coming back — the first recipe of docs/RECIPES.md, on a schedule.
# crontab: 0 9 * * 1 /path/to/examples/compose/weekly-reopen.sh >> ~/gadak-reopen.log 2>&1
# cron does not read your login shell: set GADAK to the binary's absolute path
# when `gadak` is not on cron's PATH. Extra arguments go to `gadak sql` (e.g. --csv).
set -euo pipefail
GADAK="${GADAK:-gadak}"
"$GADAK" sql "$@" "select key, summary, reopen_count, reopened_at, reopen_reason
from issues_full where reopen_count > 0
order by reopened_at desc limit 20"
