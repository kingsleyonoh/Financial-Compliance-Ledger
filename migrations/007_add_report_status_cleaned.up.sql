-- Migration 007: Add 'cleaned' status to reports table
-- Supports the report cleanup goroutine that deletes old PDF files.

ALTER TABLE reports DROP CONSTRAINT IF EXISTS reports_status_check;
ALTER TABLE reports ADD CONSTRAINT reports_status_check
    CHECK (status IN ('pending', 'generating', 'completed', 'failed', 'cleaned'));
