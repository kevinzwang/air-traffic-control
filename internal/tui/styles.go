package tui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	// Color Palette
	// Primary: Cyan - used for borders, title, key UI elements
	// Accent: Amber - used for selection/focus
	// Success: Green - used for positive actions
	// Error: Red - used for warnings/errors
	// Text: White/Gray hierarchy

	primary    = lipgloss.Color("#00d4ff") // Cyan
	accent     = lipgloss.Color("#ffb627") // Amber
	success    = lipgloss.Color("#00ff87") // Green
	danger     = lipgloss.Color("#ff5f5f") // Red
	textNormal = lipgloss.Color("#e4e4e4") // Light gray
	textMuted  = lipgloss.Color("#6c757d") // Gray
	textDim    = lipgloss.Color("#495057") // Dark gray

	// Container style - no border, we build it manually for full control
	containerStyle = lipgloss.NewStyle()

	// Header section
	headerStyle = lipgloss.NewStyle().
			PaddingBottom(1)

	// Title styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primary)

	// Repo/subtitle style
	subtitleStyle = lipgloss.NewStyle().
			Foreground(textMuted)

	// Divider style - subtle color for section separators
	dividerStyle = lipgloss.NewStyle().
			Foreground(textDim)

	// Border style - primary color for outer border
	borderStyle = lipgloss.NewStyle().
			Foreground(primary)

	// Session list styles
	selectedItemStyle = lipgloss.NewStyle().
				Foreground(accent).
				Bold(true)

	normalItemStyle = lipgloss.NewStyle().
			Foreground(textNormal)

	archivedItemStyle = lipgloss.NewStyle().
				Foreground(textDim)

	createNewStyle = lipgloss.NewStyle().
			Foreground(success).
			Bold(true)

	// Metadata styles
	metadataStyle = lipgloss.NewStyle().
			Foreground(textMuted).
			PaddingLeft(2)

	// Status message styles
	statusStyle = lipgloss.NewStyle().
			Foreground(textMuted)

	errorStyle = lipgloss.NewStyle().
			Foreground(danger).
			Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(success)

	// Help text styles
	helpStyle = lipgloss.NewStyle().
			Foreground(textMuted)

	// Dialog styles
	dialogBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primary).
			Padding(1, 2)

	dialogTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(danger)

	dialogTextStyle = lipgloss.NewStyle().
			Foreground(textNormal)

	warningStyle = lipgloss.NewStyle().
			Foreground(danger)
)
