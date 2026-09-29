-- +goose Up
-- Soft-supersede attempts: inactive assistants stay inspectable; only active
-- messages hydrate into model context. prompt_message_id links an assistant
-- attempt to the user prompt that started the turn (foothold for later forks).

ALTER TABLE messages
  ADD COLUMN active boolean NOT NULL DEFAULT true,
  ADD COLUMN prompt_message_id text REFERENCES messages(id) ON DELETE SET NULL;

CREATE INDEX messages_thread_id_active_position_idx
  ON messages (thread_id, active, position);

CREATE INDEX messages_prompt_message_id_idx
  ON messages (prompt_message_id)
  WHERE prompt_message_id IS NOT NULL;
