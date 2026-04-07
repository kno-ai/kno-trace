package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/kno-ai/kno-trace/internal/model"
)

func testPromptWithAgents() *model.Prompt {
	return &model.Prompt{
		Index:     0,
		HumanText: "run parallel agents",
		StartTime: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 1, 1, 10, 2, 0, 0, time.UTC),
		ModelName: "claude-opus-4-6",
		Agents: []*model.AgentNode{
			{
				ID:              "agent-001",
				ToolUseID:       "toolu-001",
				Label:           "subagent-1",
				SubagentType:    "Explore",
				ModelName:       "claude-haiku-4-5-20251001",
				TaskDescription: "Find TODO comments in the codebase",
				Status:          model.AgentSucceeded,
				Duration:        25 * time.Second,
				TotalToolUseCount: 8,
				TotalTokens:     20000,
				TokensIn:        15000,
				TokensOut:       5000,
				IsParallel:      true,
				ToolCalls: []*model.ToolCall{
					{ID: "tc-1", Type: model.ToolGrep, Path: "TODO|FIXME"},
					{ID: "tc-2", Type: model.ToolRead, Path: "main.go"},
					{ID: "tc-3", Type: model.ToolRead, Path: "util.go"},
				},
				FilesTouched: []string{"main.go", "util.go"},
			},
			{
				ID:              "agent-002",
				ToolUseID:       "toolu-002",
				Label:           "subagent-2",
				SubagentType:    "Explore",
				Status:          model.AgentFailed,
				Duration:        5 * time.Second,
				TotalToolUseCount: 2,
			},
		},
	}
}

func TestDetail_ViewNilPrompt(t *testing.T) {
	d := Detail{Width: 80, Height: 24, itemCursor: -1}
	v := d.View(nil, false)
	if !strings.Contains(v, "Select a prompt") {
		t.Error("expected placeholder for nil prompt")
	}
}

func TestDetail_ViewWithAgents(t *testing.T) {
	d := Detail{Width: 80, Height: 40, itemCursor: -1}
	p := testPromptWithAgents()
	v := d.View(p, false)

	if !strings.Contains(v, "subagent-1") {
		t.Error("expected subagent-1 in output")
	}
	if !strings.Contains(v, "subagent-2") {
		t.Error("expected subagent-2 in output")
	}
	if !strings.Contains(v, "Explore") {
		t.Error("expected agent type in output")
	}
}

func TestDetail_AgentCursor(t *testing.T) {
	d := Detail{Width: 80, Height: 40, itemCursor: -1}
	p := testPromptWithAgents()

	// Initially no focus.
	if d.IsItemFocused() {
		t.Error("expected no agent focused initially")
	}

	// Move cursor down.
	d.AgentCursorDown(len(p.Agents))
	if d.itemCursor != 0 {
		t.Errorf("expected cursor at 0, got %d", d.itemCursor)
	}
	if !d.IsItemFocused() {
		t.Error("expected agent focused after cursor down")
	}

	// Move cursor down again.
	d.AgentCursorDown(len(p.Agents))
	if d.itemCursor != 1 {
		t.Errorf("expected cursor at 1, got %d", d.itemCursor)
	}

	// Move cursor down at end — should stay.
	d.AgentCursorDown(len(p.Agents))
	if d.itemCursor != 1 {
		t.Errorf("expected cursor to stay at 1, got %d", d.itemCursor)
	}

	// Move cursor up.
	d.AgentCursorUp()
	if d.itemCursor != 0 {
		t.Errorf("expected cursor at 0, got %d", d.itemCursor)
	}

	// Move cursor up at top — stays at 0.
	d.AgentCursorUp()
	if d.itemCursor != 0 {
		t.Errorf("expected cursor at 0 (clamped), got %d", d.itemCursor)
	}
}

func TestDetail_ExpandCollapse(t *testing.T) {
	d := Detail{Width: 80, Height: 40, itemCursor: -1}
	p := testPromptWithAgents()

	// Items: 0=agent-0, 1=agent-1 (no prompt-level tool calls in test fixture).
	d.itemCursor = 0
	_, agentIdx := ResolveItem(p, d.itemCursor)
	if agentIdx != 0 {
		t.Fatalf("expected agent index 0 at itemCursor 0, got %d", agentIdx)
	}

	// Expand.
	ok := d.ExpandAgent(p.Agents, agentIdx)
	if !ok {
		t.Fatal("expected ExpandAgent to succeed")
	}
	if !d.IsAgentExpanded() {
		t.Error("expected agent to be expanded")
	}
	if len(d.expandedPath) != 1 {
		t.Errorf("expected 1 level expanded, got %d", len(d.expandedPath))
	}
	if d.expandedPath[0] != "toolu-001" {
		t.Errorf("expected toolu-001 in path, got %s", d.expandedPath[0])
	}

	// View should show expanded agent detail.
	v := d.View(p, false)
	if !strings.Contains(v, "Find TODO comments") {
		t.Error("expected task description in expanded view")
	}
	if !strings.Contains(v, "main.go") {
		t.Error("expected file path in expanded view")
	}
	if !strings.Contains(v, "Touched: 2 files") {
		t.Error("expected files touched count in expanded view")
	}
	if !strings.Contains(v, "#1 > subagent-1") {
		t.Error("expected breadcrumb in expanded view")
	}

	// Collapse.
	collapsed := d.CollapseAgent()
	if !collapsed {
		t.Error("expected CollapseAgent to return true")
	}
	if d.IsAgentExpanded() {
		t.Error("expected agent to be collapsed")
	}

	// Collapse at prompt level — should return false.
	collapsed = d.CollapseAgent()
	if collapsed {
		t.Error("expected CollapseAgent to return false at prompt level")
	}
}

func TestDetail_ExpandedView_ToolCallList(t *testing.T) {
	d := Detail{Width: 80, Height: 40, itemCursor: -1}
	p := testPromptWithAgents()

	d.AgentCursorDown(len(p.Agents))
	d.ExpandAgent(p.Agents, d.itemCursor)
	v := d.View(p, false)

	// Should show tool calls from the agent.
	if !strings.Contains(v, "Tool calls") {
		t.Error("expected 'Tool calls' header")
	}
	if !strings.Contains(v, "TODO|FIXME") {
		t.Error("expected grep pattern in tool calls")
	}
}

func TestDetail_ExpandedView_FileOpCounts(t *testing.T) {
	d := Detail{Width: 80, Height: 40, itemCursor: -1}
	p := testPromptWithAgents()

	d.AgentCursorDown(len(p.Agents))
	d.ExpandAgent(p.Agents, d.itemCursor)
	v := d.View(p, false)

	// main.go should show R×1 (one Read).
	if !strings.Contains(v, "R×1") {
		t.Error("expected R×1 for main.go read count")
	}
}

func TestDetail_ExpandNonexistentAgent(t *testing.T) {
	d := Detail{Width: 80, Height: 40, itemCursor: -1}
	p := testPromptWithAgents()

	// Try to expand without focusing.
	ok := d.ExpandAgent(p.Agents, d.itemCursor)
	if ok {
		t.Error("expected ExpandAgent to fail when no agent focused")
	}

	// Try with empty agents.
	d.itemCursor = 0
	ok = d.ExpandAgent(nil, 0)
	if ok {
		t.Error("expected ExpandAgent to fail with nil agents")
	}
}

func TestDetail_ExpandStalePathRecovery(t *testing.T) {
	d := Detail{Width: 80, Height: 40, itemCursor: -1}

	// Manually set a stale expansion path.
	d.expandedPath = []string{"nonexistent-toolu"}

	p := testPromptWithAgents()
	v := d.View(p, false)

	// Should recover gracefully — resolve fails, collapses back.
	// The view should show prompt-level content (not crash).
	if strings.Contains(v, "nonexistent") {
		t.Error("stale path should be cleared")
	}
	if d.IsAgentExpanded() {
		t.Error("stale path should auto-collapse")
	}
	// Should show the prompt header.
	if !strings.Contains(v, "#1") {
		t.Error("expected prompt header after stale path recovery")
	}
}

func TestDetail_ResetExpansion(t *testing.T) {
	d := Detail{Width: 80, Height: 40, itemCursor: -1}

	d.expandedPath = []string{"toolu-001"}
	d.itemCursor = 1
	d.ResetExpansion()

	if d.IsAgentExpanded() {
		t.Error("expected expansion to be cleared")
	}
	if d.itemCursor != -1 {
		t.Errorf("expected itemCursor=-1, got %d", d.itemCursor)
	}
}

func TestDetail_AgentCursorNoAgents(t *testing.T) {
	d := Detail{Width: 80, Height: 24, itemCursor: -1}
	// Should not panic with 0 agents.
	d.AgentCursorDown(0)
	d.AgentCursorUp()
	if d.itemCursor != -1 {
		t.Errorf("expected cursor -1, got %d", d.itemCursor)
	}
}

func TestDetail_NestedAgentExpansion(t *testing.T) {
	child := &model.AgentNode{
		ID:              "agent-nested",
		ToolUseID:       "toolu-nested",
		Label:           "subagent-1a",
		SubagentType:    "Plan",
		TaskDescription: "Nested task",
		Status:          model.AgentSucceeded,
	}
	parent := &model.AgentNode{
		ID:              "agent-001",
		ToolUseID:       "toolu-001",
		Label:           "subagent-1",
		TaskDescription: "Parent task",
		Status:          model.AgentSucceeded,
		Children:        []*model.AgentNode{child},
	}
	p := &model.Prompt{
		Index:     0,
		ModelName: "claude-opus-4-6",
		Agents:    []*model.AgentNode{parent},
	}

	d := Detail{Width: 80, Height: 40, itemCursor: -1}

	// Expand parent.
	d.AgentCursorDown(1)
	d.ExpandAgent(p.Agents, d.itemCursor)
	v := d.View(p, false)
	if !strings.Contains(v, "Nested agents") {
		t.Error("expected nested agents section in expanded parent")
	}
	if !strings.Contains(v, "subagent-1a") {
		t.Error("expected nested agent label")
	}
}

func TestDetail_ComparisonView(t *testing.T) {
	d := Detail{Width: 80, Height: 40, itemCursor: -1}
	p := testPromptWithAgents()

	// Set comparison content.
	d.SetComparison("  Comparing #1 → #3\n\n  main.go\n  + new line\n")
	if !d.IsComparing() {
		t.Error("expected comparison mode")
	}

	v := d.View(p, false)
	if !strings.Contains(v, "Comparing") {
		t.Error("expected comparison content in view")
	}
	// Should not show prompt detail when comparing.
	if strings.Contains(v, "run parallel agents") {
		t.Error("should not show prompt text during comparison")
	}

	d.ClearComparison()
	if d.IsComparing() {
		t.Error("expected comparison to be cleared")
	}

	v = d.View(p, false)
	if !strings.Contains(v, "run parallel agents") {
		t.Error("expected prompt text after clearing comparison")
	}
}

func TestDetail_ComparisonPersistsAcrossPromptChange(t *testing.T) {
	d := Detail{Width: 80, Height: 40, itemCursor: -1}
	d.SetComparison("comparison content")

	// ResetExpansion should NOT clear comparison.
	d.ResetExpansion()
	if !d.IsComparing() {
		t.Error("comparison should persist across ResetExpansion")
	}
}

func TestDetail_ZeroWidthHeight(t *testing.T) {
	d := Detail{Width: 0, Height: 0, itemCursor: -1}
	p := testPromptWithAgents()
	// Should not panic.
	_ = d.View(p, false)
}

func TestDetail_RenderDeterministic(t *testing.T) {
	// File activity section uses maps internally. Verify render output is
	// identical across multiple calls (no map iteration order flicker).
	p := &model.Prompt{
		Index:     0,
		ModelName: "claude-opus-4-6",
		ToolCalls: []*model.ToolCall{
			{Type: model.ToolRead, Path: "z.go"},
			{Type: model.ToolEdit, Path: "a.go", OldStr: "x", NewStr: "y"},
			{Type: model.ToolWrite, Path: "m.go", Content: "content\n"},
			{Type: model.ToolRead, Path: "b.go"},
		},
	}
	d := Detail{Width: 80, Height: 80, itemCursor: -1}
	var first string
	for i := 0; i < 20; i++ {
		v := d.View(p, false)
		if i == 0 {
			first = v
		} else if v != first {
			t.Fatalf("render %d differs from render 0 — non-deterministic", i)
		}
	}
}

func TestDetail_ExpandedViewRunningAgent(t *testing.T) {
	p := &model.Prompt{
		Index:     0,
		ModelName: "claude-opus-4-6",
		Agents: []*model.AgentNode{
			{
				ID:              "agent-running",
				ToolUseID:       "toolu-running",
				Label:           "subagent-1",
				Status:          model.AgentRunning,
				StartTime:       time.Now().Add(-10 * time.Second),
				TaskDescription: "Still working",
			},
		},
	}

	d := Detail{Width: 80, Height: 40, itemCursor: -1}
	d.AgentCursorDown(1)
	d.ExpandAgent(p.Agents, d.itemCursor)
	v := d.View(p, true)

	if !strings.Contains(v, "running") {
		t.Error("expected running status in expanded view")
	}
	if !strings.Contains(v, "Waiting for tool calls") {
		t.Error("expected waiting message for agent with no tool calls")
	}
}
