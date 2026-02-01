# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build Commands

```bash
# Build the binary
go build -o atc ./cmd/atc

# Run tests
go test ./...

# Download dependencies
go mod download
```

## Architecture

ATC (Air Traffic Control) is a Go CLI tool for managing Claude Code agent sessions across isolated git worktrees. It uses a Bubble Tea TUI for interaction.

### Layered Structure

- **cmd/atc/main.go** - Entry point, initializes database and launches TUI
- **internal/tui/** - Bubble Tea model with three states: `stateMain`, `stateCreating`, `stateConfirmDelete`
- **internal/session/** - Business logic and Session domain model
- **internal/database/** - SQLite persistence (~/.atc/sessions.db)
- **internal/worktree/** - Git worktree operations and Claude CLI integration
- **internal/config/** - Parses `.cursor/worktrees.json` for setup commands

### Key Flow

1. User creates session → validates name → creates git worktree + branch
2. Runs setup commands from `.cursor/worktrees.json` (if present)
3. User selects session → execs `claude --continue` in worktree directory
4. Sessions can be archived or deleted (worktree cleanup)

### Conventions

- Session name = git branch name (alphanumeric, `-_/.` allowed, no spaces)
- Worktrees stored at `~/.atc/worktrees/<repo-name>/<session-name>`
- Database at `~/.atc/sessions.db`
- TUI uses Bubble Tea message-driven async pattern with custom message types (e.g., `sessionCreatedMsg`, `errMsg`)

### Dependencies

- **Bubble Tea** - TUI framework
- **Bubbles** - TUI components (list, textinput, spinner)
- **Lip Gloss** - Terminal styling
- **go-sqlite3** - Database driver
