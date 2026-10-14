-- +goose Down
DROP INDEX IF EXISTS project_publishes_deploy_idx;
ALTER TABLE project_publishes DROP COLUMN IF EXISTS live;
