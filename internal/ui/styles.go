// Package ui contains all TUI views and shared styling for kno-trace.
// styles.go is the single source of truth for all colors and lipgloss styles.
// Every other UI file imports from here — never define styles inline.
package ui

import "github.com/charmbracelet/lipgloss"

// Brand palette.
var (
	ColorBg        = lipgloss.Color("#0A0E14")
	ColorBrandTeal = lipgloss.Color("#4DFFC4")
	ColorDim       = lipgloss.Color("#555555") // decorative: dividers, patterns, scroll indicators
	ColorMuted     = lipgloss.Color("#999999") // secondary: metadata, labels, key hints
	ColorContent   = lipgloss.Color("#BBBBBB") // readable secondary: response text, file paths
	ColorWhite     = lipgloss.Color("#FFFFFF") // primary: human text, selected items
	ColorYellow    = lipgloss.Color("#FFD700")
	ColorRed       = lipgloss.Color("#FF5555")
	ColorCyan      = lipgloss.Color("#8BE9FD")
)

// AgentColors is an ordered palette for agent attribution in the live ticker.
// Assigned round-robin as agents first appear. Avoids red and green which
// carry semantic meaning (errors, success) in the rest of the UI.
var AgentColors = []lipgloss.Color{
	lipgloss.Color("#6C9EFF"), // blue
	lipgloss.Color("#B39DDB"), // purple
	lipgloss.Color("#FFB74D"), // orange
	lipgloss.Color("#4DD0E1"), // cyan
	lipgloss.Color("#F48FB1"), // pink
	lipgloss.Color("#9FA8DA"), // lavender
	lipgloss.Color("#FFD54F"), // amber
	lipgloss.Color("#90A4AE"), // slate
}

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

	// Muted metadata text (labels, hints, counts).
	MutedStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// Content text — readable secondary (response text, file paths, descriptions).
	ContentStyle = lipgloss.NewStyle().
			Foreground(ColorContent)

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
