-- name: InsertMessageRound :exec
INSERT INTO message_rounds (
  message_id, round_index, model, part_index,
  prompt_tokens, completion_tokens, cached_tokens, cache_write_tokens, reasoning_tokens,
  cost_input, cost_cache_read, cost_cache_write, cost_output, cost_reasoning,
  cost_total, cost_partial, reported_cost_usd
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17
);

-- name: ListMessageRoundsByThread :many
SELECT r.message_id, r.round_index, r.model, r.part_index,
  r.prompt_tokens, r.completion_tokens, r.cached_tokens, r.cache_write_tokens, r.reasoning_tokens,
  r.cost_input, r.cost_cache_read, r.cost_cache_write, r.cost_output, r.cost_reasoning,
  r.cost_total, r.cost_partial, r.reported_cost_usd
FROM message_rounds r
JOIN messages m ON m.id = r.message_id
WHERE m.thread_id = $1
ORDER BY r.message_id, r.round_index;

-- name: SumThreadRounds :one
SELECT
  count(DISTINCT r.message_id)::int AS turns,
  count(*)::int AS requests,
  COALESCE(sum(r.prompt_tokens), 0)::bigint AS prompt_tokens,
  COALESCE(sum(r.completion_tokens), 0)::bigint AS completion_tokens,
  COALESCE(sum(r.cached_tokens), 0)::bigint AS cached_tokens,
  COALESCE(sum(r.cache_write_tokens), 0)::bigint AS cache_write_tokens,
  COALESCE(sum(r.reasoning_tokens), 0)::bigint AS reasoning_tokens,
  count(r.cost_total)::int AS priced_requests,
  COALESCE(sum(r.cost_input), 0)::float8 AS cost_input,
  COALESCE(sum(r.cost_cache_read), 0)::float8 AS cost_cache_read,
  COALESCE(sum(r.cost_cache_write), 0)::float8 AS cost_cache_write,
  COALESCE(sum(r.cost_output), 0)::float8 AS cost_output,
  COALESCE(sum(r.cost_reasoning), 0)::float8 AS cost_reasoning,
  COALESCE(sum(r.cost_total), 0)::float8 AS cost_total,
  COALESCE(bool_or(r.cost_partial), false)::boolean AS cost_partial,
  count(r.reported_cost_usd)::int AS reported_requests,
  COALESCE(sum(r.reported_cost_usd), 0)::float8 AS reported_cost_usd
FROM message_rounds r
JOIN messages m ON m.id = r.message_id
WHERE m.thread_id = $1;
