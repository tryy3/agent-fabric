package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/export"
	"github.com/tryy3/agent-fabric/internal/sandbox"
	"github.com/tryy3/agent-fabric/internal/workspace/fsops"
)

const maxEditorFileBytes = 2 << 20

type errorBody struct {
	Error string `json:"error"`
}

type fsListing struct {
	Path    string        `json:"path"`
	Entries []fsJSONEntry `json:"entries"`
}

type fsJSONEntry struct {
	Name    string `json:"name"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	ModTime string `json:"modTime"`
}

type httpAPI struct {
	opener    Opener
	store     *catalog.Store
	exporters *export.Registry
}

// Handler serves catalog filesystem and preview routes for a project.
func Handler(opener Opener) http.Handler {
	return HandlerWithStore(opener, nil)
}

func HandlerWithStore(opener Opener, store *catalog.Store) http.Handler {
	return HandlerWithExporters(opener, store, export.DefaultRegistry())
}

func HandlerWithExporters(opener Opener, store *catalog.Store, exporters *export.Registry) http.Handler {
	mux := http.NewServeMux()
	MountWithExporters(mux, opener, store, exporters)
	return mux
}

// Mount registers workspace routes on mux. Call before catalog `/v1/` so these
// patterns win over the prefix handler.
func Mount(mux *http.ServeMux, opener Opener) {
	MountWithStore(mux, opener, nil)
}

func MountWithStore(mux *http.ServeMux, opener Opener, store *catalog.Store) {
	MountWithExporters(mux, opener, store, export.DefaultRegistry())
}

func MountWithExporters(mux *http.ServeMux, opener Opener, store *catalog.Store, exporters *export.Registry) {
	if exporters == nil {
		exporters = export.DefaultRegistry()
	}
	h := &httpAPI{opener: opener, store: store, exporters: exporters}
	mux.HandleFunc("GET /v1/projects/{id}/fs", h.listFS)
	mux.HandleFunc("GET /v1/projects/{id}/files", h.getFile)
	mux.HandleFunc("PUT /v1/projects/{id}/files", h.putFile)
	mux.HandleFunc("DELETE /v1/projects/{id}/files", h.deleteFile)
	mux.HandleFunc("PUT /v1/projects/{id}/dirs", h.mkdir)
	mux.HandleFunc("POST /v1/projects/{id}/fs/move", h.movePath)
	mux.HandleFunc("POST /v1/projects/{id}/fs/copy", h.copyPath)
	mux.HandleFunc("GET /v1/projects/{id}/preview/{path...}", h.preview)
	mux.HandleFunc("GET /v1/projects/{id}/commits", h.listCommits)
	mux.HandleFunc("POST /v1/projects/{id}/checkpoints", h.createCheckpoint)
	mux.HandleFunc("POST /v1/projects/{id}/restore", h.restore)
	mux.HandleFunc("GET /v1/projects/{id}/diff", h.diff)
	mux.HandleFunc("GET /v1/projects/{id}/exporters", h.listExporters)
	mux.HandleFunc("POST /v1/projects/{id}/export", h.exportProject)
}

func (h *httpAPI) listFS(w http.ResponseWriter, r *http.Request) {
	h.withFS(w, r, func(fsys sandbox.FS) {
		requested := r.URL.Query().Get("path")
		if requested == "" {
			requested = "/"
		}
		userPath := normalizeCatalogPath(requested)
		entries, err := fsys.ReadDir(r.Context(), userPath)
		if err != nil {
			writeMappedFSError(w, err)
			return
		}
		out := make([]fsJSONEntry, 0, len(entries))
		for _, e := range entries {
			out = append(out, fsJSONEntry{
				Name:    e.Name,
				IsDir:   e.IsDir,
				Size:    e.Size,
				ModTime: e.ModTime.UTC().Format(time.RFC3339),
			})
		}
		writeJSON(w, http.StatusOK, fsListing{Path: requested, Entries: out})
	})
}

func (h *httpAPI) getFile(w http.ResponseWriter, r *http.Request) {
	h.withFS(w, r, func(fsys sandbox.FS) {
		userPath, ok := requiredPath(w, r)
		if !ok {
			return
		}
		info, err := fsys.Stat(r.Context(), userPath)
		if err != nil {
			writeMappedFSError(w, err)
			return
		}
		if info.IsDir() {
			writeError(w, http.StatusBadRequest, "path is a directory")
			return
		}
		if info.Size() > maxEditorFileBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "file exceeds 2 MiB editor limit")
			return
		}
		data, err := fsys.ReadFile(r.Context(), userPath)
		if err != nil {
			writeMappedFSError(w, err)
			return
		}
		if len(data) > maxEditorFileBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "file exceeds 2 MiB editor limit")
			return
		}
		w.Header().Set("Content-Type", mimeFromPath(userPath)+"; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	})
}

func (h *httpAPI) putFile(w http.ResponseWriter, r *http.Request) {
	h.withFS(w, r, func(fsys sandbox.FS) {
		userPath, ok := requiredPath(w, r)
		if !ok {
			return
		}
		data, err := io.ReadAll(io.LimitReader(r.Body, maxEditorFileBytes+1))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if len(data) > maxEditorFileBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "file exceeds 2 MiB editor limit")
			return
		}
		if err := fsys.WriteFile(r.Context(), userPath, data); err != nil {
			writeMappedFSError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func (h *httpAPI) deleteFile(w http.ResponseWriter, r *http.Request) {
	h.withFS(w, r, func(fsys sandbox.FS) {
		userPath, ok := requiredPath(w, r)
		if !ok {
			return
		}
		if err := fsys.Remove(r.Context(), userPath); err != nil {
			writeMappedFSError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func (h *httpAPI) mkdir(w http.ResponseWriter, r *http.Request) {
	h.withFS(w, r, func(fsys sandbox.FS) {
		userPath, ok := requiredPath(w, r)
		if !ok {
			return
		}
		if err := fsys.Mkdir(r.Context(), userPath); err != nil {
			writeMappedFSError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

type pathOpRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// movePath renames or moves an entry. The destination must not exist.
func (h *httpAPI) movePath(w http.ResponseWriter, r *http.Request) {
	h.withFS(w, r, func(fsys sandbox.FS) {
		var req pathOpRequest
		if !decodePathOp(w, r, &req) {
			return
		}
		if strings.TrimSpace(req.To) == "" {
			writeError(w, http.StatusBadRequest, "to is required")
			return
		}
		dst, err := fsops.New(fsys).Move(r.Context(), req.From, req.To)
		if err != nil {
			writeMappedFSError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"path": dst})
	})
}

// copyPath copies an entry to "to", or duplicates it beside itself under a
// generated "name copy.ext" name when "to" is omitted.
func (h *httpAPI) copyPath(w http.ResponseWriter, r *http.Request) {
	h.withFS(w, r, func(fsys sandbox.FS) {
		var req pathOpRequest
		if !decodePathOp(w, r, &req) {
			return
		}
		svc := fsops.New(fsys)
		var (
			dst string
			err error
		)
		if strings.TrimSpace(req.To) == "" {
			dst, err = svc.Duplicate(r.Context(), req.From)
		} else {
			dst, err = svc.Copy(r.Context(), req.From, req.To)
		}
		if err != nil {
			writeMappedFSError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"path": dst})
	})
}

func decodePathOp(w http.ResponseWriter, r *http.Request, req *pathOpRequest) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	if strings.TrimSpace(req.From) == "" {
		writeError(w, http.StatusBadRequest, "from is required")
		return false
	}
	return true
}

func (h *httpAPI) preview(w http.ResponseWriter, r *http.Request) {
	h.withFS(w, r, func(fsys sandbox.FS) {
		userPath := normalizeCatalogPath(r.PathValue("path"))
		if userPath == "." || strings.HasSuffix(r.URL.Path, "/") {
			writeError(w, http.StatusNotFound, "directory indexes are disabled")
			return
		}
		info, err := fsys.Stat(r.Context(), userPath)
		if err != nil {
			writeMappedFSError(w, err)
			return
		}
		if info.IsDir() {
			writeError(w, http.StatusNotFound, "directory indexes are disabled")
			return
		}
		data, err := fsys.ReadFile(r.Context(), userPath)
		if err != nil {
			writeMappedFSError(w, err)
			return
		}
		projectID := r.PathValue("id")
		writePreview(w, r, projectID, userPath, data)
	})
}

func (h *httpAPI) withFS(w http.ResponseWriter, r *http.Request, fn func(sandbox.FS)) {
	if h.opener == nil {
		writeError(w, http.StatusInternalServerError, "workspace opener is not configured")
		return
	}
	projectID := r.PathValue("id")
	env, err := h.opener.Open(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, catalog.ErrProjectNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeMappedFSError(w, err)
		return
	}
	defer env.Close(r.Context())
	fsys, ok := env.FS()
	if !ok {
		writeError(w, http.StatusInternalServerError, "filesystem is unavailable")
		return
	}
	fn(fsys)
}

func requiredPath(w http.ResponseWriter, r *http.Request) (string, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("path"))
	if raw == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return "", false
	}
	return normalizeCatalogPath(raw), true
}

func normalizeCatalogPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" || p == "." {
		return "."
	}
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return "."
	}
	return p
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorBody{Error: msg})
}

func writeMappedFSError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	msg := err.Error()
	switch {
	case errors.Is(err, catalog.ErrProjectNotFound):
		status = http.StatusNotFound
	case errors.Is(err, fsops.ErrEscapesRoot) || errors.Is(err, fsops.ErrProtected):
		status = http.StatusForbidden
	case errors.Is(err, fs.ErrExist):
		status = http.StatusConflict
	case errors.Is(err, fsops.ErrInvalidPath) || errors.Is(err, fsops.ErrIntoSelf):
		status = http.StatusBadRequest
	case errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) ||
		strings.Contains(msg, "not found") || strings.Contains(msg, "no such"):
		status = http.StatusNotFound
	case strings.Contains(msg, "escapes") || strings.Contains(msg, "not allowed") ||
		strings.Contains(msg, "not writable") || strings.Contains(msg, "not readable"):
		status = http.StatusForbidden
	case strings.Contains(msg, "directory not empty") || strings.Contains(msg, "not empty"):
		status = http.StatusConflict
	}
	writeError(w, status, msg)
}

func mimeFromPath(filePath string) string {
	switch strings.ToLower(path.Ext(filePath)) {
	case ".html", ".htm":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js", ".mjs":
		return "text/javascript"
	case ".json":
		return "application/json"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".txt", ".md":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

func writePreview(w http.ResponseWriter, r *http.Request, projectID, filePath string, data []byte) {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := r.Host
	previewPrefix := fmt.Sprintf("%s://%s/v1/projects/%s/preview/", scheme, host, projectID)
	contentType := mimeFromPath(filePath)
	if contentType == "text/html" {
		data = injectHTMLBase(data, "/v1/projects/"+projectID+"/preview/")
	}
	ancestors := previewFrameAncestors(r)
	w.Header().Set("Content-Type", contentType+"; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", previewCSP(previewPrefix, ancestors))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// previewFrameAncestors picks the embedding page's origin for frame-ancestors.
// Iframe document loads are plain GET navigations and carry no Origin header,
// so fall back to the Referer: the default referrer policy sends the embedding
// origin cross-origin. Without either, framing stays disabled ('none').
func previewFrameAncestors(r *http.Request) string {
	if origin := r.Header.Get("Origin"); origin != "" && !strings.EqualFold(origin, "null") {
		return origin
	}
	if referer := r.Header.Get("Referer"); referer != "" {
		if u, err := url.Parse(referer); err == nil && u.Host != "" &&
			(u.Scheme == "http" || u.Scheme == "https") {
			return u.Scheme + "://" + u.Host
		}
	}
	return "'none'"
}

func previewCSP(previewPrefix, frameAncestors string) string {
	return strings.Join([]string{
		"default-src 'none'",
		"script-src 'unsafe-inline' 'unsafe-eval' " + previewPrefix,
		"style-src 'unsafe-inline' " + previewPrefix,
		"img-src " + previewPrefix + " data: blob:",
		"font-src " + previewPrefix + " data:",
		"media-src " + previewPrefix + " blob:",
		"connect-src 'none'",
		"worker-src 'none'",
		"frame-src 'none'",
		"object-src 'none'",
		"base-uri " + previewPrefix,
		"form-action 'none'",
		"frame-ancestors " + frameAncestors,
	}, "; ")
}

func injectHTMLBase(html []byte, href string) []byte {
	escaped := strings.ReplaceAll(href, `"`, "")
	base := []byte(`<base href="` + escaped + `">`)
	lower := strings.ToLower(string(html))
	if i := strings.Index(lower, "<head"); i >= 0 {
		if j := strings.Index(string(html[i:]), ">"); j >= 0 {
			at := i + j + 1
			out := make([]byte, 0, len(html)+len(base))
			out = append(out, html[:at]...)
			out = append(out, base...)
			out = append(out, html[at:]...)
			return out
		}
	}
	return append(base, html...)
}
