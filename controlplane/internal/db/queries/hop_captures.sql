-- name: InsertHopCapture :one
INSERT INTO hop_captures (
  id, thread_id, message_id, session_id, round_index, hop_kind, direction,
  method, url, status_code, headers_json, body_text, meta_json, created_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
)
RETURNING id, thread_id, message_id, session_id, round_index, hop_kind, direction,
  method, url, status_code, headers_json, body_text, meta_json, created_at;

-- name: LinkHopCapturesToMessage :exec
UPDATE hop_captures
SET message_id = $3
WHERE thread_id = $1 AND session_id = $2 AND message_id IS NULL;

-- name: DeleteHopCapturesBySession :exec
DELETE FROM hop_captures
WHERE thread_id = $1 AND session_id = $2 AND message_id IS NULL;

-- name: ListHopCapturesByMessage :many
SELECT id, thread_id, message_id, session_id, round_index, hop_kind, direction,
  method, url, status_code, headers_json, body_text, meta_json, created_at
FROM hop_captures
WHERE thread_id = $1 AND message_id = $2
ORDER BY round_index ASC, created_at ASC;

-- name: ListHopCapturesByThread :many
SELECT id, thread_id, message_id, session_id, round_index, hop_kind, direction,
  method, url, status_code, headers_json, body_text, meta_json, created_at
FROM hop_captures
WHERE thread_id = $1 AND message_id IS NOT NULL
ORDER BY created_at ASC, round_index ASC;
