package catalog_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
)

func TestCreateAssistantRejectsGateInstructions(t *testing.T) {
	store := openGooseStore(t)
	srv := httptest.NewServer(catalog.Handler(store))
	defer srv.Close()
	p, err := store.CreateInferenceConnection(context.Background(), "P", catalog.TypeOpenAICompatible, "http://x/v1", "k")
	if err != nil {
		t.Fatal(err)
	}
	body := `{"name":"A","inferenceConnectionId":"` + p.ID + `","defaultModel":"m1","gateInstructions":"Never push."}`
	status, out := gateHTTP(t, http.MethodPost, srv.URL+"/v1/assistants", body)
	if status != http.StatusBadRequest || !strings.Contains(string(out), "gateInstructions") || !strings.Contains(string(out), "PATCH") {
		t.Fatalf("create with gateInstructions = %d %s, want a 400 naming the field and PATCH", status, out)
	}
	list, err := store.ListAssistants(context.Background())
	if err != nil || len(list) != 0 {
		t.Fatalf("nothing is created: %d assistants, %v", len(list), err)
	}
}

// A database error reading the plane's gate instructions must reach the
// caller: session/new would otherwise pin no gate instructions at all.
func TestPlaneGateInstructionsReportsDatabaseErrors(t *testing.T) {
	ctx := context.Background()
	url := dbtest.Start(t)
	if err := db.Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := db.OpenPool(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	store := catalog.Open(pool)
	if _, err := store.EnsurePlaneSettings(ctx, catalog.DeprecatedSandbox{}); err != nil {
		t.Fatal(err)
	}
	rules := "Never push."
	if _, err := store.PatchPlaneSettingsFull(ctx, catalog.PlaneSettingsPatch{GateInstructions: &rules}); err != nil {
		t.Fatal(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `LOCK TABLE plane_settings IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	short, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	if got, err := store.PlaneGateInstructions(short); err == nil {
		t.Fatalf("a failed read returned %q and no error", got)
	}
	_ = tx.Rollback(ctx)
	if got, err := store.PlaneGateInstructions(ctx); err != nil || got != rules {
		t.Fatalf("after the lock: %q, %v", got, err)
	}
}
