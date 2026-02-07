package worktree

import (
	"os"
	"path/filepath"
	"strings"
)

// getClaudeProjectDir returns the Claude Code project directory for a worktree path.
// Claude encodes paths by replacing / and . with -.
// Returns empty string if the directory cannot be determined.
func getClaudeProjectDir(worktreePath string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	absPath, err := filepath.Abs(worktreePath)
	if err != nil {
		return ""
	}

	// Convert path to Claude's directory naming convention
	// e.g., /Users/kevin/.atc/project -> -Users-kevin--atc-project
	encodedPath := strings.ReplaceAll(absPath, "/", "-")
	encodedPath = strings.ReplaceAll(encodedPath, ".", "-")

	return filepath.Join(homeDir, ".claude", "projects", encodedPath)
}

// HasExistingConversation checks if there are any Claude Code conversations
// for the given worktree path by looking in ~/.claude/projects/
func HasExistingConversation(worktreePath string) bool {
	projectDir := getClaudeProjectDir(worktreePath)
	if projectDir == "" {
		return false
	}

	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".jsonl") {
			return true
		}
	}
	return false
}

