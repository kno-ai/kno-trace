package parser

import (
	"encoding/json"
	"math"
	"strings"
	"time"

	"github.com/kno-ai/kno-trace/internal/config"
	"github.com/kno-ai/kno-trace/internal/model"
)

// BuildSession assembles a slice of RawEvents into a structured Session.
// Events must be pre-sorted by timestamp (ParseFile/ParseReader handles this).
func BuildSession(events []*RawEvent, cfg *config.Config) *model.Session {
	s := &model.Session{}

	var (
		currentPrompt        *model.Prompt
		promptIdx            int
		lastBranch           string
		agentCounter         int
		hasAssistantResponse bool // tracks if current prompt got any assistant message
	)

	// Map tool_use IDs to their ToolCall for result pairing.
	toolCallsByID := make(map[string]*model.ToolCall)

	for _, evt := range events {
		// Track session-level fields from first event with them.
		if s.ID == "" && evt.SessionID != "" {
			s.ID = evt.SessionID
		}
		if s.StartTime.IsZero() && !evt.Timestamp.IsZero() {
			s.StartTime = evt.Timestamp
		}
		if !evt.Timestamp.IsZero() {
			s.EndTime = evt.Timestamp
		}

		switch evt.Type {
		case "user":
			if evt.Message == nil {
				continue
			}
			if isHumanTurn(evt) {
				// Seal previous prompt.
				if currentPrompt != nil {
					currentPrompt.EndTime = evt.Timestamp
				}

				currentPrompt = &model.Prompt{
					Index:     promptIdx,
					StartTime: evt.Timestamp,
					HumanText: evt.Message.HumanText,
				}
				promptIdx++
				hasAssistantResponse = false
				s.Prompts = append(s.Prompts, currentPrompt)

				// Track git branch transitions.
				if evt.GitBranch != "" {
					if lastBranch != "" && evt.GitBranch != lastBranch {
						currentPrompt.BranchTransition = model.BranchTransition{
							From: lastBranch,
							To:   evt.GitBranch,
						}
					}
					lastBranch = evt.GitBranch
				}
			} else if isToolResult(evt) && currentPrompt != nil {
				// Pair tool result with its tool_use.
				pairToolResult(evt, toolCallsByID, currentPrompt)
			}

		case "assistant":
			if evt.Message == nil || currentPrompt == nil {
				continue
			}
			hasAssistantResponse = true

			// Track branch transitions on assistant lines too.
			if evt.GitBranch != "" && lastBranch != "" && evt.GitBranch != lastBranch {
				currentPrompt.BranchTransition = model.BranchTransition{
					From: lastBranch,
					To:   evt.GitBranch,
				}
				lastBranch = evt.GitBranch
			} else if evt.GitBranch != "" {
				lastBranch = evt.GitBranch
			}

			// Extract model and usage from the last assistant message in this prompt.
			if evt.Message.Model != "" {
				currentPrompt.ModelName = evt.Message.Model
				if s.ModelName == "" {
					s.ModelName = evt.Message.Model
				}
			}
			if evt.Message.Usage != nil {
				currentPrompt.TokensIn = evt.Message.Usage.InputTokens
				currentPrompt.TokensOut = evt.Message.Usage.OutputTokens
				currentPrompt.CacheRead = evt.Message.Usage.CacheReadInputTokens
				currentPrompt.CacheCreate = evt.Message.Usage.CacheCreationInputTokens

				// Compute ContextPct.
				if evt.Message.Usage.InputTokens > 0 && currentPrompt.ModelName != "" {
					windowSize := cfg.ContextWindowSize(currentPrompt.ModelName)
					if windowSize > 0 {
						currentPrompt.ContextPct = int(float64(evt.Message.Usage.InputTokens) / float64(windowSize) * 100)
					}
				}
			}

			// Extract tool_use blocks as ToolCalls.
			for _, block := range evt.Message.Content {
				if block.Type != "tool_use" {
					continue
				}
				tc := buildToolCall(block, evt.Timestamp, &agentCounter)
				currentPrompt.ToolCalls = append(currentPrompt.ToolCalls, tc)
				toolCallsByID[tc.ID] = tc

				// If this is an Agent tool_use, create an AgentNode.
				if tc.Type == model.ToolAgent {
					agent := &model.AgentNode{
						ToolUseID:       tc.ID,
						Label:           tc.AgentDescription,
						SubagentType:    tc.SubagentType,
						TaskDescription: tc.AgentDescription,
						TaskPrompt:      tc.AgentPrompt,
						ParentPromptIdx: currentPrompt.Index,
						StartTime:       evt.Timestamp,
						Status:          model.AgentRunning,
					}
					currentPrompt.Agents = append(currentPrompt.Agents, agent)
				}
			}

		case "system":
			if evt.Subtype == "compact_boundary" && currentPrompt != nil {
				s.CompactAt = append(s.CompactAt, currentPrompt.Index)
			}
			// turn_duration — could be used for timing but we derive from timestamps.

		case "progress":
			// Agent progress — noted but not fully processed until M5.
		}
	}

	// Handle interrupted session: last prompt has no assistant response.
	if currentPrompt != nil && !hasAssistantResponse {
		currentPrompt.Interrupted = true
		s.Interrupted = true
	}

	// Post-processing.
	classifySession(s, cfg)
	computeDurationOutliers(s)

	return s
}

// isHumanTurn checks if a user event is a human turn (not a tool result).
// Human turns have message.content as a string, or content[0].type == "text".
func isHumanTurn(evt *RawEvent) bool {
	if evt.Message == nil {
		return false
	}
	// If HumanText was set from a string content, it's a human turn.
	if evt.Message.HumanText != "" && evt.SourceToolAssistantUUID == "" {
		return true
	}
	// If content is an array, check first block type.
	if len(evt.Message.Content) > 0 {
		return evt.Message.Content[0].Type == "text"
	}
	return false
}

// isToolResult checks if a user event is a tool result.
func isToolResult(evt *RawEvent) bool {
	if evt.Message == nil {
		return false
	}
	if evt.SourceToolAssistantUUID != "" {
		return true
	}
	if len(evt.Message.Content) > 0 {
		return evt.Message.Content[0].Type == "tool_result"
	}
	return false
}

// buildToolCall creates a ToolCall from a tool_use content block.
func buildToolCall(block ContentBlock, timestamp time.Time, agentCounter *int) *model.ToolCall {
	tc := &model.ToolCall{
		ID:        block.ToolUseID,
		Timestamp: timestamp,
		ExitCode:  -1, // Default: not available.
	}

	tc.Type, tc.MCPServerName, tc.MCPToolName = classifyToolName(block.ToolName)

	// Parse tool-specific input fields.
	if len(block.ToolInput) > 0 {
		var input map[string]json.RawMessage
		if err := json.Unmarshal(block.ToolInput, &input); err == nil {
			parseToolInput(tc, input, block.ToolName, agentCounter)
		}
	}

	return tc
}

// parseToolInput extracts tool-specific fields from the input object.
func parseToolInput(tc *model.ToolCall, input map[string]json.RawMessage, toolName string, agentCounter *int) {
	switch tc.Type {
	case model.ToolWrite:
		if v, ok := input["file_path"]; ok {
			json.Unmarshal(v, &tc.Path)
		}
		if v, ok := input["content"]; ok {
			json.Unmarshal(v, &tc.Content)
		}
	case model.ToolRead:
		if v, ok := input["file_path"]; ok {
			json.Unmarshal(v, &tc.Path)
		}
	case model.ToolEdit:
		if v, ok := input["file_path"]; ok {
			json.Unmarshal(v, &tc.Path)
		}
		if v, ok := input["old_string"]; ok {
			json.Unmarshal(v, &tc.OldStr)
		}
		if v, ok := input["new_string"]; ok {
			json.Unmarshal(v, &tc.NewStr)
		}
	case model.ToolBash:
		if v, ok := input["command"]; ok {
			json.Unmarshal(v, &tc.Command)
		}
	case model.ToolAgent:
		*agentCounter++
		if v, ok := input["description"]; ok {
			json.Unmarshal(v, &tc.AgentDescription)
		}
		if v, ok := input["prompt"]; ok {
			json.Unmarshal(v, &tc.AgentPrompt)
		}
		if v, ok := input["subagent_type"]; ok {
			json.Unmarshal(v, &tc.SubagentType)
		}
	case model.ToolGlob:
		if v, ok := input["pattern"]; ok {
			var p string
			json.Unmarshal(v, &p)
			tc.Path = p
		}
	case model.ToolGrep:
		if v, ok := input["pattern"]; ok {
			var p string
			json.Unmarshal(v, &p)
			tc.Path = p
		}
	case model.ToolMCP:
		tc.MCPInput = make(map[string]any)
		for k, v := range input {
			var val any
			json.Unmarshal(v, &val)
			tc.MCPInput[k] = val
		}
	}
}

// pairToolResult matches a tool_result event to its corresponding ToolCall
// and populates result-side fields.
func pairToolResult(evt *RawEvent, toolCallsByID map[string]*model.ToolCall, prompt *model.Prompt) {
	// Find the tool_result content block to get the tool_use_id.
	for _, block := range evt.Message.Content {
		if block.Type != "tool_result" {
			continue
		}
		tc, ok := toolCallsByID[block.ToolResultID]
		if !ok {
			continue
		}

		// Populate result-side fields from toolUseResult.
		if len(evt.ToolUseResult) > 0 {
			populateToolResult(tc, evt.ToolUseResult, block.IsError)

			// Agent tool_result: link via the tool_use ID to the correct agent.
			if tc.Type == model.ToolAgent {
				populateAgentResult(evt, prompt, tc.ID)
			}
		}
	}
}

// populateToolResult fills in result-side fields on a ToolCall.
func populateToolResult(tc *model.ToolCall, result json.RawMessage, isError bool) {
	switch tc.Type {
	case model.ToolRead:
		// Read result: extract file content from toolUseResult.file.content.
		var readResult struct {
			File struct {
				Content string `json:"content"`
			} `json:"file"`
		}
		if err := json.Unmarshal(result, &readResult); err == nil && readResult.File.Content != "" {
			tc.Content = readResult.File.Content
		}
	case model.ToolBash:
		// Bash result: can be {stdout, stderr, exitCode} or string.
		var bashResult struct {
			Stdout   string `json:"stdout"`
			Stderr   string `json:"stderr"`
			ExitCode *int   `json:"exitCode"`
		}
		if err := json.Unmarshal(result, &bashResult); err == nil {
			output := bashResult.Stdout
			if bashResult.Stderr != "" {
				if output != "" {
					output += "\n"
				}
				output += bashResult.Stderr
			}
			// Truncate to 500 chars.
			if len(output) > 500 {
				output = output[:500]
			}
			tc.Output = output
			if bashResult.ExitCode != nil {
				tc.ExitCode = *bashResult.ExitCode
			}
		}
	}
}

// populateAgentResult fills in agent node fields from the toolUseResult.
// Matches the agent by ToolUseID — set at creation, unique per agent.
func populateAgentResult(evt *RawEvent, prompt *model.Prompt, toolUseID string) {
	var agentResult struct {
		Status            string `json:"status"`
		AgentID           string `json:"agentId"`
		TotalDurationMs   int    `json:"totalDurationMs"`
		TotalTokens       int    `json:"totalTokens"`
		TotalToolUseCount int    `json:"totalToolUseCount"`
	}
	if err := json.Unmarshal(evt.ToolUseResult, &agentResult); err != nil {
		return
	}

	for _, agent := range prompt.Agents {
		if agent.ToolUseID != toolUseID {
			continue
		}
		if agentResult.AgentID != "" {
			agent.ID = agentResult.AgentID
		}
		agent.EndTime = evt.Timestamp
		agent.Duration = agent.EndTime.Sub(agent.StartTime)
		agent.TotalDurationMs = agentResult.TotalDurationMs
		agent.TotalTokens = agentResult.TotalTokens
		agent.TotalToolUseCount = agentResult.TotalToolUseCount
		if agentResult.Status == "completed" {
			agent.Status = model.AgentSucceeded
		} else {
			agent.Status = model.AgentFailed
		}
		return
	}
}

// computeDurationOutliers flags prompts with duration >2σ above mean.
// Only computed when ≥5 prompts exist.
func computeDurationOutliers(s *model.Session) {
	if len(s.Prompts) < 5 {
		return
	}

	// Build durations, skipping prompts with unresolvable EndTime.
	type promptDuration struct {
		idx      int
		duration float64
	}
	var durations []promptDuration
	for i, p := range s.Prompts {
		endTime := p.EndTime
		// Last prompt may not have EndTime set (no following human turn sealed it).
		if endTime.IsZero() && i == len(s.Prompts)-1 && !s.EndTime.IsZero() {
			endTime = s.EndTime
		}
		if endTime.IsZero() || p.StartTime.IsZero() {
			continue // Skip prompts with unresolvable time — don't skew the calculation.
		}
		d := endTime.Sub(p.StartTime).Seconds()
		if d < 0 {
			d = 0
		}
		durations = append(durations, promptDuration{i, d})
	}
	if len(durations) < 5 {
		return
	}

	// Compute mean.
	var sum float64
	for _, pd := range durations {
		sum += pd.duration
	}
	mean := sum / float64(len(durations))

	// Compute standard deviation.
	var sqDiffSum float64
	for _, pd := range durations {
		diff := pd.duration - mean
		sqDiffSum += diff * diff
	}
	stddev := math.Sqrt(sqDiffSum / float64(len(durations)))

	threshold := mean + 2*stddev
	for _, pd := range durations {
		if pd.duration > threshold && stddev > 0 {
			s.Prompts[pd.idx].IsDurationOutlier = true
		}
	}
}

// classifyToolName maps a tool name string to a ToolType.
// For MCP tools, extracts server and tool names.
func classifyToolName(name string) (model.ToolType, string, string) {
	// MCP tools: mcp__<server>__<tool>
	if strings.HasPrefix(name, "mcp__") {
		parts := strings.SplitN(name, "__", 3)
		server, tool := "", name
		if len(parts) >= 3 {
			server = parts[1]
			tool = parts[2]
		}
		return model.ToolMCP, server, tool
	}

	switch name {
	case "Write":
		return model.ToolWrite, "", ""
	case "Read":
		return model.ToolRead, "", ""
	case "Edit":
		return model.ToolEdit, "", ""
	case "Bash", "PowerShell":
		return model.ToolBash, "", ""
	case "Agent":
		return model.ToolAgent, "", ""
	case "Glob":
		return model.ToolGlob, "", ""
	case "Grep":
		return model.ToolGrep, "", ""
	case "WebSearch", "WebFetch":
		return model.ToolWebSearch, "", ""
	default:
		return model.ToolOther, "", ""
	}
}
