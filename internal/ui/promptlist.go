package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/kno-ai/kno-trace/internal/model"
)

// PromptList is a scrollable list of prompts with badges and branch dividers.
type PromptList struct {
	Prompts  []*model.Prompt
	Cursor   int
	Offset   int // scroll offset (first visible index)
	Width    int
	Height   int
	CompactAt map[int]bool // prompt indices where compact occurred
}

// NewPromptList creates a prompt list from session data.
func NewPromptList(session *model.Session) PromptList {
	compactSet := make(map[int]bool)
	for _, idx := range session.CompactAt {
		compactSet[idx] = true
	}
	return PromptList{
		Prompts:   session.Prompts,
		CompactAt: compactSet,
	}
}

func (pl *PromptList) MoveDown() {
	if pl.Cursor < len(pl.Prompts)-1 {
		pl.Cursor++
		pl.ensureVisible()
	}
}

func (pl *PromptList) MoveUp() {
	if pl.Cursor > 0 {
		pl.Cursor--
		pl.ensureVisible()
	}
}

func (pl *PromptList) GoTop() {
	pl.Cursor = 0
	pl.Offset = 0
}

func (pl *PromptList) GoBottom() {
	pl.Cursor = max(0, len(pl.Prompts)-1)
	pl.ensureVisible()
}

// SelectedPrompt returns the currently selected prompt, or nil.
func (pl *PromptList) SelectedPrompt() *model.Prompt {
	if len(pl.Prompts) == 0 || pl.Cursor < 0 || pl.Cursor >= len(pl.Prompts) {
		return nil
	}
	return pl.Prompts[pl.Cursor]
}

func (pl *PromptList) ensureVisible() {
	visibleLines := pl.visibleCount()
	if visibleLines <= 0 {
		return
	}
	if pl.Cursor < pl.Offset {
		pl.Offset = pl.Cursor
	}
	if pl.Cursor >= pl.Offset+visibleLines {
		pl.Offset = pl.Cursor - visibleLines + 1
	}
}

func (pl *PromptList) visibleCount() int {
	if pl.Height <= 0 {
		return 10
	}
	return pl.Height
}

// View renders the prompt list.
func (pl *PromptList) View() string {
	if len(pl.Prompts) == 0 {
		return EmptyStateStyle.Render("No prompts")
	}

	var b strings.Builder
	visible := pl.visibleCount()
	end := min(pl.Offset+visible, len(pl.Prompts))

	for i := pl.Offset; i < end; i++ {
		p := pl.Prompts[i]

		// Branch transition divider.
		if p.BranchTransition.From != "" {
			divider := DimStyle.Render(fmt.Sprintf("── %s → %s ──",
				p.BranchTransition.From, p.BranchTransition.To))
			b.WriteString(divider)
			b.WriteString("\n")
		}

		// Compact marker.
		if pl.CompactAt[p.Index] {
			b.WriteString(DimStyle.Render("── compact ──"))
			b.WriteString("\n")
		}

		selected := i == pl.Cursor
		line := pl.renderPromptLine(p, selected)
		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}

func (pl *PromptList) renderPromptLine(p *model.Prompt, selected bool) string {
	w := pl.Width
	if w <= 0 {
		w = 40
	}

	// Index and time.
	timeStr := p.StartTime.Local().Format("15:04")
	prefix := fmt.Sprintf("#%-2d %s", p.Index+1, timeStr)

	// Badges: tool counts.
	var badges []string
	counts := toolCounts(p)
	if counts.writes > 0 {
		badges = append(badges, fmt.Sprintf("W%d", counts.writes))
	}
	if counts.edits > 0 {
		badges = append(badges, fmt.Sprintf("E%d", counts.edits))
	}
	if counts.reads > 0 {
		badges = append(badges, fmt.Sprintf("R%d", counts.reads))
	}
	if counts.bashes > 0 {
		badges = append(badges, fmt.Sprintf("B%d", counts.bashes))
	}

	// Warning badges.
	if p.IsDurationOutlier {
		badges = append(badges, "⏱")
	}
	if p.Interrupted {
		badges = append(badges, "⚡")
	}
	for _, w := range p.Warnings {
		if w.Type == model.WarnContextCritical {
			badges = append(badges, fmt.Sprintf("%d%%!", p.ContextPct))
		} else if w.Type == model.WarnLoopDetected {
			badges = append(badges, "⟳")
		}
	}
	if len(p.Agents) > 0 {
		running, failed := 0, 0
		for _, a := range p.Agents {
			switch a.Status {
			case model.AgentRunning:
				running++
			case model.AgentFailed:
				failed++
			}
		}
		badge := fmt.Sprintf("⬡%d", len(p.Agents))
		if running > 0 {
			badge = fmt.Sprintf("⬡%d active", running)
		} else if failed > 0 {
			badge += fmt.Sprintf(" ✗%d", failed)
		}
		badges = append(badges, badge)
	}

	badgeStr := ""
	if len(badges) > 0 {
		badgeStr = " " + strings.Join(badges, " ")
	}

	// Truncate human text to fill remaining space.
	prefixLen := len(prefix) + 1
	badgeLen := len(badgeStr)
	textWidth := w - prefixLen - badgeLen - 1
	if textWidth < 5 {
		textWidth = 5
	}
	text := Truncate(firstLine(p.HumanText), textWidth)
	if text == "" {
		text = "(no text)"
	}

	if selected {
		cursor := SelectedStyle.Render("> ")
		line := lipgloss.NewStyle().Foreground(ColorBrandTeal).Render(
			fmt.Sprintf("%s %s%s", prefix, text, badgeStr))
		return cursor + line
	}

	idx := MutedStyle.Render(prefix)
	body := NormalStyle.Render(" " + text)
	badge := DimStyle.Render(badgeStr)
	return "  " + idx + body + badge
}

type toolCountResult struct {
	writes, edits, reads, bashes int
}

func toolCounts(p *model.Prompt) toolCountResult {
	var c toolCountResult
	for _, tc := range p.ToolCalls {
		switch tc.Type {
		case model.ToolWrite:
			c.writes++
		case model.ToolEdit:
			c.edits++
		case model.ToolRead:
			c.reads++
		case model.ToolBash:
			c.bashes++
		}
	}
	return c
}

// firstLine returns the first line of a string, trimmed.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
