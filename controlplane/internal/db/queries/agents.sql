-- name: ListAgents :many
SELECT id, name, description, version, provider_id, default_model, created_at, updated_at
FROM agents
ORDER BY created_at ASC;

-- name: GetAgent :one
SELECT id, name, description, version, provider_id, default_model, created_at, updated_at
FROM agents
WHERE id = $1;

-- name: InsertAgent :one
INSERT INTO agents (
  id, name, description, version, provider_id, default_model, created_at, updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING id, name, description, version, provider_id, default_model, created_at, updated_at;

-- name: UpdateAgent :one
UPDATE agents
SET
  name = $2,
  description = $3,
  version = $4,
  provider_id = $5,
  default_model = $6,
  updated_at = $7
WHERE id = $1
RETURNING id, name, description, version, provider_id, default_model, created_at, updated_at;

-- name: DeleteAgent :exec
DELETE FROM agents WHERE id = $1;

-- name: CountAgentsByProvider :one
SELECT COUNT(*)::bigint FROM agents WHERE provider_id = $1;

-- name: ListAgentsByProvider :many
SELECT id, name, description, version, provider_id, default_model, created_at, updated_at
FROM agents
WHERE provider_id = $1;
