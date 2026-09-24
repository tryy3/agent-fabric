package gitrepo_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/gitrepo"
	"github.com/tryy3/agent-fabric/internal/sandbox/local"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

func TestEnsureRepoInitsMainAndSeedsGitignore(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	env, err := local.New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = env.Close(context.Background()) })
	execu, _ := env.Exec()
	fsys, _ := env.FS()
	ctx := context.Background()

	if err := gitrepo.EnsureRepo(ctx, execu, fsys); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Fatalf(".git missing: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{".DS_Store", ".env", "*.pem"} {
		if !strings.Contains(got, want) {
			t.Fatalf("gitignore missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "index.html") || strings.Contains(got, "*.js") {
		t.Fatalf("gitignore should not ignore user source:\n%s", got)
	}

	if err := gitrepo.EnsureRepo(ctx, execu, fsys); err != nil {
		t.Fatalf("idempotent EnsureRepo: %v", err)
	}
}

func TestCommitIfDirtyRestoresAndDiffsWithoutTranscripts(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	env, err := local.New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = env.Close(context.Background()) })
	execu, _ := env.Exec()
	fsys, _ := env.FS()
	ctx := context.Background()
	if err := gitrepo.EnsureRepo(ctx, execu, fsys); err != nil {
		t.Fatal(err)
	}

	if _, committed, err := gitrepo.CommitIfDirty(ctx, execu, "empty"); err != nil {
		t.Fatal(err)
	} else if committed {
		t.Fatal("empty tree should not create a commit")
	}

	if err := fsys.WriteFile(ctx, "index.html", []byte("<h1>one</h1>")); err != nil {
		t.Fatal(err)
	}
	sha1, committed, err := gitrepo.CommitIfDirty(ctx, execu, "agent: Landing (abc1234)")
	if err != nil {
		t.Fatal(err)
	}
	if !committed || sha1 == "" {
		t.Fatalf("first commit sha=%q committed=%v", sha1, committed)
	}

	if err := fsys.WriteFile(ctx, "index.html", []byte("<h1>rewrite</h1>")); err != nil {
		t.Fatal(err)
	}
	if err := fsys.WriteFile(ctx, "messages.json", []byte(`should not be a transcript policy file`)); err != nil {
		t.Fatal(err)
	}
	sha2, committed, err := gitrepo.CommitIfDirty(ctx, execu, "agent: Landing (def5678)")
	if err != nil {
		t.Fatal(err)
	}
	if !committed {
		t.Fatal("rewrite should commit")
	}

	commits, err := gitrepo.Log(ctx, execu)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) < 2 {
		t.Fatalf("commits = %+v", commits)
	}
	if commits[0].SHA != sha2 || !strings.Contains(commits[0].Message, "agent: Landing") {
		t.Fatalf("latest commit %+v", commits[0])
	}

	diff, err := gitrepo.Diff(ctx, execu, sha1, sha2)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "-<h1>one</h1>") || !strings.Contains(diff, "+<h1>rewrite</h1>") {
		t.Fatalf("diff = %s", diff)
	}

	if err := gitrepo.CheckoutForce(ctx, execu, sha1); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "<h1>one</h1>" {
		t.Fatalf("restored index.html = %q", got)
	}

	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Name()), "transcript") || e.Name() == "thoughts.json" {
			t.Fatalf("repo should not contain transcripts: %s", e.Name())
		}
	}
}

func TestAgentCommitMessageUsesTitleAndPromptSHA(t *testing.T) {
	msg := gitrepo.AgentCommitMessage("Landing page", "th_abc", "rewrite the hero")
	if !strings.HasPrefix(msg, "agent: Landing page (") {
		t.Fatalf("message = %q", msg)
	}
	if strings.Contains(msg, "rewrite the hero") {
		t.Fatalf("must not embed the prompt text: %q", msg)
	}
	empty := gitrepo.AgentCommitMessage("", "th_abc", "hi")
	if !strings.HasPrefix(empty, "agent: th_abc (") {
		t.Fatalf("fallback message = %q", empty)
	}
}

func TestAnnotatedCheckpointTag(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	env, err := local.New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = env.Close(context.Background()) })
	execu, _ := env.Exec()
	fsys, _ := env.FS()
	ctx := context.Background()
	if err := gitrepo.EnsureRepo(ctx, execu, fsys); err != nil {
		t.Fatal(err)
	}
	if err := fsys.WriteFile(ctx, "index.html", []byte("v1")); err != nil {
		t.Fatal(err)
	}
	sha, _, err := gitrepo.CommitIfDirty(ctx, execu, "agent: t (aaaaaaa)")
	if err != nil {
		t.Fatal(err)
	}
	if err := gitrepo.TagAnnotated(ctx, execu, "checkpoint/chk_test", "before rewrite"); err != nil {
		t.Fatal(err)
	}
	got, err := gitrepo.RevParse(ctx, execu, "checkpoint/chk_test")
	if err != nil {
		t.Fatal(err)
	}
	if got != sha {
		t.Fatalf("tag sha %q want %q", got, sha)
	}
}
