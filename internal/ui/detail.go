package ui

import (
	"fmt"
	"strings"

	"github.com/kno-ai/kno-trace/internal/model"
)

// Detail renders the right-pane detail view for a selected prompt.
type Detail struct {
	Width  int
	Height int
	Offset int // scroll offset for long content
}

// View renders the detail pane for the given prompt. Returns empty if nil.
func (d *Detail) View(p *model.Prompt) string {
	if p == nil {
		return MutedStyle.Render("Select a prompt")
	}

	var b strings.Builder
	w := d.Width
	if w <= 0 {
		w = 50
	}

	// Header: index, time, duration, model, tokens, context%.
	d.renderHeader(&b, p)
	b.WriteString("\n")

	// Human text.
	text := strings.TrimSpace(p.HumanText)
	if text != "" {
		// Wrap long text to width.
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

	// Agents (collapsed).
	d.renderAgents(&b, p)

	content := b.String()

	// Apply scroll offset, clamped to content bounds.
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

func (d *Detail) renderHeader(b *strings.Builder, p *model.Prompt) {
	// Time range.
	start := p.StartTime.Local().Format("15:04:05")
	end := "..."
	if !p.EndTime.IsZero() {
		end = p.EndTime.Local().Format("15:04:05")
	}
	duration := ""
	if !p.EndTime.IsZero() {
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

		b.WriteString("  " + line + "\n")
	}
}

func (d *Detail) renderAgents(b *strings.Builder, p *model.Prompt) {
	for _, agent := range p.Agents {
		status := ""
		switch agent.Status {
		case model.AgentSucceeded:
			status = "✓"
		case model.AgentFailed:
			status = "✗"
		case model.AgentRunning:
			status = "…"
		}

		agentType := agent.SubagentType
		if agentType == "" {
			agentType = "agent"
		}

		desc := Truncate(agent.TaskDescription, d.Width-30)
		meta := ""
		if agent.TotalToolUseCount > 0 {
			meta = fmt.Sprintf(" — %d tools, %s tokens, %s",
				agent.TotalToolUseCount,
				FormatTokens(agent.TotalTokens),
				FormatDuration(agent.Duration))
		}

		line := fmt.Sprintf("  ⬡ %s %s (%s) — %s%s",
			status, agent.Label, agentType, desc, DimStyle.Render(meta))
		b.WriteString(line + "\n")
	}
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
