package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

	"github.com/kno-ai/kno-trace/internal/model"
)

// pickerModel is the session picker view — the front door to kno-trace.
type pickerModel struct {
	sessions []*model.SessionMeta // all sessions, sorted by time desc
	filtered []int                // indices into sessions after fuzzy filter
	cursor   int                  // position in filtered list
	filter   string               // current filter text
	filtering bool                // true when filter input is active
	width    int
	height   int
}

func newPicker(sessions []*model.SessionMeta) pickerModel {
	m := pickerModel{
		sessions: sessions,
	}
	m.resetFilter()
	return m
}

func (m *pickerModel) resetFilter() {
	m.filtered = make([]int, len(m.sessions))
	for i := range m.sessions {
		m.filtered[i] = i
	}
	m.filter = ""
	m.filtering = false
	m.cursor = 0
}

func (m *pickerModel) applyFilter() {
	if m.filter == "" {
		m.resetFilter()
		return
	}

	// Build string list for fuzzy matching (project names).
	strs := make([]string, len(m.sessions))
	for i, s := range m.sessions {
		strs[i] = s.ProjectName
	}

	matches := fuzzy.Find(m.filter, strs)
	m.filtered = make([]int, len(matches))
	for i, match := range matches {
		m.filtered[i] = match.Index
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

// selectedSession returns the currently highlighted session, or nil.
func (m *pickerModel) selectedSession() *model.SessionMeta {
	if len(m.filtered) == 0 || m.cursor < 0 || m.cursor >= len(m.filtered) {
		return nil
	}
	return m.sessions[m.filtered[m.cursor]]
}

func (m pickerModel) Update(msg tea.Msg) (pickerModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.filtering {
			return m.updateFiltering(msg)
		}
		return m.updateNormal(msg)
	}
	return m, nil
}

func (m pickerModel) updateFiltering(msg tea.KeyMsg) (pickerModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		m.resetFilter()
		return m, nil
	case tea.KeyEnter:
		m.filtering = false
		return m, nil
	case tea.KeyBackspace:
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
			m.applyFilter()
		}
		return m, nil
	case tea.KeyRunes:
		m.filter += string(msg.Runes)
		m.applyFilter()
		return m, nil
	}
	return m, nil
}

func (m pickerModel) updateNormal(msg tea.KeyMsg) (pickerModel, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "g":
		m.cursor = 0
	case "G":
		m.cursor = max(0, len(m.filtered)-1)
	case "/":
		m.filtering = true
		m.filter = ""
	}
	return m, nil
}

func (m pickerModel) View() string {
	if len(m.sessions) == 0 {
		return EmptyStateStyle.Render("No sessions found — run Claude Code in a project directory first")
	}

	var b strings.Builder

	// Title.
	title := TitleStyle.Render("kno-trace — session picker")
	b.WriteString(title)
	b.WriteString("\n\n")

	// Filter indicator.
	if m.filtering {
		b.WriteString(FilterPromptStyle.Render("/ "))
		b.WriteString(m.filter)
		b.WriteString("_\n\n")
	} else if m.filter != "" {
		b.WriteString(FilterPromptStyle.Render("filter: "))
		b.WriteString(MutedStyle.Render(m.filter))
		b.WriteString("\n\n")
	}

	if len(m.filtered) == 0 {
		b.WriteString(EmptyStateStyle.Render("No matches"))
		return b.String()
	}

	// Calculate visible range for scrolling.
	listHeight := m.height - 8 // Reserve space for title, filter, status bar
	if listHeight < 3 {
		listHeight = 3
	}

	startIdx := 0
	if m.cursor >= listHeight {
		startIdx = m.cursor - listHeight + 1
	}
	endIdx := startIdx + listHeight
	if endIdx > len(m.filtered) {
		endIdx = len(m.filtered)
	}

	// Render sessions grouped by date.
	lastDate := ""
	for vi := startIdx; vi < endIdx; vi++ {
		idx := m.filtered[vi]
		s := m.sessions[idx]

		date := FormatDate(s.EndTime)
		if date != lastDate {
			if lastDate != "" {
				b.WriteString("\n")
			}
			b.WriteString(DateHeaderStyle.Render(date))
			b.WriteString("\n")
			lastDate = date
		}

		isSelected := vi == m.cursor
		line := m.formatSessionLine(s, isSelected)
		b.WriteString(line)
		b.WriteString("\n")
	}

	// Status bar.
	b.WriteString("\n")
	status := m.statusBar()
	b.WriteString(status)

	return b.String()
}

func (m pickerModel) formatSessionLine(s *model.SessionMeta, selected bool) string {
	projectWidth := 30
	if m.width > 0 {
		projectWidth = max(10, min(30, m.width/3))
	}

	project := Truncate(s.ProjectName, projectWidth)
	startTime := FormatTime(s.StartTime)
	duration := FormatDuration(s.Duration)
	size := FormatFileSize(s.FileSizeBytes)

	if selected {
		cursor := SelectedStyle.Render("> ")
		rest := lipgloss.NewStyle().Foreground(ColorBrandTeal).Render(
			fmt.Sprintf("%-*s  %s  %8s  %8s",
				projectWidth, project, startTime, duration, size))
		return cursor + rest
	}

	return "  " + NormalStyle.Render(
		fmt.Sprintf("%-*s", projectWidth, project)) +
		MutedStyle.Render(
			fmt.Sprintf("  %s  %8s  %8s", startTime, duration, size))
}

func (m pickerModel) statusBar() string {
	keys := []struct{ key, desc string }{
		{"enter", "open"},
		{"j/k", "navigate"},
		{"/", "filter"},
		{"q", "quit"},
	}

	var parts []string
	for _, k := range keys {
		parts = append(parts,
			KeyStyle.Render(k.key)+" "+KeyDescStyle.Render(k.desc))
	}

	count := MutedStyle.Render(fmt.Sprintf("%d sessions", len(m.filtered)))
	return StatusBarStyle.Render(strings.Join(parts, "  ") + "  " + count)
}
