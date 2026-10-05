package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tryy3/agent-fabric/internal/modelspecs"
	"io"
	"log/slog"
	"net/http"
	"strings"

	sandboxtools "github.com/tryy3/agent-fabric/internal/sandbox/tools"
)

type errorBody struct {
	Error string `json:"error"`
}

type inferenceConnectionCreate struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKey"`
}

type inferenceConnectionPatch struct {
	Name    *string `json:"name"`
	BaseURL *string `json:"baseUrl"`
	APIKey  *string `json:"apiKey"`
}

type assistantCreate struct {
	Name                  string `json:"name"`
	Description           string `json:"description"`
	Instructions          string `json:"instructions"`
	InferenceConnectionID string `json:"inferenceConnectionId"`
	DefaultModel          string `json:"defaultModel"`
}

type assistantPatch struct {
	Name                  *string         `json:"name"`
	Description           *string         `json:"description"`
	Instructions          *string         `json:"instructions"`
	InferenceConnectionID *string         `json:"inferenceConnectionId"`
	DefaultModel          *string         `json:"defaultModel"`
	Settings              json.RawMessage `json:"settings"`
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
}

type projectPatch struct {
	Name        *string         `json:"name"`
	Description *string         `json:"description"`
	Settings    json.RawMessage `json:"settings"`
	Remotes     json.RawMessage `json:"remotes"`
}

type resourceCreate struct {
	Name string          `json:"name"`
	Kind string          `json:"kind"`
	Spec json.RawMessage `json:"spec"`
}

type resourcePatch struct {
	Name *string         `json:"name"`
	Spec json.RawMessage `json:"spec"`
}

// Hooks are optional catalog HTTP side effects. Git init on project create is
// wired from the server so catalog tests stay hermetic.
type Hooks struct {
	AfterCreateProject func(ctx context.Context, project Project) error
	// Specs, when set, joins synced model specs onto served connections.
	Specs SpecsLookup
}

// SpecsLookup finds the model-specs provider for a connection.
type SpecsLookup interface {
	ProviderFor(connType, baseURL string) (modelspecs.Provider, bool)
}

// ModelPrices resolves per-million-token prices for each of a connection's
// models from the synced specs; models without a published price are omitted.
// The result is a snapshot: later syncs do not change it.
func (s *Store) ModelPrices(connType, baseURL string, models []ModelInfo) map[string]*modelspecs.Cost {
	if s.Specs == nil {
		return nil
	}
	prov, ok := s.Specs.ProviderFor(connType, baseURL)
	if !ok {
		return nil
	}
	out := make(map[string]*modelspecs.Cost)
	for _, m := range models {
		spec, found := prov.Lookup(m.ID)
		if !found || spec.Cost == nil || (spec.Cost.Input == nil && spec.Cost.Output == nil) {
			continue
		}
		c := *spec.Cost
		out[m.ID] = &c
	}
	return out
}

// withSpecs returns a copy of c with model specs joined. The stored
// connection is never modified.
func (h *httpAPI) withSpecs(c InferenceConnection) InferenceConnection {
	if h.hooks.Specs == nil {
		return c
	}
	prov, ok := h.hooks.Specs.ProviderFor(c.Type, c.BaseURL)
	if !ok {
		return c
	}
	ref := prov.Ref()
	c.SpecsProvider = &ref
	models := make([]ModelInfo, len(c.Models))
	for i, m := range c.Models {
		if spec, found := prov.Lookup(m.ID); found {
			m.Specs = &spec
		}
		models[i] = m
	}
	c.Models = models
	return c
}

// Handler serves the catalog HTTP API. POST create responses use 201 Created.
func Handler(store *Store) http.Handler {
	return HandlerWithHooks(store, Hooks{})
}

func HandlerWithHooks(store *Store, hooks Hooks) http.Handler {
	mux := http.NewServeMux()
	h := &httpAPI{store: store, hooks: hooks}

	mux.HandleFunc("GET /v1/inference/connections", h.listInferenceConnections)
	mux.HandleFunc("POST /v1/inference/connections", h.createInferenceConnection)
	mux.HandleFunc("GET /v1/inference/connections/{id}", h.getInferenceConnection)
	mux.HandleFunc("PATCH /v1/inference/connections/{id}", h.patchInferenceConnection)
	mux.HandleFunc("DELETE /v1/inference/connections/{id}", h.deleteInferenceConnection)
	mux.HandleFunc("POST /v1/inference/connections/{id}/models/refresh", h.refreshModels)

	mux.HandleFunc("GET /v1/resources", h.listResources)
	mux.HandleFunc("POST /v1/resources", h.createResource)
	mux.HandleFunc("GET /v1/resources/{id}", h.getResource)
	mux.HandleFunc("PATCH /v1/resources/{id}", h.patchResource)
	mux.HandleFunc("DELETE /v1/resources/{id}", h.deleteResource)

	mux.HandleFunc("GET /v1/permissions/builtins", h.listPermissionBuiltins)

	mux.HandleFunc("GET /v1/assistants", h.listAssistants)
	mux.HandleFunc("POST /v1/assistants", h.createAssistant)
	mux.HandleFunc("GET /v1/assistants/{id}", h.getAssistant)
	mux.HandleFunc("PATCH /v1/assistants/{id}", h.patchAssistant)
	mux.HandleFunc("DELETE /v1/assistants/{id}", h.deleteAssistant)

	mux.HandleFunc("GET /v1/threads", h.listThreads)
	mux.HandleFunc("POST /v1/threads", h.createThread)
	mux.HandleFunc("GET /v1/threads/{id}", h.getThread)
	mux.HandleFunc("PATCH /v1/threads/{id}", h.patchThread)
	mux.HandleFunc("GET /v1/threads/{id}/captures", h.listThreadCaptures)
	mux.HandleFunc("GET /v1/threads/{id}/messages/{messageId}/captures", h.listMessageCaptures)

	mux.HandleFunc("GET /v1/projects", h.listProjects)
	mux.HandleFunc("POST /v1/projects", h.createProject)
	mux.HandleFunc("GET /v1/projects/{id}", h.getProject)
	mux.HandleFunc("PATCH /v1/projects/{id}", h.patchProject)
	mux.HandleFunc("DELETE /v1/projects/{id}", h.deleteProject)
	mux.HandleFunc("GET /v1/projects/{id}/environment/resolved", h.resolvedProjectEnvironment)

	mux.HandleFunc("GET /v1/settings", h.getSettings)
	mux.HandleFunc("PATCH /v1/settings", h.patchSettings)

	mux.HandleFunc("GET /v1/tool/integrations", h.listToolIntegrations)
	mux.HandleFunc("POST /v1/tool/integrations", h.createToolIntegration)
	mux.HandleFunc("GET /v1/tool/integrations/{id}", h.getToolIntegration)
	mux.HandleFunc("PATCH /v1/tool/integrations/{id}", h.patchToolIntegration)
	mux.HandleFunc("DELETE /v1/tool/integrations/{id}", h.deleteToolIntegration)
	mux.HandleFunc("POST /v1/tool/integrations/{id}/test", h.testToolIntegration)

	mux.HandleFunc("GET /v1/tools", h.listTools)

	return mux
}

type httpAPI struct {
	store *Store
	hooks Hooks
}

func (h *httpAPI) listInferenceConnections(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListInferenceConnections(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := range list {
		list[i] = h.withSpecs(list[i])
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *httpAPI) createInferenceConnection(w http.ResponseWriter, r *http.Request) {
	var body inferenceConnectionCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := h.store.CreateInferenceConnection(r.Context(), body.Name, body.Type, body.BaseURL, body.APIKey)
	if err != nil {
		writeMappedError(w, err, "")
		return
	}
	writeJSON(w, http.StatusCreated, h.withSpecs(p))
}

func (h *httpAPI) getInferenceConnection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := h.store.GetInferenceConnection(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrInferenceConnectionNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, h.withSpecs(p))
}

func (h *httpAPI) patchInferenceConnection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body inferenceConnectionPatch
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := h.store.UpdateInferenceConnection(r.Context(), id, body.Name, body.BaseURL, body.APIKey)
	if err != nil {
		writeMappedError(w, err, id)
		return
	}
	writeJSON(w, http.StatusOK, h.withSpecs(p))
}

func (h *httpAPI) deleteInferenceConnection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.store.DeleteInferenceConnection(r.Context(), id); err != nil {
		writeMappedError(w, err, id)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *httpAPI) listResources(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListResources(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []Resource{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *httpAPI) createResource(w http.ResponseWriter, r *http.Request) {
	var body resourceCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := h.store.CreateResource(r.Context(), body.Name, body.Kind, body.Spec)
	if err != nil {
		writeMappedError(w, err, "")
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (h *httpAPI) getResource(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	res, err := h.store.GetResource(r.Context(), id)
	if err != nil {
		writeMappedError(w, err, id)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *httpAPI) patchResource(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body resourcePatch
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var spec json.RawMessage
	if len(body.Spec) > 0 {
		spec = body.Spec
	}
	res, err := h.store.UpdateResource(r.Context(), id, body.Name, spec)
	if err != nil {
		writeMappedError(w, err, id)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *httpAPI) deleteResource(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.store.DeleteResource(r.Context(), id); err != nil {
		writeMappedError(w, err, id)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *httpAPI) refreshModels(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := h.store.RefreshModels(r.Context(), id, nil)
	if err != nil {
		if errors.Is(err, ErrInferenceConnectionNotFound) {
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
	writeJSON(w, http.StatusOK, h.withSpecs(p))
}

func (h *httpAPI) listAssistants(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListAssistants(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *httpAPI) createAssistant(w http.ResponseWriter, r *http.Request) {
	var body assistantCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	a, err := h.store.CreateAssistant(r.Context(), body.Name, body.Description, body.Instructions, body.InferenceConnectionID, body.DefaultModel)
	if err != nil {
		writeMappedError(w, err, "")
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (h *httpAPI) getAssistant(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a, err := h.store.GetAssistant(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrAssistantNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (h *httpAPI) patchAssistant(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body assistantPatch
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	a, err := h.store.UpdateAssistant(r.Context(), id, body.Name, body.Description, body.Instructions, body.InferenceConnectionID, body.DefaultModel, body.Settings)
	if err != nil {
		writeMappedError(w, err, id)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (h *httpAPI) deleteAssistant(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.store.DeleteAssistant(r.Context(), id); err != nil {
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

func (h *httpAPI) listThreadCaptures(w http.ResponseWriter, r *http.Request) {
	threadID := r.PathValue("id")
	if _, err := h.store.GetThread(r.Context(), threadID); err != nil {
		if errors.Is(err, ErrThreadNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	list, err := h.store.ListHopCapturesByThread(r.Context(), threadID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []HopCapture{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *httpAPI) listMessageCaptures(w http.ResponseWriter, r *http.Request) {
	threadID := r.PathValue("id")
	messageID := r.PathValue("messageId")
	if _, err := h.store.GetThread(r.Context(), threadID); err != nil {
		if errors.Is(err, ErrThreadNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	list, err := h.store.ListHopCapturesByMessage(r.Context(), threadID, messageID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []HopCapture{}
	}
	writeJSON(w, http.StatusOK, list)
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
	p, err := h.store.CreateProject(r.Context(), body.Name, body.Description)
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
	if body.Name == nil && body.Description == nil && len(body.Settings) == 0 && len(body.Remotes) == 0 {
		writeError(w, http.StatusBadRequest, "empty patch")
		return
	}
	p, err := h.store.UpdateProject(r.Context(), id, body.Name, body.Description, body.Settings, body.Remotes)
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
	Sandbox                json.RawMessage `json:"sandbox"`
	Environment            json.RawMessage `json:"environment"`
	Integrations           json.RawMessage `json:"integrations"`
	WebSearchIntegrationID optionalString  `json:"webSearchIntegrationId"`
	FetchPageIntegrationID optionalString  `json:"fetchPageIntegrationId"`
	PlatformInstructions   *string         `json:"platformInstructions"`
	RuntimeContext         *string         `json:"runtimeContext"`
	Permissions            json.RawMessage `json:"permissions"`
}

// listPermissionBuiltins returns the gate's built-in rule tiers with their
// defaults, so settings can show what permissions.builtins overrides.
func (h *httpAPI) listPermissionBuiltins(w http.ResponseWriter, _ *http.Request) {
	var tiers any = []any{}
	if PermissionBuiltinTiersFunc != nil {
		tiers = PermissionBuiltinTiersFunc()
	}
	writeJSON(w, http.StatusOK, map[string]any{"tiers": tiers})
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
	if len(body.Sandbox) == 0 && len(body.Environment) == 0 && len(body.Integrations) == 0 &&
		!body.WebSearchIntegrationID.Present && !body.FetchPageIntegrationID.Present &&
		body.PlatformInstructions == nil && body.RuntimeContext == nil && len(body.Permissions) == 0 {
		writeError(w, http.StatusBadRequest, "settings patch is required")
		return
	}
	settings, err := h.store.PatchPlaneSettingsFull(r.Context(), PlaneSettingsPatch{
		Sandbox:                body.Sandbox,
		Environment:            body.Environment,
		Integrations:           body.Integrations,
		WebSearchIntegrationID: body.WebSearchIntegrationID,
		FetchPageIntegrationID: body.FetchPageIntegrationID,
		PlatformInstructions:   body.PlatformInstructions,
		RuntimeContext:         body.RuntimeContext,
		Permissions:            body.Permissions,
	})
	if err != nil {
		writeMappedError(w, err, "")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (h *httpAPI) resolvedProjectEnvironment(w http.ResponseWriter, r *http.Request) {
	env, err := h.store.ResolveEnvironment(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, ErrResourceNotFound) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeMappedError(w, err, r.PathValue("id"))
		return
	}
	writeJSON(w, http.StatusOK, env)
}

func (h *httpAPI) listTools(w http.ResponseWriter, r *http.Request) {
	tools, err := sandboxtools.CatalogEntries()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tools": tools})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorBody{Error: msg})
}

func writeMappedError(w http.ResponseWriter, err error, id string) {
	if errors.Is(err, ErrAssistantInUse) || errors.Is(err, ErrProjectInUse) || errors.Is(err, ErrResourceInUse) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if errors.Is(err, ErrDefaultProject) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if errors.Is(err, ErrDefaultProjectRename) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if errors.Is(err, ErrResourceNotFound) {
		writeError(w, http.StatusNotFound, fmt.Errorf("resource %q not found", id).Error())
		return
	}
	if errors.Is(err, ErrInferenceConnectionNotFound) || errors.Is(err, ErrToolIntegrationNotFound) || errors.Is(err, ErrAssistantNotFound) || errors.Is(err, ErrThreadNotFound) || errors.Is(err, ErrProjectNotFound) {
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
