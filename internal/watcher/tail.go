// Package watcher provides live file tailing for JSONL session files.
// It replays existing content on startup, then streams new lines as they
// are appended. Communication is via tea.Msg events — never access model
// types directly from the watcher goroutine.
package watcher

import (
	"bufio"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"

	"github.com/kno-ai/kno-trace/internal/config"
	"github.com/kno-ai/kno-trace/internal/parser"
)

// Bubbletea messages emitted by the watcher.

// MsgPromptSealed is sent when a prompt boundary is detected (new human turn
// seals the previous prompt). PromptIdx is the index of the sealed prompt.
type MsgPromptSealed struct{ PromptIdx int }

// MsgPromptUpdate is sent when new data arrives for the current (unsealed) prompt.
type MsgPromptUpdate struct{ PromptIdx int }

// MsgReplayDone is sent after all existing lines have been replayed.
type MsgReplayDone struct{}

// MsgSessionFileDeleted is sent if the session file is removed.
type MsgSessionFileDeleted struct{}

// MsgNewEvents carries parsed events from new lines for the UI to process.
type MsgNewEvents struct{ Events []*parser.RawEvent }

// Tailer watches a JSONL file and emits events as lines are appended.
type Tailer struct {
	path string
	cfg  *config.Config
	send func(tea.Msg)
}

// New creates a Tailer for the given session file path.
func New(path string, cfg *config.Config, send func(tea.Msg)) *Tailer {
	return &Tailer{
		path: path,
		cfg:  cfg,
		send: send,
	}
}

// Start begins tailing in a goroutine. It replays existing content first,
// emitting events incrementally, then watches for new writes.
// Returns a function to stop the watcher.
func (t *Tailer) Start() func() {
	stop := make(chan struct{})
	go t.run(stop)
	return func() { close(stop) }
}

// isHumanTurnEvent checks if a parsed event is a human turn (prompt boundary).
// Delegates to parser.IsHumanTurn for the core logic, with an additional type guard
// since the watcher sees all event types (not just "user").
func isHumanTurnEvent(evt *parser.RawEvent) bool {
	return evt.Type == "user" && parser.IsHumanTurn(evt)
}

func (t *Tailer) run(stop <-chan struct{}) {
	f, err := os.Open(t.path)
	if err != nil {
		return
	}
	defer f.Close()

	// Replay existing content.
	reader := bufio.NewReader(f)

	var (
		allReplayEvents []*parser.RawEvent
		humanTurnCount  int
	)

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 && len(strings.TrimSpace(string(line))) > 0 {
			evts, parseErr := parser.ParseReader(strings.NewReader(string(line)))
			if parseErr == nil {
				for _, evt := range evts {
					if isHumanTurnEvent(evt) {
						if humanTurnCount > 0 {
							t.send(MsgPromptSealed{PromptIdx: humanTurnCount - 1})
						}
						humanTurnCount++
					}
					allReplayEvents = append(allReplayEvents, evt)
				}
			}
		}
		if err != nil {
			break
		}
	}

	// Send all accumulated events for initial build.
	if len(allReplayEvents) > 0 {
		t.send(MsgNewEvents{Events: allReplayEvents})
	}
	t.send(MsgReplayDone{})

	// Free replay events — watcher only needs the human turn count going forward.
	allReplayEvents = nil

	// Get current file position for live tail.
	offset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return
	}

	// Set up fsnotify watcher.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer watcher.Close()

	if err := watcher.Add(t.path); err != nil {
		return
	}

	// Live tail loop.
	buf := make([]byte, 8*1024*1024)
	var partial string

	for {
		select {
		case <-stop:
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
				t.send(MsgSessionFileDeleted{})
				return
			}
			if !event.Has(fsnotify.Write) {
				continue
			}

			// Read new bytes from the file.
			n, _ := f.ReadAt(buf, offset)
			if n == 0 {
				continue
			}
			offset += int64(n)

			data := partial + string(buf[:n])
			partial = ""

			lines := strings.Split(data, "\n")

			// If the last element isn't empty, the final line is incomplete.
			if lines[len(lines)-1] != "" {
				partial = lines[len(lines)-1]
				lines = lines[:len(lines)-1]
			}

			var newEvents []*parser.RawEvent
			for _, line := range lines {
				if strings.TrimSpace(line) == "" {
					continue
				}
				evts, err := parser.ParseReader(strings.NewReader(line))
				if err != nil || len(evts) == 0 {
					continue
				}
				newEvents = append(newEvents, evts...)
			}

			if len(newEvents) > 0 {
				for _, evt := range newEvents {
					if isHumanTurnEvent(evt) {
						if humanTurnCount > 0 {
							t.send(MsgPromptSealed{PromptIdx: humanTurnCount - 1})
						}
						humanTurnCount++
					}
				}
				t.send(MsgNewEvents{Events: newEvents})
				t.send(MsgPromptUpdate{PromptIdx: max(0, humanTurnCount-1)})
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			_ = err
		}
	}
}
