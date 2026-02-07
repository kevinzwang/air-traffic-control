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
- **internal/tui/** - Bubble Tea model with split-pane layout (sidebar + embedded terminal), overlay modals for create/delete/help
- **internal/terminal/** - tmux session wrapper per session (capture-pane rendering, scrollback, resize)
- **internal/session/** - Business logic and Session domain model
- **internal/database/** - SQLite persistence (~/.atc/sessions.db)
- **internal/worktree/** - Git worktree operations
- **internal/config/** - Parses `.cursor/worktrees.json` for setup commands

### Key Flow

1. User creates session → validates name → creates git worktree + branch
2. Runs setup commands from `.cursor/worktrees.json` (if present)
3. Spawns `claude` in a tmux session inside the worktree, renders output via `capture-pane`
4. User interacts with the embedded terminal directly (keystrokes forwarded via tmux `send-keys`)
5. Sessions can be archived or deleted (worktree cleanup)

### TUI Architecture

- **Split-pane layout**: Fixed-width sidebar (session list) + terminal pane (embedded claude session)
- **Focus model**: `Ctrl+C` switches focus from terminal back to sidebar; Enter/selection activates terminal
- **Overlay modals**: Create session, delete confirmation, help, branch selection — rendered on top of the split pane
- **tmux integration**: Each active session has a `terminal.Terminal` instance that manages a tmux session. A goroutine polls `capture-pane -p -e` for output and sends Bubble Tea messages to trigger re-renders.
- **Mouse support**: Click+drag text selection with clipboard copy, mouse wheel scrollback

### Conventions

- Session name = git branch name (alphanumeric, `-_/.` allowed, no spaces)
- Worktrees stored at `~/.atc/worktrees/<repo-name>/<session-name>`
- Database at `~/.atc/sessions.db`
- TUI uses Bubble Tea message-driven async pattern with custom message types (e.g., `sessionCreatedMsg`, `errMsg`, `terminal.TerminalOutputMsg`, `terminal.TerminalExitedMsg`)
- tmux sessions persist across ATC restarts. Existing tmux sessions are reattached on startup; stopped sessions can be restarted with `--continue`.

### Dependencies

- **Bubble Tea** - TUI framework
- **Bubbles** - TUI components (textinput, spinner)
- **Lip Gloss** - Terminal styling
- **go-sqlite3** - Database driver
- **tmux** - Terminal multiplexer (runtime dependency, not a Go module)
