-- name: InsertProjectPublish :one
INSERT INTO project_publishes (
  id, project_id, provider, target, deploy_id, site_url, deploy_url, created_at, live
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING id, project_id, provider, target, deploy_id, site_url, deploy_url, created_at, live;

-- name: ListProjectPublishes :many
SELECT id, project_id, provider, target, deploy_id, site_url, deploy_url, created_at, live
FROM project_publishes
WHERE project_id = sqlc.arg(project_id)
  AND (sqlc.narg(provider)::text IS NULL OR provider = sqlc.narg(provider)::text)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit);

-- name: ListProjectPublishesForTarget :many
SELECT id, project_id, provider, target, deploy_id, site_url, deploy_url, created_at, live
FROM project_publishes
WHERE project_id = $1 AND provider = $2 AND target = $3
ORDER BY created_at DESC, id DESC;

-- name: GetProjectPublish :one
SELECT id, project_id, provider, target, deploy_id, site_url, deploy_url, created_at, live
FROM project_publishes
WHERE id = $1;

-- name: ClearProjectPublishLive :exec
UPDATE project_publishes
SET live = false
WHERE project_id = $1 AND provider = $2 AND target = $3 AND live;

-- name: UpdateProjectPublishSynced :exec
UPDATE project_publishes
SET site_url = $2, deploy_url = $3, live = $4
WHERE id = $1;

-- name: DeleteProjectPublish :exec
DELETE FROM project_publishes
WHERE id = $1;
