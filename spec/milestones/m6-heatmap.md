# M6: Rich Detail Pane — File Intelligence, Diffs, and Live Activity

**Prerequisites:** M5 complete (agent data layer, live tailing working).

**Goal:** The detail pane becomes the control room. At a glance you see what's happening — files changing, diffs inline, agents working. Drill in for depth. This is where kno-trace stops being "the same as Claude Code's output" and becomes genuinely useful.

**Deliverable:** A developer glances at the detail pane and immediately sees: which files are changing, what the changes look like, which agents are doing what. They drill into any item for full detail. The value is obvious within 5 seconds of opening.

**Use cases served:** UC1 (what did Claude do?), UC2 (what happened to this file?), UC4 (what are agents doing?), UC6 (which files are getting thrashed?)

---

## Design Principle: The Detail Pane is a Stack

The detail pane uses a consistent interaction model everywhere:

- **j/k** navigate items at the current level
- **enter** drills into the selected item
- **esc** backs out one level
- Every level has a breadcrumb showing where you are

The stack:
1. **Prompt summary** (top level) — header, human text, warnings, then sections for tool calls, agents, and file activity
2. **Agent detail** — task description, tool calls, files touched with W/R/E counts (already built in M5)
3. **Tool call detail** — for Edits: inline diff. For Bash: output. For Writes: content or diff against prior state.
4. **File history** — all operations on a file across the session, with diffs at each step

---

## Build Order

### Layer 1: Replay engine and inline diffs

Build `internal/replay/engine.go` — the foundation for everything else.

- Build `FileHistory` per file from all tool calls (parent + agent, interleaved by timestamp)
- `GetContentAt(path, promptIdx)` — snapshot reconstruction
- `Compute(a, b string) []DiffHunk` — diff computation via go-diff
- Inline mini-diffs in the detail pane: Edit ops show old/new as a colored unified diff right in the tool call list. No drilling required — you see the diff at a glance.
- Write ops where prior state exists: show line count delta, enter to see full diff

**Checkpoint:** Open a completed session with Edits. See colored diffs inline in the detail pane for every Edit tool call. `--dump` shows diffs.

### Layer 2: File activity section in detail pane

Add a "Files" section at the bottom of the prompt detail showing all files touched in this prompt (parent + agents), with:

- File path, operation badges (W/R/E), agent attribution
- HeatScore indicator (total ops on this file across the session, not just this prompt)
- Files sorted by activity (most-touched first)
- Conflict warnings inline for files touched by multiple agents

This is the "hot spot detector" — visible at a glance without switching views.

**Checkpoint:** Open a session. Scroll down in the detail pane. See which files were touched, by whom, with what intensity. Files with high churn across the session stand out.

### Layer 3: Drill-in for tool calls and file history

Make every item in the detail pane drillable:

- **Enter on an Edit tool call** → shows the full diff (already partially visible from Layer 1 inline diffs, but enter shows it with more context)
- **Enter on a Bash tool call** → shows the full command and output
- **Enter on a file in the Files section** → shows the complete file history: every operation across the entire session, with diffs at each step, agent attribution, timestamp. This is the answer to "what happened to this file?"

All drill-ins use the same enter/esc/breadcrumb pattern as agent expansion.

**Checkpoint:** Navigate to a prompt. Enter on an Edit → see the diff. Esc back. Scroll to Files section. Enter on a file → see its full history across the session.

### Layer 4: Live activity improvements

Make running agents more visible without drilling in:

- Running agents show their **latest tool call inline** in the collapsed summary:
  ```
  ⬡ subagent-1 (Explore) — running 14s
      → Edit internal/ui/detail.go
  ```
- Completed agents show file count and churn summary:
  ```
  ⬡ subagent-1 (Explore) — done 25s — 4 files, 2 edits
  ```
- File activity section updates live as agent tool calls arrive

**Checkpoint:** Run alongside Claude Code spawning agents. See agent activity updating in real time in the collapsed view. See the Files section grow as agents touch files.

---

## Scope Details

### Replay engine (`internal/replay/engine.go`)

- Builds `FileHistory` per file after full session parse
- Includes agent tool calls: collect Write/Edit/Read from `Prompt.ToolCalls` AND `Prompt.Agents[*].ToolCalls`, interleaved by timestamp
- Agent attribution: each entry tracks source agent (empty = parent)
- `GetContentAt(path, promptIdx)`: snapshot reconstruction per spec
- `GetFileHistory(path)`: returns the FileHistory for a file
- `ListFiles()`: sorted by HeatScore descending
- Bounded: WriteSnapshots capped per config (`max_snapshots_per_file`)

### Diff computation (`internal/replay/diff.go`)

- `Compute(a, b string) []DiffHunk` via go-diff
- DiffHunk: type (add/del/context), line number, content
- Truncation: max lines per hunk configurable, `... N more lines` beyond

### Inline mini-diffs in detail pane

- Edit ops: always show old_str → new_str as colored diff (teal additions, red deletions)
- Write ops with prior state: `+N -M lines — enter to expand`
- Write ops without prior state: just show path and line count
- Max ~6 lines inline; enter for full view

### File activity section

- Appears at the bottom of prompt detail, below tool calls and agents
- Shows all files touched in this prompt (parent + all agents)
- Per file: path, op badges (W×N R×M E×K), agent labels, heat indicator
- Heat indicator: visual intensity based on total session HeatScore for that file
- Sorted by HeatScore descending (most-thrashed first)
- Navigable: j/k when cursor is in this section, enter for file history

---

## Acceptance Criteria

### Replay engine
- `GetContentAt()` correct for simple.jsonl and replay_chain.jsonl fixtures
- Agent Write followed by parent Edit correctly reconstructed
- `WarnReplayGap` on genuinely unresolvable Edit; no crash on malformed data
- WriteSnapshots bounded by config

### Inline diffs
- Edit diffs render correctly with colored add/del lines
- Write diffs show correct line delta
- Long diffs truncated with "... N more lines"
- Diffs work for both parent and agent tool calls

### File activity
- Files section shows all files from prompt (parent + agents)
- Agent attribution correct
- HeatScore reflects session-wide activity, not just current prompt
- Sorted by activity
- Live updates as agent tool calls arrive

### Drill-in
- Enter on Edit → full diff view
- Enter on Bash → command + output
- Enter on file → complete file history with per-step diffs
- Esc from any drill-in → back to previous level
- Breadcrumb correct at every level

### Live activity
- Running agents show latest tool call inline
- Completed agents show file/edit counts
- File activity section updates as MsgAgentToolCall arrives

---

## Notes

- The old M6 (heatmap as a separate view) is folded into the detail pane as the "Files" section. No separate `h` key, no separate view. The detail pane IS the heatmap — sorted by activity, with heat indicators.
- The old M7 (diff view with mark workflow) is partially folded into this milestone (inline diffs, file history drill-in). The full session-scoped diff between two arbitrary prompts is deferred to M7.
- This milestone is the one that makes kno-trace worth opening. Everything before this was plumbing.
