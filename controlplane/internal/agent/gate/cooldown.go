package gate

import (
	"sync"
	"time"
)

// CooldownConfig tunes a Cooldown. The zero value is the default.
type CooldownConfig struct {
	// Base is the wait after the first block; it doubles with each further one.
	Base time.Duration
	// Cap bounds the wait.
	Cap time.Duration
	// Decay is the quiet time that forgets one block.
	Decay time.Duration
}

const (
	defaultCooldownBase  = 10 * time.Millisecond
	defaultCooldownCap   = 5 * time.Minute
	defaultCooldownDecay = 30 * time.Minute
)

func (c CooldownConfig) withDefaults() CooldownConfig {
	if c.Base <= 0 {
		c.Base = defaultCooldownBase
	}
	if c.Cap <= 0 {
		c.Cap = defaultCooldownCap
	}
	if c.Decay <= 0 {
		c.Decay = defaultCooldownDecay
	}
	return c
}

// Cooldown is one session's gate cooldown: a wait that doubles with each call
// the gate itself denied or cancelled, so an agent probing for a command
// variant the gate lets through runs out of time before it runs out of ideas.
// It is safe for concurrent use.
type Cooldown struct {
	cfg CooldownConfig
	now func() time.Time

	mu    sync.Mutex
	count int
	last  time.Time
	until time.Time
}

// NewCooldown returns a cooldown reading time from now (time.Now when nil).
func NewCooldown(cfg CooldownConfig, now func() time.Time) *Cooldown {
	if now == nil {
		now = time.Now
	}
	return &Cooldown{cfg: cfg.withDefaults(), now: now}
}

// effective is the remembered block count at t. Callers hold c.mu.
func (c *Cooldown) effective(t time.Time) int {
	if c.count == 0 {
		return 0
	}
	faded := int(t.Sub(c.last) / c.cfg.Decay)
	return max(0, c.count-faded)
}

// Active reports whether a call made now is cooling down, with the number of
// blocks the cooldown remembers and the time left. It never changes state, so
// retrying early does not extend the wait.
func (c *Cooldown) Active() (blocks int, remaining time.Duration, active bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.now()
	if !t.Before(c.until) {
		return 0, 0, false
	}
	return c.effective(t), c.until.Sub(t), true
}

// Block records a call the gate denied or cancelled and starts the next wait.
func (c *Cooldown) Block() {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.now()
	n := c.effective(t) + 1
	c.count, c.last = n, t
	wait := c.cfg.Base
	for i := 1; i < n && wait < c.cfg.Cap; i++ {
		wait *= 2
	}
	c.until = t.Add(min(wait, c.cfg.Cap))
}
