-- name: GetPlaneSettings :one
SELECT id, sandbox, environment, created_at, updated_at
FROM plane_settings
WHERE id = 'default';

-- name: InsertPlaneSettings :one
INSERT INTO plane_settings (id, sandbox, environment, created_at, updated_at)
VALUES ('default', $1, '{}'::jsonb, $2, $3)
RETURNING id, sandbox, environment, created_at, updated_at;

-- name: UpdatePlaneSettings :one
UPDATE plane_settings
SET sandbox = $1, environment = $2, updated_at = $3
WHERE id = 'default'
RETURNING id, sandbox, environment, created_at, updated_at;

-- name: GetPlaneIntegrations :one
SELECT integrations
FROM plane_settings
WHERE id = 'default';

-- name: UpdatePlaneIntegrations :exec
UPDATE plane_settings
SET integrations = $1, updated_at = $2
WHERE id = 'default';

-- name: GetPlaneInstructions :one
SELECT platform_instructions, runtime_context
FROM plane_settings
WHERE id = 'default';

-- name: UpdatePlaneInstructions :exec
UPDATE plane_settings
SET platform_instructions = $1, runtime_context = $2, updated_at = $3
WHERE id = 'default';

-- name: GetPlaneGateInstructions :one
SELECT gate_instructions
FROM plane_settings
WHERE id = 'default';

-- name: UpdatePlaneGateInstructions :exec
UPDATE plane_settings
SET gate_instructions = $1, updated_at = $2
WHERE id = 'default';

-- name: GetPlanePermissions :one
SELECT permissions
FROM plane_settings
WHERE id = 'default';

-- name: UpdatePlanePermissions :exec
UPDATE plane_settings
SET permissions = $1, updated_at = $2
WHERE id = 'default';

-- name: CountAllThreads :one
SELECT count(*) FROM threads;

-- name: CountNonDefaultProjects :one
SELECT count(*) FROM projects WHERE name <> 'Default';

-- name: GetPlaneSkills :one
SELECT skills
FROM plane_settings
WHERE id = 'default';

-- name: UpdatePlaneSkills :exec
UPDATE plane_settings
SET skills = $1, updated_at = $2
WHERE id = 'default';

-- name: GetPlaneLimits :one
SELECT limits
FROM plane_settings
WHERE id = 'default';

-- name: UpdatePlaneLimits :exec
UPDATE plane_settings
SET limits = $1, updated_at = $2
WHERE id = 'default';
