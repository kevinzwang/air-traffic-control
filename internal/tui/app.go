package tui

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kevinzwang/air-traffic-control/internal/session"
	"github.com/kevinzwang/air-traffic-control/internal/worktree"
)

type state int

const (
	stateMain state = iota
	stateCreating
	stateConfirmDelete
	stateSelectBaseBranch
	stateSelectExistingBranch
	stateConfirmBranchWithSession
	stateEnterNewSessionName
)

// Layout constants
const (
	appVersion         = "0.1.0"
	contentWidth       = 62 // Inner width (total 64 - 2 for borders)
	fixedUILines       = 12 // Lines: 2 borders + repo + input + message + 2 dividers + create new + 2 help rows
	linesPerSession    = 2  // Each session displays name + metadata
	maxSessionsVisible = 15 // Cap to prevent overly tall box
	summaryMaxLen      = 35 // Max characters for conversation summary
	summaryTruncateAt  = 32 // Where to truncate before adding "..."
)

type Model struct {
	state            state
	service          *session.Service
	repoName         string
	sessions         []*session.Session
	filteredSessions []*session.Session
	textInput        textinput.Model
	cursor           int
	scrollOffset     int
	spinner          spinner.Model
	err              error
	message          string
	selectedSession  *session.Session
	createOutput     string
	commandToExec    string
	windowHeight     int
	windowWidth      int

	// Branch selection fields
	branches             []string          // Available branches
	filteredBranches     []string          // Branches filtered by search
	branchInput          textinput.Model   // Text input for branch filtering
	branchCursor         int               // Cursor for branch selection
	branchScrollOffset   int               // Scroll offset for branch list
	pendingSessionName   string            // Session name entered before base branch selection
	branchesWithSessions map[string]bool   // Tracks which branches already have sessions
	currentBranch        string            // Current HEAD branch name
	selectedBranchName   string            // Branch selected that has existing session (for confirmation dialog)
	newSessionInput      textinput.Model   // Text input for new session name when branching from existing
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

type branchesLoadedMsg struct {
	branches             []string
	branchesWithSessions map[string]bool
}

func NewModel(service *session.Service, repoName string, invokingBranch string) *Model {
	ti := textinput.New()
	ti.Placeholder = "Type to search or create..."
	ti.Focus()
	ti.CharLimit = 100
	ti.Width = 50

	s := spinner.New()
	s.Spinner = spinner.Dot

	return &Model{
		state:         stateMain,
		service:       service,
		repoName:      repoName,
		textInput:     ti,
		spinner:       s,
		cursor:        0,
		currentBranch: invokingBranch, // Use the branch from the invoking directory
	}
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.loadSessions(),
		m.spinner.Tick,
	)
}

func (m *Model) loadSessions() tea.Cmd {
	return func() tea.Msg {
		sessions, err := m.service.ListSessions("")
		if err != nil {
			return errMsg{err}
		}
		return sessionsLoadedMsg{sessions}
	}
}

func (m *Model) loadBranches() tea.Cmd {
	return func() tea.Msg {
		branches, err := m.service.ListBranches()
		if err != nil {
			return errMsg{err}
		}

		// Check which branches already have sessions
		branchesWithSessions := make(map[string]bool)
		for _, branch := range branches {
			sess, _ := m.service.GetSessionByBranch(branch)
			if sess != nil {
				branchesWithSessions[branch] = true
			}
		}

		return branchesLoadedMsg{
			branches:             branches,
			branchesWithSessions: branchesWithSessions,
		}
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.windowHeight = msg.Height
		m.windowWidth = msg.Width
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case sessionsLoadedMsg:
		m.sessions = msg.sessions
		m.filterSessions()
		return m, nil

	case branchesLoadedMsg:
		m.branches = msg.branches
		m.branchesWithSessions = msg.branchesWithSessions
		m.filterBranches()
		return m, nil

	case sessionCreatedMsg:
		// Enter the newly created session immediately
		m.commandToExec = worktree.GetClaudeCommand(msg.session.WorktreePath)
		return m, tea.Quit

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
	case stateSelectBaseBranch:
		return m.handleSelectBaseBranchKeys(msg)
	case stateSelectExistingBranch:
		return m.handleSelectExistingBranchKeys(msg)
	case stateConfirmBranchWithSession:
		return m.handleConfirmBranchWithSessionKeys(msg)
	case stateEnterNewSessionName:
		return m.handleEnterNewSessionNameKeys(msg)
	}
	return m, nil
}

func (m *Model) handleMainKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit

	case "up":
		if m.cursor > 0 {
			m.cursor--
			m.adjustScroll()
		}
		return m, nil

	case "down":
		maxCursor := len(m.filteredSessions) // +1 for "Create new" option
		if m.cursor < maxCursor {
			m.cursor++
			m.adjustScroll()
		}
		return m, nil

	case "enter":
		return m.handleEnter()

	case "ctrl+d":
		return m.handleDelete()

	case "ctrl+a":
		return m.handleArchiveToggle()

	case "ctrl+b":
		return m.handleCreateFromExistingBranch()

	default:
		// Pass key to text input for typing
		m.message = ""
		m.err = nil
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		m.filterSessions()
		return m, cmd
	}
}

func (m *Model) handleEnter() (tea.Model, tea.Cmd) {
	// First option is always "Create new"
	if m.cursor == 0 {
		return m.createSession()
	}

	// Enter existing session
	selected := m.getSelectedSession()
	if selected == nil {
		return m, nil
	}

	cmd, err := m.service.EnterSession(selected.Name)
	if err != nil {
		m.err = err
		return m, nil
	}

	m.commandToExec = cmd
	return m, tea.Quit
}

func (m *Model) createSession() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(m.textInput.Value())
	if name == "" {
		m.err = fmt.Errorf("session name cannot be empty")
		return m, nil
	}

	// Validate name before showing branch selection
	if err := worktree.ValidateBranchName(name); err != nil {
		m.err = fmt.Errorf("invalid session name: %w", err)
		return m, nil
	}

	// Store the pending session name and transition to branch selection
	m.pendingSessionName = name
	m.state = stateSelectBaseBranch
	m.initBranchInput()

	return m, m.loadBranches()
}

// initBranchInput creates and initializes the branch filter input
func (m *Model) initBranchInput() {
	m.branchInput = textinput.New()
	m.branchInput.Placeholder = "Type to filter branches..."
	m.branchInput.Focus()
	m.branchInput.CharLimit = 100
	m.branchInput.Width = 50
	m.branchCursor = 0
	m.branchScrollOffset = 0
}

func (m *Model) doCreateSession(baseBranch string, useExisting bool) tea.Cmd {
	name := m.pendingSessionName
	m.state = stateCreating
	m.createOutput = ""

	return func() tea.Msg {
		var buf bytes.Buffer
		session, err := m.service.CreateSession(name, baseBranch, useExisting, &buf)
		if err != nil {
			return errMsg{err}
		}
		return sessionCreatedMsg{
			session: session,
			output:  buf.String(),
		}
	}
}

// getSelectedSession returns the currently selected session, or nil if cursor
// is on "Create new" option or out of bounds
func (m *Model) getSelectedSession() *session.Session {
	if m.cursor == 0 {
		return nil
	}
	sessionIdx := m.cursor - 1
	if sessionIdx >= 0 && sessionIdx < len(m.filteredSessions) {
		return m.filteredSessions[sessionIdx]
	}
	return nil
}

func (m *Model) handleDelete() (tea.Model, tea.Cmd) {
	selected := m.getSelectedSession()
	if selected == nil {
		return m, nil
	}
	m.selectedSession = selected
	m.state = stateConfirmDelete
	return m, nil
}

func (m *Model) handleArchiveToggle() (tea.Model, tea.Cmd) {
	selected := m.getSelectedSession()
	if selected == nil {
		return m, nil
	}

	if selected.Status == "archived" {
		return m, func() tea.Msg {
			err := m.service.UnarchiveSession(selected.Name)
			if err != nil {
				return errMsg{err}
			}
			return sessionUnarchivedMsg{selected.Name}
		}
	}
	return m, func() tea.Msg {
		err := m.service.ArchiveSession(selected.Name)
		if err != nil {
			return errMsg{err}
		}
		return sessionArchivedMsg{selected.Name}
	}
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

func (m *Model) handleCreateFromExistingBranch() (tea.Model, tea.Cmd) {
	m.state = stateSelectExistingBranch
	m.initBranchInput()

	return m, m.loadBranches()
}

// showHeadOption returns true if the HEAD option should be visible in the branch list
func (m *Model) showHeadOption() bool {
	filter := strings.ToLower(m.branchInput.Value())
	return filter == "" || strings.Contains("head", filter)
}

func (m *Model) handleSelectBaseBranchKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	showHead := m.showHeadOption()
	totalItems := len(m.filteredBranches)
	if showHead {
		totalItems++
	}

	switch msg.String() {
	case "ctrl+c", "esc":
		m.state = stateMain
		m.pendingSessionName = ""
		return m, nil

	case "up":
		if m.branchCursor > 0 {
			m.branchCursor--
			m.adjustBranchScroll(totalItems)
		}
		return m, nil

	case "down":
		if m.branchCursor < totalItems-1 {
			m.branchCursor++
			m.adjustBranchScroll(totalItems)
		}
		return m, nil

	case "enter":
		if totalItems == 0 {
			return m, nil
		}
		baseBranch := m.getSelectedBaseBranch(showHead)
		if baseBranch == "" {
			return m, nil
		}
		return m, m.doCreateSession(baseBranch, false)

	default:
		var cmd tea.Cmd
		m.branchInput, cmd = m.branchInput.Update(msg)
		m.filterBranches()
		m.clampBranchCursor()
		return m, cmd
	}
}

// getSelectedBaseBranch returns the branch name for the current cursor position
func (m *Model) getSelectedBaseBranch(showHead bool) string {
	if showHead && m.branchCursor == 0 {
		return "HEAD"
	}
	branchIdx := m.branchCursor
	if showHead {
		branchIdx--
	}
	if branchIdx >= 0 && branchIdx < len(m.filteredBranches) {
		return m.filteredBranches[branchIdx]
	}
	return ""
}

// clampBranchCursor ensures the branch cursor stays within valid bounds
func (m *Model) clampBranchCursor() {
	total := len(m.filteredBranches)
	if m.showHeadOption() {
		total++
	}
	if m.branchCursor >= total {
		m.branchCursor = total - 1
	}
	if m.branchCursor < 0 {
		m.branchCursor = 0
	}
}

func (m *Model) handleSelectExistingBranchKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	totalItems := len(m.filteredBranches)

	switch msg.String() {
	case "ctrl+c", "esc":
		m.state = stateMain
		return m, nil

	case "up":
		if m.branchCursor > 0 {
			m.branchCursor--
			m.adjustBranchScroll(totalItems)
		}
		return m, nil

	case "down":
		if m.branchCursor < totalItems-1 {
			m.branchCursor++
			m.adjustBranchScroll(totalItems)
		}
		return m, nil

	case "enter":
		if totalItems == 0 || m.branchCursor >= totalItems {
			return m, nil
		}
		selectedBranch := m.filteredBranches[m.branchCursor]

		if m.branchesWithSessions[selectedBranch] {
			m.selectedBranchName = selectedBranch
			m.state = stateConfirmBranchWithSession
			return m, nil
		}

		m.pendingSessionName = selectedBranch
		return m, m.doCreateSession(selectedBranch, true)

	default:
		var cmd tea.Cmd
		m.branchInput, cmd = m.branchInput.Update(msg)
		m.filterBranches()
		m.clampExistingBranchCursor()
		return m, cmd
	}
}

// clampExistingBranchCursor ensures the cursor stays within filtered branch bounds
func (m *Model) clampExistingBranchCursor() {
	if m.branchCursor >= len(m.filteredBranches) {
		m.branchCursor = len(m.filteredBranches) - 1
	}
	if m.branchCursor < 0 {
		m.branchCursor = 0
	}
}

func (m *Model) handleConfirmBranchWithSessionKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		// User wants to create a new session from this branch
		// Transition to new session name input
		m.newSessionInput = textinput.New()
		m.newSessionInput.Placeholder = "Enter new session name..."
		m.newSessionInput.Focus()
		m.newSessionInput.CharLimit = 100
		m.newSessionInput.Width = 50
		m.state = stateEnterNewSessionName
		return m, nil

	case "n", "N", "esc":
		// Go back to branch selection
		m.state = stateSelectExistingBranch
		m.selectedBranchName = ""
		return m, nil

	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) handleEnterNewSessionNameKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "esc":
		// Go back to branch selection
		m.state = stateSelectExistingBranch
		m.selectedBranchName = ""
		return m, nil

	case "enter":
		name := strings.TrimSpace(m.newSessionInput.Value())
		if name == "" {
			m.err = fmt.Errorf("session name cannot be empty")
			return m, nil
		}

		// Validate name
		if err := worktree.ValidateBranchName(name); err != nil {
			m.err = fmt.Errorf("invalid session name: %w", err)
			return m, nil
		}

		// Create session with new branch from the selected branch
		m.pendingSessionName = name
		return m, m.doCreateSession(m.selectedBranchName, false) // false = create new branch

	default:
		// Pass key to text input
		var cmd tea.Cmd
		m.newSessionInput, cmd = m.newSessionInput.Update(msg)
		m.err = nil // Clear error on typing
		return m, cmd
	}
}

// filterBranches filters branches based on the branch input query
func (m *Model) filterBranches() {
	query := strings.ToLower(strings.TrimSpace(m.branchInput.Value()))

	if query == "" {
		m.filteredBranches = m.branches
	} else {
		m.filteredBranches = []string{}
		for _, branch := range m.branches {
			if strings.Contains(strings.ToLower(branch), query) {
				m.filteredBranches = append(m.filteredBranches, branch)
			}
		}
	}

	// Clamp cursor to valid range
	maxCursor := len(m.filteredBranches)
	if m.state == stateSelectBaseBranch {
		// +1 for HEAD option in base branch selection
		if m.branchCursor > maxCursor {
			m.branchCursor = maxCursor
		}
	} else {
		// For existing branch selection, all branches are selectable
		if m.branchCursor >= maxCursor {
			m.branchCursor = maxCursor - 1
			if m.branchCursor < 0 {
				m.branchCursor = 0
			}
		}
	}
	m.branchScrollOffset = 0
}

// adjustBranchScroll ensures the branch cursor is visible within the scroll window
func (m *Model) adjustBranchScroll(totalItems int) {
	maxVisible := m.maxVisibleBranches()
	if maxVisible <= 0 {
		maxVisible = 1
	}

	if m.branchCursor < m.branchScrollOffset {
		m.branchScrollOffset = m.branchCursor
	}

	if m.branchCursor >= m.branchScrollOffset+maxVisible {
		m.branchScrollOffset = m.branchCursor - maxVisible + 1
	}

	if m.branchScrollOffset < 0 {
		m.branchScrollOffset = 0
	}
}

// maxVisibleBranches returns how many branches can fit in the terminal
func (m *Model) maxVisibleBranches() int {
	if m.windowHeight <= fixedUILines {
		return 1
	}
	availableLines := m.windowHeight - fixedUILines
	// Each branch is 1 line
	maxBranches := availableLines

	if maxBranches > maxSessionsVisible {
		maxBranches = maxSessionsVisible
	}
	return maxBranches
}

func (m *Model) filterSessions() {
	query := strings.ToLower(strings.TrimSpace(m.textInput.Value()))

	// Partition sessions into active and archived, filtering by query
	var active, archived []*session.Session
	for _, s := range m.sessions {
		if query != "" && !strings.Contains(strings.ToLower(s.Name), query) {
			continue
		}
		if s.Status == "archived" {
			archived = append(archived, s)
		} else {
			active = append(active, s)
		}
	}

	// Display order: active sessions first, then archived
	m.filteredSessions = append(active, archived...)

	// Clamp cursor to valid range
	if maxCursor := len(m.filteredSessions); m.cursor > maxCursor {
		m.cursor = maxCursor
	}
	m.adjustScroll()
}

// maxVisibleSessions returns how many sessions can fit in the terminal
func (m *Model) maxVisibleSessions() int {
	if m.windowHeight <= fixedUILines {
		return 1
	}
	availableLines := m.windowHeight - fixedUILines
	maxSessions := availableLines / linesPerSession

	if maxSessions > maxSessionsVisible {
		maxSessions = maxSessionsVisible
	}
	return maxSessions
}

// adjustScroll ensures the cursor is visible within the scroll window
func (m *Model) adjustScroll() {
	maxVisible := m.maxVisibleSessions()
	if maxVisible <= 0 {
		maxVisible = 1
	}

	// If cursor is above visible area, scroll up
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	}

	// If cursor is below visible area, scroll down
	if m.cursor >= m.scrollOffset+maxVisible {
		m.scrollOffset = m.cursor - maxVisible + 1
	}

	// Ensure scroll offset is not negative
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

func (m *Model) View() string {
	switch m.state {
	case stateMain:
		return m.viewMain()
	case stateCreating:
		return m.viewCreating()
	case stateConfirmDelete:
		return m.viewConfirmDelete()
	case stateSelectBaseBranch:
		return m.viewSelectBaseBranch()
	case stateSelectExistingBranch:
		return m.viewSelectExistingBranch()
	case stateConfirmBranchWithSession:
		return m.viewConfirmBranchWithSession()
	case stateEnterNewSessionName:
		return m.viewEnterNewSessionName()
	}
	return ""
}

func (m *Model) viewMain() string {
	var content strings.Builder
	divider := dividerStyle.Render(strings.Repeat("─", contentWidth))
	pad := " " // Manual padding since container has none

	// Repo name (right-aligned)
	repoNameStyled := subtitleStyle.Render("repo: " + m.repoName)
	repoNameWidth := lipgloss.Width(repoNameStyled)
	repoNamePadding := contentWidth - repoNameWidth - 1 // -1 for right margin
	if repoNamePadding < 0 {
		repoNamePadding = 0
	}
	content.WriteString(strings.Repeat(" ", repoNamePadding) + repoNameStyled + "\n")
	// Input
	content.WriteString(pad + m.textInput.View() + "\n")
	// Error/Message right under search bar (always reserve this line for consistent height)
	if m.err != nil {
		content.WriteString(pad + errorStyle.Render(fmt.Sprintf("Error: %s", m.err.Error())))
	} else if m.message != "" {
		content.WriteString(pad + successStyle.Render(m.message))
	} else {
		content.WriteString(pad)
	}
	content.WriteString("\n")
	// Divider before session list
	content.WriteString(divider + "\n")

	// "Create new" option (always visible, position 0)
	createNewText := fmt.Sprintf("+ New session \"%s\"", m.textInput.Value())
	if m.cursor == 0 {
		content.WriteString(pad + selectedItemStyle.Render("▶ " + createNewText))
	} else {
		content.WriteString(pad + createNewStyle.Render("  " + createNewText))
	}
	content.WriteString("\n")

	// Calculate visible range
	maxVisible := m.maxVisibleSessions()
	totalSessions := len(m.filteredSessions)

	// Always show scroll indicator line (empty if nothing above)
	if m.scrollOffset > 0 {
		content.WriteString(pad + metadataStyle.Render(fmt.Sprintf("  ↑ %d more", m.scrollOffset)))
	} else {
		content.WriteString(pad) // Reserve space
	}
	content.WriteString("\n")

	// Render visible sessions
	endIdx := m.scrollOffset + maxVisible
	if endIdx > totalSessions {
		endIdx = totalSessions
	}

	sessionsRendered := 0
	for i := m.scrollOffset; i < endIdx; i++ {
		s := m.filteredSessions[i]
		cursorPos := i + 1 // +1 for "Create new" option
		m.renderSession(&content, s, cursorPos)
		sessionsRendered++
	}

	// Add padding lines to fill terminal height (each session is 2 lines)
	paddingNeeded := (maxVisible - sessionsRendered) * linesPerSession
	// Account for remainder when available lines is odd
	availableLines := m.windowHeight - fixedUILines
	if availableLines > 0 && availableLines%linesPerSession != 0 {
		paddingNeeded += availableLines % linesPerSession
	}
	for i := 0; i < paddingNeeded; i++ {
		content.WriteString(pad + "\n")
	}

	// Always show scroll indicator line (empty if nothing below)
	if endIdx < totalSessions {
		content.WriteString(pad + metadataStyle.Render(fmt.Sprintf("  ↓ %d more", totalSessions-endIdx)))
	} else {
		content.WriteString(pad) // Reserve space
	}
	content.WriteString("\n")

	// Help text (divider then instructions on two rows)
	content.WriteString(divider + "\n")
	content.WriteString(pad + helpStyle.Render("[↑/↓] Navigate  [Enter] Select  [^B] From branch"))
	content.WriteString("\n")
	content.WriteString(pad + helpStyle.Render("[^D] Delete  [^A] Archive  [Esc] Quit"))
	content.WriteString("\n")

	// Build the box manually for full control (no lipgloss border)

	// Top border with embedded title
	title := titleStyle.Render("🛫 Air Traffic Control") + " " + subtitleStyle.Render("v"+appVersion)
	topLeft := borderStyle.Render("╭─ ")
	titleWidth := lipgloss.Width(topLeft) + lipgloss.Width(title) + 2 // +2 for space and closing corner
	dashCount := contentWidth - titleWidth + 2                        // +2 for side borders
	if dashCount < 1 {
		dashCount = 1
	}
	topRight := borderStyle.Render(" " + strings.Repeat("─", dashCount) + "╮")
	topBorder := topLeft + title + topRight

	// Wrap each content line with side borders
	lines := strings.Split(content.String(), "\n")
	var body strings.Builder
	body.WriteString(topBorder + "\n")
	leftBorder := borderStyle.Render("│")
	rightBorder := borderStyle.Render("│")
	for _, line := range lines {
		if line == "" {
			continue // Skip empty lines
		}
		// Use lipgloss.Width for visual width (handles ANSI codes)
		visualWidth := lipgloss.Width(line)
		padding := contentWidth - visualWidth
		if padding < 0 {
			padding = 0
		}
		body.WriteString(leftBorder + line + strings.Repeat(" ", padding) + rightBorder + "\n")
	}

	// Bottom border
	body.WriteString(borderStyle.Render("╰" + strings.Repeat("─", contentWidth) + "╯"))

	return body.String()
}

func (m *Model) renderSession(b *strings.Builder, s *session.Session, cursorPos int) {
	pad := " " // Manual padding
	cursor := "  "
	style := normalItemStyle
	metaStyle := metadataStyle
	if m.cursor == cursorPos {
		cursor = "▶ "
		style = selectedItemStyle
	}

	if s.Status == "archived" {
		style = archivedItemStyle
		metaStyle = archivedItemStyle
	}

	// Session name
	b.WriteString(pad + style.Render(cursor+s.Name))
	b.WriteString("\n")

	// Metadata line: summary (if any) + time
	var timeStr string
	if s.ArchivedAt != nil {
		timeStr = "archived " + formatTime(*s.ArchivedAt)
	} else if s.LastAccessed != nil {
		timeStr = formatTime(*s.LastAccessed)
	} else {
		timeStr = formatTime(s.CreatedAt)
	}

	// Get conversation summary
	summary := worktree.GetConversationSummary(s.WorktreePath)
	var metaLine string
	if summary != "" {
		if len(summary) > summaryMaxLen {
			summary = summary[:summaryTruncateAt] + "..."
		}
		metaLine = fmt.Sprintf("    %s · %s", summary, timeStr)
	} else {
		metaLine = "    " + timeStr
	}
	b.WriteString(pad + metaStyle.Render(metaLine))
	b.WriteString("\n")
}

func (m *Model) viewCreating() string {
	var content strings.Builder

	header := titleStyle.Render("✈ Air Traffic Control")
	content.WriteString(headerStyle.Render(header))
	content.WriteString("\n\n")

	content.WriteString(statusStyle.Render(fmt.Sprintf("%s Creating \"%s\"...", m.spinner.View(), m.textInput.Value())))
	content.WriteString("\n\n")

	if m.createOutput != "" {
		content.WriteString(dialogTextStyle.Render(m.createOutput))
	}

	return containerStyle.Render(content.String())
}

func (m *Model) viewConfirmDelete() string {
	if m.selectedSession == nil {
		return ""
	}

	var b strings.Builder

	b.WriteString(dialogTitleStyle.Render("Delete Session"))
	b.WriteString("\n\n")

	b.WriteString(dialogTextStyle.Render(fmt.Sprintf("Delete \"%s\"?", m.selectedSession.Name)))
	b.WriteString("\n\n")

	b.WriteString(dialogTextStyle.Render("This will:"))
	b.WriteString("\n")
	b.WriteString(dialogTextStyle.Render("  • Remove the git worktree"))
	b.WriteString("\n")
	b.WriteString(dialogTextStyle.Render("  • Delete all local changes"))
	b.WriteString("\n")
	b.WriteString(dialogTextStyle.Render("  • Remove the session"))
	b.WriteString("\n\n")

	b.WriteString(warningStyle.Render("This cannot be undone."))
	b.WriteString("\n\n")

	b.WriteString(dialogTextStyle.Render("[Y] Yes, delete    [N] Cancel"))

	return dialogBoxStyle.Render(b.String())
}

func (m *Model) viewSelectBaseBranch() string {
	var content strings.Builder
	divider := dividerStyle.Render(strings.Repeat("─", contentWidth))
	pad := " "

	titleLine := fmt.Sprintf("Creating session \"%s\"", m.pendingSessionName)
	content.WriteString(pad + titleStyle.Render(titleLine) + "\n")
	content.WriteString(pad + subtitleStyle.Render("Select base branch:") + "\n")
	content.WriteString(pad + m.branchInput.View() + "\n")
	content.WriteString(pad + "\n")
	content.WriteString(divider + "\n")
	content.WriteString(pad + "\n")

	showHead := m.showHeadOption()
	if showHead {
		headLabel := fmt.Sprintf("HEAD (%s)", m.currentBranch)
		if m.branchCursor == 0 {
			content.WriteString(pad + selectedItemStyle.Render("▶ "+headLabel) + "\n")
		} else {
			content.WriteString(pad + normalItemStyle.Render("  "+headLabel) + "\n")
		}
	}

	// Calculate visible range for branches
	maxVisible := m.maxVisibleBranches() - 1 // -1 for HEAD option
	if maxVisible < 1 {
		maxVisible = 1
	}
	totalBranches := len(m.filteredBranches)

	// Adjust scroll for branch list (cursor 1+ maps to branches, or 0+ if HEAD hidden)
	cursorOffset := 0
	if showHead {
		cursorOffset = 1
	}
	branchCursor := m.branchCursor - cursorOffset // Convert to branch index
	startIdx := m.branchScrollOffset
	if branchCursor >= 0 {
		if branchCursor < startIdx {
			startIdx = branchCursor
		}
		if branchCursor >= startIdx+maxVisible {
			startIdx = branchCursor - maxVisible + 1
		}
	}
	endIdx := startIdx + maxVisible
	if endIdx > totalBranches {
		endIdx = totalBranches
	}

	// Scroll up indicator
	if startIdx > 0 {
		content.WriteString(pad + metadataStyle.Render(fmt.Sprintf("  ↑ %d more", startIdx)) + "\n")
	}

	// Render visible branches
	for i := startIdx; i < endIdx; i++ {
		branch := m.filteredBranches[i]
		cursorPos := i + cursorOffset // Adjust for HEAD option
		if m.branchCursor == cursorPos {
			content.WriteString(pad + selectedItemStyle.Render("▶ "+branch) + "\n")
		} else {
			content.WriteString(pad + normalItemStyle.Render("  "+branch) + "\n")
		}
	}

	// Scroll down indicator
	if endIdx < totalBranches {
		content.WriteString(pad + metadataStyle.Render(fmt.Sprintf("  ↓ %d more", totalBranches-endIdx)) + "\n")
	}

	content.WriteString(pad + "\n")
	content.WriteString(divider + "\n")
	content.WriteString(pad + helpStyle.Render("[↑/↓] Navigate  [Enter] Select  [Esc] Cancel") + "\n")

	return m.wrapInBox(content.String())
}

func (m *Model) viewSelectExistingBranch() string {
	var content strings.Builder
	divider := dividerStyle.Render(strings.Repeat("─", contentWidth))
	pad := " "

	// Title and input
	content.WriteString(pad + titleStyle.Render("Create session from existing branch:") + "\n")
	content.WriteString(pad + m.branchInput.View() + "\n")
	content.WriteString(divider + "\n")

	// Calculate how many branches we can show
	// Fixed lines: title(1) + input(1) + divider(1) + divider(1) + help(1) + borders(2) = 8
	branchAreaHeight := m.windowHeight - 8
	if branchAreaHeight < 1 {
		branchAreaHeight = 1
	}
	if branchAreaHeight > maxSessionsVisible {
		branchAreaHeight = maxSessionsVisible
	}

	totalBranches := len(m.filteredBranches)

	if totalBranches == 0 {
		content.WriteString(pad + metadataStyle.Render("  No branches match filter") + "\n")
	} else {
		// Calculate scroll window
		startIdx := m.branchScrollOffset
		if m.branchCursor < startIdx {
			startIdx = m.branchCursor
		}
		if m.branchCursor >= startIdx+branchAreaHeight {
			startIdx = m.branchCursor - branchAreaHeight + 1
		}
		if startIdx < 0 {
			startIdx = 0
		}
		endIdx := startIdx + branchAreaHeight
		if endIdx > totalBranches {
			endIdx = totalBranches
		}

		// Scroll up indicator
		if startIdx > 0 {
			content.WriteString(pad + metadataStyle.Render(fmt.Sprintf("  ↑ %d more", startIdx)) + "\n")
		}

		// Render visible branches
		for i := startIdx; i < endIdx; i++ {
			branch := m.filteredBranches[i]
			hasSession := m.branchesWithSessions[branch]

			displayName := branch
			if hasSession {
				displayName = "● " + branch
			}

			if m.branchCursor == i {
				content.WriteString(pad + selectedItemStyle.Render("▶ "+displayName) + "\n")
			} else {
				content.WriteString(pad + normalItemStyle.Render("  "+displayName) + "\n")
			}
		}

		// Scroll down indicator
		if endIdx < totalBranches {
			content.WriteString(pad + metadataStyle.Render(fmt.Sprintf("  ↓ %d more", totalBranches-endIdx)) + "\n")
		}
	}

	content.WriteString(divider + "\n")
	content.WriteString(pad + helpStyle.Render("[↑/↓] Navigate  [Enter] Select  [Esc] Cancel  ● has session"))

	return m.wrapInBox(content.String())
}

func (m *Model) viewConfirmBranchWithSession() string {
	var b strings.Builder

	b.WriteString(dialogTitleStyle.Render("Branch Has Existing Session"))
	b.WriteString("\n\n")

	b.WriteString(dialogTextStyle.Render(fmt.Sprintf("Branch \"%s\" already has a session.", m.selectedBranchName)))
	b.WriteString("\n\n")

	b.WriteString(dialogTextStyle.Render("Would you like to create a new session"))
	b.WriteString("\n")
	b.WriteString(dialogTextStyle.Render("branching from this branch?"))
	b.WriteString("\n\n")

	b.WriteString(dialogTextStyle.Render("[Y] Yes, create new branch    [N] Cancel"))

	return dialogBoxStyle.Render(b.String())
}

func (m *Model) viewEnterNewSessionName() string {
	var content strings.Builder
	divider := dividerStyle.Render(strings.Repeat("─", contentWidth))
	pad := " "

	// Title
	content.WriteString(pad + titleStyle.Render(fmt.Sprintf("New session from \"%s\"", m.selectedBranchName)) + "\n")
	content.WriteString(pad + subtitleStyle.Render("Enter a name for the new session:") + "\n")
	content.WriteString(pad + "\n")

	// Input
	content.WriteString(pad + m.newSessionInput.View() + "\n")
	content.WriteString(pad + "\n")

	// Error if any
	if m.err != nil {
		content.WriteString(pad + errorStyle.Render(fmt.Sprintf("Error: %s", m.err.Error())) + "\n")
		content.WriteString(pad + "\n")
	}

	content.WriteString(divider + "\n")
	content.WriteString(pad + helpStyle.Render("[Enter] Create  [Esc] Cancel") + "\n")

	return m.wrapInBox(content.String())
}

// wrapInBox wraps content in the standard ATC border box
func (m *Model) wrapInBox(content string) string {
	// Top border with embedded title
	title := titleStyle.Render("🛫 Air Traffic Control") + " " + subtitleStyle.Render("v"+appVersion)
	topLeft := borderStyle.Render("╭─ ")
	titleWidth := lipgloss.Width(topLeft) + lipgloss.Width(title) + 2 // +2 for space and closing corner
	dashCount := contentWidth - titleWidth + 2                        // +2 for side borders
	if dashCount < 1 {
		dashCount = 1
	}
	topRight := borderStyle.Render(" " + strings.Repeat("─", dashCount) + "╮")
	topBorder := topLeft + title + topRight

	// Wrap each content line with side borders
	lines := strings.Split(content, "\n")
	var body strings.Builder
	body.WriteString(topBorder + "\n")
	leftBorder := borderStyle.Render("│")
	rightBorder := borderStyle.Render("│")
	for _, line := range lines {
		if line == "" {
			continue
		}
		visualWidth := lipgloss.Width(line)
		padding := contentWidth - visualWidth
		if padding < 0 {
			padding = 0
		}
		body.WriteString(leftBorder + line + strings.Repeat(" ", padding) + rightBorder + "\n")
	}

	// Bottom border
	body.WriteString(borderStyle.Render("╰" + strings.Repeat("─", contentWidth) + "╯"))

	return body.String()
}

func formatTime(t time.Time) string {
	diff := time.Since(t)

	if diff < time.Minute {
		return "just now"
	}

	type timeUnit struct {
		threshold time.Duration
		divisor   time.Duration
		singular  string
		plural    string
	}

	units := []timeUnit{
		{time.Hour, time.Minute, "minute", "minutes"},
		{24 * time.Hour, time.Hour, "hour", "hours"},
		{7 * 24 * time.Hour, 24 * time.Hour, "day", "days"},
		{30 * 24 * time.Hour, 7 * 24 * time.Hour, "week", "weeks"},
		{365 * 24 * time.Hour, 30 * 24 * time.Hour, "month", "months"},
	}

	for _, unit := range units {
		if diff < unit.threshold {
			count := int(diff / unit.divisor)
			if count == 1 {
				return "1 " + unit.singular + " ago"
			}
			return fmt.Sprintf("%d %s ago", count, unit.plural)
		}
	}

	// Fallback for very old times (over a year)
	months := int(diff / (30 * 24 * time.Hour))
	if months == 1 {
		return "1 month ago"
	}
	return fmt.Sprintf("%d months ago", months)
}

// GetCommandToExec returns the command that should be executed after TUI exits
func (m *Model) GetCommandToExec() string {
	return m.commandToExec
}
