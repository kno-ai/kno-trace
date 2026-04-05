// Package parser converts Claude Code JSONL session files into structured data.
// The parser never crashes on unexpected data — it extracts what it recognizes
// and skips what it doesn't.
package parser

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/kno-ai/kno-trace/internal/config"
)

// RawEvent represents a single parsed JSONL line with enough structure
// to build Session/Prompt trees. Fields are populated lazily from the raw JSON.
type RawEvent struct {
	Type      string    // "user", "assistant", "system", "progress", etc.
	Timestamp time.Time
	UUID      string
	SessionID string
	GitBranch string

	// For user and assistant lines.
	Message *RawMessage

	// For system lines.
	Subtype         string
	CompactMetadata *CompactMeta

	// For progress lines.
	ProgressData *ProgressData

	// Top-level fields on user lines.
	IsMeta                  bool
	ToolUseResult           json.RawMessage // varies: string, object, null
	SourceToolAssistantUUID string

	// For streaming snapshot dedup.
	RequestID string
}

// RawMessage holds the parsed message object from user/assistant lines.
type RawMessage struct {
	Role       string
	Model      string
	Content    []ContentBlock // parsed content blocks (empty for string content on user lines)
	StopReason *string       // nil = streaming snapshot, "end_turn" or "tool_use" = final
	Usage      *Usage
	HumanText  string // extracted text from human turn (content was string or text block)
}

// ContentBlock is one element of message.content[].
type ContentBlock struct {
	Type string // "text", "tool_use", "thinking", "tool_result", or unknown

	// text block
	Text string

	// tool_use block
	ToolUseID string
	ToolName  string
	ToolInput json.RawMessage

	// tool_result block
	ToolResultID string
	IsError      bool
	ResultContent string
}

// Usage holds token counts from assistant messages.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

// CompactMeta holds /compact metadata from system lines.
type CompactMeta struct {
	Trigger   string `json:"trigger"`
	PreTokens int    `json:"preTokens"`
}

// ProgressData holds parsed progress line data.
type ProgressData struct {
	Type    string // "agent_progress", "hook_progress", etc.
	AgentID string
}

// ParseFile reads a JSONL file and returns all parsed events.
// Streaming snapshots are deduplicated: only the final response per requestId
// is returned, with tool_use blocks merged from ALL snapshots.
// Files larger than cfg.MaxFileSize() are rejected.
func ParseFile(path string, cfg *config.Config) ([]*RawEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > cfg.MaxFileSize() {
		return nil, fmt.Errorf("session file too large (%d bytes, max %d) — adjust max_file_size_mb in config",
			info.Size(), cfg.MaxFileSize())
	}

	return ParseReader(f, cfg.MaxLineSize())
}

// ParseReader reads JSONL from a reader and returns parsed events.
// maxLineSize is the largest single line to parse (0 = no limit).
func ParseReader(r io.Reader, maxLineSize ...int) ([]*RawEvent, error) {
	lineLimit := 0
	if len(maxLineSize) > 0 {
		lineLimit = maxLineSize[0]
	}
	reader := bufio.NewReader(r)

	var events []*RawEvent
	// Track streaming snapshots for dedup: requestId → accumulated tool_use blocks + latest event.
	type snapshotState struct {
		toolUses []*ContentBlock
		final    *RawEvent
		latest   *RawEvent
	}
	snapshots := make(map[string]*snapshotState)

	lineNum := 0
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			lineNum++

			// Skip blank lines.
			if len(strings.TrimSpace(string(line))) == 0 {
				if err != nil {
					break
				}
				continue
			}

			// Guard against pathologically large lines.
			if lineLimit > 0 && len(line) > lineLimit {
				fmt.Fprintf(os.Stderr, "kno-trace: skipping line %d: too large (%d bytes, max %d) — adjust max_line_size_mb in config\n",
					lineNum, len(line), lineLimit)
				if err != nil {
					break
				}
				continue
			}

			evt, parseErr := parseLine(line)
			if parseErr != nil {
				fmt.Fprintf(os.Stderr, "kno-trace: skipping line %d: %v\n", lineNum, parseErr)
			} else if evt != nil {
				// Streaming snapshot dedup for assistant messages.
				if evt.Type == "assistant" && evt.RequestID != "" && evt.Message != nil {
					state, exists := snapshots[evt.RequestID]
					if !exists {
						state = &snapshotState{}
						snapshots[evt.RequestID] = state
					}
					for i := range evt.Message.Content {
						if evt.Message.Content[i].Type == "tool_use" {
							block := evt.Message.Content[i]
							state.toolUses = append(state.toolUses, &block)
						}
					}
					state.latest = evt
					if evt.Message.StopReason != nil {
						state.final = evt
					}
				} else {
					events = append(events, evt)
				}
			}
		}
		if err != nil {
			break // EOF or read error.
		}
	}

	// Resolve streaming snapshots: emit one event per requestId.
	var resolved []*RawEvent
	for _, state := range snapshots {
		evt := state.final
		if evt == nil {
			evt = state.latest
		}
		if evt == nil || evt.Message == nil {
			continue
		}

		// Merge tool_use blocks from earlier snapshots not in the final event.
		existing := make(map[string]bool)
		for _, b := range evt.Message.Content {
			if b.Type == "tool_use" {
				existing[b.ToolUseID] = true
			}
		}
		var merged []ContentBlock
		for _, tu := range state.toolUses {
			if !existing[tu.ToolUseID] {
				merged = append(merged, *tu)
				existing[tu.ToolUseID] = true
			}
		}
		merged = append(merged, evt.Message.Content...)
		evt.Message.Content = merged
		resolved = append(resolved, evt)
	}

	events = append(events, resolved...)
	sortEventsByTimestamp(events)

	return events, nil
}

// parseLine parses a single JSONL line into a RawEvent.
// Returns nil for lines that should be filtered (isMeta, etc.).
func parseLine(line []byte) (*RawEvent, error) {
	// Quick pre-check: must start with '{'.
	trimmed := strings.TrimSpace(string(line))
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, fmt.Errorf("not JSON")
	}

	// Parse into a generic map first to extract top-level fields.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil {
		return nil, fmt.Errorf("JSON parse: %w", err)
	}

	evt := &RawEvent{}

	// Extract type.
	if v, ok := raw["type"]; ok {
		json.Unmarshal(v, &evt.Type)
	}
	if evt.Type == "" {
		return nil, nil // No type field — skip.
	}

	// Extract common fields.
	if v, ok := raw["timestamp"]; ok {
		var ts string
		json.Unmarshal(v, &ts)
		evt.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
	}
	if v, ok := raw["uuid"]; ok {
		json.Unmarshal(v, &evt.UUID)
	}
	if v, ok := raw["sessionId"]; ok {
		json.Unmarshal(v, &evt.SessionID)
	}
	if v, ok := raw["gitBranch"]; ok {
		json.Unmarshal(v, &evt.GitBranch)
	}
	if v, ok := raw["requestId"]; ok {
		json.Unmarshal(v, &evt.RequestID)
	}
	// Check isMeta — skip these lines entirely.
	if v, ok := raw["isMeta"]; ok {
		json.Unmarshal(v, &evt.IsMeta)
		if evt.IsMeta {
			return nil, nil
		}
	}

	switch evt.Type {
	case "user", "assistant":
		if err := parseMessageLine(raw, evt); err != nil {
			return nil, err
		}
	case "system":
		parseSystemLine(raw, evt)
	case "progress":
		parseProgressLine(raw, evt)
	case "queue-operation", "file-history-snapshot", "last-prompt":
		// Known non-message types — return event for timestamp tracking but no message.
	default:
		// Unknown type — skip silently per spec.
		return nil, nil
	}

	// Extract toolUseResult and sourceToolAssistantUUID for tool_result lines.
	if v, ok := raw["toolUseResult"]; ok {
		evt.ToolUseResult = v
	}
	if v, ok := raw["sourceToolAssistantUUID"]; ok {
		json.Unmarshal(v, &evt.SourceToolAssistantUUID)
	}

	return evt, nil
}

// parseMessageLine parses the message object on user/assistant lines.
func parseMessageLine(raw map[string]json.RawMessage, evt *RawEvent) error {
	msgRaw, ok := raw["message"]
	if !ok {
		return nil // No message field — valid line but nothing to parse.
	}

	var msgMap map[string]json.RawMessage
	if err := json.Unmarshal(msgRaw, &msgMap); err != nil {
		return nil // Message isn't an object — skip.
	}

	msg := &RawMessage{}

	if v, ok := msgMap["role"]; ok {
		json.Unmarshal(v, &msg.Role)
	}
	if v, ok := msgMap["model"]; ok {
		json.Unmarshal(v, &msg.Model)
	}
	if v, ok := msgMap["stop_reason"]; ok {
		// stop_reason can be null (streaming) or a string.
		var sr string
		if err := json.Unmarshal(v, &sr); err == nil && sr != "" {
			msg.StopReason = &sr
		}
		// If null or empty, StopReason stays nil.
	}

	// Parse usage.
	if v, ok := msgMap["usage"]; ok {
		var usage Usage
		json.Unmarshal(v, &usage)
		msg.Usage = &usage
	}

	// Parse content — can be string or array.
	if v, ok := msgMap["content"]; ok {
		parseContent(v, msg)
	}

	evt.Message = msg
	return nil
}

// parseContent handles message.content which can be a string or array of blocks.
func parseContent(raw json.RawMessage, msg *RawMessage) {
	if len(raw) == 0 {
		return
	}

	// Try as string first (human turns, isMeta messages).
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		msg.HumanText = str
		return
	}

	// Try as array of content blocks.
	var blocks []json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return // Can't parse — skip.
	}

	for _, blockRaw := range blocks {
		var blockMap map[string]json.RawMessage
		if err := json.Unmarshal(blockRaw, &blockMap); err != nil {
			continue
		}

		var blockType string
		if v, ok := blockMap["type"]; ok {
			json.Unmarshal(v, &blockType)
		}

		block := ContentBlock{Type: blockType}

		switch blockType {
		case "text":
			if v, ok := blockMap["text"]; ok {
				json.Unmarshal(v, &block.Text)
			}
			// If this is the first text block on a user message, capture as HumanText.
			if msg.Role == "user" && msg.HumanText == "" {
				msg.HumanText = block.Text
			}
		case "tool_use":
			if v, ok := blockMap["id"]; ok {
				json.Unmarshal(v, &block.ToolUseID)
			}
			if v, ok := blockMap["name"]; ok {
				json.Unmarshal(v, &block.ToolName)
			}
			if v, ok := blockMap["input"]; ok {
				block.ToolInput = v
			}
		case "tool_result":
			if v, ok := blockMap["tool_use_id"]; ok {
				json.Unmarshal(v, &block.ToolResultID)
			}
			if v, ok := blockMap["is_error"]; ok {
				json.Unmarshal(v, &block.IsError)
			}
			// content can be string or array.
			if v, ok := blockMap["content"]; ok {
				var s string
				if err := json.Unmarshal(v, &s); err == nil {
					block.ResultContent = s
				} else {
					// Array of content blocks — extract text.
					var inner []map[string]json.RawMessage
					if err := json.Unmarshal(v, &inner); err == nil {
						for _, item := range inner {
							if t, ok := item["text"]; ok {
								var txt string
								json.Unmarshal(t, &txt)
								block.ResultContent += txt
							}
						}
					}
				}
			}
		case "thinking":
			// Skip thinking blocks — not user-facing content.
			continue
		default:
			// Unknown content block type — skip per spec.
			continue
		}

		msg.Content = append(msg.Content, block)
	}
}

// parseSystemLine extracts subtype and compact metadata from system lines.
func parseSystemLine(raw map[string]json.RawMessage, evt *RawEvent) {
	if v, ok := raw["subtype"]; ok {
		json.Unmarshal(v, &evt.Subtype)
	}
	if evt.Subtype == "compact_boundary" {
		if v, ok := raw["compactMetadata"]; ok {
			var cm CompactMeta
			json.Unmarshal(v, &cm)
			evt.CompactMetadata = &cm
		}
	}
}

// parseProgressLine extracts agent progress data.
func parseProgressLine(raw map[string]json.RawMessage, evt *RawEvent) {
	dataRaw, ok := raw["data"]
	if !ok {
		return
	}
	var dataMap map[string]json.RawMessage
	if err := json.Unmarshal(dataRaw, &dataMap); err != nil {
		return
	}

	pd := &ProgressData{}
	if v, ok := dataMap["type"]; ok {
		json.Unmarshal(v, &pd.Type)
	}
	if v, ok := dataMap["agentId"]; ok {
		json.Unmarshal(v, &pd.AgentID)
	}
	evt.ProgressData = pd
}

// sortEventsByTimestamp sorts events in-place by timestamp, preserving
// relative order for events with equal timestamps.
func sortEventsByTimestamp(events []*RawEvent) {
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})
}
