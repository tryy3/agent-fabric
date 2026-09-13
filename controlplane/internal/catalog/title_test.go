package catalog

import "testing"

func TestAutoTitleFirstEightWords(t *testing.T) {
	got := AutoTitle("  How do I pin an agent to a thread please  ")
	want := "How do I pin an agent to a"
	if got != want {
		t.Fatalf("AutoTitle = %q, want %q", got, want)
	}
}

func TestAutoTitleShortPrompt(t *testing.T) {
	got := AutoTitle("hello there")
	if got != "hello there" {
		t.Fatalf("AutoTitle = %q", got)
	}
}

func TestAutoTitleEmptyStaysUntitled(t *testing.T) {
	got := AutoTitle("   ")
	if got != "Untitled" {
		t.Fatalf("AutoTitle = %q, want Untitled", got)
	}
}
