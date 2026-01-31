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

// CreateSession creates a new session with worktree and setup commands
func (s *Service) CreateSession(name string, output io.Writer) (*Session, error) {
	// Check if session name already exists
	// Validate name is valid for git branch
	if err := worktree.ValidateBranchName(name); err != nil {
		return nil, fmt.Errorf("invalid session name: %w", err)
	}

	existing, _ := s.db.GetSessionByName(name)
	if existing != nil {
		return nil, fmt.Errorf("session with name '%s' already exists", name)
	}

	// Create session record (name = branch name)
	session := &Session{
		ID:           uuid.New().String(),
		Name:         name,
		RepoPath:     s.repoPath,
		RepoName:     s.repoName,
		WorktreePath: filepath.Join(s.atcDir, "worktrees", s.repoName, name),
		BranchName:   name,
		CreatedAt:    time.Now(),
		Status:       "active",
	}

	// Create worktree
	fmt.Fprintf(output, "Creating git worktree...\n")
	if err := worktree.CreateWorktree(s.repoPath, name, session.BranchName, session.WorktreePath); err != nil {
		return nil, fmt.Errorf("failed to create worktree: %w", err)
	}

	// Load config for setup commands
	cfg, err := config.Load(s.repoPath)
	if err != nil {
		// Clean up worktree on error
		worktree.DeleteWorktree(session.WorktreePath)
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Run setup commands if any
	if len(cfg.SetupWorktree) > 0 {
		fmt.Fprintf(output, "✓ Worktree created\n")
		fmt.Fprintf(output, "Running setup commands...\n")
		if err := worktree.RunSetupCommands(session.WorktreePath, cfg.SetupWorktree, output); err != nil {
			// Clean up worktree on error
			worktree.DeleteWorktree(session.WorktreePath)
			return nil, fmt.Errorf("setup commands failed: %w", err)
		}
		fmt.Fprintf(output, "✓ Setup complete\n")
	} else {
		fmt.Fprintf(output, "✓ Worktree created\n")
	}

	// Save to database
	if err := s.db.InsertSession(session.toDBSession()); err != nil {
		// Clean up worktree on error
		worktree.DeleteWorktree(session.WorktreePath)
		return nil, fmt.Errorf("failed to save session: %w", err)
	}

	return session, nil
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

// DeleteSession removes a session and its worktree
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

// EnterSession prepares to enter a session and returns the command to exec
func (s *Service) EnterSession(name string) (string, error) {
	sess, err := s.GetSession(name)
	if err != nil {
		return "", err
	}

	// Update last accessed time
	now := time.Now()
	sess.LastAccessed = &now

	if err := s.db.UpdateSession(sess.toDBSession()); err != nil {
		return "", fmt.Errorf("failed to update session: %w", err)
	}

	return worktree.GetClaudeCommand(sess.WorktreePath), nil
}
