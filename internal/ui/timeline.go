package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kno-ai/kno-trace/internal/model"
)

// timelineModel is the main session view — prompt list on the left, detail on the right.
type timelineModel struct {
	session   *model.Session
	list      PromptList
	detail    Detail
	width     int
	height    int

	// Search/filter state.
	filtering    bool
	filter       string
	filteredIdxs []int // indices into session.Prompts; nil = show all
}

func newTimeline(session *model.Session) timelineModel {
	return timelineModel{
		session: session,
		list:    NewPromptList(session),
		detail:  Detail{},
	}
}

func (t *timelineModel) setSize(w, h int) {
	t.width = w
	t.height = h

	// Split: left pane ~40%, right pane ~60%, minus borders/padding.
	listWidth := w * 2 / 5
	if listWidth < 25 {
		listWidth = min(25, w)
	}
	detailWidth := w - listWidth - 3 // 3 for divider + padding

	// Reserve 3 lines for stats bar + padding.
	contentHeight := h - 3

	t.list.Width = listWidth
	t.list.Height = contentHeight
	t.detail.Width = detailWidth
	t.detail.Height = contentHeight
}

func (t timelineModel) Update(msg tea.KeyMsg) (timelineModel, tea.Cmd) {
	if t.filtering {
		return t.updateFiltering(msg)
	}
	return t.updateNormal(msg)
}

func (t timelineModel) updateNormal(msg tea.KeyMsg) (timelineModel, tea.Cmd) {
	prevCursor := t.list.Cursor
	switch msg.String() {
	case "j", "down":
		t.list.MoveDown()
	case "k", "up":
		t.list.MoveUp()
	case "g":
		t.list.GoTop()
	case "G":
		t.list.GoBottom()
	case "/":
		t.filtering = true
		t.filter = ""
	case "l", "right":
		t.detail.ScrollDown()
	case "h", "left":
		t.detail.ScrollUp()
	}

	// Reset detail scroll when cursor moves.
	if t.list.Cursor != prevCursor {
		t.detail.Offset = 0
	}

	return t, nil
}

func (t timelineModel) updateFiltering(msg tea.KeyMsg) (timelineModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		t.filtering = false
		t.filter = ""
		t.filteredIdxs = nil
		t.restoreFullList()
		return t, nil
	case tea.KeyEnter:
		t.filtering = false
		// Keep filter active — j/k now navigate the filtered list.
		return t, nil
	case tea.KeyUp:
		t.list.MoveUp()
		t.detail.Offset = 0
		return t, nil
	case tea.KeyDown:
		t.list.MoveDown()
		t.detail.Offset = 0
		return t, nil
	case tea.KeyBackspace:
		if len(t.filter) > 0 {
			t.filter = t.filter[:len(t.filter)-1]
			t.applyFilter()
		}
		return t, nil
	case tea.KeyRunes:
		t.filter += string(msg.Runes)
		t.applyFilter()
		return t, nil
	}
	return t, nil
}

func (t *timelineModel) applyFilter() {
	if t.filter == "" || t.session == nil {
		t.restoreFullList()
		return
	}

	lower := strings.ToLower(t.filter)
	var matched []*model.Prompt
	var matchedIdxs []int

	for _, p := range t.session.Prompts {
		if t.promptMatchesFilter(p, lower) {
			matched = append(matched, p)
			matchedIdxs = append(matchedIdxs, p.Index)
		}
	}

	t.filteredIdxs = matchedIdxs
	t.list.Prompts = matched
	t.list.Cursor = 0
	t.list.Offset = 0
}

func (t *timelineModel) restoreFullList() {
	if t.session != nil {
		t.list.Prompts = t.session.Prompts
	}
	t.filteredIdxs = nil
}

func (t *timelineModel) promptMatchesFilter(p *model.Prompt, lower string) bool {
	// Match human text.
	if strings.Contains(strings.ToLower(p.HumanText), lower) {
		return true
	}
	// Match file paths and tool names.
	for _, tc := range p.ToolCalls {
		if strings.Contains(strings.ToLower(tc.Path), lower) {
			return true
		}
		if strings.Contains(strings.ToLower(string(tc.Type)), lower) {
			return true
		}
		if strings.Contains(strings.ToLower(tc.Command), lower) {
			return true
		}
	}
	return false
}

// View renders the full timeline layout.
func (t timelineModel) View() string {
	if t.session == nil || len(t.session.Prompts) == 0 {
		return EmptyStateStyle.Render("No prompts in session")
	}

	// Left pane: prompt list.
	leftContent := t.list.View()

	// Filter bar above list.
	filterBar := ""
	if t.filtering {
		filterBar = FilterPromptStyle.Render("/ ") + t.filter + "_  " +
			DimStyle.Render("enter to keep · esc to clear") + "\n"
	} else if t.filter != "" {
		filterBar = FilterPromptStyle.Render("filter: ") + MutedStyle.Render(t.filter) + "  " +
			DimStyle.Render("esc to clear") + "\n"
	}

	leftPane := filterBar + leftContent

	// Right pane: detail for selected prompt.
	selected := t.list.SelectedPrompt()
	rightContent := t.detail.View(selected)

	// Divider.
	divider := lipgloss.NewStyle().
		Foreground(ColorDim).
		Render(strings.Repeat("│\n", max(1, t.height-3)))

	// Layout: left | divider | right.
	layout := lipgloss.JoinHorizontal(lipgloss.Top,
		leftPane,
		" "+divider+" ",
		rightContent,
	)

	// Stats bar at bottom.
	stats := t.statsBar()

	return layout + "\n" + stats
}

func (t timelineModel) statsBar() string {
	if t.session == nil {
		return ""
	}

	var parts []string

	// Session name.
	parts = append(parts, SelectedStyle.Render(t.session.ProjectName))

	// Prompt count.
	total := len(t.session.Prompts)
	if t.filteredIdxs != nil {
		parts = append(parts, MutedStyle.Render(
			fmt.Sprintf("%d/%d prompts", len(t.list.Prompts), total)))
	} else {
		parts = append(parts, MutedStyle.Render(
			fmt.Sprintf("%d prompts", total)))
	}

	// File count (unique paths across all tool calls).
	files := make(map[string]bool)
	var totalTokens int
	var lastContextPct int
	for _, p := range t.session.Prompts {
		for _, tc := range p.ToolCalls {
			if tc.Path != "" {
				files[tc.Path] = true
			}
		}
		totalTokens += p.TokensIn + p.TokensOut
		if p.ContextPct > 0 {
			lastContextPct = p.ContextPct
		}
	}
	if len(files) > 0 {
		parts = append(parts, MutedStyle.Render(fmt.Sprintf("%d files", len(files))))
	}

	// Total tokens.
	if totalTokens > 0 {
		parts = append(parts, MutedStyle.Render(FormatTokens(totalTokens)+" tokens"))
	}

	// Context% of most recent prompt.
	if lastContextPct > 0 {
		ctxStr := fmt.Sprintf("ctx: %d%%", lastContextPct)
		if lastContextPct >= 85 {
			parts = append(parts, MutedStyle.Foreground(ColorRed).Render(ctxStr))
		} else if lastContextPct >= 70 {
			parts = append(parts, MutedStyle.Foreground(ColorYellow).Render(ctxStr))
		} else {
			parts = append(parts, MutedStyle.Render(ctxStr))
		}
	}

	// Key hints.
	keys := KeyStyle.Render("j/k") + " " + KeyDescStyle.Render("nav") + "  " +
		KeyStyle.Render("/") + " " + KeyDescStyle.Render("filter") + "  " +
		KeyStyle.Render("P") + " " + KeyDescStyle.Render("picker") + "  " +
		KeyStyle.Render("q") + " " + KeyDescStyle.Render("quit")

	return StatusBarStyle.Render(strings.Join(parts, "  ·  ") + "    " + keys)
}
