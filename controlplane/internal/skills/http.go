package skills

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/tryy3/agent-fabric/internal/catalog"
)

// Mount registers the read-only skills catalog routes on mux:
//
//	GET /v1/skills?projectId=&assistantId=   effective list with source and enabled state
//	GET /v1/skills/{name}?projectId=&assistantId=   one skill with its full text
func Mount(mux *http.ServeMux, svc *Service) {
	h := &httpAPI{svc: svc}
	mux.HandleFunc("GET /v1/skills", h.list)
	mux.HandleFunc("GET /v1/skills/{name}", h.get)
}

type httpAPI struct{ svc *Service }

type skillDetail struct {
	Entry
	Body string `json:"body"`
}

func (h *httpAPI) resolve(w http.ResponseWriter, r *http.Request) (Resolution, bool) {
	q := r.URL.Query()
	res, err := h.svc.Resolve(r.Context(), strings.TrimSpace(q.Get("projectId")), strings.TrimSpace(q.Get("assistantId")))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, catalog.ErrProjectNotFound) || errors.Is(err, catalog.ErrAssistantNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return Resolution{}, false
	}
	return res, true
}

func (h *httpAPI) list(w http.ResponseWriter, r *http.Request) {
	res, ok := h.resolve(w, r)
	if !ok {
		return
	}
	if res.Entries == nil {
		res.Entries = []Entry{}
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *httpAPI) get(w http.ResponseWriter, r *http.Request) {
	res, ok := h.resolve(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	for _, e := range res.Entries {
		if e.Name == name {
			writeJSON(w, http.StatusOK, skillDetail{Entry: e, Body: e.Body})
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "skill not found: " + name})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
