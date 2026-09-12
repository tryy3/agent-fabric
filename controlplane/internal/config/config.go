package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	BaseURL string
	APIKey  string
	Model   string
}

func Load() (Config, error) {
	cfg := Config{
		BaseURL: strings.TrimRight(os.Getenv("OPENAI_BASE_URL"), "/"),
		APIKey:  os.Getenv("OPENAI_API_KEY"),
		Model:   os.Getenv("OPENAI_MODEL"),
	}
	switch {
	case cfg.BaseURL == "":
		return Config{}, fmt.Errorf("missing required env OPENAI_BASE_URL")
	case cfg.APIKey == "":
		return Config{}, fmt.Errorf("missing required env OPENAI_API_KEY")
	case cfg.Model == "":
		return Config{}, fmt.Errorf("missing required env OPENAI_MODEL")
	}
	return cfg, nil
}
