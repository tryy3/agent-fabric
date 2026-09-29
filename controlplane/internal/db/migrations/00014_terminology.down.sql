-- +goose Down
UPDATE plane_settings
SET sandbox = (sandbox - 'projectRoot') || jsonb_build_object('workspaceRoot', sandbox->'projectRoot')
WHERE sandbox ? 'projectRoot';

UPDATE plane_settings
SET environment = (environment - 'projectRoot') || jsonb_build_object('workspaceRoot', environment->'projectRoot')
WHERE environment ? 'projectRoot';

UPDATE projects
SET settings = jsonb_set(
  settings - 'sandbox',
  '{sandbox}',
  ((settings->'sandbox') - 'projectRoot') || jsonb_build_object('workspaceRoot', settings->'sandbox'->'projectRoot')
)
WHERE settings ? 'sandbox'
  AND jsonb_typeof(settings->'sandbox') = 'object'
  AND (settings->'sandbox') ? 'projectRoot';

UPDATE projects
SET settings = jsonb_set(
  settings - 'environment',
  '{environment}',
  ((settings->'environment') - 'projectRoot') || jsonb_build_object('workspaceRoot', settings->'environment'->'projectRoot')
)
WHERE settings ? 'environment'
  AND jsonb_typeof(settings->'environment') = 'object'
  AND (settings->'environment') ? 'projectRoot';

UPDATE projects
SET settings = (settings - 'projectRoot') || jsonb_build_object('workspaceRoot', settings->'projectRoot')
WHERE settings ? 'projectRoot';

UPDATE assistants
SET settings = (settings - 'projectRoot') || jsonb_build_object('workspaceRoot', settings->'projectRoot')
WHERE settings ? 'projectRoot';

UPDATE projects
SET settings = (settings - 'allowedAssistants') || jsonb_build_object('allowedAgents', settings->'allowedAssistants')
WHERE settings ? 'allowedAssistants';

ALTER TABLE threads RENAME COLUMN assistant_id TO agent_id;

ALTER INDEX IF EXISTS assistants_inference_connection_id_idx RENAME TO agents_provider_id_idx;
ALTER TABLE assistants RENAME COLUMN inference_connection_id TO provider_id;
ALTER TABLE assistants RENAME TO agents;

ALTER TABLE inference_connections RENAME TO providers;
