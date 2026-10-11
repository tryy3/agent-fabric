-- name: InsertProjectPublish :one
INSERT INTO project_publishes (
  id, project_id, provider, target, deploy_id, site_url, deploy_url, created_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING id, project_id, provider, target, deploy_id, site_url, deploy_url, created_at;

-- name: ListProjectPublishes :many
SELECT id, project_id, provider, target, deploy_id, site_url, deploy_url, created_at
FROM project_publishes
WHERE project_id = sqlc.arg(project_id)
  AND (sqlc.narg(provider)::text IS NULL OR provider = sqlc.narg(provider)::text)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(row_limit);
