# ✈️ Air Traffic Control

Manage a fleet of coding agents without going insane. 

Air Traffic Control (ATC) is a TUI that helps you isolate and collaborate with multiple Claude Code instances on the same project.
Manage agent "sessions", which tie together agents with code and git state. Let ATC automatically handle setup and management of git worktrees, and seamlessly spawn and move across agent conversations.

<p align="center">
  <img src="screenshot.png" width="67%" alt="ATC Screenshot">
</p>

## Features

- **Embedded Terminal**: Claude Code sessions run inside a split-pane TUI via tmux — no more switching windows
- **Session Management**: Create, list, archive, and delete Claude Code sessions
- **Git Worktrees**: Each session runs in its own isolated git worktree
- **Fuzzy Search**: Quickly find sessions by typing partial names
- **Setup Commands**: Automatically run setup commands from `.cursor/worktrees.json`
- **Session Persistence**: tmux sessions survive ATC restarts — quit and relaunch without interrupting running agents
- **Text Selection**: Click and drag to select text, automatically copied to clipboard
- **Scrollback**: Mouse wheel scrolling through terminal history
- **Intuitive TUI**: Beautiful terminal interface built with Bubble Tea

## Installation

### Quick Install (macOS/Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/kevinzwang/air-traffic-control/main/scripts/install.sh | bash
```

This installs the latest release to `~/.local/bin/atc`. Options:

```bash
# Install specific version
curl -fsSL https://raw.githubusercontent.com/kevinzwang/air-traffic-control/main/scripts/install.sh | bash -s -- --version v1.0.0

# Install to custom directory
curl -fsSL https://raw.githubusercontent.com/kevinzwang/air-traffic-control/main/scripts/install.sh | bash -s -- --install-dir /usr/local/bin
```

### Build from Source

Prerequisites: Go 1.21+, Git

```bash
git clone https://github.com/kevinzwang/air-traffic-control.git
cd air-traffic-control
go build -o atc ./cmd/atc
mv atc ~/.local/bin/  # or /usr/local/bin/
```

### Prerequisites

- Git
- [tmux](https://github.com/tmux/tmux)
- Claude Code CLI (`claude`)

## Usage

Navigate to any git repository and run:

```bash
cd /path/to/your/repo
atc
```

## Configuration

### Setup Commands

ATC is compatible with the `.cursor/worktrees.json` format. Create this file in your repository root:

```json
{
  "setup-worktree": [
    "npm install",
    "npm run build",
    "cp .env.example .env"
  ]
}
```

These commands will run automatically when creating a new session.

### Database

ATC stores session metadata in `~/.atc/sessions.db` (SQLite).

### Worktrees

All worktrees are stored at `~/.atc/worktrees/<repo-name>/<session-name>`.

## Architecture

```
air-traffic-control/
├── cmd/atc/           # Main entry point
├── internal/
│   ├── config/        # Config file parsing
│   ├── database/      # SQLite operations
│   ├── terminal/      # tmux session wrapper per session
│   ├── worktree/      # Git worktree management
│   ├── session/       # Business logic
│   └── tui/           # Terminal UI (split-pane layout)
```

## How It Works

1. **Session Creation**:
   - Creates a git worktree in `~/.atc/worktrees/`
   - Creates a new branch based on the session name
   - Runs setup commands from `.cursor/worktrees.json`
   - Saves session metadata to SQLite

2. **Session Activation**:
   - Reattaches to an existing tmux session if one is still running from a previous ATC instance
   - Otherwise spawns `claude` (with `--continue` if a prior conversation exists) in a new tmux session
   - Terminal output is rendered via `tmux capture-pane` in the right pane
   - Keystrokes are forwarded via `tmux send-keys` for instant feedback
   - Use `Ctrl+C` to switch focus back to the session list

3. **Session Deletion**:
   - Kills the tmux session if running
   - Removes git worktree with `git worktree remove`
   - Deletes session record from database

## Troubleshooting

### "not a git repository"

Make sure you run `atc` from inside a git repository:

```bash
cd /path/to/your/git/repo
atc
```

### "Claude CLI not installed"

Install the Claude Code CLI first:

```bash
# Follow instructions at https://github.com/anthropics/claude-code
```

### Orphaned Worktrees

If you manually delete a worktree directory, you can clean up git's records:

```bash
git worktree prune
```

### Option+Key Shortcuts Not Working (macOS)

If shortcuts like Option+Delete (word deletion) or Option+Enter (newline) don't work inside ATC sessions, your terminal is likely not sending the Option key as an escape prefix.

**iTerm2**: Go to **Settings → Profiles → Keys** and set **Left Option key** to **Esc+** (instead of "Normal").

**Terminal.app**: Go to **Settings → Profiles → Keyboard** and check **Use Option as Meta key**.

### Database Issues

If you encounter database corruption, you can reset it:

```bash
rm ~/.atc/sessions.db
```

Note: This will delete all session records (but not the worktrees themselves).

## Development

### Run Tests

```bash
go test ./...
```

### Build

```bash
go build -o atc ./cmd/atc
```

### Dependencies

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Bubbles](https://github.com/charmbracelet/bubbles) - TUI components
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Styling
- [go-sqlite3](https://github.com/mattn/go-sqlite3) - SQLite driver
- [tmux](https://github.com/tmux/tmux) - Terminal multiplexer (runtime dependency)
- [uuid](https://github.com/google/uuid) - UUID generation

## License

MIT

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

## Credits

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) by Charm.
