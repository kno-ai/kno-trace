// Package agent builds and enriches the agent tree from subagent JSONL files.
// After the parser creates AgentNode stubs (from the parent session), the tree
// builder reads each agent's dedicated JSONL file to populate ToolCalls,
// FilesTouched, and model info. It also detects file conflicts between parallel
// agents and resolves linkage failures gracefully.
package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kno-ai/kno-trace/internal/config"
	"github.com/kno-ai/kno-trace/internal/model"
	"github.com/kno-ai/kno-trace/internal/parser"
)

// EnrichSession reads subagent JSONL files and populates agent nodes in place.
// sessionDir is the directory containing the parent JSONL file (e.g.,
// ~/.claude/projects/<encoded>/) and sessionID is Session.ID (the UUID used
// as the subdirectory name for subagent files).
//
// For each agent with an ID, it looks for:
//
//	<sessionDir>/<sessionID>/subagents/agent-a<agentID>.jsonl
//
// Agents whose files cannot be found are left with their existing summary data
// from toolUseResult. Agents whose IDs cannot be resolved are moved to
// Session.UnlinkedAgents.
func EnrichSession(s *model.Session, sessionDir string, cfg *config.Config) {
	if s == nil {
		return
	}

	for _, prompt := range s.Prompts {
		enrichAgents(s, prompt.Agents, sessionDir, s.ID, cfg)
	}

	detectParallelAgents(s)
	detectFileConflicts(s)
}

// enrichAgents populates agent nodes recursively (handles nested agents).
func enrichAgents(s *model.Session, agents []*model.AgentNode, sessionDir, sessionID string, cfg *config.Config) {
	var unlinked []*model.AgentNode

	for _, agent := range agents {
		if agent.ID == "" {
			// No agentId resolved from toolUseResult — can't find the file.
			if agent.Status != model.AgentRunning {
				// Only flag completed agents as unlinked; running agents
				// may get their ID when the tool_result arrives.
				unlinked = append(unlinked, agent)
			}
			continue
		}

		subagentPath := SubagentFilePath(sessionDir, sessionID, agent.ID)
		if err := EnrichFromFile(agent, subagentPath, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "kno-trace: agent %s: %v\n", agent.ID, err)
			// Leave existing summary data from toolUseResult intact.
		}

		// Recurse into nested agents.
		if len(agent.Children) > 0 {
			enrichAgents(s, agent.Children, sessionDir, sessionID, cfg)
		}
	}

	if len(unlinked) > 0 {
		for _, agent := range unlinked {
			agent.Status = model.AgentFailed // Mark so UI can distinguish
			s.UnlinkedAgents = append(s.UnlinkedAgents, agent)
		}
		// Add a session-level warning for each prompt that had unlinked agents.
		if len(s.Prompts) > 0 {
			for _, agent := range unlinked {
				if agent.ParentPromptIdx < len(s.Prompts) {
					prompt := s.Prompts[agent.ParentPromptIdx]
					prompt.Warnings = append(prompt.Warnings, model.Warning{
						Type:    model.WarnAgentUnlinked,
						Message: fmt.Sprintf("agent %q could not be linked — no agentId in result", agent.Label),
					})
				}
			}
		}
	}
}

// SubagentFilePath returns the expected path of a subagent JSONL file.
// Claude Code names subagent files as agent-<agentId>.jsonl where the agentId
// is a hex string (the leading 'a' in filenames like agent-ab502... is part of
// the hex ID, not a separate prefix).
func SubagentFilePath(sessionDir, sessionID, agentID string) string {
	return filepath.Join(sessionDir, sessionID, "subagents", "agent-"+agentID+".jsonl")
}

// SubagentsDir returns the directory where subagent files live for a session.
func SubagentsDir(sessionDir, sessionID string) string {
	return filepath.Join(sessionDir, sessionID, "subagents")
}

// EnrichFromFile parses a subagent JSONL file and populates the agent node.
// Exported for use by the UI when a live agent completes (enriches in place).
func EnrichFromFile(agent *model.AgentNode, path string, cfg *config.Config) error {
	events, err := parser.ParseFile(path, cfg)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("subagent file not found: %s", path)
		}
		return fmt.Errorf("parsing subagent file: %w", err)
	}

	return enrichFromEvents(agent, events)
}

// enrichFromEvents populates an agent node from parsed subagent events.
// Exported for use by the live subagent tailer (layer 2).
func enrichFromEvents(agent *model.AgentNode, events []*parser.RawEvent) error {
	var agentCounter int
	toolCallsByID := make(map[string]*parser.ContentBlock)

	for _, evt := range events {
		switch evt.Type {
		case "assistant":
			if evt.Message == nil {
				continue
			}

			// Capture model from first assistant message with one.
			if agent.ModelName == "" && evt.Message.Model != "" {
				agent.ModelName = evt.Message.Model
			}

			// Accumulate per-message token counts. These provide granular
			// in/out breakdowns. TotalTokens (from toolUseResult) is the
			// authoritative aggregate — use it for summary display.
			if evt.Message.Usage != nil {
				agent.TokensIn += evt.Message.Usage.InputTokens
				agent.TokensOut += evt.Message.Usage.OutputTokens
			}

			// Extract tool_use blocks.
			for _, block := range evt.Message.Content {
				if block.Type != "tool_use" {
					continue
				}

				tc := buildAgentToolCall(block, evt.Timestamp, agent.ID, &agentCounter)
				agent.ToolCalls = append(agent.ToolCalls, tc)

				// Track tool_use for result pairing.
				b := block // capture
				toolCallsByID[block.ToolUseID] = &b
			}

		case "user":
			if evt.Message == nil {
				continue
			}
			// Pair tool results — extract paths from Read results too.
			for _, block := range evt.Message.Content {
				if block.Type != "tool_result" {
					continue
				}
				if _, ok := toolCallsByID[block.ToolResultID]; ok {
					// Find the corresponding ToolCall and populate result fields.
					for _, tc := range agent.ToolCalls {
						if tc.ID == block.ToolResultID {
							parser.PopulateToolResult(tc, evt.ToolUseResult, block.IsError)
							break
						}
					}
				}
			}
		}
	}

	// Build deduplicated FilesTouched from tool calls that reference actual files.
	// Grep/Glob patterns are search queries, not file paths — exclude them.
	seen := make(map[string]bool)
	for _, tc := range agent.ToolCalls {
		if tc.Path != "" && IsFilePath(tc.Type) && !seen[tc.Path] {
			agent.FilesTouched = append(agent.FilesTouched, tc.Path)
			seen[tc.Path] = true
		}
	}

	return nil
}

// buildAgentToolCall creates a ToolCall from a subagent's tool_use content block.
func buildAgentToolCall(block parser.ContentBlock, ts time.Time, agentID string, counter *int) *model.ToolCall {
	tc := &model.ToolCall{
		ID:          block.ToolUseID,
		SourceAgent: agentID,
		Timestamp:   ts,
		ExitCode:    -1,
	}

	tc.Type, tc.MCPServerName, tc.MCPToolName = parser.ClassifyToolName(block.ToolName)

	// Parse tool-specific input fields — reuses the parser's exported function
	// to avoid duplication.
	if len(block.ToolInput) > 0 {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(block.ToolInput, &fields); err == nil {
			parser.ParseToolInput(tc, fields, counter)
		}
	}

	return tc
}

// isWriteOp returns true for tool types that modify files.
func isWriteOp(t model.ToolType) bool {
	return t == model.ToolWrite || t == model.ToolEdit
}

// IsFilePath returns true for tool types whose Path field is an actual file path
// (not a search pattern). Grep and Glob store patterns in Path for display
// convenience, but those are not files the agent "touched."
func IsFilePath(t model.ToolType) bool {
	return t == model.ToolWrite || t == model.ToolRead || t == model.ToolEdit
}

// detectParallelAgents marks agents that ran concurrently within each prompt.
// Agent B is parallel with Agent A if B was spawned (StartTime) before A
// completed (EndTime). Both agents are marked IsParallel.
func detectParallelAgents(s *model.Session) {
	for _, prompt := range s.Prompts {
		if len(prompt.Agents) < 2 {
			continue
		}
		for i := 0; i < len(prompt.Agents); i++ {
			for j := i + 1; j < len(prompt.Agents); j++ {
				a, b := prompt.Agents[i], prompt.Agents[j]
				// Parallel if either was spawned before the other completed.
				aRunning := a.EndTime.IsZero() || b.StartTime.Before(a.EndTime)
				bRunning := b.EndTime.IsZero() || a.StartTime.Before(b.EndTime)
				if aRunning && bRunning {
					a.IsParallel = true
					b.IsParallel = true
				}
			}
		}
	}
}

// detectFileConflicts scans parallel agents within each prompt for shared file
// paths and emits WarnAgentConflict warnings. Only flags conflicts between
// agents that are confirmed parallel (IsParallel == true) and where at least
// one agent performed a write/edit on the shared path.
func detectFileConflicts(s *model.Session) {
	for _, prompt := range s.Prompts {
		if len(prompt.Agents) < 2 {
			continue
		}

		// Build map of file → agents that touched it, tracking if any wrote.
		type fileInfo struct {
			agents    []string // agent labels
			hasWriter bool
		}
		files := make(map[string]*fileInfo)

		for _, agent := range prompt.Agents {
			if !agent.IsParallel {
				continue
			}
			wroteFiles := make(map[string]bool)
			for _, tc := range agent.ToolCalls {
				if tc.Path != "" && isWriteOp(tc.Type) {
					wroteFiles[tc.Path] = true
				}
			}
			for _, path := range agent.FilesTouched {
				fi, ok := files[path]
				if !ok {
					fi = &fileInfo{}
					files[path] = fi
				}
				fi.agents = append(fi.agents, agent.Label)
				if wroteFiles[path] {
					fi.hasWriter = true
				}
			}
		}

		// Emit warnings for files touched by multiple parallel agents
		// where at least one performed a write.
		for path, fi := range files {
			if len(fi.agents) > 1 && fi.hasWriter {
				prompt.Warnings = append(prompt.Warnings, model.Warning{
					Type: model.WarnAgentConflict,
					Message: fmt.Sprintf("file conflict: %s touched by %s",
						path, strings.Join(fi.agents, ", ")),
				})
			}
		}
	}
}
