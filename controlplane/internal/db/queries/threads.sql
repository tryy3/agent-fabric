-- name: InsertThread :one
INSERT INTO threads (
  id, title, title_source, assistant_id, current_model, project_id, created_at, updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING id, title, title_source, assistant_id, current_model, view_mode_id, project_id, created_at, updated_at;

-- name: ListThreads :many
SELECT
  t.id,
  t.title,
  t.title_source,
  t.assistant_id,
  t.current_model,
  t.view_mode_id,
  t.project_id,
  t.created_at,
  t.updated_at,
  (SELECT count(*)::int FROM messages m WHERE m.thread_id = t.id) AS message_count
FROM threads t
WHERE sqlc.narg('project_id')::text IS NULL OR t.project_id = sqlc.narg('project_id')
ORDER BY t.updated_at DESC;

-- name: GetThread :one
SELECT id, title, title_source, assistant_id, current_model, view_mode_id, project_id, created_at, updated_at
FROM threads
WHERE id = $1;

-- name: GetThreadForUpdate :one
SELECT id, title, title_source, assistant_id, current_model, view_mode_id, project_id, created_at, updated_at
FROM threads
WHERE id = $1
FOR UPDATE;

-- name: RenameThread :one
UPDATE threads
SET title = $2, title_source = $3, updated_at = $4
WHERE id = $1
RETURNING id, title, title_source, assistant_id, current_model, view_mode_id, project_id, created_at, updated_at;

-- name: PinThreadAssistant :one
UPDATE threads
SET assistant_id = $2, updated_at = $3
WHERE id = $1
RETURNING id, title, title_source, assistant_id, current_model, view_mode_id, project_id, created_at, updated_at;

-- name: SetThreadModel :one
UPDATE threads
SET current_model = $2, updated_at = $3
WHERE id = $1
RETURNING id, title, title_source, assistant_id, current_model, view_mode_id, project_id, created_at, updated_at;

-- name: SetThreadViewMode :one
UPDATE threads
SET view_mode_id = $2, updated_at = $3
WHERE id = $1
RETURNING id, title, title_source, assistant_id, current_model, view_mode_id, project_id, created_at, updated_at;

-- name: SetThreadTitleIfAuto :exec
UPDATE threads
SET title = $2, updated_at = $3
WHERE id = $1 AND title_source = 'auto';

-- name: TouchThread :exec
UPDATE threads SET updated_at = $2 WHERE id = $1;

-- name: InsertMessage :one
INSERT INTO messages (
  id, thread_id, role, content, position, created_at,
  parts, model, provider_id, provider_name, stop_reason,
  active, prompt_message_id, status
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING id, thread_id, role, content, position, created_at,
  parts, model, provider_id, provider_name, stop_reason,
  active, prompt_message_id, status;

-- name: ListMessages :many
SELECT id, thread_id, role, content, position, created_at,
  parts, model, provider_id, provider_name, stop_reason,
  active, prompt_message_id, status
FROM messages
WHERE thread_id = $1
ORDER BY position ASC;

-- name: ListActiveMessages :many
SELECT id, thread_id, role, content, position, created_at,
  parts, model, provider_id, provider_name, stop_reason,
  active, prompt_message_id, status
FROM messages
WHERE thread_id = $1 AND active = true
ORDER BY position ASC;

-- name: NextMessagePosition :one
SELECT COALESCE(MAX(position), -1)::int AS max_position
FROM messages
WHERE thread_id = $1;

-- name: GetLastActiveUserMessage :one
SELECT id, thread_id, role, content, position, created_at,
  parts, model, provider_id, provider_name, stop_reason,
  active, prompt_message_id, status
FROM messages
WHERE thread_id = $1 AND role = 'user' AND active = true
ORDER BY position DESC
LIMIT 1;

-- name: GetMessage :one
SELECT id, thread_id, role, content, position, created_at,
  parts, model, provider_id, provider_name, stop_reason,
  active, prompt_message_id, status
FROM messages
WHERE id = $1 AND thread_id = $2;

-- name: UpdateMessageParts :exec
UPDATE messages
SET parts = $3, content = $4
WHERE id = $1 AND thread_id = $2 AND status = 'running';

-- name: FinalizeMessageAttempt :exec
UPDATE messages
SET status = $3,
    stop_reason = $4,
    parts = $5,
    content = $6,
    model = COALESCE($7, model),
    provider_id = COALESCE($8, provider_id),
    provider_name = COALESCE($9, provider_name),
    active = $10
WHERE id = $1 AND thread_id = $2 AND status = 'running';

-- name: ActivateMessage :exec
UPDATE messages
SET active = true
WHERE id = $1 AND thread_id = $2;

-- name: ListRunningAssistantMessages :many
SELECT id, thread_id, role, content, position, created_at,
  parts, model, provider_id, provider_name, stop_reason,
  active, prompt_message_id, status
FROM messages
WHERE role = 'assistant' AND status = 'running'
ORDER BY created_at ASC;

-- name: InterruptRunningAssistants :exec
UPDATE messages
SET status = 'failed',
    stop_reason = 'interrupted'
WHERE role = 'assistant' AND status = 'running';

-- name: SupersedeMessage :exec
UPDATE messages
SET active = false
WHERE id = $1 AND thread_id = $2 AND active = true;

-- name: SupersedeActiveAssistantsForPrompt :exec
UPDATE messages
SET active = false
WHERE thread_id = $1
  AND role = 'assistant'
  AND active = true
  AND prompt_message_id = $2;

-- name: CountThreadsByAssistant :one
SELECT count(*) FROM threads WHERE assistant_id = $1;
