package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kno-ai/kno-trace/internal/config"
	"github.com/kno-ai/kno-trace/internal/model"
)

// testdataPath returns the absolute path to internal/testdata/<name>.
func testdataPath(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "testdata", name)
}

// loadFixtureEvents parses the live_session.jsonl fixture and returns all events.
func loadFixtureEvents(t *testing.T) []*RawEvent {
	t.Helper()
	f, err := os.Open(testdataPath("live_session.jsonl"))
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()
	events, err := ParseReader(f)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return events
}

// TestRebuildActivePrompt verifies that incremental builds produce the same
// result as a full build. Events are split at multiple points: full build of
// first N, then incremental for the rest.
func TestRebuildActivePrompt(t *testing.T) {
	cfg := config.Default()
	events := loadFixtureEvents(t)

	if len(events) < 5 {
		t.Fatalf("expected at least 5 events, got %d", len(events))
	}

	// Full build for comparison.
	fullSession := BuildSession(events, cfg)
	if fullSession == nil {
		t.Fatal("full build returned nil")
	}

	// Test splitting at various points.
	splitPoints := []int{3, 6, 10}
	for _, split := range splitPoints {
		if split >= len(events) {
			continue
		}
		t.Run(
			fmt.Sprintf("split_at_%d", split),
			func(t *testing.T) {
				// Phase 1: full build from first `split` events.
				initialEvents := events[:split]
				session := BuildSession(initialEvents, cfg)
				if session == nil {
					t.Fatal("initial build returned nil")
				}
				toolCallsByID := BuildToolCallsByID(session)

				// Phase 2: incremental from remaining events.
				allEvents := make([]*RawEvent, len(events))
				copy(allEvents, events)
				RebuildActivePrompt(session, allEvents, split, cfg, toolCallsByID, "", CountAgents(session))

				// Compare prompt counts.
				if len(session.Prompts) != len(fullSession.Prompts) {
					t.Errorf("prompt count: got %d, want %d",
						len(session.Prompts), len(fullSession.Prompts))
				}

				// Compare tool call counts per prompt.
				for i := 0; i < min(len(session.Prompts), len(fullSession.Prompts)); i++ {
					got := len(session.Prompts[i].ToolCalls)
					want := len(fullSession.Prompts[i].ToolCalls)
					if got != want {
						t.Errorf("prompt %d: tool calls: got %d, want %d", i, got, want)
					}
				}

				// Compare agent counts per prompt.
				for i := 0; i < min(len(session.Prompts), len(fullSession.Prompts)); i++ {
					got := len(session.Prompts[i].Agents)
					want := len(fullSession.Prompts[i].Agents)
					if got != want {
						t.Errorf("prompt %d: agents: got %d, want %d", i, got, want)
					}
				}
			},
		)
	}
}

// TestProgressToolExtraction verifies that progress lines yield ToolUseBlocks
// with the correct tool names and agent IDs.
func TestProgressToolExtraction(t *testing.T) {
	events := loadFixtureEvents(t)

	var progressEvents []*RawEvent
	for _, evt := range events {
		if evt.Type == "progress" && evt.ProgressData != nil &&
			evt.ProgressData.Type == "agent_progress" {
			progressEvents = append(progressEvents, evt)
		}
	}

	if len(progressEvents) == 0 {
		t.Fatal("expected progress events, got none")
	}

	// Verify first agent's progress lines have tool_use blocks.
	var agent1Blocks int
	var agent2Blocks int
	for _, evt := range progressEvents {
		for range evt.ProgressData.ToolUseBlocks {
			if evt.ProgressData.AgentID == "agent-live-001" {
				agent1Blocks++
			} else if evt.ProgressData.AgentID == "agent-live-002" {
				agent2Blocks++
			}
		}
	}

	if agent1Blocks != 2 {
		t.Errorf("agent-live-001 tool_use blocks: got %d, want 2", agent1Blocks)
	}
	if agent2Blocks != 1 {
		t.Errorf("agent-live-002 tool_use blocks: got %d, want 1", agent2Blocks)
	}

	// Verify parentToolUseID is extracted.
	for _, evt := range progressEvents {
		if evt.ProgressData.ParentToolUseID == "" {
			t.Errorf("progress event for agent %s missing parentToolUseID",
				evt.ProgressData.AgentID)
		}
	}

	// Verify tool names from embedded messages.
	first := progressEvents[0]
	if len(first.ProgressData.ToolUseBlocks) == 0 {
		t.Fatal("first progress event has no tool_use blocks")
	}
	if first.ProgressData.ToolUseBlocks[0].ToolName != "Glob" {
		t.Errorf("first tool_use: got %q, want %q",
			first.ProgressData.ToolUseBlocks[0].ToolName, "Glob")
	}
}

// TestActivePromptDetection verifies that the last prompt in the fixture has
// zero EndTime (it's the active/live prompt with no sealing human turn).
func TestActivePromptDetection(t *testing.T) {
	cfg := config.Default()
	events := loadFixtureEvents(t)
	session := BuildSession(events, cfg)

	if len(session.Prompts) != 3 {
		t.Fatalf("expected 3 prompts, got %d", len(session.Prompts))
	}

	// Prompts 1 and 2 should be sealed.
	if session.Prompts[0].EndTime.IsZero() {
		t.Error("prompt 0 should be sealed (non-zero EndTime)")
	}
	if session.Prompts[1].EndTime.IsZero() {
		t.Error("prompt 1 should be sealed (non-zero EndTime)")
	}

	// Prompt 3 (active) should have zero EndTime.
	if !session.Prompts[2].EndTime.IsZero() {
		t.Error("prompt 2 (active) should have zero EndTime")
	}

	// Verify loop detection on prompt 3: 4 Edits on config.go.
	hasLoop := false
	for _, w := range session.Prompts[2].Warnings {
		if w.Type == model.WarnLoopDetected {
			hasLoop = true
			break
		}
	}
	if !hasLoop {
		t.Error("prompt 2 should have WarnLoopDetected (4 Edit config.go)")
	}
}

// TestIncrementalIsLive verifies that RebuildActivePrompt sets IsLive correctly.
func TestIncrementalIsLive(t *testing.T) {
	cfg := config.Default()
	events := loadFixtureEvents(t)

	// Build from first 6 events (prompt 1 sealed by human turn 2).
	session := BuildSession(events[:6], cfg)
	toolCallsByID := BuildToolCallsByID(session)

	// After adding more events up to prompt 3 (active, no seal):
	allEvents := make([]*RawEvent, len(events))
	copy(allEvents, events)
	RebuildActivePrompt(session, allEvents, 6, cfg, toolCallsByID, "", CountAgents(session))

	if !session.IsLive {
		t.Error("session should be live (last prompt has no EndTime)")
	}
}

// TestBuildSessionEmpty verifies that BuildSession handles zero events gracefully.
func TestBuildSessionEmpty(t *testing.T) {
	cfg := config.Default()
	session := BuildSession(nil, cfg)
	if session == nil {
		t.Fatal("BuildSession(nil) returned nil")
	}
	if len(session.Prompts) != 0 {
		t.Errorf("expected 0 prompts, got %d", len(session.Prompts))
	}
	if session.Interrupted {
		t.Error("empty session should not be interrupted")
	}
	if session.IsLive {
		t.Error("empty session should not be live")
	}
}

// TestRebuildActivePromptZeroNewEvents verifies that incremental rebuild with
// no new events is a safe no-op.
func TestRebuildActivePromptZeroNewEvents(t *testing.T) {
	cfg := config.Default()
	events := loadFixtureEvents(t)

	session := BuildSession(events, cfg)
	promptCount := len(session.Prompts)
	toolCallsByID := BuildToolCallsByID(session)

	// Rebuild with zero new events.
	branch, sealed, _ := RebuildActivePrompt(session, nil, 0, cfg, toolCallsByID, "main", 0)

	if len(sealed) != 0 {
		t.Errorf("expected no sealed prompts, got %d", len(sealed))
	}
	if len(session.Prompts) != promptCount {
		t.Errorf("prompt count changed: got %d, want %d", len(session.Prompts), promptCount)
	}
	if branch != "main" {
		t.Errorf("lastBranch should be unchanged, got %q", branch)
	}
}

// TestRebuildActivePromptSealsMidBatch verifies that a human turn event in the
// middle of a batch correctly seals the previous prompt.
func TestRebuildActivePromptSealsMidBatch(t *testing.T) {
	cfg := config.Default()
	events := loadFixtureEvents(t)

	// Build from first 5 events (mid prompt 1).
	session := BuildSession(events[:5], cfg)
	initialPrompts := len(session.Prompts)
	toolCallsByID := BuildToolCallsByID(session)

	// Feed a batch that contains the sealing human turn + prompt 2 events.
	remaining := events[5:]
	_, sealed, _ := RebuildActivePrompt(session, remaining, 0, cfg, toolCallsByID, "", CountAgents(session))

	if len(session.Prompts) <= initialPrompts {
		t.Error("expected new prompt(s) after sealing")
	}
	if len(sealed) == 0 {
		t.Error("expected at least one sealed prompt index")
	}
	// Sealed prompt should have non-zero EndTime.
	for _, idx := range sealed {
		if idx < len(session.Prompts) && session.Prompts[idx].EndTime.IsZero() {
			t.Errorf("sealed prompt %d has zero EndTime", idx)
		}
	}
}

// TestProgressLineWithoutToolBlocks verifies that progress lines with user-type
// embedded messages (tool results, not tool_use blocks) are handled gracefully.
func TestProgressLineWithoutToolBlocks(t *testing.T) {
	// Progress line with a user-type embedded message (no tool_use blocks).
	line := `{"type":"progress","data":{"message":{"type":"user","message":{"role":"user","content":[{"tool_use_id":"t1","type":"tool_result","content":"ok"}]},"uuid":"u1","timestamp":"2024-01-01T00:00:01Z"},"type":"agent_progress","agentId":"agent-1"},"parentToolUseID":"toolu-1","uuid":"u2","timestamp":"2024-01-01T00:00:01Z"}`
	events, err := ParseReader(strings.NewReader(line))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	evt := events[0]
	if evt.ProgressData == nil {
		t.Fatal("expected ProgressData")
	}
	if evt.ProgressData.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want agent-1", evt.ProgressData.AgentID)
	}
	if evt.ProgressData.ParentToolUseID != "toolu-1" {
		t.Errorf("ParentToolUseID = %q, want toolu-1", evt.ProgressData.ParentToolUseID)
	}
	// User-type messages have no tool_use blocks — should be empty, not nil-panic.
	if len(evt.ProgressData.ToolUseBlocks) != 0 {
		t.Errorf("expected 0 tool_use blocks from user progress, got %d", len(evt.ProgressData.ToolUseBlocks))
	}
}

// TestClassifyToolNameExported verifies the exported ClassifyToolName function.
func TestClassifyToolNameExported(t *testing.T) {
	tests := []struct {
		name     string
		wantType model.ToolType
	}{
		{"Read", model.ToolRead},
		{"Write", model.ToolWrite},
		{"Edit", model.ToolEdit},
		{"Bash", model.ToolBash},
		{"Agent", model.ToolAgent},
		{"Glob", model.ToolGlob},
		{"Grep", model.ToolGrep},
		{"WebSearch", model.ToolWebSearch},
		{"WebFetch", model.ToolWebSearch},
		{"mcp__kno__note_create", model.ToolMCP},
		{"FutureTool", model.ToolOther},
		{"", model.ToolOther},
	}
	for _, tt := range tests {
		toolType, _, _ := ClassifyToolName(tt.name)
		if toolType != tt.wantType {
			t.Errorf("ClassifyToolName(%q) = %q, want %q", tt.name, toolType, tt.wantType)
		}
	}

	// MCP tool should extract server and tool name.
	_, server, tool := ClassifyToolName("mcp__kno__note_create")
	if server != "kno" {
		t.Errorf("MCP server = %q, want kno", server)
	}
	if tool != "note_create" {
		t.Errorf("MCP tool = %q, want note_create", tool)
	}
}

