-- name: InsertThread :one
INSERT INTO threads (
  id, title, title_source, agent_id, current_model, created_at, updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7
)
RETURNING id, title, title_source, agent_id, current_model, created_at, updated_at;

-- name: ListThreads :many
SELECT
  t.id,
  t.title,
  t.title_source,
  t.agent_id,
  t.current_model,
  t.created_at,
  t.updated_at,
  (SELECT count(*)::int FROM messages m WHERE m.thread_id = t.id) AS message_count
FROM threads t
ORDER BY t.updated_at DESC;

-- name: GetThread :one
SELECT id, title, title_source, agent_id, current_model, created_at, updated_at
FROM threads
WHERE id = $1;

-- name: GetThreadForUpdate :one
SELECT id, title, title_source, agent_id, current_model, created_at, updated_at
FROM threads
WHERE id = $1
FOR UPDATE;

-- name: RenameThread :one
UPDATE threads
SET title = $2, title_source = $3, updated_at = $4
WHERE id = $1
RETURNING id, title, title_source, agent_id, current_model, created_at, updated_at;

-- name: PinThreadAgent :one
UPDATE threads
SET agent_id = $2, updated_at = $3
WHERE id = $1
RETURNING id, title, title_source, agent_id, current_model, created_at, updated_at;

-- name: SetThreadModel :one
UPDATE threads
SET current_model = $2, updated_at = $3
WHERE id = $1
RETURNING id, title, title_source, agent_id, current_model, created_at, updated_at;

-- name: SetThreadTitleIfAuto :exec
UPDATE threads
SET title = $2, updated_at = $3
WHERE id = $1 AND title_source = 'auto';

-- name: TouchThread :exec
UPDATE threads SET updated_at = $2 WHERE id = $1;

-- name: InsertMessage :one
INSERT INTO messages (id, thread_id, role, content, position, created_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, thread_id, role, content, position, created_at;

-- name: ListMessages :many
SELECT id, thread_id, role, content, position, created_at
FROM messages
WHERE thread_id = $1
ORDER BY position ASC;

-- name: NextMessagePosition :one
SELECT COALESCE(MAX(position), -1)::int AS max_position
FROM messages
WHERE thread_id = $1;

-- name: CountThreadsByAgent :one
SELECT count(*) FROM threads WHERE agent_id = $1;
