package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
)

func testContainerSpec(volumeIDs ...string) json.RawMessage {
	volumes := make([]map[string]any, 0, len(volumeIDs))
	for i, vid := range volumeIDs {
		volumes = append(volumes, map[string]any{
			"id": vid, "enabled": true, "name": fmt.Sprintf("disk%d", i), "target": fmt.Sprintf("/mnt%d", i),
			"whitelisted": true, "read": true, "write": true, "exec": true,
		})
	}
	raw, _ := json.Marshal(map[string]any{
		"image": "alpine:3.20", "containerName": "work", "volumes": volumes,
	})
	return raw
}

func insertResourceWithID(t *testing.T, queries *db.Queries, id string, spec json.RawMessage) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	ts := pgtype.Timestamptz{Time: now, Valid: true}
	_, err := queries.InsertResource(ctx, db.InsertResourceParams{
		ID:        id,
		Name:      "Test",
		Kind:      catalog.KindContainer,
		Spec:      spec,
		CreatedAt: ts,
		UpdatedAt: ts,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestEnvironmentPatchMergesGrantsByVolumeID(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Open(t)
	queries := db.New(pool)
	spec := testContainerSpec("vol_0123456789abcdef", "vol_0123456789abcd00")
	insertResourceWithID(t, queries, "res_aaaaaaaaaaaaaaaa", spec)
	store := catalog.Open(pool)
	if _, err := store.GetPlaneSettings(ctx); err != nil {
		t.Fatal(err)
	}

	seed := json.RawMessage(`{"resourceId":"res_aaaaaaaaaaaaaaaa","grants":[{"volumeId":"vol_0123456789abcdef","read":true}]}`)
	if _, err := store.PatchPlaneSettings(ctx, nil, seed); err != nil {
		t.Fatal(err)
	}

	got, err := store.PatchPlaneSettings(ctx, nil, json.RawMessage(`{"grants":[{"volumeId":"vol_0123456789abcdef","write":false}]}`))
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		Grants []struct {
			VolumeID string `json:"volumeId"`
			Read     *bool  `json:"read"`
			Write    *bool  `json:"write"`
		} `json:"grants"`
	}
	if err := json.Unmarshal(got.Environment, &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Grants) != 1 {
		t.Fatalf("grants = %+v", env.Grants)
	}
	if env.Grants[0].VolumeID != "vol_0123456789abcdef" || env.Grants[0].Read == nil || !*env.Grants[0].Read ||
		env.Grants[0].Write == nil || *env.Grants[0].Write {
		t.Fatalf("merged grant = %+v", env.Grants[0])
	}

	got, err = store.PatchPlaneSettings(ctx, nil, json.RawMessage(`{"grants":[{"volumeId":"vol_0123456789abcd00","read":false}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(got.Environment, &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Grants) != 2 {
		t.Fatalf("grants after append = %+v", env.Grants)
	}

	got, err = store.PatchPlaneSettings(ctx, nil, json.RawMessage(`{"grants":[null]}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(got.Environment, &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Grants) != 0 {
		t.Fatalf("grants should be deleted: %+v", env.Grants)
	}
}

func TestDeleteResourceInUse(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	spec := testContainerSpec("vol_0123456789abcdef")
	res, err := store.CreateResource(ctx, "Work", catalog.KindContainer, spec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetPlaneSettings(ctx); err != nil {
		t.Fatal(err)
	}

	envPatch, _ := json.Marshal(map[string]string{"resourceId": res.ID})
	if _, err := store.PatchPlaneSettings(ctx, nil, envPatch); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteResource(ctx, res.ID); !errors.Is(err, catalog.ErrResourceInUse) {
		t.Fatalf("delete in use (plane): %v", err)
	}

	if _, err := store.PatchPlaneSettings(ctx, nil, json.RawMessage(`{"resourceId":null}`)); err != nil {
		t.Fatal(err)
	}
	project, err := store.CreateProject(ctx, "App", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateProject(ctx, project.ID, nil, nil, nil, json.RawMessage(`{"environment":{"resourceId":"`+res.ID+`"}}`), nil); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteResource(ctx, res.ID); !errors.Is(err, catalog.ErrResourceInUse) {
		t.Fatalf("delete in use (project): %v", err)
	}

	if _, err := store.UpdateProject(ctx, project.ID, nil, nil, nil, json.RawMessage(`{"environment":null}`), nil); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteResource(ctx, res.ID); err != nil {
		t.Fatalf("delete after clear: %v", err)
	}
}

func TestRejectsUnknownResourceAndVolume(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	if _, err := store.GetPlaneSettings(ctx); err != nil {
		t.Fatal(err)
	}

	_, err := store.PatchPlaneSettings(ctx, nil, json.RawMessage(`{"resourceId":"res_missing"}`))
	if err == nil || !strings.Contains(err.Error(), `resource "res_missing" not found`) {
		t.Fatalf("unknown resource: %v", err)
	}

	spec := testContainerSpec("vol_0123456789abcdef")
	res, err := store.CreateResource(ctx, "Work", catalog.KindContainer, spec)
	if err != nil {
		t.Fatal(err)
	}
	envPatch, _ := json.Marshal(map[string]string{"resourceId": res.ID})
	if _, err := store.PatchPlaneSettings(ctx, nil, envPatch); err != nil {
		t.Fatal(err)
	}

	_, err = store.PatchPlaneSettings(ctx, nil, json.RawMessage(`{"grants":[{"volumeId":"vol_not_on_resource","read":true}]}`))
	if err == nil || !strings.Contains(err.Error(), `unknown volume "vol_not_on_resource"`) {
		t.Fatalf("unknown volume: %v", err)
	}

	if _, err := store.PatchPlaneSettings(ctx, nil, json.RawMessage(`{"resourceId":null}`)); err != nil {
		t.Fatal(err)
	}
	_, err = store.PatchPlaneSettings(ctx, nil, json.RawMessage(`{"grants":[{"volumeId":"vol_0123456789abcdef","read":true}]}`))
	if err == nil || !strings.Contains(err.Error(), "no resource selected") {
		t.Fatalf("no resource: %v", err)
	}
}
