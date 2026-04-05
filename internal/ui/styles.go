// Package ui contains all TUI views and shared styling for kno-trace.
// styles.go is the single source of truth for all colors and lipgloss styles.
// Every other UI file imports from here — never define styles inline.
package ui

import "github.com/charmbracelet/lipgloss"

// Brand palette.
var (
	ColorBg        = lipgloss.Color("#0A0E14")
	ColorBrandTeal = lipgloss.Color("#4DFFC4")
	ColorDim       = lipgloss.Color("#555555")
	ColorMuted     = lipgloss.Color("#888888")
	ColorWhite     = lipgloss.Color("#FFFFFF")
	ColorYellow    = lipgloss.Color("#FFD700")
	ColorRed       = lipgloss.Color("#FF5555")
	ColorCyan      = lipgloss.Color("#8BE9FD")
)

// Shared styles used across views.
var (
	// Title bar at the top of each screen.
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorBrandTeal).
			Padding(0, 1)

	// Status bar at the bottom.
	StatusBarStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Padding(0, 1)

	// Active/selected item in a list.
	SelectedStyle = lipgloss.NewStyle().
			Foreground(ColorBrandTeal).
			Bold(true)

	// Normal (unselected) list item.
	NormalStyle = lipgloss.NewStyle().
			Foreground(ColorWhite)

	// Dim/secondary text.
	DimStyle = lipgloss.NewStyle().
			Foreground(ColorDim)

	// Muted metadata text.
	MutedStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// Date group headers in the picker.
	DateHeaderStyle = lipgloss.NewStyle().
			Foreground(ColorYellow).
			Bold(true).
			Padding(0, 1)

	// Filter/search input prompt.
	FilterPromptStyle = lipgloss.NewStyle().
				Foreground(ColorBrandTeal)

	// Empty state message.
	EmptyStateStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true).
			Padding(1, 2)

	// Key binding hints.
	KeyStyle = lipgloss.NewStyle().
			Foreground(ColorBrandTeal).
			Bold(true)

	// Description next to key hints.
	KeyDescStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// Card/box for the session summary.
	CardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBrandTeal).
			Padding(1, 2)

	// Label in key-value displays.
	LabelStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Width(14)

	// Value in key-value displays.
	ValueStyle = lipgloss.NewStyle().
			Foreground(ColorWhite)
)
