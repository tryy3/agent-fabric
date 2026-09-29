-- name: ListToolIntegrations :many
SELECT id, name, kind, enabled, scope, endpoint, mode, capabilities, config, secrets,
       health_status, health_checked_at, created_at, updated_at
FROM tool_integrations
ORDER BY created_at ASC;

-- name: GetToolIntegration :one
SELECT id, name, kind, enabled, scope, endpoint, mode, capabilities, config, secrets,
       health_status, health_checked_at, created_at, updated_at
FROM tool_integrations
WHERE id = $1;

-- name: InsertToolIntegration :one
INSERT INTO tool_integrations (
  id, name, kind, enabled, scope, endpoint, mode, capabilities, config, secrets,
  health_status, health_checked_at, created_at, updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
)
RETURNING id, name, kind, enabled, scope, endpoint, mode, capabilities, config, secrets,
          health_status, health_checked_at, created_at, updated_at;

-- name: UpdateToolIntegration :one
UPDATE tool_integrations
SET
  name = $2,
  enabled = $3,
  endpoint = $4,
  mode = $5,
  capabilities = $6,
  config = $7,
  secrets = $8,
  health_status = $9,
  health_checked_at = $10,
  updated_at = $11
WHERE id = $1
RETURNING id, name, kind, enabled, scope, endpoint, mode, capabilities, config, secrets,
          health_status, health_checked_at, created_at, updated_at;

-- name: DeleteToolIntegration :exec
DELETE FROM tool_integrations WHERE id = $1;

-- name: GetPlaneToolDefaults :one
SELECT web_search_integration_id, fetch_page_integration_id
FROM plane_settings
WHERE id = 'default';

-- name: UpdatePlaneToolDefaults :exec
UPDATE plane_settings
SET
  web_search_integration_id = $1,
  fetch_page_integration_id = $2,
  updated_at = $3
WHERE id = 'default';
