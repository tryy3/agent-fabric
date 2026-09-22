package catalog_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
)

func TestBackfillIsolatedProjectKeepsPhase1Disk(t *testing.T) {
	ctx := context.Background()
	store, pool := openBackfillDB(t)

	projects, err := store.ListProjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("projects %d", len(projects))
	}
	project := projects[0]
	if project.Name != catalog.DefaultProjectName {
		t.Fatalf("project %q", project.Name)
	}

	if _, err := store.EnsurePlaneSettings(ctx, catalog.DeprecatedSandbox{}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.BackfillResources(ctx, pool, ""); err != nil {
		t.Fatal(err)
	}

	resources, err := store.ListResources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 {
		t.Fatalf("resources %d", len(resources))
	}
	spec := decodeBackfillSpec(t, resources[0].Spec)
	if spec.ContainerName != "agent-fabric-container-"+project.ID {
		t.Fatalf("containerName %q", spec.ContainerName)
	}
	workspace := workspaceVolumeName(t, spec)
	if workspace != "agent-fabric-vol-"+project.ID {
		t.Fatalf("workspace volume %q", workspace)
	}

	updated, err := store.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := settingsResourceID(t, updated.Settings); got != resources[0].ID {
		t.Fatalf("project resourceId %q, want %q", got, resources[0].ID)
	}

	settings, err := store.GetPlaneSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := environmentResourceID(t, settings.Environment); got != "" {
		t.Fatalf("global resourceId %q", got)
	}
	if !projectsEnvironmentIDColumnExists(t, pool) {
		t.Fatal("projects.environment_id column is missing")
	}
}

func TestBackfillSharedEnvironmentIsOneResource(t *testing.T) {
	ctx := context.Background()
	store, pool := openBackfillDB(t)
	if _, err := store.EnsurePlaneSettings(ctx, catalog.DeprecatedSandbox{}); err != nil {
		t.Fatal(err)
	}

	env, err := store.CreateEnvironment(ctx, "Shared", "docker", nil)
	if err != nil {
		t.Fatal(err)
	}
	older, err := store.CreateSharedProject(ctx, "Older", "", env.ID)
	if err != nil {
		t.Fatal(err)
	}
	newer, err := store.CreateSharedProject(ctx, "Newer", "", env.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(
		ctx,
		`UPDATE projects SET created_at = created_at - interval '1 minute' WHERE id = $1`,
		older.ID,
	); err != nil {
		t.Fatal(err)
	}
	olderPatch := json.RawMessage(`{"sandbox":{"containerName":"older-{projectID}"}}`)
	newerPatch := json.RawMessage(`{"sandbox":{"containerName":"newer-{projectID}"}}`)
	if _, err := store.UpdateProject(ctx, older.ID, nil, nil, nil, olderPatch); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateProject(ctx, newer.ID, nil, nil, nil, newerPatch); err != nil {
		t.Fatal(err)
	}

	if err := catalog.BackfillResources(ctx, pool, ""); err != nil {
		t.Fatal(err)
	}

	olderGot, err := store.GetProject(ctx, older.ID)
	if err != nil {
		t.Fatal(err)
	}
	newerGot, err := store.GetProject(ctx, newer.ID)
	if err != nil {
		t.Fatal(err)
	}
	olderResourceID := settingsResourceID(t, olderGot.Settings)
	newerResourceID := settingsResourceID(t, newerGot.Settings)
	if olderResourceID == "" || olderResourceID != newerResourceID {
		t.Fatalf("resource ids %q and %q", olderResourceID, newerResourceID)
	}

	resource, err := store.GetResource(ctx, olderResourceID)
	if err != nil {
		t.Fatal(err)
	}
	spec := decodeBackfillSpec(t, resource.Spec)
	if spec.ContainerName != "older-"+older.ID {
		t.Fatalf("containerName %q", spec.ContainerName)
	}
	workspace := workspaceVolumeName(t, spec)
	if workspace != "agent-fabric.env."+env.ID {
		t.Fatalf("workspace volume %q", workspace)
	}
}

func TestBackfillRejectsLocal(t *testing.T) {
	ctx := context.Background()
	store, pool := openBackfillDB(t)
	if _, err := store.EnsurePlaneSettings(ctx, catalog.DeprecatedSandbox{Kind: "local"}); err != nil {
		t.Fatal(err)
	}
	projects, err := store.ListProjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("projects %d", len(projects))
	}

	err = catalog.BackfillResources(ctx, pool, "")
	want := fmt.Sprintf("local sandbox cannot be migrated for project %q", projects[0].ID)
	if err == nil || err.Error() != want {
		t.Fatalf("err %v, want %s", err, want)
	}

	resources, listErr := store.ListResources(ctx)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(resources) != 0 {
		t.Fatalf("resources %d, want none after rollback", len(resources))
	}
}

func TestBackfillOmittedFlagsDenyExceptWrite(t *testing.T) {
	ctx := context.Background()
	store, pool := openBackfillDB(t)
	if _, err := store.EnsurePlaneSettings(ctx, catalog.DeprecatedSandbox{}); err != nil {
		t.Fatal(err)
	}
	projects, err := store.ListProjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("projects %d", len(projects))
	}
	patch := json.RawMessage(`{"sandbox":{"volumes":[{
		"id":"vol_cache",
		"enabled":true,
		"name":"cache",
		"target":"/cache"
	}]}}`)
	if _, err := store.UpdateProject(ctx, projects[0].ID, nil, nil, nil, patch); err != nil {
		t.Fatal(err)
	}
	if err := catalog.BackfillResources(ctx, pool, ""); err != nil {
		t.Fatal(err)
	}

	resources, err := store.ListResources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 {
		t.Fatalf("resources %d", len(resources))
	}
	spec := decodeBackfillSpec(t, resources[0].Spec)
	var found bool
	for _, volume := range spec.Volumes {
		if volume.Target != "/cache" {
			continue
		}
		found = true
		if volume.Whitelisted || volume.Read || !volume.Write || volume.Exec {
			t.Fatalf(
				"flags whitelisted=%v read=%v write=%v exec=%v",
				volume.Whitelisted,
				volume.Read,
				volume.Write,
				volume.Exec,
			)
		}
	}
	if !found {
		t.Fatal("missing /cache volume")
	}
}

func openBackfillDB(t *testing.T) (*catalog.Store, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	url := dbtest.Start(t)
	if err := db.MigrateTo(ctx, url, 10); err != nil {
		t.Fatal(err)
	}
	pool, err := db.OpenPool(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return catalog.Open(pool), pool
}

type backfillSpec struct {
	ContainerName string `json:"containerName"`
	Volumes       []struct {
		Name        string `json:"name"`
		Target      string `json:"target"`
		Whitelisted bool   `json:"whitelisted"`
		Read        bool   `json:"read"`
		Write       bool   `json:"write"`
		Exec        bool   `json:"exec"`
	} `json:"volumes"`
}

func decodeBackfillSpec(t *testing.T, raw json.RawMessage) backfillSpec {
	t.Helper()
	var spec backfillSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	return spec
}

func workspaceVolumeName(t *testing.T, spec backfillSpec) string {
	t.Helper()
	for _, volume := range spec.Volumes {
		if volume.Target == catalog.DefaultWorkspaceRoot {
			return volume.Name
		}
	}
	t.Fatalf("no volume targets %s", catalog.DefaultWorkspaceRoot)
	return ""
}

func settingsResourceID(t *testing.T, settings json.RawMessage) string {
	t.Helper()
	env, err := catalog.EnvironmentFromSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	return environmentResourceID(t, env)
}

func environmentResourceID(t *testing.T, env json.RawMessage) string {
	t.Helper()
	var bag struct {
		ResourceID string `json:"resourceId"`
	}
	if len(env) == 0 {
		return ""
	}
	if err := json.Unmarshal(env, &bag); err != nil {
		t.Fatal(err)
	}
	return bag.ResourceID
}

func projectsEnvironmentIDColumnExists(t *testing.T, pool *pgxpool.Pool) bool {
	t.Helper()
	var exists bool
	err := pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = 'projects'
			  AND column_name = 'environment_id'
		)`).Scan(&exists)
	if err != nil {
		t.Fatal(err)
	}
	return exists
}
