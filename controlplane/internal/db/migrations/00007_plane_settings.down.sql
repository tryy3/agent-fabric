-- +goose Down
ALTER TABLE agents DROP COLUMN settings;

DROP TABLE plane_settings;
