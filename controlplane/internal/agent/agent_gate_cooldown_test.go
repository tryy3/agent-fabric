package agent

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tryy3/agent-fabric/internal/agent/gate"
)

type cooldownClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *cooldownClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *cooldownClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newCooldownAgent() (*Agent, *cooldownClock) {
	clk := &cooldownClock{t: time.Unix(1_700_000_000, 0)}
	return &Agent{cooldownClock: clk.now}, clk
}

// gated fakes runGatedTool: it records a decision on the trace and counts runs.
type gated struct {
	runs int
	tr   gateTrace
	err  error
}

func (g *gated) run(tr *gateTrace) func() (string, error) {
	return func() (string, error) {
		g.runs++
		*tr = g.tr
		return "", g.err
	}
}

func blocked(outcome string, risk int) *gated {
	return &gated{
		tr:  gateTrace{Evaluated: true, Outcome: outcome, Decision: gate.Decision{Kind: gate.Deny, Risk: risk}},
		err: errors.New("denied: x"),
	}
}

func call(a *Agent, session string, g *gated) (gateTrace, error) {
	var tr gateTrace
	_, err := a.withCooldown(session, &tr, g.run(&tr))
	return tr, err
}

func TestCooldownBlocksFollowingCallWithoutEvaluating(t *testing.T) {
	a, clk := newCooldownAgent()
	g := blocked(outcomeDenied, 10)
	call(a, "s", g)
	tr, err := call(a, "s", g)
	if g.runs != 1 {
		t.Fatalf("gate ran %d times, want 1", g.runs)
	}
	if err == nil || !strings.HasPrefix(err.Error(), "denied: cooling down after 1 blocked calls, next attempt in ~") {
		t.Fatalf("err = %v", err)
	}
	if tr.Outcome != outcomeCooldown || !tr.Evaluated {
		t.Fatalf("trace = %+v", tr)
	}
	if m := tr.meta(); m["outcome"] != "cooldown" || m["verdict"] != "deny" {
		t.Fatalf("meta = %v", m)
	}
	clk.advance(10 * time.Millisecond)
	call(a, "s", g)
	if g.runs != 2 {
		t.Fatalf("gate ran %d times after the wait, want 2", g.runs)
	}
}

func TestCooldownRetryDuringCooldownDoesNotCount(t *testing.T) {
	a, clk := newCooldownAgent()
	g := blocked(outcomeCancelled, 9)
	call(a, "s", g)
	for range 10 {
		call(a, "s", g)
	}
	clk.advance(10 * time.Millisecond)
	call(a, "s", g) // second real block: 20 ms
	if _, err := call(a, "s", g); !strings.Contains(err.Error(), "after 2 blocked calls") {
		t.Fatalf("err = %v, want 2 blocked calls", err)
	}
}

func TestCooldownIgnoresNonGateBlocks(t *testing.T) {
	for name, g := range map[string]*gated{
		"rejected":   blocked(outcomeRejected, 5),
		"dismissed":  blocked(outcomeDismissed, 5),
		"structural": blocked(outcomeDenied, 0),
		"allowed":    {tr: gateTrace{Evaluated: true, Outcome: outcomeAllowed, Decision: gate.Decision{Kind: gate.Allow, Risk: 1}}},
		"approved":   {tr: gateTrace{Evaluated: true, Outcome: outcomeApproved, Decision: gate.Decision{Kind: gate.Ask, Risk: 5}}},
	} {
		a, _ := newCooldownAgent()
		for range 3 {
			if _, err := call(a, "s", g); err != nil && strings.Contains(err.Error(), "cooling down") {
				t.Errorf("%s: cooled down", name)
			}
		}
		if g.runs != 3 {
			t.Errorf("%s: gate ran %d times, want 3", name, g.runs)
		}
	}
}

func TestCooldownAllowedCallsDoNotLaunderCount(t *testing.T) {
	a, clk := newCooldownAgent()
	deny := blocked(outcomeDenied, 10)
	ok := &gated{tr: gateTrace{Evaluated: true, Outcome: outcomeAllowed, Decision: gate.Decision{Kind: gate.Allow, Risk: 1}}}
	call(a, "s", deny)
	clk.advance(10 * time.Millisecond)
	call(a, "s", ok)
	call(a, "s", deny) // second block
	if _, err := call(a, "s", ok); !strings.Contains(err.Error(), "after 2 blocked calls") {
		t.Fatalf("err = %v", err)
	}
}

func TestCooldownTaintedSessionUnaffected(t *testing.T) {
	a, _ := newCooldownAgent()
	g := &gated{tr: gateTrace{Tainted: true, Evaluated: true, Outcome: outcomeApproved, Decision: gate.Decision{Kind: gate.Ask, Risk: 6}}}
	for range 3 {
		if _, err := call(a, "s", g); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCooldownSessionsAreIsolated(t *testing.T) {
	a, _ := newCooldownAgent()
	call(a, "one", blocked(outcomeDenied, 10))
	g := &gated{tr: gateTrace{Evaluated: true, Outcome: outcomeAllowed, Decision: gate.Decision{Kind: gate.Allow, Risk: 1}}}
	if _, err := call(a, "two", g); err != nil || g.runs != 1 {
		t.Fatalf("session two affected: err=%v runs=%d", err, g.runs)
	}
}

func TestCooldownConcurrentCalls(t *testing.T) {
	a, _ := newCooldownAgent()
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			session := "a"
			if i%2 == 1 {
				session = "b"
			}
			for range 50 {
				var tr gateTrace
				a.withCooldown(session, &tr, func() (string, error) {
					tr = gateTrace{Evaluated: true, Outcome: outcomeDenied, Decision: gate.Decision{Kind: gate.Deny, Risk: 10}}
					return "", errors.New("denied")
				})
			}
		}()
	}
	wg.Wait()
}

func TestCooldownSeconds(t *testing.T) {
	for in, want := range map[time.Duration]string{
		10 * time.Millisecond:   "0.01",
		15 * time.Millisecond:   "0.02",
		time.Second:             "1",
		1500 * time.Millisecond: "2",
		5 * time.Minute:         "300",
	} {
		if got := cooldownSeconds(in); got != want {
			t.Errorf("cooldownSeconds(%v) = %q, want %q", in, got, want)
		}
	}
}
