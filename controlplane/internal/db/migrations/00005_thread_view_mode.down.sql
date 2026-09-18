-- +goose Down
ALTER TABLE threads
  DROP COLUMN view_mode_id;
