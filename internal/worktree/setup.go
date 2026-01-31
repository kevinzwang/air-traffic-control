package worktree

import (
	"fmt"
	"io"
	"os/exec"
)

// RunSetupCommands executes a list of shell commands in the worktree directory
// Streams output to stdout for user visibility
func RunSetupCommands(worktreePath string, commands []string, output io.Writer) error {
	for _, cmdStr := range commands {
		if cmdStr == "" {
			continue
		}

		fmt.Fprintf(output, "  $ %s\n", cmdStr)

		// Execute command using shell to support piping, environment variables, etc.
		cmd := exec.Command("sh", "-c", cmdStr)
		cmd.Dir = worktreePath
		cmd.Stdout = output
		cmd.Stderr = output

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("command failed: %s: %w", cmdStr, err)
		}
	}

	return nil
}
