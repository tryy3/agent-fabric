package catalog

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

type toolIntegrationCreate struct {
	Name     string             `json:"name"`
	Kind     string             `json:"kind"`
	Enabled  *bool              `json:"enabled"`
	Endpoint string             `json:"endpoint"`
	Mode     string             `json:"mode"`
	Config   json.RawMessage    `json:"config"`
	Secrets  map[string]*string `json:"secrets"`
}

type toolIntegrationPatch struct {
	Name     *string            `json:"name"`
	Enabled  *bool              `json:"enabled"`
	Endpoint *string            `json:"endpoint"`
	Mode     *string            `json:"mode"`
	Config   json.RawMessage    `json:"config"`
	Secrets  map[string]*string `json:"secrets"`
}

type toolIntegrationTestResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	ToolIntegration
}

func (h *httpAPI) listToolIntegrations(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListToolIntegrations(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"toolIntegrations": list})
}

func (h *httpAPI) createToolIntegration(w http.ResponseWriter, r *http.Request) {
	var body toolIntegrationCreate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ti, err := h.store.CreateToolIntegration(r.Context(), CreateToolIntegrationParams{
		Name:     body.Name,
		Kind:     body.Kind,
		Enabled:  body.Enabled,
		Endpoint: body.Endpoint,
		Mode:     body.Mode,
		Config:   body.Config,
		Secrets:  body.Secrets,
	})
	if err != nil {
		writeMappedError(w, err, "")
		return
	}
	writeJSON(w, http.StatusCreated, ti)
}

func (h *httpAPI) getToolIntegration(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ti, err := h.store.GetToolIntegration(r.Context(), id)
	if err != nil {
		writeMappedError(w, err, id)
		return
	}
	writeJSON(w, http.StatusOK, ti)
}

func (h *httpAPI) patchToolIntegration(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body toolIntegrationPatch
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Name == nil && body.Enabled == nil && body.Endpoint == nil && body.Mode == nil &&
		len(body.Config) == 0 && body.Secrets == nil {
		writeError(w, http.StatusBadRequest, "empty patch")
		return
	}
	ti, err := h.store.UpdateToolIntegration(r.Context(), id, PatchToolIntegrationParams{
		Name:     body.Name,
		Enabled:  body.Enabled,
		Endpoint: body.Endpoint,
		Mode:     body.Mode,
		Config:   body.Config,
		Secrets:  body.Secrets,
	})
	if err != nil {
		writeMappedError(w, err, id)
		return
	}
	writeJSON(w, http.StatusOK, ti)
}

func (h *httpAPI) deleteToolIntegration(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.store.DeleteToolIntegration(r.Context(), id); err != nil {
		writeMappedError(w, err, id)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// TestToolIntegrationFunc is injected for connection tests (set by server wiring).
var TestToolIntegrationFunc = func(ti ToolIntegration, secrets ToolIntegrationSecrets) error {
	return errors.New("connection test is not configured")
}

func (h *httpAPI) testToolIntegration(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ti, err := h.store.GetToolIntegration(r.Context(), id)
	if err != nil {
		writeMappedError(w, err, id)
		return
	}
	secrets, err := h.store.GetToolIntegrationSecrets(r.Context(), id)
	if err != nil {
		writeMappedError(w, err, id)
		return
	}
	testErr := TestToolIntegrationFunc(ti, secrets)
	status := HealthHealthy
	msg := "ok"
	ok := true
	if testErr != nil {
		status = HealthUnhealthy
		msg = testErr.Error()
		ok = false
	}
	updated, err := h.store.UpdateToolIntegrationHealth(r.Context(), id, status, time.Now().UTC())
	if err != nil {
		writeMappedError(w, err, id)
		return
	}
	writeJSON(w, http.StatusOK, toolIntegrationTestResult{
		OK:              ok,
		Message:         msg,
		ToolIntegration: updated,
	})
}
