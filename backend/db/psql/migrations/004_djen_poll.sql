-- 004_djen_poll.sql
-- DJEN case-mode poll scheduling.
--
-- Problem: DJEN case mode used to reuse ListWatcherCasesForPoll, which gates
-- concluded cases on watcher_tracking.last_polled_at. But the DataJud watcher
-- cron runs at 02:10 (before DJEN at 03:40) and refreshes last_polled_at, so
-- concluded cases never fell inside DJEN's "polled > 7 days ago" window and were
-- never listed. DJEN now tracks its own poll cursor in a dedicated column so its
-- cadence is independent of the DataJud watcher.

ALTER TABLE watcher_tracking
  ADD COLUMN IF NOT EXISTS djen_last_polled_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_watcher_djen_last_polled
  ON watcher_tracking (djen_last_polled_at);
