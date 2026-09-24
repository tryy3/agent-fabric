-- name: ListResources :many
SELECT id, name, kind, spec, created_at, updated_at
FROM resources
ORDER BY created_at ASC, id ASC;

-- name: GetResource :one
SELECT id, name, kind, spec, created_at, updated_at
FROM resources
WHERE id = $1;

-- name: InsertResource :one
INSERT INTO resources (id, name, kind, spec, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, name, kind, spec, created_at, updated_at;

-- name: UpdateResource :one
UPDATE resources
SET name = $2, spec = $3, updated_at = $4
WHERE id = $1
RETURNING id, name, kind, spec, created_at, updated_at;

-- name: DeleteResource :exec
DELETE FROM resources WHERE id = $1;
