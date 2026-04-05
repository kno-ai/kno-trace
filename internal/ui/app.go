package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kno-ai/kno-trace/internal/config"
	"github.com/kno-ai/kno-trace/internal/discovery"
	"github.com/kno-ai/kno-trace/internal/model"
	"github.com/kno-ai/kno-trace/internal/parser"
	"github.com/kno-ai/kno-trace/internal/watcher"
)

// viewMode tracks which screen is currently displayed.
type viewMode int

const (
	viewPicker   viewMode = iota
	viewLoading           // session selected, watcher replaying
	viewTimeline          // full timeline view
)

// App is the root Bubbletea model for kno-trace.
type App struct {
	view        viewMode
	picker      pickerModel
	timeline    timelineModel
	sessionMeta *model.SessionMeta // selected session metadata
	session     *model.Session     // fully parsed session (built incrementally)
	events      []*parser.RawEvent // accumulated events from watcher
	statusMsg   string
	allSessions []*model.SessionMeta
	cfg         *config.Config
	stopWatcher func()
	width       int
	height      int
}

// NewApp creates the root model starting at the session picker.
func NewApp(sessions []*model.SessionMeta, cfg *config.Config) App {
	return App{
		view:        viewPicker,
		picker:      newPicker(sessions),
		allSessions: sessions,
		cfg:         cfg,
	}
}

// NewAppWithSession creates the root model auto-opened to a session.
// Starts loading immediately — the watcher will be launched in Init().
func NewAppWithSession(sessions []*model.SessionMeta, session *model.SessionMeta, statusMsg string, cfg *config.Config) App {
	return App{
		view:        viewLoading,
		picker:      newPicker(sessions),
		sessionMeta: session,
		statusMsg:   statusMsg,
		allSessions: sessions,
		cfg:         cfg,
	}
}

func (a App) Init() tea.Cmd {
	if a.view == viewLoading && a.sessionMeta != nil {
		return a.startWatcher(a.sessionMeta.FilePath)
	}
	return nil
}

// startWatcher launches the watcher goroutine and returns a Cmd that sends its first message.
func (a App) startWatcher(path string) tea.Cmd {
	return func() tea.Msg {
		ch := make(chan tea.Msg, 256)
		done := make(chan struct{})
		tailer := watcher.New(path, a.cfg, func(msg tea.Msg) {
			select {
			case ch <- msg:
			case <-done:
				// Watcher was stopped — discard the message.
			}
		})
		stopTailer := tailer.Start()

		// Wrap the stop function: signal done first (unblocks send),
		// then stop the tailer, then close ch (unblocks waitForWatcher).
		stop := func() {
			close(done)
			stopTailer()
			close(ch)
		}

		return msgWatcherStarted{ch: ch, stop: stop}
	}
}

// Internal messages for watcher integration.
type msgWatcherStarted struct {
	ch   <-chan tea.Msg
	stop func()
}

// waitForWatcher returns a Cmd that waits for the next message from the watcher channel.
func waitForWatcher(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msgWatcherEvent{inner: msg, ch: ch}
	}
}

type msgWatcherEvent struct {
	inner tea.Msg
	ch    <-chan tea.Msg
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.picker.width = msg.Width
		a.picker.height = msg.Height
		a.timeline.setSize(msg.Width, msg.Height)
		return a, nil

	case msgWatcherStarted:
		a.stopWatcher = msg.stop
		return a, waitForWatcher(msg.ch)

	case msgWatcherEvent:
		cmd := a.handleWatcherMsg(msg.inner)
		// Keep listening for more watcher messages.
		return a, tea.Batch(cmd, waitForWatcher(msg.ch))

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			a.cleanup()
			return a, tea.Quit
		}

		switch a.view {
		case viewPicker:
			return a.updatePicker(msg)
		case viewLoading:
			return a.updateLoading(msg)
		case viewTimeline:
			return a.updateTimeline(msg)
		}
	}

	return a, nil
}

// handleWatcherMsg processes a message from the watcher goroutine.
func (a *App) handleWatcherMsg(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case watcher.MsgNewEvents:
		a.events = append(a.events, msg.Events...)
		// Don't rebuild session here — just accumulate events.
		// Full build happens on MsgReplayDone.
	case watcher.MsgReplayDone:
		// Replay complete — stop the watcher (option 2: M4 will remove this).
		a.cleanup()
		a.rebuildSession()
		if a.session != nil {
			a.timeline = newTimeline(a.session)
			a.timeline.setSize(a.width, a.height)
		}
		a.view = viewTimeline
	case watcher.MsgPromptSealed:
		// Don't rebuild the full session on every sealed prompt (O(n²)).
		// The loading screen uses event count as a progress indicator.
	}
	return nil
}

func (a *App) rebuildSession() {
	if len(a.events) == 0 {
		return
	}
	a.session = parser.BuildSession(a.events, a.cfg)
	// Propagate project info from meta if available.
	if a.sessionMeta != nil {
		if a.session.ProjectName == "" {
			a.session.ProjectName = a.sessionMeta.ProjectName
		}
		if a.session.ProjectPath == "" {
			a.session.ProjectPath = a.sessionMeta.ProjectPath
		}
		a.session.FilePath = a.sessionMeta.FilePath
	}
}

func (a App) updatePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.picker.filtering {
		var cmd tea.Cmd
		a.picker, cmd = a.picker.Update(msg)
		return a, cmd
	}

	switch msg.String() {
	case "enter":
		sel := a.picker.selectedSession()
		if sel != nil {
			a.resetSession()
			a.sessionMeta = sel
			a.view = viewLoading
			a.statusMsg = ""
			return a, a.startWatcher(sel.FilePath)
		}
		return a, nil
	case "q":
		a.cleanup()
		return a, tea.Quit
	}

	var cmd tea.Cmd
	a.picker, cmd = a.picker.Update(msg)
	return a, cmd
}

func (a App) updateLoading(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		a.cleanup()
		return a, tea.Quit
	case "P", "esc":
		a.resetSession()
		a.ensurePickerLoaded()
		a.view = viewPicker
		return a, nil
	}
	return a, nil
}

func (a App) updateTimeline(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// If the timeline is filtering, let it handle all keys.
	if a.timeline.filtering {
		var cmd tea.Cmd
		a.timeline, cmd = a.timeline.Update(msg)
		return a, cmd
	}

	switch msg.String() {
	case "q":
		a.cleanup()
		return a, tea.Quit
	case "P":
		a.resetSession()
		a.ensurePickerLoaded()
		a.view = viewPicker
		a.statusMsg = ""
		return a, nil
	case "esc":
		// If filter is active (but not filtering), clear it.
		if a.timeline.filter != "" {
			a.timeline.filter = ""
			a.timeline.filteredIdxs = nil
			a.timeline.restoreFullList()
			return a, nil
		}
		a.resetSession()
		a.ensurePickerLoaded()
		a.view = viewPicker
		a.statusMsg = ""
		return a, nil
	}

	var cmd tea.Cmd
	a.timeline, cmd = a.timeline.Update(msg)
	return a, cmd
}

// cleanup stops the watcher if running. Safe to call multiple times.
func (a *App) cleanup() {
	if a.stopWatcher != nil {
		a.stopWatcher()
		a.stopWatcher = nil
	}
}

// resetSession clears all session state for navigating to a new session.
func (a *App) resetSession() {
	a.cleanup()
	a.events = nil
	a.session = nil
}

// ensurePickerLoaded populates the picker with sessions if it's empty.
func (a *App) ensurePickerLoaded() {
	if len(a.picker.sessions) > 0 {
		return
	}
	sessions, _ := discovery.ScanAll()
	if a.cfg != nil && len(sessions) > a.cfg.MaxPickerSessions {
		sessions = sessions[:a.cfg.MaxPickerSessions]
	}
	if len(sessions) > 0 {
		a.allSessions = sessions
		a.picker = newPicker(sessions)
		a.picker.width = a.width
		a.picker.height = a.height
	}
}

func (a App) View() string {
	switch a.view {
	case viewPicker:
		return a.picker.View()
	case viewLoading:
		return a.viewLoading()
	case viewTimeline:
		return a.timeline.View()
	default:
		return ""
	}
}

func (a App) viewLoading() string {
	var b strings.Builder
	b.WriteString(TitleStyle.Render("kno-trace"))
	b.WriteString("\n\n")

	name := "session"
	if a.sessionMeta != nil {
		name = a.sessionMeta.ProjectName
	}

	b.WriteString(fmt.Sprintf("  Loading %s... %d events", name, len(a.events)))
	b.WriteString("\n\n")

	if a.statusMsg != "" {
		b.WriteString(MutedStyle.Render("  " + a.statusMsg))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(StatusBarStyle.Render(
		KeyStyle.Render("P") + " " + KeyDescStyle.Render("picker") + "  " +
			KeyStyle.Render("q") + " " + KeyDescStyle.Render("quit")))

	return b.String()
}
