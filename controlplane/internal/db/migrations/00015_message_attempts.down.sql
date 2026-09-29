-- +goose Down
DROP INDEX IF EXISTS messages_prompt_message_id_idx;
DROP INDEX IF EXISTS messages_thread_id_active_position_idx;

ALTER TABLE messages
  DROP COLUMN IF EXISTS prompt_message_id,
  DROP COLUMN IF EXISTS active;
