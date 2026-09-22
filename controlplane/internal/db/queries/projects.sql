-- name: InsertProject :one
INSERT INTO projects (
  id, name, description, settings, remotes, created_at, updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7
)
RETURNING id, name, description, settings, remotes, created_at, updated_at;

-- name: ListProjects :many
SELECT id, name, description, settings, remotes, created_at, updated_at
FROM projects
ORDER BY created_at ASC;

-- name: GetProject :one
SELECT id, name, description, settings, remotes, created_at, updated_at
FROM projects
WHERE id = $1;

-- name: GetDefaultProject :one
SELECT id, name, description, settings, remotes, created_at, updated_at
FROM projects
WHERE name = 'Default'
ORDER BY created_at ASC
LIMIT 1;

-- name: UpdateProject :one
UPDATE projects
SET
  name = $2,
  description = $3,
  settings = $4,
  remotes = $5,
  updated_at = $6
WHERE id = $1
RETURNING id, name, description, settings, remotes, created_at, updated_at;

-- name: DeleteProject :exec
DELETE FROM projects WHERE id = $1;

-- name: CountThreadsByProject :one
SELECT count(*) FROM threads WHERE project_id = $1;

-- name: DeleteThreadsByProject :exec
DELETE FROM threads WHERE project_id = $1;
