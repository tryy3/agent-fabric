-- +goose Up
CREATE TABLE hop_captures (
    id            text PRIMARY KEY,
    thread_id     text NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    message_id    text NULL REFERENCES messages(id) ON DELETE CASCADE,
    session_id    text NULL,
    round_index   int NOT NULL,
    hop_kind      text NOT NULL,
    direction     text NOT NULL,
    method        text NULL,
    url           text NULL,
    status_code   int NULL,
    headers_json  jsonb NOT NULL,
    body_text     text NOT NULL,
    meta_json     jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX hop_captures_thread_message_idx
  ON hop_captures (thread_id, message_id);

CREATE INDEX hop_captures_thread_session_round_idx
  ON hop_captures (thread_id, session_id, round_index);
