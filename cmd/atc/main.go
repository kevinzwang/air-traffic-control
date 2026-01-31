package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kevinzwang/air-traffic-control/internal/database"
	"github.com/kevinzwang/air-traffic-control/internal/session"
	"github.com/kevinzwang/air-traffic-control/internal/tui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Get current directory (should be a git repo)
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Check if we're in a git repository
	if !isGitRepo(cwd) {
		return fmt.Errorf("not a git repository (or any of the parent directories)")
	}

	// Get the root of the git repository
	repoPath, err := getGitRoot(cwd)
	if err != nil {
		return fmt.Errorf("failed to get git root: %w", err)
	}

	// Get home directory for ATC database
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	// Open database
	dbPath := filepath.Join(homeDir, ".atc", "sessions.db")
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Create session service
	service, err := session.NewService(db, repoPath)
	if err != nil {
		return fmt.Errorf("failed to create session service: %w", err)
	}

	// Extract repo name for display
	repoName := filepath.Base(repoPath)

	// Launch TUI
	model := tui.NewModel(service, repoName)
	p := tea.NewProgram(model, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	// Check if we need to exec into a session
	m := finalModel.(*tui.Model)
	if m.GetCommandToExec() != "" {
		cmd := m.GetCommandToExec()
		return execCommand(cmd)
	}

	return nil
}

// isGitRepo checks if the directory is inside a git repository
func isGitRepo(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = dir
	return cmd.Run() == nil
}

// getGitRoot returns the root directory of the git repository
func getGitRoot(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// execCommand replaces the current process with the given shell command
func execCommand(cmdStr string) error {
	fmt.Printf("\nEntering session...\n")

	// Use bash to execute the command
	cmd := exec.Command("bash", "-c", cmdStr)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
