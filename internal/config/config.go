package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WorktreeConfig represents the structure of .cursor/worktrees.json
type WorktreeConfig struct {
	SetupWorktree []string `json:"setup-worktree"`
}

// Load finds and parses .cursor/worktrees.json starting from the given directory
// Returns an empty config if no file is found (graceful degradation)
func Load(startDir string) (*WorktreeConfig, error) {
	configPath, err := findConfig(startDir)
	if err != nil {
		// Return empty config if not found
		return &WorktreeConfig{
			SetupWorktree: []string{},
		}, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config WorktreeConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// findConfig searches for .cursor/worktrees.json in the directory tree
func findConfig(startDir string) (string, error) {
	dir := startDir

	for {
		configPath := filepath.Join(dir, ".cursor", "worktrees.json")
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}

		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root
			return "", fmt.Errorf("config file not found")
		}
		dir = parent
	}
}
