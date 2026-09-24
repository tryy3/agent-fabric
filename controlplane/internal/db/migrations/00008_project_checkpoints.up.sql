-- +goose Up
CREATE TABLE project_checkpoints (
    id          text PRIMARY KEY,
    project_id  text NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    sha         text NOT NULL,
    label       text NOT NULL,
    thread_id   text REFERENCES threads(id) ON DELETE SET NULL,
    message_id  text,
    created_at  timestamptz NOT NULL
);

CREATE INDEX project_checkpoints_project_id_idx
    ON project_checkpoints (project_id, created_at DESC);
