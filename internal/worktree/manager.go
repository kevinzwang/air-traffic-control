package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CreateWorktree creates a new git worktree
// If useExisting is true, it attaches to an existing branch instead of creating a new one
// baseBranch specifies the base for new branches (ignored when useExisting is true)
func CreateWorktree(repoPath, sessionName, branchName, targetPath, baseBranch string, useExisting bool) error {
	// Ensure target directory's parent exists
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	var cmd *exec.Cmd
	if useExisting {
		// Attach worktree to existing branch
		cmd = exec.Command("git", "worktree", "add", targetPath, branchName)
	} else {
		// Create new branch from base
		if baseBranch == "" {
			baseBranch = "HEAD"
		}
		cmd = exec.Command("git", "worktree", "add", "-b", branchName, targetPath, baseBranch)
	}
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

// ListBranches returns all local branch names for a repository
func ListBranches(repoPath string) ([]string, error) {
	cmd := exec.Command("git", "branch", "--format=%(refname:short)")
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w\nOutput: %s", err, string(output))
	}

	branches := []string{}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}

	return branches, nil
}

// GetCurrentBranch returns the name of the current HEAD branch
func GetCurrentBranch(repoPath string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w\nOutput: %s", err, string(output))
	}

	return strings.TrimSpace(string(output)), nil
}

// ValidateBranchName checks if a name is valid for use as a git branch name
// Returns an error describing the issue if invalid, nil if valid
func ValidateBranchName(name string) error {
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}

	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("name cannot start with '-'")
	}

	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("name cannot start with '.'")
	}

	if strings.Contains(name, "..") {
		return fmt.Errorf("name cannot contain '..'")
	}

	if strings.Contains(name, " ") {
		return fmt.Errorf("name cannot contain spaces")
	}

	for _, r := range name {
		if !isValidBranchChar(r) {
			return fmt.Errorf("name contains invalid character '%c'", r)
		}
	}

	return nil
}

// isValidBranchChar returns true if the rune is valid in a git branch name
func isValidBranchChar(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	case r >= '0' && r <= '9':
		return true
	case r == '-', r == '_', r == '/', r == '.':
		return true
	default:
		return false
	}
}
