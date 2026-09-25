package export

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/sandbox"
)

const (
	MethodDownload  = "download"
	MethodGitHub    = "github"
	MethodNetlify   = "netlify"
	DefaultMaxBytes = 50 << 20
)

var (
	ErrUnknownMethod       = errors.New("unknown export method")
	ErrDisabled            = errors.New("export method is not available yet")
	ErrTooLarge            = errors.New("export exceeds 50 MiB; use GitHub or S3")
	ErrMissingCredentials  = errors.New("configure Netlify credentials in Settings → Integrations")
	ErrPublishFailed       = errors.New("publish failed")
)

type Method struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Enabled bool   `json:"enabled"`
	Reason  string `json:"reason,omitempty"`
}

type Request struct {
	Project      catalog.Project
	Threads      []catalog.ThreadDetail
	Agents       []catalog.Agent
	FS           sandbox.FS
	Integrations json.RawMessage
}

// Link is a user-facing URL returned by a publish exporter.
type Link struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	URL   string `json:"url"`
}

type Result struct {
	MediaType string
	Filename  string
	Body      []byte
	Links     []Link
	Message   string
	// RemotesPatch is a remotes[] JSON array applied by the HTTP layer after a
	// successful publish (e.g. bind a newly created Netlify site).
	RemotesPatch json.RawMessage
}

// IsPublish reports whether the result should be returned as JSON links rather
// than an archive download.
func (r Result) IsPublish() bool {
	return len(r.Links) > 0
}

type Exporter interface {
	Method() Method
	Export(ctx context.Context, req Request) (Result, error)
}

type Registry struct {
	exporters []Exporter
}

func NewRegistry() *Registry {
	return &Registry{}
}

func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(Download{})
	r.Register(GitHub{})
	r.Register(Netlify{})
	return r
}

func (r *Registry) Register(exporter Exporter) {
	if r == nil || exporter == nil {
		return
	}
	id := exporter.Method().ID
	for i, existing := range r.exporters {
		if existing.Method().ID == id {
			r.exporters[i] = exporter
			return
		}
	}
	r.exporters = append(r.exporters, exporter)
}

func (r *Registry) Methods() []Method {
	if r == nil {
		return nil
	}
	out := make([]Method, 0, len(r.exporters))
	for _, e := range r.exporters {
		out = append(out, e.Method())
	}
	return out
}

func (r *Registry) Export(ctx context.Context, method string, req Request) (Result, error) {
	if r == nil {
		return Result{}, ErrUnknownMethod
	}
	method = strings.TrimSpace(method)
	if method == "" {
		method = MethodDownload
	}
	for _, e := range r.exporters {
		m := e.Method()
		if m.ID != method {
			continue
		}
		if !m.Enabled {
			return Result{}, fmt.Errorf("%w: %s", ErrDisabled, m.ID)
		}
		return e.Export(ctx, req)
	}
	return Result{}, fmt.Errorf("%w: %s", ErrUnknownMethod, method)
}
