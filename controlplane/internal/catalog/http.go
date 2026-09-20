package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

type errorBody struct {
	Error string `json:"error"`
}

type providerCreate struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKey"`
}

type providerPatch struct {
	Name    *string `json:"name"`
	BaseURL *string `json:"baseUrl"`
	APIKey  *string `json:"apiKey"`
}

type agentCreate struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	ProviderID   string `json:"providerId"`
	DefaultModel string `json:"defaultModel"`
}

type agentPatch struct {
	Name         *string         `json:"name"`
	Description  *string         `json:"description"`
	ProviderID   *string         `json:"providerId"`
	DefaultModel *string         `json:"defaultModel"`
	Settings     json.RawMessage `json:"settings"`
}

type threadCreate struct {
	ProjectID string `json:"projectId"`
}

type threadPatch struct {
	Title      *string        `json:"title"`
	ViewModeID optionalString `json:"viewModeId"`
}

type projectCreate struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Isolation   string `json:"isolation"`
}

type projectPatch struct {
	Name        *string         `json:"name"`
	Description *string         `json:"description"`
	Isolation   *string         `json:"isolation"`
	Settings    json.RawMessage `json:"settings"`
}

// Hooks are optional catalog HTTP side effects. Git init on project create is
// wired from the server so catalog tests stay hermetic.
type Hooks struct {
	AfterCreateProject func(ctx context.Context, project Project) error
}

// Handler serves the catalog HTTP API. POST create responses use 201 Created.
func Handler(store *Store) http.Handler {
	return HandlerWithHooks(store, Hooks{})
}

func HandlerWithHooks(store *Store, hooks Hooks) http.Handler {
	mux := http.NewServeMux()
	h := &httpAPI{store: store, hooks: hooks}

	mux.HandleFunc("GET /v1/providers", h.listProviders)
	mux.HandleFunc("POST /v1/providers", h.createProvider)
	mux.HandleFunc("GET /v1/providers/{id}", h.getProvider)
	mux.HandleFunc("PATCH /v1/providers/{id}", h.patchProvider)
	mux.HandleFunc("DELETE /v1/providers/{id}", h.deleteProvider)
	mux.HandleFunc("POST /v1/providers/{id}/models/refresh", h.refreshModels)

	mux.HandleFunc("GET /v1/agents", h.listAgents)
	mux.HandleFunc("POST /v1/agents", h.createAgent)
	mux.HandleFunc("GET /v1/agents/{id}", h.getAgent)
	mux.HandleFunc("PATCH /v1/agents/{id}", h.patchAgent)
	mux.HandleFunc("DELETE /v1/agents/{id}", h.deleteAgent)

	mux.HandleFunc("GET /v1/threads", h.listThreads)
	mux.HandleFunc("POST /v1/threads", h.createThread)
	mux.HandleFunc("GET /v1/threads/{id}", h.getThread)
	mux.HandleFunc("PATCH /v1/threads/{id}", h.patchThread)

	mux.HandleFunc("GET /v1/projects", h.listProjects)
	mux.HandleFunc("POST /v1/projects", h.createProject)
	mux.HandleFunc("GET /v1/projects/{id}", h.getProject)
	mux.HandleFunc("PATCH /v1/projects/{id}", h.patchProject)
	mux.HandleFunc("DELETE /v1/projects/{id}", h.deleteProject)

	mux.HandleFunc("GET /v1/settings", h.getSettings)
	mux.HandleFunc("PATCH /v1/settings", h.patchSettings)

	return mux
}

type httpAPI struct {
	store *Store
	hooks Hooks
}

func (h *httpAPI) listProviders(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListProviders(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *httpAPI) createProvider(w http.ResponseWriter, r *http.Request) {
	var body providerCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := h.store.CreateProvider(r.Context(), body.Name, body.Type, body.BaseURL, body.APIKey)
	if err != nil {
		writeMappedError(w, err, "")
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *httpAPI) getProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := h.store.GetProvider(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrProviderNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *httpAPI) patchProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body providerPatch
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := h.store.UpdateProvider(r.Context(), id, body.Name, body.BaseURL, body.APIKey)
	if err != nil {
		writeMappedError(w, err, id)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *httpAPI) deleteProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.store.DeleteProvider(r.Context(), id); err != nil {
		writeMappedError(w, err, id)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *httpAPI) refreshModels(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := h.store.RefreshModels(r.Context(), id, nil)
	if err != nil {
		if errors.Is(err, ErrProviderNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if isUpstreamRefreshError(err) {
			slog.Error("catalog models refresh failed", "provider", id, "err", err)
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *httpAPI) listAgents(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListAgents(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *httpAPI) createAgent(w http.ResponseWriter, r *http.Request) {
	var body agentCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	a, err := h.store.CreateAgent(r.Context(), body.Name, body.Description, body.ProviderID, body.DefaultModel)
	if err != nil {
		writeMappedError(w, err, "")
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (h *httpAPI) getAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a, err := h.store.GetAgent(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrAgentNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (h *httpAPI) patchAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body agentPatch
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	a, err := h.store.UpdateAgent(r.Context(), id, body.Name, body.Description, body.ProviderID, body.DefaultModel, body.Settings)
	if err != nil {
		writeMappedError(w, err, id)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (h *httpAPI) deleteAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.store.DeleteAgent(r.Context(), id); err != nil {
		writeMappedError(w, err, id)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *httpAPI) listThreads(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListThreads(r.Context(), r.URL.Query().Get("projectId"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []ThreadListItem{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *httpAPI) createThread(w http.ResponseWriter, r *http.Request) {
	var body threadCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var (
		th  Thread
		err error
	)
	if strings.TrimSpace(body.ProjectID) == "" {
		th, err = h.store.CreateThread(r.Context())
	} else {
		th, err = h.store.CreateThreadForProject(r.Context(), body.ProjectID)
	}
	if err != nil {
		writeMappedError(w, err, "")
		return
	}
	writeJSON(w, http.StatusCreated, th)
}

func (h *httpAPI) getThread(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	detail, err := h.store.GetThread(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrThreadNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if detail.Messages == nil {
		detail.Messages = make([]ThreadMessage, 0)
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h *httpAPI) patchThread(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body threadPatch
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Title == nil && !body.ViewModeID.Present {
		writeError(w, http.StatusBadRequest, "empty patch")
		return
	}
	var th Thread
	var err error
	if body.Title != nil {
		th, err = h.store.RenameThread(r.Context(), id, *body.Title)
		if err != nil {
			writeMappedError(w, err, id)
			return
		}
	}
	if body.ViewModeID.Present {
		if body.ViewModeID.Value != nil && !ValidViewModeID(*body.ViewModeID.Value) {
			writeError(w, http.StatusBadRequest, "invalid viewModeId")
			return
		}
		th, err = h.store.SetThreadViewMode(r.Context(), id, body.ViewModeID.Value)
		if err != nil {
			writeMappedError(w, err, id)
			return
		}
	}
	writeJSON(w, http.StatusOK, th)
}

func (h *httpAPI) listProjects(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListProjects(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []Project{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *httpAPI) createProject(w http.ResponseWriter, r *http.Request) {
	var body projectCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := h.store.CreateProject(r.Context(), body.Name, body.Description, body.Isolation)
	if err != nil {
		writeMappedError(w, err, "")
		return
	}
	if h.hooks.AfterCreateProject != nil {
		if hookErr := h.hooks.AfterCreateProject(r.Context(), p); hookErr != nil {
			slog.Error("git init after project create failed", "project", p.ID, "err", hookErr)
		}
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *httpAPI) getProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := h.store.GetProject(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *httpAPI) patchProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body projectPatch
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Name == nil && body.Description == nil && body.Isolation == nil && len(body.Settings) == 0 {
		writeError(w, http.StatusBadRequest, "empty patch")
		return
	}
	p, err := h.store.UpdateProject(r.Context(), id, body.Name, body.Description, body.Isolation, body.Settings)
	if err != nil {
		writeMappedError(w, err, id)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *httpAPI) deleteProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.store.DeleteProject(r.Context(), id); err != nil {
		writeMappedError(w, err, id)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type settingsPatch struct {
	Sandbox json.RawMessage `json:"sandbox"`
}

func (h *httpAPI) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.store.GetPlaneSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (h *httpAPI) patchSettings(w http.ResponseWriter, r *http.Request) {
	var body settingsPatch
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(body.Sandbox) == 0 {
		writeError(w, http.StatusBadRequest, "sandbox patch is required")
		return
	}
	settings, err := h.store.PatchPlaneSettings(r.Context(), body.Sandbox)
	if err != nil {
		writeMappedError(w, err, "")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorBody{Error: msg})
}

func writeMappedError(w http.ResponseWriter, err error, _ string) {
	if errors.Is(err, ErrAgentInUse) || errors.Is(err, ErrProjectInUse) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if errors.Is(err, ErrProviderNotFound) || errors.Is(err, ErrAgentNotFound) || errors.Is(err, ErrThreadNotFound) || errors.Is(err, ErrProjectNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}

func isUpstreamRefreshError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "fetch models") ||
		strings.Contains(msg, "models HTTP") ||
		strings.Contains(msg, "decode models") ||
		strings.Contains(msg, "create models request")
}
