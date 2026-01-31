package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
}

// Open connects to the SQLite database and creates it if it doesn't exist
func Open(path string) (*DB, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db := &DB{conn: conn}

	// Run migrations
	if err := db.Migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.conn.Close()
}

// Migrate creates the database schema
func (db *DB) Migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		repo_path TEXT NOT NULL,
		repo_name TEXT NOT NULL,
		worktree_path TEXT NOT NULL,
		branch_name TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		last_accessed TIMESTAMP,
		archived_at TIMESTAMP,
		status TEXT DEFAULT 'active'
	);

	CREATE INDEX IF NOT EXISTS idx_sessions_repo ON sessions(repo_name);
	CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);
	CREATE INDEX IF NOT EXISTS idx_sessions_archived ON sessions(archived_at);
	`

	_, err := db.conn.Exec(schema)
	return err
}
