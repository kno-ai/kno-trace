package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kno-ai/kno-trace/internal/model"
)

// swimlaneModel renders parallel agent lanes for a selected prompt.
// Each agent gets a horizontal lane showing its tool calls as blocks.
// The parent session gets its own lane showing agent spawn/completion points.
type swimlaneModel struct {
	session *model.Session
	width   int
	height  int

	// promptIdx is the index into session.Prompts of the displayed prompt.
	// Only prompts with agents are navigable.
	promptIdx int

	// scroll tracks vertical scroll offset within the lanes.
	scroll int

	// laneCursor selects which lane is focused (-1 = parent, 0+ = agent index).
	laneCursor int

	// collapsed tracks which lanes have their tool blocks hidden.
	// Key is agent index (or -1 for parent).
	collapsed map[int]bool

	// isLive mirrors session.IsLive.
	isLive bool
}

func newSwimlane(session *model.Session) swimlaneModel {
	s := swimlaneModel{
		session:    session,
		laneCursor: -1, // start on parent lane
		collapsed:  make(map[int]bool),
	}
	s.promptIdx = s.lastPromptWithAgents()
	return s
}

func (s *swimlaneModel) setSize(w, h int) {
	s.width = w
	s.height = h
}

func (s *swimlaneModel) syncSession(session *model.Session) {
	s.session = session
	s.isLive = session.IsLive
}

// Update handles key input for the swimlane view.
func (s swimlaneModel) Update(msg tea.KeyMsg) (swimlaneModel, tea.Cmd) {
	prompt := s.selectedPrompt()
	agentCount := 0
	if prompt != nil {
		agentCount = len(prompt.Agents)
	}

	switch msg.String() {
	case "j", "down":
		// Navigate between lanes.
		if s.laneCursor < agentCount-1 {
			s.laneCursor++
		}
	case "k", "up":
		// Navigate between lanes (-1 = parent).
		if s.laneCursor > -1 {
			s.laneCursor--
		}
	case "g":
		s.laneCursor = -1
		s.scroll = 0
	case "G":
		s.laneCursor = max(0, agentCount-1)
	case "enter":
		// Toggle collapse/expand on the focused lane.
		if s.collapsed == nil {
			s.collapsed = make(map[int]bool)
		}
		s.collapsed[s.laneCursor] = !s.collapsed[s.laneCursor]
	case "n", "right", "l", "tab":
		s.nextPromptWithAgents()
		s.laneCursor = -1
		s.collapsed = make(map[int]bool)
	case "N", "left", "h", "shift+tab":
		s.prevPromptWithAgents()
		s.laneCursor = -1
		s.collapsed = make(map[int]bool)
	}
	return s, nil
}

// View renders the swimlane layout.
func (s swimlaneModel) View() string {
	if s.session == nil {
		return EmptyStateStyle.Render("No session")
	}

	var b strings.Builder
	w := s.width
	if w <= 0 {
		w = 80
	}

	// Title bar.
	b.WriteString(TitleStyle.Render("kno-trace — swimlane"))
	b.WriteString("\n")

	// Prompt selector bar.
	b.WriteString(s.promptSelectorBar(w))
	b.WriteString("\n")

	prompt := s.selectedPrompt()
	if prompt == nil || len(prompt.Agents) == 0 {
		b.WriteString(EmptyStateStyle.Render("No agents in this prompt"))
		b.WriteString("\n")
		b.WriteString(s.statusBar())
		return b.String()
	}

	// Conflict banner.
	for _, warn := range prompt.Warnings {
		if warn.Type == model.WarnAgentConflict {
			b.WriteString(MutedStyle.Foreground(ColorRed).Render("  ⚠ " + warn.Message))
			b.WriteString("\n")
		}
	}

	// Render lanes.
	laneContent := s.renderLanes(prompt, w)
	b.WriteString(laneContent)

	// Stats bar.
	b.WriteString(s.statusBar())

	return b.String()
}

// selectedPrompt returns the currently selected prompt, or nil.
func (s *swimlaneModel) selectedPrompt() *model.Prompt {
	if s.session == nil || s.promptIdx < 0 || s.promptIdx >= len(s.session.Prompts) {
		return nil
	}
	return s.session.Prompts[s.promptIdx]
}

// promptSelectorBar renders a bar showing all prompts, highlighting those with agents.
func (s swimlaneModel) promptSelectorBar(w int) string {
	if s.session == nil || len(s.session.Prompts) == 0 {
		return ""
	}

	var parts []string
	for _, p := range s.session.Prompts {
		label := fmt.Sprintf("#%d", p.Index+1)
		if p.Index == s.promptIdx {
			if len(p.Agents) > 0 {
				parts = append(parts, SelectedStyle.Render(label+" ⬡"+fmt.Sprintf("%d", len(p.Agents))))
			} else {
				parts = append(parts, SelectedStyle.Render(label))
			}
		} else if len(p.Agents) > 0 {
			parts = append(parts, NormalStyle.Render(label+" ⬡"+fmt.Sprintf("%d", len(p.Agents))))
		} else {
			parts = append(parts, DimStyle.Render(label))
		}
	}

	line := "  " + strings.Join(parts, "  ")
	if len(line) > w {
		line = line[:w]
	}
	return line
}

// renderLanes renders the parent lane and each agent lane.
func (s swimlaneModel) renderLanes(prompt *model.Prompt, w int) string {
	var lines []string

	// Parent lane — shows spawn points and completion.
	isFocused := s.laneCursor == -1
	isCollapsed := s.collapsed[-1]
	lines = append(lines, s.renderParentLane(prompt, w, isFocused, isCollapsed))
	lines = append(lines, "")

	// Agent lanes.
	for i, ag := range prompt.Agents {
		color := AgentColors[i%len(AgentColors)]
		isFocused := s.laneCursor == i
		isCollapsed := s.collapsed[i]
		lines = append(lines, s.renderAgentLane(ag, prompt, color, w, isFocused, isCollapsed)...)
		lines = append(lines, "")
	}

	// Unlinked agents.
	if s.session != nil {
		for _, ag := range s.session.UnlinkedAgents {
			if ag.ParentPromptIdx == prompt.Index {
				lines = append(lines, MutedStyle.Foreground(ColorYellow).Render(
					fmt.Sprintf("  ⚠ %s — agent linkage unresolved", ag.Label)))
			}
		}
	}

	// Apply vertical scroll.
	visibleHeight := s.height - 5 // title + selector + stats + padding
	if visibleHeight < 3 {
		visibleHeight = 3
	}
	if s.scroll > 0 {
		if s.scroll >= len(lines) {
			s.scroll = max(0, len(lines)-1)
		}
		lines = lines[s.scroll:]
	}
	if len(lines) > visibleHeight {
		lines = lines[:visibleHeight]
	}

	return strings.Join(lines, "\n") + "\n"
}

// renderParentLane shows agent spawn/completion events from the parent prompt.
func (s swimlaneModel) renderParentLane(prompt *model.Prompt, w int, isFocused, isCollapsed bool) string {
	cursor := "  "
	if isFocused {
		cursor = SelectedStyle.Render("> ")
	}

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorBrandTeal)
	header := cursor + headerStyle.Render("parent")

	// Show the parent's duration if available.
	if !prompt.StartTime.IsZero() && !prompt.EndTime.IsZero() {
		header += "  " + DimStyle.Render(FormatDuration(prompt.EndTime.Sub(prompt.StartTime)))
	} else if !prompt.StartTime.IsZero() && s.isLive {
		header += "  " + DimStyle.Render(FormatDuration(time.Since(prompt.StartTime)))
	}

	if isCollapsed {
		agentSpawns := 0
		for _, tc := range prompt.ToolCalls {
			if tc.Type == model.ToolAgent {
				agentSpawns++
			}
		}
		if agentSpawns > 0 {
			header += "  " + DimStyle.Render(fmt.Sprintf("(%d spawns)", agentSpawns))
		}
		return header
	}

	var blocks []string
	for _, tc := range prompt.ToolCalls {
		if tc.Type == model.ToolAgent {
			blocks = append(blocks, DimStyle.Render(fmt.Sprintf("    spawn %s", Truncate(tc.AgentDescription, w-15))))
		}
	}

	result := header
	if len(blocks) > 0 {
		result += "\n" + strings.Join(blocks, "\n")
	}
	return result
}

// renderAgentLane renders a single agent's lane with header and tool blocks.
func (s swimlaneModel) renderAgentLane(ag *model.AgentNode, prompt *model.Prompt, color lipgloss.Color, w int, isFocused, isCollapsed bool) []string {
	var lines []string

	// Lane header.
	cursor := "  "
	if isFocused {
		cursor = SelectedStyle.Render("> ")
	}
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(color)
	header := cursor + headerStyle.Render(ag.Label)

	// Type and model.
	typeLabel := ag.SubagentType
	if typeLabel == "" {
		typeLabel = "agent"
	}
	if ag.ModelName != "" && ag.ModelName != prompt.ModelName {
		typeLabel += ", " + shortModel(ag.ModelName)
	}
	header += " " + DimStyle.Render("("+typeLabel+")")

	// Status indicator.
	switch ag.Status {
	case model.AgentRunning:
		elapsed := ""
		if !ag.StartTime.IsZero() && s.isLive {
			elapsed = " " + FormatDuration(time.Since(ag.StartTime))
		}
		header += "  " + headerStyle.Render("running"+elapsed)
	case model.AgentSucceeded:
		dur := ""
		if ag.Duration > 0 {
			dur = " " + FormatDuration(ag.Duration)
		}
		header += "  " + DimStyle.Render("done"+dur)
	case model.AgentFailed:
		dur := ""
		if ag.Duration > 0 {
			dur = " " + FormatDuration(ag.Duration)
		}
		header += "  " + MutedStyle.Foreground(ColorRed).Render("✗ failed"+dur)
	}

	// Parallel badge.
	if ag.IsParallel {
		header += "  " + DimStyle.Render("[parallel]")
	}

	lines = append(lines, header)

	// Collapsed: show summary line only.
	if isCollapsed {
		summary := fmt.Sprintf("    %d tools", len(ag.ToolCalls))
		if len(ag.FilesTouched) > 0 {
			summary += fmt.Sprintf(", %d files", len(ag.FilesTouched))
		}
		lines = append(lines, DimStyle.Render(summary))
		return lines
	}

	// Tool blocks.
	blockStyle := lipgloss.NewStyle().Foreground(color)
	maxBlocks := w / 2 // reasonable limit
	if maxBlocks < 10 {
		maxBlocks = 10
	}

	for i, tc := range ag.ToolCalls {
		if i >= maxBlocks {
			lines = append(lines, DimStyle.Render(fmt.Sprintf("    ... +%d more", len(ag.ToolCalls)-maxBlocks)))
			break
		}

		icon := toolIcon(tc.Type)
		path := tc.Path
		if tc.Type == model.ToolBash {
			path = Truncate(tc.Command, w-20)
		}
		if path == "" {
			path = string(tc.Type)
		}

		// Highlight the latest tool block for running agents.
		isLatest := ag.Status == model.AgentRunning && i == len(ag.ToolCalls)-1
		blockText := fmt.Sprintf("    %s %s", icon, Truncate(path, w-15))
		if isLatest && s.isLive {
			lines = append(lines, blockStyle.Bold(true).Render(blockText))
		} else {
			lines = append(lines, blockStyle.Render(blockText))
		}
	}

	// File conflict markers on individual files.
	if ag.IsParallel && len(prompt.Warnings) > 0 {
		conflictPaths := extractConflictPaths(prompt.Warnings)
		for _, path := range ag.FilesTouched {
			if conflictPaths[path] {
				lines = append(lines, MutedStyle.Foreground(ColorRed).Render(
					fmt.Sprintf("    ⚠ %s (conflict)", path)))
			}
		}
	}

	// Files touched summary.
	if len(ag.FilesTouched) > 0 && ag.Status != model.AgentRunning {
		lines = append(lines, DimStyle.Render(
			fmt.Sprintf("    %d files touched", len(ag.FilesTouched))))
	}

	return lines
}

// extractConflictPaths extracts file paths mentioned in AgentConflict warnings.
func extractConflictPaths(warnings []model.Warning) map[string]bool {
	paths := make(map[string]bool)
	for _, w := range warnings {
		if w.Type != model.WarnAgentConflict {
			continue
		}
		// Message format: "file conflict: <path> touched by ..."
		const prefix = "file conflict: "
		if strings.HasPrefix(w.Message, prefix) {
			rest := w.Message[len(prefix):]
			if idx := strings.Index(rest, " touched by"); idx > 0 {
				paths[rest[:idx]] = true
			}
		}
	}
	return paths
}

// Navigation helpers.

func (s *swimlaneModel) nextPromptWithAgents() {
	if s.session == nil {
		return
	}
	for i := s.promptIdx + 1; i < len(s.session.Prompts); i++ {
		if len(s.session.Prompts[i].Agents) > 0 {
			s.promptIdx = i
			s.scroll = 0
			return
		}
	}
}

func (s *swimlaneModel) prevPromptWithAgents() {
	if s.session == nil {
		return
	}
	for i := s.promptIdx - 1; i >= 0; i-- {
		if len(s.session.Prompts[i].Agents) > 0 {
			s.promptIdx = i
			s.scroll = 0
			return
		}
	}
}

func (s *swimlaneModel) lastPromptWithAgents() int {
	if s.session == nil {
		return 0
	}
	for i := len(s.session.Prompts) - 1; i >= 0; i-- {
		if len(s.session.Prompts[i].Agents) > 0 {
			return i
		}
	}
	return max(0, len(s.session.Prompts)-1)
}

func (s swimlaneModel) statusBar() string {
	keys := KeyStyle.Render("j/k") + " " + KeyDescStyle.Render("lane") + "  " +
		KeyStyle.Render("enter") + " " + KeyDescStyle.Render("toggle") + "  " +
		KeyStyle.Render("←/→") + " " + KeyDescStyle.Render("prompt") + "  " +
		KeyStyle.Render("esc") + " " + KeyDescStyle.Render("timeline") + "  " +
		KeyStyle.Render("q") + " " + KeyDescStyle.Render("quit")
	return StatusBarStyle.Render(keys)
}
