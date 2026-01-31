package tui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	// Colors
	primaryColor   = lipgloss.Color("205") // Pink
	secondaryColor = lipgloss.Color("99")  // Purple
	accentColor    = lipgloss.Color("212") // Light pink
	mutedColor     = lipgloss.Color("241") // Gray
	errorColor     = lipgloss.Color("196") // Red
	successColor   = lipgloss.Color("42")  // Green

	// Title styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			PaddingLeft(2)

	// Input styles
	inputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			PaddingLeft(2)

	focusedInputStyle = lipgloss.NewStyle().
				Foreground(accentColor).
				Bold(true).
				PaddingLeft(2)

	// Session list styles
	selectedItemStyle = lipgloss.NewStyle().
				Foreground(accentColor).
				Bold(true).
				PaddingLeft(2)

	normalItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			PaddingLeft(2)

	archivedItemStyle = lipgloss.NewStyle().
				Foreground(mutedColor).
				PaddingLeft(2)

	createNewStyle = lipgloss.NewStyle().
			Foreground(successColor).
			Bold(true).
			PaddingLeft(2)

	// Metadata styles
	metadataStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			PaddingLeft(4)

	// Separator styles
	separatorStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			PaddingLeft(2)

	// Status message styles
	statusStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			PaddingLeft(2)

	errorStyle = lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true).
			PaddingLeft(2)

	successStyle = lipgloss.NewStyle().
			Foreground(successColor).
			PaddingLeft(2)

	// Help text styles
	helpStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			PaddingLeft(2).
			PaddingTop(1)

	// Dialog styles
	dialogBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Padding(1, 2).
			Width(60)

	dialogTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(primaryColor)

	dialogTextStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	warningStyle = lipgloss.NewStyle().
			Foreground(errorColor)
)
