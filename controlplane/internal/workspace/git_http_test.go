package workspace_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db/dbtest"
	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/sandbox/local"
	"github.com/tryy3/agent-fabric/internal/workspace"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

func gitTestServer(t *testing.T) (*httptest.Server, string, string) {
	t.Helper()
	requireGit(t)
	root := t.TempDir()
	env, err := local.New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = env.Close(context.Background()) })
	store := catalog.Open(dbtest.Open(t))
	project, err := store.CreateProject(context.Background(), "Landing", "")
	if err != nil {
		t.Fatal(err)
	}
	opener := &mapOpener{envs: map[string]sandbox.Environment{
		project.ID: env,
	}}
	srv := httptest.NewServer(workspace.HandlerWithStore(opener, store))
	t.Cleanup(srv.Close)
	return srv, project.ID, root
}

func TestUserSaveDoesNotAutoCommit(t *testing.T) {
	srv, id, root := gitTestServer(t)

	put, err := http.NewRequest(http.MethodPut, srv.URL+"/v1/projects/"+id+"/files?path=index.html", strings.NewReader("<h1>draft</h1>"))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(put)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("put status %d", resp.StatusCode)
	}

	list, err := http.Get(srv.URL + "/v1/projects/" + id + "/commits")
	if err != nil {
		t.Fatal(err)
	}
	defer list.Body.Close()
	if list.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(list.Body)
		t.Fatalf("commits status %d body %s", list.StatusCode, body)
	}
	var commits catalog.CommitList
	if err := json.NewDecoder(list.Body).Decode(&commits); err != nil {
		t.Fatal(err)
	}
	for _, c := range commits.Commits {
		if strings.Contains(c.Message, "draft") || strings.Contains(strings.ToLower(c.Message), "index.html") {
			t.Fatalf("user save auto-committed: %+v", c)
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "<h1>draft</h1>" {
		t.Fatalf("file = %q", data)
	}
}

func TestCheckpointRestoreAndDiff(t *testing.T) {
	srv, id, root := gitTestServer(t)

	put, _ := http.NewRequest(http.MethodPut, srv.URL+"/v1/projects/"+id+"/files?path=index.html", strings.NewReader("<h1>one</h1>"))
	resp, err := http.DefaultClient.Do(put)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	chk1, err := http.Post(srv.URL+"/v1/projects/"+id+"/checkpoints", "application/json", strings.NewReader(`{"label":"before rewrite"}`))
	if err != nil {
		t.Fatal(err)
	}
	if chk1.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(chk1.Body)
		chk1.Body.Close()
		t.Fatalf("checkpoint status %d body %s", chk1.StatusCode, body)
	}
	var first catalog.Checkpoint
	if err := json.NewDecoder(chk1.Body).Decode(&first); err != nil {
		t.Fatal(err)
	}
	chk1.Body.Close()
	if first.Label != "before rewrite" || first.SHA == "" || !strings.HasPrefix(first.ID, "chk_") {
		t.Fatalf("checkpoint %+v", first)
	}

	put, _ = http.NewRequest(http.MethodPut, srv.URL+"/v1/projects/"+id+"/files?path=index.html", strings.NewReader("<h1>rewrite</h1>"))
	resp, err = http.DefaultClient.Do(put)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	chk2, err := http.Post(srv.URL+"/v1/projects/"+id+"/checkpoints", "application/json", strings.NewReader(`{"label":"after rewrite"}`))
	if err != nil {
		t.Fatal(err)
	}
	if chk2.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(chk2.Body)
		chk2.Body.Close()
		t.Fatalf("second checkpoint %d %s", chk2.StatusCode, body)
	}
	var second catalog.Checkpoint
	if err := json.NewDecoder(chk2.Body).Decode(&second); err != nil {
		t.Fatal(err)
	}
	chk2.Body.Close()

	diffResp, err := http.Get(srv.URL + "/v1/projects/" + id + "/diff?from=" + first.SHA + "&to=" + second.SHA)
	if err != nil {
		t.Fatal(err)
	}
	defer diffResp.Body.Close()
	if diffResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(diffResp.Body)
		t.Fatalf("diff status %d body %s", diffResp.StatusCode, body)
	}
	var diff catalog.DiffResult
	if err := json.NewDecoder(diffResp.Body).Decode(&diff); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff.Diff, "-<h1>one</h1>") || !strings.Contains(diff.Diff, "+<h1>rewrite</h1>") {
		t.Fatalf("diff = %s", diff.Diff)
	}

	restore, err := http.Post(srv.URL+"/v1/projects/"+id+"/restore", "application/json", bytes.NewReader([]byte(`{"sha":"`+first.SHA+`"}`)))
	if err != nil {
		t.Fatal(err)
	}
	if restore.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(restore.Body)
		restore.Body.Close()
		t.Fatalf("restore status %d body %s", restore.StatusCode, body)
	}
	restore.Body.Close()

	got, err := os.ReadFile(filepath.Join(root, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "<h1>one</h1>" {
		t.Fatalf("restored file = %q", got)
	}

	list, err := http.Get(srv.URL + "/v1/projects/" + id + "/commits")
	if err != nil {
		t.Fatal(err)
	}
	defer list.Body.Close()
	var commits catalog.CommitList
	if err := json.NewDecoder(list.Body).Decode(&commits); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range commits.Commits {
		if c.CheckpointID != nil && *c.CheckpointID == first.ID && c.Label != nil && *c.Label == "before rewrite" {
			found = true
		}
	}
	if !found {
		t.Fatalf("commits missing checkpoint label: %+v", commits.Commits)
	}
}

func TestGitRoutesAreNotACP(t *testing.T) {
	srv, id, _ := gitTestServer(t)
	resp, err := http.Get(srv.URL + "/v1/projects/" + id + "/commits")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("catalog git status %d", resp.StatusCode)
	}
	acpResp, err := http.Get(srv.URL + "/acp/fs")
	if err != nil {
		t.Fatal(err)
	}
	acpResp.Body.Close()
	if acpResp.StatusCode == http.StatusOK {
		t.Fatal("ACP must not serve git/fs")
	}
}
