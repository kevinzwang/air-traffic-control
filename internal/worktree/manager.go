package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CreateWorktree creates a new git worktree
func CreateWorktree(repoPath, sessionName, branchName, targetPath string) error {
	// Ensure target directory's parent exists
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Create the worktree
	cmd := exec.Command("git", "worktree", "add", "-b", branchName, targetPath, "HEAD")
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create worktree: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// DeleteWorktree removes a git worktree
func DeleteWorktree(worktreePath string) error {
	// Get the parent git repository to execute the command from
	// We need to find the main repo by looking at the worktree's .git file
	gitFile := filepath.Join(worktreePath, ".git")
	data, err := os.ReadFile(gitFile)
	if err != nil {
		return fmt.Errorf("failed to read .git file: %w", err)
	}

	// Parse "gitdir: /path/to/main/repo/.git/worktrees/name"
	gitdir := strings.TrimSpace(strings.TrimPrefix(string(data), "gitdir:"))
	if gitdir == "" {
		return fmt.Errorf("invalid .git file format")
	}

	// Extract main repo path (remove /.git/worktrees/name)
	parts := strings.Split(gitdir, "/.git/worktrees/")
	if len(parts) != 2 {
		return fmt.Errorf("unexpected gitdir format: %s", gitdir)
	}
	mainRepoPath := parts[0]

	// Remove the worktree
	cmd := exec.Command("git", "worktree", "remove", worktreePath, "--force")
	cmd.Dir = mainRepoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove worktree: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// ListWorktrees returns all worktrees for a repository
func ListWorktrees(repoPath string) ([]string, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	worktrees := []string{}
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "worktree ") {
			path := strings.TrimPrefix(line, "worktree ")
			worktrees = append(worktrees, path)
		}
	}

	return worktrees, nil
}

// NormalizeBranchName converts a session name to a valid git branch name
func NormalizeBranchName(name string) string {
	// Convert to lowercase and replace spaces with hyphens
	normalized := strings.ToLower(name)
	normalized = strings.ReplaceAll(normalized, " ", "-")

	// Replace other invalid characters
	normalized = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, normalized)

	return normalized
}
