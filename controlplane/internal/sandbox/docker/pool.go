package docker

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/tryy3/agent-fabric/internal/sandbox/container"
	"github.com/tryy3/agent-fabric/internal/sandbox/sandboxcore"
)

const defaultReapInterval = time.Minute

type runtimeRunner interface {
	container.Runner
	CommandRunner
}

type managerKey struct {
	runtime string
	binPath string
}

type managerPool struct {
	ctx          context.Context
	runner       runtimeRunner
	reapInterval time.Duration
	now          func() time.Time

	startOnce sync.Once
	mu        sync.Mutex
	managers  map[managerKey]*container.Manager
}

var defaultManagerPool = newManagerPool(
	context.Background(),
	OSRunner{},
	defaultReapInterval,
	time.Now,
)

func newManagerPool(
	ctx context.Context,
	runner runtimeRunner,
	reapInterval time.Duration,
	now func() time.Time,
) *managerPool {
	if reapInterval <= 0 {
		reapInterval = defaultReapInterval
	}
	if now == nil {
		now = time.Now
	}
	return &managerPool{
		ctx:          ctx,
		runner:       runner,
		reapInterval: reapInterval,
		now:          now,
		managers:     make(map[managerKey]*container.Manager),
	}
}

func OpenDefault(
	ctx context.Context,
	opts sandboxcore.OpenOptions,
) (sandboxcore.Environment, error) {
	return defaultManagerPool.open(ctx, opts)
}

func (p *managerPool) open(
	ctx context.Context,
	opts sandboxcore.OpenOptions,
) (sandboxcore.Environment, error) {
	if opts.Docker == nil {
		return nil, errors.New("docker options are required")
	}
	p.start()
	manager := p.manager(*opts.Docker)
	return openWithRunner(ctx, opts, manager, p.runner)
}

func (p *managerPool) manager(opts sandboxcore.DockerOptions) *container.Manager {
	key := managerKey{
		runtime: opts.Runtime,
		binPath: opts.BinPath,
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	manager := p.managers[key]
	if manager == nil {
		manager = container.NewManager(p.runner, container.ManagerOptions{
			IdleTTL: opts.IdleTTL,
			Now:     p.now,
		})
		p.managers[key] = manager
	} else {
		// A runtime has one refcount domain. Prefer the shorter requested TTL
		// without splitting that domain across independently reaping managers.
		manager.UseIdleTTL(opts.IdleTTL)
	}
	return manager
}

func (p *managerPool) start() {
	p.startOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(p.reapInterval)
			defer ticker.Stop()
			for {
				select {
				case <-p.ctx.Done():
					return
				case <-ticker.C:
					_ = p.reap(context.Background())
				}
			}
		}()
	})
}

func (p *managerPool) reap(ctx context.Context) error {
	p.mu.Lock()
	managers := make([]*container.Manager, 0, len(p.managers))
	for _, manager := range p.managers {
		managers = append(managers, manager)
	}
	p.mu.Unlock()

	var reapErrors []error
	for _, manager := range managers {
		if err := manager.Reap(ctx); err != nil {
			reapErrors = append(reapErrors, err)
		}
	}
	return errors.Join(reapErrors...)
}
