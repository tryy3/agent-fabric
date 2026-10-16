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
	ErrUnknownMethod      = errors.New("unknown export method")
	ErrDisabled           = errors.New("export method is not available yet")
	ErrTooLarge           = errors.New("export exceeds 50 MiB; use GitHub or S3")
	ErrMissingCredentials = errors.New("configure Netlify credentials in Settings → Integrations")
	ErrPublishFailed      = errors.New("publish failed")
	// ErrPublishLive is returned when deleting the deploy a target serves at
	// its latest link.
	ErrPublishLive = errors.New("the live deploy can't be deleted; publish a newer version first")
	// ErrPublishNotManaged is returned for a provider that can't list or
	// delete its deploys.
	ErrPublishNotManaged = errors.New("this publish provider can't sync or delete deploys")
)

type Method struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Enabled bool   `json:"enabled"`
	Reason  string `json:"reason,omitempty"`
	// ManagePublishes is set by the registry when the exporter implements
	// PublishManager (publish sync and deploy delete).
	ManagePublishes bool `json:"managePublishes,omitempty"`
}

type Request struct {
	Project      catalog.Project
	Threads      []catalog.ThreadDetail
	Assistants   []catalog.Assistant
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
	// Publish describes a successful publish for the project's publish history.
	// The HTTP layer stores it with the exporter's method ID as the provider.
	Publish *catalog.PublishInput
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

// TargetDeploys is one provider target's deploys as the provider reports
// them, for publish sync.
type TargetDeploys struct {
	Target string
	// Deploys are the target's successful deploys, newest first. Provider and
	// Target on each entry are filled in by the caller.
	Deploys []catalog.PublishInput
	// Complete is false when the provider's list was cut off, so missing older
	// deploys must not be treated as deleted.
	Complete bool
}

// PublishManager is implemented by publish exporters whose provider can list
// and delete deploys.
type PublishManager interface {
	// ListDeploys reports the deploys of every target the project is bound
	// to. A project with no target on this provider returns nil.
	ListDeploys(ctx context.Context, req Request) ([]TargetDeploys, error)
	// DeleteDeploy removes one deploy on the provider. A deploy that is
	// already gone is not an error; the live deploy returns ErrPublishLive.
	DeleteDeploy(ctx context.Context, req Request, target, deployID string) error
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
		m := e.Method()
		_, m.ManagePublishes = e.(PublishManager)
		m.ManagePublishes = m.ManagePublishes && m.Enabled
		out = append(out, m)
	}
	return out
}

// Manager returns the publish manager for an enabled exporter method.
func (r *Registry) Manager(method string) (PublishManager, bool) {
	if r == nil {
		return nil, false
	}
	for _, e := range r.exporters {
		m := e.Method()
		if m.ID != method || !m.Enabled {
			continue
		}
		pm, ok := e.(PublishManager)
		return pm, ok
	}
	return nil, false
}

// Managers returns the method IDs of enabled exporters that implement
// PublishManager, in registration order.
func (r *Registry) Managers() []string {
	var out []string
	for _, m := range r.Methods() {
		if m.ManagePublishes {
			out = append(out, m.ID)
		}
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
