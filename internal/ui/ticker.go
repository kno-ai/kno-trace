// Package ui — ticker.go implements the live activity ticker strip.
// The ticker shows tool calls of the active in-flight prompt as they arrive,
// with agent attribution (colored by agent), loop detection, and a quiet-state
// indicator when the session appears idle.
package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/kno-ai/kno-trace/internal/model"
)

// TickerEntry represents a single tool call event for the ticker display.
type TickerEntry struct {
	ToolType  model.ToolType
	Path      string    // file path, command, pattern — depends on tool type
	AgentID   string    // empty = parent session
	Timestamp time.Time
}

// loopKey tracks tool+path pairs for loop detection.
type loopKey struct {
	toolType model.ToolType
	path     string
}

// Ticker tracks live tool call activity for the currently active prompt.
type Ticker struct {
	entries      []TickerEntry
	agentColors  map[string]int // agentID → palette index
	nextColor    int
	loopCounts   map[loopKey]int
	loopWarning  string // active loop warning text; empty = no loop
	lastActivity time.Time
	threshold    int // loop detection threshold (from config)
	maxEntries   int
}

// NewTicker creates a ticker with the given loop detection threshold.
func NewTicker(threshold int) Ticker {
	return Ticker{
		agentColors: make(map[string]int),
		loopCounts:  make(map[loopKey]int),
		threshold:   threshold,
		maxEntries:  50,
	}
}

// Push appends new entries to the ticker, updating loop detection and trimming
// to the max entry cap. Safe to call on a zero-value Ticker.
func (t *Ticker) Push(entries []TickerEntry) {
	if len(entries) == 0 {
		return
	}
	// Defensive: initialize maps if ticker was zero-valued (e.g., session
	// started with 0 prompts and became live after events arrived).
	if t.agentColors == nil {
		t.agentColors = make(map[string]int)
	}
	if t.loopCounts == nil {
		t.loopCounts = make(map[loopKey]int)
	}
	if t.maxEntries == 0 {
		t.maxEntries = 50
	}
	if t.threshold <= 0 {
		t.threshold = 3 // safe default matching config.Default()
	}
	for _, e := range entries {
		t.entries = append(t.entries, e)
		if !e.Timestamp.IsZero() {
			t.lastActivity = e.Timestamp
		} else {
			t.lastActivity = time.Now()
		}

		// Assign agent color on first sight.
		if e.AgentID != "" {
			if _, ok := t.agentColors[e.AgentID]; !ok {
				t.agentColors[e.AgentID] = t.nextColor
				t.nextColor++
			}
		}

		// Loop detection: same tool+path pair repeated.
		if e.Path != "" {
			key := loopKey{e.ToolType, e.Path}
			t.loopCounts[key]++
			count := t.loopCounts[key]
			if count >= t.threshold {
				t.loopWarning = fmt.Sprintf("⟳ possible loop: %s %s repeated %d×",
					e.ToolType, e.Path, count)
			} else if t.loopWarning != "" {
				// A different tool+path broke the repetition — clear warning.
				t.loopWarning = ""
			}
		}
	}

	// Trim to max.
	if len(t.entries) > t.maxEntries {
		t.entries = t.entries[len(t.entries)-t.maxEntries:]
	}
}

// ResetForNewPrompt clears all state for a new prompt boundary.
// Agent color assignments are preserved across prompts.
func (t *Ticker) ResetForNewPrompt() {
	t.entries = nil
	t.loopCounts = make(map[loopKey]int)
	t.loopWarning = ""
	// lastActivity intentionally not reset — quiet state should reflect
	// actual time since last activity, not prompt boundary.
}

// AgentColor returns the palette color for the given agent ID.
// Assigns the next color if this agent hasn't been seen before.
func (t *Ticker) AgentColor(agentID string) lipgloss.Color {
	if t.agentColors == nil {
		t.agentColors = make(map[string]int)
	}
	idx, ok := t.agentColors[agentID]
	if !ok {
		idx = t.nextColor
		t.agentColors[agentID] = idx
		t.nextColor++
	}
	return AgentColors[idx%len(AgentColors)]
}

// IsQuiet returns true if more than 10 seconds have passed since the last
// tool call activity.
func (t *Ticker) IsQuiet(now time.Time) bool {
	if t.lastActivity.IsZero() {
		return false
	}
	return now.Sub(t.lastActivity) > 10*time.Second
}

// HasEntries returns true if the ticker has any entries or is in a displayable
// state (quiet or loop warning).
func (t *Ticker) HasEntries() bool {
	return len(t.entries) > 0 || t.loopWarning != "" || !t.lastActivity.IsZero()
}

// View renders the ticker as a single-line strip that fits within width.
func (t *Ticker) View(width int, now time.Time) string {
	if width <= 0 {
		width = 80
	}

	// Loop warning takes priority.
	if t.loopWarning != "" {
		return lipgloss.NewStyle().Foreground(ColorYellow).Render(
			Truncate(t.loopWarning, width))
	}

	// Quiet state.
	if t.IsQuiet(now) {
		elapsed := int(now.Sub(t.lastActivity).Seconds())
		msg := fmt.Sprintf("last activity %ds ago", elapsed)
		return DimStyle.Render(msg)
	}

	// Normal: render recent entries, newest on right.
	if len(t.entries) == 0 {
		return ""
	}

	sep := " · "
	var parts []string
	used := 0
	// Walk backwards from newest entry, measuring plain text width.
	for i := len(t.entries) - 1; i >= 0; i-- {
		plain := t.entryText(t.entries[i])
		needed := len(plain)
		if len(parts) > 0 {
			needed += len(sep)
		}
		if used+needed > width && len(parts) > 0 {
			break
		}
		parts = append(parts, t.renderEntry(t.entries[i], plain))
		used += needed
	}

	// Reverse so oldest is on left, newest on right.
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}

	return strings.Join(parts, DimStyle.Render(sep))
}

// entryText returns the unstyled text for a ticker entry (for width measurement).
func (t *Ticker) entryText(e TickerEntry) string {
	return toolIcon(e.ToolType) + " " + shortenPath(e.Path, e.ToolType)
}

// renderEntry styles a ticker entry. plain is the pre-computed unstyled text.
func (t *Ticker) renderEntry(e TickerEntry, plain string) string {
	if e.AgentID != "" {
		return lipgloss.NewStyle().Foreground(t.AgentColor(e.AgentID)).Render(plain)
	}
	return NormalStyle.Render(plain)
}

// shortenPath returns a display-friendly version of a tool call path.
func shortenPath(path string, toolType model.ToolType) string {
	if path == "" {
		return string(toolType)
	}
	if len(path) > 30 {
		if idx := strings.LastIndex(path, "/"); idx >= 0 {
			return path[idx+1:]
		}
	}
	return path
}
