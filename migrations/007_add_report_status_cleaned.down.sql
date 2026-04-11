-- Revert 007: Remove 'cleaned' status from reports table

-- First update any 'cleaned' rows back to 'completed'
UPDATE reports SET status = 'completed' WHERE status = 'cleaned';

ALTER TABLE reports DROP CONSTRAINT IF EXISTS reports_status_check;
ALTER TABLE reports ADD CONSTRAINT reports_status_check
    CHECK (status IN ('pending', 'generating', 'completed', 'failed'));
