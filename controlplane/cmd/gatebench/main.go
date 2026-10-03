// Command gatebench scores gate setups (rules, LLM scorers, mixes) against the
// labelled tool-call dataset in bench/gate. It is deliberately not part of
// `go test`: LLM setups cost tokens and time. Run it when changing the gate.
//
//	go -C controlplane run ./cmd/gatebench                        # rules only
//	go -C controlplane run ./cmd/gatebench -setups bench/gate/setups.example.json
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"strings"
	"time"

	gatecases "github.com/tryy3/agent-fabric/bench/gate"
	"github.com/tryy3/agent-fabric/internal/catalog"
	"github.com/tryy3/agent-fabric/internal/db"
	"github.com/tryy3/agent-fabric/internal/gatebench"
)

func main() {
	setupsPath := flag.String("setups", "", "setups JSON file (default: the rules-only setup)")
	only := flag.String("only", "", "comma-separated setup names to run")
	casesDir := flag.String("cases", "", "directory of case JSON files (default: the embedded dataset)")
	cats := flag.String("category", "", "comma-separated categories to run")
	out := flag.String("out", "", "write the JSON report to this file")
	compare := flag.String("compare", "", "baseline JSON report to compare against")
	failUnder := flag.Float64("fail-under", 0, "exit 1 if any setup's composite score is below this")
	jobs := flag.Int("j", 4, "concurrent cases")
	timeout := flag.Duration("case-timeout", 60*time.Second, "per-case timeout")
	verbose := flag.Bool("v", false, "list every case that is off target")
	flag.Parse()

	if err := run(*setupsPath, *only, *casesDir, *cats, *out, *compare, *failUnder, *jobs, *timeout, *verbose); err != nil {
		fmt.Fprintln(os.Stderr, "gatebench:", err)
		os.Exit(1)
	}
}

func run(setupsPath, only, casesDir, cats, out, compare string, failUnder float64, jobs int, timeout time.Duration, verbose bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var fsys fs.FS = gatecases.Cases
	dir := "cases"
	if casesDir != "" {
		fsys, dir = os.DirFS(casesDir), "."
	}
	cases, err := gatebench.LoadCases(fsys, dir)
	if err != nil {
		return err
	}
	cases = gatebench.Filter(cases, splitList(cats))
	if len(cases) == 0 {
		return fmt.Errorf("no cases selected")
	}

	cfg := gatebench.Config{}
	if setupsPath == "" {
		cfg.Setups = []gatebench.SetupConfig{{Name: "rules", Evaluators: []gatebench.EvaluatorConfig{{Type: "rules"}}}}
	} else {
		raw, err := os.ReadFile(setupsPath)
		if err != nil {
			return err
		}
		if cfg, err = gatebench.LoadConfig(raw); err != nil {
			return err
		}
	}
	want := splitList(only)

	resolver, closeDB, err := catalogResolver(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeDB()

	rep := gatebench.Report{GeneratedAt: time.Now().UTC(), Cases: len(cases)}
	for _, sc := range cfg.Setups {
		if len(want) > 0 && !contains(want, sc.Name) {
			continue
		}
		setup, err := gatebench.Build(ctx, sc, gatebench.BuildOptions{Resolve: resolver})
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "running %s on %d cases...\n", sc.Name, len(cases))
		rep.Setups = append(rep.Setups, gatebench.Run(ctx, setup, cases, gatebench.RunOptions{Concurrency: jobs, CaseTimeout: timeout}))
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	if len(rep.Setups) == 0 {
		return fmt.Errorf("no setups selected")
	}

	gatebench.WriteText(os.Stdout, rep, verbose)
	if compare != "" {
		raw, err := os.ReadFile(compare)
		if err != nil {
			return err
		}
		base, err := gatebench.ReadReport(raw)
		if err != nil {
			return fmt.Errorf("read baseline: %w", err)
		}
		gatebench.WriteComparison(os.Stdout, base, rep)
	}
	if out != "" {
		raw, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(out, raw, 0o644); err != nil {
			return err
		}
	}
	for _, s := range rep.Setups {
		if s.Metrics.Composite < failUnder {
			return fmt.Errorf("setup %q composite %.1f is below -fail-under %.1f", s.Name, s.Metrics.Composite, failUnder)
		}
	}
	return nil
}

// catalogResolver connects to Postgres only when a setup references a catalog
// connection by id, so rules-only and direct-endpoint runs need no database.
func catalogResolver(ctx context.Context, cfg gatebench.Config) (gatebench.ConnectionResolver, func(), error) {
	need := false
	for _, s := range cfg.Setups {
		for _, e := range s.Evaluators {
			if e.Connection != nil && e.Connection.ID != "" {
				need = true
			}
		}
	}
	if !need {
		return nil, func() {}, nil
	}
	pool, err := db.OpenPool(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return nil, nil, fmt.Errorf("catalog connections need DATABASE_URL: %w", err)
	}
	store := catalog.Open(pool)
	return func(ctx context.Context, id string) (string, string, string, error) {
		c, err := store.GetInferenceConnection(ctx, id)
		if err != nil {
			return "", "", "", err
		}
		return c.Type, c.BaseURL, c.APIKey, nil
	}, pool.Close, nil
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
