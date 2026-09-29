-- +goose Up
-- Rename catalog tables and columns to Assistant / InferenceConnection terminology.
-- Also rewrite JSON keys: allowedAgents → allowedAssistants, workspaceRoot → projectRoot.

ALTER TABLE providers RENAME TO inference_connections;

ALTER TABLE agents RENAME TO assistants;
ALTER TABLE assistants RENAME COLUMN provider_id TO inference_connection_id;
ALTER INDEX IF EXISTS agents_provider_id_idx RENAME TO assistants_inference_connection_id_idx;

ALTER TABLE threads RENAME COLUMN agent_id TO assistant_id;

-- Project settings: allowedAgents → allowedAssistants
UPDATE projects
SET settings = (settings - 'allowedAgents') || jsonb_build_object('allowedAssistants', settings->'allowedAgents')
WHERE settings ? 'allowedAgents';

-- Assistant settings: workspaceRoot → projectRoot (top-level legacy keys)
UPDATE assistants
SET settings = (settings - 'workspaceRoot') || jsonb_build_object('projectRoot', settings->'workspaceRoot')
WHERE settings ? 'workspaceRoot';

-- Project settings: workspaceRoot at top level
UPDATE projects
SET settings = (settings - 'workspaceRoot') || jsonb_build_object('projectRoot', settings->'workspaceRoot')
WHERE settings ? 'workspaceRoot';

-- Project settings.environment.workspaceRoot → projectRoot
UPDATE projects
SET settings = jsonb_set(
  settings - 'environment',
  '{environment}',
  ((settings->'environment') - 'workspaceRoot') || jsonb_build_object('projectRoot', settings->'environment'->'workspaceRoot')
)
WHERE settings ? 'environment'
  AND jsonb_typeof(settings->'environment') = 'object'
  AND (settings->'environment') ? 'workspaceRoot';

-- Project settings.sandbox.workspaceRoot → projectRoot (legacy overlay bag)
UPDATE projects
SET settings = jsonb_set(
  settings - 'sandbox',
  '{sandbox}',
  ((settings->'sandbox') - 'workspaceRoot') || jsonb_build_object('projectRoot', settings->'sandbox'->'workspaceRoot')
)
WHERE settings ? 'sandbox'
  AND jsonb_typeof(settings->'sandbox') = 'object'
  AND (settings->'sandbox') ? 'workspaceRoot';

-- Plane settings environment.workspaceRoot → projectRoot
UPDATE plane_settings
SET environment = (environment - 'workspaceRoot') || jsonb_build_object('projectRoot', environment->'workspaceRoot')
WHERE environment ? 'workspaceRoot';

-- Plane settings sandbox.workspaceRoot → projectRoot
UPDATE plane_settings
SET sandbox = (sandbox - 'workspaceRoot') || jsonb_build_object('projectRoot', sandbox->'workspaceRoot')
WHERE sandbox ? 'workspaceRoot';
