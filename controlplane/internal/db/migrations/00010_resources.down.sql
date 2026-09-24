-- +goose Down
ALTER TABLE plane_settings DROP COLUMN environment;
DROP TABLE resources;
