-- name: ListInferenceConnections :many
SELECT id, name, type, base_url, api_key, models, models_updated_at, created_at, updated_at
FROM inference_connections
ORDER BY created_at ASC;

-- name: GetInferenceConnection :one
SELECT id, name, type, base_url, api_key, models, models_updated_at, created_at, updated_at
FROM inference_connections
WHERE id = $1;

-- name: InsertInferenceConnection :one
INSERT INTO inference_connections (
  id, name, type, base_url, api_key, models, models_updated_at, created_at, updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING id, name, type, base_url, api_key, models, models_updated_at, created_at, updated_at;

-- name: UpdateInferenceConnection :one
UPDATE inference_connections
SET
  name = $2,
  base_url = $3,
  api_key = $4,
  updated_at = $5
WHERE id = $1
RETURNING id, name, type, base_url, api_key, models, models_updated_at, created_at, updated_at;

-- name: UpdateInferenceConnectionModels :one
UPDATE inference_connections
SET
  models = $2,
  models_updated_at = $3,
  updated_at = $4
WHERE id = $1
RETURNING id, name, type, base_url, api_key, models, models_updated_at, created_at, updated_at;

-- name: DeleteInferenceConnection :exec
DELETE FROM inference_connections WHERE id = $1;
