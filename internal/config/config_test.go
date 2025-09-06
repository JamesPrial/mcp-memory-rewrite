package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	// Test case 1: Environment variables not set
	os.Unsetenv("MEMORY_DB_PATH")
	cfg, err := Load()
	assert.NoError(t, err)
	assert.Contains(t, cfg.DBPath, ".mcp-memory/memory.db")

	// Test case 2: Environment variable set
	os.Setenv("MEMORY_DB_PATH", "/tmp/test.db")
	cfg, err = Load()
	assert.NoError(t, err)
	assert.Equal(t, "/tmp/test.db", cfg.DBPath)
}
