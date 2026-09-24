-- +goose Down
UPDATE projects SET name = 'Personal' WHERE name = 'Default';
