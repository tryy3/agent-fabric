package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
)

func TestCreateResourcePrefixesAndRejectsDuplicateNames(t *testing.T) {
	ctx := context.Background()
	store := catalog.Open(dbtest.Open(t))
	store.IdentityPrefix = "dev-"

	spec := json.RawMessage(`{
  "image": "alpine:3.20",
  "containerName": "work",
  "volumes": [{
    "id": "vol_0123456789abcdef",
    "enabled": true,
    "name": "disk",
    "target": "/workspace",
    "whitelisted": true,
    "read": true,
    "write": true,
    "exec": true
  }]
}`)
	got, err := store.CreateResource(ctx, " Work ", catalog.KindContainer, spec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.ID, "res_") {
		t.Fatalf("id %q", got.ID)
	}
	if got.Name != "Work" {
		t.Fatalf("name %q", got.Name)
	}

	var stored struct {
		Image          string `json:"image"`
		ContainerName  string `json:"containerName"`
		IdleTTLSeconds int64  `json:"idleTTLSeconds"`
		Volumes        []struct {
			Name string `json:"name"`
		} `json:"volumes"`
	}
	if err := json.Unmarshal(got.Spec, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.ContainerName != "dev-work" {
		t.Fatalf("containerName %q", stored.ContainerName)
	}
	if len(stored.Volumes) != 1 || stored.Volumes[0].Name != "dev-disk" {
		t.Fatalf("volumes %+v", stored.Volumes)
	}
	if stored.IdleTTLSeconds != 3600 {
		t.Fatalf("idleTTLSeconds %d", stored.IdleTTLSeconds)
	}

	dupContainerSpec := json.RawMessage(`{
  "image": "alpine:3.20",
  "containerName": "dev-work",
  "volumes": [{
    "id": "vol_0123456789abcd00",
    "enabled": true,
    "name": "other",
    "target": "/data",
    "whitelisted": true,
    "read": true,
    "write": true,
    "exec": true
  }]
}`)
	_, err = store.CreateResource(ctx, "Other", catalog.KindContainer, dupContainerSpec)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate container: %v", err)
	}

	dupVolumeSpec := json.RawMessage(`{
  "image": "alpine:3.20",
  "containerName": "other-box",
  "volumes": [{
    "id": "vol_0123456789abcd01",
    "enabled": true,
    "name": "dev-disk",
    "target": "/data",
    "whitelisted": true,
    "read": true,
    "write": true,
    "exec": true
  }]
}`)
	_, err = store.CreateResource(ctx, "Other box", catalog.KindContainer, dupVolumeSpec)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate volume: %v", err)
	}

	_, err = store.CreateResource(ctx, "SSH", "ssh", spec)
	if err == nil || !strings.Contains(err.Error(), "container") {
		t.Fatalf("bad kind: %v", err)
	}

	badImage := json.RawMessage(`{"image":"","containerName":"x","volumes":[]}`)
	_, err = store.CreateResource(ctx, "Bad", catalog.KindContainer, badImage)
	if err == nil || !strings.Contains(err.Error(), "image") {
		t.Fatalf("empty image: %v", err)
	}

	_, err = store.GetResource(ctx, "res_missing")
	if err == nil || !errors.Is(err, catalog.ErrResourceNotFound) {
		t.Fatalf("missing: %v", err)
	}
}
