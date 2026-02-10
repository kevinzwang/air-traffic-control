package tui

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kevinzwang/air-traffic-control/internal/database"
	"github.com/kevinzwang/air-traffic-control/internal/session"
	"github.com/kevinzwang/air-traffic-control/internal/terminal"
	"github.com/kevinzwang/air-traffic-control/internal/worktree"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\([A-Za-z]`)

// Version is set via ldflags at build time
var Version = "dev"

// Focus state
type focus int

const (
	focusSidebar focus = iota
	focusTerminal
)

// Overlay state
type overlay int

const (
	overlayNone overlay = iota
	overlayCreateSession
	overlaySelectBaseBranch
	overlaySelectExistingBranch
	overlayConfirmBranchWithSession
	overlayEnterNewSessionName
	overlayDeleteConfirm
	overlayHelp
	overlayCreating
	overlayArchivedSessions
	overlaySelectProject
)

// Custom messages
type sessionsLoadedMsg struct {
	sessions []*session.Session
}

type sessionCreatedMsg struct {
	session       *session.Session
	setupCommands []string
}

type setupCompleteMsg struct {
	sessionName string
	err         error
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

type projectsLoadedMsg struct {
	projects []*database.Project
}

type projectSwitchedMsg struct {
	service  *session.Service
	repoName string
}

type Model struct {
	// Core state
	focus         focus
	overlay       overlay
	db            *database.DB
	service       *session.Service
	repoName      string
	sessions      []*session.Session
	cursor        int
	scrollOffset  int
	activeSession *session.Session // Currently viewed session

	// Terminal instances (session name -> Terminal)
	terminals  map[string]*terminal.Terminal
	program    *tea.Program
	tmuxSocket string

	// Project selection state
	projects             []*database.Project
	filteredProjects     []*database.Project
	projectCursor        int
	projectScrollOffset  int
	projectInput         textinput.Model
	noProjectMode        bool

	// Window dimensions
	windowWidth  int
	windowHeight int

	// Archived sessions overlay
	archivedCursor       int
	archivedScrollOffset int
	archivedList         []*session.Session
	deleteFromArchived   bool

	// Spinner for creating state
	spinner            spinner.Model
	err                error
	message            string
	settingUpSessions  map[string]bool

	// Session creation fields
	createInput        textinput.Model
	pendingSessionName string
	selectAfterLoad    string // session name to select after next sessionsLoadedMsg
	activatingSession  string // session name currently being activated (to prevent double-create)

	// Branch selection fields
	branches             []string
	filteredBranches     []string
	branchInput          textinput.Model
	branchCursor         int
	branchScrollOffset   int
	branchesWithSessions map[string]bool
	currentBranch        string
	selectedBranchName   string
	newSessionInput      textinput.Model

	// Delete confirmation
	selectedSession *session.Session

	// Text selection state
	selecting    bool // currently dragging
	selStartCol  int  // terminal-relative column where drag started
	selStartRow  int  // terminal-relative row where drag started
	selEndCol    int  // current drag column
	selEndRow    int  // current drag row
	hasSelection bool // selection is visible

	// Budget of SGR mouse fragment characters to suppress (over SSH,
	// TCP can split escape sequences and Bubble Tea parses the tail
	// bytes as individual key events). Reset after each mouse event.
	mouseFragmentBudget int
}

func NewModel(db *database.DB, service *session.Service, repoName string, invokingBranch string) *Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	var tmuxSocket string
	if service != nil {
		// Stable socket name based on repo path so tmux sessions persist across ATC restarts
		hash := sha256.Sum256([]byte(service.RepoPath()))
		tmuxSocket = fmt.Sprintf("atc-%x", hash[:4])
	}

	return &Model{
		focus:             focusSidebar,
		overlay:           overlayNone,
		db:                db,
		service:           service,
		repoName:          repoName,
		spinner:           s,
		currentBranch:     invokingBranch,
		terminals:         make(map[string]*terminal.Terminal),
		tmuxSocket:        tmuxSocket,
		settingUpSessions: make(map[string]bool),
		noProjectMode:     service == nil,
	}
}

// SetProgram sets the Bubble Tea program reference, needed for terminal async messages.
func (m *Model) SetProgram(p *tea.Program) {
	m.program = p
}

func (m *Model) Init() tea.Cmd {
	if m.noProjectMode {
		return tea.Batch(
			m.loadProjects(),
			m.spinner.Tick,
		)
	}
	return tea.Batch(
		m.loadSessions(),
		m.spinner.Tick,
	)
}

// detachTerminal detaches the terminal for the given session name (stops polling)
// and removes it from the terminals map. The tmux session keeps running.
func (m *Model) detachTerminal(name string) {
	if t, ok := m.terminals[name]; ok {
		t.Detach()
		delete(m.terminals, name)
	}
}

// --- Data loading commands ---

func (m *Model) loadSessions() tea.Cmd {
	return func() tea.Msg {
		if m.service == nil {
			return sessionsLoadedMsg{sessions: nil}
		}
		sessions, err := m.service.ListSessions("")
		if err != nil {
			return errMsg{err}
		}
		return sessionsLoadedMsg{sessions}
	}
}

func (m *Model) loadBranches() tea.Cmd {
	return func() tea.Msg {
		if m.service == nil {
			return errMsg{fmt.Errorf("no project selected")}
		}
		branches, err := m.service.ListBranches()
		if err != nil {
			return errMsg{err}
		}

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

func (m *Model) loadProjects() tea.Cmd {
	return func() tea.Msg {
		projects, err := m.db.ListProjects()
		if err != nil {
			return errMsg{err}
		}
		return projectsLoadedMsg{projects}
	}
}

func (m *Model) switchProject(project *database.Project) tea.Cmd {
	return func() tea.Msg {
		svc, err := session.NewService(m.db, project.RepoPath)
		if err != nil {
			return errMsg{err}
		}
		return projectSwitchedMsg{service: svc, repoName: project.RepoName}
	}
}

// --- Update ---

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.windowWidth = msg.Width
		m.windowHeight = msg.Height
		// Resize active terminal
		if m.activeSession != nil {
			if t, ok := m.terminals[m.activeSession.Name]; ok {
				tw, th := m.terminalPaneDimensions()
				t.Resize(tw, th)
			}
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case tea.MouseMsg:
		return m.handleMouseMsg(msg)

	case sessionsLoadedMsg:
		m.sessions = msg.sessions
		active := m.activeSessions()
		// If we need to select a specific session (e.g. just created), move cursor to it
		if m.selectAfterLoad != "" {
			for i, s := range active {
				if s.Name == m.selectAfterLoad {
					m.cursor = i
					break
				}
			}
			m.selectAfterLoad = ""
		}
		// Clamp cursor to valid range
		maxIdx := len(active) - 1
		if m.archivedCount() > 0 {
			maxIdx++
		}
		if maxIdx < 0 {
			maxIdx = 0
		}
		if m.cursor > maxIdx {
			m.cursor = maxIdx
		}
		cmd := m.switchViewToCurrentSession()
		// Refresh archived overlay if open
		if m.overlay == overlayArchivedSessions {
			m.archivedList = m.archivedSessionsList()
			if len(m.archivedList) == 0 {
				m.overlay = overlayNone
			} else if m.archivedCursor >= len(m.archivedList) {
				m.archivedCursor = len(m.archivedList) - 1
			}
		}
		return m, cmd

	case branchesLoadedMsg:
		m.branches = msg.branches
		m.branchesWithSessions = msg.branchesWithSessions
		m.filterBranches()
		return m, nil

	case sessionCreatedMsg:
		m.overlay = overlayNone
		m.pendingSessionName = ""
		m.selectAfterLoad = msg.session.Name
		m.activatingSession = msg.session.Name
		cmds := []tea.Cmd{m.loadSessions(), m.activateSession(msg.session, true)}
		if len(msg.setupCommands) > 0 {
			m.settingUpSessions[msg.session.Name] = true
			cmds = append(cmds, m.runSetupInBackground(msg.session.Name, msg.session.WorktreePath, msg.setupCommands))
		}
		return m, tea.Batch(cmds...)

	case sessionDeletedMsg:
		m.message = fmt.Sprintf("Session '%s' deleted", msg.name)
		m.selectedSession = nil
		if m.activeSession != nil && m.activeSession.Name == msg.name {
			m.activeSession = nil
		}
		if m.deleteFromArchived {
			m.overlay = overlayArchivedSessions
			m.deleteFromArchived = false
		} else {
			m.overlay = overlayNone
		}
		return m, m.loadSessions()

	case sessionArchivedMsg:
		m.message = fmt.Sprintf("Session '%s' archived", msg.name)
		m.detachTerminal(msg.name)
		if m.activeSession != nil && m.activeSession.Name == msg.name {
			m.activeSession = nil
		}
		return m, m.loadSessions()

	case sessionUnarchivedMsg:
		m.message = fmt.Sprintf("Session '%s' unarchived", msg.name)
		return m, m.loadSessions()

	case setupCompleteMsg:
		if !m.settingUpSessions[msg.sessionName] {
			return m, nil
		}
		delete(m.settingUpSessions, msg.sessionName)
		if msg.err != nil {
			m.err = fmt.Errorf("setup failed for '%s': %w", msg.sessionName, msg.err)
		} else {
			m.message = fmt.Sprintf("Setup complete for '%s'", msg.sessionName)
		}
		return m, nil

	case projectsLoadedMsg:
		m.projects = msg.projects
		m.filteredProjects = msg.projects
		m.projectCursor = 0
		m.projectScrollOffset = 0
		if m.noProjectMode || m.overlay == overlaySelectProject {
			m.initProjectInput()
			m.overlay = overlaySelectProject
		}
		return m, nil

	case projectSwitchedMsg:
		m.service = msg.service
		m.repoName = msg.repoName
		// Recompute tmux socket for the new project
		hash := sha256.Sum256([]byte(msg.service.RepoPath()))
		m.tmuxSocket = fmt.Sprintf("atc-%x", hash[:4])
		m.activeSession = nil
		m.cursor = 0
		m.scrollOffset = 0
		m.noProjectMode = false
		m.overlay = overlayNone
		return m, m.loadSessions()

	case errMsg:
		m.err = msg.err
		if m.overlay == overlayCreating {
			m.overlay = overlayNone
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case terminal.TerminalOutputMsg:
		// Terminal output arrived, just re-render
		return m, nil

	case terminal.TerminalExitedMsg:
		// Terminal process exited - no action needed, View() will show last state
		return m, nil
	}

	return m, nil
}

func (m *Model) activateSession(sess *session.Session, switchFocus bool) tea.Cmd {
	return func() tea.Msg {
		m.activeSession = sess
		if switchFocus {
			m.message = ""
			m.err = nil
			m.focus = focusTerminal
		}

		tw, th := m.terminalPaneDimensions()

		if err := m.ensureTerminal(sess, tw, th); err != nil {
			return errMsg{err}
		}

		if m.service != nil {
			m.service.TouchSession(sess.Name)
		}
		m.activatingSession = ""
		return nil
	}
}

// ensureTerminal guarantees a running terminal wrapper exists for the session.
// It reuses an existing wrapper, reattaches to a persisted tmux session,
// or creates a new tmux session as needed.
func (m *Model) ensureTerminal(sess *session.Session, width, height int) error {
	if m.tmuxSocket == "" {
		return fmt.Errorf("no project selected")
	}

	// If we have a running terminal wrapper, just resize
	if t, ok := m.terminals[sess.Name]; ok && t.IsRunning() {
		t.Resize(width, height)
		return nil
	}

	// If wrapper exists but stopped, detach it before reattaching
	m.detachTerminal(sess.Name)

	// If tmux session already exists on the socket, reattach
	if terminal.SessionExists(m.tmuxSocket, sess.Name) {
		t, err := terminal.Attach(sess.Name, width, height, m.program, m.tmuxSocket)
		if err != nil {
			return err
		}
		m.terminals[sess.Name] = t
		// If the pane process died while ATC was away, respawn with --continue
		if !t.IsRunning() {
			if err := t.Respawn(true); err != nil {
				return err
			}
		}
		return nil
	}

	// No tmux session exists, create a new one
	continueSession := worktree.HasExistingConversation(sess.WorktreePath)
	t, err := terminal.New(sess.Name, sess.WorktreePath, width, height, continueSession, m.program, m.tmuxSocket)
	if err != nil {
		return err
	}
	m.terminals[sess.Name] = t
	return nil
}

// --- Key handling ---

func (m *Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Clear text selection on any key press
	m.hasSelection = false
	m.selecting = false

	// Handle overlays first
	if m.overlay != overlayNone {
		return m.handleOverlayKeys(msg)
	}

	// Ctrl+C from terminal switches back to sidebar
	if msg.String() == "ctrl+c" && m.focus == focusTerminal {
		m.focus = focusSidebar
		m.resizeTerminalIfNeeded()
		return m, nil
	}

	if m.focus == focusTerminal {
		return m.handleTerminalKeys(msg)
	}
	return m.handleSidebarKeys(msg)
}

func (m *Model) handleMouseMsg(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m.mouseFragmentBudget = 15 // max bytes in one SGR mouse seq after ESC

	// Dispatch overlay mouse events first
	if m.overlay != overlayNone {
		return m.handleOverlayMouse(msg)
	}

	var termStartX int
	if m.sidebarVisible() {
		termStartX = sidebarWidth + 1 // sidebar visual width (includes border) + spacer
	} else {
		termStartX = 0
	}

	// Sidebar mouse events (click or wheel in sidebar area)
	if m.sidebarVisible() && msg.X < sidebarWidth {
		switch {
		case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
			return m.handleSidebarMouse(msg)
		case msg.Button == tea.MouseButtonWheelUp:
			return m.handleSidebarWheelUp()
		case msg.Button == tea.MouseButtonWheelDown:
			return m.handleSidebarWheelDown()
		}
		return m, nil
	}

	// Terminal pane mouse events
	switch {
	case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
		// Start selection if click is in terminal pane area
		if msg.X >= termStartX && m.activeSession != nil {
			col, row := m.mouseToTermCoords(msg.X, msg.Y, termStartX)
			m.selecting = true
			m.selStartCol = col
			m.selStartRow = row
			m.selEndCol = col
			m.selEndRow = row
			m.hasSelection = false
			// Click on terminal switches focus
			if m.focus == focusSidebar {
				m.message = ""
				m.err = nil
				m.focus = focusTerminal
				m.resizeTerminalIfNeeded()
			}
		} else {
			m.hasSelection = false
			m.selecting = false
		}
		return m, nil

	case msg.Action == tea.MouseActionMotion:
		if m.selecting {
			col, row := m.mouseToTermCoords(msg.X, msg.Y, termStartX)
			m.selEndCol = col
			m.selEndRow = row
			m.hasSelection = true
		}
		return m, nil

	case msg.Action == tea.MouseActionRelease:
		if m.selecting {
			m.selecting = false
			col, row := m.mouseToTermCoords(msg.X, msg.Y, termStartX)
			m.selEndCol = col
			m.selEndRow = row
			// Only finalize selection if the mouse actually moved
			if m.selStartRow != m.selEndRow || m.selStartCol != m.selEndCol {
				m.hasSelection = true
				m.copySelectionToClipboard()
			} else {
				m.hasSelection = false
			}
		}
		return m, nil

	case msg.Button == tea.MouseButtonWheelUp:
		if m.activeSession == nil {
			return m, nil
		}
		t, ok := m.terminals[m.activeSession.Name]
		if !ok || !t.IsRunning() {
			return m, nil
		}
		m.hasSelection = false
		t.ScrollUp(2)
		return m, nil

	case msg.Button == tea.MouseButtonWheelDown:
		if m.activeSession == nil {
			return m, nil
		}
		t, ok := m.terminals[m.activeSession.Name]
		if !ok || !t.IsRunning() {
			return m, nil
		}
		m.hasSelection = false
		t.ScrollDown(2)
		return m, nil
	}
	return m, nil
}

// handleSidebarMouse handles left-click events in the sidebar area.
func (m *Model) handleSidebarMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	m.hasSelection = false
	m.selecting = false
	m.focus = focusSidebar
	m.resizeTerminalIfNeeded()

	kind, idx := m.sidebarHitTest(msg.Y)
	switch kind {
	case "session":
		m.cursor = idx
		m.adjustScroll()
		return m, m.switchViewToCurrentSession()
	case "archived":
		return m.openArchivedOverlay()
	case "scroll_up":
		if m.scrollOffset > 0 {
			m.scrollOffset--
			// Keep cursor in visible range
			if m.cursor >= m.scrollOffset+m.maxVisibleSessions() {
				m.cursor = m.scrollOffset + m.maxVisibleSessions() - 1
			}
		}
		return m, nil
	case "scroll_down":
		active := m.activeSessions()
		maxVisible := m.maxVisibleSessions()
		maxOffset := len(active) - maxVisible
		if maxOffset < 0 {
			maxOffset = 0
		}
		if m.scrollOffset < maxOffset {
			m.scrollOffset++
			// Keep cursor in visible range
			if m.cursor < m.scrollOffset {
				m.cursor = m.scrollOffset
			}
		}
		return m, nil
	}
	return m, nil
}

// handleSidebarWheelUp scrolls the sidebar session list up.
func (m *Model) handleSidebarWheelUp() (tea.Model, tea.Cmd) {
	if m.scrollOffset > 0 {
		m.scrollOffset--
		// Keep cursor in visible range
		maxVisible := m.maxVisibleSessions()
		if m.cursor >= m.scrollOffset+maxVisible {
			m.cursor = m.scrollOffset + maxVisible - 1
		}
	}
	return m, nil
}

// handleSidebarWheelDown scrolls the sidebar session list down.
func (m *Model) handleSidebarWheelDown() (tea.Model, tea.Cmd) {
	active := m.activeSessions()
	maxVisible := m.maxVisibleSessions()
	maxOffset := len(active) - maxVisible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.scrollOffset < maxOffset {
		m.scrollOffset++
		// Keep cursor in visible range
		if m.cursor < m.scrollOffset {
			m.cursor = m.scrollOffset
		}
	}
	return m, nil
}

// handleOverlayMouse handles mouse events when an overlay is active.
func (m *Model) handleOverlayMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	inside := m.isInsideOverlay(msg.X, msg.Y)

	switch {
	case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
		if !inside {
			// Click outside overlay dismisses it
			return m.dismissOverlay()
		}
		// Click inside specific overlays
		switch m.overlay {
		case overlayHelp:
			// Any click inside help dismisses it
			m.overlay = overlayNone
			return m, nil
		case overlaySelectBaseBranch, overlaySelectExistingBranch:
			return m.handleBranchOverlayClick(msg)
		case overlayArchivedSessions:
			return m.handleArchivedOverlayClick(msg)
		case overlaySelectProject:
			return m.handleProjectOverlayClick(msg)
		}
		return m, nil

	case msg.Button == tea.MouseButtonWheelUp:
		if !inside {
			return m, nil
		}
		switch m.overlay {
		case overlaySelectBaseBranch, overlaySelectExistingBranch:
			if m.branchCursor > 0 {
				m.branchCursor--
			}
		case overlayArchivedSessions:
			if m.archivedCursor > 0 {
				m.archivedCursor--
				if m.archivedCursor < m.archivedScrollOffset {
					m.archivedScrollOffset = m.archivedCursor
				}
			}
		case overlaySelectProject:
			if m.projectCursor > 0 {
				m.projectCursor--
				if m.projectCursor < m.projectScrollOffset {
					m.projectScrollOffset = m.projectCursor
				}
			}
		}
		return m, nil

	case msg.Button == tea.MouseButtonWheelDown:
		if !inside {
			return m, nil
		}
		switch m.overlay {
		case overlaySelectBaseBranch:
			showHead := m.showHeadOption()
			totalItems := len(m.filteredBranches)
			if showHead {
				totalItems++
			}
			if m.branchCursor < totalItems-1 {
				m.branchCursor++
			}
		case overlaySelectExistingBranch:
			if m.branchCursor < len(m.filteredBranches)-1 {
				m.branchCursor++
			}
		case overlayArchivedSessions:
			if m.archivedCursor < len(m.archivedList)-1 {
				m.archivedCursor++
				maxVisible := 10
				if m.archivedCursor >= m.archivedScrollOffset+maxVisible {
					m.archivedScrollOffset = m.archivedCursor - maxVisible + 1
				}
			}
		case overlaySelectProject:
			if m.projectCursor < len(m.filteredProjects)-1 {
				m.projectCursor++
				maxVisible := 10
				if m.projectCursor >= m.projectScrollOffset+maxVisible {
					m.projectScrollOffset = m.projectCursor - maxVisible + 1
				}
			}
		}
		return m, nil
	}
	return m, nil
}

// handleBranchOverlayClick handles clicks inside branch selection overlays.
func (m *Model) handleBranchOverlayClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	startRow, _, _, _ := m.overlayBounds()

	// The overlay content structure for branch selection:
	// dialogBoxStyle adds 1 line top border + 1 line padding = 2 lines before content
	// Then: title line, subtitle (base branch only), blank line, input line, blank line
	// Then the list items start
	contentStart := startRow + 2 // border + padding

	var listStart int
	switch m.overlay {
	case overlaySelectBaseBranch:
		// title + subtitle + blank + input + blank = 5 lines
		listStart = contentStart + 5
	case overlaySelectExistingBranch:
		// title + blank + input + blank = 4 lines
		listStart = contentStart + 4
	default:
		return m, nil
	}

	clickedIdx := msg.Y - listStart
	if clickedIdx < 0 {
		return m, nil
	}

	// Account for "↑ N more" indicator shifting items down
	if m.overlay == overlaySelectBaseBranch {
		// No scroll indicators in base branch (it uses startIdx logic in view)
		// The view handles scroll internally; just map to branchCursor directly
		showHead := m.showHeadOption()
		totalItems := len(m.filteredBranches)
		if showHead {
			totalItems++
		}

		// Compute what's visible: HEAD option, then startIdx..endIdx branches
		cursorOffset := 0
		if showHead {
			cursorOffset = 1
		}
		branchIdx := m.branchCursor - cursorOffset
		maxVisible := 10
		startIdx := 0
		if branchIdx >= startIdx+maxVisible {
			startIdx = branchIdx - maxVisible + 1
		}
		if branchIdx < startIdx && branchIdx >= 0 {
			startIdx = branchIdx
		}

		lineOffset := 0
		// HEAD option takes one line if shown
		if showHead {
			if clickedIdx == 0 {
				m.branchCursor = 0
				return m, nil
			}
			lineOffset = 1
		}
		// "↑ N more" takes one line
		if startIdx > 0 {
			if clickedIdx == lineOffset {
				// Clicked on "↑ N more", scroll up
				if m.branchCursor > 0 {
					m.branchCursor--
				}
				return m, nil
			}
			lineOffset++
		}
		itemIdx := clickedIdx - lineOffset
		if itemIdx >= 0 && startIdx+itemIdx < len(m.filteredBranches) {
			m.branchCursor = startIdx + itemIdx + cursorOffset
			if m.branchCursor >= totalItems {
				m.branchCursor = totalItems - 1
			}
		}
	} else {
		// overlaySelectExistingBranch
		maxVisible := 10
		startIdx := 0
		if m.branchCursor >= startIdx+maxVisible {
			startIdx = m.branchCursor - maxVisible + 1
		}
		endIdx := startIdx + maxVisible
		if endIdx > len(m.filteredBranches) {
			endIdx = len(m.filteredBranches)
		}

		lineOffset := 0
		if startIdx > 0 {
			if clickedIdx == 0 {
				if m.branchCursor > 0 {
					m.branchCursor--
				}
				return m, nil
			}
			lineOffset = 1
		}
		itemIdx := clickedIdx - lineOffset
		if itemIdx >= 0 && startIdx+itemIdx < endIdx {
			m.branchCursor = startIdx + itemIdx
		}
	}

	return m, nil
}

// handleArchivedOverlayClick handles clicks inside the archived sessions overlay.
func (m *Model) handleArchivedOverlayClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if len(m.archivedList) == 0 {
		return m, nil
	}

	startRow, _, _, _ := m.overlayBounds()
	// border + padding + title + blank = 4 lines before list
	listStart := startRow + 4

	clickedIdx := msg.Y - listStart
	if clickedIdx < 0 {
		return m, nil
	}

	lineOffset := 0
	if m.archivedScrollOffset > 0 {
		if clickedIdx == 0 {
			// Clicked "↑ N more"
			if m.archivedCursor > 0 {
				m.archivedCursor--
				if m.archivedCursor < m.archivedScrollOffset {
					m.archivedScrollOffset = m.archivedCursor
				}
			}
			return m, nil
		}
		lineOffset = 1
	}

	maxVisible := 10
	endIdx := m.archivedScrollOffset + maxVisible
	if endIdx > len(m.archivedList) {
		endIdx = len(m.archivedList)
	}

	itemIdx := clickedIdx - lineOffset
	if itemIdx >= 0 && m.archivedScrollOffset+itemIdx < endIdx {
		m.archivedCursor = m.archivedScrollOffset + itemIdx
	}

	return m, nil
}

func (m *Model) handleSidebarKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		// Detach all terminals (stop polling) but leave tmux sessions running
		for _, t := range m.terminals {
			t.Detach()
		}
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.adjustScroll()
			return m, m.switchViewToCurrentSession()
		}
		return m, nil

	case "down", "j":
		active := m.activeSessions()
		maxIdx := len(active) - 1
		if m.archivedCount() > 0 {
			maxIdx++
		}
		if m.cursor < maxIdx {
			m.cursor++
			m.adjustScroll()
			return m, m.switchViewToCurrentSession()
		}
		return m, nil

	case "enter":
		return m.handleEnter()

	case "n":
		if m.service == nil {
			return m, nil
		}
		return m.openCreateOverlay()

	case "d":
		return m.openDeleteOverlay()

	case "a":
		return m.handleArchive()

	case "p":
		m.initProjectInput()
		m.overlay = overlaySelectProject
		return m, m.loadProjects()

	case "?":
		m.overlay = overlayHelp
		return m, nil

	case "esc":
		if m.activeSession != nil {
			m.message = ""
			m.err = nil
			m.focus = focusTerminal
			m.resizeTerminalIfNeeded()
			return m, nil
		}
		return m, nil

	default:
		return m, nil
	}
}

func (m *Model) handleTerminalKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.activeSession == nil {
		return m, nil
	}

	t, ok := m.terminals[m.activeSession.Name]
	if !ok {
		return m, nil
	}

	// Check if session ended - Enter restarts
	if !t.IsRunning() {
		if msg.Type == tea.KeyEnter {
			if err := t.Respawn(true); err != nil {
				m.err = err
				return m, nil
			}
			return m, nil
		}
		return m, nil
	}

	// Page Up/Down for scrolling
	if msg.Type == tea.KeyPgUp {
		_, th := m.terminalPaneDimensions()
		t.ScrollUp(th / 2)
		return m, nil
	}
	if msg.Type == tea.KeyPgDown {
		_, th := m.terminalPaneDimensions()
		t.ScrollDown(th / 2)
		return m, nil
	}

	// Suppress SGR mouse escape sequence fragments split across TCP packets
	// (common over SSH). A fragmented \x1b[<Cb;Cx;CyM yields an ESC key
	// event followed by individual rune keys: [ < 0-9 ; M m.
	// Each MouseMsg grants a budget; we eat matching chars until consumed.
	// We intentionally do NOT reset the budget on non-matching events,
	// because ESC (KeyEscape) arrives between fragments and would clear it.
	if m.mouseFragmentBudget > 0 {
		// ESC is the leading byte of mouse sequences; eat it while in
		// scroll mode to prevent it from prematurely exiting scroll mode.
		if msg.Type == tea.KeyEscape && t.IsScrollMode() {
			m.mouseFragmentBudget--
			return m, nil
		}
		if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
			r := msg.Runes[0]
			if r == '[' || r == '<' || r == ';' || r == 'M' || r == 'm' || (r >= '0' && r <= '9') {
				m.mouseFragmentBudget--
				return m, nil
			}
		}
	}

	// Any other key exits scroll mode (don't forward to tmux —
	// prevents partially-parsed mouse escape sequences from leaking through)
	if t.IsScrollMode() {
		t.ExitScrollMode()
		return m, nil
	}

	// Send key to tmux session
	t.SendKeys(msg)
	return m, nil
}

func (m *Model) handleEnter() (tea.Model, tea.Cmd) {
	active := m.activeSessions()
	// Cursor on the archived line?
	if m.archivedCount() > 0 && m.cursor == len(active) {
		return m.openArchivedOverlay()
	}
	if m.cursor >= len(active) {
		return m, nil
	}
	return m, m.activateSession(active[m.cursor], true)
}

func (m *Model) openCreateOverlay() (tea.Model, tea.Cmd) {
	m.createInput = textinput.New()
	m.createInput.Placeholder = "Session name..."
	m.createInput.Focus()
	m.createInput.CharLimit = 100
	m.createInput.Width = 40
	m.overlay = overlayCreateSession
	m.err = nil
	return m, textinput.Blink
}

func (m *Model) openDeleteOverlay() (tea.Model, tea.Cmd) {
	active := m.activeSessions()
	if len(active) == 0 || m.cursor >= len(active) {
		return m, nil
	}
	m.selectedSession = active[m.cursor]
	m.overlay = overlayDeleteConfirm
	return m, nil
}

func (m *Model) handleArchive() (tea.Model, tea.Cmd) {
	active := m.activeSessions()
	if len(active) == 0 || m.cursor >= len(active) || m.service == nil {
		return m, nil
	}
	selected := active[m.cursor]
	return m, func() tea.Msg {
		if err := m.service.ArchiveSession(selected.Name); err != nil {
			return errMsg{err}
		}
		return sessionArchivedMsg{selected.Name}
	}
}

// --- Overlay key handlers ---

func (m *Model) handleOverlayKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.overlay {
	case overlayCreateSession:
		return m.handleCreateOverlayKeys(msg)
	case overlaySelectBaseBranch:
		return m.handleSelectBaseBranchKeys(msg)
	case overlaySelectExistingBranch:
		return m.handleSelectExistingBranchKeys(msg)
	case overlayConfirmBranchWithSession:
		return m.handleConfirmBranchWithSessionKeys(msg)
	case overlayEnterNewSessionName:
		return m.handleEnterNewSessionNameKeys(msg)
	case overlayDeleteConfirm:
		return m.handleDeleteConfirmKeys(msg)
	case overlayHelp:
		return m.handleHelpKeys(msg)
	case overlayCreating:
		return m, nil
	case overlayArchivedSessions:
		return m.handleArchivedOverlayKeys(msg)
	case overlaySelectProject:
		return m.handleSelectProjectKeys(msg)
	}
	return m, nil
}

// dismissOverlay mirrors the Esc key behavior for each overlay type.
func (m *Model) dismissOverlay() (tea.Model, tea.Cmd) {
	switch m.overlay {
	case overlayHelp:
		m.overlay = overlayNone
	case overlayCreateSession:
		m.overlay = overlayNone
		m.err = nil
	case overlayEnterNewSessionName:
		m.overlay = overlaySelectExistingBranch
		m.selectedBranchName = ""
	case overlaySelectBaseBranch:
		m.overlay = overlayCreateSession
		m.createInput.Focus()
		return m, textinput.Blink
	case overlaySelectExistingBranch:
		m.overlay = overlayCreateSession
		m.createInput.Focus()
		return m, textinput.Blink
	case overlayConfirmBranchWithSession:
		m.overlay = overlaySelectExistingBranch
		m.selectedBranchName = ""
	case overlayDeleteConfirm:
		if m.deleteFromArchived {
			m.overlay = overlayArchivedSessions
			m.deleteFromArchived = false
		} else {
			m.overlay = overlayNone
		}
		m.selectedSession = nil
	case overlayArchivedSessions:
		m.overlay = overlayNone
	case overlaySelectProject:
		if m.noProjectMode {
			// Can't dismiss project picker when launched outside a git repo
			return m, tea.Quit
		}
		m.overlay = overlayNone
	case overlayCreating:
		// Cannot dismiss while creating
		return m, nil
	}
	return m, nil
}

func (m *Model) handleCreateOverlayKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.overlay = overlayNone
		m.err = nil
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "ctrl+b":
		m.overlay = overlaySelectExistingBranch
		m.initBranchInput()
		return m, m.loadBranches()
	case "enter":
		name := strings.TrimSpace(m.createInput.Value())
		if name == "" {
			m.err = fmt.Errorf("session name cannot be empty")
			return m, nil
		}
		if err := worktree.ValidateBranchName(name); err != nil {
			m.err = fmt.Errorf("invalid session name: %w", err)
			return m, nil
		}
		m.pendingSessionName = name
		m.overlay = overlaySelectBaseBranch
		m.initBranchInput()
		return m, m.loadBranches()
	default:
		var cmd tea.Cmd
		m.createInput, cmd = m.createInput.Update(msg)
		m.err = nil
		return m, cmd
	}
}

func (m *Model) initBranchInput() {
	m.branchInput = textinput.New()
	m.branchInput.Placeholder = "Filter branches..."
	m.branchInput.Focus()
	m.branchInput.CharLimit = 100
	m.branchInput.Width = 40
	m.branchCursor = 0
	m.branchScrollOffset = 0
}

func (m *Model) handleSelectBaseBranchKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	showHead := m.showHeadOption()
	totalItems := len(m.filteredBranches)
	if showHead {
		totalItems++
	}

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.overlay = overlayCreateSession
		m.createInput.Focus()
		return m, textinput.Blink

	case "up":
		if m.branchCursor > 0 {
			m.branchCursor--
		}
		return m, nil

	case "down":
		if m.branchCursor < totalItems-1 {
			m.branchCursor++
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
		m.clampBranchCursor(totalItems)
		return m, cmd
	}
}

func (m *Model) handleSelectExistingBranchKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	totalItems := len(m.filteredBranches)

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.overlay = overlayCreateSession
		m.createInput.Focus()
		return m, textinput.Blink

	case "up":
		if m.branchCursor > 0 {
			m.branchCursor--
		}
		return m, nil

	case "down":
		if m.branchCursor < totalItems-1 {
			m.branchCursor++
		}
		return m, nil

	case "enter":
		if totalItems == 0 || m.branchCursor >= totalItems {
			return m, nil
		}
		selectedBranch := m.filteredBranches[m.branchCursor]
		if m.branchesWithSessions[selectedBranch] {
			m.selectedBranchName = selectedBranch
			m.overlay = overlayConfirmBranchWithSession
			return m, nil
		}
		m.pendingSessionName = selectedBranch
		return m, m.doCreateSession(selectedBranch, true)

	default:
		var cmd tea.Cmd
		m.branchInput, cmd = m.branchInput.Update(msg)
		m.filterBranches()
		if m.branchCursor >= len(m.filteredBranches) {
			m.branchCursor = len(m.filteredBranches) - 1
			if m.branchCursor < 0 {
				m.branchCursor = 0
			}
		}
		return m, cmd
	}
}

func (m *Model) handleConfirmBranchWithSessionKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.newSessionInput = textinput.New()
		m.newSessionInput.Placeholder = "New session name..."
		m.newSessionInput.Focus()
		m.newSessionInput.CharLimit = 100
		m.newSessionInput.Width = 40
		m.overlay = overlayEnterNewSessionName
		return m, textinput.Blink
	case "n", "N", "esc":
		m.overlay = overlaySelectExistingBranch
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
		m.overlay = overlaySelectExistingBranch
		m.selectedBranchName = ""
		return m, nil
	case "enter":
		name := strings.TrimSpace(m.newSessionInput.Value())
		if name == "" {
			m.err = fmt.Errorf("session name cannot be empty")
			return m, nil
		}
		if err := worktree.ValidateBranchName(name); err != nil {
			m.err = fmt.Errorf("invalid session name: %w", err)
			return m, nil
		}
		m.pendingSessionName = name
		return m, m.doCreateSession(m.selectedBranchName, false)
	default:
		var cmd tea.Cmd
		m.newSessionInput, cmd = m.newSessionInput.Update(msg)
		m.err = nil
		return m, cmd
	}
}

func (m *Model) handleDeleteConfirmKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		name := m.selectedSession.Name
		delete(m.settingUpSessions, name)
		// Close terminal if running
		if t, ok := m.terminals[name]; ok {
			t.Close()
			delete(m.terminals, name)
		}
		return m, func() tea.Msg {
			if m.service == nil {
				return errMsg{fmt.Errorf("no project selected")}
			}
			if err := m.service.DeleteSession(name); err != nil {
				return errMsg{err}
			}
			return sessionDeletedMsg{name}
		}
	case "n", "N", "esc":
		if m.deleteFromArchived {
			m.overlay = overlayArchivedSessions
			m.deleteFromArchived = false
		} else {
			m.overlay = overlayNone
		}
		m.selectedSession = nil
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) handleHelpKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "?", "q":
		m.overlay = overlayNone
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) doCreateSession(baseBranch string, useExisting bool) tea.Cmd {
	name := m.pendingSessionName
	m.overlay = overlayCreating

	return func() tea.Msg {
		if m.service == nil {
			return errMsg{fmt.Errorf("no project selected")}
		}
		sess, setupCmds, err := m.service.CreateSession(name, baseBranch, useExisting)
		if err != nil {
			return errMsg{err}
		}
		return sessionCreatedMsg{session: sess, setupCommands: setupCmds}
	}
}

func (m *Model) runSetupInBackground(sessionName, worktreePath string, commands []string) tea.Cmd {
	return func() tea.Msg {
		var buf bytes.Buffer
		err := worktree.RunSetupCommands(worktreePath, commands, &buf)
		return setupCompleteMsg{sessionName: sessionName, err: err}
	}
}

// --- Helper methods ---

func (m *Model) activeSessions() []*session.Session {
	var active []*session.Session
	for _, s := range m.sessions {
		if s.Status != "archived" {
			active = append(active, s)
		}
	}
	return active
}

func (m *Model) archivedCount() int {
	count := 0
	for _, s := range m.sessions {
		if s.Status == "archived" {
			count++
		}
	}
	return count
}

func (m *Model) archivedSessionsList() []*session.Session {
	var archived []*session.Session
	for _, s := range m.sessions {
		if s.Status == "archived" {
			archived = append(archived, s)
		}
	}
	return archived
}

func (m *Model) showHeadOption() bool {
	filter := strings.ToLower(m.branchInput.Value())
	return filter == "" || strings.Contains("head", filter)
}

func (m *Model) getSelectedBaseBranch(showHead bool) string {
	if showHead && m.branchCursor == 0 {
		return m.currentBranch
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

func (m *Model) clampBranchCursor(total int) {
	if m.branchCursor >= total {
		m.branchCursor = total - 1
	}
	if m.branchCursor < 0 {
		m.branchCursor = 0
	}
}

func (m *Model) filterBranches() {
	query := strings.ToLower(strings.TrimSpace(m.branchInput.Value()))

	if query == "" {
		m.filteredBranches = m.branches
	} else {
		m.filteredBranches = nil
		for _, branch := range m.branches {
			if strings.Contains(strings.ToLower(branch), query) {
				m.filteredBranches = append(m.filteredBranches, branch)
			}
		}
	}
}

func (m *Model) adjustScroll() {
	maxVisible := m.maxVisibleSessions()
	if maxVisible <= 0 {
		maxVisible = 1
	}
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	}
	if m.cursor >= m.scrollOffset+maxVisible {
		m.scrollOffset = m.cursor - maxVisible + 1
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

func (m *Model) maxVisibleSessions() int {
	// tower+blank+topborder(8) + [archived line(1)] + bottom border(1) = 10
	available := m.windowHeight - 10
	if available < 1 {
		return 1
	}
	return available // 1 line per session now
}

// sidebarHitTest maps a mouse Y coordinate to the sidebar element at that position.
// Returns a kind string and an index (meaningful only for "session").
func (m *Model) sidebarHitTest(y int) (kind string, index int) {
	y++ // empirical offset: Bubble Tea mouse Y is 1 above rendered row
	towerHeight := 8 // lines consumed by the tower + blank + top border
	if y < towerHeight {
		return "tower", 0
	}

	active := m.activeSessions()
	maxVisible := m.maxVisibleSessions()
	row := y - towerHeight // row within the bordered sidebar content

	// Account for the border top line (the border is rendered by lipgloss but
	// we removed the top border, so the first content line is at row 0 inside
	// the bordered area). Actually, since BorderTop(false) is used, row 0 is
	// the first content line.

	lineIdx := 0

	// "↑ N more" indicator
	hasScrollUp := m.scrollOffset > 0
	if hasScrollUp {
		if row == lineIdx {
			return "scroll_up", 0
		}
		lineIdx++
	}

	// Session rows
	endIdx := m.scrollOffset + maxVisible
	if endIdx > len(active) {
		endIdx = len(active)
	}
	visibleCount := endIdx - m.scrollOffset
	if row >= lineIdx && row < lineIdx+visibleCount {
		sessionIdx := m.scrollOffset + (row - lineIdx)
		return "session", sessionIdx
	}
	lineIdx += visibleCount

	// "↓ N more" indicator
	hasScrollDown := endIdx < len(active)
	if hasScrollDown {
		if row == lineIdx {
			return "scroll_down", 0
		}
		lineIdx++
	}

	// Archived sessions indicator
	if m.archivedCount() > 0 {
		if row == lineIdx {
			return "archived", 0
		}
		lineIdx++
	}

	return "empty", 0
}

// sidebarVisible returns whether the sidebar should be rendered.
// On narrow screens (< smallScreenThreshold), the sidebar is only shown when focused.
func (m *Model) sidebarVisible() bool {
	if m.windowWidth >= smallScreenThreshold {
		return true
	}
	return m.focus == focusSidebar
}

// resizeTerminalIfNeeded resizes the active terminal to match current pane dimensions.
func (m *Model) resizeTerminalIfNeeded() {
	if m.activeSession != nil {
		if t, ok := m.terminals[m.activeSession.Name]; ok {
			tw, th := m.terminalPaneDimensions()
			t.Resize(tw, th)
		}
	}
}

// terminalPaneDimensions returns the inner width/height for the terminal pane.
func (m *Model) terminalPaneDimensions() (int, int) {
	var termWidth int
	if m.sidebarVisible() {
		// sidebarWidth already includes border chars, plus 1 for spacer
		termWidth = m.windowWidth - sidebarWidth - 1
	} else {
		termWidth = m.windowWidth
	}
	if termWidth < 10 {
		termWidth = 10
	}
	termHeight := m.windowHeight // no terminal border
	if termHeight < 5 {
		termHeight = 5
	}
	return termWidth, termHeight
}

// mouseToTermCoords converts raw mouse coordinates to clamped terminal pane
// coordinates, applying empirical offsets for Bubble Tea's coordinate reporting.
func (m *Model) mouseToTermCoords(mouseX, mouseY, termStartX int) (col, row int) {
	tw, th := m.terminalPaneDimensions()
	col = mouseX - termStartX - 1 // Bubble Tea mouse X is 1 to the right
	if col < 0 {
		col = 0
	}
	if col >= tw {
		col = tw - 1
	}
	row = mouseY + 1 // Bubble Tea mouse Y is 1 above rendered row
	if row < 0 {
		row = 0
	}
	if row >= th {
		row = th - 1
	}
	return col, row
}

// --- View ---

func (m *Model) View() string {
	if m.windowWidth == 0 || m.windowHeight == 0 {
		return "Loading..."
	}

	var layout string
	if !m.sidebarVisible() {
		// Narrow screen + terminal focused: terminal only
		layout = m.viewTerminal()
	} else {
		// Sidebar visible: both panes side by side
		sidebar := m.viewSidebar()
		termPane := m.viewTerminal()
		layout = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, " ", termPane)
	}

	// Render overlay on top if active
	if m.overlay != overlayNone {
		overlayContent := m.viewOverlay()
		if overlayContent != "" {
			return m.renderOverlayOnTop(layout, overlayContent)
		}
	}

	return layout
}

func (m *Model) viewSidebar() string {
	innerWidth := sidebarWidth - 2
	if innerWidth < 1 {
		innerWidth = 1
	}

	// Focus-mapped styles: primary→textMuted, textNormal→textMuted, textMuted→textDim
	var towerStyle, atcStyle, versionStyle, repoStyle, helpKeyStyle, helpDescStyle lipgloss.Style
	if m.focus == focusSidebar {
		towerStyle = lipgloss.NewStyle().Foreground(primary)
		atcStyle = lipgloss.NewStyle().Foreground(textNormal).Bold(true)
		versionStyle = lipgloss.NewStyle().Foreground(textMuted)
		repoStyle = lipgloss.NewStyle().Foreground(primary)
		helpKeyStyle = lipgloss.NewStyle().Foreground(textNormal)
		helpDescStyle = lipgloss.NewStyle().Foreground(textMuted)
	} else {
		towerStyle = lipgloss.NewStyle().Foreground(textMuted)
		atcStyle = lipgloss.NewStyle().Foreground(textMuted)
		versionStyle = lipgloss.NewStyle().Foreground(textDim)
		repoStyle = lipgloss.NewStyle().Foreground(textMuted)
		helpKeyStyle = lipgloss.NewStyle().Foreground(textMuted)
		helpDescStyle = lipgloss.NewStyle().Foreground(textDim)
	}

	helpItem := func(key, desc string) string {
		return helpDescStyle.Render("[") + helpKeyStyle.Render(key) + helpDescStyle.Render("]") + " " + helpDescStyle.Render(desc)
	}

	// Tower with keyboard shortcuts (rendered outside the border)
	var tower strings.Builder
	pad := "    "
	tower.WriteString("\n")
	tower.WriteString("  " + towerStyle.Render("__\\-----/__") + pad + helpItem("^C", "back to sidebar") + "\n")
	tower.WriteString("  " + towerStyle.Render("\\         /") + pad + helpItem("n", " new session") + "\n")
	tower.WriteString("  " + towerStyle.Render(" \\  ") + atcStyle.Render("ATC") + towerStyle.Render("  /") + pad + " " + helpItem("a", " archive") + "\n")
	tower.WriteString("  " + towerStyle.Render("  \\  _  /") + pad + "  " + helpItem("?", " help") + "\n")
	tower.WriteString("  " + towerStyle.Render("   |   |") + pad + "   " + versionStyle.Render(Version) + "\n")
	tower.WriteString("\n")

	// Top border with embedded repo name
	var borderColor lipgloss.Color
	if m.focus == focusSidebar {
		borderColor = primary
	} else {
		borderColor = textDim
	}
	borderStyle := lipgloss.NewStyle().Foreground(borderColor)
	repoName := m.repoName
	if repoName == "" {
		repoName = "ATC"
	}
	repoName = truncate(repoName, innerWidth-2)
	fillLen := innerWidth - 2 - len(repoName) // innerWidth minus spaces and name
	if fillLen < 0 {
		fillLen = 0
	}
	tower.WriteString(borderStyle.Render("┌") + " " + repoStyle.Render(repoName) + " " + borderStyle.Render(strings.Repeat("─", fillLen)+"┐") + "\n")

	// Sidebar content (inside the border)
	var b strings.Builder

	// Session list
	filtered := m.activeSessions()
	maxVisible := m.maxVisibleSessions()

	if len(filtered) == 0 && m.archivedCount() == 0 {
		b.WriteString(metadataStyle.Render("No sessions") + "\n")
	} else {
		endIdx := m.scrollOffset + maxVisible
		if endIdx > len(filtered) {
			endIdx = len(filtered)
		}

		if m.scrollOffset > 0 {
			b.WriteString(metadataStyle.Render(fmt.Sprintf("  ↑ %d more", m.scrollOffset)) + "\n")
		}

		for i := m.scrollOffset; i < endIdx; i++ {
			s := filtered[i]
			m.renderSidebarSession(&b, s, i, innerWidth)
		}

		if endIdx < len(filtered) {
			b.WriteString(metadataStyle.Render(fmt.Sprintf("  ↓ %d more", len(filtered)-endIdx)) + "\n")
		}
	}

	// Archived sessions indicator
	if archivedN := m.archivedCount(); archivedN > 0 {
		label := fmt.Sprintf("(%d archived)", archivedN)
		isOnArchived := m.cursor == len(filtered)
		if isOnArchived {
			b.WriteString(lipgloss.NewStyle().
				Background(textMuted).
				Foreground(lipgloss.Color("#000000")).
				Bold(true).
				Width(innerWidth).
				Render(" "+label) + "\n")
		} else if m.focus == focusSidebar {
			b.WriteString(lipgloss.NewStyle().Foreground(textMuted).Render(" "+label) + "\n")
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(textDim).Render(" "+label) + "\n")
		}
	}

	// Fill remaining space
	towerHeight := 8 // 6 tower lines + 1 blank line + 1 custom top border
	sidebarHeight := m.windowHeight - towerHeight - 1 // minus tower, minus bottom border only
	if sidebarHeight < 1 {
		sidebarHeight = 1
	}

	// Reserve lines for status bar if needed (divider + message = 2 lines)
	statusLines := 0
	if m.err != nil || m.message != "" {
		statusLines = 2
	}

	contentLines := strings.Count(b.String(), "\n")
	targetLines := sidebarHeight - statusLines
	if targetLines < contentLines {
		targetLines = contentLines
	}
	for contentLines < targetLines {
		b.WriteString("\n")
		contentLines++
	}

	// Status bar (errors/messages only)
	if m.err != nil {
		b.WriteString(dividerStyle.Render(strings.Repeat("─", innerWidth)) + "\n")
		b.WriteString(errorStyle.Render(truncate(m.err.Error(), innerWidth)) + "\n")
	} else if m.message != "" {
		b.WriteString(dividerStyle.Render(strings.Repeat("─", innerWidth)) + "\n")
		b.WriteString(successStyle.Render(truncate(m.message, innerWidth)) + "\n")
	}

	style := sidebarUnfocusedStyle.BorderTop(false)
	if m.focus == focusSidebar {
		style = sidebarFocusedStyle.BorderTop(false)
	}

	bordered := style.
		Width(sidebarWidth - 2).
		Height(sidebarHeight).
		Render(b.String())

	return tower.String() + bordered
}

func (m *Model) renderSidebarSession(b *strings.Builder, s *session.Session, idx int, maxWidth int) {
	isSelected := m.cursor == idx
	isSettingUp := m.settingUpSessions[s.Name]

	prefix := " "
	if isSettingUp {
		prefix = " " + m.spinner.View() + " "
	}
	name := truncate(s.Name, maxWidth-lipgloss.Width(prefix)-1)

	var style lipgloss.Style
	if m.focus == focusSidebar {
		if isSelected {
			style = sidebarSessionSelectedStyle.Width(maxWidth)
		} else {
			style = sidebarSessionStyle
		}
	} else {
		if isSelected {
			style = sidebarSessionDimSelectedStyle.Width(maxWidth)
		} else {
			style = sidebarSessionDimStyle
		}
	}
	b.WriteString(style.Render(prefix+name) + "\n")
}

func (m *Model) viewTerminal() string {
	tw, th := m.terminalPaneDimensions()

	// tmux capture-pane output is already at the correct dimensions.

	if m.activeSession != nil {
		if t, ok := m.terminals[m.activeSession.Name]; ok {
			var rendered string
			if !t.IsRunning() {
				rendered = t.Render() + "\n\n  Session ended. Press Enter to restart."
			} else {
				rendered = t.Render()
			}

			// Overlay scroll indicator when in scroll mode
			scrollPos := t.ScrollPosition()
			if scrollPos > 0 {
				indicator := scrollIndicatorStyle.Render(fmt.Sprintf(" SCROLL -%d ", scrollPos))
				lines := strings.Split(rendered, "\n")
				if len(lines) > 0 {
					indicatorW := lipgloss.Width(indicator)
					padLen := tw - indicatorW
					if padLen < 0 {
						padLen = 0
					}
					lines[0] = strings.Repeat(" ", padLen) + indicator
				}
				rendered = strings.Join(lines, "\n")
			}

			// Apply selection highlight
			if m.hasSelection || m.selecting {
				rendered = m.applySelectionHighlight(rendered)
			}

			// Dim terminal content when sidebar is focused
			if m.focus == focusSidebar {
				rendered = dimANSIColors(rendered, 0.4)
			}

			return rendered
		}
	}

	// Placeholder content — use lipgloss to fill the pane
	var content string
	if m.activeSession == nil {
		var placeholder string
		if m.noProjectMode {
			placeholder = placeholderStyle.Render("Press 'p' to select a project")
		} else {
			placeholder = placeholderStyle.Render("Select a session or press 'n' to create one")
		}
		content = "\n\n" + centerText(placeholder, tw)
	} else {
		placeholder := placeholderStyle.Render("Press Enter to start session")
		content = "\n\n" + centerText(placeholder, tw)
	}

	return lipgloss.NewStyle().
		Width(tw).
		Height(th).
		Render(content)
}

func (m *Model) viewOverlay() string {
	switch m.overlay {
	case overlayCreateSession:
		return m.viewCreateOverlay()
	case overlaySelectBaseBranch:
		return m.viewSelectBaseBranch()
	case overlaySelectExistingBranch:
		return m.viewSelectExistingBranch()
	case overlayConfirmBranchWithSession:
		return m.viewConfirmBranchWithSession()
	case overlayEnterNewSessionName:
		return m.viewEnterNewSessionName()
	case overlayDeleteConfirm:
		return m.viewDeleteOverlay()
	case overlayHelp:
		return m.viewHelpOverlay()
	case overlayCreating:
		return m.viewCreatingOverlay()
	case overlayArchivedSessions:
		return m.viewArchivedOverlay()
	case overlaySelectProject:
		return m.viewSelectProject()
	}
	return ""
}

func (m *Model) viewCreateOverlay() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("New Session"))
	b.WriteString("\n\n")
	b.WriteString(dialogTextStyle.Render("Session name:"))
	b.WriteString("\n")
	b.WriteString(m.createInput.View())
	b.WriteString("\n")
	if m.err != nil {
		b.WriteString("\n" + errorStyle.Render(m.err.Error()))
	}
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("[Enter] Next  [^B] From branch  [Esc] Cancel"))
	return dialogBoxStyle.Render(b.String())
}

func (m *Model) viewSelectBaseBranch() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("Creating \"%s\"", m.pendingSessionName)))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("Select base branch:"))
	b.WriteString("\n\n")
	b.WriteString(m.branchInput.View())
	b.WriteString("\n\n")

	showHead := m.showHeadOption()

	maxVisible := 10
	startIdx := 0
	cursorOffset := 0
	if showHead {
		cursorOffset = 1
	}
	branchIdx := m.branchCursor - cursorOffset
	if branchIdx >= startIdx+maxVisible {
		startIdx = branchIdx - maxVisible + 1
	}
	if branchIdx < startIdx && branchIdx >= 0 {
		startIdx = branchIdx
	}
	endIdx := startIdx + maxVisible
	if endIdx > len(m.filteredBranches) {
		endIdx = len(m.filteredBranches)
	}

	// Compute max item width for full-width highlight (match widest dialog element)
	helpText := "[↑/↓] Navigate  [Enter] Select  [Esc] Back"
	itemWidth := len(helpText)
	if showHead {
		w := len(fmt.Sprintf("HEAD (%s)", m.currentBranch))
		if w > itemWidth {
			itemWidth = w
		}
	}
	for i := startIdx; i < endIdx; i++ {
		if len(m.filteredBranches[i]) > itemWidth {
			itemWidth = len(m.filteredBranches[i])
		}
	}

	if showHead {
		headLabel := fmt.Sprintf("HEAD (%s)", m.currentBranch)
		if m.branchCursor == 0 {
			b.WriteString(selectedItemStyle.Width(itemWidth).Render(headLabel) + "\n")
		} else {
			b.WriteString(normalItemStyle.Width(itemWidth).Render(headLabel) + "\n")
		}
	}

	if startIdx > 0 {
		b.WriteString(metadataStyle.Render("  ↑ "+fmt.Sprintf("%d more", startIdx)) + "\n")
	}
	for i := startIdx; i < endIdx; i++ {
		branch := m.filteredBranches[i]
		pos := i + cursorOffset
		if m.branchCursor == pos {
			b.WriteString(selectedItemStyle.Width(itemWidth).Render(branch) + "\n")
		} else {
			b.WriteString(normalItemStyle.Width(itemWidth).Render(branch) + "\n")
		}
	}
	if endIdx < len(m.filteredBranches) {
		b.WriteString(metadataStyle.Render(fmt.Sprintf("  ↓ %d more", len(m.filteredBranches)-endIdx)) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render(helpText))
	return dialogBoxStyle.Render(b.String())
}

func (m *Model) viewSelectExistingBranch() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("From existing branch"))
	b.WriteString("\n\n")
	b.WriteString(m.branchInput.View())
	b.WriteString("\n\n")

	maxVisible := 10
	startIdx := 0
	if m.branchCursor >= startIdx+maxVisible {
		startIdx = m.branchCursor - maxVisible + 1
	}
	endIdx := startIdx + maxVisible
	if endIdx > len(m.filteredBranches) {
		endIdx = len(m.filteredBranches)
	}

	if len(m.filteredBranches) == 0 {
		b.WriteString(metadataStyle.Render("  No branches match filter") + "\n")
	} else {
		// Compute max item width for full-width highlight (match widest dialog element)
		helpText := "[↑/↓] Navigate  [Enter] Select  [Esc] Back  + has session"
		itemWidth := len(helpText)
		for i := startIdx; i < endIdx; i++ {
			// Reserve space for " +" suffix on branches with sessions
			w := len(m.filteredBranches[i]) + 2
			if w > itemWidth {
				itemWidth = w
			}
		}

		if startIdx > 0 {
			b.WriteString(metadataStyle.Render("  ↑ "+fmt.Sprintf("%d more", startIdx)) + "\n")
		}
		for i := startIdx; i < endIdx; i++ {
			branch := m.filteredBranches[i]
			hasSession := m.branchesWithSessions[branch]
			displayName := branch
			if hasSession {
				// itemWidth includes style padding (1 left + 1 right), so content area is itemWidth-2
				contentWidth := itemWidth - 2
				pad := contentWidth - len(branch) - 1
				if pad < 1 {
					pad = 1
				}
				displayName = branch + strings.Repeat(" ", pad) + "+"
			}
			if m.branchCursor == i {
				b.WriteString(selectedItemStyle.Width(itemWidth).Render(displayName) + "\n")
			} else {
				b.WriteString(normalItemStyle.Width(itemWidth).Render(displayName) + "\n")
			}
		}
		if endIdx < len(m.filteredBranches) {
			b.WriteString(metadataStyle.Render("  ↓ "+fmt.Sprintf("%d more", len(m.filteredBranches)-endIdx)) + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("[↑/↓] Navigate  [Enter] Select  [Esc] Back  + has session"))
	return dialogBoxStyle.Render(b.String())
}

func (m *Model) viewConfirmBranchWithSession() string {
	var b strings.Builder
	b.WriteString(dialogTitleStyle.Render("Branch Has Existing Session"))
	b.WriteString("\n\n")
	b.WriteString(dialogTextStyle.Render(fmt.Sprintf("Branch \"%s\" already has a session.", m.selectedBranchName)))
	b.WriteString("\n\n")
	b.WriteString(dialogTextStyle.Render("Create a new session branching from it?"))
	b.WriteString("\n\n")
	b.WriteString(dialogTextStyle.Render("[Y] Yes    [N] Cancel"))
	return dialogBoxStyle.Render(b.String())
}

func (m *Model) viewEnterNewSessionName() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("New session from \"%s\"", m.selectedBranchName)))
	b.WriteString("\n\n")
	b.WriteString(dialogTextStyle.Render("Session name:"))
	b.WriteString("\n")
	b.WriteString(m.newSessionInput.View())
	if m.err != nil {
		b.WriteString("\n\n" + errorStyle.Render(m.err.Error()))
	}
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("[Enter] Create  [Esc] Back"))
	return dialogBoxStyle.Render(b.String())
}

func (m *Model) viewDeleteOverlay() string {
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
	b.WriteString(dialogTextStyle.Render("  - Kill the Claude process (if running)"))
	b.WriteString("\n")
	b.WriteString(dialogTextStyle.Render("  - Remove the git worktree"))
	b.WriteString("\n")
	b.WriteString(dialogTextStyle.Render("  - Delete all local changes"))
	b.WriteString("\n\n")
	b.WriteString(warningStyle.Render("This cannot be undone."))
	b.WriteString("\n\n")
	b.WriteString(dialogTextStyle.Render("[Y] Yes, delete    [N] Cancel"))
	return dialogBoxStyle.Render(b.String())
}

func (m *Model) viewHelpOverlay() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Keyboard Shortcuts"))
	b.WriteString("\n\n")
	b.WriteString(dialogTextStyle.Render("Sidebar:"))
	b.WriteString("\n")
	b.WriteString(dialogTextStyle.Render("  j/k or ↑/↓  Navigate sessions"))
	b.WriteString("\n")
	b.WriteString(dialogTextStyle.Render("  Enter        Start/resume session"))
	b.WriteString("\n")
	b.WriteString(dialogTextStyle.Render("  n            New session"))
	b.WriteString("\n")
	b.WriteString(dialogTextStyle.Render("  d            Delete session"))
	b.WriteString("\n")
	b.WriteString(dialogTextStyle.Render("  a            Archive session"))
	b.WriteString("\n")
	b.WriteString(dialogTextStyle.Render("  p            Switch project"))
	b.WriteString("\n")
	b.WriteString(dialogTextStyle.Render("  q            Quit ATC"))
	b.WriteString("\n\n")
	b.WriteString(dialogTextStyle.Render("Terminal:"))
	b.WriteString("\n")
	b.WriteString(dialogTextStyle.Render("  All keys forwarded to Claude"))
	b.WriteString("\n")
	b.WriteString(dialogTextStyle.Render("  Scroll/PgUp  Scroll up (enter scroll mode)"))
	b.WriteString("\n")
	b.WriteString(dialogTextStyle.Render("  Scroll/PgDn  Scroll down (any key exits)"))
	b.WriteString("\n")
	b.WriteString(dialogTextStyle.Render("  Click+drag   Select text (copies to clipboard)"))
	b.WriteString("\n\n")
	b.WriteString(dialogTextStyle.Render("Global:"))
	b.WriteString("\n")
	b.WriteString(dialogTextStyle.Render("  Ctrl+C       Back to sidebar (from terminal)"))
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("Press Esc or ? to close"))
	return dialogBoxStyle.Render(b.String())
}

func (m *Model) viewCreatingOverlay() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Creating Session"))
	b.WriteString("\n\n")
	b.WriteString(m.spinner.View() + " Creating \"" + m.pendingSessionName + "\"...")
	return dialogBoxStyle.Render(b.String())
}

// --- Archived sessions overlay ---

func (m *Model) openArchivedOverlay() (tea.Model, tea.Cmd) {
	m.archivedList = m.archivedSessionsList()
	m.archivedCursor = 0
	m.archivedScrollOffset = 0
	m.overlay = overlayArchivedSessions
	return m, nil
}

func (m *Model) handleArchivedOverlayKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.archivedCursor > 0 {
			m.archivedCursor--
			// Adjust scroll
			if m.archivedCursor < m.archivedScrollOffset {
				m.archivedScrollOffset = m.archivedCursor
			}
		}
		return m, nil

	case "down", "j":
		if m.archivedCursor < len(m.archivedList)-1 {
			m.archivedCursor++
			// Adjust scroll
			maxVisible := 10
			if m.archivedCursor >= m.archivedScrollOffset+maxVisible {
				m.archivedScrollOffset = m.archivedCursor - maxVisible + 1
			}
		}
		return m, nil

	case "u":
		if len(m.archivedList) == 0 || m.archivedCursor >= len(m.archivedList) || m.service == nil {
			return m, nil
		}
		selected := m.archivedList[m.archivedCursor]
		return m, func() tea.Msg {
			if err := m.service.UnarchiveSession(selected.Name); err != nil {
				return errMsg{err}
			}
			return sessionUnarchivedMsg{selected.Name}
		}

	case "d":
		if len(m.archivedList) == 0 || m.archivedCursor >= len(m.archivedList) {
			return m, nil
		}
		m.selectedSession = m.archivedList[m.archivedCursor]
		m.deleteFromArchived = true
		m.overlay = overlayDeleteConfirm
		return m, nil

	case "esc":
		m.overlay = overlayNone
		return m, nil

	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) viewArchivedOverlay() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Archived Sessions"))
	b.WriteString("\n\n")

	if len(m.archivedList) == 0 {
		b.WriteString(metadataStyle.Render("No archived sessions") + "\n")
	} else {
		maxVisible := 10
		endIdx := m.archivedScrollOffset + maxVisible
		if endIdx > len(m.archivedList) {
			endIdx = len(m.archivedList)
		}

		if m.archivedScrollOffset > 0 {
			b.WriteString(metadataStyle.Render(fmt.Sprintf("  ↑ %d more", m.archivedScrollOffset)) + "\n")
		}

		// Compute max item width for full-width highlight (match widest dialog element)
		helpText := "[↑/↓] Navigate  [u] Unarchive  [d] Delete  [Esc] Close"
		itemWidth := len(helpText)
		for i := m.archivedScrollOffset; i < endIdx; i++ {
			if len(m.archivedList[i].Name) > itemWidth {
				itemWidth = len(m.archivedList[i].Name)
			}
		}

		for i := m.archivedScrollOffset; i < endIdx; i++ {
			s := m.archivedList[i]
			if i == m.archivedCursor {
				b.WriteString(selectedItemStyle.Width(itemWidth).Render(s.Name) + "\n")
			} else {
				b.WriteString(normalItemStyle.Width(itemWidth).Render(s.Name) + "\n")
			}
		}

		if endIdx < len(m.archivedList) {
			b.WriteString(metadataStyle.Render(fmt.Sprintf("  ↓ %d more", len(m.archivedList)-endIdx)) + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("[↑/↓] Navigate  [u] Unarchive  [d] Delete  [Esc] Close"))
	return dialogBoxStyle.Render(b.String())
}

// --- View switching ---

func (m *Model) switchViewToCurrentSession() tea.Cmd {
	active := m.activeSessions()
	if m.cursor >= 0 && m.cursor < len(active) {
		sess := active[m.cursor]
		m.activeSession = sess
		if t, ok := m.terminals[sess.Name]; ok {
			// Terminal exists (running or stopped) — just resize if running
			if t.IsRunning() {
				tw, th := m.terminalPaneDimensions()
				t.Resize(tw, th)
			}
			return nil
		}
		// No terminal exists — auto-activate unless activation is already in flight
		if m.activatingSession == sess.Name {
			return nil
		}
		return m.activateSession(sess, false)
	}
	// If cursor is on the archived line, don't change activeSession
	return nil
}

// overlayBounds computes the centered position and dimensions of the current overlay.
func (m *Model) overlayBounds() (startRow, startCol, height, width int) {
	overlayStr := m.viewOverlay()
	if overlayStr == "" {
		return 0, 0, 0, 0
	}
	olLines := strings.Split(overlayStr, "\n")
	height = len(olLines)
	for _, line := range olLines {
		w := lipgloss.Width(line)
		if w > width {
			width = w
		}
	}
	startRow = (m.windowHeight - height) / 2
	startCol = (m.windowWidth - width) / 2
	if startRow < 0 {
		startRow = 0
	}
	if startCol < 0 {
		startCol = 0
	}
	return
}

// isInsideOverlay returns true if the given screen coordinates fall within the overlay bounds.
func (m *Model) isInsideOverlay(x, y int) bool {
	startRow, startCol, height, width := m.overlayBounds()
	return x >= startCol && x < startCol+width && y >= startRow && y < startRow+height
}

// renderOverlayOnTop centers the overlay on top of the background
func (m *Model) renderOverlayOnTop(background, overlayStr string) string {
	bgLines := strings.Split(background, "\n")
	olLines := strings.Split(overlayStr, "\n")

	olWidth := 0
	for _, line := range olLines {
		w := lipgloss.Width(line)
		if w > olWidth {
			olWidth = w
		}
	}

	startRow := (m.windowHeight - len(olLines)) / 2
	startCol := (m.windowWidth - olWidth) / 2
	if startRow < 0 {
		startRow = 0
	}
	if startCol < 0 {
		startCol = 0
	}

	for len(bgLines) < m.windowHeight {
		bgLines = append(bgLines, "")
	}

	for i, olLine := range olLines {
		row := startRow + i
		if row >= len(bgLines) {
			break
		}
		// Preserve background content on both sides of the overlay
		leftBg := truncateAnsi(bgLines[row], startCol)
		// Pad left side if background is shorter than startCol
		leftWidth := lipgloss.Width(leftBg)
		if leftWidth < startCol {
			leftBg += strings.Repeat(" ", startCol-leftWidth)
		}
		// Pad overlay line to consistent width
		olVisWidth := lipgloss.Width(olLine)
		paddedOl := olLine
		if olVisWidth < olWidth {
			paddedOl += strings.Repeat(" ", olWidth-olVisWidth)
		}
		// Get background content to the right of the overlay
		rightBg := skipAnsi(bgLines[row], startCol+olWidth)
		bgLines[row] = leftBg + paddedOl + rightBg
	}

	return strings.Join(bgLines[:m.windowHeight], "\n")
}

// --- Utility ---

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func centerText(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	pad := (width - w) / 2
	return strings.Repeat(" ", pad) + s
}

func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// ansiEscapeEnd returns the byte index just past the ANSI escape sequence
// starting at s[i] (where s[i] == '\x1b'). Handles CSI (\x1b[...X) and
// charset (\x1b(X) sequences.
func ansiEscapeEnd(s string, i int) int {
	j := i + 1
	if j >= len(s) {
		return j
	}
	if s[j] == '[' {
		j++
		for j < len(s) && !((s[j] >= 'A' && s[j] <= 'Z') || (s[j] >= 'a' && s[j] <= 'z')) {
			j++
		}
		if j < len(s) {
			j++
		}
	} else if s[j] == '(' {
		j += 2
		if j > len(s) {
			j = len(s)
		}
	}
	return j
}

// truncateAnsi returns the first maxWidth visible characters of s,
// preserving any ANSI escape sequences encountered along the way.
func truncateAnsi(s string, maxWidth int) string {
	var result strings.Builder
	visCol := 0
	i := 0
	for i < len(s) && visCol < maxWidth {
		if s[i] == '\x1b' && i+1 < len(s) {
			j := ansiEscapeEnd(s, i)
			result.WriteString(s[i:j])
			i = j
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		result.WriteString(s[i : i+size])
		i += size
		visCol++
	}
	return result.String()
}

// skipAnsi skips past the first skip visible characters in s and returns
// the remainder, including any ANSI sequences that appear after the skip point.
func skipAnsi(s string, skip int) string {
	visCol := 0
	i := 0
	for i < len(s) && visCol < skip {
		if s[i] == '\x1b' && i+1 < len(s) {
			i = ansiEscapeEnd(s, i)
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
		visCol++
	}
	return s[i:]
}

// --- Text selection ---

// normalizedSelection returns selection coordinates with start before end.
func (m *Model) normalizedSelection() (startRow, startCol, endRow, endCol int) {
	startRow, startCol = m.selStartRow, m.selStartCol
	endRow, endCol = m.selEndRow, m.selEndCol
	if startRow > endRow || (startRow == endRow && startCol > endCol) {
		startRow, startCol, endRow, endCol = endRow, endCol, startRow, startCol
	}
	return
}

const selectionLightenFactor = 0.35

// applySelectionHighlight overlays a lightened highlight on the selected text region.
func (m *Model) applySelectionHighlight(content string) string {
	lines := strings.Split(content, "\n")
	startRow, startCol, endRow, endCol := m.normalizedSelection()
	termWidth, _ := m.terminalPaneDimensions()

	for i := startRow; i <= endRow && i < len(lines); i++ {
		if i < 0 {
			continue
		}
		lsc := 0
		lec := -1 // will be set below

		if i == startRow {
			lsc = startCol
		}
		if i == endRow {
			lec = endCol
		} else {
			// Non-last line — extend highlight to full terminal width.
			lec = termWidth - 1
		}
		if lec < lsc {
			continue
		}

		lines[i] = applyHighlightToLine(lines[i], lsc, lec, selectionLightenFactor)
	}

	return strings.Join(lines, "\n")
}

// getSelectedText returns the plain text of the current selection.
func (m *Model) getSelectedText() string {
	if m.activeSession == nil {
		return ""
	}
	t, ok := m.terminals[m.activeSession.Name]
	if !ok {
		return ""
	}

	content := t.Render()
	lines := strings.Split(content, "\n")
	startRow, startCol, endRow, endCol := m.normalizedSelection()

	var sb strings.Builder
	for i := startRow; i <= endRow && i < len(lines); i++ {
		if i < 0 {
			continue
		}
		stripped := stripANSI(lines[i])
		runes := []rune(stripped)

		lsc := 0
		lec := len(runes)
		if i == startRow {
			lsc = startCol
		}
		if i == endRow {
			lec = endCol + 1
		}
		if lsc > len(runes) {
			lsc = len(runes)
		}
		if lec > len(runes) {
			lec = len(runes)
		}
		if lsc > lec {
			continue
		}

		sb.WriteString(string(runes[lsc:lec]))
		if i < endRow {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// handleProjectOverlayClick handles clicks inside the project selection overlay.
func (m *Model) handleProjectOverlayClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if len(m.filteredProjects) == 0 {
		return m, nil
	}

	startRow, _, _, _ := m.overlayBounds()
	// border + padding + title + blank + input + blank = 6 lines before list
	listStart := startRow + 6

	clickedIdx := msg.Y - listStart
	if clickedIdx < 0 {
		return m, nil
	}

	lineOffset := 0
	if m.projectScrollOffset > 0 {
		if clickedIdx == 0 {
			if m.projectCursor > 0 {
				m.projectCursor--
				if m.projectCursor < m.projectScrollOffset {
					m.projectScrollOffset = m.projectCursor
				}
			}
			return m, nil
		}
		lineOffset = 1
	}

	maxVisible := 10
	endIdx := m.projectScrollOffset + maxVisible
	if endIdx > len(m.filteredProjects) {
		endIdx = len(m.filteredProjects)
	}

	itemIdx := clickedIdx - lineOffset
	if itemIdx >= 0 && m.projectScrollOffset+itemIdx < endIdx {
		m.projectCursor = m.projectScrollOffset + itemIdx
	}

	return m, nil
}

// --- Project selection overlay ---

func (m *Model) initProjectInput() {
	m.projectInput = textinput.New()
	m.projectInput.Placeholder = "Filter projects..."
	m.projectInput.Focus()
	m.projectInput.CharLimit = 100
	m.projectInput.Width = 40
	m.projectCursor = 0
	m.projectScrollOffset = 0
}

func (m *Model) filterProjects() {
	query := strings.ToLower(strings.TrimSpace(m.projectInput.Value()))
	if query == "" {
		m.filteredProjects = m.projects
	} else {
		m.filteredProjects = nil
		for _, p := range m.projects {
			if strings.Contains(strings.ToLower(p.RepoName), query) || strings.Contains(strings.ToLower(p.RepoPath), query) {
				m.filteredProjects = append(m.filteredProjects, p)
			}
		}
	}
}

func (m *Model) handleSelectProjectKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	totalItems := len(m.filteredProjects)

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		return m.dismissOverlay()

	case "up", "k":
		if m.projectCursor > 0 {
			m.projectCursor--
			if m.projectCursor < m.projectScrollOffset {
				m.projectScrollOffset = m.projectCursor
			}
		}
		return m, nil

	case "down", "j":
		if m.projectCursor < totalItems-1 {
			m.projectCursor++
			maxVisible := 10
			if m.projectCursor >= m.projectScrollOffset+maxVisible {
				m.projectScrollOffset = m.projectCursor - maxVisible + 1
			}
		}
		return m, nil

	case "enter":
		if totalItems == 0 || m.projectCursor >= totalItems {
			return m, nil
		}
		selected := m.filteredProjects[m.projectCursor]
		// Check if selecting the current project (no-op)
		if m.service != nil && m.service.RepoPath() == selected.RepoPath {
			m.overlay = overlayNone
			return m, nil
		}
		// Detach all terminals before switching
		for name, t := range m.terminals {
			t.Detach()
			delete(m.terminals, name)
		}
		return m, m.switchProject(selected)

	default:
		var cmd tea.Cmd
		m.projectInput, cmd = m.projectInput.Update(msg)
		m.filterProjects()
		if m.projectCursor >= len(m.filteredProjects) {
			m.projectCursor = len(m.filteredProjects) - 1
			if m.projectCursor < 0 {
				m.projectCursor = 0
			}
		}
		m.projectScrollOffset = 0
		return m, cmd
	}
}

func (m *Model) viewSelectProject() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Switch Project"))
	b.WriteString("\n\n")
	b.WriteString(m.projectInput.View())
	b.WriteString("\n\n")

	if len(m.filteredProjects) == 0 {
		if len(m.projects) == 0 {
			b.WriteString(metadataStyle.Render("  No projects found") + "\n")
		} else {
			b.WriteString(metadataStyle.Render("  No projects match filter") + "\n")
		}
	} else {
		maxVisible := 10
		startIdx := m.projectScrollOffset
		endIdx := startIdx + maxVisible
		if endIdx > len(m.filteredProjects) {
			endIdx = len(m.filteredProjects)
		}

		// Compute max item width
		helpText := "[↑/↓] Navigate  [Enter] Select  [Esc] Cancel"
		if m.noProjectMode {
			helpText = "[↑/↓] Navigate  [Enter] Select  [Esc] Quit"
		}
		itemWidth := len(helpText)

		// Check for duplicate repo names to decide if we need path disambiguation
		nameCount := make(map[string]int)
		for _, p := range m.filteredProjects {
			nameCount[p.RepoName]++
		}

		for i := startIdx; i < endIdx; i++ {
			p := m.filteredProjects[i]
			label := p.RepoName
			if nameCount[p.RepoName] > 1 {
				label = fmt.Sprintf("%s (%s)", p.RepoName, truncatePath(p.RepoPath, 30))
			}
			if m.service != nil && m.service.RepoPath() == p.RepoPath {
				label += " (current)"
			}
			if len(label) > itemWidth {
				itemWidth = len(label)
			}
		}

		if startIdx > 0 {
			b.WriteString(metadataStyle.Render(fmt.Sprintf("  ↑ %d more", startIdx)) + "\n")
		}
		for i := startIdx; i < endIdx; i++ {
			p := m.filteredProjects[i]
			label := p.RepoName
			if nameCount[p.RepoName] > 1 {
				label = fmt.Sprintf("%s (%s)", p.RepoName, truncatePath(p.RepoPath, 30))
			}
			if m.service != nil && m.service.RepoPath() == p.RepoPath {
				label += " (current)"
			}
			if m.projectCursor == i {
				b.WriteString(selectedItemStyle.Width(itemWidth).Render(label) + "\n")
			} else {
				b.WriteString(normalItemStyle.Width(itemWidth).Render(label) + "\n")
			}
		}
		if endIdx < len(m.filteredProjects) {
			b.WriteString(metadataStyle.Render(fmt.Sprintf("  ↓ %d more", len(m.filteredProjects)-endIdx)) + "\n")
		}
	}

	b.WriteString("\n")
	if m.noProjectMode {
		b.WriteString(helpStyle.Render("[↑/↓] Navigate  [Enter] Select  [Esc] Quit"))
	} else {
		b.WriteString(helpStyle.Render("[↑/↓] Navigate  [Enter] Select  [Esc] Cancel"))
	}
	return dialogBoxStyle.Render(b.String())
}

// truncatePath shortens a path for display, keeping the last components
func truncatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	return "..." + path[len(path)-maxLen+3:]
}

// copySelectionToClipboard copies the selected text to the system clipboard.
func (m *Model) copySelectionToClipboard() {
	text := m.getSelectedText()
	if text == "" {
		return
	}
	// Use OSC 52 escape sequence to set the system clipboard.
	// This works over SSH because the escape sequence is interpreted
	// by the local terminal emulator, not the remote host.
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	fmt.Fprintf(os.Stderr, "\x1b]52;c;%s\x07", encoded)
}
