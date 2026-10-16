-- +goose Up
-- live marks the deploy a provider target currently serves at its latest
-- link. It moves on each publish and is refreshed by publish sync, so a
-- rollback made on the provider shows up after a sync.
ALTER TABLE project_publishes ADD COLUMN live boolean NOT NULL DEFAULT false;

-- Before this column the newest publish of each target was the live one.
UPDATE project_publishes SET live = true
WHERE id IN (
    SELECT DISTINCT ON (project_id, provider, target) id
    FROM project_publishes
    ORDER BY project_id, provider, target, created_at DESC, id DESC
);

-- Sync matches provider deploys to stored rows by deploy id.
CREATE UNIQUE INDEX project_publishes_deploy_idx
    ON project_publishes (project_id, provider, target, deploy_id)
    WHERE deploy_id <> '';
