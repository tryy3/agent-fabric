-- name: ListAssistants :many
SELECT
  a.id, a.name, a.description, a.instructions, a.version, a.inference_connection_id, a.default_model,
  a.settings, a.created_at, a.updated_at,
  c.name AS inference_connection_name
FROM assistants a
LEFT JOIN inference_connections c ON c.id = a.inference_connection_id
ORDER BY a.created_at ASC;

-- name: GetAssistant :one
SELECT
  a.id, a.name, a.description, a.instructions, a.version, a.inference_connection_id, a.default_model,
  a.settings, a.created_at, a.updated_at,
  c.name AS inference_connection_name
FROM assistants a
LEFT JOIN inference_connections c ON c.id = a.inference_connection_id
WHERE a.id = $1;

-- name: InsertAssistant :one
INSERT INTO assistants (
  id, name, description, instructions, version, inference_connection_id, default_model, settings, created_at, updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
RETURNING id, name, description, instructions, version, inference_connection_id, default_model, settings, created_at, updated_at;

-- name: UpdateAssistant :one
UPDATE assistants
SET
  name = $2,
  description = $3,
  instructions = $4,
  version = $5,
  inference_connection_id = $6,
  default_model = $7,
  settings = $8,
  updated_at = $9
WHERE id = $1
RETURNING id, name, description, instructions, version, inference_connection_id, default_model, settings, created_at, updated_at;

-- name: DeleteAssistant :exec
DELETE FROM assistants WHERE id = $1;

-- name: CountAssistantsByInferenceConnection :one
SELECT COUNT(*)::bigint FROM assistants WHERE inference_connection_id = $1;

-- name: ListAssistantsByInferenceConnection :many
SELECT id, name, description, instructions, version, inference_connection_id, default_model, settings, created_at, updated_at
FROM assistants
WHERE inference_connection_id = $1
ORDER BY created_at ASC;

-- name: UnlinkAssistantsByInferenceConnection :exec
UPDATE assistants
SET
  inference_connection_id = NULL,
  default_model = NULL,
  version = version + 1,
  updated_at = $2
WHERE inference_connection_id = $1;
