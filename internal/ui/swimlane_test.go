package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/kno-ai/kno-trace/internal/model"
)

func testSession() *model.Session {
	return &model.Session{
		ID:          "test-session",
		ProjectName: "test",
		ModelName:   "claude-opus-4-6",
		StartTime:   time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		EndTime:     time.Date(2026, 1, 1, 10, 5, 0, 0, time.UTC),
		Prompts: []*model.Prompt{
			{
				Index:     0,
				HumanText: "do something simple",
				StartTime: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2026, 1, 1, 10, 1, 0, 0, time.UTC),
				ModelName: "claude-opus-4-6",
			},
			{
				Index:     1,
				HumanText: "run agents in parallel",
				StartTime: time.Date(2026, 1, 1, 10, 1, 0, 0, time.UTC),
				EndTime:   time.Date(2026, 1, 1, 10, 3, 0, 0, time.UTC),
				ModelName: "claude-opus-4-6",
				Agents: []*model.AgentNode{
					{
						ID:              "agent-001",
						ToolUseID:       "toolu-001",
						Label:           "subagent-1",
						SubagentType:    "Explore",
						ModelName:       "claude-haiku-4-5-20251001",
						TaskDescription: "Find TODO comments",
						StartTime:       time.Date(2026, 1, 1, 10, 1, 5, 0, time.UTC),
						EndTime:         time.Date(2026, 1, 1, 10, 1, 30, 0, time.UTC),
						Duration:        25 * time.Second,
						Status:          model.AgentSucceeded,
						IsParallel:      true,
						TotalToolUseCount: 8,
						TotalTokens:     20000,
						ToolCalls: []*model.ToolCall{
							{ID: "tc-1", Type: model.ToolGrep, Path: "TODO|FIXME", SourceAgent: "agent-001"},
							{ID: "tc-2", Type: model.ToolRead, Path: "main.go", SourceAgent: "agent-001"},
						},
						FilesTouched: []string{"main.go"},
					},
					{
						ID:              "agent-002",
						ToolUseID:       "toolu-002",
						Label:           "subagent-2",
						SubagentType:    "Explore",
						ModelName:       "claude-haiku-4-5-20251001",
						TaskDescription: "Find external imports",
						StartTime:       time.Date(2026, 1, 1, 10, 1, 5, 0, time.UTC),
						EndTime:         time.Date(2026, 1, 1, 10, 2, 0, 0, time.UTC),
						Duration:        55 * time.Second,
						Status:          model.AgentSucceeded,
						IsParallel:      true,
						TotalToolUseCount: 15,
						TotalTokens:     40000,
						ToolCalls: []*model.ToolCall{
							{ID: "tc-3", Type: model.ToolGlob, Path: "**/*.go", SourceAgent: "agent-002"},
							{ID: "tc-4", Type: model.ToolRead, Path: "main.go", SourceAgent: "agent-002"},
							{ID: "tc-5", Type: model.ToolEdit, Path: "main.go", SourceAgent: "agent-002", OldStr: "x", NewStr: "y"},
						},
						FilesTouched: []string{"main.go"},
					},
				},
				Warnings: []model.Warning{
					{Type: model.WarnAgentConflict, Message: "file conflict: main.go touched by subagent-1, subagent-2"},
				},
			},
		},
	}
}

func TestSwimlane_NoSession(t *testing.T) {
	s := swimlaneModel{}
	s.setSize(80, 24)
	v := s.View()
	if !strings.Contains(v, "No session") {
		t.Error("expected 'No session' for nil session")
	}
}

func TestSwimlane_NoAgents(t *testing.T) {
	session := &model.Session{
		Prompts: []*model.Prompt{
			{Index: 0, HumanText: "hello"},
		},
	}
	s := newSwimlane(session)
	s.setSize(80, 24)
	v := s.View()
	if !strings.Contains(v, "No agents in this prompt") {
		t.Error("expected empty state for prompt without agents")
	}
}

func TestSwimlane_ParallelAgents(t *testing.T) {
	session := testSession()
	s := newSwimlane(session)
	s.setSize(120, 40)
	v := s.View()

	// Should show both agent lanes.
	if !strings.Contains(v, "subagent-1") {
		t.Error("expected subagent-1 in swimlane")
	}
	if !strings.Contains(v, "subagent-2") {
		t.Error("expected subagent-2 in swimlane")
	}

	// Should show parallel badge.
	if !strings.Contains(v, "[parallel]") {
		t.Error("expected [parallel] badge")
	}

	// Should show conflict banner.
	if !strings.Contains(v, "file conflict") {
		t.Error("expected file conflict warning")
	}

	// Should show parent lane.
	if !strings.Contains(v, "parent") {
		t.Error("expected parent lane")
	}

	// Should auto-select prompt with agents (prompt index 1).
	if s.promptIdx != 1 {
		t.Errorf("expected promptIdx=1 (prompt with agents), got %d", s.promptIdx)
	}
}

func TestSwimlane_FailedAgent(t *testing.T) {
	session := &model.Session{
		Prompts: []*model.Prompt{
			{
				Index: 0,
				Agents: []*model.AgentNode{
					{
						Label:    "subagent-1",
						Status:   model.AgentFailed,
						Duration: 5 * time.Second,
						TotalToolUseCount: 2,
					},
				},
			},
		},
	}
	s := newSwimlane(session)
	s.setSize(80, 24)
	v := s.View()
	if !strings.Contains(v, "✗ failed") {
		t.Error("expected failed indicator")
	}
}

func TestSwimlane_RunningAgent(t *testing.T) {
	session := &model.Session{
		IsLive: true,
		Prompts: []*model.Prompt{
			{
				Index: 0,
				Agents: []*model.AgentNode{
					{
						Label:     "subagent-1",
						Status:    model.AgentRunning,
						StartTime: time.Now().Add(-10 * time.Second),
						ToolCalls: []*model.ToolCall{
							{ID: "tc-1", Type: model.ToolRead, Path: "file.go"},
						},
					},
				},
			},
		},
	}
	s := newSwimlane(session)
	s.isLive = true
	s.setSize(80, 24)
	v := s.View()
	if !strings.Contains(v, "running") {
		t.Error("expected running indicator")
	}
}

func TestSwimlane_PromptNavigation(t *testing.T) {
	session := testSession()
	s := newSwimlane(session)
	s.setSize(80, 24)

	// Should start on prompt 1 (has agents).
	if s.promptIdx != 1 {
		t.Fatalf("expected promptIdx=1, got %d", s.promptIdx)
	}

	// Navigate to prev — no earlier prompt with agents.
	s.prevPromptWithAgents()
	if s.promptIdx != 1 {
		t.Errorf("expected promptIdx to stay at 1 (no earlier agents), got %d", s.promptIdx)
	}

	// Navigate to next — no later prompt with agents.
	s.nextPromptWithAgents()
	if s.promptIdx != 1 {
		t.Errorf("expected promptIdx to stay at 1 (no later agents), got %d", s.promptIdx)
	}
}

func TestSwimlane_ScrollClamp(t *testing.T) {
	session := testSession()
	s := newSwimlane(session)
	s.setSize(80, 5) // very small height

	// Should not panic with scroll beyond content.
	s.scroll = 1000
	_ = s.View()
}

func TestSwimlane_EmptySession(t *testing.T) {
	session := &model.Session{Prompts: []*model.Prompt{}}
	s := newSwimlane(session)
	s.setSize(80, 24)
	// Should not panic.
	_ = s.View()
}

func TestSwimlane_PromptSelectorBar(t *testing.T) {
	session := testSession()
	s := newSwimlane(session)
	bar := s.promptSelectorBar(120)

	// Should show both prompts.
	if !strings.Contains(bar, "#1") {
		t.Error("expected #1 in selector bar")
	}
	if !strings.Contains(bar, "#2") {
		t.Error("expected #2 in selector bar")
	}
}

func TestSwimlane_UnlinkedAgents(t *testing.T) {
	session := &model.Session{
		Prompts: []*model.Prompt{
			{
				Index: 0,
				Agents: []*model.AgentNode{
					{Label: "subagent-1", Status: model.AgentSucceeded},
				},
			},
		},
		UnlinkedAgents: []*model.AgentNode{
			{Label: "mystery-agent", ParentPromptIdx: 0},
		},
	}
	s := newSwimlane(session)
	s.setSize(80, 24)
	v := s.View()
	if !strings.Contains(v, "mystery-agent") {
		t.Error("expected unlinked agent to appear")
	}
	if !strings.Contains(v, "unresolved") {
		t.Error("expected 'unresolved' label for unlinked agent")
	}
}

func TestExtractConflictPaths(t *testing.T) {
	warnings := []model.Warning{
		{Type: model.WarnAgentConflict, Message: "file conflict: main.go touched by subagent-1, subagent-2"},
		{Type: model.WarnAgentConflict, Message: "file conflict: util.go touched by subagent-1, subagent-2"},
		{Type: model.WarnLoopDetected, Message: "unrelated warning"},
	}
	paths := extractConflictPaths(warnings)
	if !paths["main.go"] {
		t.Error("expected main.go in conflict paths")
	}
	if !paths["util.go"] {
		t.Error("expected util.go in conflict paths")
	}
	if len(paths) != 2 {
		t.Errorf("expected 2 conflict paths, got %d", len(paths))
	}
}
