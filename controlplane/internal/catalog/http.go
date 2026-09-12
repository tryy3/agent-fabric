package catalog

import (
	"encoding/json"
	"errors"
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
	Name         *string `json:"name"`
	Description  *string `json:"description"`
	ProviderID   *string `json:"providerId"`
	DefaultModel *string `json:"defaultModel"`
}

// Handler serves the catalog HTTP API. POST create responses use 201 Created.
func Handler(store *Store) http.Handler {
	mux := http.NewServeMux()
	h := &httpAPI{store: store}

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

	return mux
}

type httpAPI struct {
	store *Store
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
		if isNotFoundFor(err, "provider", id) {
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
		if isNotFoundFor(err, "provider", id) {
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
		if isNotFoundFor(err, "agent", id) {
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
	a, err := h.store.UpdateAgent(r.Context(), id, body.Name, body.Description, body.ProviderID, body.DefaultModel)
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorBody{Error: msg})
}

func writeMappedError(w http.ResponseWriter, err error, resourceID string) {
	if errors.Is(err, ErrProviderInUse) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if resourceID != "" && (isNotFoundFor(err, "provider", resourceID) || isNotFoundFor(err, "agent", resourceID)) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}

func isNotFoundFor(err error, kind, id string) bool {
	return err.Error() == kind+` "`+id+`" not found`
}

func isUpstreamRefreshError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "fetch models") ||
		strings.Contains(msg, "models HTTP") ||
		strings.Contains(msg, "decode models") ||
		strings.Contains(msg, "create models request")
}
