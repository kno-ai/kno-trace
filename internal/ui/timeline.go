package ui

import (
	"fmt"
	"strings"
	"time"

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

	// splitPct controls the left/right pane split (0-100, percentage for left pane).
	// Default 40. Adjusted with [ and ].
	splitPct int

	// Search/filter state.
	filtering    bool
	filter       string
	filteredIdxs []int // indices into session.Prompts; nil = show all

	// Live session state.
	isLive     bool   // mirrors session.IsLive
	autoFollow bool   // true = cursor tracks latest prompt
	ticker     Ticker // live tool call ticker
}

func newTimeline(session *model.Session) timelineModel {
	return timelineModel{
		session:  session,
		list:     NewPromptList(session),
		detail:   Detail{itemCursor: -1},
		splitPct: 40,
	}
}

func (t *timelineModel) setSize(w, h int) {
	t.width = w
	t.height = h

	if t.splitPct < 15 {
		t.splitPct = 15
	}
	if t.splitPct > 85 {
		t.splitPct = 85
	}

	// Split using splitPct, minus divider padding.
	listWidth := max(15, w*t.splitPct/100)
	detailWidth := max(1, w-listWidth-3) // 3 for divider + padding

	// Reserve lines: breadcrumb + stats bar + padding + ticker (when live).
	reserved := 4 // breadcrumb + stats + padding
	if t.isLive {
		reserved = 5 // extra line for ticker strip
	}
	contentHeight := max(1, h-reserved)

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
	case "[":
		t.splitPct -= 5
		t.setSize(t.width, t.height)
	case "]":
		t.splitPct += 5
		t.setSize(t.width, t.height)
	}

	// Reset detail scroll when cursor moves.
	if t.list.Cursor != prevCursor {
		t.detail.Offset = 0
		// Disengage auto-follow on manual navigation.
		t.autoFollow = false
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

// syncSession updates the timeline's prompt list and session reference after
// an incremental rebuild. Preserves cursor position when possible.
func (t *timelineModel) syncSession(s *model.Session) {
	t.session = s
	t.isLive = s.IsLive

	// If no filter active, sync prompt list directly.
	if t.filteredIdxs == nil {
		t.list.Prompts = s.Prompts
		// Update compact set.
		compactSet := make(map[int]bool)
		for _, idx := range s.CompactAt {
			compactSet[idx] = true
		}
		t.list.CompactAt = compactSet
	}

	// Auto-follow: keep cursor on latest prompt and scroll detail to bottom.
	// Suppressed when the user is drilling into the detail pane — don't yank
	// them away from what they're looking at.
	if t.autoFollow && !t.detail.HasFocus && len(t.list.Prompts) > 0 {
		t.list.Cursor = len(t.list.Prompts) - 1
		t.list.ensureVisible()
		t.detail.ScrollToBottom()
	}

	// Recalculate layout (ticker visibility may have changed).
	t.setSize(t.width, t.height)
}

// onPromptSealed is called when a new prompt boundary is detected during live
// tailing. Re-engages auto-follow and jumps to the latest prompt.
func (t *timelineModel) onPromptSealed() {
	t.autoFollow = true
	t.followLatest()
}

// followLatest moves the cursor to the last prompt and scrolls to show it.
func (t *timelineModel) followLatest() {
	if len(t.list.Prompts) == 0 {
		return
	}
	t.list.Cursor = len(t.list.Prompts) - 1
	t.list.ensureVisible()
	t.detail.Offset = 0
}

// View renders the full timeline layout.
func (t timelineModel) View() string {
	if t.session == nil || len(t.session.Prompts) == 0 {
		if t.session != nil && t.session.IsLive {
			return EmptyStateStyle.Render("Waiting for first prompt...")
		}
		return EmptyStateStyle.Render("No prompts in session")
	}

	// Breadcrumb.
	breadcrumb := TitleStyle.Render("kno-trace")
	if t.session.ProjectName != "" {
		breadcrumb += MutedStyle.Render(" > ") + SelectedStyle.Render(t.session.ProjectName)
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

	leftPane := breadcrumb + "\n" + filterBar + leftContent

	// Right pane: detail for selected prompt.
	selected := t.list.SelectedPrompt()
	rightContent := t.detail.View(selected, t.isLive)

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

	// Ticker strip (only for live sessions with activity).
	tickerLine := ""
	if t.isLive && t.ticker.HasEntries() {
		tickerLine = "  " + t.ticker.View(t.width-4, time.Now()) + "\n"
	}

	// Stats bar at bottom.
	stats := t.statsBar()

	return layout + "\n" + tickerLine + stats
}

func (t timelineModel) statsBar() string {
	if t.session == nil {
		return ""
	}

	var parts []string

	// Live indicator dot.
	if t.isLive {
		liveDot := lipgloss.NewStyle().Foreground(ColorBrandTeal).Bold(true).Render("●")
		parts = append(parts, liveDot)
	}

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

	// Agent activity indicator (live sessions only).
	if t.isLive && len(t.session.Prompts) > 0 {
		activePrompt := t.session.Prompts[len(t.session.Prompts)-1]
		var running, done int
		for _, a := range activePrompt.Agents {
			switch a.Status {
			case model.AgentRunning:
				running++
			case model.AgentSucceeded, model.AgentFailed:
				done++
			}
		}
		if running > 0 {
			agentStr := fmt.Sprintf("⬡ %d active", running)
			if done > 0 {
				agentStr += fmt.Sprintf(", %d done", done)
			}
			parts = append(parts, MutedStyle.Render(agentStr))
		}
	}

	// Selection count.
	selCount := t.list.SelectedCount()
	if selCount > 0 {
		parts = append(parts, SelectedStyle.Render(fmt.Sprintf("%d selected", selCount)))
	}

	// Key hints — context-appropriate based on focus.
	var keys string
	if t.detail.HasFocus {
		keys = KeyStyle.Render("j/k") + " " + KeyDescStyle.Render("items") + "  " +
			KeyStyle.Render("enter") + " " + KeyDescStyle.Render("drill in") + "  " +
			KeyStyle.Render("esc") + " " + KeyDescStyle.Render("back") + "  " +
			KeyStyle.Render("q") + " " + KeyDescStyle.Render("quit")
	} else if selCount > 0 {
		keys = KeyStyle.Render("j/k") + " " + KeyDescStyle.Render("turns") + "  " +
			KeyStyle.Render("space") + " " + KeyDescStyle.Render("select") + "  " +
			KeyStyle.Render("c") + " " + KeyDescStyle.Render("compare") + "  " +
			KeyStyle.Render("esc") + " " + KeyDescStyle.Render("clear") + "  " +
			KeyStyle.Render("q") + " " + KeyDescStyle.Render("quit")
	} else {
		keys = KeyStyle.Render("j/k") + " " + KeyDescStyle.Render("turns") + "  " +
			KeyStyle.Render("enter") + " " + KeyDescStyle.Render("detail") + "  " +
			KeyStyle.Render("space") + " " + KeyDescStyle.Render("select") + "  " +
			KeyStyle.Render("[/]") + " " + KeyDescStyle.Render("resize") + "  " +
			KeyStyle.Render("/") + " " + KeyDescStyle.Render("filter") + "  " +
			KeyStyle.Render("esc") + " " + KeyDescStyle.Render("sessions") + "  " +
			KeyStyle.Render("q") + " " + KeyDescStyle.Render("quit")
	}

	return StatusBarStyle.Render(strings.Join(parts, "  ·  ") + "    " + keys)
}
