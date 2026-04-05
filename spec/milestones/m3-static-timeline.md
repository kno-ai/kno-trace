# M3: Static Timeline

**Prerequisites:** Read [spec/README.md](../README.md) (core spec) and `SCHEMA.md`. M2 must be complete (parser, builder, watcher all working).

**Goal:** The core control room layout — a navigable view of everything Claude did in a session, with at-a-glance status and on-demand detail. After this milestone, kno-trace is useful for reviewing completed sessions.

**Deliverable:** User opens kno-trace, selects a completed session, and sees the full timeline: prompts listed with badges, tool calls in the detail pane, search/filter to find specific files or prompts. No live updates yet — this is the static foundation that M4 brings to life.

**Use cases served:** UC1 (what did Claude just do? — post-hoc), UC5 (context filling up? — badges and stats), UC7 (quickly check a past session)

---

## Scope

- `internal/ui/timeline.go`:
  - Left pane: Bubbles `list` — prompt items with index, truncated text, time, badges (write/read/edit/bash counts), warning indicators
  - **Git branch transition marker:** When a prompt has `BranchTransition`, show a divider above it in the prompt list: `── branch: main → feature/auth ──`. Visually separates work across branches.
  - **Duration outlier badge:** Prompts flagged as `IsDurationOutlier` show a `⏱ slow` badge in the prompt list item. Helps users spot where to drill in during session review.
  - Interrupted prompt: `⚡` badge
  - Right pane: Bubbles `viewport` — prompt detail:
    - Header: index, time range, duration, model name, tokens in/out (with cache read/create if present), context% badge colored by threshold (only shown when ContextPct > 0)
    - Human text block
    - Warning alerts in colored boxes (above tool list)
    - Tool calls: type icon + name + path + delta (+N/-M lines)
    - pin icon on CLAUDE.md writes
    - Distinct styling on MCP calls with irreversible note
    - Subagent spawn shown as collapsed `⬡ subagent-N — <task>` node with tool count and file count (non-expandable until M5; pressing `enter` does nothing)
    - Compact marker shown on prompt where compact occurred
- `internal/ui/components/promptlist.go` and `detail.go`
- Stats bar: session name, prompt count, file count, total tokens, context% of most recent prompt
  - No live dot, no agent activity indicator — those are M4
- Context% in stats bar and prompt header: shown only when > 0
- `/` search/filter in timeline:
  - Activates a search input bar at the top of the prompt list
  - Filters prompts in real time as the user types
  - Matches against: human text content, file paths in tool calls (including partial path/folder matches), tool type names
  - Example: typing `parser` shows prompts whose human text mentions "parser" OR that touched files with "parser" in the path
  - Example: typing `internal/ui/` shows all prompts that touched any file in that folder
  - `enter` dismisses search bar but keeps filter active; `esc` clears filter and restores full list
  - Prompt count in stats bar updates to show `"N/M prompts"` when filtered
- **Session loading:** Use M2's watcher in **replay-only mode** — the watcher replays all existing lines, emitting `MsgPromptSealed` per prompt incrementally (prompts appear one by one during load, never a blank screen). After all lines are replayed, the watcher emits `MsgReplayDone` and **stops** — no live tail, no fsnotify watch. This ensures one code path (the watcher's streaming parser) for both M3 and M4, avoiding the need to reconcile a separate "full parse" path later. M4 upgrades this to continue tailing after replay.
  - This also respects the "never load entire JSONL into memory" constraint — lines are streamed and parsed incrementally, even though the end result is all prompts in memory.
- `P` returns to session picker from any view
- All views propagate `tea.WindowSizeMsg` to sub-models
- `t` key activates timeline view
- `j`/`k` navigation in prompt list

---

## Acceptance Criteria

- Completed session shows all prompts in timeline, loaded incrementally via watcher replay (prompts appear one by one, not all at once)
- `j`/`k` navigates prompt list; detail pane updates to show selected prompt
- Context% badge only shown when token data is present; hidden otherwise
- Pin icon visible on CLAUDE.md writes
- Compact marker visible on correct prompt
- MCP calls distinctly styled
- `/` search filters by human text, file path, and folder
- Git branch transition dividers appear at correct prompts
- Duration outlier `⏱ slow` badge appears on correct prompts; not shown when <5 prompts in session
- Subagent nodes visible but non-expandable (collapsed only)
- `P` returns to session picker
- Terminal resize does not corrupt layout
- `q` exits cleanly
