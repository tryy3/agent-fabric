-- +goose Down
ALTER TABLE assistants
    DROP COLUMN gate_instructions;

ALTER TABLE plane_settings
    DROP COLUMN gate_instructions;
