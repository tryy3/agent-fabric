-- +goose Down
ALTER TABLE assistants
    DROP COLUMN instructions;

ALTER TABLE plane_settings
    DROP COLUMN harness_instructions;
