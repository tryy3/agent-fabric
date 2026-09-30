-- +goose Up
-- Attempt lifecycle status for incremental persistence (#52) and cancel (#55).
-- Existing rows are completed; new in-flight assistants start as running.

ALTER TABLE messages
  ADD COLUMN status text NOT NULL DEFAULT 'completed';

ALTER TABLE messages
  ADD CONSTRAINT messages_status_check
  CHECK (status IN ('running', 'completed', 'failed', 'cancelled'));

CREATE INDEX messages_status_running_idx
  ON messages (status)
  WHERE status = 'running';
