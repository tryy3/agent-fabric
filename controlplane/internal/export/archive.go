package export

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode"

	"github.com/tryy3/agent-fabric/internal/sandbox"
)

type projectArchive struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Settings    json.RawMessage `json:"settings"`
	Agents      []agentRef      `json:"agents,omitempty"`
}

type agentRef struct {
	Name    string `json:"name"`
	ModelID string `json:"modelId,omitempty"`
}

type limitedBuffer struct {
	max int
	buf bytes.Buffer
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if l.max > 0 && l.buf.Len()+len(p) > l.max {
		return 0, ErrTooLarge
	}
	return l.buf.Write(p)
}

func buildZip(ctx context.Context, req Request, maxBytes int) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	out := &limitedBuffer{max: maxBytes}
	zw := zip.NewWriter(out)
	projectJSON, err := marshalProject(req)
	if err != nil {
		return nil, err
	}
	if err := writeZipFile(zw, "project.json", projectJSON); err != nil {
		return nil, err
	}
	threadsJSON, err := json.Marshal(req.Threads)
	if err != nil {
		return nil, fmt.Errorf("encode threads: %w", err)
	}
	if threadsJSON == nil {
		threadsJSON = []byte("[]")
	}
	if err := writeZipFile(zw, "threads.json", threadsJSON); err != nil {
		return nil, err
	}
	if req.FS != nil {
		if err := addWorkspaceTree(ctx, zw, req.FS, ".", "workspace"); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.buf.Bytes(), nil
}

// netlifyZipRoot is the single top-level directory inside Netlify deploy zips.
// Netlify's zip unpacker drops filenames when a lone file sits at the archive
// root (deployed path becomes "/" and Content-Type text/plain). Wrapping files
// under one folder avoids that; Netlify strips the common root on publish.
const netlifyZipRoot = "site"

// buildWorkspaceZip packs the sandbox workspace for a Netlify zip deploy.
// Files are placed under netlifyZipRoot (not the archive root) so a single-file
// site still retains its path after Netlify unpacks the archive.
func buildWorkspaceZip(ctx context.Context, fsys sandbox.FS, maxBytes int) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	if fsys == nil {
		return nil, fmt.Errorf("workspace filesystem is required")
	}
	out := &limitedBuffer{max: maxBytes}
	zw := zip.NewWriter(out)
	if err := addWorkspaceTree(ctx, zw, fsys, ".", netlifyZipRoot); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.buf.Bytes(), nil
}

func marshalProject(req Request) ([]byte, error) {
	settings := stripSecretsRaw(req.Project.Settings)
	agents := make([]agentRef, 0, len(req.Agents))
	seen := map[string]struct{}{}
	for _, ag := range req.Agents {
		name := strings.TrimSpace(ag.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		ref := agentRef{Name: name}
		if ag.DefaultModel != nil {
			ref.ModelID = strings.TrimSpace(*ag.DefaultModel)
		}
		agents = append(agents, ref)
	}
	return json.Marshal(projectArchive{
		Name:        req.Project.Name,
		Description: req.Project.Description,
		Settings:    settings,
		Agents:      agents,
	})
}

func addWorkspaceTree(ctx context.Context, zw *zip.Writer, fsys sandbox.FS, dir, zipPrefix string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entries, err := fsys.ReadDir(ctx, dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		child := entry.Name
		if dir != "" && dir != "." && dir != "/" {
			child = path.Join(dir, entry.Name)
		}
		if entry.IsDir {
			if err := addWorkspaceTree(ctx, zw, fsys, child, zipPrefix); err != nil {
				return err
			}
			continue
		}
		data, err := fsys.ReadFile(ctx, child)
		if err != nil {
			return err
		}
		cleaned := path.Clean(child)
		name := cleaned
		if zipPrefix != "" {
			name = path.Join(zipPrefix, cleaned)
		}
		if err := writeZipFile(zw, name, data); err != nil {
			return err
		}
	}
	return nil
}

func writeZipFile(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	if err != nil && err != ErrTooLarge {
		if err == io.ErrShortWrite {
			return ErrTooLarge
		}
	}
	return err
}

func zipFilename(name string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(name) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == ' ' || r == '_' || r == '-' || r == '.':
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "project"
	}
	return out + ".zip"
}

func stripSecretsRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("{}")
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return json.RawMessage("{}")
	}
	cleaned := stripSecrets(v)
	out, err := json.Marshal(cleaned)
	if err != nil {
		return json.RawMessage("{}")
	}
	return out
}

func stripSecrets(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if secretKey(k) {
				continue
			}
			out[k] = stripSecrets(val)
		}
		return out
	case []any:
		out := make([]any, 0, len(t))
		for _, item := range t {
			out = append(out, stripSecrets(item))
		}
		return out
	default:
		return v
	}
}

func secretKey(k string) bool {
	n := strings.ToLower(strings.TrimSpace(k))
	n = strings.ReplaceAll(n, "_", "")
	n = strings.ReplaceAll(n, "-", "")
	switch n {
	case "apikey", "token", "secret", "password", "authorization", "accesskey", "secretkey":
		return true
	default:
		return strings.Contains(n, "apikey") || strings.HasSuffix(n, "token") || strings.HasSuffix(n, "secret")
	}
}
