package parser

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kno-ai/kno-trace/internal/config"
	"github.com/kno-ai/kno-trace/internal/model"
)

func testdataDir() string {
	return filepath.Join("..", "testdata")
}

func parseFixture(t *testing.T, name string) *model.Session {
	t.Helper()
	path := filepath.Join(testdataDir(), name)
	cfg := config.Default()
	events, err := ParseFile(path, cfg)
	if err != nil {
		t.Fatalf("ParseFile(%s) failed: %v", name, err)
	}
	return BuildSession(events, cfg)
}

// --- Fixture integration tests ---

func TestSimple(t *testing.T) {
	s := parseFixture(t, "simple.jsonl")

	if len(s.Prompts) != 3 {
		t.Fatalf("got %d prompts, want 3", len(s.Prompts))
	}
	if s.ModelName != "claude-opus-4-6" {
		t.Errorf("ModelName = %q, want claude-opus-4-6", s.ModelName)
	}
	if s.Interrupted {
		t.Error("session should not be interrupted")
	}

	// Prompt 1: Read + Bash.
	p1 := s.Prompts[0]
	if p1.HumanText != "[implement the parser]" {
		t.Errorf("p1.HumanText = %q", p1.HumanText)
	}
	toolTypes := toolTypeList(p1)
	if !contains(toolTypes, model.ToolRead) {
		t.Errorf("p1 missing Read tool, got %v", toolTypes)
	}
	if !contains(toolTypes, model.ToolBash) {
		t.Errorf("p1 missing Bash tool, got %v", toolTypes)
	}

	// Prompt 2: Write + Edit.
	p2 := s.Prompts[1]
	if !contains(toolTypeList(p2), model.ToolWrite) {
		t.Error("p2 missing Write")
	}
	if !contains(toolTypeList(p2), model.ToolEdit) {
		t.Error("p2 missing Edit")
	}

	// Prompt 3: Bash.
	p3 := s.Prompts[2]
	if !contains(toolTypeList(p3), model.ToolBash) {
		t.Error("p3 missing Bash")
	}
}

func TestInterrupted(t *testing.T) {
	s := parseFixture(t, "interrupted.jsonl")

	if len(s.Prompts) != 2 {
		t.Fatalf("got %d prompts, want 2", len(s.Prompts))
	}
	if !s.Interrupted {
		t.Error("session should be interrupted")
	}
	if !s.Prompts[1].Interrupted {
		t.Error("prompt 2 should be interrupted")
	}
	if s.Prompts[0].Interrupted {
		t.Error("prompt 1 should NOT be interrupted")
	}
}

func TestWithAgent(t *testing.T) {
	s := parseFixture(t, "with_agent.jsonl")

	if len(s.Prompts) < 1 {
		t.Fatal("no prompts")
	}
	p1 := s.Prompts[0]

	if len(p1.Agents) != 1 {
		t.Fatalf("p1 has %d agents, want 1", len(p1.Agents))
	}
	agent := p1.Agents[0]
	if agent.SubagentType != "Explore" {
		t.Errorf("agent SubagentType = %q, want Explore", agent.SubagentType)
	}
	if agent.Status != model.AgentSucceeded {
		t.Errorf("agent Status = %q, want succeeded", agent.Status)
	}
	if agent.TotalToolUseCount != 8 {
		t.Errorf("agent TotalToolUseCount = %d, want 8", agent.TotalToolUseCount)
	}
}

func TestParallelAgents(t *testing.T) {
	s := parseFixture(t, "parallel_agents.jsonl")

	if len(s.Prompts) < 1 {
		t.Fatal("no prompts")
	}
	p1 := s.Prompts[0]

	if len(p1.Agents) != 2 {
		t.Fatalf("p1 has %d agents, want 2", len(p1.Agents))
	}

	// Both agents should have tool_use entries.
	agentToolCalls := 0
	for _, tc := range p1.ToolCalls {
		if tc.Type == model.ToolAgent {
			agentToolCalls++
		}
	}
	if agentToolCalls != 2 {
		t.Errorf("got %d agent tool calls, want 2", agentToolCalls)
	}
}

func TestWithCompact(t *testing.T) {
	s := parseFixture(t, "with_compact.jsonl")

	if len(s.Prompts) != 2 {
		t.Fatalf("got %d prompts, want 2", len(s.Prompts))
	}
	if len(s.CompactAt) != 1 {
		t.Fatalf("CompactAt = %v, want 1 entry", s.CompactAt)
	}
}

func TestMCPCalls(t *testing.T) {
	s := parseFixture(t, "mcp_calls.jsonl")

	if len(s.Prompts) < 1 {
		t.Fatal("no prompts")
	}

	mcpCount := 0
	for _, tc := range s.Prompts[0].ToolCalls {
		if tc.Type == model.ToolMCP {
			mcpCount++
		}
	}
	if mcpCount != 2 {
		t.Errorf("got %d MCP calls, want 2", mcpCount)
	}

	// Should have MCP warnings.
	mcpWarnings := 0
	for _, w := range s.Prompts[0].Warnings {
		if w.Type == model.WarnMCPExternal {
			mcpWarnings++
		}
	}
	if mcpWarnings != 2 {
		t.Errorf("got %d MCP warnings, want 2", mcpWarnings)
	}
}

func TestAdvancedSession(t *testing.T) {
	s := parseFixture(t, "advanced_session.jsonl")

	if len(s.Prompts) != 5 {
		t.Fatalf("got %d prompts, want 5", len(s.Prompts))
	}

	// Branch transition should be on prompt 2 (index 1).
	p2 := s.Prompts[1]
	if p2.BranchTransition.From != "main" || p2.BranchTransition.To != "feature/auth-refactor" {
		t.Errorf("p2 BranchTransition = %+v, want main→feature/auth-refactor", p2.BranchTransition)
	}

	// Model switch: prompts 1-2 opus, 3-5 sonnet.
	if !strings.Contains(s.Prompts[0].ModelName, "opus") {
		t.Errorf("p1 model = %q, want opus", s.Prompts[0].ModelName)
	}
	if !strings.Contains(s.Prompts[2].ModelName, "sonnet") {
		t.Errorf("p3 model = %q, want sonnet", s.Prompts[2].ModelName)
	}

	// Duration outlier: 5 prompts exist, so computation runs.
	// Verify it doesn't crash and at least runs. The fixture has durations
	// of ~60s, ~60s, ~60s, ~60s, ~60s derived from timestamps — may not
	// produce an outlier depending on exact values.
	outlierCount := 0
	for _, p := range s.Prompts {
		if p.IsDurationOutlier {
			outlierCount++
		}
	}
	// With 5 prompts, outlier detection should have run (no panic).
	t.Logf("duration outliers found: %d", outlierCount)

	// Agent on prompt 2.
	if len(p2.Agents) != 1 {
		t.Fatalf("p2 has %d agents, want 1", len(p2.Agents))
	}
}

func TestReplayChain(t *testing.T) {
	s := parseFixture(t, "replay_chain.jsonl")

	if len(s.Prompts) != 2 {
		t.Fatalf("got %d prompts, want 2", len(s.Prompts))
	}

	// Prompt 1: Read + 2 Edits.
	p1 := s.Prompts[0]
	types1 := toolTypeList(p1)
	readCount := countType(types1, model.ToolRead)
	editCount := countType(types1, model.ToolEdit)
	if readCount != 1 {
		t.Errorf("p1: %d reads, want 1", readCount)
	}
	if editCount != 2 {
		t.Errorf("p1: %d edits, want 2", editCount)
	}

	// Prompt 2: Write + 2 Edits (one fails).
	p2 := s.Prompts[1]
	types2 := toolTypeList(p2)
	writeCount := countType(types2, model.ToolWrite)
	editCount2 := countType(types2, model.ToolEdit)
	if writeCount != 1 {
		t.Errorf("p2: %d writes, want 1", writeCount)
	}
	if editCount2 != 2 {
		t.Errorf("p2: %d edits, want 2", editCount2)
	}
}

// --- Resilience tests ---

func TestEdgeCases(t *testing.T) {
	// Must not panic on malformed JSON, blank lines, isMeta messages.
	s := parseFixture(t, "edge_cases.jsonl")

	if len(s.Prompts) != 2 {
		t.Fatalf("got %d prompts, want 2", len(s.Prompts))
	}
	// Should parse despite malformed line 4 and blank line 5.
}

func TestMissingType(t *testing.T) {
	events, err := ParseReader(strings.NewReader(`{"uuid":"x","timestamp":"2024-01-01T00:00:00Z"}`))
	if err != nil {
		t.Fatal(err)
	}
	// No type field → should be skipped.
	if len(events) != 0 {
		t.Errorf("got %d events, want 0 (no type field)", len(events))
	}
}

func TestMissingMessage(t *testing.T) {
	events, err := ParseReader(strings.NewReader(
		`{"type":"user","uuid":"x","timestamp":"2024-01-01T00:00:00Z"}`))
	if err != nil {
		t.Fatal(err)
	}
	// user line with no message → event exists but no message.
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].Message != nil {
		t.Error("expected nil message")
	}
}

func TestNullContent(t *testing.T) {
	line := `{"type":"assistant","message":{"role":"assistant","content":null,"stop_reason":"end_turn"},"uuid":"x","timestamp":"2024-01-01T00:00:00Z","requestId":"r1"}`
	events, err := ParseReader(strings.NewReader(line))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if len(events[0].Message.Content) != 0 {
		t.Error("expected empty content from null")
	}
}

func TestEmptyContentArray(t *testing.T) {
	line := `{"type":"assistant","message":{"role":"assistant","content":[],"stop_reason":"end_turn"},"uuid":"x","timestamp":"2024-01-01T00:00:00Z","requestId":"r1"}`
	events, err := ParseReader(strings.NewReader(line))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if len(events[0].Message.Content) != 0 {
		t.Error("expected empty content")
	}
}

func TestNullToolUseResult(t *testing.T) {
	line := `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1"}]},"toolUseResult":null,"sourceToolAssistantUUID":"a1","uuid":"x","timestamp":"2024-01-01T00:00:00Z"}`
	events, err := ParseReader(strings.NewReader(line))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
}

func TestUnknownContentBlockType(t *testing.T) {
	line := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"new_block_type","data":"x"},{"type":"text","text":"hello"}],"stop_reason":"end_turn"},"uuid":"x","timestamp":"2024-01-01T00:00:00Z","requestId":"r1"}`
	events, err := ParseReader(strings.NewReader(line))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events", len(events))
	}
	// Unknown block skipped, text block kept.
	if len(events[0].Message.Content) != 1 {
		t.Errorf("got %d content blocks, want 1 (text only)", len(events[0].Message.Content))
	}
	if events[0].Message.Content[0].Text != "hello" {
		t.Error("text block not preserved")
	}
}

func TestUnknownType(t *testing.T) {
	events, err := ParseReader(strings.NewReader(
		`{"type":"future_type","uuid":"x","timestamp":"2024-01-01T00:00:00Z"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Errorf("unknown type should be skipped, got %d events", len(events))
	}
}

func TestUnknownSystemSubtype(t *testing.T) {
	line := `{"type":"system","subtype":"future_subtype","uuid":"x","timestamp":"2024-01-01T00:00:00Z"}`
	events, err := ParseReader(strings.NewReader(line))
	if err != nil {
		t.Fatal(err)
	}
	// Unknown system subtype → event exists (for timestamp) but no special handling.
	if len(events) != 1 {
		t.Errorf("got %d events, want 1", len(events))
	}
}

func TestUnknownProgressDataType(t *testing.T) {
	line := `{"type":"progress","data":{"type":"future_progress"},"uuid":"x","timestamp":"2024-01-01T00:00:00Z"}`
	events, err := ParseReader(strings.NewReader(line))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Errorf("got %d events, want 1", len(events))
	}
}

func TestBadTimestamp(t *testing.T) {
	line := `{"type":"user","message":{"role":"user","content":"hello"},"uuid":"x","timestamp":"not-a-date"}`
	events, err := ParseReader(strings.NewReader(line))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events", len(events))
	}
	if !events[0].Timestamp.IsZero() {
		t.Error("expected zero time for bad timestamp")
	}
}

func TestAllStopReasonNull(t *testing.T) {
	// All assistant lines have stop_reason: null — should use last snapshot.
	lines := `{"type":"user","message":{"role":"user","content":"test"},"uuid":"u1","timestamp":"2024-01-01T00:00:00Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"partial"}],"stop_reason":null},"uuid":"u2","timestamp":"2024-01-01T00:00:01Z","requestId":"r1"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"more partial"}],"stop_reason":null},"uuid":"u3","timestamp":"2024-01-01T00:00:02Z","requestId":"r1"}`
	events, err := ParseReader(strings.NewReader(lines))
	if err != nil {
		t.Fatal(err)
	}
	// Should get user + one resolved assistant (latest snapshot as fallback).
	assistantCount := 0
	for _, e := range events {
		if e.Type == "assistant" {
			assistantCount++
		}
	}
	if assistantCount != 1 {
		t.Errorf("got %d assistant events, want 1 (deduped)", assistantCount)
	}
}

// --- Classify tests ---

func TestIsCLAUDEMDPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"CLAUDE.md", true},
		{"claude.md", true},
		{"AGENTS.md", true},
		{".claude/memory/foo.md", true},
		{".claude/settings.json", true},
		{".claude/commands/build.md", true},
		{"internal/parser/parser.go", false},
		{"README.md", false},
	}
	for _, tt := range tests {
		got := isCLAUDEMDPath(tt.path)
		if got != tt.want {
			t.Errorf("isCLAUDEMDPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestLoopDetection(t *testing.T) {
	cfg := config.Default()
	cfg.LoopDetectionThreshold = 3

	p := &model.Prompt{
		ToolCalls: []*model.ToolCall{
			{Type: model.ToolEdit, Path: "foo.go"},
			{Type: model.ToolEdit, Path: "foo.go"},
			{Type: model.ToolEdit, Path: "foo.go"}, // 3rd → triggers
		},
	}
	classifyPrompt(p, cfg)

	loopWarnings := 0
	for _, w := range p.Warnings {
		if w.Type == model.WarnLoopDetected {
			loopWarnings++
		}
	}
	if loopWarnings != 1 {
		t.Errorf("got %d loop warnings, want 1", loopWarnings)
	}
}

func TestLoopDetectionBelowThreshold(t *testing.T) {
	cfg := config.Default()
	cfg.LoopDetectionThreshold = 3

	p := &model.Prompt{
		ToolCalls: []*model.ToolCall{
			{Type: model.ToolEdit, Path: "foo.go"},
			{Type: model.ToolEdit, Path: "foo.go"},
		},
	}
	classifyPrompt(p, cfg)

	for _, w := range p.Warnings {
		if w.Type == model.WarnLoopDetected {
			t.Error("should not fire below threshold")
		}
	}
}

func TestContextWarnings(t *testing.T) {
	cfg := config.Default()
	cfg.ContextHighPct = 70
	cfg.ContextCriticalPct = 85

	p := &model.Prompt{ContextPct: 75}
	classifyPrompt(p, cfg)
	hasHigh := false
	for _, w := range p.Warnings {
		if w.Type == model.WarnContextHigh {
			hasHigh = true
		}
	}
	if !hasHigh {
		t.Error("expected WarnContextHigh at 75%")
	}

	p2 := &model.Prompt{ContextPct: 90}
	classifyPrompt(p2, cfg)
	hasCritical := false
	for _, w := range p2.Warnings {
		if w.Type == model.WarnContextCritical {
			hasCritical = true
		}
	}
	if !hasCritical {
		t.Error("expected WarnContextCritical at 90%")
	}
}

func TestDurationOutlierRequires5Prompts(t *testing.T) {
	s := parseFixture(t, "simple.jsonl")
	// 3 prompts — no outliers should be computed.
	for _, p := range s.Prompts {
		if p.IsDurationOutlier {
			t.Error("should not flag outliers with <5 prompts")
		}
	}
}

// --- Helpers ---

func toolTypeList(p *model.Prompt) []model.ToolType {
	var types []model.ToolType
	for _, tc := range p.ToolCalls {
		types = append(types, tc.Type)
	}
	return types
}

func contains(list []model.ToolType, item model.ToolType) bool {
	for _, v := range list {
		if v == item {
			return true
		}
	}
	return false
}

func countType(list []model.ToolType, item model.ToolType) int {
	n := 0
	for _, v := range list {
		if v == item {
			n++
		}
	}
	return n
}
