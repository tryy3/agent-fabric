-- name: GetPlaneSettings :one
SELECT id, sandbox, created_at, updated_at
FROM plane_settings
WHERE id = 'default';

-- name: InsertPlaneSettings :one
INSERT INTO plane_settings (id, sandbox, created_at, updated_at)
VALUES ('default', $1, $2, $3)
RETURNING id, sandbox, created_at, updated_at;

-- name: UpdatePlaneSettings :one
UPDATE plane_settings
SET sandbox = $1, updated_at = $2
WHERE id = 'default'
RETURNING id, sandbox, created_at, updated_at;

-- name: CountAllThreads :one
SELECT count(*) FROM threads;

-- name: CountNonDefaultProjects :one
SELECT count(*) FROM projects WHERE name <> 'Default';
