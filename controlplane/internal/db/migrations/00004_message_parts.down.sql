-- +goose Down
ALTER TABLE messages
  DROP COLUMN IF EXISTS stop_reason,
  DROP COLUMN IF EXISTS provider_name,
  DROP COLUMN IF EXISTS provider_id,
  DROP COLUMN IF EXISTS model,
  DROP COLUMN IF EXISTS parts;
