# M7: File Intelligence — Replay Engine, Hot Files, File History

**Prerequisites:** M6 complete (unified navigation, inline diffs, detail drill-in working).

**Goal:** Answer "what happened to this file?" and "which files are getting thrashed?" — directly from the left pane. The replay engine builds session-wide file histories. The stacked left pane shows hot files alongside turns. Drilling into a file shows its complete story.

**Deliverable:** A developer glances at the left pane and immediately sees which files have the most activity. They select a file and see every operation on it across the session — with diffs, agent attribution, and timestamps. This is the heatmap vision, integrated into the two-pane layout.

**Use cases served:** UC2 (what happened to this file?), UC6 (which files are getting thrashed?)

---

## Build Order

### Layer 1: Replay engine

Build `internal/replay/engine.go` — the foundation for file intelligence.

- **FileHistory per file**: collect all Write/Edit/Read operations from `Prompt.ToolCalls` and `Prompt.Agents[*].ToolCalls`, interleaved by timestamp.
- **Agent attribution**: each entry tracks the source agent (empty = parent).
- **HeatScore**: count of distinct prompts with Write or Edit ops on the file.
- **Session-wide**: built once after full parse, updated incrementally during live sessions.
- **`ListFiles()`**: returns all files sorted by HeatScore descending.
- **`GetFileHistory(path)`**: returns the FileHistory for a file.
- Bounded: WriteSnapshots capped per config.

**Checkpoint:** `--dump` output shows a "Hot files" section listing files by activity.

### Layer 2: Stacked left pane — turns + hot files

Split the left pane vertically: turn list on top, hot files list on bottom.

- **Turn list**: compact, shows ~8-10 most recent turns. Cursor at bottom by default for completed sessions (most recent first in view).
- **File list**: all files touched in the session, sorted by HeatScore. Shows path + op badges (W×N E×M). Updated live as agent tool calls arrive.
- **Tab** switches focus between the two lists. Active list has brighter header.
- **j/k** navigates within the focused list. **enter** drills in:
  - Turn list enter → detail pane shows turn detail (existing behavior)
  - File list enter → detail pane shows file history
- **Default split**: left pane 30% of terminal width (down from 40%). Turn list gets 60% of left pane height, files get 40%.

**Checkpoint:** Open a session. See turns at top-left, hot files at bottom-left. Tab to files. Enter on a file → see its history in the detail pane.

### Layer 3: File history view in detail pane

When entering a file from the file list, the detail pane shows:

- **File path header** with total op count and heat score
- **Chronological entry list**: every operation on this file across the session
  - Per entry: turn index, timestamp, op type (W/E/R), agent label (if from agent)
  - Edit entries: inline mini-diff showing what changed
  - Write entries: line count delta
- **Navigable**: j/k moves between entries, enter on an Edit shows full diff
- **esc** returns to the file list

**Checkpoint:** Navigate to a hot file. See its complete history with diffs at each step. Drill into an Edit for the full diff. Esc back.

### Layer 4: Live updates to file list

During live sessions, the file list updates as new tool calls arrive:

- New files appear in the list when first touched
- HeatScore and op counts update as edits/writes happen
- Files re-sort by activity as heat changes
- Agent-attributed ops show ⬡ indicator

**Checkpoint:** Run alongside Claude Code. Watch the file list grow and re-sort as agents work.

---

## Acceptance Criteria

### Replay engine
- FileHistory built correctly from test fixtures
- Agent tool calls included and attributed
- HeatScore reflects session-wide activity
- ListFiles sorted by HeatScore descending
- WriteSnapshots bounded by config

### Stacked left pane
- Turn list and file list both visible and navigable
- Tab switches focus between them
- Turn list starts at bottom for completed sessions
- File list sorted by activity
- Left pane default 30% width

### File history
- Shows all operations chronologically with agent attribution
- Edit entries show inline diffs
- Navigable: j/k between entries, enter for full diff
- Esc returns to file list

### Live updates
- File list updates as MsgAgentToolCall arrives
- HeatScore and sorting update in real time
- New files appear when first touched

---

## Notes

- The replay engine replaces the per-render file activity computation in detail.go's `renderFileActivity`. That function currently rebuilds the file map on every render — the engine builds it once.
- `GetContentAt(path, promptIdx)` (snapshot reconstruction) is deferred — not needed for file history display since we show per-op diffs from OldStr/NewStr, not reconstructed snapshots.
- The stacked left pane is a significant layout change but follows the same navigation model (j/k/enter/esc). Tab for switching sub-panels is standard (lazygit uses tab for panel switching).
