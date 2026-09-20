-- name: InsertEnvironment :one
INSERT INTO environments (
  id, name, kind, spec, volume_name, created_at, updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7
)
RETURNING id, name, kind, spec, volume_name, created_at, updated_at;

-- name: GetEnvironment :one
SELECT id, name, kind, spec, volume_name, created_at, updated_at
FROM environments
WHERE id = $1;
