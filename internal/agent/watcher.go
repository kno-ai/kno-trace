// Package agent — watcher.go provides live tailing of subagent JSONL files.
//
// When a live session spawns agents, the AgentWatcher detects their subagent
// files appearing on disk and tails them in real time — extracting tool calls
// as they happen. This gives the UI live visibility into what each agent is
// doing, even agents that emit zero progress lines in the parent session.
//
// Lifecycle:
//  1. WatchAgent(agentID, toolUseID) — called when Agent tool_use appears in parent
//  2. AgentWatcher watches the subagents/ directory for file creation
//  3. Once agent-a<agentID>.jsonl appears, a per-agent tailer goroutine starts
//  4. Tailer emits MsgAgentToolCall for each tool_use found
//  5. StopAgent(toolUseID) — called when Agent tool_result appears in parent
//  6. Tailer does a final read, then exits
package agent

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"

	"github.com/kno-ai/kno-trace/internal/model"
	"github.com/kno-ai/kno-trace/internal/parser"
)

// MsgAgentToolCall is emitted when a subagent file tailer detects a new tool call.
// The UI appends this to the agent's ToolCalls and re-renders.
type MsgAgentToolCall struct {
	AgentID    string
	ToolUseID  string // the parent tool_use ID that spawned this agent
	ToolCall   *model.ToolCall
}

// MsgAgentFileFound is emitted when a subagent's JSONL file appears on disk.
type MsgAgentFileFound struct {
	AgentID   string
	ToolUseID string
}

// MsgAgentFileMissing is emitted when a subagent's file was never found
// before the agent completed.
type MsgAgentFileMissing struct {
	AgentID   string
	ToolUseID string
}

// AgentWatcher manages per-agent tailer goroutines for a live session.
// It watches the subagents/ directory and starts/stops tailers as agents
// are spawned and completed.
type AgentWatcher struct {
	sessionDir string
	sessionID  string
	send       func(interface{}) // sends messages to the UI (tea.Msg)

	mu      sync.Mutex
	agents  map[string]*agentTailer // keyed by toolUseID
	dirWatcher *fsnotify.Watcher
	stop    chan struct{}
	stopped bool
}

// agentTailer tracks the state of a single subagent file tail.
type agentTailer struct {
	agentID   string
	toolUseID string
	filePath  string
	stop      chan struct{}
	found     bool // true once the file was found and tailing started
}

// NewAgentWatcher creates a watcher for the subagents directory of a live session.
func NewAgentWatcher(sessionDir, sessionID string, send func(interface{})) *AgentWatcher {
	return &AgentWatcher{
		sessionDir: sessionDir,
		sessionID:  sessionID,
		send:       send,
		agents:     make(map[string]*agentTailer),
		stop:       make(chan struct{}),
	}
}

// Start begins watching the subagents directory for new files.
// Must be called before WatchAgent.
func (w *AgentWatcher) Start() error {
	subDir := SubagentsDir(w.sessionDir, w.sessionID)

	dirWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("agent watcher: fsnotify: %w", err)
	}
	w.dirWatcher = dirWatcher

	// Watch the subagents directory if it exists. Claude Code creates it
	// when the first agent spawns. We never create directories ourselves.
	if _, statErr := os.Stat(subDir); statErr == nil {
		if err := dirWatcher.Add(subDir); err != nil {
			dirWatcher.Close()
			w.dirWatcher = nil
		}
	} else {
		// Directory doesn't exist yet — watch the parent (session) directory
		// so we can detect when subagents/ is created.
		parentDir := filepath.Join(w.sessionDir, w.sessionID)
		if err := dirWatcher.Add(parentDir); err != nil {
			dirWatcher.Close()
			w.dirWatcher = nil
		}
	}

	go w.runDirWatcher()
	return nil
}

// WatchAgent begins watching for a specific agent's subagent JSONL file.
// Called when an Agent tool_use event appears in the parent session.
func (w *AgentWatcher) WatchAgent(agentID, toolUseID string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.stopped {
		return
	}

	// Already watching this agent.
	if _, ok := w.agents[toolUseID]; ok {
		return
	}

	filePath := SubagentFilePath(w.sessionDir, w.sessionID, agentID)
	at := &agentTailer{
		agentID:   agentID,
		toolUseID: toolUseID,
		filePath:  filePath,
		stop:      make(chan struct{}),
	}
	w.agents[toolUseID] = at

	// Check if file already exists (agent may have been fast).
	if _, err := os.Stat(filePath); err == nil {
		at.found = true
		w.send(MsgAgentFileFound{AgentID: agentID, ToolUseID: toolUseID})
		go w.tailFile(at)
	}
	// Otherwise, the directory watcher will detect the file when it appears.
}

// StopAgent stops tailing a specific agent. Called when the Agent tool_result
// arrives in the parent session. Does a final read to catch any trailing lines.
func (w *AgentWatcher) StopAgent(toolUseID string) {
	w.mu.Lock()
	at, ok := w.agents[toolUseID]
	if !ok {
		w.mu.Unlock()
		return
	}
	delete(w.agents, toolUseID)
	w.mu.Unlock()

	// Signal the tailer to stop.
	close(at.stop)

	if !at.found {
		w.send(MsgAgentFileMissing{AgentID: at.agentID, ToolUseID: at.toolUseID})
	}
}

// Stop shuts down all agent tailers and the directory watcher.
func (w *AgentWatcher) Stop() {
	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		return
	}
	w.stopped = true

	// Copy agents to stop outside lock.
	agents := make([]*agentTailer, 0, len(w.agents))
	for _, at := range w.agents {
		agents = append(agents, at)
	}
	w.agents = make(map[string]*agentTailer)
	w.mu.Unlock()

	// Stop all active tailers.
	for _, at := range agents {
		close(at.stop)
	}

	// Signal the directory watcher to stop.
	close(w.stop)
	if w.dirWatcher != nil {
		w.dirWatcher.Close()
	}
}

// runDirWatcher watches the subagents directory for new file creation
// and starts tailers for pending agents whose files appear.
// If the subagents/ directory didn't exist at Start() time, it watches the
// parent directory and switches to the subagents/ directory when it appears.
func (w *AgentWatcher) runDirWatcher() {
	if w.dirWatcher == nil {
		return
	}

	subDir := SubagentsDir(w.sessionDir, w.sessionID)
	watchingSubDir := false
	// Check if we're already watching the subagents dir.
	if _, err := os.Stat(subDir); err == nil {
		watchingSubDir = true
	}

	for {
		select {
		case <-w.stop:
			return
		case event, ok := <-w.dirWatcher.Events:
			if !ok {
				return
			}

			// If we're watching the parent dir, look for subagents/ creation.
			if !watchingSubDir && event.Has(fsnotify.Create) {
				if filepath.Base(event.Name) == "subagents" {
					if err := w.dirWatcher.Add(subDir); err == nil {
						watchingSubDir = true
					}
					continue
				}
			}

			if !event.Has(fsnotify.Create) && !event.Has(fsnotify.Write) {
				continue
			}

			filename := filepath.Base(event.Name)
			if !strings.HasSuffix(filename, ".jsonl") || !strings.HasPrefix(filename, "agent-") {
				continue
			}

			// Extract agentID from filename: agent-<agentID>.jsonl
			agentID := strings.TrimPrefix(filename, "agent-")
			agentID = strings.TrimSuffix(agentID, ".jsonl")

			w.mu.Lock()
			var matched *agentTailer
			for _, at := range w.agents {
				if at.agentID == agentID && !at.found {
					at.found = true
					matched = at
					break
				}
			}
			w.mu.Unlock()

			// Send outside the lock to avoid blocking while holding mu.
			if matched != nil {
				w.send(MsgAgentFileFound{AgentID: matched.agentID, ToolUseID: matched.toolUseID})
				go w.tailFile(matched)
			}

		case _, ok := <-w.dirWatcher.Errors:
			if !ok {
				return
			}
		}
	}
}

// tailFile tails a single subagent JSONL file, emitting MsgAgentToolCall
// for each tool_use block found. Stops when at.stop is closed.
func (w *AgentWatcher) tailFile(at *agentTailer) {
	f, err := os.Open(at.filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kno-trace: agent %s: cannot open file: %v\n", at.agentID, err)
		return
	}
	defer f.Close()

	// Read existing content first.
	reader := bufio.NewReader(f)
	w.processExistingLines(at, reader)

	// Get current position for live tail.
	offset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return
	}

	// Watch the file for new writes.
	fileWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer fileWatcher.Close()

	if err := fileWatcher.Add(at.filePath); err != nil {
		return
	}

	buf := make([]byte, 1024*1024) // 1MB buffer for subagent lines
	var partial string

	for {
		select {
		case <-at.stop:
			// Final read to catch any trailing lines.
			w.readNewBytes(at, f, buf, &offset, &partial)
			return
		case event, ok := <-fileWatcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
				return
			}
			if !event.Has(fsnotify.Write) {
				continue
			}
			w.readNewBytes(at, f, buf, &offset, &partial)

		case _, ok := <-fileWatcher.Errors:
			if !ok {
				return
			}
		}
	}
}

// processExistingLines reads and processes all lines currently in the file.
func (w *AgentWatcher) processExistingLines(at *agentTailer, reader *bufio.Reader) {
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			w.processLine(at, line)
		}
		if err != nil {
			break
		}
	}
}

// readNewBytes reads newly appended bytes from the file and processes complete lines.
func (w *AgentWatcher) readNewBytes(at *agentTailer, f *os.File, buf []byte, offset *int64, partial *string) {
	n, _ := f.ReadAt(buf, *offset)
	if n == 0 {
		return
	}
	*offset += int64(n)

	data := *partial + string(buf[:n])
	*partial = ""

	lines := strings.Split(data, "\n")
	if lines[len(lines)-1] != "" {
		*partial = lines[len(lines)-1]
		lines = lines[:len(lines)-1]
	}

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		w.processLine(at, []byte(line))
	}
}

// processLine parses a single JSONL line and emits MsgAgentToolCall for any tool_use blocks.
func (w *AgentWatcher) processLine(at *agentTailer, line []byte) {
	evts, err := parser.ParseReader(strings.NewReader(string(line)))
	if err != nil || len(evts) == 0 {
		return
	}

	var agentCounter int
	for _, evt := range evts {
		if evt.Type != "assistant" || evt.Message == nil {
			continue
		}
		for _, block := range evt.Message.Content {
			if block.Type != "tool_use" {
				continue
			}
			tc := buildAgentToolCall(block, evt.Timestamp, at.agentID, &agentCounter)
			w.send(MsgAgentToolCall{
				AgentID:   at.agentID,
				ToolUseID: at.toolUseID,
				ToolCall:  tc,
			})
		}
	}
}
