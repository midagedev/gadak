-- Single owner of the bar-widget query. BarWidget.qml and verify.sh both
-- run this file through `gadak sql --json`. Do not open the mirror file.
--
-- open  = issues whose status_category is not `done`
-- stuck = those same rows whose status_changed_at is older than 7 days
--         (time-in-status is not a stored column; compute it here)
-- NULL status_changed_at cannot be aged, so it is not stuck.
SELECT
  COUNT(*) AS open,
  COALESCE(SUM(CASE
    WHEN status_changed_at IS NOT NULL
     AND julianday('now') - julianday(status_changed_at) > 7
    THEN 1 ELSE 0 END), 0) AS stuck
FROM issues_full
WHERE status_category != 'done';
