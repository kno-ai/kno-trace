package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kno-ai/kno-trace/internal/agent"
	"github.com/kno-ai/kno-trace/internal/config"
	"github.com/kno-ai/kno-trace/internal/discovery"
	"github.com/kno-ai/kno-trace/internal/model"
	"github.com/kno-ai/kno-trace/internal/parser"
	"github.com/kno-ai/kno-trace/internal/watcher"
)

// leftPane tracks what the left pane is showing.
type leftPane int

const (
	leftSessions leftPane = iota // session list (top level)
	leftLoading                  // loading a session (transient)
	leftTurns                    // turns within a session
)

// tickMsg fires every second during live sessions for elapsed time updates.
type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// App is the root Bubbletea model for kno-trace.
type App struct {
	left        leftPane
	sessions    sessionListModel     // session list for the left pane
	timeline    timelineModel        // turns + detail for when a session is open
	sessionMeta *model.SessionMeta   // selected session metadata
	session     *model.Session       // fully parsed session
	events      []*parser.RawEvent   // accumulated events during loading
	statusMsg   string
	cfg         *config.Config
	stopWatcher func()
	width       int
	height      int

	// Live session state.
	liveToolCallsByID  map[string]*model.ToolCall
	lastBranch         string
	agentCounter       int
	agentWatcher       *agent.AgentWatcher
	agentWatcherCh     <-chan interface{}
	agentWatcherSendCh chan interface{}
	agentWatcherDone   chan struct{}
}

// NewApp creates the root model starting at the session list.
func NewApp(sessions []*model.SessionMeta, cfg *config.Config) App {
	return App{
		left:     leftSessions,
		sessions: newSessionList(sessions),
		cfg:      cfg,
	}
}

// NewAppWithSession creates the root model auto-opened to a session.
func NewAppWithSession(sessions []*model.SessionMeta, session *model.SessionMeta, statusMsg string, cfg *config.Config) App {
	return App{
		left:        leftLoading,
		sessions:    newSessionList(sessions),
		sessionMeta: session,
		statusMsg:   statusMsg,
		cfg:         cfg,
	}
}

func (a App) Init() tea.Cmd {
	if a.left == leftLoading && a.sessionMeta != nil {
		return a.startWatcher(a.sessionMeta.FilePath)
	}
	return nil
}

// startWatcher launches the watcher goroutine.
func (a App) startWatcher(path string) tea.Cmd {
	return func() tea.Msg {
		ch := make(chan tea.Msg, 256)
		done := make(chan struct{})
		tailer := watcher.New(path, a.cfg, func(msg tea.Msg) {
			select {
			case ch <- msg:
			case <-done:
			}
		})
		stopTailer := tailer.Start()
		stop := func() {
			close(done)
			stopTailer()
			close(ch)
		}
		return msgWatcherStarted{ch: ch, stop: stop}
	}
}

type msgWatcherStarted struct {
	ch   <-chan tea.Msg
	stop func()
}

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

type msgAgentWatcherEvent struct {
	inner interface{}
	ch    <-chan interface{}
}

func waitForAgentWatcher(ch <-chan interface{}) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msgAgentWatcherEvent{inner: msg, ch: ch}
	}
}

// msgSessionRefresh signals a rescan of sessions.
type msgSessionRefresh struct{}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.sessions.setSize(msg.Width, msg.Height)
		a.timeline.setSize(msg.Width, msg.Height)
		return a, nil

	case msgWatcherStarted:
		a.stopWatcher = msg.stop
		return a, waitForWatcher(msg.ch)

	case msgWatcherEvent:
		cmd := a.handleWatcherMsg(msg.inner)
		return a, tea.Batch(cmd, waitForWatcher(msg.ch))

	case msgAgentWatcherEvent:
		cmd := a.handleAgentWatcherMsg(msg.inner)
		return a, tea.Batch(cmd, waitForAgentWatcher(msg.ch))

	case msgSessionRefresh:
		a.refreshSessions()
		a.statusMsg = ""
		return a, nil

	case tickMsg:
		if a.left == leftTurns && a.session != nil && a.session.IsLive {
			return a, tickCmd()
		}
		return a, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			a.cleanup()
			return a, tea.Quit
		}

		switch a.left {
		case leftSessions:
			return a.updateSessions(msg)
		case leftLoading:
			return a.updateLoading(msg)
		case leftTurns:
			return a.updateTurns(msg)
		}
	}

	return a, nil
}

// --- Session list (left pane = sessions) ---

func (a App) updateSessions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.sessions.filtering {
		var cmd tea.Cmd
		a.sessions, cmd = a.sessions.Update(msg)
		return a, cmd
	}

	switch msg.String() {
	case "enter":
		sel := a.sessions.selectedSession()
		if sel != nil {
			a.resetSession()
			a.sessionMeta = sel
			a.left = leftLoading
			a.statusMsg = ""
			return a, a.startWatcher(sel.FilePath)
		}
		return a, nil
	case "q":
		a.cleanup()
		return a, tea.Quit
	case "esc":
		if a.sessions.filter != "" {
			a.sessions.clearFilter()
			return a, nil
		}
		a.cleanup()
		return a, tea.Quit
	case "R":
		a.refreshSessions()
		return a, nil
	}

	var cmd tea.Cmd
	a.sessions, cmd = a.sessions.Update(msg)
	return a, cmd
}

func (a App) updateLoading(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		a.cleanup()
		return a, tea.Quit
	case "esc":
		a.resetSession()
		a.refreshSessions()
		a.left = leftSessions
		return a, nil
	}
	return a, nil
}

// --- Turns (left pane = turns, right pane = detail) ---

func (a App) updateTurns(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// If filtering, let the timeline handle all keys.
	if a.timeline.filtering {
		var cmd tea.Cmd
		a.timeline, cmd = a.timeline.Update(msg)
		return a, cmd
	}

	// If detail pane has focus, route keys there.
	if a.timeline.detail.HasFocus {
		return a.updateDetailFocused(msg)
	}

	// Left pane (turns list) has focus.
	switch msg.String() {
	case "q":
		a.cleanup()
		return a, tea.Quit
	case "esc":
		// Layered: comparison → filter → sessions.
		if a.timeline.detail.IsComparing() {
			a.timeline.detail.ClearComparison()
			return a, nil
		}
		if a.timeline.filter != "" {
			a.timeline.filter = ""
			a.timeline.filteredIdxs = nil
			a.timeline.restoreFullList()
			return a, nil
		}
		// Clear selection if any, otherwise back to sessions.
		if a.timeline.list.SelectedCount() > 0 {
			a.timeline.list.ClearSelection()
			return a, nil
		}
		a.resetSession()
		a.refreshSessions()
		a.left = leftSessions
		a.statusMsg = ""
		return a, nil
	case " ":
		// Toggle selection on current turn.
		a.timeline.list.ToggleSelection()
		return a, nil
	case "c":
		// Compare selected turns.
		indices := a.timeline.list.SelectedPromptIndices()
		if len(indices) >= 2 && a.session != nil {
			content := buildComparison(a.session, indices[0], indices[len(indices)-1], a.timeline.detail.Width)
			a.timeline.detail.SetComparison(content)
		}
		return a, nil
	case "enter":
		// Give focus to the detail pane.
		selected := a.timeline.list.SelectedPrompt()
		if selected != nil {
			a.timeline.detail.HasFocus = true
			// Disengage auto-follow — the user is drilling into a specific turn.
			a.timeline.autoFollow = false
			if len(selected.Agents) > 0 {
				a.timeline.detail.itemCursor = 0
			}
		}
		return a, nil
	}

	prevCursor := a.timeline.list.Cursor
	var cmd tea.Cmd
	a.timeline, cmd = a.timeline.Update(msg)

	// Reset detail state when prompt cursor changes.
	if a.timeline.list.Cursor != prevCursor {
		a.timeline.detail.ResetExpansion()
	}

	return a, cmd
}

// updateDetailFocused handles keys when the detail pane has focus.
func (a App) updateDetailFocused(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		a.cleanup()
		return a, tea.Quit
	case "esc":
		// Layered dismissal: tool call → expanded → agent focus → left pane.
		if a.timeline.detail.IsDrilledIntoToolCall() {
			a.timeline.detail.ExitToolCallDrillIn()
			return a, nil
		}
		if a.timeline.detail.IsAgentExpanded() {
			a.timeline.detail.CollapseAgent()
			return a, nil
		}
		if a.timeline.detail.IsAgentFocused() {
			a.timeline.detail.itemCursor = -1
			return a, nil
		}
		// Return focus to the left pane (turns list).
		a.timeline.detail.HasFocus = false
		return a, nil
	case "enter":
		if a.timeline.detail.IsDrilledIntoToolCall() {
			return a, nil
		}
		selected := a.timeline.list.SelectedPrompt()
		if selected == nil {
			return a, nil
		}
		if a.timeline.detail.IsAgentExpanded() {
			ag := a.timeline.detail.resolveExpandedAgent(selected)
			if ag == nil {
				return a, nil
			}
			// Inside expanded agent: cursor navigates tool calls + children.
			cursor := a.timeline.detail.itemCursor
			if cursor >= 0 && cursor < len(ag.ToolCalls) {
				// Drill into a tool call.
				var b strings.Builder
				a.timeline.detail.renderToolCallDetail(&b, selected, ag.ToolCalls[cursor], a.timeline.detail.Width)
				a.timeline.detail.DrillIntoToolCall(b.String())
			} else {
				childIdx := cursor - len(ag.ToolCalls)
				if childIdx >= 0 && childIdx < len(ag.Children) {
					a.timeline.detail.ExpandAgent(ag.Children, childIdx)
				}
			}
		} else {
			// At prompt level: resolve what the cursor points at.
			tc, agentIdx := ResolveItem(selected, a.timeline.detail.itemCursor)
			if tc != nil {
				// Drill into tool call.
				var b strings.Builder
				a.timeline.detail.renderToolCallDetail(&b, selected, tc, a.timeline.detail.Width)
				a.timeline.detail.DrillIntoToolCall(b.String())
			} else if agentIdx >= 0 {
				a.timeline.detail.ExpandAgent(selected.Agents, agentIdx)
			}
		}
		return a, nil
	case "j", "down":
		selected := a.timeline.list.SelectedPrompt()
		if selected == nil {
			return a, nil
		}
		if a.timeline.detail.IsDrilledIntoToolCall() {
			a.timeline.detail.ScrollDown()
			return a, nil
		}
		if a.timeline.detail.IsAgentExpanded() {
			ag := a.timeline.detail.resolveExpandedAgent(selected)
			if ag != nil {
				a.timeline.detail.AgentCursorDown(len(ag.ToolCalls) + len(ag.Children))
			}
		} else {
			a.timeline.detail.AgentCursorDown(ItemCount(selected))
		}
		return a, nil
	case "k", "up":
		if a.timeline.detail.IsDrilledIntoToolCall() {
			a.timeline.detail.ScrollUp()
			return a, nil
		}
		a.timeline.detail.AgentCursorUp()
		return a, nil
	case "h", "left", "l", "right":
		// Scroll in drill-in views (full diff, bash output).
		if a.timeline.detail.IsDrilledIntoToolCall() {
			if msg.String() == "j" || msg.String() == "down" || msg.String() == "l" || msg.String() == "right" {
				a.timeline.detail.ScrollDown()
			} else {
				a.timeline.detail.ScrollUp()
			}
		}
		return a, nil
	}
	return a, nil
}

// --- Watcher message handling ---

func (a *App) handleWatcherMsg(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case watcher.MsgNewEvents:
		if a.left == leftTurns && a.session != nil {
			var branch string
			var sealedIdxs []int
			branch, sealedIdxs, a.agentCounter = parser.RebuildActivePrompt(
				a.session, msg.Events, 0, a.cfg, a.liveToolCallsByID, a.lastBranch, a.agentCounter)
			a.lastBranch = branch

			a.notifyAgentWatcher(msg.Events)

			tickerEntries := extractTickerEntries(msg.Events)
			a.timeline.ticker.Push(tickerEntries)

			a.timeline.syncSession(a.session)

			if len(sealedIdxs) > 0 {
				a.timeline.ticker.ResetForNewPrompt()
				a.timeline.onPromptSealed()
			}
		} else {
			a.events = append(a.events, msg.Events...)
		}

	case watcher.MsgReplayDone:
		a.rebuildSession()
		if a.session != nil {
			a.liveToolCallsByID = parser.BuildToolCallsByID(a.session)
			a.agentCounter = parser.CountAgents(a.session)

			for i := len(a.events) - 1; i >= 0; i-- {
				if a.events[i].GitBranch != "" {
					a.lastBranch = a.events[i].GitBranch
					break
				}
			}
			a.events = nil

			if len(a.session.Prompts) > 0 {
				last := a.session.Prompts[len(a.session.Prompts)-1]
				a.session.IsLive = last.EndTime.IsZero()
			}

			a.timeline = newTimeline(a.session)
			a.timeline.setSize(a.width, a.height)

			if a.session.IsLive {
				a.timeline.autoFollow = true
				a.timeline.isLive = true
				a.timeline.ticker = NewTicker(a.cfg.LoopDetectionThreshold)
				a.timeline.followLatest()

				if a.session.FilePath != "" {
					sessionDir := filepath.Dir(a.session.FilePath)
					ch := make(chan interface{}, 256)
					done := make(chan struct{})
					a.agentWatcherCh = ch
					a.agentWatcherSendCh = ch
					a.agentWatcherDone = done
					aw := agent.NewAgentWatcher(sessionDir, a.session.ID, func(msg interface{}) {
						select {
						case ch <- msg:
						case <-done:
						}
					})
					if err := aw.Start(); err == nil {
						a.agentWatcher = aw
					}
				}
			}
		}
		a.left = leftTurns

		if a.session != nil && a.session.IsLive {
			var cmds []tea.Cmd
			cmds = append(cmds, tickCmd())
			if a.agentWatcherCh != nil {
				cmds = append(cmds, waitForAgentWatcher(a.agentWatcherCh))
			}
			return tea.Batch(cmds...)
		}
		a.cleanup()

	case watcher.MsgSessionFileDeleted:
		if a.session != nil {
			a.session.IsLive = false
		}
		a.timeline.isLive = false
		a.statusMsg = "Session file removed"
		a.cleanup()

	case watcher.MsgWatcherError:
		a.resetSession()
		a.refreshSessions()
		a.left = leftSessions
		a.statusMsg = "Session unavailable — list refreshed"
	}
	return nil
}

func (a *App) handleAgentWatcherMsg(msg interface{}) tea.Cmd {
	switch msg := msg.(type) {
	case agent.MsgAgentToolCall:
		if a.session == nil {
			return nil
		}
		for _, prompt := range a.session.Prompts {
			for _, ag := range prompt.Agents {
				if ag.ToolUseID == msg.ToolUseID {
					ag.ToolCalls = append(ag.ToolCalls, msg.ToolCall)
					if msg.ToolCall.Path != "" && agent.IsFilePath(msg.ToolCall.Type) {
						found := false
						for _, p := range ag.FilesTouched {
							if p == msg.ToolCall.Path {
								found = true
								break
							}
						}
						if !found {
							ag.FilesTouched = append(ag.FilesTouched, msg.ToolCall.Path)
						}
					}
					if msg.ToolCall.Path != "" && agent.IsFilePath(msg.ToolCall.Type) {
						a.checkLiveConflict(prompt, ag, msg.ToolCall.Path)
					}
					a.timeline.syncSession(a.session)
					return nil
				}
			}
		}
	case agent.MsgAgentFileFound:
	case agent.MsgAgentFileMissing:
	}
	return nil
}

func (a *App) checkLiveConflict(prompt *model.Prompt, currentAgent *model.AgentNode, path string) {
	if len(prompt.Agents) < 2 {
		return
	}
	for _, other := range prompt.Agents {
		if other.ToolUseID == currentAgent.ToolUseID {
			continue
		}
		if other.Status != model.AgentRunning && other.Status != model.AgentSucceeded {
			continue
		}
		for _, otherPath := range other.FilesTouched {
			if otherPath == path {
				msg := fmt.Sprintf("file conflict: %s touched by %s, %s",
					path, currentAgent.Label, other.Label)
				for _, w := range prompt.Warnings {
					if w.Type == model.WarnAgentConflict && w.Message == msg {
						return
					}
				}
				prompt.Warnings = append(prompt.Warnings, model.Warning{
					Type:    model.WarnAgentConflict,
					Message: msg,
				})
				return
			}
		}
	}
}

func (a *App) notifyAgentWatcher(events []*parser.RawEvent) {
	if a.agentWatcher == nil {
		return
	}
	for _, evt := range events {
		switch evt.Type {
		case "user":
			if evt.Message == nil {
				continue
			}
			for _, block := range evt.Message.Content {
				if block.Type != "tool_result" {
					continue
				}
				tc, ok := a.liveToolCallsByID[block.ToolResultID]
				if !ok || tc.Type != model.ToolAgent {
					continue
				}
				a.agentWatcher.StopAgent(block.ToolResultID)
				a.enrichCompletedAgent(block.ToolResultID)
			}
		case "progress":
			if evt.ProgressData == nil || evt.ProgressData.Type != "agent_progress" {
				continue
			}
			agentID := evt.ProgressData.AgentID
			parentToolUseID := evt.ProgressData.ParentToolUseID
			if agentID != "" && parentToolUseID != "" {
				a.agentWatcher.WatchAgent(agentID, parentToolUseID)
			}
		}
	}
}

func (a *App) enrichCompletedAgent(toolUseID string) {
	if a.session == nil || a.session.FilePath == "" {
		return
	}
	for _, prompt := range a.session.Prompts {
		for _, ag := range prompt.Agents {
			if ag.ToolUseID != toolUseID || ag.ID == "" {
				continue
			}
			sessionDir := filepath.Dir(a.session.FilePath)
			path := agent.SubagentFilePath(sessionDir, a.session.ID, ag.ID)
			ag.ToolCalls = nil
			ag.FilesTouched = nil
			ag.TokensIn = 0
			ag.TokensOut = 0
			if err := agent.EnrichFromFile(ag, path, a.cfg); err != nil {
				fmt.Fprintf(os.Stderr, "kno-trace: enriching agent %s: %v\n", ag.ID, err)
			}
			a.timeline.syncSession(a.session)
			return
		}
	}
}

// --- Helpers ---

func extractTickerEntries(events []*parser.RawEvent) []TickerEntry {
	var entries []TickerEntry
	for _, evt := range events {
		switch evt.Type {
		case "assistant":
			if evt.Message == nil {
				continue
			}
			for _, block := range evt.Message.Content {
				if block.Type != "tool_use" {
					continue
				}
				entries = append(entries, tickerEntryFromBlock(block, evt.Timestamp, ""))
			}
		case "progress":
			if evt.ProgressData == nil || evt.ProgressData.Type != "agent_progress" {
				continue
			}
			for _, block := range evt.ProgressData.ToolUseBlocks {
				entries = append(entries, tickerEntryFromBlock(
					block, evt.Timestamp, evt.ProgressData.AgentID))
			}
		}
	}
	return entries
}

func tickerEntryFromBlock(block parser.ContentBlock, ts time.Time, agentID string) TickerEntry {
	toolType, _, _ := parser.ClassifyToolName(block.ToolName)
	return TickerEntry{
		ToolType:  toolType,
		Path:      extractToolPath(block),
		AgentID:   agentID,
		Timestamp: ts,
	}
}

func extractToolPath(block parser.ContentBlock) string {
	if len(block.ToolInput) == 0 {
		return ""
	}
	type pathInput struct {
		FilePath string `json:"file_path"`
		Pattern  string `json:"pattern"`
		Command  string `json:"command"`
	}
	var pi pathInput
	if err := json.Unmarshal(block.ToolInput, &pi); err == nil {
		if pi.FilePath != "" {
			return pi.FilePath
		}
		if pi.Pattern != "" {
			return pi.Pattern
		}
		if pi.Command != "" {
			if len(pi.Command) > 50 {
				return pi.Command[:50] + "..."
			}
			return pi.Command
		}
	}
	return ""
}

func (a *App) rebuildSession() {
	if len(a.events) == 0 {
		return
	}
	a.session = parser.BuildSession(a.events, a.cfg)
	if a.sessionMeta != nil {
		if a.session.ProjectName == "" {
			a.session.ProjectName = a.sessionMeta.ProjectName
		}
		if a.session.ProjectPath == "" {
			a.session.ProjectPath = a.sessionMeta.ProjectPath
		}
		a.session.FilePath = a.sessionMeta.FilePath
	}
	if a.session.FilePath != "" {
		sessionDir := filepath.Dir(a.session.FilePath)
		agent.EnrichSession(a.session, sessionDir, a.cfg)
	}
}

func (a *App) cleanup() {
	if a.agentWatcher != nil {
		a.agentWatcher.Stop()
		a.agentWatcher = nil
	}
	if a.agentWatcherDone != nil {
		close(a.agentWatcherDone)
		a.agentWatcherDone = nil
	}
	if a.agentWatcherSendCh != nil {
		close(a.agentWatcherSendCh)
		a.agentWatcherSendCh = nil
		a.agentWatcherCh = nil
	}
	if a.stopWatcher != nil {
		a.stopWatcher()
		a.stopWatcher = nil
	}
}

func (a *App) resetSession() {
	a.cleanup()
	a.events = nil
	a.session = nil
	a.liveToolCallsByID = nil
	a.lastBranch = ""
	a.agentCounter = 0
}

func (a *App) refreshSessions() {
	sessions, _ := discovery.ScanAll()
	if a.cfg != nil && len(sessions) > a.cfg.MaxPickerSessions {
		sessions = sessions[:a.cfg.MaxPickerSessions]
	}
	a.sessions = newSessionList(sessions)
	a.sessions.setSize(a.width, a.height)
}

// --- View ---

func (a App) View() string {
	switch a.left {
	case leftSessions:
		return a.viewSessions()
	case leftLoading:
		return a.viewLoading()
	case leftTurns:
		return a.timeline.View()
	default:
		return ""
	}
}

func (a App) viewSessions() string {
	v := a.sessions.View()
	if a.statusMsg != "" {
		v += "\n" + MutedStyle.Render("  "+a.statusMsg)
	}
	return v
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
		KeyStyle.Render("esc") + " " + KeyDescStyle.Render("back") + "  " +
			KeyStyle.Render("q") + " " + KeyDescStyle.Render("quit")))

	return b.String()
}
