-- +goose Up
CREATE TABLE threads (
    id            text PRIMARY KEY,
    title         text NOT NULL DEFAULT 'Untitled',
    title_source  text NOT NULL CHECK (title_source IN ('auto', 'user')),
    agent_id      text REFERENCES agents(id) ON DELETE RESTRICT,
    current_model text,
    created_at    timestamptz NOT NULL,
    updated_at    timestamptz NOT NULL
);

CREATE TABLE messages (
    id         text PRIMARY KEY,
    thread_id  text NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    role       text NOT NULL CHECK (role IN ('user', 'assistant')),
    content    text NOT NULL,
    position   int NOT NULL,
    created_at timestamptz NOT NULL,
    UNIQUE (thread_id, position)
);

CREATE INDEX messages_thread_id_position_idx ON messages (thread_id, position);

-- +goose Down
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS threads;
