package terminal

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TerminalOutputMsg is sent when new output is available from the terminal.
type TerminalOutputMsg struct{}

// TerminalExitedMsg is sent when the child process exits.
type TerminalExitedMsg struct {
	Name string
}

// Terminal wraps a tmux session for a single Claude session.
type Terminal struct {
	socket  string // tmux socket name (shared across all terminals)
	session string // tmux session name (unique per terminal)
	program *tea.Program
	name    string
	done    chan struct{}
	mu      sync.Mutex
	closed  bool

	// Rendering
	lastCapture string // last captured pane content (for change detection)
	visWidth    int
	visHeight   int

	// Scrollback
	scrollLines    int // lines scrolled back from bottom (0 = live)
	cachedHistSize int // cached history_size from last poll

	// Exit detection
	paneDead bool
}

// New creates a tmux session running claude in the given worktree directory.
// tmuxSocket is the shared socket name (e.g. "atc-<pid>").
func New(name, worktreePath string, width, height int, continueSession bool, p *tea.Program, tmuxSocket string) (*Terminal, error) {
	cmd := "claude"
	if continueSession {
		cmd = "claude --continue"
	}

	args := []string{"-L", tmuxSocket, "new-session", "-d",
		"-s", name,
		"-x", fmt.Sprintf("%d", width),
		"-y", fmt.Sprintf("%d", height),
		"-E", // don't apply update-environment
		cmd}
	createCmd := exec.Command("tmux", args...)
	createCmd.Dir = worktreePath
	createCmd.Env = append(os.Environ(), "TERM=xterm-256color")
	if out, err := createCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to create tmux session: %w: %s", err, string(out))
	}

	// Configure: keep pane alive after process exits, set scrollback
	exec.Command("tmux", "-L", tmuxSocket, "set-option", "-t", name, "remain-on-exit", "on").Run()
	exec.Command("tmux", "-L", tmuxSocket, "set-option", "-t", name, "history-limit", "50000").Run()

	t := &Terminal{
		socket:    tmuxSocket,
		session:   name,
		program:   p,
		name:      name,
		done:      make(chan struct{}),
		visWidth:  width,
		visHeight: height,
	}

	go t.pollLoop()
	return t, nil
}

// pollLoop captures pane content periodically and sends Bubble Tea messages on change.
func (t *Terminal) pollLoop() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-t.done:
			return
		case <-ticker.C:
			output := t.capturePaneVisible()
			histSize := t.historySize()

			t.mu.Lock()
			changed := output != t.lastCapture
			t.lastCapture = output
			t.cachedHistSize = histSize
			t.mu.Unlock()

			if changed && t.program != nil {
				t.program.Send(TerminalOutputMsg{})
			}

			// Check if process exited
			if t.isPaneDead() {
				t.mu.Lock()
				wasDead := t.paneDead
				t.paneDead = true
				t.mu.Unlock()

				if !wasDead && t.program != nil {
					t.program.Send(TerminalExitedMsg{Name: t.name})
				}
				// Slow down polling since nothing is changing
				ticker.Reset(500 * time.Millisecond)
			}
		}
	}
}

func (t *Terminal) capturePaneVisible() string {
	out, _ := exec.Command("tmux", "-L", t.socket,
		"capture-pane", "-t", t.session, "-p", "-e").Output()
	return string(out)
}

func (t *Terminal) capturePaneRange(startLine, endLine int) string {
	out, _ := exec.Command("tmux", "-L", t.socket,
		"capture-pane", "-t", t.session, "-p", "-e",
		"-S", fmt.Sprintf("%d", startLine),
		"-E", fmt.Sprintf("%d", endLine)).Output()
	return string(out)
}

func (t *Terminal) isPaneDead() bool {
	out, _ := exec.Command("tmux", "-L", t.socket,
		"display-message", "-t", t.session, "-p", "#{pane_dead}").Output()
	return strings.TrimSpace(string(out)) == "1"
}

func (t *Terminal) historySize() int {
	out, _ := exec.Command("tmux", "-L", t.socket,
		"display-message", "-t", t.session, "-p", "#{history_size}").Output()
	n := 0
	fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &n)
	return n
}

// SendKeys translates a Bubble Tea KeyMsg and sends it to the tmux session.
func (t *Terminal) SendKeys(msg tea.KeyMsg) {
	args := t.keyMsgToTmuxArgs(msg)
	if args == nil {
		return
	}
	exec.Command("tmux", args...).Run()
}

func (t *Terminal) keyMsgToTmuxArgs(msg tea.KeyMsg) []string {
	base := []string{"-L", t.socket, "send-keys", "-t", t.session}

	switch msg.Type {
	case tea.KeyRunes:
		return append(base, "-l", string(msg.Runes))
	case tea.KeyEnter:
		return append(base, "Enter")
	case tea.KeyBackspace:
		return append(base, "BSpace")
	case tea.KeyTab:
		return append(base, "Tab")
	case tea.KeyEscape:
		return append(base, "Escape")
	case tea.KeyUp:
		return append(base, "Up")
	case tea.KeyDown:
		return append(base, "Down")
	case tea.KeyRight:
		return append(base, "Right")
	case tea.KeyLeft:
		return append(base, "Left")
	case tea.KeySpace:
		return append(base, "Space")
	case tea.KeyHome:
		return append(base, "Home")
	case tea.KeyEnd:
		return append(base, "End")
	case tea.KeyDelete:
		return append(base, "DC")
	case tea.KeyPgUp:
		return append(base, "PPage")
	case tea.KeyPgDown:
		return append(base, "NPage")
	case tea.KeyCtrlA:
		return append(base, "C-a")
	case tea.KeyCtrlB:
		return append(base, "C-b")
	case tea.KeyCtrlC:
		return append(base, "C-c")
	case tea.KeyCtrlD:
		return append(base, "C-d")
	case tea.KeyCtrlE:
		return append(base, "C-e")
	case tea.KeyCtrlF:
		return append(base, "C-f")
	case tea.KeyCtrlG:
		return append(base, "C-g")
	case tea.KeyCtrlH:
		return append(base, "C-h")
	// KeyCtrlI = Tab, handled above
	case tea.KeyCtrlJ:
		return append(base, "C-j")
	case tea.KeyCtrlK:
		return append(base, "C-k")
	case tea.KeyCtrlL:
		return append(base, "C-l")
	// KeyCtrlM = Enter, handled above
	case tea.KeyCtrlN:
		return append(base, "C-n")
	case tea.KeyCtrlO:
		return append(base, "C-o")
	case tea.KeyCtrlP:
		return append(base, "C-p")
	case tea.KeyCtrlQ:
		return append(base, "C-q")
	case tea.KeyCtrlR:
		return append(base, "C-r")
	case tea.KeyCtrlS:
		return append(base, "C-s")
	// KeyCtrlC is intercepted by the TUI for focus switching
	case tea.KeyCtrlT:
		return append(base, "C-t")
	case tea.KeyCtrlU:
		return append(base, "C-u")
	case tea.KeyCtrlV:
		return append(base, "C-v")
	case tea.KeyCtrlW:
		return append(base, "C-w")
	case tea.KeyCtrlX:
		return append(base, "C-x")
	case tea.KeyCtrlY:
		return append(base, "C-y")
	case tea.KeyCtrlZ:
		return append(base, "C-z")
	}
	return nil
}

// Render returns the current terminal content as an ANSI string.
func (t *Terminal) Render() string {
	t.mu.Lock()
	scrollLines := t.scrollLines
	lastCapture := t.lastCapture
	t.mu.Unlock()

	if scrollLines == 0 {
		return strings.TrimRight(lastCapture, "\n")
	}

	// Scroll mode — capture a specific range from scrollback.
	// In tmux, line 0 is the top of the visible pane, negative lines are scrollback.
	// scrollLines=3 means "shift the viewport up by 3": show lines -3 to h-4.
	h := t.visHeight
	startLine := -scrollLines
	endLine := -scrollLines + h - 1
	captured := t.capturePaneRange(startLine, endLine)
	return strings.TrimRight(captured, "\n")
}

// Resize updates the tmux session dimensions.
func (t *Terminal) Resize(width, height int) {
	t.mu.Lock()
	t.visWidth = width
	t.visHeight = height
	t.mu.Unlock()

	exec.Command("tmux", "-L", t.socket,
		"resize-window", "-t", t.session,
		"-x", fmt.Sprintf("%d", width),
		"-y", fmt.Sprintf("%d", height)).Run()
}

// IsRunning returns true if the child process is still alive.
func (t *Terminal) IsRunning() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return !t.paneDead
}

// Respawn restarts the claude process in the tmux pane.
func (t *Terminal) Respawn(continueSession bool) error {
	cmd := "claude"
	if continueSession {
		cmd = "claude --continue"
	}
	err := exec.Command("tmux", "-L", t.socket,
		"respawn-pane", "-t", t.session, "-k", cmd).Run()
	if err != nil {
		return err
	}
	t.mu.Lock()
	t.paneDead = false
	t.mu.Unlock()
	return nil
}

// Close kills the tmux session and stops the poll loop.
func (t *Terminal) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	close(t.done)
	exec.Command("tmux", "-L", t.socket, "kill-session", "-t", t.session).Run()
	return nil
}

// ScrollUp scrolls back by the given number of lines.
func (t *Terminal) ScrollUp(lines int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.scrollLines += lines
	if t.scrollLines > t.cachedHistSize {
		t.scrollLines = t.cachedHistSize
	}
}

// ScrollDown scrolls forward by the given number of lines.
func (t *Terminal) ScrollDown(lines int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.scrollLines -= lines
	if t.scrollLines < 0 {
		t.scrollLines = 0
	}
}

// IsScrollMode returns true if the terminal is in scroll mode.
func (t *Terminal) IsScrollMode() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.scrollLines > 0
}

// ScrollPosition returns the current scroll offset in lines.
func (t *Terminal) ScrollPosition() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.scrollLines
}

// ExitScrollMode returns to live view.
func (t *Terminal) ExitScrollMode() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.scrollLines = 0
}

// SessionExists checks whether a tmux session with the given name exists on the socket.
func SessionExists(socket, name string) bool {
	err := exec.Command("tmux", "-L", socket, "has-session", "-t", name).Run()
	return err == nil
}

// Attach wraps an existing tmux session, resizes it, and starts polling for output.
func Attach(name, worktreePath string, width, height int, p *tea.Program, tmuxSocket string) (*Terminal, error) {
	// Resize to match current terminal pane
	exec.Command("tmux", "-L", tmuxSocket,
		"resize-window", "-t", name,
		"-x", fmt.Sprintf("%d", width),
		"-y", fmt.Sprintf("%d", height)).Run()

	t := &Terminal{
		socket:    tmuxSocket,
		session:   name,
		program:   p,
		name:      name,
		done:      make(chan struct{}),
		visWidth:  width,
		visHeight: height,
	}

	// Check if the pane process has already exited
	if t.isPaneDead() {
		t.paneDead = true
	}

	go t.pollLoop()
	return t, nil
}

// Detach stops the poll loop but does NOT kill the tmux session.
// The tmux session continues running in the background.
func (t *Terminal) Detach() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return
	}
	t.closed = true
	close(t.done)
}
