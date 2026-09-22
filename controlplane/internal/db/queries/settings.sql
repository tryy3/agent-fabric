-- name: GetPlaneSettings :one
SELECT id, sandbox, environment, created_at, updated_at
FROM plane_settings
WHERE id = 'default';

-- name: InsertPlaneSettings :one
INSERT INTO plane_settings (id, sandbox, environment, created_at, updated_at)
VALUES ('default', $1, '{}'::jsonb, $2, $3)
RETURNING id, sandbox, environment, created_at, updated_at;

-- name: UpdatePlaneSettings :one
UPDATE plane_settings
SET sandbox = $1, environment = $2, updated_at = $3
WHERE id = 'default'
RETURNING id, sandbox, environment, created_at, updated_at;

-- name: CountAllThreads :one
SELECT count(*) FROM threads;

-- name: CountNonDefaultProjects :one
SELECT count(*) FROM projects WHERE name <> 'Default';
