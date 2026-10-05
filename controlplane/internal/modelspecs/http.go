package modelspecs

import (
	"context"
	"encoding/json"
	"net/http"
)

// Handler serves /v1/model-specs/*.
func Handler(s *Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/model-specs/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.Status())
	})
	mux.HandleFunc("POST /v1/model-specs/sync", func(w http.ResponseWriter, r *http.Request) {
		// Detach from the request so a client disconnect cannot leave a half-recorded sync.
		st, err := s.Sync(context.WithoutCancel(r.Context()))
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error(), "status": st})
			return
		}
		writeJSON(w, http.StatusOK, st)
	})
	mux.HandleFunc("GET /v1/model-specs/settings", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.Status().Settings)
	})
	mux.HandleFunc("PATCH /v1/model-specs/settings", func(w http.ResponseWriter, r *http.Request) {
		var patch struct {
			SourceURL         *string `json:"sourceUrl"`
			SyncIntervalHours *int    `json:"syncIntervalHours"`
			Enabled           *bool   `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}
		cur := s.Status().Settings
		if patch.SourceURL != nil {
			cur.SourceURL = *patch.SourceURL
		}
		if patch.SyncIntervalHours != nil {
			cur.SyncIntervalHours = *patch.SyncIntervalHours
		}
		if patch.Enabled != nil {
			cur.Enabled = *patch.Enabled
		}
		out, err := s.UpdateSettings(r.Context(), cur)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
