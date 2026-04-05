# M3: Timeline View

**Prerequisites:** Read [spec/README.md](../README.md) (core spec) and `SCHEMA.md`. M2 must be complete (parser, builder, watcher all working).

**Goal:** The core control room — a live feed of everything Claude is doing, with at-a-glance status and on-demand detail. After this milestone, kno-trace is usable as a daily driver.

**Deliverable:** User opens kno-trace in a second terminal and immediately sees session state: prompts appearing in real time, active prompt timer counting, tool calls streaming across the ticker, context% climbing. They can drill into any prompt for full detail.

**Use cases served:** UC1 (what did Claude just do?), UC4 (what are my agents doing? — live ticker and stats bar lay the groundwork), UC5 (context filling up?)

---

## Scope

- `internal/ui/timeline.go`:
  - Left pane: Bubbles `list` — prompt items with index, truncated text, time, badges (write/read/edit/bash counts), warning indicators
  - Active live prompt: distinct styling; does not seal until next human turn arrives
  - **Active prompt elapsed time:** The live prompt shows a running timer — `#7 [14:32-...] 3m 42s` — derived from the difference between now and the prompt's start timestamp. This is the control room's most basic signal: "how long has Claude been working on this?" If the timer is climbing with no ticker activity, something may be stalled.
  - Interrupted prompt: `��` badge
  - Right pane: Bubbles `viewport` — prompt detail:
    - Header: index, time range, duration, model name, tokens in/out (with cache read/create if present), context% badge colored by threshold (only shown when ContextPct > 0)
    - Human text block
    - Warning alerts in colored boxes (above tool list)
    - Tool calls: type icon + name + path + delta (+N/-M lines)
    - pin icon on CLAUDE.md writes
    - Distinct styling on MCP calls with irreversible note
    - Subagent spawn shown as collapsed `⬡ subagent-N — <task>` node with tool count and file count (full expansion and rich detail implemented in M4)
    - **Live agent status:** While agents are active, each agent node shows a live status: `⬡ subagent-1 — running — 4 tools so far — 12s`. When complete: `⬡ subagent-1 — done — 7 tools, 3 files — 18s` or `⬡ subagent-1 — failed — 2 tools — 5s`. The "running" state is determined by whether the agent's Task tool_result has been received yet (exact, not heuristic). This is the detail pane's answer to "is anything happening?" while the parent waits.
    - Compact marker shown on prompt where compact occurred
- `internal/ui/components/ticker.go` — live ticker strip at bottom:
  - Shows tool calls of the active in-flight prompt as they arrive
  - **Agent attribution:** When a tool call belongs to a subagent, prefix with the agent label and color-code to match: `subagent-1: Edit internal/parser/jsonl.go`. This is critical — while the parent session is "awaiting agents," the ticker is the user's primary signal that work is happening. Without attribution, multiple agent tool calls look like a confusing stream of parent activity.
  - When multiple agents are active simultaneously, interleave their tool calls chronologically with distinct colors per agent — the user should immediately see "two agents are both working"
  - Hidden when no live prompt is active
- Live wiring:
  - `MsgPromptSealed` -> append to list, update stats bar
  - `MsgPromptUpdate` -> update active prompt tool calls in real time
  - `MsgReplayDone` -> remove "replaying..." from status bar
  - `MsgSessionFileDeleted` -> show "Session file removed — press P for picker or q to quit"
  - Prompts appear incrementally during replay — not batched at `MsgReplayDone`
- **Auto-follow (control room behavior):** When session is live, the prompt list auto-scrolls to the latest prompt and the detail pane tracks it. If the user manually navigates to a different prompt, auto-follow disengages — the user is investigating. Auto-follow re-engages when a new prompt seals (the user is likely done investigating). This mirrors how a log viewer or monitoring tool works: follow the tail unless the user scrolls away.
- **Quiet state indicator:** When the ticker has had no activity for > 10 seconds during a live session, show elapsed time since last tool call in the ticker strip: `last activity 45s ago`. This is factual (derived from timestamps) and answers "is Claude thinking, or is something stuck?" Disappears as soon as new activity arrives.
- `internal/ui/components/promptlist.go` and `detail.go`
- Stats bar: live dot (only when `Session.IsLive`), session name, prompt count, file count, total tokens, context% of most recent prompt
  - **Agent activity indicator:** When the active prompt has spawned agents that haven't completed yet, show agent status in the stats bar: `⬡ 2 active` or `⬡ 1 active, 1 done`. This gives passive visibility without needing to drill into the detail pane or switch to swimlane. Updates in real time as agents complete.
- Context% in stats bar and prompt header: shown only when > 0
- `/` search/filter in timeline:
  - Activates a search input bar at the top of the prompt list
  - Filters prompts in real time as the user types
  - Matches against: human text content, file paths in tool calls (including partial path/folder matches), tool type names
  - Example: typing `parser` shows prompts whose human text mentions "parser" OR that touched files with "parser" in the path
  - Example: typing `internal/ui/` shows all prompts that touched any file in that folder
  - `enter` dismisses search bar but keeps filter active; `esc` clears filter and restores full list
  - Prompt count in stats bar updates to show `"N/M prompts"` when filtered
- `P` returns to session picker from any view
- All views propagate `tea.WindowSizeMsg` to sub-models
- `t` key activates timeline view

---

## Acceptance Criteria

- Completed session shows all prompts populated incrementally during replay
- Live session appends prompts in real time without full re-render
- Active prompt shows running elapsed time
- Auto-follow tracks latest prompt during live session; disengages on manual navigation; re-engages on new prompt seal
- Quiet state indicator shows "last activity Ns ago" when ticker is idle for >10s
- Context% badge only shown when token data is present; hidden otherwise
- Pin icon visible on CLAUDE.md writes
- Compact marker visible on correct prompt
- MCP calls distinctly styled
- `/` search filters by human text, file path, and folder
- Ticker shows agent-attributed tool calls with distinct colors when agents are active
- Stats bar shows `⬡ N active` when agents are running in the live prompt
- Agent nodes in detail pane show live running/done/failed status
- `MsgSessionFileDeleted` message shown
- Terminal resize does not corrupt layout
- `q` exits cleanly
