package scrub_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/scrub"
)

func TestJSONStringsScrubsLeafStrings(t *testing.T) {
	s := scrub.JSONStrings{
		Inner: scrub.Fake{Replacements: map[string]string{
			"alice@acme.com": "«Email_1»",
		}},
	}
	in := `{"messages":[{"role":"user","content":"mail alice@acme.com"}]}`
	got, err := s.Scrub(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "«Email_1»") {
		t.Fatalf("email not scrubbed: %s", got)
	}
	if strings.Contains(got, "alice@acme.com") {
		t.Fatalf("raw email leaked: %s", got)
	}
	if !json.Valid([]byte(got)) {
		t.Fatalf("invalid JSON after scrub: %s", got)
	}
}

func TestJSONStringsDoesNotTreatJSONEscapeAsWindowsPath(t *testing.T) {
	// Mimic prompt-scrub PathDetector: raw JSON bytes `Draft:\"` contain `t:\`.
	s := scrub.JSONStrings{
		Inner: scrub.Fake{Replacements: map[string]string{
			`t:\`: "«Path_1»",
		}},
	}
	in := `{"thought":"Draft:\"I en skog av silver\". Done."}`
	got, err := s.Scrub(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "«Path_1»") {
		t.Fatalf("JSON escape falsely scrubbed as path: %s", got)
	}
	if !json.Valid([]byte(got)) {
		t.Fatalf("invalid JSON after scrub: %s", got)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatal(err)
	}
	thought, _ := parsed["thought"].(string)
	if !strings.Contains(thought, `Draft:"I en skog`) {
		t.Fatalf("thought corrupted: %q", thought)
	}
}

func TestJSONStringsStillScrubsRealWindowsPathsInStrings(t *testing.T) {
	s := scrub.JSONStrings{
		Inner: scrub.Fake{Replacements: map[string]string{
			`C:\Users\alice\secret.txt`: "«Path_1»",
		}},
	}
	in := `{"content":"see C:\\Users\\alice\\secret.txt please"}`
	got, err := s.Scrub(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "«Path_1»") {
		t.Fatalf("real path not scrubbed: %s", got)
	}
	if !json.Valid([]byte(got)) {
		t.Fatalf("invalid JSON: %s", got)
	}
}

func TestJSONStringsPlaintextPassthrough(t *testing.T) {
	s := scrub.JSONStrings{
		Inner: scrub.Fake{Replacements: map[string]string{
			"alice@acme.com": "«Email_1»",
		}},
	}
	got, err := s.Scrub(context.Background(), "mail alice@acme.com")
	if err != nil {
		t.Fatal(err)
	}
	if got != "mail «Email_1»" {
		t.Fatalf("got %q", got)
	}
}

func TestPipelineScrubBodyPreservesJSONEscapes(t *testing.T) {
	p := scrub.Pipeline{
		Headers: scrub.DefaultHeaders{},
		Body: scrub.Fake{Replacements: map[string]string{
			`t:\`: "«Path_1»",
		}},
	}
	in := `{"thought":"Draft:\"hello\".","n":1}`
	got := p.ScrubBody(context.Background(), in)
	if strings.Contains(got, "«Path_1»") {
		t.Fatalf("pipeline path false positive: %s", got)
	}
	if !json.Valid([]byte(got)) {
		t.Fatalf("invalid JSON: %s", got)
	}
}

func TestJSONStringsPromptScrubIntegrationNoPathFalsePositive(t *testing.T) {
	bin := os.Getenv("PROMPT_SCRUB_BIN")
	if bin == "" {
		bin = "prompt-scrub"
	}
	if _, err := exec.LookPath(bin); err != nil {
		t.Skip("prompt-scrub not on PATH")
	}
	s := scrub.JSONStrings{Inner: scrub.PromptScrubCLI{Bin: bin}}
	in := `{"thought":"Draft:\"I en skog av silver\". Done. full text:\"En liten\" end."}`
	got, err := s.Scrub(context.Background(), in)
	if err != nil {
		t.Fatalf("Scrub: %v", err)
	}
	if !json.Valid([]byte(got)) {
		t.Fatalf("prompt-scrub left invalid JSON: %s", got)
	}
	if strings.Contains(got, "«Path_") {
		t.Fatalf("Windows path false positive on JSON escape: %s", got)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatal(err)
	}
	thought, _ := parsed["thought"].(string)
	if !strings.Contains(thought, "Draft:") || !strings.Contains(thought, "I en skog") {
		t.Fatalf("thought lost draft text: %q", thought)
	}
}
