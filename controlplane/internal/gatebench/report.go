package gatebench

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/tryy3/agent-fabric/internal/agent/gate"
)

// Report is the JSON output of a benchmark run.
type Report struct {
	GeneratedAt time.Time     `json:"generatedAt"`
	Cases       int           `json:"cases"`
	Setups      []SetupResult `json:"setups"`
}

// ReadReport decodes a saved report.
func ReadReport(raw []byte) (Report, error) {
	var r Report
	err := json.Unmarshal(raw, &r)
	return r, err
}

func pct(f float64) string { return fmt.Sprintf("%.0f%%", f*100) }

// WriteText prints the comparison table, per-category breakdown, and the
// cases each setup got wrong (all of them when verbose, else the worst few).
func WriteText(w io.Writer, rep Report, verbose bool) {
	fmt.Fprintf(w, "Gate benchmark: %d cases, %d setups\n\n", rep.Cases, len(rep.Setups))

	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "SETUP\tSCORE\tDECISIONS\tIN-TOL\tDANGER MISS\tOVER-ASK\tMAE\tASYM\tp50/p95 ms\tLLM calls (fail)")
	for _, s := range rep.Setups {
		m := s.Metrics
		llm := "-"
		if s.Usage != nil {
			llm = fmt.Sprintf("%d (%d)", s.Usage.Calls, s.Usage.Failures)
		}
		fmt.Fprintf(tw, "%s\t%.1f\t%s\t%s\t%s\t%s\t%.2f\t%.2f\t%.0f/%.0f\t%s\n",
			s.Name, m.Composite, pct(m.DecisionAvg), pct(m.ScoreWithinTolerance),
			pct(m.DangerMissRate), pct(m.OverAskRate), m.MeanAbsError, m.AsymmetricError,
			m.LatencyP50Ms, m.LatencyP95Ms, llm)
	}
	tw.Flush()

	fmt.Fprintln(w, "\nDecision accuracy by mode")
	tw = tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	header := "SETUP"
	for _, mode := range gate.Modes {
		header += "\t" + string(mode)
	}
	fmt.Fprintln(tw, header)
	for _, s := range rep.Setups {
		line := s.Name
		for _, mode := range gate.Modes {
			line += "\t" + pct(s.Metrics.DecisionAccuracy[mode])
		}
		fmt.Fprintln(tw, line)
	}
	tw.Flush()

	cats := categories(rep)
	fmt.Fprintln(w, "\nBy category (decision accuracy, in-tolerance)")
	tw = tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	header = "CATEGORY\tCASES"
	for _, s := range rep.Setups {
		header += "\t" + s.Name
	}
	fmt.Fprintln(tw, header)
	for _, cat := range cats {
		line := cat
		first := true
		for _, s := range rep.Setups {
			cm := s.Metrics.ByCategory[cat]
			if first {
				line += fmt.Sprintf("\t%d", cm.Cases)
				first = false
			}
			line += fmt.Sprintf("\t%s, %s", pct(cm.DecisionAvg), pct(cm.ScoreWithinTolerance))
		}
		fmt.Fprintln(tw, line)
	}
	tw.Flush()

	for _, s := range rep.Setups {
		if s.Usage == nil || len(s.Usage.Errors) == 0 {
			continue
		}
		fmt.Fprintf(w, "\n%s: LLM scorer failures (each scored fail-closed)\n", s.Name)
		msgs := make([]string, 0, len(s.Usage.Errors))
		for m := range s.Usage.Errors {
			msgs = append(msgs, m)
		}
		sort.Slice(msgs, func(i, j int) bool { return s.Usage.Errors[msgs[i]] > s.Usage.Errors[msgs[j]] })
		for _, m := range msgs {
			fmt.Fprintf(w, "  %4dx %s\n", s.Usage.Errors[m], m)
		}
	}

	for _, s := range rep.Setups {
		wrong := wrongCases(s)
		if len(wrong) == 0 {
			continue
		}
		limit := len(wrong)
		if !verbose && limit > 10 {
			limit = 10
		}
		fmt.Fprintf(w, "\n%s: %d cases off target", s.Name, len(wrong))
		if limit < len(wrong) {
			fmt.Fprintf(w, " (worst %d; -v for all)", limit)
		}
		fmt.Fprintln(w)
		for _, r := range wrong[:limit] {
			fmt.Fprintf(w, "  %-28s want %2d got %2d  %s", r.ID, r.ExpectScore, r.GotScore, r.Tool)
			if r.Err != "" {
				fmt.Fprintf(w, "  ERROR %s", r.Err)
			} else if r.Rationale != "" {
				fmt.Fprintf(w, "  %q", r.Rationale)
			}
			fmt.Fprintln(w)
		}
	}
}

func categories(rep Report) []string {
	set := map[string]bool{}
	for _, s := range rep.Setups {
		for c := range s.Metrics.ByCategory {
			set[c] = true
		}
	}
	out := make([]string, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// wrongCases lists cases outside tolerance or with a wrong decision, the
// most under-scored first.
func wrongCases(s SetupResult) []CaseResult {
	var out []CaseResult
	for _, r := range s.Cases {
		if !r.ScoreOK || !r.DecisionsOK() {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].GotScore-out[i].ExpectScore < out[j].GotScore-out[j].ExpectScore
	})
	return out
}

// WriteComparison prints composite changes against a baseline report and the
// cases that were right in the baseline but are wrong now.
func WriteComparison(w io.Writer, base, cur Report) {
	fmt.Fprintln(w, "\nComparison with baseline")
	baseBy := map[string]SetupResult{}
	for _, s := range base.Setups {
		baseBy[s.Name] = s
	}
	for _, s := range cur.Setups {
		b, ok := baseBy[s.Name]
		if !ok {
			fmt.Fprintf(w, "  %s: not in baseline\n", s.Name)
			continue
		}
		fmt.Fprintf(w, "  %s: composite %.1f -> %.1f (%+.1f), danger miss %s -> %s\n", s.Name,
			b.Metrics.Composite, s.Metrics.Composite, s.Metrics.Composite-b.Metrics.Composite,
			pct(b.Metrics.DangerMissRate), pct(s.Metrics.DangerMissRate))
		was := map[string]CaseResult{}
		for _, r := range b.Cases {
			was[r.ID] = r
		}
		var regress []string
		for _, r := range s.Cases {
			if p, ok := was[r.ID]; ok && p.ScoreOK && p.DecisionsOK() && (!r.ScoreOK || !r.DecisionsOK()) {
				regress = append(regress, fmt.Sprintf("%s (%d -> %d, want %d)", r.ID, p.GotScore, r.GotScore, r.ExpectScore))
			}
		}
		if len(regress) > 0 {
			fmt.Fprintf(w, "    regressions: %s\n", strings.Join(regress, ", "))
		}
	}
}
