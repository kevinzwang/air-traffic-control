package worktree

import (
	"fmt"
)

// GetContinueCommand returns the command to cd to the worktree and exec claude --continue
func GetContinueCommand(worktreePath string) string {
	return fmt.Sprintf("cd %s && exec claude --continue", worktreePath)
}
