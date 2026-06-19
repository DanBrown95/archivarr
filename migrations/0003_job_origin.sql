-- Distinguish automatically-scheduled jobs from manually-triggered ones.
--
-- Pause suspends *automation*: 'auto' jobs (created by the scheduler) wait while
-- paused, but 'manual' jobs (started by the user via the UI/API) run regardless.
-- Existing rows default to 'manual' (they were user/most-recent activity).

ALTER TABLE jobs ADD COLUMN origin TEXT NOT NULL DEFAULT 'manual'
    CHECK (origin IN ('manual','auto'));
