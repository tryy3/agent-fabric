-- name: InsertProject :one
INSERT INTO projects (
  id, name, description, isolation, environment_id, settings, remotes, created_at, updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING id, name, description, isolation, environment_id, settings, remotes, created_at, updated_at;

-- name: ListProjects :many
SELECT id, name, description, isolation, environment_id, settings, remotes, created_at, updated_at
FROM projects
ORDER BY created_at ASC;

-- name: GetProject :one
SELECT id, name, description, isolation, environment_id, settings, remotes, created_at, updated_at
FROM projects
WHERE id = $1;

-- name: GetDefaultProject :one
SELECT id, name, description, isolation, environment_id, settings, remotes, created_at, updated_at
FROM projects
WHERE name = 'Default'
ORDER BY created_at ASC
LIMIT 1;

-- name: UpdateProject :one
UPDATE projects
SET
  name = $2,
  description = $3,
  isolation = $4,
  environment_id = $5,
  settings = $6,
  remotes = $7,
  updated_at = $8
WHERE id = $1
RETURNING id, name, description, isolation, environment_id, settings, remotes, created_at, updated_at;

-- name: DeleteProject :exec
DELETE FROM projects WHERE id = $1;

-- name: CountThreadsByProject :one
SELECT count(*) FROM threads WHERE project_id = $1;
