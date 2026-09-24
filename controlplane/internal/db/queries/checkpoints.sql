-- name: InsertProjectCheckpoint :one
INSERT INTO project_checkpoints (
  id, project_id, sha, label, thread_id, message_id, created_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7
)
RETURNING id, project_id, sha, label, thread_id, message_id, created_at;

-- name: ListProjectCheckpoints :many
SELECT id, project_id, sha, label, thread_id, message_id, created_at
FROM project_checkpoints
WHERE project_id = $1
ORDER BY created_at DESC;

-- name: GetProjectCheckpoint :one
SELECT id, project_id, sha, label, thread_id, message_id, created_at
FROM project_checkpoints
WHERE id = $1;
