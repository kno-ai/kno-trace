package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/kno-ai/kno-trace/internal/model"
	"github.com/kno-ai/kno-trace/internal/replay"
)

// Detail renders the right-pane detail view for a selected prompt.
type Detail struct {
	Width  int
	Height int
	Offset int // scroll offset for long content

	// HasFocus is true when the detail pane is the active pane.
	HasFocus bool

	// Agent expansion state.
	expandedPath    []string
	expandedCursors []int

	// agentCursor tracks which agent is focused. -1 = no agent focused.
	agentCursor int

	// toolCallDrillIn is set when the user drills into a specific tool call.
	toolCallDrillIn *model.ToolCall

	// comparison holds the rendered comparison content when comparing selected turns.
	// Non-empty = showing comparison view. Cleared by esc.
	comparison string
}

// View renders the detail pane for the given prompt.
// isLive enables elapsed-time display for the active prompt and running agents.
func (d *Detail) View(p *model.Prompt, isLive bool) string {
	if p == nil {
		return MutedStyle.Render("Select a prompt")
	}

	var b strings.Builder
	w := d.Width
	if w <= 0 {
		w = 50
	}

	// Comparison mode: show comparison content.
	if d.comparison != "" {
		return d.applyScroll(d.comparison)
	}

	// Tool call drill-in: show full detail for a specific tool call.
	if d.toolCallDrillIn != nil {
		d.renderToolCallDetail(&b, p, d.toolCallDrillIn, w)
		return d.applyScroll(b.String())
	}

	// Agent expansion: show agent detail.
	if len(d.expandedPath) > 0 {
		ag := d.resolveExpandedAgent(p)
		if ag != nil {
			d.renderExpandedAgent(&b, p, ag, isLive, w)
			return d.applyScroll(b.String())
		}
		d.expandedPath = nil
		d.agentCursor = -1
	}

	// Header: index, time, duration, model, tokens, context%.
	d.renderHeader(&b, p, isLive)
	b.WriteString("\n")

	// Human text.
	text := strings.TrimSpace(p.HumanText)
	if text != "" {
		if len(text) > w*3 {
			text = text[:w*3] + "..."
		}
		b.WriteString(NormalStyle.Render(text))
		b.WriteString("\n\n")
	}

	// Warnings.
	d.renderWarnings(&b, p)

	// Tool calls.
	d.renderToolCalls(&b, p)

	// Agents — with cursor for selection.
	d.renderAgentsWithCursor(&b, p, isLive)

	// File activity section.
	d.renderFileActivity(&b, p)

	return d.applyScroll(b.String())
}

// applyScroll handles vertical scrolling within the detail content.
func (d *Detail) applyScroll(content string) string {
	lines := strings.Split(content, "\n")
	visible := d.Height
	if visible <= 0 {
		visible = len(lines)
	}
	maxOffset := len(lines) - visible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if d.Offset > maxOffset {
		d.Offset = maxOffset
	}
	if d.Offset > 0 {
		lines = lines[d.Offset:]
	}
	if len(lines) > visible {
		lines = lines[:visible]
	}
	return strings.Join(lines, "\n")
}

// IsAgentFocused returns true if an agent is currently focused by the cursor.
func (d *Detail) IsAgentFocused() bool {
	return d.agentCursor >= 0
}

// IsAgentExpanded returns true if an agent is currently expanded (drilled into).
func (d *Detail) IsAgentExpanded() bool {
	return len(d.expandedPath) > 0
}

// ExpandAgent expands the currently focused agent (enter key).
// Returns true if an agent was expanded.
func (d *Detail) ExpandAgent(agents []*model.AgentNode) bool {
	if d.agentCursor < 0 || d.agentCursor >= len(agents) {
		return false
	}
	ag := agents[d.agentCursor]
	d.expandedPath = append(d.expandedPath, ag.ToolUseID)
	// Save cursor position so collapse restores it.
	d.expandedCursors = append(d.expandedCursors, d.agentCursor)
	d.agentCursor = -1
	d.Offset = 0
	return true
}

// CollapseAgent pops one level of agent expansion (esc key).
// Restores the agent cursor to the position before expansion, so the user
// lands back on the agent they just collapsed from.
func (d *Detail) CollapseAgent() bool {
	if len(d.expandedPath) == 0 {
		return false
	}
	d.expandedPath = d.expandedPath[:len(d.expandedPath)-1]
	// Restore cursor from the saved stack.
	if len(d.expandedCursors) > 0 {
		d.agentCursor = d.expandedCursors[len(d.expandedCursors)-1]
		d.expandedCursors = d.expandedCursors[:len(d.expandedCursors)-1]
	} else {
		d.agentCursor = 0
	}
	d.Offset = 0
	return true
}

// AgentCursorDown moves the agent cursor down within the agent list.
func (d *Detail) AgentCursorDown(agentCount int) {
	if agentCount == 0 {
		return
	}
	if d.agentCursor < agentCount-1 {
		d.agentCursor++
	}
}

// AgentCursorUp moves the agent cursor up. -1 means exiting agent focus.
func (d *Detail) AgentCursorUp() {
	if d.agentCursor > -1 {
		d.agentCursor--
	}
}

// ResetExpansion clears all agent expansion state.
// Called when the user navigates to a different prompt.
func (d *Detail) ResetExpansion() {
	d.expandedPath = nil
	d.expandedCursors = nil
	d.agentCursor = -1
	d.toolCallDrillIn = nil
	// Note: comparison is NOT cleared here — it persists while browsing turns.
	// Cleared explicitly by esc from comparison or ClearComparison.
	d.HasFocus = false
}

// SetComparison sets the comparison content to display.
func (d *Detail) SetComparison(content string) {
	d.comparison = content
	d.Offset = 0
}

// IsComparing returns true if a comparison view is active.
func (d *Detail) IsComparing() bool {
	return d.comparison != ""
}

// ClearComparison exits comparison mode.
func (d *Detail) ClearComparison() {
	d.comparison = ""
	d.Offset = 0
}

// DrillIntoToolCall sets the detail to show a specific tool call's full detail.
func (d *Detail) DrillIntoToolCall(tc *model.ToolCall) {
	d.toolCallDrillIn = tc
	d.Offset = 0
}

// IsDrilledIntoToolCall returns true if viewing a tool call detail.
func (d *Detail) IsDrilledIntoToolCall() bool {
	return d.toolCallDrillIn != nil
}

// ExitToolCallDrillIn returns to the turn-level or agent-level view.
func (d *Detail) ExitToolCallDrillIn() {
	d.toolCallDrillIn = nil
	d.Offset = 0
}

func (d *Detail) ScrollDown() {
	d.Offset++
	// Offset is clamped during View rendering.
}

func (d *Detail) ScrollUp() {
	if d.Offset > 0 {
		d.Offset--
	}
}

func (d *Detail) ScrollTop() {
	d.Offset = 0
}

// ScrollToBottom sets offset to show the last lines of content.
// Used by auto-follow to keep new activity visible.
func (d *Detail) ScrollToBottom() {
	// Set to a large value — applyScroll will clamp it to maxOffset.
	d.Offset = 999999
}

func (d *Detail) renderHeader(b *strings.Builder, p *model.Prompt, isLive bool) {
	// Time range.
	start := p.StartTime.Local().Format("15:04:05")
	end := "..."
	duration := ""

	isActivePrompt := p.EndTime.IsZero() && isLive
	if isActivePrompt && !p.StartTime.IsZero() {
		// Active live prompt: show running elapsed time.
		elapsed := time.Since(p.StartTime)
		duration = " " + FormatDuration(elapsed)
	} else if !p.EndTime.IsZero() {
		end = p.EndTime.Local().Format("15:04:05")
		duration = " (" + FormatDuration(p.EndTime.Sub(p.StartTime)) + ")"
	}

	header := fmt.Sprintf("#%d  %s – %s%s", p.Index+1, start, end, duration)
	if p.IsDurationOutlier {
		header += "  ⏱ slow"
	}

	b.WriteString(SelectedStyle.Render(header))
	b.WriteString("\n")

	// Model and tokens.
	var meta []string
	if p.ModelName != "" {
		meta = append(meta, p.ModelName)
	}
	if p.TokensIn > 0 || p.TokensOut > 0 {
		meta = append(meta, fmt.Sprintf("%s in / %s out",
			FormatTokens(p.TokensIn), FormatTokens(p.TokensOut)))
	}
	if p.CacheRead > 0 {
		meta = append(meta, fmt.Sprintf("cache read %s", FormatTokens(p.CacheRead)))
	}
	if p.CacheCreate > 0 {
		meta = append(meta, fmt.Sprintf("cache create %s", FormatTokens(p.CacheCreate)))
	}
	if len(meta) > 0 {
		b.WriteString(MutedStyle.Render(strings.Join(meta, "  ·  ")))
		b.WriteString("\n")
	}

	// Context%.
	if p.ContextPct > 0 {
		ctxStr := fmt.Sprintf("ctx: %d%%", p.ContextPct)
		var ctxStyle = MutedStyle
		for _, w := range p.Warnings {
			if w.Type == model.WarnContextCritical {
				ctxStyle = MutedStyle.Foreground(ColorRed)
				break
			} else if w.Type == model.WarnContextHigh {
				ctxStyle = MutedStyle.Foreground(ColorYellow)
				break
			}
		}
		b.WriteString(ctxStyle.Render(ctxStr))
		b.WriteString("\n")
	}
}

func (d *Detail) renderWarnings(b *strings.Builder, p *model.Prompt) {
	for _, w := range p.Warnings {
		switch w.Type {
		case model.WarnInterrupted:
			b.WriteString(MutedStyle.Foreground(ColorYellow).Render("⚡ " + w.Message))
			b.WriteString("\n")
		case model.WarnLoopDetected:
			b.WriteString(MutedStyle.Foreground(ColorYellow).Render("⟳ " + w.Message))
			b.WriteString("\n")
		case model.WarnAgentConflict:
			b.WriteString(MutedStyle.Foreground(ColorRed).Render("⚠ " + w.Message))
			b.WriteString("\n")
		case model.WarnAgentUnlinked:
			b.WriteString(MutedStyle.Foreground(ColorYellow).Render("⚠ " + w.Message))
			b.WriteString("\n")
		case model.WarnMCPExternal:
			// Shown inline with tool calls instead.
		case model.WarnContextHigh, model.WarnContextCritical:
			// Shown in header.
		}
	}
}

func (d *Detail) renderToolCalls(b *strings.Builder, p *model.Prompt) {
	for _, tc := range p.ToolCalls {
		if tc.Type == model.ToolAgent {
			continue // Rendered separately in agents section.
		}
		d.renderOneToolCall(b, tc, "  ")
	}
}

// renderOneToolCall renders a single tool call line with optional inline diff.
func (d *Detail) renderOneToolCall(b *strings.Builder, tc *model.ToolCall, indent string) {
	icon := toolIcon(tc.Type)
	line := icon + " "

	switch tc.Type {
	case model.ToolWrite:
		delta := fmt.Sprintf("+%d", strings.Count(tc.Content, "\n"))
		pin := ""
		if tc.IsCLAUDEMD {
			pin = " 📌"
		}
		line += fmt.Sprintf("%s %s%s", tc.Path, DimStyle.Render(delta), pin)
	case model.ToolEdit:
		added := strings.Count(tc.NewStr, "\n")
		removed := strings.Count(tc.OldStr, "\n")
		delta := fmt.Sprintf("+%d -%d", added, removed)
		pin := ""
		if tc.IsCLAUDEMD {
			pin = " 📌"
		}
		line += fmt.Sprintf("%s %s%s", tc.Path, DimStyle.Render(delta), pin)
	case model.ToolRead:
		pin := ""
		if tc.IsCLAUDEMD {
			pin = " 📌"
		}
		line += tc.Path + pin
	case model.ToolBash:
		cmd := Truncate(tc.Command, d.Width-10)
		if tc.ExitCode > 0 {
			line += MutedStyle.Foreground(ColorRed).Render(cmd)
		} else {
			line += cmd
		}
	case model.ToolMCP:
		line += MutedStyle.Foreground(ColorYellow).Render(
			fmt.Sprintf("%s/%s ⚠ external", tc.MCPServerName, tc.MCPToolName))
	case model.ToolGlob:
		line += DimStyle.Render(tc.Path)
	case model.ToolGrep:
		line += DimStyle.Render(tc.Path)
	default:
		line += DimStyle.Render(string(tc.Type))
	}

	b.WriteString(indent + line + "\n")

	// Inline mini-diff for Edit tool calls.
	if tc.Type == model.ToolEdit && tc.OldStr != "" && tc.NewStr != "" {
		miniDiff := replay.FormatMiniDiff(tc.OldStr, tc.NewStr, 4, d.Width)
		if miniDiff != "" {
			// Color the diff lines.
			for _, diffLine := range strings.Split(miniDiff, "\n") {
				if diffLine == "" {
					continue
				}
				trimmed := strings.TrimSpace(diffLine)
				if strings.HasPrefix(trimmed, "+") {
					b.WriteString(indent + MutedStyle.Foreground(ColorBrandTeal).Render(diffLine) + "\n")
				} else if strings.HasPrefix(trimmed, "-") {
					b.WriteString(indent + MutedStyle.Foreground(ColorRed).Render(diffLine) + "\n")
				} else {
					b.WriteString(indent + DimStyle.Render(diffLine) + "\n")
				}
			}
		}
	}
}

// renderAgentsWithCursor renders agents with a selection cursor for enter-to-expand.
func (d *Detail) renderAgentsWithCursor(b *strings.Builder, p *model.Prompt, isLive bool) {
	for i, agent := range p.Agents {
		isFocused := d.agentCursor == i
		d.renderOneAgent(b, agent, p, isLive, isFocused, "")
	}
}

// renderExpandedAgent renders the detailed view of an expanded agent.
func (d *Detail) renderExpandedAgent(b *strings.Builder, p *model.Prompt, ag *model.AgentNode, isLive bool, w int) {
	// Breadcrumb: #N > subagent-1 > subagent-1a
	crumbs := fmt.Sprintf("#%d", p.Index+1)
	agents := p.Agents
	for i, toolUseID := range d.expandedPath {
		for _, a := range agents {
			if a.ToolUseID == toolUseID {
				crumbs += " > " + a.Label
				if i < len(d.expandedPath)-1 {
					agents = a.Children
				}
				break
			}
		}
	}
	b.WriteString(SelectedStyle.Render(crumbs))
	b.WriteString("\n\n")

	// Agent header.
	typeLabel := ag.SubagentType
	if typeLabel == "" {
		typeLabel = "agent"
	}
	if ag.ModelName != "" && ag.ModelName != p.ModelName {
		typeLabel += ", " + shortModel(ag.ModelName)
	}

	statusStr := string(ag.Status)
	switch ag.Status {
	case model.AgentRunning:
		if !ag.StartTime.IsZero() && isLive {
			statusStr = "running — " + FormatDuration(time.Since(ag.StartTime))
		}
	case model.AgentSucceeded:
		statusStr = "done"
		if ag.Duration > 0 {
			statusStr += " — " + FormatDuration(ag.Duration)
		}
	case model.AgentFailed:
		statusStr = "✗ failed"
		if ag.Duration > 0 {
			statusStr += " — " + FormatDuration(ag.Duration)
		}
	}
	b.WriteString(fmt.Sprintf("  ⬡ %s (%s) — %s\n", ag.Label, typeLabel, statusStr))

	// Badges.
	var badges []string
	if ag.IsParallel {
		badges = append(badges, "parallel")
	}
	if len(badges) > 0 {
		b.WriteString("  " + DimStyle.Render("["+strings.Join(badges, ", ")+"]") + "\n")
	}
	b.WriteString("\n")

	// Task description.
	if ag.TaskDescription != "" {
		b.WriteString(MutedStyle.Render("  Asked: ") + Truncate(ag.TaskDescription, w-10) + "\n")
	}

	// Scope summary.
	if len(ag.FilesTouched) > 0 {
		b.WriteString(MutedStyle.Render(fmt.Sprintf("  Touched: %d files", len(ag.FilesTouched))) + "\n")
		for _, path := range ag.FilesTouched {
			// Count operations per file.
			var ops []string
			wc, rc, ec := 0, 0, 0
			for _, tc := range ag.ToolCalls {
				if tc.Path != path {
					continue
				}
				switch tc.Type {
				case model.ToolWrite:
					wc++
				case model.ToolRead:
					rc++
				case model.ToolEdit:
					ec++
				}
			}
			if wc > 0 {
				ops = append(ops, fmt.Sprintf("W×%d", wc))
			}
			if rc > 0 {
				ops = append(ops, fmt.Sprintf("R×%d", rc))
			}
			if ec > 0 {
				ops = append(ops, fmt.Sprintf("E×%d", ec))
			}
			opStr := ""
			if len(ops) > 0 {
				opStr = " " + DimStyle.Render("("+strings.Join(ops, " ")+")")
			}
			b.WriteString("    " + Truncate(path, w-20) + opStr + "\n")
		}
		b.WriteString("\n")
	}

	// Tokens.
	if ag.TokensIn > 0 || ag.TokensOut > 0 {
		b.WriteString(MutedStyle.Render(fmt.Sprintf("  Tokens: %s in / %s out",
			FormatTokens(ag.TokensIn), FormatTokens(ag.TokensOut))))
		b.WriteString("\n\n")
	}

	// Full tool call list.
	if len(ag.ToolCalls) > 0 {
		b.WriteString(MutedStyle.Render("  Tool calls:") + "\n")
		for _, tc := range ag.ToolCalls {
			icon := toolIcon(tc.Type)
			line := icon + " "
			switch tc.Type {
			case model.ToolWrite:
				delta := fmt.Sprintf("+%d", strings.Count(tc.Content, "\n"))
				line += fmt.Sprintf("%s %s", tc.Path, DimStyle.Render(delta))
			case model.ToolEdit:
				added := strings.Count(tc.NewStr, "\n")
				removed := strings.Count(tc.OldStr, "\n")
				line += fmt.Sprintf("%s %s", tc.Path, DimStyle.Render(fmt.Sprintf("+%d -%d", added, removed)))
			case model.ToolRead:
				line += tc.Path
			case model.ToolBash:
				line += Truncate(tc.Command, w-15)
			case model.ToolGlob, model.ToolGrep:
				line += DimStyle.Render(tc.Path)
			case model.ToolMCP:
				line += fmt.Sprintf("%s/%s", tc.MCPServerName, tc.MCPToolName)
			default:
				line += DimStyle.Render(string(tc.Type))
			}
			b.WriteString("    " + line + "\n")
		}
	} else if ag.Status == model.AgentRunning {
		b.WriteString(DimStyle.Render("  Waiting for tool calls...") + "\n")
	}

	// Nested agents.
	if len(ag.Children) > 0 {
		b.WriteString("\n" + MutedStyle.Render("  Nested agents:") + "\n")
		for i, child := range ag.Children {
			isFocused := d.agentCursor == i
			d.renderOneAgent(b, child, p, isLive, isFocused, "  ")
		}
	}
}

// resolveExpandedAgent walks the expansion path to find the target agent.
func (d *Detail) resolveExpandedAgent(p *model.Prompt) *model.AgentNode {
	agents := p.Agents
	var found *model.AgentNode
	for _, toolUseID := range d.expandedPath {
		found = nil
		for _, ag := range agents {
			if ag.ToolUseID == toolUseID {
				found = ag
				agents = ag.Children
				break
			}
		}
		if found == nil {
			return nil // Path is stale.
		}
	}
	return found
}

// renderOneAgent renders a single agent line with optional focus indicator.
func (d *Detail) renderOneAgent(b *strings.Builder, agent *model.AgentNode, p *model.Prompt, isLive bool, isFocused bool, indent string) {
	agentType := agent.SubagentType
	if agentType == "" {
		agentType = "agent"
	}

	typeLabel := agentType
	if agent.ModelName != "" && agent.ModelName != p.ModelName {
		typeLabel += ", " + shortModel(agent.ModelName)
	}

	desc := Truncate(agent.TaskDescription, d.Width-30)

	var suffixes []string
	if agent.IsParallel {
		suffixes = append(suffixes, "parallel")
	}
	suffix := ""
	if len(suffixes) > 0 {
		suffix = "  " + DimStyle.Render("["+strings.Join(suffixes, ", ")+"]")
	}

	// Focus indicator.
	cursor := "  "
	if isFocused {
		cursor = SelectedStyle.Render("> ")
	}
	prefix := indent + cursor

	switch agent.Status {
	case model.AgentRunning:
		var parts []string
		if n := len(agent.ToolCalls); n > 0 {
			parts = append(parts, fmt.Sprintf("%d tools", n))
		}
		if !agent.StartTime.IsZero() && isLive {
			parts = append(parts, FormatDuration(time.Since(agent.StartTime)))
		}
		meta := ""
		if len(parts) > 0 {
			meta = " — " + strings.Join(parts, " — ")
		}
		b.WriteString(fmt.Sprintf("%s⬡ %s (%s) — running%s%s\n",
			prefix, agent.Label, typeLabel, DimStyle.Render(meta), suffix))
		// Show latest tool call inline so you see what the agent is doing now.
		if n := len(agent.ToolCalls); n > 0 {
			latest := agent.ToolCalls[n-1]
			latestIcon := toolIcon(latest.Type)
			latestPath := latest.Path
			if latest.Type == model.ToolBash {
				latestPath = Truncate(latest.Command, d.Width-20)
			}
			if latestPath == "" {
				latestPath = string(latest.Type)
			}
			b.WriteString(indent + "    " + DimStyle.Render("→ "+latestIcon+" "+latestPath) + "\n")
		} else if desc != "" {
			b.WriteString(indent + "    " + DimStyle.Render(desc) + "\n")
		}

	case model.AgentSucceeded:
		meta := ""
		if agent.TotalToolUseCount > 0 || len(agent.ToolCalls) > 0 {
			toolCount := agent.TotalToolUseCount
			if toolCount == 0 {
				toolCount = len(agent.ToolCalls)
			}
			fileCount := len(agent.FilesTouched)
			meta = fmt.Sprintf(" — %d tools, %d files, %s",
				toolCount, fileCount,
				FormatDuration(agent.Duration))
		}
		b.WriteString(fmt.Sprintf("%s⬡ ✓ %s (%s) — done%s%s\n",
			prefix, agent.Label, typeLabel, DimStyle.Render(meta), suffix))

	case model.AgentFailed:
		meta := ""
		if agent.TotalToolUseCount > 0 {
			meta = fmt.Sprintf(" — %d tools, %s",
				agent.TotalToolUseCount,
				FormatDuration(agent.Duration))
		}
		b.WriteString(fmt.Sprintf("%s⬡ ✗ %s (%s) — failed%s%s\n",
			prefix, agent.Label, typeLabel, MutedStyle.Foreground(ColorRed).Render(meta), suffix))

	default:
		b.WriteString(fmt.Sprintf("%s⬡ %s (%s) — %s%s\n",
			prefix, agent.Label, typeLabel, DimStyle.Render(desc), suffix))
	}
}

// renderToolCallDetail renders the full detail for a drilled-into tool call.
func (d *Detail) renderToolCallDetail(b *strings.Builder, p *model.Prompt, tc *model.ToolCall, w int) {
	// Breadcrumb.
	crumb := fmt.Sprintf("#%d > %s %s", p.Index+1, toolIcon(tc.Type), Truncate(tc.Path, w-20))
	if tc.Type == model.ToolBash {
		crumb = fmt.Sprintf("#%d > $ %s", p.Index+1, Truncate(tc.Command, w-20))
	}
	b.WriteString(SelectedStyle.Render(crumb))
	b.WriteString("\n\n")

	switch tc.Type {
	case model.ToolEdit:
		b.WriteString(MutedStyle.Render("  File: ") + tc.Path + "\n\n")
		if tc.OldStr != "" && tc.NewStr != "" {
			renderColoredDiff(b, replay.FormatFullDiff(tc.OldStr, tc.NewStr, w))
		}

	case model.ToolWrite:
		b.WriteString(MutedStyle.Render("  File: ") + tc.Path + "\n")
		lineCount := strings.Count(tc.Content, "\n")
		b.WriteString(MutedStyle.Render(fmt.Sprintf("  Lines: %d", lineCount)) + "\n\n")
		if tc.Content != "" {
			// Show first ~20 lines.
			lines := strings.Split(tc.Content, "\n")
			shown := min(20, len(lines))
			for _, line := range lines[:shown] {
				b.WriteString("  " + DimStyle.Render(Truncate(line, w-4)) + "\n")
			}
			if len(lines) > shown {
				b.WriteString(DimStyle.Render(fmt.Sprintf("  ... +%d more lines", len(lines)-shown)) + "\n")
			}
		}

	case model.ToolBash:
		b.WriteString(MutedStyle.Render("  Command:") + "\n")
		b.WriteString("  " + tc.Command + "\n\n")
		if tc.ExitCode >= 0 {
			exitStyle := MutedStyle
			if tc.ExitCode > 0 {
				exitStyle = MutedStyle.Foreground(ColorRed)
			}
			b.WriteString(exitStyle.Render(fmt.Sprintf("  Exit code: %d", tc.ExitCode)) + "\n")
		}
		if tc.Output != "" {
			b.WriteString(MutedStyle.Render("  Output:") + "\n")
			for _, line := range strings.Split(tc.Output, "\n") {
				b.WriteString("  " + DimStyle.Render(Truncate(line, w-4)) + "\n")
			}
		}

	case model.ToolRead:
		b.WriteString(MutedStyle.Render("  File: ") + tc.Path + "\n")
		if tc.Content != "" {
			lineCount := strings.Count(tc.Content, "\n")
			b.WriteString(MutedStyle.Render(fmt.Sprintf("  Lines: %d", lineCount)) + "\n")
		}

	default:
		b.WriteString(MutedStyle.Render(fmt.Sprintf("  Type: %s", tc.Type)) + "\n")
		if tc.Path != "" {
			b.WriteString(MutedStyle.Render("  Path: ") + tc.Path + "\n")
		}
	}
}

// renderFileActivity renders the file activity section at the bottom of turn detail.
func (d *Detail) renderFileActivity(b *strings.Builder, p *model.Prompt) {
	// Collect all files touched in this turn (parent + agents).
	type fileEntry struct {
		path       string
		writes     int
		reads      int
		edits      int
		agents     []string // agent labels that touched this file
	}
	files := make(map[string]*fileEntry)

	addOp := func(path string, t model.ToolType, agentLabel string) {
		if path == "" {
			return
		}
		fe, ok := files[path]
		if !ok {
			fe = &fileEntry{path: path}
			files[path] = fe
		}
		switch t {
		case model.ToolWrite:
			fe.writes++
		case model.ToolRead:
			fe.reads++
		case model.ToolEdit:
			fe.edits++
		}
		if agentLabel != "" {
			found := false
			for _, a := range fe.agents {
				if a == agentLabel {
					found = true
					break
				}
			}
			if !found {
				fe.agents = append(fe.agents, agentLabel)
			}
		}
	}

	for _, tc := range p.ToolCalls {
		if tc.Type == model.ToolWrite || tc.Type == model.ToolRead || tc.Type == model.ToolEdit {
			addOp(tc.Path, tc.Type, "")
		}
	}
	for _, ag := range p.Agents {
		for _, tc := range ag.ToolCalls {
			if tc.Type == model.ToolWrite || tc.Type == model.ToolRead || tc.Type == model.ToolEdit {
				addOp(tc.Path, tc.Type, ag.Label)
			}
		}
	}

	if len(files) == 0 {
		return
	}

	// Sort by activity (writes + edits first), then alphabetically for stability.
	type sortEntry struct {
		*fileEntry
		heat int
	}
	var sorted []sortEntry
	for _, fe := range files {
		sorted = append(sorted, sortEntry{fe, fe.writes + fe.edits})
	}
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0; j-- {
			if sorted[j].heat > sorted[j-1].heat ||
				(sorted[j].heat == sorted[j-1].heat && sorted[j].path < sorted[j-1].path) {
				sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
			} else {
				break
			}
		}
	}

	b.WriteString("\n")
	b.WriteString(MutedStyle.Render("  Files:") + "\n")
	for _, se := range sorted {
		var ops []string
		if se.writes > 0 {
			ops = append(ops, fmt.Sprintf("W×%d", se.writes))
		}
		if se.edits > 0 {
			ops = append(ops, fmt.Sprintf("E×%d", se.edits))
		}
		if se.reads > 0 {
			ops = append(ops, fmt.Sprintf("R×%d", se.reads))
		}
		opStr := DimStyle.Render("(" + strings.Join(ops, " ") + ")")

		agentStr := ""
		if len(se.agents) > 0 {
			agentStr = " " + DimStyle.Render("⬡ "+strings.Join(se.agents, ", "))
		}

		path := Truncate(se.path, d.Width-30)
		b.WriteString("    " + path + " " + opStr + agentStr + "\n")
	}
}

// renderColoredDiff writes diff lines with add=teal, del=red, context=dim.
func renderColoredDiff(b *strings.Builder, diffText string) {
	for _, line := range strings.Split(diffText, "\n") {
		if line == "" {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "+") {
			b.WriteString(MutedStyle.Foreground(ColorBrandTeal).Render(line) + "\n")
		} else if strings.HasPrefix(trimmed, "-") {
			b.WriteString(MutedStyle.Foreground(ColorRed).Render(line) + "\n")
		} else {
			b.WriteString(DimStyle.Render(line) + "\n")
		}
	}
}

// shortModel extracts a short form like "haiku" or "sonnet" from a model ID.
func shortModel(m string) string {
	lower := strings.ToLower(m)
	for _, name := range []string{"opus", "sonnet", "haiku"} {
		if strings.Contains(lower, name) {
			return name
		}
	}
	if m != "" {
		return m
	}
	return ""
}

func toolIcon(t model.ToolType) string {
	switch t {
	case model.ToolWrite:
		return "W"
	case model.ToolEdit:
		return "E"
	case model.ToolRead:
		return "R"
	case model.ToolBash:
		return "$"
	case model.ToolGlob:
		return "G"
	case model.ToolGrep:
		return "/"
	case model.ToolMCP:
		return "⚡"
	case model.ToolWebSearch:
		return "🔍"
	default:
		return "·"
	}
}
