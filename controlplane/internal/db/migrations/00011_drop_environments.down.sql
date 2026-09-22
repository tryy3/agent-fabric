-- +goose Down
CREATE TABLE environments (
    id          text PRIMARY KEY,
    name        text NOT NULL,
    kind        text NOT NULL CHECK (kind IN ('local', 'docker')),
    spec        jsonb NOT NULL DEFAULT '{}'::jsonb,
    volume_name text,
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL
);

ALTER TABLE projects
    ADD COLUMN isolation text NOT NULL DEFAULT 'isolated' CHECK (isolation IN ('isolated', 'shared'));

ALTER TABLE projects
    ADD COLUMN environment_id text REFERENCES environments(id) ON DELETE RESTRICT;
