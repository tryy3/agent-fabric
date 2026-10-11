package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/export"
)

type exportersBody struct {
	Exporters []export.Method `json:"exporters"`
}

type exportBody struct {
	Method string `json:"method"`
}

type publishResponse struct {
	Method  string        `json:"method"`
	Message string        `json:"message,omitempty"`
	Links   []export.Link `json:"links"`
	// Publish is the stored history entry; absent when no store is configured
	// or the history write failed.
	Publish *publishJSON `json:"publish,omitempty"`
}

// publishJSON is a stored publish plus the provider's display label, so
// clients can tell providers apart without a second lookup.
type publishJSON struct {
	catalog.Publish
	ProviderLabel string `json:"providerLabel"`
}

type publishesBody struct {
	Publishes []publishJSON `json:"publishes"`
}

func (h *httpAPI) listExporters(w http.ResponseWriter, r *http.Request) {
	if h.store != nil {
		if _, err := h.store.GetProject(r.Context(), r.PathValue("id")); err != nil {
			writeExportError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, exportersBody{Exporters: h.exporters.Methods()})
}

func (h *httpAPI) exportProject(w http.ResponseWriter, r *http.Request) {
	var body exportBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	projectID := r.PathValue("id")
	req, err := h.catalogExportRequest(r.Context(), projectID)
	if err != nil {
		writeExportError(w, err)
		return
	}
	if h.opener != nil {
		env, openErr := h.opener.Open(r.Context(), projectID)
		if openErr != nil {
			writeExportError(w, openErr)
			return
		}
		defer env.Close(r.Context())
		if fsys, ok := env.FS(); ok {
			req.FS = fsys
		}
	}
	if h.store != nil {
		settings, settingsErr := h.store.GetPlaneSettings(r.Context())
		if settingsErr != nil {
			writeExportError(w, settingsErr)
			return
		}
		req.Integrations = settings.Integrations
	}
	result, err := h.exporters.Export(r.Context(), body.Method, req)
	if err != nil {
		writeExportError(w, err)
		return
	}
	if len(result.RemotesPatch) > 0 && h.store != nil {
		if _, patchErr := h.store.UpdateProject(r.Context(), projectID, nil, nil, nil, result.RemotesPatch); patchErr != nil {
			slog.Error("export remotes persist failed", "projectId", projectID, "err", patchErr)
			writeError(w, http.StatusInternalServerError, "published but failed to save site binding: "+patchErr.Error())
			return
		}
	}
	if result.IsPublish() {
		method := strings.TrimSpace(body.Method)
		if method == "" {
			method = export.MethodDownload
		}
		resp := publishResponse{
			Method:  method,
			Message: result.Message,
			Links:   result.Links,
		}
		if result.Publish != nil && h.store != nil {
			in := *result.Publish
			in.Provider = method
			// The publish already succeeded upstream; a failed history write
			// must not hide its links from the user.
			if saved, saveErr := h.store.InsertPublish(r.Context(), projectID, in); saveErr != nil {
				slog.Error("export publish history persist failed", "projectId", projectID, "err", saveErr)
			} else {
				item := h.publishJSON(saved)
				resp.Publish = &item
			}
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}
	mediaType := result.MediaType
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mediaType)
	if result.Filename != "" {
		safe := strings.ReplaceAll(result.Filename, `"`, "")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safe))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(result.Body)
}

// listPublishes returns the project's publish history, newest first.
// ?provider= narrows to one provider; ?limit= caps the count.
func (h *httpAPI) listPublishes(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusOK, publishesBody{Publishes: []publishJSON{}})
		return
	}
	limit := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = n
	}
	provider := strings.TrimSpace(r.URL.Query().Get("provider"))
	rows, err := h.store.ListPublishes(r.Context(), r.PathValue("id"), provider, limit)
	if err != nil {
		writeExportError(w, err)
		return
	}
	out := publishesBody{Publishes: make([]publishJSON, 0, len(rows))}
	for _, row := range rows {
		out.Publishes = append(out.Publishes, h.publishJSON(row))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *httpAPI) publishJSON(p catalog.Publish) publishJSON {
	label := p.Provider
	for _, m := range h.exporters.Methods() {
		if m.ID == p.Provider {
			label = m.Label
			break
		}
	}
	return publishJSON{Publish: p, ProviderLabel: label}
}

func (h *httpAPI) catalogExportRequest(ctx context.Context, projectID string) (export.Request, error) {
	req := export.Request{
		Project: catalog.Project{
			ID:       projectID,
			Name:     "project",
			Settings: json.RawMessage(`{}`),
			Remotes:  json.RawMessage(`[]`),
		},
	}
	if h.store == nil {
		return req, nil
	}
	project, err := h.store.GetProject(ctx, projectID)
	if err != nil {
		return export.Request{}, err
	}
	req.Project = project
	items, err := h.store.ListThreads(ctx, projectID)
	if err != nil {
		return export.Request{}, err
	}
	req.Threads = make([]catalog.ThreadDetail, 0, len(items))
	for _, item := range items {
		detail, err := h.store.GetThread(ctx, item.ID)
		if err != nil {
			return export.Request{}, err
		}
		req.Threads = append(req.Threads, detail)
	}
	assistants, err := h.store.ListAssistants(ctx)
	if err != nil {
		return export.Request{}, err
	}
	req.Assistants = assistants
	return req, nil
}

func writeExportError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, export.ErrTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, export.ErrTooLarge.Error())
	case errors.Is(err, export.ErrDisabled):
		writeError(w, http.StatusNotImplemented, err.Error())
	case errors.Is(err, export.ErrUnknownMethod):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, export.ErrMissingCredentials):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, export.ErrPublishFailed):
		slog.Error("export publish failed", "err", err)
		writeError(w, http.StatusBadGateway, err.Error())
	case errors.Is(err, catalog.ErrProjectNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		writeMappedFSError(w, err)
	}
}
