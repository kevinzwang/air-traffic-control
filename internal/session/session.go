package session

import (
	"time"
)

// Session represents a Claude Code session in a git worktree
type Session struct {
	ID            string
	Name          string
	RepoPath      string
	RepoName      string
	WorktreePath  string
	BranchName    string
	CreatedAt     time.Time
	LastAccessed  *time.Time
	ArchivedAt    *time.Time
	Status        string
}
