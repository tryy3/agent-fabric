-- +goose Down
DROP INDEX IF EXISTS threads_project_id_idx;
ALTER TABLE threads DROP COLUMN IF EXISTS project_id;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS environments;
