package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

	"github.com/kno-ai/kno-trace/internal/model"
)

// sessionListModel renders the session list as the left pane at the top level.
// Same navigation style as the turns list — flat, j/k, enter to open.
type sessionListModel struct {
	sessions  []*model.SessionMeta
	filtered  []int // indices into sessions after fuzzy filter
	cursor    int
	offset    int // scroll offset
	filter    string
	filtering bool
	width     int
	height    int
}

func newSessionList(sessions []*model.SessionMeta) sessionListModel {
	m := sessionListModel{sessions: sessions}
	m.resetFilter()
	return m
}

func (m *sessionListModel) setSize(w, h int) {
	m.width = w
	m.height = h
}

func (m *sessionListModel) resetFilter() {
	m.filtered = make([]int, len(m.sessions))
	for i := range m.sessions {
		m.filtered[i] = i
	}
	m.filter = ""
	m.filtering = false
	m.cursor = 0
	m.offset = 0
}

func (m *sessionListModel) clearFilter() {
	m.resetFilter()
}

func (m *sessionListModel) applyFilter() {
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

func (m *sessionListModel) selectedSession() *model.SessionMeta {
	if len(m.filtered) == 0 || m.cursor < 0 || m.cursor >= len(m.filtered) {
		return nil
	}
	return m.sessions[m.filtered[m.cursor]]
}

func (m *sessionListModel) moveDown() {
	if m.cursor < len(m.filtered)-1 {
		m.cursor++
		m.ensureVisible()
	}
}

func (m *sessionListModel) moveUp() {
	if m.cursor > 0 {
		m.cursor--
		m.ensureVisible()
	}
}

func (m *sessionListModel) ensureVisible() {
	visible := m.visibleCount()
	if visible <= 0 {
		return
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+visible {
		m.offset = m.cursor - visible + 1
	}
}

func (m *sessionListModel) visibleCount() int {
	h := m.height - 4 // title + filter + status bar + padding
	if h < 3 {
		h = 3
	}
	return h
}

func (m sessionListModel) Update(msg tea.KeyMsg) (sessionListModel, tea.Cmd) {
	if m.filtering {
		return m.updateFiltering(msg)
	}
	return m.updateNormal(msg)
}

func (m sessionListModel) updateFiltering(msg tea.KeyMsg) (sessionListModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		m.resetFilter()
	case tea.KeyEnter:
		m.filtering = false
	case tea.KeyUp:
		m.moveUp()
	case tea.KeyDown:
		m.moveDown()
	case tea.KeyBackspace:
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
			m.applyFilter()
		}
	case tea.KeyRunes:
		m.filter += string(msg.Runes)
		m.applyFilter()
	}
	return m, nil
}

func (m sessionListModel) updateNormal(msg tea.KeyMsg) (sessionListModel, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		m.moveDown()
	case "k", "up":
		m.moveUp()
	case "g":
		m.cursor = 0
		m.offset = 0
	case "G":
		m.cursor = max(0, len(m.filtered)-1)
		m.ensureVisible()
	case "/":
		m.filtering = true
		m.filter = ""
	}
	return m, nil
}

// View renders the full session list screen (two-pane: list + summary).
func (m sessionListModel) View() string {
	if len(m.sessions) == 0 {
		return EmptyStateStyle.Render("No sessions found — run Claude Code in a project directory first") +
			"\n\n" + m.statusBar()
	}

	var b strings.Builder

	// Title.
	b.WriteString(TitleStyle.Render("kno-trace"))
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

	// Render visible rows.
	visible := m.visibleCount()
	end := m.offset + visible
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	// Calculate left pane width (same ratio as timeline).
	listWidth := m.width * 2 / 5
	if listWidth < 25 {
		listWidth = min(25, m.width)
	}

	// Build left pane rows.
	var leftLines []string
	for vi := m.offset; vi < end; vi++ {
		idx := m.filtered[vi]
		s := m.sessions[idx]
		isSelected := vi == m.cursor
		leftLines = append(leftLines, m.formatRow(s, isSelected, listWidth))
	}
	leftContent := strings.Join(leftLines, "\n")

	// Build right pane (summary for selected session).
	detailWidth := max(1, m.width-listWidth-3)
	rightContent := m.renderSummary(detailWidth)

	// Divider.
	divHeight := max(1, len(leftLines))
	divider := lipgloss.NewStyle().
		Foreground(ColorDim).
		Render(strings.Repeat("│\n", divHeight))

	layout := lipgloss.JoinHorizontal(lipgloss.Top,
		leftContent,
		" "+divider+" ",
		rightContent,
	)

	b.WriteString(layout)
	b.WriteString("\n")
	b.WriteString(m.statusBar())

	return b.String()
}

func (m sessionListModel) formatRow(s *model.SessionMeta, selected bool, w int) string {
	projectWidth := max(10, min(30, w/2))
	project := Truncate(s.ProjectName, projectWidth)
	startTime := FormatTime(s.StartTime)
	duration := FormatDuration(s.Duration)

	if selected {
		cursor := SelectedStyle.Render("> ")
		rest := lipgloss.NewStyle().Foreground(ColorBrandTeal).Render(
			fmt.Sprintf("%-*s  %s  %s", projectWidth, project, startTime, duration))
		return cursor + rest
	}

	return "  " + NormalStyle.Render(
		fmt.Sprintf("%-*s", projectWidth, project)) +
		MutedStyle.Render(
			fmt.Sprintf("  %s  %s", startTime, duration))
}

func (m sessionListModel) renderSummary(w int) string {
	sel := m.selectedSession()
	if sel == nil {
		return MutedStyle.Render("No session selected")
	}

	var b strings.Builder

	b.WriteString(SelectedStyle.Render(sel.ProjectName))
	b.WriteString("\n\n")

	// Details.
	if !sel.StartTime.IsZero() {
		b.WriteString(MutedStyle.Render("Started: ") + sel.StartTime.Local().Format("2006-01-02 15:04"))
		b.WriteString("\n")
	}
	if sel.Duration > 0 {
		b.WriteString(MutedStyle.Render("Duration: ") + FormatDuration(sel.Duration))
		b.WriteString("\n")
	}
	if sel.FileSizeBytes > 0 {
		b.WriteString(MutedStyle.Render("Size: ") + FormatFileSize(sel.FileSizeBytes))
		b.WriteString("\n")
	}
	if sel.ProjectPath != "" {
		path := sel.ProjectPath
		if w > 16 && len(path) > w-10 {
			path = "..." + path[len(path)-(w-13):]
		} else if w <= 16 {
			path = Truncate(path, max(5, w))
		}
		b.WriteString(MutedStyle.Render("Path: ") + MutedStyle.Render(path))
		b.WriteString("\n")
	}

	return b.String()
}

func (m sessionListModel) statusBar() string {
	keys := KeyStyle.Render("j/k") + " " + KeyDescStyle.Render("nav") + "  " +
		KeyStyle.Render("enter") + " " + KeyDescStyle.Render("open") + "  " +
		KeyStyle.Render("/") + " " + KeyDescStyle.Render("filter") + "  " +
		KeyStyle.Render("R") + " " + KeyDescStyle.Render("refresh") + "  " +
		KeyStyle.Render("q") + " " + KeyDescStyle.Render("quit")

	count := MutedStyle.Render(fmt.Sprintf("  %d sessions", len(m.filtered)))
	return StatusBarStyle.Render(keys + count)
}
