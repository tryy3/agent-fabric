package skills

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"slices"
	"strings"

	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/sandbox"
	bundled "github.com/tryy3/agent-fabric/skills"
)

// ProjectSkillRoots are the project folders skills are read from, lowest
// precedence first, so `.agent-fabric/skills` wins over `.agents/skills`.
var ProjectSkillRoots = []string{".agents/skills", ".agent-fabric/skills"}

// ProjectOpener opens the execution environment of a project, the same one the
// Files pane shows. The caller of Open closes the environment.
type ProjectOpener interface {
	Open(ctx context.Context, projectID string) (sandbox.Environment, error)
}

// Service resolves the effective skill list for a project and assistant.
type Service struct {
	Catalog  *catalog.Store
	Opener   ProjectOpener
	Builtins Reader
}

// NewService returns a Service that reads the embedded built-in skills and,
// when opener is non-nil, project skills from the project workspace.
func NewService(store *catalog.Store, opener ProjectOpener) *Service {
	return &Service{
		Catalog:  store,
		Opener:   opener,
		Builtins: FSReader{FS: bundled.Builtin},
	}
}

// Resolution is the effective skill list plus the folders that were skipped.
type Resolution struct {
	Entries  []Entry   `json:"skills"`
	Problems []Problem `json:"problems,omitempty"`
}

// Resolve layers built-in, plane and project skills and applies the plane,
// project and assistant toggles. projectID and assistantID may be empty. Project
// skill folders that cannot be read are reported as problems, not errors, so a
// broken workspace never hides the built-ins.
func (s *Service) Resolve(ctx context.Context, projectID, assistantID string) (Resolution, error) {
	var res Resolution
	builtin, problems, err := LoadDir(ctx, s.Builtins, bundled.BuiltinRoot, SourceBuiltin)
	if err != nil {
		return res, err
	}
	res.Problems = append(res.Problems, problems...)
	layers := []Layer{{Source: SourceBuiltin, Skills: builtin}}

	var toggles []Toggles
	if s.Catalog != nil {
		toggles = append(toggles, Toggles{Disabled: s.Catalog.PlaneSkills(ctx).Disabled})
	}
	if projectID != "" && s.Catalog != nil {
		project, err := s.Catalog.GetProject(ctx, projectID)
		if err != nil {
			return res, err
		}
		toggles = append(toggles, Toggles{Disabled: catalog.SkillsFromSettings(project.Settings).Disabled})
		projectLayers, projectProblems := s.loadProject(ctx, projectID)
		layers = append(layers, projectLayers...)
		res.Problems = append(res.Problems, projectProblems...)
	}
	if assistantID != "" && s.Catalog != nil {
		assistant, err := s.Catalog.GetAssistant(ctx, assistantID)
		if err != nil {
			return res, err
		}
		toggles = append(toggles, Toggles{Disabled: catalog.SkillsFromSettings(assistant.Settings).Disabled})
	}
	res.Entries = Resolve(layers, toggles...)
	return res, nil
}

func (s *Service) loadProject(ctx context.Context, projectID string) ([]Layer, []Problem) {
	if s.Opener == nil {
		return nil, nil
	}
	env, err := s.Opener.Open(ctx, projectID)
	if err != nil {
		slog.Warn("skills: open project environment", "project", projectID, "err", err)
		return nil, []Problem{{Source: SourceProject, Path: ".", Error: err.Error()}}
	}
	defer func() {
		if closeErr := env.Close(context.Background()); closeErr != nil {
			slog.Warn("skills: close project environment", "project", projectID, "err", closeErr)
		}
	}()
	fsys, ok := env.FS()
	if !ok {
		return nil, nil
	}
	reader := sandboxReader{fs: fsys}
	var layers []Layer
	var problems []Problem
	for _, root := range ProjectSkillRoots {
		skills, p, err := LoadDir(ctx, reader, root, SourceProject)
		problems = append(problems, p...)
		if err != nil {
			problems = append(problems, Problem{Source: SourceProject, Path: root, Error: err.Error()})
			continue
		}
		layers = append(layers, Layer{Source: SourceProject, Skills: skills})
	}
	return layers, problems
}

// sandboxReader adapts a project sandbox.FS to Reader. The sandbox does not
// always wrap fs.ErrNotExist, so a missing path is recognised by message too.
type sandboxReader struct{ fs sandbox.FS }

func (r sandboxReader) ReadFile(ctx context.Context, name string) ([]byte, error) {
	data, err := r.fs.ReadFile(ctx, relPath(name))
	return data, notExist(err)
}

func (r sandboxReader) ReadDir(ctx context.Context, name string) ([]DirEntry, error) {
	entries, err := r.fs.ReadDir(ctx, relPath(name))
	if err != nil {
		return nil, notExist(err)
	}
	out := make([]DirEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, DirEntry{Name: e.Name, IsDir: e.IsDir})
	}
	return out, nil
}

func relPath(name string) string {
	return path.Clean(strings.TrimPrefix(name, "/"))
}

func notExist(err error) error {
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return err
	}
	msg := err.Error()
	if strings.Contains(msg, "no such") || strings.Contains(msg, "not found") {
		return errors.Join(fs.ErrNotExist, err)
	}
	return err
}

// Load returns the text of a skill: its SKILL.md body when path is empty,
// otherwise the extra file at path inside the skill folder. When wantHash is
// not empty it must match the skill's current hash, so a session never reads
// a skill that changed after it was pinned.
func (s *Service) Load(ctx context.Context, projectID, assistantID, name, rel, wantHash string) (Entry, string, error) {
	res, err := s.Resolve(ctx, projectID, assistantID)
	if err != nil {
		return Entry{}, "", err
	}
	var entry *Entry
	for i := range res.Entries {
		if res.Entries[i].Name == name {
			entry = &res.Entries[i]
			break
		}
	}
	if entry == nil || !entry.Enabled {
		return Entry{}, "", fmt.Errorf("skill %q is not available", name)
	}
	if wantHash != "" && entry.Hash != wantHash {
		return Entry{}, "", fmt.Errorf("skill %q changed since this session started; start a new session to use the new version", name)
	}
	if rel == "" {
		return *entry, entry.Body, nil
	}
	rel = path.Clean(strings.TrimPrefix(rel, "/"))
	if !slices.Contains(entry.Files, rel) {
		return Entry{}, "", fmt.Errorf("skill %q has no file %q; its files are: %s", name, rel, strings.Join(entry.Files, ", "))
	}
	reader, closeFn, err := s.readerFor(ctx, projectID, entry.Source)
	if err != nil {
		return Entry{}, "", err
	}
	defer closeFn()
	data, err := reader.ReadFile(ctx, path.Join(entry.Path, rel))
	if err != nil {
		return Entry{}, "", fmt.Errorf("read %s: %w", rel, err)
	}
	return *entry, string(data), nil
}

// readerFor returns a Reader for the source a skill came from, and a function
// that releases it.
func (s *Service) readerFor(ctx context.Context, projectID string, source Source) (Reader, func(), error) {
	if source == SourceBuiltin {
		return s.Builtins, func() {}, nil
	}
	if source != SourceProject || s.Opener == nil || projectID == "" {
		return nil, nil, fmt.Errorf("no reader for %s skills", source)
	}
	env, err := s.Opener.Open(ctx, projectID)
	if err != nil {
		return nil, nil, err
	}
	closeFn := func() { _ = env.Close(context.Background()) }
	fsys, ok := env.FS()
	if !ok {
		closeFn()
		return nil, nil, fmt.Errorf("project filesystem is unavailable")
	}
	return sandboxReader{fs: fsys}, closeFn, nil
}
