package parser

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kno-ai/kno-trace/internal/config"
	"github.com/kno-ai/kno-trace/internal/model"
)

// classifySession runs post-parse classification on a fully built session.
// Sets IsCLAUDEMD, generates warnings, detects loops.
func classifySession(s *model.Session, cfg *config.Config) {
	for _, p := range s.Prompts {
		classifyPrompt(p, cfg)
	}
}

// classifyPrompt classifies tool calls and generates warnings for a single prompt.
func classifyPrompt(p *model.Prompt, cfg *config.Config) {
	// Track tool+path repetitions for loop detection.
	type toolPathKey struct {
		toolType model.ToolType
		path     string
	}
	loopCounts := make(map[toolPathKey]int)

	for _, tc := range p.ToolCalls {
		// Set IsCLAUDEMD.
		if tc.Path != "" {
			tc.IsCLAUDEMD = isCLAUDEMDPath(tc.Path)
		}

		// MCP warning.
		if tc.Type == model.ToolMCP {
			p.Warnings = append(p.Warnings, model.Warning{
				Type:    model.WarnMCPExternal,
				Message: fmt.Sprintf("MCP tool call: %s/%s", tc.MCPServerName, tc.MCPToolName),
			})
		}

		// Loop detection: count tool+path pairs.
		if tc.Path != "" {
			key := toolPathKey{tc.Type, tc.Path}
			loopCounts[key]++
			if loopCounts[key] == cfg.LoopDetectionThreshold {
				p.Warnings = append(p.Warnings, model.Warning{
					Type:    model.WarnLoopDetected,
					Message: fmt.Sprintf("%s %s repeated %d×", tc.Type, tc.Path, loopCounts[key]),
				})
			}
		}
	}

	// Context warnings.
	if p.ContextPct > 0 {
		if p.ContextPct >= cfg.ContextCriticalPct {
			p.Warnings = append(p.Warnings, model.Warning{
				Type:    model.WarnContextCritical,
				Message: fmt.Sprintf("context at %d%%", p.ContextPct),
			})
		} else if p.ContextPct >= cfg.ContextHighPct {
			p.Warnings = append(p.Warnings, model.Warning{
				Type:    model.WarnContextHigh,
				Message: fmt.Sprintf("context at %d%%", p.ContextPct),
			})
		}
	}

	// Interrupted warning.
	if p.Interrupted {
		p.Warnings = append(p.Warnings, model.Warning{
			Type:    model.WarnInterrupted,
			Message: "session interrupted during this prompt",
		})
	}
}

// isCLAUDEMDPath checks if a file path matches the CLAUDE.md patterns.
// Matches: CLAUDE.md, AGENTS.md, .claude/memory*, .claude/settings*, .claude/commands*
func isCLAUDEMDPath(path string) bool {
	base := filepath.Base(path)
	lower := strings.ToLower(base)

	if lower == "claude.md" || lower == "agents.md" {
		return true
	}

	// Check .claude/ directory patterns.
	// Paths may be relative (.claude/memory/foo.md) or absolute (/home/user/.claude/settings.json).
	normalized := filepath.ToSlash(path)
	const prefix = ".claude/"
	idx := strings.Index(normalized, prefix)
	if idx >= 0 {
		after := normalized[idx+len(prefix):]
		if strings.HasPrefix(after, "memory") ||
			strings.HasPrefix(after, "settings") ||
			strings.HasPrefix(after, "commands") {
			return true
		}
	}

	return false
}
