package session

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/kevinzwang/air-traffic-control/internal/config"
	"github.com/kevinzwang/air-traffic-control/internal/database"
	"github.com/kevinzwang/air-traffic-control/internal/worktree"
)

// Service manages session operations
type Service struct {
	db       *database.DB
	atcDir   string
	repoPath string
	repoName string
}

// NewService creates a new session service
func NewService(db *database.DB, repoPath string) (*Service, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	atcDir := filepath.Join(homeDir, ".atc")
	repoName := filepath.Base(repoPath)

	return &Service{
		db:       db,
		atcDir:   atcDir,
		repoPath: repoPath,
		repoName: repoName,
	}, nil
}

// RepoName returns the repository name
func (s *Service) RepoName() string {
	return s.repoName
}

// RepoPath returns the repository path
func (s *Service) RepoPath() string {
	return s.repoPath
}

// CreateSession creates a new session with worktree and setup commands
// baseBranch specifies the base for new branches (empty string defaults to HEAD)
// useExistingBranch when true will attach to an existing branch instead of creating a new one
func (s *Service) CreateSession(name, baseBranch string, useExistingBranch bool, output io.Writer) (*Session, error) {
	if err := worktree.ValidateBranchName(name); err != nil {
		return nil, fmt.Errorf("invalid session name: %w", err)
	}

	existing, _ := s.db.GetSessionByName(name)
	if existing != nil {
		return nil, fmt.Errorf("session with name '%s' already exists", name)
	}

	if useExistingBranch {
		existingByBranch, err := s.db.GetSessionByBranchName(name)
		if err != nil {
			return nil, fmt.Errorf("failed to check branch: %w", err)
		}
		if existingByBranch != nil {
			return nil, fmt.Errorf("branch '%s' already has a session", name)
		}
	}

	sess := &Session{
		ID:           uuid.New().String(),
		Name:         name,
		RepoPath:     s.repoPath,
		RepoName:     s.repoName,
		WorktreePath: filepath.Join(s.atcDir, "worktrees", s.repoName, name),
		BranchName:   name,
		CreatedAt:    time.Now(),
		Status:       "active",
	}

	fmt.Fprintf(output, "Creating git worktree...\n")
	if err := worktree.CreateWorktree(s.repoPath, name, sess.BranchName, sess.WorktreePath, baseBranch, useExistingBranch); err != nil {
		return nil, fmt.Errorf("failed to create worktree: %w", err)
	}

	// cleanupWorktree ensures worktree is removed on any subsequent error
	cleanupWorktree := func() { worktree.DeleteWorktree(sess.WorktreePath) }

	cfg, err := config.Load(s.repoPath)
	if err != nil {
		cleanupWorktree()
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Fprintf(output, "Worktree created\n")
	if len(cfg.SetupWorktree) > 0 {
		fmt.Fprintf(output, "Running setup commands...\n")
		if err := worktree.RunSetupCommands(sess.WorktreePath, cfg.SetupWorktree, output); err != nil {
			cleanupWorktree()
			return nil, fmt.Errorf("setup commands failed: %w", err)
		}
		fmt.Fprintf(output, "Setup complete\n")
	}

	if err := s.db.InsertSession(sess.toDBSession()); err != nil {
		cleanupWorktree()
		return nil, fmt.Errorf("failed to save session: %w", err)
	}

	return sess, nil
}

// ListSessions returns all sessions, optionally filtered by query
func (s *Service) ListSessions(query string) ([]*Session, error) {
	dbSessions, err := s.db.ListSessions(s.repoName, query)
	if err != nil {
		return nil, err
	}

	sessions := make([]*Session, len(dbSessions))
	for i, dbs := range dbSessions {
		sessions[i] = fromDBSession(dbs)
	}
	return sessions, nil
}

// GetSession retrieves a session by name
func (s *Service) GetSession(name string) (*Session, error) {
	dbs, err := s.db.GetSessionByName(name)
	if err != nil {
		return nil, err
	}
	return fromDBSession(dbs), nil
}

// DeleteSession removes a session and its worktree.
// The caller (TUI) is responsible for closing the terminal process first.
func (s *Service) DeleteSession(name string) error {
	session, err := s.GetSession(name)
	if err != nil {
		return err
	}

	// Remove worktree
	if err := worktree.DeleteWorktree(session.WorktreePath); err != nil {
		return fmt.Errorf("failed to remove worktree: %w", err)
	}

	// Remove from database
	if err := s.db.DeleteSession(session.ID); err != nil {
		return fmt.Errorf("failed to delete session from database: %w", err)
	}

	return nil
}

// ArchiveSession marks a session as archived
func (s *Service) ArchiveSession(name string) error {
	session, err := s.GetSession(name)
	if err != nil {
		return err
	}

	return s.db.ArchiveSession(session.ID)
}

// UnarchiveSession marks a session as active
func (s *Service) UnarchiveSession(name string) error {
	session, err := s.GetSession(name)
	if err != nil {
		return err
	}

	return s.db.UnarchiveSession(session.ID)
}

// ListBranches returns all local branches in the repository
func (s *Service) ListBranches() ([]string, error) {
	return worktree.ListBranches(s.repoPath)
}

// GetCurrentBranch returns the current HEAD branch name
func (s *Service) GetCurrentBranch() (string, error) {
	return worktree.GetCurrentBranch(s.repoPath)
}

// GetSessionByBranch returns a session for a given branch name, or nil if none exists
func (s *Service) GetSessionByBranch(branchName string) (*Session, error) {
	dbs, err := s.db.GetSessionByBranchName(branchName)
	if err != nil {
		return nil, err
	}
	if dbs == nil {
		return nil, nil
	}
	return fromDBSession(dbs), nil
}

// TouchSession updates the last accessed time for a session
func (s *Service) TouchSession(name string) error {
	sess, err := s.GetSession(name)
	if err != nil {
		return err
	}

	now := time.Now()
	sess.LastAccessed = &now

	if err := s.db.UpdateSession(sess.toDBSession()); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	return nil
}
