package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/kno-ai/kno-trace/internal/model"
)

func testCompareSession() *model.Session {
	return &model.Session{
		Prompts: []*model.Prompt{
			{
				Index:     0,
				HumanText: "initial setup",
				StartTime: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
				EndTime:   time.Date(2026, 1, 1, 10, 1, 0, 0, time.UTC),
				ToolCalls: []*model.ToolCall{
					{Type: model.ToolWrite, Path: "main.go", Content: "package main\n\nfunc main() {}\n"},
				},
			},
			{
				Index:     1,
				HumanText: "add feature",
				StartTime: time.Date(2026, 1, 1, 10, 1, 0, 0, time.UTC),
				EndTime:   time.Date(2026, 1, 1, 10, 2, 0, 0, time.UTC),
				ToolCalls: []*model.ToolCall{
					{Type: model.ToolEdit, Path: "main.go", OldStr: "func main() {}", NewStr: "func main() {\n\tfmt.Println(\"hello\")\n}"},
				},
			},
			{
				Index:     2,
				HumanText: "fix bug",
				StartTime: time.Date(2026, 1, 1, 10, 2, 0, 0, time.UTC),
				EndTime:   time.Date(2026, 1, 1, 10, 3, 0, 0, time.UTC),
				ToolCalls: []*model.ToolCall{
					{Type: model.ToolEdit, Path: "main.go", OldStr: "hello", NewStr: "world"},
					{Type: model.ToolBash, Command: "go build ./..."},
				},
			},
		},
	}
}

func TestBuildComparison_TwoTurns(t *testing.T) {
	s := testCompareSession()
	result := buildComparison(s, 0, 1, 80)

	if !strings.Contains(result, "Comparing #1 → #2") {
		t.Error("expected comparison header")
	}
	if !strings.Contains(result, "main.go") {
		t.Error("expected main.go in comparison")
	}
	if !strings.Contains(result, "1 files changed") {
		t.Error("expected file count")
	}
}

func TestBuildComparison_Range(t *testing.T) {
	s := testCompareSession()
	result := buildComparison(s, 0, 2, 80)

	if !strings.Contains(result, "Comparing #1 → #3") {
		t.Error("expected comparison header for range")
	}
	if !strings.Contains(result, "main.go") {
		t.Error("expected main.go in range comparison")
	}
}

func TestBuildComparison_NoChanges(t *testing.T) {
	s := &model.Session{
		Prompts: []*model.Prompt{
			{Index: 0, ToolCalls: []*model.ToolCall{{Type: model.ToolRead, Path: "foo.go"}}},
			{Index: 1, ToolCalls: []*model.ToolCall{{Type: model.ToolRead, Path: "bar.go"}}},
		},
	}
	result := buildComparison(s, 0, 1, 80)

	if !strings.Contains(result, "No file changes") {
		t.Error("expected no changes message")
	}
}

func TestBuildComparison_NilSession(t *testing.T) {
	result := buildComparison(nil, 0, 1, 80)
	if !strings.Contains(result, "Select two turns") {
		t.Error("expected prompt to select turns")
	}
}

func TestBuildComparison_InvalidRange(t *testing.T) {
	s := testCompareSession()
	result := buildComparison(s, 2, 0, 80) // reversed
	if !strings.Contains(result, "Select two turns") {
		t.Error("expected prompt for invalid range")
	}
}

func TestBuildComparison_WithAgentChanges(t *testing.T) {
	s := &model.Session{
		Prompts: []*model.Prompt{
			{Index: 0},
			{
				Index: 1,
				Agents: []*model.AgentNode{
					{
						Label: "subagent-1",
						ToolCalls: []*model.ToolCall{
							{Type: model.ToolEdit, Path: "agent.go", OldStr: "old", NewStr: "new"},
						},
					},
				},
			},
		},
	}
	result := buildComparison(s, 0, 1, 80)

	if !strings.Contains(result, "agent.go") {
		t.Error("expected agent file change")
	}
	if !strings.Contains(result, "subagent-1") {
		t.Error("expected agent attribution")
	}
}

func TestBuildComparison_BashWarning(t *testing.T) {
	s := &model.Session{
		Prompts: []*model.Prompt{
			{Index: 0},
			{
				Index: 1,
				ToolCalls: []*model.ToolCall{
					{Type: model.ToolEdit, Path: "main.go", OldStr: "a", NewStr: "b"},
					{Type: model.ToolBash, Command: "make build"},
				},
			},
		},
	}
	result := buildComparison(s, 0, 1, 80)

	if !strings.Contains(result, "main.go") {
		t.Error("expected file in comparison")
	}
}

func TestPromptList_Selection(t *testing.T) {
	pl := PromptList{
		Prompts: []*model.Prompt{
			{Index: 0},
			{Index: 1},
			{Index: 2},
		},
	}

	if pl.SelectedCount() != 0 {
		t.Error("expected 0 selected initially")
	}

	pl.Cursor = 0
	pl.ToggleSelection()
	if pl.SelectedCount() != 1 {
		t.Errorf("expected 1 selected, got %d", pl.SelectedCount())
	}

	pl.Cursor = 2
	pl.ToggleSelection()
	if pl.SelectedCount() != 2 {
		t.Errorf("expected 2 selected, got %d", pl.SelectedCount())
	}

	indices := pl.SelectedPromptIndices()
	if len(indices) != 2 || indices[0] != 0 || indices[1] != 2 {
		t.Errorf("expected [0, 2], got %v", indices)
	}

	// Deselect.
	pl.Cursor = 0
	pl.ToggleSelection()
	if pl.SelectedCount() != 1 {
		t.Errorf("expected 1 after deselect, got %d", pl.SelectedCount())
	}

	pl.ClearSelection()
	if pl.SelectedCount() != 0 {
		t.Error("expected 0 after clear")
	}
}

func TestPromptList_SelectionOutOfBounds(t *testing.T) {
	pl := PromptList{}
	// Should not panic.
	pl.ToggleSelection()
	pl.ClearSelection()
	if pl.SelectedCount() != 0 {
		t.Error("expected 0")
	}
}
