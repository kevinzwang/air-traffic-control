package worktree

import (
	"bufio"
	"encoding/json"
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

// GetConversationSummary returns the most recent conversation summary for a worktree.
// Returns empty string if no conversation or summary found.
func GetConversationSummary(worktreePath string) string {
	projectDir := getClaudeProjectDir(worktreePath)
	if projectDir == "" {
		return ""
	}

	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return ""
	}

	// Search all .jsonl files for summaries, return the last one found
	var lastSummary string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".jsonl") {
			filePath := filepath.Join(projectDir, entry.Name())
			if summary := extractLastSummary(filePath); summary != "" {
				lastSummary = summary
			}
		}
	}

	return lastSummary
}

// extractLastSummary reads a JSONL file and returns the last summary found
func extractLastSummary(filePath string) string {
	file, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer file.Close()

	var lastSummary string
	scanner := bufio.NewScanner(file)
	// Increase buffer size for large lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()

		// Quick check before parsing JSON
		if !strings.Contains(string(line), `"summary"`) {
			continue
		}

		var entry struct {
			Type    string `json:"type"`
			Summary string `json:"summary"`
		}

		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		if entry.Type == "summary" && entry.Summary != "" {
			lastSummary = entry.Summary
		}
	}

	return lastSummary
}
