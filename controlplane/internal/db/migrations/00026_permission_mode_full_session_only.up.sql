-- +goose Up
-- Full access is chosen per session over ACP (session/set_mode) and is never
-- stored on an assistant (decision 24). An assistant that stored it falls back
-- to Run automatically, the most permissive mode that still asks.
UPDATE assistants
SET settings = jsonb_set(settings, '{permissions,mode}', '"auto"'::jsonb)
WHERE settings->'permissions'->>'mode' = 'full';
