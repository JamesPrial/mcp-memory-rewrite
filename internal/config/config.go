package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	DBPath string
}

// Load loads configuration from environment variables with defaults
func Load() (*Config, error) {
	cfg := &Config{}

	// Database path configuration
	cfg.DBPath = os.Getenv("MEMORY_DB_PATH")
	if cfg.DBPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		cfg.DBPath = filepath.Join(homeDir, ".mcp-memory", "memory.db")
	}

	// Ensure the directory exists
	dir := filepath.Dir(cfg.DBPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	return cfg, nil
}