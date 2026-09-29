-- +goose Down
ALTER TABLE plane_settings
    DROP COLUMN IF EXISTS web_search_integration_id,
    DROP COLUMN IF EXISTS fetch_page_integration_id;

DROP TABLE IF EXISTS tool_integrations;
