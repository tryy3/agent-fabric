-- +goose Down
DROP INDEX IF EXISTS messages_status_running_idx;

ALTER TABLE messages
  DROP CONSTRAINT IF EXISTS messages_status_check;

ALTER TABLE messages
  DROP COLUMN IF EXISTS status;
