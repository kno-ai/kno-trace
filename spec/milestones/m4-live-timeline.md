# M4: Live Timeline & Ticker

**Prerequisites:** Read [spec/README.md](../README.md) (core spec) and `SCHEMA.md`. M3 must be complete (static timeline rendering working).

**Goal:** The control room comes alive — real-time updates as Claude works, a ticker streaming tool calls, and the loop/stall indicators that make kno-trace worth leaving open in a second terminal. After this milestone, kno-trace is usable as a daily driver alongside active Claude Code sessions.

**Deliverable:** User opens kno-trace in a second terminal alongside a running Claude Code session. Prompts appear in real time. Tool calls stream across the ticker with agent attribution. The user sees immediately if Claude is working, stalled, or stuck in a loop.

**Use cases served:** UC1 (what did Claude just do? — live), UC4 (what are my agents doing? — live ticker and stats bar lay the groundwork for M5), UC5 (context filling up? — real-time context%)

---

## Scope

### Live session wiring

- **Upgrade M3's watcher from replay-only to live tail.** M3 already uses the watcher for incremental replay (prompts appear one by one via `MsgPromptSealed`). M4 keeps the watcher running after `MsgReplayDone` to tail new lines via fsnotify. This is a configuration change to the watcher startup, not a new integration — the event handling in the UI is the same.
- Add handling for events that only matter during live sessions:
  - `MsgPromptUpdate` → update active prompt tool calls in real time
  - `MsgSessionFileDeleted` → show "Session file removed — press P for picker or q to quit"
- `MsgPromptSealed` and `MsgReplayDone` are already handled by M3
- Completed sessions still load via replay-only (M3 behavior unchanged)

### Active prompt

- Active live prompt: distinct styling; does not seal until next human turn arrives
- **Active prompt elapsed time:** The live prompt shows a running timer — `#7 [14:32-...] 3m 42s` — derived from the difference between now and the prompt's start timestamp. This is the control room's most basic signal: "how long has Claude been working on this?" If the timer is climbing with no ticker activity, something may be stalled.

### Auto-follow

When session is live, the prompt list auto-scrolls to the latest prompt and the detail pane tracks it. If the user manually navigates to a different prompt, auto-follow disengages — the user is investigating. Auto-follow re-engages when a new prompt seals (the user is likely done investigating). This mirrors how a log viewer or monitoring tool works: follow the tail unless the user scrolls away.

### Ticker

- `internal/ui/components/ticker.go` — live ticker strip at bottom:
  - Shows tool calls of the active in-flight prompt as they arrive
  - **Agent attribution (lightweight, from progress lines):** When a tool call arrives via a `progress` line with `data.type: "agent_progress"`, the parser already extracts `data.agentId` and `parentToolUseID`. M4 uses this to set `ToolCall.SourceAgent` and prefix the ticker with the agent label and color: `subagent-1: Edit internal/parser/jsonl.go`. This does NOT require the full agent tree builder from M5 — it's simple field extraction from progress lines.
    - **When progress lines are absent:** Some agents (particularly `general-purpose` type) may not emit progress lines. In this case, the ticker shows parent tool calls only. The agent's activity becomes visible when its `tool_result` arrives (with `toolUseResult` summary). This is a known limitation before M5 adds subagent JSONL file reading.
  - When multiple agents are active simultaneously and emitting progress lines, interleave their tool calls chronologically with distinct colors per agent — the user should immediately see "two agents are both working"
  - **Loop/spin indicator (live detection):** The ticker independently tracks incoming tool calls for the active prompt using the same sliding-window logic as M2's `classify.go` (same tool+path pair repeated N times). When the threshold is hit, the ticker shows a persistent warning: `⟳ possible loop: Edit config.go repeated 4×`. Clears when a different tool+path appears, breaking the repetition. This is the control room's early warning — the user can Ctrl-C instead of waiting 10 minutes.
    - This is separate from M2's post-hoc `WarnLoopDetected` (which fires when the prompt seals and appears as a badge in the timeline). The ticker needs live detection because the prompt hasn't sealed yet — the whole point is catching loops *during* the prompt, not after.
  - **Quiet state indicator:** When the ticker has had no activity for > 10 seconds during a live session, show elapsed time since last tool call in the ticker strip: `last activity 45s ago`. This is factual (derived from timestamps) and answers "is Claude thinking, or is something stuck?" Disappears as soon as new activity arrives.
  - Hidden when no live prompt is active

### Stats bar upgrades

- Live dot (only when `Session.IsLive`) added to M3's stats bar
- **Agent activity indicator:** When the active prompt has spawned agents that haven't completed yet, show agent status in the stats bar: `⬡ 2 active` or `⬡ 1 active, 1 done`. This gives passive visibility without needing to drill into the detail pane or switch to swimlane. Updates in real time as agents complete.

### Detail pane live updates

- **Live agent status:** While agents are active, each agent node shows a live status: `⬡ subagent-1 — running — 4 tools so far — 12s`. When complete: `⬡ subagent-1 — done — 7 tools, 3 files — 18s` or `⬡ subagent-1 — failed — 2 tools — 5s`. The "running" state is determined by whether the agent's Agent tool_result has been received yet (exact, not heuristic). This is the detail pane's answer to "is anything happening?" while the parent waits.
- Agent nodes remain non-expandable (M5 adds full expansion and swimlane)

---

## Acceptance Criteria

- Live session appends prompts in real time without full re-render
- Completed sessions still load correctly (M3 behavior preserved)
- Active prompt shows running elapsed time
- Auto-follow tracks latest prompt during live session; disengages on manual navigation; re-engages on new prompt seal
- Ticker shows tool calls of active prompt as they arrive
- Ticker shows agent-attributed tool calls with distinct colors when agents are active
- Loop/spin indicator appears in ticker when tool+path repetition exceeds threshold; clears when broken
- Quiet state indicator shows "last activity Ns ago" when ticker is idle for >10s
- Stats bar shows live dot when session is live
- Stats bar shows `⬡ N active` when agents are running in the live prompt
- Agent nodes in detail pane show live running/done/failed status
- `MsgSessionFileDeleted` message shown
- Terminal resize does not corrupt layout (M3 behavior preserved)
