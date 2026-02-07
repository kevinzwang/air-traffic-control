package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Session represents a session record
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

// InsertSession adds a new session to the database
func (db *DB) InsertSession(s *Session) error {
	query := `
		INSERT INTO sessions (
			id, name, repo_path, repo_name, worktree_path, branch_name,
			created_at, last_accessed, archived_at, status
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := db.conn.Exec(query,
		s.ID, s.Name, s.RepoPath, s.RepoName, s.WorktreePath, s.BranchName,
		s.CreatedAt, s.LastAccessed, s.ArchivedAt, s.Status,
	)
	if err != nil {
		return fmt.Errorf("failed to insert session: %w", err)
	}
	return nil
}

// GetSessionByName retrieves a session by its name
func (db *DB) GetSessionByName(name string) (*Session, error) {
	query := `
		SELECT id, name, repo_path, repo_name, worktree_path, branch_name,
		       created_at, last_accessed, archived_at, status
		FROM sessions
		WHERE name = ?
	`

	var s Session
	err := db.conn.QueryRow(query, name).Scan(
		&s.ID, &s.Name, &s.RepoPath, &s.RepoName, &s.WorktreePath, &s.BranchName,
		&s.CreatedAt, &s.LastAccessed, &s.ArchivedAt, &s.Status,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("session not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}
	return &s, nil
}

// GetSessionByBranchName retrieves a session by its branch name
func (db *DB) GetSessionByBranchName(branchName string) (*Session, error) {
	query := `
		SELECT id, name, repo_path, repo_name, worktree_path, branch_name,
		       created_at, last_accessed, archived_at, status
		FROM sessions
		WHERE branch_name = ?
	`

	var s Session
	err := db.conn.QueryRow(query, branchName).Scan(
		&s.ID, &s.Name, &s.RepoPath, &s.RepoName, &s.WorktreePath, &s.BranchName,
		&s.CreatedAt, &s.LastAccessed, &s.ArchivedAt, &s.Status,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil // Return nil, nil when not found (branch has no session)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session by branch: %w", err)
	}
	return &s, nil
}

// ListSessions retrieves sessions with optional filtering
func (db *DB) ListSessions(repoFilter string, query string) ([]*Session, error) {
	querySQL := `
		SELECT id, name, repo_path, repo_name, worktree_path, branch_name,
		       created_at, last_accessed, archived_at, status
		FROM sessions
		WHERE 1=1
	`
	args := []interface{}{}

	// Filter by repo if specified
	if repoFilter != "" {
		querySQL += " AND repo_name = ?"
		args = append(args, repoFilter)
	}

	// Fuzzy search by name if query is provided
	if query != "" {
		querySQL += " AND LOWER(name) LIKE ?"
		args = append(args, "%"+strings.ToLower(query)+"%")
	}

	querySQL += " ORDER BY created_at DESC"

	rows, err := db.conn.Query(querySQL, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}
	defer rows.Close()

	sessions := []*Session{}
	for rows.Next() {
		var s Session
		err := rows.Scan(
			&s.ID, &s.Name, &s.RepoPath, &s.RepoName, &s.WorktreePath, &s.BranchName,
			&s.CreatedAt, &s.LastAccessed, &s.ArchivedAt, &s.Status,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}
		sessions = append(sessions, &s)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating sessions: %w", err)
	}

	return sessions, nil
}

// UpdateSession updates a session's metadata
func (db *DB) UpdateSession(s *Session) error {
	query := `
		UPDATE sessions
		SET name = ?, repo_path = ?, repo_name = ?, worktree_path = ?,
		    branch_name = ?, last_accessed = ?, archived_at = ?, status = ?
		WHERE id = ?
	`

	_, err := db.conn.Exec(query,
		s.Name, s.RepoPath, s.RepoName, s.WorktreePath, s.BranchName,
		s.LastAccessed, s.ArchivedAt, s.Status, s.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}
	return nil
}

// ArchiveSession marks a session as archived
func (db *DB) ArchiveSession(id string) error {
	now := time.Now()
	query := `
		UPDATE sessions
		SET archived_at = ?, status = 'archived'
		WHERE id = ?
	`

	_, err := db.conn.Exec(query, now, id)
	if err != nil {
		return fmt.Errorf("failed to archive session: %w", err)
	}
	return nil
}

// UnarchiveSession marks a session as active
func (db *DB) UnarchiveSession(id string) error {
	query := `
		UPDATE sessions
		SET archived_at = NULL, status = 'active'
		WHERE id = ?
	`

	_, err := db.conn.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to unarchive session: %w", err)
	}
	return nil
}

// DeleteSession removes a session from the database
func (db *DB) DeleteSession(id string) error {
	query := `DELETE FROM sessions WHERE id = ?`

	_, err := db.conn.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	return nil
}
