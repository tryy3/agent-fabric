-- +goose Up
CREATE TABLE tool_integrations (
    id                 text PRIMARY KEY,
    name               text NOT NULL,
    kind               text NOT NULL,
    enabled            boolean NOT NULL DEFAULT true,
    scope              text NOT NULL DEFAULT 'plane',
    endpoint           text NOT NULL DEFAULT '',
    mode               text NOT NULL DEFAULT 'external',
    capabilities       jsonb NOT NULL DEFAULT '[]'::jsonb,
    config             jsonb NOT NULL DEFAULT '{}'::jsonb,
    secrets            jsonb NOT NULL DEFAULT '{}'::jsonb,
    health_status      text NOT NULL DEFAULT 'unknown',
    health_checked_at  timestamptz,
    created_at         timestamptz NOT NULL,
    updated_at         timestamptz NOT NULL,
    CONSTRAINT tool_integrations_kind_check CHECK (
        kind IN ('searxng', 'linkup', 'get_md', 'crawl4ai')
    ),
    CONSTRAINT tool_integrations_scope_check CHECK (scope = 'plane'),
    CONSTRAINT tool_integrations_mode_check CHECK (mode IN ('bundled', 'external'))
);

CREATE INDEX tool_integrations_kind_idx ON tool_integrations (kind);

ALTER TABLE plane_settings
    ADD COLUMN web_search_integration_id text
        REFERENCES tool_integrations (id) ON DELETE SET NULL,
    ADD COLUMN fetch_page_integration_id text
        REFERENCES tool_integrations (id) ON DELETE SET NULL;
