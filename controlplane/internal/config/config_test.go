package config_test

import (
	"strings"
	"testing"

	"github.com/tryy3/agent-fabric/internal/config"
)

func TestLoadSuccessTrimsTrailingSlash(t *testing.T) {
	t.Setenv("OPENAI_BASE_URL", "http://127.0.0.1:8000/v1/")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("OPENAI_MODEL", "my-model")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BaseURL != "http://127.0.0.1:8000/v1" {
		t.Fatalf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.APIKey != "sk-test" {
		t.Fatalf("APIKey = %q", cfg.APIKey)
	}
	if cfg.Model != "my-model" {
		t.Fatalf("Model = %q", cfg.Model)
	}
}

func TestLoadMissingBaseURL(t *testing.T) {
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("OPENAI_MODEL", "my-model")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "OPENAI_BASE_URL") {
		t.Fatalf("error = %v, want OPENAI_BASE_URL", err)
	}
}

func TestLoadMissingAPIKey(t *testing.T) {
	t.Setenv("OPENAI_BASE_URL", "http://127.0.0.1:8000/v1")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_MODEL", "my-model")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		t.Fatalf("error = %v, want OPENAI_API_KEY", err)
	}
}

func TestLoadMissingModel(t *testing.T) {
	t.Setenv("OPENAI_BASE_URL", "http://127.0.0.1:8000/v1")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("OPENAI_MODEL", "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "OPENAI_MODEL") {
		t.Fatalf("error = %v, want OPENAI_MODEL", err)
	}
}
