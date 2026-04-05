package agent

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// collectMessages collects messages from an agent watcher into a slice.
// Returns the slice and a function to retrieve collected messages.
func collectMessages(t *testing.T) (func(interface{}), func() []interface{}) {
	t.Helper()
	var mu sync.Mutex
	var msgs []interface{}
	send := func(msg interface{}) {
		mu.Lock()
		msgs = append(msgs, msg)
		mu.Unlock()
	}
	get := func() []interface{} {
		mu.Lock()
		defer mu.Unlock()
		cp := make([]interface{}, len(msgs))
		copy(cp, msgs)
		return cp
	}
	return send, get
}

// waitFor polls until condition is true or timeout. Returns false on timeout.
func waitFor(t *testing.T, timeout time.Duration, condition func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func TestAgentWatcher_FileAlreadyExists(t *testing.T) {
	// Set up a temp directory mimicking the subagents/ structure.
	tmpDir := t.TempDir()
	sessionID := "test-session"
	subDir := filepath.Join(tmpDir, sessionID, "subagents")
	os.MkdirAll(subDir, 0755)

	// Pre-create the subagent file with a tool_use.
	agentFile := filepath.Join(subDir, "agent-test123.jsonl")
	content := `{"type":"assistant","message":{"model":"claude-haiku-4-5-20251001","role":"assistant","content":[{"type":"tool_use","id":"tc-1","name":"Read","input":{"file_path":"foo.go"}}],"stop_reason":"tool_use","usage":{"input_tokens":100,"output_tokens":20}},"timestamp":"2026-01-01T00:00:01Z","uuid":"u1","sessionId":"test-session"}` + "\n"
	os.WriteFile(agentFile, []byte(content), 0644)

	send, get := collectMessages(t)
	w := NewAgentWatcher(tmpDir, sessionID, send)
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	// File already exists — should be found immediately.
	w.WatchAgent("test123", "toolu-parent-1")

	ok := waitFor(t, 2*time.Second, func() bool {
		msgs := get()
		for _, m := range msgs {
			if _, ok := m.(MsgAgentToolCall); ok {
				return true
			}
		}
		return false
	})
	if !ok {
		t.Error("expected MsgAgentToolCall from pre-existing file")
	}

	// Verify the file-found message was sent.
	msgs := get()
	foundFileMsg := false
	for _, m := range msgs {
		if ff, ok := m.(MsgAgentFileFound); ok && ff.AgentID == "test123" {
			foundFileMsg = true
		}
	}
	if !foundFileMsg {
		t.Error("expected MsgAgentFileFound")
	}
}

func TestAgentWatcher_FileAppearsLater(t *testing.T) {
	tmpDir := t.TempDir()
	sessionID := "test-session"
	subDir := filepath.Join(tmpDir, sessionID, "subagents")
	os.MkdirAll(subDir, 0755)

	send, get := collectMessages(t)
	w := NewAgentWatcher(tmpDir, sessionID, send)
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	// Start watching before file exists.
	w.WatchAgent("late456", "toolu-parent-2")

	// Wait a moment then create the file.
	time.Sleep(50 * time.Millisecond)
	agentFile := filepath.Join(subDir, "agent-late456.jsonl")
	content := `{"type":"assistant","message":{"model":"claude-haiku-4-5-20251001","role":"assistant","content":[{"type":"tool_use","id":"tc-2","name":"Glob","input":{"pattern":"**/*.go"}}],"stop_reason":"tool_use","usage":{"input_tokens":100,"output_tokens":10}},"timestamp":"2026-01-01T00:00:02Z","uuid":"u2","sessionId":"test-session"}` + "\n"
	os.WriteFile(agentFile, []byte(content), 0644)

	ok := waitFor(t, 3*time.Second, func() bool {
		for _, m := range get() {
			if _, ok := m.(MsgAgentToolCall); ok {
				return true
			}
		}
		return false
	})
	if !ok {
		t.Error("expected MsgAgentToolCall after file appeared")
	}
}

func TestAgentWatcher_LiveAppend(t *testing.T) {
	tmpDir := t.TempDir()
	sessionID := "test-session"
	subDir := filepath.Join(tmpDir, sessionID, "subagents")
	os.MkdirAll(subDir, 0755)

	// Create file with initial content.
	agentFile := filepath.Join(subDir, "agent-append789.jsonl")
	line1 := `{"type":"assistant","message":{"model":"claude-haiku-4-5-20251001","role":"assistant","content":[{"type":"tool_use","id":"tc-a","name":"Read","input":{"file_path":"a.go"}}],"stop_reason":"tool_use","usage":{"input_tokens":100,"output_tokens":10}},"timestamp":"2026-01-01T00:00:01Z","uuid":"u1","sessionId":"test-session"}` + "\n"
	os.WriteFile(agentFile, []byte(line1), 0644)

	send, get := collectMessages(t)
	w := NewAgentWatcher(tmpDir, sessionID, send)
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	w.WatchAgent("append789", "toolu-parent-3")

	// Wait for initial tool call to be processed.
	waitFor(t, 2*time.Second, func() bool {
		for _, m := range get() {
			if tc, ok := m.(MsgAgentToolCall); ok && tc.ToolCall.ID == "tc-a" {
				return true
			}
		}
		return false
	})

	// Append a second tool call.
	line2 := `{"type":"assistant","message":{"model":"claude-haiku-4-5-20251001","role":"assistant","content":[{"type":"tool_use","id":"tc-b","name":"Edit","input":{"file_path":"b.go","old_string":"x","new_string":"y"}}],"stop_reason":"tool_use","usage":{"input_tokens":200,"output_tokens":20}},"timestamp":"2026-01-01T00:00:05Z","uuid":"u2","sessionId":"test-session"}` + "\n"
	f, _ := os.OpenFile(agentFile, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString(line2)
	f.Close()

	ok := waitFor(t, 3*time.Second, func() bool {
		for _, m := range get() {
			if tc, ok := m.(MsgAgentToolCall); ok && tc.ToolCall.ID == "tc-b" {
				return true
			}
		}
		return false
	})
	if !ok {
		t.Error("expected MsgAgentToolCall for appended line")
	}
}

func TestAgentWatcher_StopAgent(t *testing.T) {
	tmpDir := t.TempDir()
	sessionID := "test-session"
	subDir := filepath.Join(tmpDir, sessionID, "subagents")
	os.MkdirAll(subDir, 0755)

	send, _ := collectMessages(t)
	w := NewAgentWatcher(tmpDir, sessionID, send)
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	// Watch an agent whose file doesn't exist.
	w.WatchAgent("missing999", "toolu-parent-4")

	// Stop it — should get MsgAgentFileMissing since file never appeared.
	w.StopAgent("toolu-parent-4")

	// Should not panic or deadlock.
}

func TestAgentWatcher_StopAll(t *testing.T) {
	tmpDir := t.TempDir()
	sessionID := "test-session"
	subDir := filepath.Join(tmpDir, sessionID, "subagents")
	os.MkdirAll(subDir, 0755)

	send, _ := collectMessages(t)
	w := NewAgentWatcher(tmpDir, sessionID, send)
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	w.WatchAgent("a1", "t1")
	w.WatchAgent("a2", "t2")

	// Stop should not panic or deadlock.
	w.Stop()

	// Double stop should be safe.
	w.Stop()
}

func TestAgentWatcher_StopAgentEmitsFileMissing(t *testing.T) {
	tmpDir := t.TempDir()
	sessionID := "test-session"
	subDir := filepath.Join(tmpDir, sessionID, "subagents")
	os.MkdirAll(subDir, 0755)

	send, get := collectMessages(t)
	w := NewAgentWatcher(tmpDir, sessionID, send)
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	w.WatchAgent("neverappears", "toolu-parent-x")
	w.StopAgent("toolu-parent-x")

	// Give send a moment to process.
	time.Sleep(20 * time.Millisecond)

	found := false
	for _, m := range get() {
		if fm, ok := m.(MsgAgentFileMissing); ok && fm.AgentID == "neverappears" {
			found = true
		}
	}
	if !found {
		t.Error("expected MsgAgentFileMissing when file never appeared")
	}
}

func TestAgentWatcher_FinalReadOnStop(t *testing.T) {
	tmpDir := t.TempDir()
	sessionID := "test-session"
	subDir := filepath.Join(tmpDir, sessionID, "subagents")
	os.MkdirAll(subDir, 0755)

	agentFile := filepath.Join(subDir, "agent-finalread.jsonl")
	line1 := `{"type":"assistant","message":{"model":"claude-haiku-4-5-20251001","role":"assistant","content":[{"type":"tool_use","id":"tc-first","name":"Read","input":{"file_path":"first.go"}}],"stop_reason":"tool_use","usage":{"input_tokens":100,"output_tokens":10}},"timestamp":"2026-01-01T00:00:01Z","uuid":"u1","sessionId":"test-session"}` + "\n"
	os.WriteFile(agentFile, []byte(line1), 0644)

	send, get := collectMessages(t)
	w := NewAgentWatcher(tmpDir, sessionID, send)
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	w.WatchAgent("finalread", "toolu-final")

	// Wait for initial tool call.
	waitFor(t, 2*time.Second, func() bool {
		for _, m := range get() {
			if tc, ok := m.(MsgAgentToolCall); ok && tc.ToolCall.ID == "tc-first" {
				return true
			}
		}
		return false
	})

	// Append a second line, then immediately stop.
	line2 := `{"type":"assistant","message":{"model":"claude-haiku-4-5-20251001","role":"assistant","content":[{"type":"tool_use","id":"tc-last","name":"Read","input":{"file_path":"last.go"}}],"stop_reason":"tool_use","usage":{"input_tokens":200,"output_tokens":20}},"timestamp":"2026-01-01T00:00:05Z","uuid":"u2","sessionId":"test-session"}` + "\n"
	f, _ := os.OpenFile(agentFile, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString(line2)
	f.Close()

	// Stop agent — the final read should catch the trailing line.
	w.StopAgent("toolu-final")

	// Give goroutine time to do its final read.
	time.Sleep(50 * time.Millisecond)

	found := false
	for _, m := range get() {
		if tc, ok := m.(MsgAgentToolCall); ok && tc.ToolCall.ID == "tc-last" {
			found = true
		}
	}
	if !found {
		t.Error("expected final read to catch tc-last on stop")
	}
}

func TestAgentWatcher_WatchAfterStop(t *testing.T) {
	tmpDir := t.TempDir()
	sessionID := "test-session"
	subDir := filepath.Join(tmpDir, sessionID, "subagents")
	os.MkdirAll(subDir, 0755)

	send, _ := collectMessages(t)
	w := NewAgentWatcher(tmpDir, sessionID, send)
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	w.Stop()

	// WatchAgent after Stop should not panic.
	w.WatchAgent("late", "toolu-late")
}

func TestAgentWatcher_MalformedLines(t *testing.T) {
	tmpDir := t.TempDir()
	sessionID := "test-session"
	subDir := filepath.Join(tmpDir, sessionID, "subagents")
	os.MkdirAll(subDir, 0755)

	// File with a mix of valid and malformed lines.
	agentFile := filepath.Join(subDir, "agent-malformed.jsonl")
	content := "not json at all\n" +
		`{"type":"assistant","message":{"model":"claude-haiku-4-5-20251001","role":"assistant","content":[{"type":"tool_use","id":"tc-ok","name":"Read","input":{"file_path":"ok.go"}}],"stop_reason":"tool_use","usage":{"input_tokens":100,"output_tokens":10}},"timestamp":"2026-01-01T00:00:01Z","uuid":"u1","sessionId":"test-session"}` + "\n" +
		"{truncated json\n"
	os.WriteFile(agentFile, []byte(content), 0644)

	send, get := collectMessages(t)
	w := NewAgentWatcher(tmpDir, sessionID, send)
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	w.WatchAgent("malformed", "toolu-mal")

	// Should get the valid tool call despite malformed lines.
	ok := waitFor(t, 2*time.Second, func() bool {
		for _, m := range get() {
			if tc, ok := m.(MsgAgentToolCall); ok && tc.ToolCall.ID == "tc-ok" {
				return true
			}
		}
		return false
	})
	if !ok {
		t.Error("expected MsgAgentToolCall for valid line despite malformed lines in file")
	}
}

func TestAgentWatcher_SubagentsDirCreatedLater(t *testing.T) {
	tmpDir := t.TempDir()
	sessionID := "test-session"
	// Create the session dir but NOT the subagents/ dir.
	sessionDir := filepath.Join(tmpDir, sessionID)
	os.MkdirAll(sessionDir, 0755)

	send, get := collectMessages(t)
	w := NewAgentWatcher(tmpDir, sessionID, send)
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	w.WatchAgent("delayed", "toolu-delayed")

	// Now create the subagents dir and the agent file.
	time.Sleep(50 * time.Millisecond)
	subDir := filepath.Join(sessionDir, "subagents")
	os.MkdirAll(subDir, 0755)

	time.Sleep(50 * time.Millisecond)
	agentFile := filepath.Join(subDir, "agent-delayed.jsonl")
	content := `{"type":"assistant","message":{"model":"claude-haiku-4-5-20251001","role":"assistant","content":[{"type":"tool_use","id":"tc-delayed","name":"Read","input":{"file_path":"delayed.go"}}],"stop_reason":"tool_use","usage":{"input_tokens":100,"output_tokens":10}},"timestamp":"2026-01-01T00:00:01Z","uuid":"u1","sessionId":"test-session"}` + "\n"
	os.WriteFile(agentFile, []byte(content), 0644)

	ok := waitFor(t, 3*time.Second, func() bool {
		for _, m := range get() {
			if tc, ok := m.(MsgAgentToolCall); ok && tc.ToolCall.ID == "tc-delayed" {
				return true
			}
		}
		return false
	})
	if !ok {
		t.Error("expected MsgAgentToolCall after subagents/ dir was created later")
	}
}

func TestAgentWatcher_MultipleConcurrentAgents(t *testing.T) {
	tmpDir := t.TempDir()
	sessionID := "test-session"
	subDir := filepath.Join(tmpDir, sessionID, "subagents")
	os.MkdirAll(subDir, 0755)

	send, get := collectMessages(t)
	w := NewAgentWatcher(tmpDir, sessionID, send)
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	// Create two agent files.
	for _, id := range []string{"multi1", "multi2"} {
		agentFile := filepath.Join(subDir, "agent-"+id+".jsonl")
		content := `{"type":"assistant","message":{"model":"claude-haiku-4-5-20251001","role":"assistant","content":[{"type":"tool_use","id":"tc-` + id + `","name":"Read","input":{"file_path":"` + id + `.go"}}],"stop_reason":"tool_use","usage":{"input_tokens":100,"output_tokens":10}},"timestamp":"2026-01-01T00:00:01Z","uuid":"u-` + id + `","sessionId":"test-session"}` + "\n"
		os.WriteFile(agentFile, []byte(content), 0644)
	}

	w.WatchAgent("multi1", "toolu-m1")
	w.WatchAgent("multi2", "toolu-m2")

	// Both agents should emit tool calls.
	ok := waitFor(t, 3*time.Second, func() bool {
		foundM1, foundM2 := false, false
		for _, m := range get() {
			if tc, ok := m.(MsgAgentToolCall); ok {
				if tc.AgentID == "multi1" {
					foundM1 = true
				}
				if tc.AgentID == "multi2" {
					foundM2 = true
				}
			}
		}
		return foundM1 && foundM2
	})
	if !ok {
		t.Error("expected MsgAgentToolCall from both concurrent agents")
	}
}
