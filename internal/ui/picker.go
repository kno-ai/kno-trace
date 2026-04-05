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
	sessions  []*model.SessionMeta
	filtered  []int // indices into sessions after fuzzy filter
	cursor    int   // position in filtered list
	filter    string
	filtering bool
	width     int
	height    int
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

func (m *pickerModel) selectedSession() *model.SessionMeta {
	if len(m.filtered) == 0 || m.cursor < 0 || m.cursor >= len(m.filtered) {
		return nil
	}
	return m.sessions[m.filtered[m.cursor]]
}

func (m *pickerModel) moveDown() {
	if m.cursor < len(m.filtered)-1 {
		m.cursor++
	}
}

func (m *pickerModel) moveUp() {
	if m.cursor > 0 {
		m.cursor--
	}
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
	case tea.KeyUp:
		m.moveUp()
		return m, nil
	case tea.KeyDown:
		m.moveDown()
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
		m.moveDown()
	case "k", "up":
		m.moveUp()
	case "g":
		m.cursor = 0
	case "G":
		m.cursor = max(0, len(m.filtered)-1)
	case "/":
		m.filtering = true
		m.filter = ""
	case "esc":
		if m.filter != "" {
			m.resetFilter()
		}
	}
	return m, nil
}

func (m pickerModel) View() string {
	if len(m.sessions) == 0 {
		return EmptyStateStyle.Render("No sessions found — run Claude Code in a project directory first") +
			"\n\n" + StatusBarStyle.Render(KeyStyle.Render("q")+" "+KeyDescStyle.Render("quit"))
	}

	var b strings.Builder

	// Title.
	b.WriteString(TitleStyle.Render("kno-trace — session picker"))
	b.WriteString("\n")

	// Filter bar.
	if m.filtering {
		b.WriteString(FilterPromptStyle.Render("/ "))
		b.WriteString(m.filter)
		b.WriteString("_  ")
		b.WriteString(DimStyle.Render("enter to keep · esc to clear"))
	} else if m.filter != "" {
		b.WriteString(FilterPromptStyle.Render("filter: "))
		b.WriteString(MutedStyle.Render(m.filter))
		b.WriteString("  ")
		b.WriteString(DimStyle.Render("/ to edit · esc to clear"))
	}
	b.WriteString("\n")

	if len(m.filtered) == 0 {
		b.WriteString(EmptyStateStyle.Render("No matches"))
		b.WriteString("\n")
		b.WriteString(m.statusBar())
		return b.String()
	}

	// Build all lines first, then window them.
	type renderedLine struct {
		text      string
		itemIndex int // -1 for headers/dividers
	}
	var lines []renderedLine

	lastDate := ""
	for vi := 0; vi < len(m.filtered); vi++ {
		idx := m.filtered[vi]
		s := m.sessions[idx]

		date := FormatDate(s.EndTime)
		if date != lastDate {
			if lastDate != "" {
				lines = append(lines, renderedLine{"", -1})
			}
			lines = append(lines, renderedLine{DateHeaderStyle.Render(date), -1})
			lastDate = date
		}

		isSelected := vi == m.cursor
		lines = append(lines, renderedLine{m.formatSessionLine(s, isSelected), vi})
	}

	// Find the rendered line index for the cursor.
	cursorLine := 0
	for i, l := range lines {
		if l.itemIndex == m.cursor {
			cursorLine = i
			break
		}
	}

	// Window the lines around the cursor.
	// Reserve: 2 lines for title+filter, 2 for status bar + padding.
	listHeight := m.height - 4
	if listHeight < 5 {
		listHeight = 5
	}

	startLine := 0
	if cursorLine >= listHeight {
		startLine = cursorLine - listHeight/2
	}
	if startLine < 0 {
		startLine = 0
	}
	endLine := startLine + listHeight
	if endLine > len(lines) {
		endLine = len(lines)
		startLine = max(0, endLine-listHeight)
	}

	for i := startLine; i < endLine; i++ {
		b.WriteString(lines[i].text)
		b.WriteString("\n")
	}

	// Status bar.
	b.WriteString(m.statusBar())

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
