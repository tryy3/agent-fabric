-- +goose Down
ALTER TABLE agents
    ALTER COLUMN provider_id SET NOT NULL,
    ALTER COLUMN default_model SET NOT NULL;
