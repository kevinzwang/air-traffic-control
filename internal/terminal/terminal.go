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
	name    string // tmux session name (unique per terminal)
	program *tea.Program
	done    chan struct{}
	mu      sync.Mutex
	closed  bool

	// Rendering
	lastCapture string // last captured pane content (for change detection)
	visHeight   int

	// Scrollback
	scrollLines    int // lines scrolled back from bottom (0 = live)
	cachedHistSize int // cached history_size from last poll

	// Exit detection
	paneDead bool
}

// newTerminal creates a Terminal struct and starts its poll loop.
func newTerminal(name string, width, height int, p *tea.Program, socket string) *Terminal {
	t := &Terminal{
		socket:    socket,
		name:      name,
		program:   p,
		done:      make(chan struct{}),
		visHeight: height,
	}
	go t.pollLoop()
	return t
}

// New creates a tmux session running claude in the given worktree directory.
// tmuxSocket is the shared socket name (e.g. "atc-<hash>").
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

	return newTerminal(name, width, height, p, tmuxSocket), nil
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
		"capture-pane", "-t", t.name, "-p", "-e").Output()
	return string(out)
}

func (t *Terminal) capturePaneRange(startLine, endLine int) string {
	out, _ := exec.Command("tmux", "-L", t.socket,
		"capture-pane", "-t", t.name, "-p", "-e",
		"-S", fmt.Sprintf("%d", startLine),
		"-E", fmt.Sprintf("%d", endLine)).Output()
	return string(out)
}

func (t *Terminal) isPaneDead() bool {
	out, _ := exec.Command("tmux", "-L", t.socket,
		"display-message", "-t", t.name, "-p", "#{pane_dead}").Output()
	return strings.TrimSpace(string(out)) == "1"
}

func (t *Terminal) historySize() int {
	out, _ := exec.Command("tmux", "-L", t.socket,
		"display-message", "-t", t.name, "-p", "#{history_size}").Output()
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
	base := []string{"-L", t.socket, "send-keys", "-t", t.name}

	// Alt+Runes: send ESC + rune as a single literal string so both bytes
	// arrive in one PTY write. If they're split across writes, the process
	// inside tmux may see a standalone Escape followed by the rune.
	if msg.Type == tea.KeyRunes && msg.Alt {
		return append(base, "-l", "\x1b"+string(msg.Runes))
	}

	// Regular runes (no Alt).
	if msg.Type == tea.KeyRunes {
		return append(base, "-l", string(msg.Runes))
	}

	// For Alt + single-byte keys (Enter, Backspace, Tab, Space, Escape,
	// Ctrl+letter), we must send ESC + the key's byte as a single literal
	// string via -l. tmux send-keys with separate args ("Escape" "Enter")
	// writes them in separate PTY writes, and the app inside tmux parses
	// the lone ESC as a standalone Escape key.
	if msg.Alt {
		if b := keyByte(msg.Type); b != 0 {
			return append(base, "-l", "\x1b"+string([]byte{b}))
		}
	}

	// Map key type to tmux key name.
	var tmuxKey string
	switch msg.Type {
	case tea.KeyEnter:
		tmuxKey = "Enter"
	case tea.KeyBackspace:
		tmuxKey = "BSpace"
	case tea.KeyTab:
		tmuxKey = "Tab"
	case tea.KeyShiftTab:
		tmuxKey = "BTab"
	case tea.KeyEscape:
		tmuxKey = "Escape"
	case tea.KeySpace:
		tmuxKey = "Space"

	// Arrow keys
	case tea.KeyUp:
		tmuxKey = "Up"
	case tea.KeyDown:
		tmuxKey = "Down"
	case tea.KeyRight:
		tmuxKey = "Right"
	case tea.KeyLeft:
		tmuxKey = "Left"

	// Shift+Arrow keys
	case tea.KeyShiftUp:
		tmuxKey = "S-Up"
	case tea.KeyShiftDown:
		tmuxKey = "S-Down"
	case tea.KeyShiftLeft:
		tmuxKey = "S-Left"
	case tea.KeyShiftRight:
		tmuxKey = "S-Right"

	// Ctrl+Arrow keys
	case tea.KeyCtrlUp:
		tmuxKey = "C-Up"
	case tea.KeyCtrlDown:
		tmuxKey = "C-Down"
	case tea.KeyCtrlLeft:
		tmuxKey = "C-Left"
	case tea.KeyCtrlRight:
		tmuxKey = "C-Right"

	// Ctrl+Shift+Arrow keys
	case tea.KeyCtrlShiftUp:
		tmuxKey = "C-S-Up"
	case tea.KeyCtrlShiftDown:
		tmuxKey = "C-S-Down"
	case tea.KeyCtrlShiftLeft:
		tmuxKey = "C-S-Left"
	case tea.KeyCtrlShiftRight:
		tmuxKey = "C-S-Right"

	// Navigation keys
	case tea.KeyHome:
		tmuxKey = "Home"
	case tea.KeyEnd:
		tmuxKey = "End"
	case tea.KeyShiftHome:
		tmuxKey = "S-Home"
	case tea.KeyShiftEnd:
		tmuxKey = "S-End"
	case tea.KeyCtrlHome:
		tmuxKey = "C-Home"
	case tea.KeyCtrlEnd:
		tmuxKey = "C-End"
	case tea.KeyCtrlShiftHome:
		tmuxKey = "C-S-Home"
	case tea.KeyCtrlShiftEnd:
		tmuxKey = "C-S-End"
	case tea.KeyInsert:
		tmuxKey = "IC"
	case tea.KeyDelete:
		tmuxKey = "DC"
	case tea.KeyPgUp:
		tmuxKey = "PPage"
	case tea.KeyPgDown:
		tmuxKey = "NPage"
	case tea.KeyCtrlPgUp:
		tmuxKey = "C-PPage"
	case tea.KeyCtrlPgDown:
		tmuxKey = "C-NPage"

	// Function keys
	case tea.KeyF1:
		tmuxKey = "F1"
	case tea.KeyF2:
		tmuxKey = "F2"
	case tea.KeyF3:
		tmuxKey = "F3"
	case tea.KeyF4:
		tmuxKey = "F4"
	case tea.KeyF5:
		tmuxKey = "F5"
	case tea.KeyF6:
		tmuxKey = "F6"
	case tea.KeyF7:
		tmuxKey = "F7"
	case tea.KeyF8:
		tmuxKey = "F8"
	case tea.KeyF9:
		tmuxKey = "F9"
	case tea.KeyF10:
		tmuxKey = "F10"
	case tea.KeyF11:
		tmuxKey = "F11"
	case tea.KeyF12:
		tmuxKey = "F12"
	case tea.KeyF13:
		tmuxKey = "F13"
	case tea.KeyF14:
		tmuxKey = "F14"
	case tea.KeyF15:
		tmuxKey = "F15"
	case tea.KeyF16:
		tmuxKey = "F16"
	case tea.KeyF17:
		tmuxKey = "F17"
	case tea.KeyF18:
		tmuxKey = "F18"
	case tea.KeyF19:
		tmuxKey = "F19"
	case tea.KeyF20:
		tmuxKey = "F20"

	// Ctrl+letter keys
	case tea.KeyCtrlA:
		tmuxKey = "C-a"
	case tea.KeyCtrlB:
		tmuxKey = "C-b"
	case tea.KeyCtrlC:
		tmuxKey = "C-c"
	case tea.KeyCtrlD:
		tmuxKey = "C-d"
	case tea.KeyCtrlE:
		tmuxKey = "C-e"
	case tea.KeyCtrlF:
		tmuxKey = "C-f"
	case tea.KeyCtrlG:
		tmuxKey = "C-g"
	case tea.KeyCtrlH:
		tmuxKey = "C-h"
	// KeyCtrlI = Tab, handled above
	case tea.KeyCtrlJ:
		tmuxKey = "C-j"
	case tea.KeyCtrlK:
		tmuxKey = "C-k"
	case tea.KeyCtrlL:
		tmuxKey = "C-l"
	// KeyCtrlM = Enter, handled above
	case tea.KeyCtrlN:
		tmuxKey = "C-n"
	case tea.KeyCtrlO:
		tmuxKey = "C-o"
	case tea.KeyCtrlP:
		tmuxKey = "C-p"
	case tea.KeyCtrlQ:
		tmuxKey = "C-q"
	case tea.KeyCtrlR:
		tmuxKey = "C-r"
	case tea.KeyCtrlS:
		tmuxKey = "C-s"
	case tea.KeyCtrlT:
		tmuxKey = "C-t"
	case tea.KeyCtrlU:
		tmuxKey = "C-u"
	case tea.KeyCtrlV:
		tmuxKey = "C-v"
	case tea.KeyCtrlW:
		tmuxKey = "C-w"
	case tea.KeyCtrlX:
		tmuxKey = "C-x"
	case tea.KeyCtrlY:
		tmuxKey = "C-y"
	case tea.KeyCtrlZ:
		tmuxKey = "C-z"
	}

	if tmuxKey == "" {
		return nil
	}

	// For Alt + multi-byte named keys (arrows, function keys, etc.), send
	// ESC + the raw escape sequence as a single literal via -l so both
	// arrive in one PTY write. Sending them as separate tmux args causes
	// two writes, making the shell see a standalone Escape + a plain key.
	if msg.Alt {
		if seq := keySequence(msg.Type); seq != "" {
			return append(base, "-l", "\x1b"+seq)
		}
	}

	return append(base, tmuxKey)
}

// keyByte returns the raw byte for single-byte key types, or 0 if the key
// type corresponds to a multi-byte escape sequence (arrows, function keys, etc.).
func keyByte(kt tea.KeyType) byte {
	switch kt {
	case tea.KeyEnter:
		return '\r'
	case tea.KeyTab:
		return '\t'
	case tea.KeyBackspace:
		return 0x7f
	case tea.KeyEscape:
		return 0x1b
	case tea.KeySpace:
		return ' '
	// Ctrl+A through Ctrl+Z are bytes 1-26.
	case tea.KeyCtrlA:
		return 1
	case tea.KeyCtrlB:
		return 2
	case tea.KeyCtrlC:
		return 3
	case tea.KeyCtrlD:
		return 4
	case tea.KeyCtrlE:
		return 5
	case tea.KeyCtrlF:
		return 6
	case tea.KeyCtrlG:
		return 7
	case tea.KeyCtrlH:
		return 8
	// KeyCtrlI = Tab (9), handled above
	case tea.KeyCtrlJ:
		return 10
	case tea.KeyCtrlK:
		return 11
	case tea.KeyCtrlL:
		return 12
	// KeyCtrlM = Enter (13), handled above
	case tea.KeyCtrlN:
		return 14
	case tea.KeyCtrlO:
		return 15
	case tea.KeyCtrlP:
		return 16
	case tea.KeyCtrlQ:
		return 17
	case tea.KeyCtrlR:
		return 18
	case tea.KeyCtrlS:
		return 19
	case tea.KeyCtrlT:
		return 20
	case tea.KeyCtrlU:
		return 21
	case tea.KeyCtrlV:
		return 22
	case tea.KeyCtrlW:
		return 23
	case tea.KeyCtrlX:
		return 24
	case tea.KeyCtrlY:
		return 25
	case tea.KeyCtrlZ:
		return 26
	}
	return 0
}

// keySequence returns the raw terminal escape sequence for multi-byte key
// types (arrows, navigation, function keys), or "" if unknown. These match
// the sequences in Bubble Tea's key.go sequences map.
func keySequence(kt tea.KeyType) string {
	switch kt {
	// Arrow keys
	case tea.KeyUp:
		return "\x1b[A"
	case tea.KeyDown:
		return "\x1b[B"
	case tea.KeyRight:
		return "\x1b[C"
	case tea.KeyLeft:
		return "\x1b[D"

	// Shift+Arrow keys
	case tea.KeyShiftUp:
		return "\x1b[1;2A"
	case tea.KeyShiftDown:
		return "\x1b[1;2B"
	case tea.KeyShiftRight:
		return "\x1b[1;2C"
	case tea.KeyShiftLeft:
		return "\x1b[1;2D"

	// Ctrl+Arrow keys
	case tea.KeyCtrlUp:
		return "\x1b[1;5A"
	case tea.KeyCtrlDown:
		return "\x1b[1;5B"
	case tea.KeyCtrlRight:
		return "\x1b[1;5C"
	case tea.KeyCtrlLeft:
		return "\x1b[1;5D"

	// Ctrl+Shift+Arrow keys
	case tea.KeyCtrlShiftUp:
		return "\x1b[1;6A"
	case tea.KeyCtrlShiftDown:
		return "\x1b[1;6B"
	case tea.KeyCtrlShiftRight:
		return "\x1b[1;6C"
	case tea.KeyCtrlShiftLeft:
		return "\x1b[1;6D"

	// Navigation keys
	case tea.KeyHome:
		return "\x1b[H"
	case tea.KeyEnd:
		return "\x1b[F"
	case tea.KeyShiftHome:
		return "\x1b[1;2H"
	case tea.KeyShiftEnd:
		return "\x1b[1;2F"
	case tea.KeyCtrlHome:
		return "\x1b[1;5H"
	case tea.KeyCtrlEnd:
		return "\x1b[1;5F"
	case tea.KeyCtrlShiftHome:
		return "\x1b[1;6H"
	case tea.KeyCtrlShiftEnd:
		return "\x1b[1;6F"
	case tea.KeyInsert:
		return "\x1b[2~"
	case tea.KeyDelete:
		return "\x1b[3~"
	case tea.KeyPgUp:
		return "\x1b[5~"
	case tea.KeyPgDown:
		return "\x1b[6~"
	case tea.KeyCtrlPgUp:
		return "\x1b[5;5~"
	case tea.KeyCtrlPgDown:
		return "\x1b[6;5~"

	// Function keys
	case tea.KeyF1:
		return "\x1bOP"
	case tea.KeyF2:
		return "\x1bOQ"
	case tea.KeyF3:
		return "\x1bOR"
	case tea.KeyF4:
		return "\x1bOS"
	case tea.KeyF5:
		return "\x1b[15~"
	case tea.KeyF6:
		return "\x1b[17~"
	case tea.KeyF7:
		return "\x1b[18~"
	case tea.KeyF8:
		return "\x1b[19~"
	case tea.KeyF9:
		return "\x1b[20~"
	case tea.KeyF10:
		return "\x1b[21~"
	case tea.KeyF11:
		return "\x1b[23~"
	case tea.KeyF12:
		return "\x1b[24~"
	case tea.KeyF13:
		return "\x1b[25~"
	case tea.KeyF14:
		return "\x1b[26~"
	case tea.KeyF15:
		return "\x1b[28~"
	case tea.KeyF16:
		return "\x1b[29~"
	case tea.KeyF17:
		return "\x1b[31~"
	case tea.KeyF18:
		return "\x1b[32~"
	case tea.KeyF19:
		return "\x1b[33~"
	case tea.KeyF20:
		return "\x1b[34~"
	}
	return ""
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
	t.visHeight = height
	t.mu.Unlock()

	exec.Command("tmux", "-L", t.socket,
		"resize-window", "-t", t.name,
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
		"respawn-pane", "-t", t.name, "-k", cmd).Run()
	if err != nil {
		return err
	}
	t.mu.Lock()
	t.paneDead = false
	t.mu.Unlock()
	return nil
}

// stopPollLoop stops the poll goroutine. Must be called with t.mu held.
// Returns false if already stopped.
func (t *Terminal) stopPollLoop() bool {
	if t.closed {
		return false
	}
	t.closed = true
	close(t.done)
	return true
}

// Close kills the tmux session and stops the poll loop.
func (t *Terminal) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.stopPollLoop() {
		return nil
	}
	exec.Command("tmux", "-L", t.socket, "kill-session", "-t", t.name).Run()
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
func Attach(name string, width, height int, p *tea.Program, tmuxSocket string) (*Terminal, error) {
	// Resize to match current terminal pane
	exec.Command("tmux", "-L", tmuxSocket,
		"resize-window", "-t", name,
		"-x", fmt.Sprintf("%d", width),
		"-y", fmt.Sprintf("%d", height)).Run()

	t := newTerminal(name, width, height, p, tmuxSocket)

	// Check if the pane process has already exited
	if t.isPaneDead() {
		t.paneDead = true
	}

	return t, nil
}

// Detach stops the poll loop but does NOT kill the tmux session.
// The tmux session continues running in the background.
func (t *Terminal) Detach() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopPollLoop()
}
