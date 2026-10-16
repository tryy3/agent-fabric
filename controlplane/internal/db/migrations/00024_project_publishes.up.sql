-- +goose Up
-- One row per successful publish of a project's files to a publish provider
-- (Netlify today). site_url is the provider's always-latest link at publish
-- time; deploy_url is the link pinned to this publish.
CREATE TABLE project_publishes (
    id          text PRIMARY KEY,
    project_id  text NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    provider    text NOT NULL,
    -- Provider-side destination (Netlify site id), so one provider can hold
    -- more than one target per project later.
    target      text NOT NULL DEFAULT '',
    deploy_id   text NOT NULL DEFAULT '',
    site_url    text NOT NULL DEFAULT '',
    deploy_url  text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL
);

CREATE INDEX project_publishes_project_id_idx
    ON project_publishes (project_id, created_at DESC);
