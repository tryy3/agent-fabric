-- name: GetModelSpecs :one
SELECT id, source_url, sync_interval_hours, enabled, snapshot_source_url, etag, bytes,
       fetched_at, last_attempt_at, last_error, data
FROM model_specs
WHERE id = 'default';

-- name: UpdateModelSpecsSettings :exec
UPDATE model_specs
SET source_url = $1, sync_interval_hours = $2, enabled = $3
WHERE id = 'default';

-- name: SaveModelSpecsSnapshot :exec
UPDATE model_specs
SET snapshot_source_url = $1, etag = $2, bytes = $3, fetched_at = $4,
    last_attempt_at = $4, last_error = '', data = $5
WHERE id = 'default';

-- name: TouchModelSpecsSnapshot :exec
UPDATE model_specs
SET fetched_at = $1, last_attempt_at = $1, last_error = ''
WHERE id = 'default';

-- name: RecordModelSpecsError :exec
UPDATE model_specs
SET last_attempt_at = $1, last_error = $2
WHERE id = 'default';
