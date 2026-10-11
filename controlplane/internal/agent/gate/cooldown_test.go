package gate

import (
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newTestCooldown() (*Cooldown, *fakeClock) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	return NewCooldown(CooldownConfig{}, clk.now), clk
}

// waitAfterBlock blocks and returns the wait that started.
func waitAfterBlock(c *Cooldown) time.Duration {
	c.Block()
	_, rem, _ := c.Active()
	return rem
}

func TestCooldownInactiveWithoutBlocks(t *testing.T) {
	c, _ := newTestCooldown()
	if _, _, active := c.Active(); active {
		t.Fatal("fresh cooldown must not be active")
	}
}

func TestCooldownDoublesThenCaps(t *testing.T) {
	c, clk := newTestCooldown()
	want := 10 * time.Millisecond
	for i := 1; i <= 20; i++ {
		if got := waitAfterBlock(c); got != min(want, 5*time.Minute) {
			t.Fatalf("block %d: wait %v, want %v", i, got, min(want, 5*time.Minute))
		}
		clk.advance(min(want, 5*time.Minute))
		want *= 2
	}
}

func TestCooldownExpires(t *testing.T) {
	c, clk := newTestCooldown()
	c.Block()
	clk.advance(10 * time.Millisecond)
	if _, _, active := c.Active(); active {
		t.Fatal("cooldown must end after the wait")
	}
}

func TestCooldownRetryDoesNotExtend(t *testing.T) {
	c, clk := newTestCooldown()
	c.Block()
	c.Block() // second block while the first wait is moot: 20 ms
	clk.advance(5 * time.Millisecond)
	_, first, _ := c.Active()
	for range 5 {
		c.Active()
	}
	_, second, _ := c.Active()
	if first != second || first != 15*time.Millisecond {
		t.Fatalf("remaining %v then %v, want a stable 15ms", first, second)
	}
}

func TestCooldownReportsBlockCount(t *testing.T) {
	c, _ := newTestCooldown()
	c.Block()
	c.Block()
	c.Block()
	if n, _, _ := c.Active(); n != 3 {
		t.Fatalf("blocks %d, want 3", n)
	}
}

func TestCooldownDecaysOneBlockPerPeriod(t *testing.T) {
	c, clk := newTestCooldown()
	for range 4 {
		c.Block()
		clk.advance(time.Minute)
	}
	// Four blocks remembered; after 2 decay periods of quiet two are left, so
	// the next block counts as the third and waits 40 ms.
	clk.advance(2*30*time.Minute - time.Minute)
	if got := waitAfterBlock(c); got != 40*time.Millisecond {
		t.Fatalf("wait after decay %v, want 40ms", got)
	}
}

func TestCooldownFullyDecays(t *testing.T) {
	c, clk := newTestCooldown()
	for range 6 {
		c.Block()
	}
	clk.advance(10 * 30 * time.Minute)
	if got := waitAfterBlock(c); got != 10*time.Millisecond {
		t.Fatalf("wait after full decay %v, want the base", got)
	}
}

func TestCooldownNewBlockRefreshesClock(t *testing.T) {
	c, clk := newTestCooldown()
	c.Block()
	c.Block()
	clk.advance(29 * time.Minute)
	c.Block() // 3 blocks, clock restarts here
	clk.advance(29 * time.Minute)
	// 58 minutes since the first blocks but only 29 since the last: no decay yet.
	if got := waitAfterBlock(c); got != 80*time.Millisecond {
		t.Fatalf("wait %v, want 80ms (4th block, nothing faded)", got)
	}
}

func TestCooldownConcurrent(t *testing.T) {
	c, _ := newTestCooldown()
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				c.Block()
				c.Active()
			}
		}()
	}
	wg.Wait()
	if n, _, active := c.Active(); !active || n != 3200 {
		t.Fatalf("blocks %d active %v, want 3200 and active", n, active)
	}
}
