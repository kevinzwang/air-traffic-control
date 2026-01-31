package tui

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/kevinwang/air-traffic-control/internal/session"
)

type state int

const (
	stateMain state = iota
	stateCreating
	stateConfirmDelete
)

type Model struct {
	state          state
	service        *session.Service
	repoName       string
	sessions       []*session.Session
	filteredSessions []*session.Session
	textInput      textinput.Model
	cursor         int
	spinner        spinner.Model
	err            error
	message        string
	selectedSession *session.Session
	createOutput   string
	commandToExec  string
}

type sessionsLoadedMsg struct {
	sessions []*session.Session
}

type sessionCreatedMsg struct {
	session *session.Session
	output  string
}

type sessionDeletedMsg struct {
	name string
}

type sessionArchivedMsg struct {
	name string
}

type sessionUnarchivedMsg struct {
	name string
}

type errMsg struct {
	err error
}

func NewModel(service *session.Service, repoName string) Model {
	ti := textinput.New()
	ti.Placeholder = "Type to search or create..."
	ti.Focus()
	ti.CharLimit = 100
	ti.Width = 50

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		state:     stateMain,
		service:   service,
		repoName:  repoName,
		textInput: ti,
		spinner:   s,
		cursor:    0,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.loadSessions(),
		m.spinner.Tick,
	)
}

func (m Model) loadSessions() tea.Cmd {
	return func() tea.Msg {
		sessions, err := m.service.ListSessions("")
		if err != nil {
			return errMsg{err}
		}
		return sessionsLoadedMsg{sessions}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case sessionsLoadedMsg:
		m.sessions = msg.sessions
		m.filterSessions()
		return m, nil

	case sessionCreatedMsg:
		m.state = stateMain
		m.message = fmt.Sprintf("✓ Session '%s' created successfully", msg.session.Name)
		m.textInput.SetValue("")
		return m, m.loadSessions()

	case sessionDeletedMsg:
		m.state = stateMain
		m.message = fmt.Sprintf("✓ Session '%s' deleted", msg.name)
		m.selectedSession = nil
		return m, m.loadSessions()

	case sessionArchivedMsg:
		m.message = fmt.Sprintf("✓ Session '%s' archived", msg.name)
		return m, m.loadSessions()

	case sessionUnarchivedMsg:
		m.message = fmt.Sprintf("✓ Session '%s' unarchived", msg.name)
		return m, m.loadSessions()

	case errMsg:
		m.err = msg.err
		m.state = stateMain
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	m.filterSessions()

	return m, cmd
}

func (m *Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case stateMain:
		return m.handleMainKeys(msg)
	case stateConfirmDelete:
		return m.handleDeleteConfirmKeys(msg)
	case stateCreating:
		// No input during creation
		return m, nil
	}
	return m, nil
}

func (m *Model) handleMainKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case "down", "j":
		maxCursor := len(m.filteredSessions) // +1 for "Create new" option
		if m.cursor < maxCursor {
			m.cursor++
		}
		return m, nil

	case "enter":
		return m.handleEnter()

	case "ctrl+d":
		return m.handleDelete()

	case "ctrl+a":
		return m.handleArchiveToggle()

	default:
		// Clear message on any key press
		m.message = ""
		m.err = nil
		return m, nil
	}
}

func (m *Model) handleEnter() (tea.Model, tea.Cmd) {
	// First option is always "Create new"
	if m.cursor == 0 {
		return m.createSession()
	}

	// Enter existing session
	sessionIdx := m.cursor - 1
	if sessionIdx >= 0 && sessionIdx < len(m.filteredSessions) {
		selectedSession := m.filteredSessions[sessionIdx]
		cmd, err := m.service.EnterSession(selectedSession.Name)
		if err != nil {
			m.err = err
			return m, nil
		}

		// Store command and quit to exec it
		m.commandToExec = cmd
		return m, tea.Quit
	}

	return m, nil
}

func (m *Model) createSession() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(m.textInput.Value())
	if name == "" {
		m.err = fmt.Errorf("session name cannot be empty")
		return m, nil
	}

	m.state = stateCreating
	m.createOutput = ""

	return m, func() tea.Msg {
		var buf bytes.Buffer
		session, err := m.service.CreateSession(name, &buf)
		if err != nil {
			return errMsg{err}
		}
		return sessionCreatedMsg{
			session: session,
			output:  buf.String(),
		}
	}
}

func (m *Model) handleDelete() (tea.Model, tea.Cmd) {
	// Can't delete "Create new" option
	if m.cursor == 0 {
		return m, nil
	}

	sessionIdx := m.cursor - 1
	if sessionIdx >= 0 && sessionIdx < len(m.filteredSessions) {
		m.selectedSession = m.filteredSessions[sessionIdx]
		m.state = stateConfirmDelete
	}

	return m, nil
}

func (m *Model) handleArchiveToggle() (tea.Model, tea.Cmd) {
	// Can't archive "Create new" option
	if m.cursor == 0 {
		return m, nil
	}

	sessionIdx := m.cursor - 1
	if sessionIdx >= 0 && sessionIdx < len(m.filteredSessions) {
		selectedSession := m.filteredSessions[sessionIdx]

		if selectedSession.Status == "archived" {
			return m, func() tea.Msg {
				err := m.service.UnarchiveSession(selectedSession.Name)
				if err != nil {
					return errMsg{err}
				}
				return sessionUnarchivedMsg{selectedSession.Name}
			}
		} else {
			return m, func() tea.Msg {
				err := m.service.ArchiveSession(selectedSession.Name)
				if err != nil {
					return errMsg{err}
				}
				return sessionArchivedMsg{selectedSession.Name}
			}
		}
	}

	return m, nil
}

func (m *Model) handleDeleteConfirmKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		name := m.selectedSession.Name
		return m, func() tea.Msg {
			err := m.service.DeleteSession(name)
			if err != nil {
				return errMsg{err}
			}
			return sessionDeletedMsg{name}
		}

	case "n", "N", "esc":
		m.state = stateMain
		m.selectedSession = nil
		return m, nil
	}

	return m, nil
}

func (m *Model) filterSessions() {
	query := strings.ToLower(strings.TrimSpace(m.textInput.Value()))

	if query == "" {
		m.filteredSessions = m.sessions
	} else {
		filtered := []*session.Session{}
		for _, s := range m.sessions {
			if strings.Contains(strings.ToLower(s.Name), query) {
				filtered = append(filtered, s)
			}
		}
		m.filteredSessions = filtered
	}

	// Reset cursor if out of bounds
	maxCursor := len(m.filteredSessions)
	if m.cursor > maxCursor {
		m.cursor = maxCursor
	}
}

func (m Model) View() string {
	switch m.state {
	case stateMain:
		return m.viewMain()
	case stateCreating:
		return m.viewCreating()
	case stateConfirmDelete:
		return m.viewConfirmDelete()
	}
	return ""
}

func (m Model) viewMain() string {
	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render("Air Traffic Control (ATC)"))
	b.WriteString("\n")
	b.WriteString(statusStyle.Render(fmt.Sprintf("Repo: %s", m.repoName)))
	b.WriteString("\n\n")

	// Input
	b.WriteString(inputStyle.Render("> ") + m.textInput.View())
	b.WriteString("\n\n")

	// Error/Message
	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("✗ Error: %s", m.err.Error())))
		b.WriteString("\n\n")
	} else if m.message != "" {
		b.WriteString(successStyle.Render(m.message))
		b.WriteString("\n\n")
	}

	// "Create new" option
	createNewText := fmt.Sprintf("✨ Create new session \"%s\"", m.textInput.Value())
	if m.cursor == 0 {
		b.WriteString(selectedItemStyle.Render("→ " + createNewText))
	} else {
		b.WriteString(createNewStyle.Render("  " + createNewText))
	}
	b.WriteString("\n\n")

	// Session list
	activeSessions := []*session.Session{}
	archivedSessions := []*session.Session{}

	for _, s := range m.filteredSessions {
		if s.Status == "archived" {
			archivedSessions = append(archivedSessions, s)
		} else {
			activeSessions = append(activeSessions, s)
		}
	}

	// Render active sessions
	for i, s := range activeSessions {
		cursorPos := i + 1 // +1 for "Create new" option
		m.renderSession(&b, s, cursorPos)
	}

	// Separator if there are archived sessions
	if len(archivedSessions) > 0 {
		b.WriteString("\n")
		b.WriteString(separatorStyle.Render("─────────────── Archived ───────────────"))
		b.WriteString("\n")

		// Render archived sessions
		for i, s := range archivedSessions {
			cursorPos := len(activeSessions) + i + 1
			m.renderSession(&b, s, cursorPos)
		}
	}

	// Help text
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("[↑/↓] Navigate  [Enter] Open/Create  [Ctrl+D] Delete  [Ctrl+A] Archive  [Esc] Quit"))

	return b.String()
}

func (m Model) renderSession(b *strings.Builder, s *session.Session, cursorPos int) {
	cursor := "  "
	style := normalItemStyle
	if m.cursor == cursorPos {
		cursor = "→ "
		style = selectedItemStyle
	}

	if s.Status == "archived" {
		style = archivedItemStyle
	}

	sessionLine := fmt.Sprintf("%s%-30s %s", cursor, s.Name, s.BranchName)
	b.WriteString(style.Render(sessionLine))
	b.WriteString("\n")

	// Metadata
	timeStr := formatTime(s.CreatedAt)
	if s.ArchivedAt != nil {
		timeStr = "Archived " + formatTime(*s.ArchivedAt)
	}
	b.WriteString(metadataStyle.Render("└─ " + timeStr))
	b.WriteString("\n")
}

func (m Model) viewCreating() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(fmt.Sprintf("Creating session \"%s\"...", m.textInput.Value())))
	b.WriteString("\n\n")

	b.WriteString(statusStyle.Render(fmt.Sprintf("%s Please wait...", m.spinner.View())))
	b.WriteString("\n\n")

	if m.createOutput != "" {
		b.WriteString(dialogTextStyle.Render(m.createOutput))
	}

	return b.String()
}

func (m Model) viewConfirmDelete() string {
	if m.selectedSession == nil {
		return ""
	}

	var b strings.Builder

	b.WriteString(dialogTitleStyle.Render("Confirm Deletion"))
	b.WriteString("\n\n")

	b.WriteString(dialogTextStyle.Render(fmt.Sprintf("Delete session \"%s\"?", m.selectedSession.Name)))
	b.WriteString("\n\n")

	b.WriteString(dialogTextStyle.Render("This will:"))
	b.WriteString("\n")
	b.WriteString(dialogTextStyle.Render("- Remove the git worktree"))
	b.WriteString("\n")
	b.WriteString(dialogTextStyle.Render("- Delete all local changes in the worktree"))
	b.WriteString("\n")
	b.WriteString(dialogTextStyle.Render("- Remove the session from ATC"))
	b.WriteString("\n\n")

	b.WriteString(warningStyle.Render("This action cannot be undone."))
	b.WriteString("\n\n")

	b.WriteString(dialogTextStyle.Render("[Y] Yes, delete    [N] Cancel"))

	return dialogBoxStyle.Render(b.String())
}

func formatTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	if diff < time.Minute {
		return "just now"
	} else if diff < time.Hour {
		minutes := int(diff.Minutes())
		if minutes == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", minutes)
	} else if diff < 24*time.Hour {
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	} else if diff < 7*24*time.Hour {
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	} else if diff < 30*24*time.Hour {
		weeks := int(diff.Hours() / 24 / 7)
		if weeks == 1 {
			return "1 week ago"
		}
		return fmt.Sprintf("%d weeks ago", weeks)
	} else {
		months := int(diff.Hours() / 24 / 30)
		if months == 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	}
}

// GetCommandToExec returns the command that should be executed after TUI exits
func (m Model) GetCommandToExec() string {
	return m.commandToExec
}
