-- +goose Up
CREATE TABLE environments (
    id          text PRIMARY KEY,
    name        text NOT NULL,
    kind        text NOT NULL CHECK (kind IN ('local', 'docker')),
    spec        jsonb NOT NULL DEFAULT '{}'::jsonb,
    volume_name text,
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL
);

CREATE TABLE projects (
    id             text PRIMARY KEY,
    name           text NOT NULL,
    description    text NOT NULL DEFAULT '',
    isolation      text NOT NULL DEFAULT 'isolated' CHECK (isolation IN ('isolated', 'shared')),
    environment_id text REFERENCES environments(id) ON DELETE RESTRICT,
    settings       jsonb NOT NULL DEFAULT '{}'::jsonb,
    remotes        jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at     timestamptz NOT NULL,
    updated_at     timestamptz NOT NULL
);

INSERT INTO projects (id, name, description, isolation, settings, remotes, created_at, updated_at)
VALUES (
    'proj_' || substr(replace(gen_random_uuid()::text, '-', ''), 1, 16),
    'Personal',
    '',
    'isolated',
    '{}'::jsonb,
    '[]'::jsonb,
    now(),
    now()
);

ALTER TABLE threads
    ADD COLUMN project_id text REFERENCES projects(id) ON DELETE RESTRICT;

UPDATE threads
SET project_id = (SELECT id FROM projects WHERE name = 'Personal' ORDER BY created_at ASC LIMIT 1)
WHERE project_id IS NULL;

ALTER TABLE threads
    ALTER COLUMN project_id SET NOT NULL;

CREATE INDEX threads_project_id_idx ON threads (project_id);
