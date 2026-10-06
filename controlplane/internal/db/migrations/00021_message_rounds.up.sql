-- +goose Up
-- Plane-computed data for each LLM call of an assistant attempt. The message
-- parts stay what was exchanged with the provider; cost and the place in the
-- transcript where a round begins live here, linked to the message.
CREATE TABLE message_rounds (
    message_id          text NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    round_index         int NOT NULL,
    model               text NOT NULL DEFAULT '',
    -- Index into messages.parts of the first part this round produced.
    part_index          int NOT NULL,
    -- Counts the cost was calculated from.
    prompt_tokens       int NULL,
    completion_tokens   int NULL,
    cached_tokens       int NULL,
    cache_write_tokens  int NULL,
    reasoning_tokens    int NULL,
    -- Estimated cost in USD; NULL when the model has no published price.
    cost_input          double precision NULL,
    cost_cache_read     double precision NULL,
    cost_cache_write    double precision NULL,
    cost_output         double precision NULL,
    cost_reasoning      double precision NULL,
    cost_total          double precision NULL,
    cost_partial        boolean NOT NULL DEFAULT false,
    -- Cost the provider itself reported for this call, when it did.
    reported_cost_usd   double precision NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (message_id, round_index)
);
