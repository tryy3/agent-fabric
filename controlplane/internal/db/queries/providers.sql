-- name: ListProviders :many
SELECT id, name, type, base_url, api_key, models, models_updated_at, created_at, updated_at
FROM providers
ORDER BY created_at ASC;

-- name: GetProvider :one
SELECT id, name, type, base_url, api_key, models, models_updated_at, created_at, updated_at
FROM providers
WHERE id = $1;

-- name: GetProviderForUpdate :one
SELECT id, name, type, base_url, api_key, models, models_updated_at, created_at, updated_at
FROM providers
WHERE id = $1
FOR UPDATE;

-- name: InsertProvider :one
INSERT INTO providers (
  id, name, type, base_url, api_key, models, models_updated_at, created_at, updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING id, name, type, base_url, api_key, models, models_updated_at, created_at, updated_at;

-- name: UpdateProvider :one
UPDATE providers
SET
  name = $2,
  base_url = $3,
  api_key = $4,
  updated_at = $5
WHERE id = $1
RETURNING id, name, type, base_url, api_key, models, models_updated_at, created_at, updated_at;

-- name: UpdateProviderModels :one
UPDATE providers
SET
  models = $2,
  models_updated_at = $3,
  updated_at = $4
WHERE id = $1
RETURNING id, name, type, base_url, api_key, models, models_updated_at, created_at, updated_at;

-- name: DeleteProvider :exec
DELETE FROM providers WHERE id = $1;
