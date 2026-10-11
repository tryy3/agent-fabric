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
	// CanDelete is true when the provider can delete this deploy: it manages
	// publishes, the deploy id is known, and it is not the live deploy.
	CanDelete bool `json:"canDelete"`
}

type publishesBody struct {
	Publishes []publishJSON `json:"publishes"`
}

// publishSyncProvider reports what publish sync changed for one provider.
type publishSyncProvider struct {
	Provider      string `json:"provider"`
	ProviderLabel string `json:"providerLabel"`
	catalog.PublishSyncStats
	Error string `json:"error,omitempty"`
}

type publishSyncBody struct {
	// Providers lists each publish provider the project is bound to.
	Providers []publishSyncProvider `json:"providers"`
	Publishes []publishJSON         `json:"publishes"`
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
	writeJSON(w, http.StatusOK, publishesBody{Publishes: h.publishesJSON(rows)})
}

// syncPublishes reconciles the publish history with every provider that can
// list its deploys, then returns the refreshed history. A failing provider
// is reported in its entry and does not stop the others.
func (h *httpAPI) syncPublishes(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusInternalServerError, "publish store is not configured")
		return
	}
	projectID := r.PathValue("id")
	req, err := h.publishManageRequest(r.Context(), projectID)
	if err != nil {
		writeExportError(w, err)
		return
	}
	out := publishSyncBody{Providers: []publishSyncProvider{}}
	for _, provider := range h.exporters.Managers() {
		mgr, _ := h.exporters.Manager(provider)
		entry := publishSyncProvider{Provider: provider, ProviderLabel: h.providerLabel(provider)}
		targets, listErr := mgr.ListDeploys(r.Context(), req)
		if listErr != nil {
			slog.Error("publish sync list failed", "projectId", projectID, "provider", provider, "err", listErr)
			entry.Error = listErr.Error()
			out.Providers = append(out.Providers, entry)
			continue
		}
		if len(targets) == 0 {
			continue
		}
		for _, target := range targets {
			stats, syncErr := h.store.ReconcilePublishes(r.Context(), projectID, provider, target.Target, target.Deploys, target.Complete)
			if syncErr != nil {
				slog.Error("publish sync store failed", "projectId", projectID, "provider", provider, "err", syncErr)
				entry.Error = syncErr.Error()
				break
			}
			entry.Added += stats.Added
			entry.Removed += stats.Removed
			entry.Updated += stats.Updated
		}
		out.Providers = append(out.Providers, entry)
	}
	rows, err := h.store.ListPublishes(r.Context(), projectID, "", 0)
	if err != nil {
		writeExportError(w, err)
		return
	}
	out.Publishes = h.publishesJSON(rows)
	writeJSON(w, http.StatusOK, out)
}

// deletePublish deletes a deploy on its provider, then drops it from the
// publish history.
func (h *httpAPI) deletePublish(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusInternalServerError, "publish store is not configured")
		return
	}
	projectID := r.PathValue("id")
	publish, err := h.store.GetPublish(r.Context(), projectID, r.PathValue("publishId"))
	if err != nil {
		writeExportError(w, err)
		return
	}
	mgr, ok := h.exporters.Manager(publish.Provider)
	if !ok {
		writeExportError(w, export.ErrPublishNotManaged)
		return
	}
	req, err := h.publishManageRequest(r.Context(), projectID)
	if err != nil {
		writeExportError(w, err)
		return
	}
	if err := mgr.DeleteDeploy(r.Context(), req, publish.Target, publish.DeployID); err != nil {
		writeExportError(w, err)
		return
	}
	if err := h.store.DeletePublish(r.Context(), projectID, publish.ID); err != nil {
		writeExportError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// publishManageRequest is the export request publish managers need: the
// project (for its provider bindings) and the integration credentials.
func (h *httpAPI) publishManageRequest(ctx context.Context, projectID string) (export.Request, error) {
	project, err := h.store.GetProject(ctx, projectID)
	if err != nil {
		return export.Request{}, err
	}
	settings, err := h.store.GetPlaneSettings(ctx)
	if err != nil {
		return export.Request{}, err
	}
	return export.Request{Project: project, Integrations: settings.Integrations}, nil
}

func (h *httpAPI) publishesJSON(rows []catalog.Publish) []publishJSON {
	out := make([]publishJSON, 0, len(rows))
	for _, row := range rows {
		out = append(out, h.publishJSON(row))
	}
	return out
}

func (h *httpAPI) publishJSON(p catalog.Publish) publishJSON {
	_, managed := h.exporters.Manager(p.Provider)
	return publishJSON{
		Publish:       p,
		ProviderLabel: h.providerLabel(p.Provider),
		CanDelete:     managed && p.DeployID != "" && !p.Live,
	}
}

func (h *httpAPI) providerLabel(provider string) string {
	for _, m := range h.exporters.Methods() {
		if m.ID == provider {
			return m.Label
		}
	}
	return provider
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
	case errors.Is(err, export.ErrPublishNotManaged):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, export.ErrPublishLive):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, catalog.ErrPublishNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, export.ErrPublishFailed):
		slog.Error("export publish failed", "err", err)
		writeError(w, http.StatusBadGateway, err.Error())
	case errors.Is(err, catalog.ErrProjectNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		writeMappedFSError(w, err)
	}
}
