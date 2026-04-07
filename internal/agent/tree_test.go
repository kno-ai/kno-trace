package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kno-ai/kno-trace/internal/config"
	"github.com/kno-ai/kno-trace/internal/model"
	"github.com/kno-ai/kno-trace/internal/parser"
)

// testdataDir returns the absolute path to the testdata directory.
func testdataDir(t *testing.T) string {
	t.Helper()
	// Tests run from the package directory, so testdata is at ../testdata/.
	abs, err := filepath.Abs("../testdata")
	if err != nil {
		t.Fatalf("failed to resolve testdata: %v", err)
	}
	return abs
}

// buildTestSession parses a parent JSONL fixture into a Session.
func buildTestSession(t *testing.T, fixture string) *model.Session {
	t.Helper()
	path := filepath.Join(testdataDir(t), fixture)
	cfg := config.Load()
	events, err := parser.ParseFile(path, cfg)
	if err != nil {
		t.Fatalf("ParseFile(%s): %v", fixture, err)
	}
	return parser.BuildSession(events, cfg)
}

func TestEnrichSession_ParallelAgents(t *testing.T) {
	s := buildTestSession(t, "parallel_agents.jsonl")
	cfg := config.Load()
	sessionDir := testdataDir(t) // subagent files are under testdata/session-001/subagents/

	if len(s.Prompts) == 0 {
		t.Fatal("expected at least 1 prompt")
	}
	if len(s.Prompts[0].Agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(s.Prompts[0].Agents))
	}

	// Verify agents have IDs before enrichment.
	for _, agent := range s.Prompts[0].Agents {
		if agent.ID == "" {
			t.Errorf("agent %q has no ID before enrichment — toolUseResult not parsed?", agent.Label)
		}
	}

	EnrichSession(s, sessionDir, cfg)

	prompt := s.Prompts[0]

	// Agent 001: the TODO finder — should have Grep, Read, Read, Glob = 4 tool calls.
	agent001 := findAgent(prompt.Agents, "agent-001")
	if agent001 == nil {
		t.Fatal("agent-001 not found")
	}
	if len(agent001.ToolCalls) != 4 {
		t.Errorf("agent-001: expected 4 tool calls, got %d", len(agent001.ToolCalls))
		for i, tc := range agent001.ToolCalls {
			t.Logf("  [%d] %s %s", i, tc.Type, tc.Path)
		}
	}
	if len(agent001.FilesTouched) == 0 {
		t.Error("agent-001: expected FilesTouched to be populated")
	}
	if agent001.ModelName == "" {
		t.Error("agent-001: expected ModelName to be set from subagent file")
	}
	// Verify tool calls are attributed to this agent.
	for _, tc := range agent001.ToolCalls {
		if tc.SourceAgent != "agent-001" {
			t.Errorf("agent-001 tool call %s has SourceAgent=%q, want agent-001", tc.ID, tc.SourceAgent)
		}
	}

	// Agent 002: the import finder — should have Glob, Grep, Read, Read, Edit = 5 tool calls.
	agent002 := findAgent(prompt.Agents, "agent-002")
	if agent002 == nil {
		t.Fatal("agent-002 not found")
	}
	if len(agent002.ToolCalls) != 5 {
		t.Errorf("agent-002: expected 5 tool calls, got %d", len(agent002.ToolCalls))
		for i, tc := range agent002.ToolCalls {
			t.Logf("  [%d] %s %s", i, tc.Type, tc.Path)
		}
	}

	// Both agents are parallel (agent-002 spawned before agent-001 completed).
	if !agent001.IsParallel {
		t.Error("agent-001 should be marked parallel")
	}
	if !agent002.IsParallel {
		t.Error("agent-002 should be marked parallel")
	}

	// File conflict: both touch internal/parser/builder.go, and agent-002 edits it.
	hasConflict := false
	for _, w := range prompt.Warnings {
		if w.Type == model.WarnAgentConflict {
			hasConflict = true
			break
		}
	}
	if !hasConflict {
		t.Error("expected WarnAgentConflict for parallel agents touching internal/parser/builder.go")
	}

	// No unlinked agents.
	if len(s.UnlinkedAgents) > 0 {
		t.Errorf("expected 0 unlinked agents, got %d", len(s.UnlinkedAgents))
	}
}

func TestEnrichSession_SingleAgent(t *testing.T) {
	s := buildTestSession(t, "with_agent.jsonl")
	cfg := config.Load()
	sessionDir := testdataDir(t)

	EnrichSession(s, sessionDir, cfg)

	if len(s.Prompts) == 0 {
		t.Fatal("expected at least 1 prompt")
	}

	// Find the prompt with agents (prompt 0 has the agent).
	var agentPrompt *model.Prompt
	for _, p := range s.Prompts {
		if len(p.Agents) > 0 {
			agentPrompt = p
			break
		}
	}
	if agentPrompt == nil {
		t.Fatal("no prompt with agents found")
	}

	agent := agentPrompt.Agents[0]
	if agent.ID != "agent-001" {
		t.Errorf("expected agent ID agent-001, got %q", agent.ID)
	}

	// Subagent file has: Glob, Read, Read = 3 tool calls.
	if len(agent.ToolCalls) != 3 {
		t.Errorf("expected 3 tool calls, got %d", len(agent.ToolCalls))
		for i, tc := range agent.ToolCalls {
			t.Logf("  [%d] %s %s", i, tc.Type, tc.Path)
		}
	}

	// FilesTouched should have docs/api.md, docs/README.md, and the glob pattern.
	if len(agent.FilesTouched) == 0 {
		t.Error("expected FilesTouched to be populated")
	}

	// No file conflicts (single agent).
	for _, w := range agentPrompt.Warnings {
		if w.Type == model.WarnAgentConflict {
			t.Error("unexpected file conflict warning for single agent")
		}
	}
}

func TestEnrichSession_MissingSubagentFile(t *testing.T) {
	s := buildTestSession(t, "parallel_agents.jsonl")
	cfg := config.Load()

	// Point to a nonexistent directory — all subagent files will be missing.
	EnrichSession(s, "/nonexistent/path", cfg)

	// Agents should still have their summary data from toolUseResult.
	if len(s.Prompts) == 0 {
		t.Fatal("expected at least 1 prompt")
	}
	for _, agent := range s.Prompts[0].Agents {
		if agent.TotalToolUseCount == 0 {
			t.Errorf("agent %q: expected TotalToolUseCount from toolUseResult to survive", agent.Label)
		}
		// ToolCalls should be empty (no file data).
		if len(agent.ToolCalls) > 0 {
			t.Errorf("agent %q: expected 0 tool calls without subagent file, got %d", agent.Label, len(agent.ToolCalls))
		}
	}
}

func TestSubagentFilePath(t *testing.T) {
	got := SubagentFilePath("/home/user/.claude/projects/myproj", "abc-123", "deadbeef01234567")
	want := filepath.Join("/home/user/.claude/projects/myproj", "abc-123", "subagents", "agent-deadbeef01234567.jsonl")
	if got != want {
		t.Errorf("SubagentFilePath: got %s, want %s", got, want)
	}
}

func TestEnrichFromEvents_EmptyEvents(t *testing.T) {
	agent := &model.AgentNode{ID: "test"}
	err := enrichFromEvents(agent, nil)
	if err != nil {
		t.Errorf("enrichFromEvents(nil): unexpected error: %v", err)
	}
	if len(agent.ToolCalls) != 0 {
		t.Error("expected 0 tool calls for empty events")
	}
}

func TestSubagentFileExists(t *testing.T) {
	dir := testdataDir(t)

	// Verify the fixture files actually exist.
	for _, name := range []string{
		"session-001/subagents/agent-agent-001.jsonl",
		"session-001/subagents/agent-agent-002.jsonl",
		"session-003/subagents/agent-agent-001.jsonl",
	} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("fixture file missing: %s", path)
		}
	}
}

func TestEnrichSession_ParallelAgents_FilesTouched(t *testing.T) {
	s := buildTestSession(t, "parallel_agents.jsonl")
	cfg := config.Load()
	sessionDir := testdataDir(t)

	EnrichSession(s, sessionDir, cfg)

	agent001 := findAgent(s.Prompts[0].Agents, "agent-001")
	if agent001 == nil {
		t.Fatal("agent-001 not found")
	}

	// FilesTouched should NOT contain grep/glob patterns like "TODO|FIXME" or "**/*.go".
	for _, path := range agent001.FilesTouched {
		if path == "TODO|FIXME" || path == "**/*.go" {
			t.Errorf("FilesTouched contains search pattern %q — should only have file paths", path)
		}
	}

	// Should contain the actual file paths that were Read.
	hasBuilder := false
	for _, path := range agent001.FilesTouched {
		if path == "internal/parser/builder.go" {
			hasBuilder = true
		}
	}
	if !hasBuilder {
		t.Error("FilesTouched missing internal/parser/builder.go")
	}
}

func TestEnrichSession_TokenAccumulation(t *testing.T) {
	s := buildTestSession(t, "with_agent.jsonl")
	cfg := config.Load()
	sessionDir := testdataDir(t)

	EnrichSession(s, sessionDir, cfg)

	var agentPrompt *model.Prompt
	for _, p := range s.Prompts {
		if len(p.Agents) > 0 {
			agentPrompt = p
			break
		}
	}
	if agentPrompt == nil {
		t.Fatal("no prompt with agents found")
	}

	agent := agentPrompt.Agents[0]

	// The subagent fixture has 4 assistant messages with usage data.
	// TokensIn and TokensOut should be accumulated (non-zero).
	if agent.TokensIn == 0 {
		t.Error("expected TokensIn to be accumulated from subagent assistant messages")
	}
	if agent.TokensOut == 0 {
		t.Error("expected TokensOut to be accumulated from subagent assistant messages")
	}

	// TotalTokens (from toolUseResult) should still be set from the parent parser.
	if agent.TotalTokens == 0 {
		t.Error("expected TotalTokens from toolUseResult to survive enrichment")
	}
}

func TestDetectParallelAgents_Sequential(t *testing.T) {
	// Two agents that ran sequentially should NOT be marked parallel.
	s := &model.Session{
		Prompts: []*model.Prompt{{
			Agents: []*model.AgentNode{
				{
					ID:        "a1",
					StartTime: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
					EndTime:   time.Date(2026, 1, 1, 10, 1, 0, 0, time.UTC),
				},
				{
					ID:        "a2",
					StartTime: time.Date(2026, 1, 1, 10, 2, 0, 0, time.UTC), // after a1 ended
					EndTime:   time.Date(2026, 1, 1, 10, 3, 0, 0, time.UTC),
				},
			},
		}},
	}

	detectParallelAgents(s)

	for _, a := range s.Prompts[0].Agents {
		if a.IsParallel {
			t.Errorf("agent %s should NOT be marked parallel (sequential run)", a.ID)
		}
	}
}

func TestDetectParallelAgents_OneRunning(t *testing.T) {
	// Agent still running (EndTime zero) + another spawned = parallel.
	s := &model.Session{
		Prompts: []*model.Prompt{{
			Agents: []*model.AgentNode{
				{
					ID:        "a1",
					StartTime: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
					// EndTime zero = still running
				},
				{
					ID:        "a2",
					StartTime: time.Date(2026, 1, 1, 10, 0, 30, 0, time.UTC),
					EndTime:   time.Date(2026, 1, 1, 10, 1, 0, 0, time.UTC),
				},
			},
		}},
	}

	detectParallelAgents(s)

	for _, a := range s.Prompts[0].Agents {
		if !a.IsParallel {
			t.Errorf("agent %s should be marked parallel (a1 still running when a2 spawned)", a.ID)
		}
	}
}

func TestAgentLabels(t *testing.T) {
	s := buildTestSession(t, "parallel_agents.jsonl")

	// Labels should be sequential "subagent-N" format.
	for _, a := range s.Prompts[0].Agents {
		if a.Label != "subagent-1" && a.Label != "subagent-2" {
			t.Errorf("expected label like subagent-N, got %q", a.Label)
		}
	}
}

func TestEnrichSession_NilSession(t *testing.T) {
	cfg := config.Load()
	// Should not panic.
	EnrichSession(nil, "/some/path", cfg)
}

func TestEnrichSession_ToolCallTypes(t *testing.T) {
	s := buildTestSession(t, "parallel_agents.jsonl")
	cfg := config.Load()
	sessionDir := testdataDir(t)
	EnrichSession(s, sessionDir, cfg)

	agent001 := findAgent(s.Prompts[0].Agents, "agent-001")
	if agent001 == nil {
		t.Fatal("agent-001 not found")
	}

	// Verify exact tool call types and order: Grep, Read, Read, Glob.
	expected := []model.ToolType{model.ToolGrep, model.ToolRead, model.ToolRead, model.ToolGlob}
	if len(agent001.ToolCalls) != len(expected) {
		t.Fatalf("agent-001: got %d tool calls, want %d", len(agent001.ToolCalls), len(expected))
	}
	for i, tc := range agent001.ToolCalls {
		if tc.Type != expected[i] {
			t.Errorf("agent-001 tool call [%d]: type=%s, want %s", i, tc.Type, expected[i])
		}
	}

	agent002 := findAgent(s.Prompts[0].Agents, "agent-002")
	if agent002 == nil {
		t.Fatal("agent-002 not found")
	}

	// Agent-002: Glob, Grep, Read, Read, Edit.
	expected2 := []model.ToolType{model.ToolGlob, model.ToolGrep, model.ToolRead, model.ToolRead, model.ToolEdit}
	if len(agent002.ToolCalls) != len(expected2) {
		t.Fatalf("agent-002: got %d tool calls, want %d", len(agent002.ToolCalls), len(expected2))
	}
	for i, tc := range agent002.ToolCalls {
		if tc.Type != expected2[i] {
			t.Errorf("agent-002 tool call [%d]: type=%s, want %s", i, tc.Type, expected2[i])
		}
	}
}

func TestEnrichSession_ToolResultPairing(t *testing.T) {
	s := buildTestSession(t, "parallel_agents.jsonl")
	cfg := config.Load()
	sessionDir := testdataDir(t)
	EnrichSession(s, sessionDir, cfg)

	// Agent-002 has a Read of go.mod — the toolUseResult has file.content.
	agent002 := findAgent(s.Prompts[0].Agents, "agent-002")
	if agent002 == nil {
		t.Fatal("agent-002 not found")
	}

	// Find the Read of go.mod.
	var goModRead *model.ToolCall
	for _, tc := range agent002.ToolCalls {
		if tc.Type == model.ToolRead && tc.Path == "go.mod" {
			goModRead = tc
			break
		}
	}
	if goModRead == nil {
		t.Fatal("agent-002: Read go.mod tool call not found")
	}
	if goModRead.Content == "" {
		t.Error("agent-002: Read go.mod should have Content populated from tool result")
	}
}

func TestDetectFileConflicts_ReadOnlyNoConflict(t *testing.T) {
	// Two parallel agents that only Read the same file — no conflict.
	s := &model.Session{
		Prompts: []*model.Prompt{{
			Agents: []*model.AgentNode{
				{
					ID:         "a1",
					Label:      "subagent-1",
					IsParallel: true,
					ToolCalls: []*model.ToolCall{
						{Type: model.ToolRead, Path: "shared.go"},
					},
					FilesTouched: []string{"shared.go"},
				},
				{
					ID:         "a2",
					Label:      "subagent-2",
					IsParallel: true,
					ToolCalls: []*model.ToolCall{
						{Type: model.ToolRead, Path: "shared.go"},
					},
					FilesTouched: []string{"shared.go"},
				},
			},
		}},
	}

	detectFileConflicts(s)

	for _, w := range s.Prompts[0].Warnings {
		if w.Type == model.WarnAgentConflict {
			t.Error("read-only shared file should not trigger WarnAgentConflict")
		}
	}
}

func TestDetectFileConflicts_SpecificPath(t *testing.T) {
	s := buildTestSession(t, "parallel_agents.jsonl")
	cfg := config.Load()
	sessionDir := testdataDir(t)
	EnrichSession(s, sessionDir, cfg)

	// Verify the conflict warning names the specific path.
	found := false
	for _, w := range s.Prompts[0].Warnings {
		if w.Type == model.WarnAgentConflict {
			if !strings.Contains(w.Message, "internal/parser/builder.go") {
				t.Errorf("conflict warning should mention internal/parser/builder.go, got: %s", w.Message)
			}
			found = true
		}
	}
	if !found {
		t.Error("expected WarnAgentConflict")
	}
}

func TestEnrichFromFile_Directly(t *testing.T) {
	cfg := config.Load()
	path := filepath.Join(testdataDir(t), "session-003", "subagents", "agent-agent-001.jsonl")

	ag := &model.AgentNode{ID: "agent-001"}
	if err := EnrichFromFile(ag, path, cfg); err != nil {
		t.Fatalf("EnrichFromFile: %v", err)
	}
	if len(ag.ToolCalls) != 3 {
		t.Errorf("expected 3 tool calls, got %d", len(ag.ToolCalls))
	}
	if ag.ModelName == "" {
		t.Error("expected ModelName to be set")
	}
	if ag.TokensIn == 0 {
		t.Error("expected TokensIn to be accumulated")
	}
}

func TestEnrichFromFile_NotFound(t *testing.T) {
	cfg := config.Load()
	ag := &model.AgentNode{ID: "missing"}
	err := EnrichFromFile(ag, "/nonexistent/path.jsonl", cfg)
	if err == nil {
		t.Error("expected error for missing file")
	}
	// Agent should be untouched.
	if len(ag.ToolCalls) != 0 {
		t.Error("expected 0 tool calls for missing file")
	}
}

func TestEnrichFromEvents_NoToolCalls(t *testing.T) {
	// Agent with only text responses — no tool_use blocks.
	events := []*parser.RawEvent{
		{
			Type: "assistant",
			Message: &parser.RawMessage{
				Model: "claude-haiku-4-5-20251001",
				Content: []parser.ContentBlock{
					{Type: "text", Text: "I found nothing."},
				},
				Usage: &parser.Usage{InputTokens: 100, OutputTokens: 50},
			},
		},
	}

	ag := &model.AgentNode{ID: "text-only"}
	err := enrichFromEvents(ag, events)
	if err != nil {
		t.Fatalf("enrichFromEvents: %v", err)
	}
	if len(ag.ToolCalls) != 0 {
		t.Error("expected 0 tool calls for text-only agent")
	}
	if len(ag.FilesTouched) != 0 {
		t.Error("expected 0 files touched")
	}
	if ag.ModelName != "claude-haiku-4-5-20251001" {
		t.Errorf("ModelName = %q, want claude-haiku-4-5-20251001", ag.ModelName)
	}
	if ag.TokensIn != 100 {
		t.Errorf("TokensIn = %d, want 100", ag.TokensIn)
	}
}

func TestDetectFileConflicts_DeterministicOrder(t *testing.T) {
	// Multiple conflicting files should produce warnings in sorted path order.
	// Run multiple times to verify map iteration randomness doesn't affect output.
	for attempt := 0; attempt < 10; attempt++ {
		s := &model.Session{
			Prompts: []*model.Prompt{{
				Agents: []*model.AgentNode{
					{
						ID:         "a1",
						Label:      "subagent-1",
						IsParallel: true,
						ToolCalls: []*model.ToolCall{
							{Type: model.ToolEdit, Path: "z_last.go"},
							{Type: model.ToolEdit, Path: "a_first.go"},
							{Type: model.ToolEdit, Path: "m_middle.go"},
						},
						FilesTouched: []string{"z_last.go", "a_first.go", "m_middle.go"},
					},
					{
						ID:         "a2",
						Label:      "subagent-2",
						IsParallel: true,
						ToolCalls: []*model.ToolCall{
							{Type: model.ToolEdit, Path: "z_last.go"},
							{Type: model.ToolEdit, Path: "a_first.go"},
							{Type: model.ToolEdit, Path: "m_middle.go"},
						},
						FilesTouched: []string{"z_last.go", "a_first.go", "m_middle.go"},
					},
				},
			}},
		}

		detectFileConflicts(s)

		warnings := s.Prompts[0].Warnings
		if len(warnings) != 3 {
			t.Fatalf("attempt %d: expected 3 warnings, got %d", attempt, len(warnings))
		}
		// Warnings should be in alphabetical path order.
		if !strings.Contains(warnings[0].Message, "a_first.go") {
			t.Errorf("attempt %d: first warning should be a_first.go, got: %s", attempt, warnings[0].Message)
		}
		if !strings.Contains(warnings[1].Message, "m_middle.go") {
			t.Errorf("attempt %d: second warning should be m_middle.go, got: %s", attempt, warnings[1].Message)
		}
		if !strings.Contains(warnings[2].Message, "z_last.go") {
			t.Errorf("attempt %d: third warning should be z_last.go, got: %s", attempt, warnings[2].Message)
		}
	}
}

// findAgent finds an agent by ID in a slice.
func findAgent(agents []*model.AgentNode, id string) *model.AgentNode {
	for _, a := range agents {
		if a.ID == id {
			return a
		}
	}
	return nil
}
