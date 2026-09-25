-- +goose Down
ALTER TABLE plane_settings
    DROP COLUMN integrations;
