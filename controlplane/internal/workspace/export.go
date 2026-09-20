package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	result, err := h.exporters.Export(r.Context(), body.Method, req)
	if err != nil {
		writeExportError(w, err)
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
	agents, err := h.store.ListAgents(ctx)
	if err != nil {
		return export.Request{}, err
	}
	req.Agents = agents
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
	case errors.Is(err, catalog.ErrProjectNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		writeMappedFSError(w, err)
	}
}
