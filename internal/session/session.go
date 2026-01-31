package session

import (
	"time"

	"github.com/kevinzwang/air-traffic-control/internal/database"
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

// fromDBSession converts a database.Session to a session.Session
func fromDBSession(dbs *database.Session) *Session {
	return &Session{
		ID:           dbs.ID,
		Name:         dbs.Name,
		RepoPath:     dbs.RepoPath,
		RepoName:     dbs.RepoName,
		WorktreePath: dbs.WorktreePath,
		BranchName:   dbs.BranchName,
		CreatedAt:    dbs.CreatedAt,
		LastAccessed: dbs.LastAccessed,
		ArchivedAt:   dbs.ArchivedAt,
		Status:       dbs.Status,
	}
}

// toDBSession converts a session.Session to a database.Session
func (s *Session) toDBSession() *database.Session {
	return &database.Session{
		ID:           s.ID,
		Name:         s.Name,
		RepoPath:     s.RepoPath,
		RepoName:     s.RepoName,
		WorktreePath: s.WorktreePath,
		BranchName:   s.BranchName,
		CreatedAt:    s.CreatedAt,
		LastAccessed: s.LastAccessed,
		ArchivedAt:   s.ArchivedAt,
		Status:       s.Status,
	}
}
