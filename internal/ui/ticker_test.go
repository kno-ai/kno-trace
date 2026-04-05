package ui

import (
	"testing"
	"time"

	"github.com/kno-ai/kno-trace/internal/model"
)

func TestTickerZeroValueSafe(t *testing.T) {
	// A zero-value Ticker (not created via NewTicker) must not panic.
	var ticker Ticker
	ticker.Push([]TickerEntry{{
		ToolType:  model.ToolRead,
		Path:      "file.go",
		Timestamp: time.Now(),
	}})
	if len(ticker.entries) != 1 {
		t.Errorf("entries: got %d, want 1", len(ticker.entries))
	}

	// AgentColor on zero-value ticker should also not panic.
	_ = ticker.AgentColor("agent-1")

	// View should not panic.
	_ = ticker.View(80, time.Now())
}

func TestTickerPush(t *testing.T) {
	ticker := NewTicker(3)

	entries := make([]TickerEntry, 5)
	for i := range entries {
		entries[i] = TickerEntry{
			ToolType:  model.ToolRead,
			Path:      "file.go",
			Timestamp: time.Now(),
		}
	}
	ticker.Push(entries)

	if len(ticker.entries) != 5 {
		t.Errorf("entries: got %d, want 5", len(ticker.entries))
	}
}

func TestTickerPushTrim(t *testing.T) {
	ticker := NewTicker(3)
	ticker.maxEntries = 10

	entries := make([]TickerEntry, 15)
	for i := range entries {
		entries[i] = TickerEntry{
			ToolType:  model.ToolRead,
			Path:      "file.go",
			Timestamp: time.Now(),
		}
	}
	ticker.Push(entries)

	if len(ticker.entries) != 10 {
		t.Errorf("entries after trim: got %d, want 10", len(ticker.entries))
	}
}

func TestTickerLoopDetection(t *testing.T) {
	ticker := NewTicker(3)

	// Push 3 entries with same tool+path — should trigger loop warning.
	for i := 0; i < 3; i++ {
		ticker.Push([]TickerEntry{{
			ToolType:  model.ToolEdit,
			Path:      "config.go",
			Timestamp: time.Now(),
		}})
	}

	if ticker.loopWarning == "" {
		t.Error("expected loop warning after 3 identical tool+path entries")
	}
}

func TestTickerLoopUpdatesCount(t *testing.T) {
	ticker := NewTicker(3)

	// Push 5 entries with same tool+path — warning should update count.
	for i := 0; i < 5; i++ {
		ticker.Push([]TickerEntry{{
			ToolType:  model.ToolEdit,
			Path:      "config.go",
			Timestamp: time.Now(),
		}})
	}

	if ticker.loopWarning == "" {
		t.Error("expected loop warning")
	}
	// The warning should reflect the latest count (5).
	expected := "⟳ possible loop: edit config.go repeated 5×"
	if ticker.loopWarning != expected {
		t.Errorf("loop warning: got %q, want %q", ticker.loopWarning, expected)
	}
}

func TestTickerLoopClearsOnDifferentEntry(t *testing.T) {
	ticker := NewTicker(3)

	// Trigger loop warning.
	for i := 0; i < 3; i++ {
		ticker.Push([]TickerEntry{{
			ToolType:  model.ToolEdit,
			Path:      "config.go",
			Timestamp: time.Now(),
		}})
	}
	if ticker.loopWarning == "" {
		t.Fatal("expected loop warning")
	}

	// Different tool+path should clear the warning.
	ticker.Push([]TickerEntry{{
		ToolType:  model.ToolRead,
		Path:      "other.go",
		Timestamp: time.Now(),
	}})

	if ticker.loopWarning != "" {
		t.Errorf("loop warning should be cleared after different entry, got %q", ticker.loopWarning)
	}
}

func TestTickerResetOnNewPrompt(t *testing.T) {
	ticker := NewTicker(3)

	// Create a loop condition.
	for i := 0; i < 4; i++ {
		ticker.Push([]TickerEntry{{
			ToolType:  model.ToolEdit,
			Path:      "config.go",
			Timestamp: time.Now(),
		}})
	}
	if ticker.loopWarning == "" {
		t.Fatal("expected loop warning before reset")
	}

	ticker.ResetForNewPrompt()

	if len(ticker.entries) != 0 {
		t.Errorf("entries after reset: got %d, want 0", len(ticker.entries))
	}
	if ticker.loopWarning != "" {
		t.Errorf("loop warning after reset: got %q, want empty", ticker.loopWarning)
	}
	if len(ticker.loopCounts) != 0 {
		t.Errorf("loop counts after reset: got %d entries, want 0", len(ticker.loopCounts))
	}
}

func TestTickerAgentColorAssignment(t *testing.T) {
	ticker := NewTicker(3)

	// Push entries from 3 different agents.
	agents := []string{"agent-1", "agent-2", "agent-3"}
	for _, id := range agents {
		ticker.Push([]TickerEntry{{
			ToolType:  model.ToolRead,
			Path:      "file.go",
			AgentID:   id,
			Timestamp: time.Now(),
		}})
	}

	// Each agent should get a distinct color.
	colors := make(map[string]bool)
	for _, id := range agents {
		c := ticker.AgentColor(id)
		colors[string(c)] = true
	}
	if len(colors) != 3 {
		t.Errorf("distinct colors: got %d, want 3", len(colors))
	}

	// Repeated calls for the same agent should return the same color.
	c1 := ticker.AgentColor("agent-1")
	c2 := ticker.AgentColor("agent-1")
	if c1 != c2 {
		t.Errorf("color inconsistency for agent-1: got %v then %v", c1, c2)
	}
}

func TestTickerAgentColorsPreservedAcrossReset(t *testing.T) {
	ticker := NewTicker(3)

	ticker.Push([]TickerEntry{{
		ToolType:  model.ToolRead,
		Path:      "file.go",
		AgentID:   "agent-1",
		Timestamp: time.Now(),
	}})
	colorBefore := ticker.AgentColor("agent-1")

	ticker.ResetForNewPrompt()
	colorAfter := ticker.AgentColor("agent-1")

	if colorBefore != colorAfter {
		t.Errorf("agent color changed after reset: %v → %v", colorBefore, colorAfter)
	}
}

func TestTickerQuietState(t *testing.T) {
	ticker := NewTicker(3)

	now := time.Now()

	// No entries yet — not quiet (nothing to be quiet about).
	if ticker.IsQuiet(now) {
		t.Error("should not be quiet with no entries")
	}

	// Push entry with old timestamp.
	ticker.Push([]TickerEntry{{
		ToolType:  model.ToolRead,
		Path:      "file.go",
		Timestamp: now.Add(-15 * time.Second),
	}})

	// 15 seconds ago → should be quiet.
	if !ticker.IsQuiet(now) {
		t.Error("should be quiet 15s after last activity")
	}

	// Push a recent entry.
	ticker.Push([]TickerEntry{{
		ToolType:  model.ToolWrite,
		Path:      "file.go",
		Timestamp: now.Add(-2 * time.Second),
	}})

	// 2 seconds ago → should NOT be quiet.
	if ticker.IsQuiet(now) {
		t.Error("should not be quiet 2s after last activity")
	}
}

func TestTickerHasEntries(t *testing.T) {
	ticker := NewTicker(3)

	if ticker.HasEntries() {
		t.Error("new ticker should not HasEntries")
	}

	ticker.Push([]TickerEntry{{
		ToolType:  model.ToolRead,
		Path:      "file.go",
		Timestamp: time.Now(),
	}})

	if !ticker.HasEntries() {
		t.Error("ticker with entries should HasEntries")
	}
}

func TestTickerViewNormal(t *testing.T) {
	ticker := NewTicker(3)

	ticker.Push([]TickerEntry{
		{ToolType: model.ToolRead, Path: "foo.go", Timestamp: time.Now()},
		{ToolType: model.ToolWrite, Path: "bar.go", Timestamp: time.Now()},
	})

	view := ticker.View(80, time.Now())
	if view == "" {
		t.Error("expected non-empty view")
	}
}

func TestTickerViewLoopWarning(t *testing.T) {
	ticker := NewTicker(3)

	for i := 0; i < 3; i++ {
		ticker.Push([]TickerEntry{{
			ToolType:  model.ToolEdit,
			Path:      "config.go",
			Timestamp: time.Now(),
		}})
	}

	view := ticker.View(80, time.Now())
	// View should contain the loop symbol.
	if view == "" {
		t.Error("expected non-empty view for loop warning")
	}
}

func TestTickerViewQuietState(t *testing.T) {
	ticker := NewTicker(3)

	old := time.Now().Add(-30 * time.Second)
	ticker.Push([]TickerEntry{{
		ToolType:  model.ToolRead,
		Path:      "file.go",
		Timestamp: old,
	}})

	view := ticker.View(80, time.Now())
	if view == "" {
		t.Error("expected non-empty view for quiet state")
	}
}
