package container

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"
)

const (
	defaultIdleTTL     = 10 * time.Minute
	defaultLabelPrefix = "agent-fabric.sandbox"
)

type Runner interface {
	LookPath(name string) (string, error)
	CombinedOutput(
		ctx context.Context,
		name string,
		args ...string,
	) ([]byte, error)
}

type ManagerOptions struct {
	IdleTTL     time.Duration
	LabelPrefix string
	Now         func() time.Time
}

type ContainerSpec struct {
	Image         string
	Mounts        []sandboxcore.Mount
	WorkspaceRoot string
	Labels        map[string]string
}

type Manager struct {
	runner      Runner
	idleTTL     time.Duration
	labelPrefix string
	now         func() time.Time

	mu      sync.Mutex
	binary  string
	entries map[string]*entry
}

type entry struct {
	containerID string
	busyRefs    int
	lastActive  time.Time
}

func NewManager(runner Runner, opts ManagerOptions) *Manager {
	idleTTL := opts.IdleTTL
	if idleTTL == 0 {
		idleTTL = defaultIdleTTL
	}
	labelPrefix := opts.LabelPrefix
	if labelPrefix == "" {
		labelPrefix = defaultLabelPrefix
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}

	return &Manager{
		runner:      runner,
		idleTTL:     idleTTL,
		labelPrefix: labelPrefix,
		now:         now,
		entries:     map[string]*entry{},
	}
}

func (m *Manager) ResolveBinary(binPath, runtime string) (string, error) {
	if binPath != "" {
		m.setBinary(binPath)
		return binPath, nil
	}

	var candidates []string
	switch runtime {
	case "", "auto":
		candidates = []string{"podman", "docker"}
	case "podman", "docker":
		candidates = []string{runtime}
	default:
		return "", fmt.Errorf("unsupported container runtime %q", runtime)
	}

	var lookupErrors []error
	for _, candidate := range candidates {
		binary, err := m.runner.LookPath(candidate)
		if err == nil {
			m.setBinary(binary)
			return binary, nil
		}
		lookupErrors = append(
			lookupErrors,
			fmt.Errorf("look up %s: %w", candidate, err),
		)
	}
	return "", fmt.Errorf(
		"resolve container runtime: %w",
		errors.Join(lookupErrors...),
	)
}

func (m *Manager) Acquire(
	ctx context.Context,
	key string,
	spec ContainerSpec,
) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if key == "" {
		return "", errors.New("container scope key is empty")
	}
	if spec.Image == "" {
		return "", errors.New("container image is empty")
	}

	binary, err := m.currentBinary()
	if err != nil {
		return "", err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	label := m.scopeLabel(key)
	output, err := m.runner.CombinedOutput(
		ctx,
		binary,
		"ps",
		"-q",
		"-f",
		"label="+label,
	)
	if err != nil {
		return "", commandError("find scoped container", output, err)
	}

	containerID := firstLine(output)
	if containerID == "" {
		args := m.runArgs(label, spec)
		output, err = m.runner.CombinedOutput(ctx, binary, args...)
		if err != nil {
			return "", commandError("start scoped container", output, err)
		}
		containerID = firstLine(output)
		if containerID == "" {
			return "", errors.New("start scoped container: empty container ID")
		}
	}

	state := m.entries[key]
	if state == nil || state.containerID != containerID {
		state = &entry{containerID: containerID}
		m.entries[key] = state
	}
	state.busyRefs++
	state.lastActive = m.now()
	return containerID, nil
}

func (m *Manager) Touch(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state := m.entries[key]; state != nil {
		state.lastActive = m.now()
	}
}

func (m *Manager) Done(key string) {
	m.Release(key)
}

func (m *Manager) Release(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state := m.entries[key]
	if state == nil {
		return
	}
	if state.busyRefs > 0 {
		state.busyRefs--
	}
}

func (m *Manager) Reap(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	binary, err := m.currentBinary()
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	var reapErrors []error
	for key, state := range m.entries {
		if state.busyRefs != 0 || now.Sub(state.lastActive) < m.idleTTL {
			continue
		}

		output, removeErr := m.runner.CombinedOutput(
			ctx,
			binary,
			"rm",
			"-f",
			state.containerID,
		)
		if removeErr != nil {
			reapErrors = append(
				reapErrors,
				commandError("remove idle container", output, removeErr),
			)
			continue
		}
		delete(m.entries, key)
	}
	return errors.Join(reapErrors...)
}

func (m *Manager) setBinary(binary string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.binary = binary
}

func (m *Manager) currentBinary() (string, error) {
	m.mu.Lock()
	binary := m.binary
	m.mu.Unlock()
	if binary != "" {
		return binary, nil
	}
	return m.ResolveBinary("", "auto")
}

func (m *Manager) scopeLabel(key string) string {
	return m.labelPrefix + ".scope=" + key
}

func (m *Manager) runArgs(label string, spec ContainerSpec) []string {
	args := []string{"run", "-d"}
	if spec.WorkspaceRoot != "" {
		args = append(args, "--workdir", spec.WorkspaceRoot)
	}
	args = append(args, "--label", label)

	labelNames := make([]string, 0, len(spec.Labels))
	for name := range spec.Labels {
		if name != m.labelPrefix+".scope" {
			labelNames = append(labelNames, name)
		}
	}
	sort.Strings(labelNames)
	for _, name := range labelNames {
		args = append(args, "--label", name+"="+spec.Labels[name])
	}

	for _, mount := range spec.Mounts {
		value := "type=bind,source=" + mount.Source + ",target=" + mount.Target
		if mount.ReadOnly {
			value += ",readonly"
		}
		args = append(args, "--mount", value)
	}
	return append(args, spec.Image, "sleep", "infinity")
}

func firstLine(output []byte) string {
	line, _, _ := strings.Cut(strings.TrimSpace(string(output)), "\n")
	return strings.TrimSpace(line)
}

func commandError(action string, output []byte, err error) error {
	detail := strings.TrimSpace(string(output))
	if detail == "" {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%s: %w: %s", action, err, detail)
}
