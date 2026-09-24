package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/gitrepo"
	"github.com/tryy3/agent-fabric/internal/sandbox"
)

type checkpointBody struct {
	Label     string `json:"label"`
	ThreadID  string `json:"threadId"`
	MessageID string `json:"messageId"`
}

type restoreBody struct {
	SHA string `json:"sha"`
}

// InitRepo runs git init + .gitignore seed inside the project environment.
func InitRepo(ctx context.Context, opener Opener, projectID string) error {
	if opener == nil {
		return fmt.Errorf("workspace opener is not configured")
	}
	env, err := opener.Open(ctx, projectID)
	if err != nil {
		return err
	}
	defer env.Close(ctx)
	return ensureGitRepo(ctx, env)
}

func ensureGitRepo(ctx context.Context, env sandbox.Environment) error {
	execu, ok := env.Exec()
	if !ok {
		return fmt.Errorf("git executor is unavailable")
	}
	fsys, _ := env.FS()
	return gitrepo.EnsureRepo(ctx, execu, fsys)
}

func (h *httpAPI) listCommits(w http.ResponseWriter, r *http.Request) {
	h.withGit(w, r, func(env sandbox.Environment, execu sandbox.Executor) {
		if err := ensureGitRepo(r.Context(), env); err != nil {
			writeMappedFSError(w, err)
			return
		}
		commits, err := gitrepo.Log(r.Context(), execu)
		if err != nil {
			writeMappedFSError(w, err)
			return
		}
		checkpoints := map[string]catalog.Checkpoint{}
		if h.store != nil {
			rows, listErr := h.store.ListCheckpoints(r.Context(), r.PathValue("id"))
			if listErr != nil {
				writeMappedFSError(w, listErr)
				return
			}
			for _, row := range rows {
				if _, ok := checkpoints[row.SHA]; !ok {
					checkpoints[row.SHA] = row
				}
			}
		}
		out := catalog.CommitList{Commits: make([]catalog.GitCommit, 0, len(commits))}
		for _, c := range commits {
			item := catalog.GitCommit{
				SHA:         c.SHA,
				Message:     c.Message,
				CommittedAt: c.CommittedAt,
			}
			if chk, ok := checkpoints[c.SHA]; ok {
				id := chk.ID
				label := chk.Label
				item.CheckpointID = &id
				item.Label = &label
			}
			out.Commits = append(out.Commits, item)
		}
		writeJSON(w, http.StatusOK, out)
	})
}

func (h *httpAPI) createCheckpoint(w http.ResponseWriter, r *http.Request) {
	var body checkpointBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	label := strings.TrimSpace(body.Label)
	if label == "" {
		writeError(w, http.StatusBadRequest, "label is required")
		return
	}
	h.withGit(w, r, func(env sandbox.Environment, execu sandbox.Executor) {
		if err := ensureGitRepo(r.Context(), env); err != nil {
			writeMappedFSError(w, err)
			return
		}
		message := "checkpoint: " + label
		sha, committed, err := gitrepo.CommitIfDirty(r.Context(), execu, message)
		if err != nil {
			writeMappedFSError(w, err)
			return
		}
		if !committed {
			sha, err = gitrepo.RevParse(r.Context(), execu, "HEAD")
			if err != nil {
				writeMappedFSError(w, err)
				return
			}
		}
		if h.store == nil {
			writeError(w, http.StatusInternalServerError, "checkpoint store is not configured")
			return
		}
		projectID := r.PathValue("id")
		chk, err := h.store.InsertCheckpoint(r.Context(), projectID, sha, label, optString(body.ThreadID), optString(body.MessageID))
		if err != nil {
			writeMappedFSError(w, err)
			return
		}
		if err := gitrepo.TagAnnotated(r.Context(), execu, "checkpoint/"+chk.ID, label); err != nil {
			writeMappedFSError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, chk)
	})
}

func (h *httpAPI) restore(w http.ResponseWriter, r *http.Request) {
	var body restoreBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sha := strings.TrimSpace(body.SHA)
	if sha == "" {
		writeError(w, http.StatusBadRequest, "sha is required")
		return
	}
	h.withGit(w, r, func(env sandbox.Environment, execu sandbox.Executor) {
		if err := ensureGitRepo(r.Context(), env); err != nil {
			writeMappedFSError(w, err)
			return
		}
		resolved, err := gitrepo.RevParse(r.Context(), execu, sha)
		if err != nil {
			writeMappedFSError(w, err)
			return
		}
		if err := gitrepo.CheckoutForce(r.Context(), execu, resolved); err != nil {
			writeMappedFSError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"sha": resolved})
	})
}

func (h *httpAPI) diff(w http.ResponseWriter, r *http.Request) {
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	if from == "" {
		writeError(w, http.StatusBadRequest, "from is required")
		return
	}
	h.withGit(w, r, func(env sandbox.Environment, execu sandbox.Executor) {
		if err := ensureGitRepo(r.Context(), env); err != nil {
			writeMappedFSError(w, err)
			return
		}
		text, err := gitrepo.Diff(r.Context(), execu, from, to)
		if err != nil {
			writeMappedFSError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, catalog.DiffResult{From: from, To: to, Diff: text})
	})
}

func (h *httpAPI) withGit(w http.ResponseWriter, r *http.Request, fn func(sandbox.Environment, sandbox.Executor)) {
	if h.opener == nil {
		writeError(w, http.StatusInternalServerError, "workspace opener is not configured")
		return
	}
	projectID := r.PathValue("id")
	env, err := h.opener.Open(r.Context(), projectID)
	if err != nil {
		writeMappedFSError(w, err)
		return
	}
	defer func() {
		if closeErr := env.Close(r.Context()); closeErr != nil {
			slog.Error("workspace git close failed", "project", projectID, "err", closeErr)
		}
	}()
	execu, ok := env.Exec()
	if !ok {
		writeError(w, http.StatusInternalServerError, "git executor is unavailable")
		return
	}
	fn(env, execu)
}

func optString(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}
