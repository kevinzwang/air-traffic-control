package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Layout constants
const (
	sidebarWidth         = 36
	smallScreenThreshold = 100
)

var (
	// Color palette
	// Non-monochrome: primary, success, danger
	// Monochrome: textNormal, textMuted, textDim
	//
	// Focus mapping (sidebar focused → unfocused):
	//   primary    → textMuted
	//   textNormal → textMuted
	//   textMuted  → textDim
	primary    = lipgloss.Color("#00d4ff") // Cyan
	success    = lipgloss.Color("#00ff87") // Green
	danger     = lipgloss.Color("#ff5f5f") // Red
	textNormal = lipgloss.Color("#e4e4e4") // Light gray
	textMuted  = lipgloss.Color("#6c757d") // Gray
	textDim    = lipgloss.Color("#495057") // Dark gray

	// --- Sidebar styles ---

	sidebarFocusedStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(primary)

	sidebarUnfocusedStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(textDim)

	// Sidebar session list (focused)
	sidebarSessionStyle = lipgloss.NewStyle().
				Foreground(textNormal)

	sidebarSessionSelectedStyle = lipgloss.NewStyle().
					Background(primary).
					Foreground(lipgloss.Color("#000000")).
					Bold(true)

	// Sidebar session list (unfocused)
	sidebarSessionDimStyle = lipgloss.NewStyle().
				Foreground(textMuted)

	sidebarSessionDimSelectedStyle = lipgloss.NewStyle().
					Background(textDim).
					Foreground(lipgloss.Color("#000000")).
					Bold(true)

	// --- Dialog styles ---

	dialogBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(primary).
			Padding(1, 2)

	dialogTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(danger)

	dialogTextStyle = lipgloss.NewStyle().
			Foreground(textNormal)

	warningStyle = lipgloss.NewStyle().
			Foreground(danger)

	// Dialog list items
	selectedItemStyle = lipgloss.NewStyle().
				Background(primary).
				Foreground(lipgloss.Color("#000000")).
				Bold(true).
				PaddingLeft(1).
				PaddingRight(1)

	normalItemStyle = lipgloss.NewStyle().
			Foreground(textNormal).
			PaddingLeft(1).
			PaddingRight(1)

	// --- General text styles ---

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primary)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(textMuted)

	metadataStyle = lipgloss.NewStyle().
			Foreground(textMuted)

	helpStyle = lipgloss.NewStyle().
			Foreground(textMuted)

	dividerStyle = lipgloss.NewStyle().
			Foreground(textDim)

	placeholderStyle = lipgloss.NewStyle().
				Foreground(textDim).
				Italic(true)

	// --- Status styles ---

	errorStyle = lipgloss.NewStyle().
			Foreground(danger).
			Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(success)

	scrollIndicatorStyle = lipgloss.NewStyle().
				Background(primary).
				Foreground(lipgloss.Color("#000000")).
				Bold(true)
)
