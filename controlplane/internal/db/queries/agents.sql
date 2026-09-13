-- name: ListAgents :many
SELECT
  a.id, a.name, a.description, a.version, a.provider_id, a.default_model,
  a.created_at, a.updated_at,
  p.name AS provider_name
FROM agents a
LEFT JOIN providers p ON p.id = a.provider_id
ORDER BY a.created_at ASC;

-- name: GetAgent :one
SELECT
  a.id, a.name, a.description, a.version, a.provider_id, a.default_model,
  a.created_at, a.updated_at,
  p.name AS provider_name
FROM agents a
LEFT JOIN providers p ON p.id = a.provider_id
WHERE a.id = $1;

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
WHERE provider_id = $1
ORDER BY created_at ASC;

-- name: UnlinkAgentsByProvider :exec
UPDATE agents
SET
  provider_id = NULL,
  default_model = NULL,
  version = version + 1,
  updated_at = $2
WHERE provider_id = $1;
