package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config holds the application configuration
type Config struct {
	FireflyURL   string
	FireflyToken string
}

// AccountMatcher defines the matching criteria for account mapping
type AccountMatcher struct {
	Description *string `json:"description,omitempty"`
	Category    *string `json:"category,omitempty"`
	Source      *string `json:"source,omitempty"`
	Destination *string `json:"destination,omitempty"`
}

// Load loads the configuration from environment variables and config files
func Load() (*Config, error) {
	// Load .env file (silently ignore if it doesn't exist)
	_ = godotenv.Load()

	cfg := &Config{
		FireflyURL:   os.Getenv("FIREFLY_URL"),
		FireflyToken: os.Getenv("FIREFLY_TOKEN"),
	}

	if cfg.FireflyToken == "" {
		return nil, fmt.Errorf("FIREFLY_TOKEN environment variable is required")
	}

	return cfg, nil
}
