# M7: File Intelligence — Replay Engine, Hot Files, Session Views

**Prerequisites:** M6 complete (unified navigation, inline diffs, detail drill-in working).

**Goal:** Answer "what happened to this file?" and "which files are getting thrashed?" The replay engine builds session-wide file histories. The stacked left pane shows hot files alongside turns. Session views provide high-level insights including a heatmap grid.

**Use cases served:** UC2 (what happened to this file?), UC6 (which files are getting thrashed?), UC7 (quickly check a past session)

---

## Build Order

### Layer 1: Replay engine (data only)

Build `internal/replay/engine.go`. No UI — just data structures and `--dump` output.

- **FileHistory per file**: collect all Write/Edit operations from `Prompt.ToolCalls` and `Prompt.Agents[*].ToolCalls` (recursing into nested agents). Reads are tracked for file discovery but don't contribute to HeatScore.
- **Each entry**: prompt index, timestamp, op type (W/E), agent label, tool call reference.
- **HeatScore**: count of write+edit operations on the file across the session. Higher = hotter.
- **`ListFiles()`**: returns all files sorted by HeatScore descending.
- **`GetFileHistory(path)`**: returns the complete history for one file.
- **`GetHeatmap()`**: returns the files × prompts matrix (see Layer 4 for grid design).
- Built once after full parse. Incrementally updated during live sessions when new tool calls arrive (add entries, re-sort).
- Wire into `rebuildSession` (after `EnrichSession`) and `handleAgentWatcherMsg` (incremental).

**Checkpoint:** `--dump` shows "Hot files" section with files sorted by edit count. Tests verify correctness against existing fixtures.

### Layer 2: Left pane improvements + file list

Refactor the left pane. Three changes in one layer since they're tightly coupled:

- **Default split**: 30% left pane (down from 40%).
- **Cursor at bottom**: completed sessions start with cursor on the last turn (most recent visible). Currently starts at top.
- **Stacked layout**: turn list (top ~60% of left height) + file list (bottom ~40%). File list shows files from the replay engine sorted by HeatScore. Each row: truncated path + edit count.
- **Tab** switches focus between turn list and file list. Active list has highlighted header. Inactive list is dimmed.
- **enter** from turn list → turn detail (existing). **enter** from file list → file history (Layer 3).
- **When no item has been selected yet** (first entering a session), the detail pane shows the session summary (Layer 3).
- **Live**: file list updates as `MsgAgentToolCall` arrives — new files appear, edit counts increment, list re-sorts.

**Checkpoint:** Open a session. Turns at top-left, hot files at bottom-left. Tab between them. Left pane is 30% width. Completed sessions show recent turns.

### Layer 3: Session summary + file history (detail modes)

Two new detail pane modes. Both use the existing drill-in/esc-back pattern. Live updates woven in.

**Session summary** (default — shown when no turn or file is selected):
- Session header: project name, model, duration, tokens, prompt count, context%
- Hot files: top N files by HeatScore with intensity bars (`████` proportional to max). Edit count per file.
- Agent summary: total spawned, parallel, failed
- Quick stats: total files touched, total edits
- Press `h` from here to switch to heatmap grid (Layer 4)
- Live: stats and hot files update as new data arrives

**File history** (shown when entering a file from the file list):
- File path header with total edit count and prompt range
- Chronological entries: every write/edit across the session
  - Per entry: `#N HH:MM  E  old → new` with inline mini-diff
  - Agent attribution: `(subagent-1)` suffix when from an agent
  - Write entries: `+N lines`
- j/k navigates entries, enter on an edit shows full diff
- esc returns to session summary
- Live: new entries append as the file is edited during the session

**Checkpoint:** Enter session → see summary with intensity bars. Tab to files → select one → see its history with diffs. Esc → back to summary.

### Layer 4: Heatmap grid

A 2D visualization of edit activity: files × prompts. Accessible via `h` from the session summary.

**Design:**

```
                    recent ←──────────── old
                    #57 #56 #55 #54 #53 #52 #51 #50
  detail.go          3   2   ·   1   2   1   ·   ·
  app.go             1   ·   1   ·   2   ·   ·   ·
  timeline.go        ·   1   ·   ·   1   ·   ·   ·
  builder.go         ·   ·   ·   1   ·   ·   ·   ·
  tree.go            ·   ·   ·   ·   ·   1   ·   ·
```

**Key design decisions:**

- **Columns = prompts, most recent on the LEFT.** For live sessions, new prompts appear on the left edge — you watch activity come in without scrolling. Old prompts scroll off to the right. This matches the natural reading direction for "what's happening now."
- **Cell value = edit count** (writes + edits in that prompt for that file). Not just presence/absence — a cell with `3` means three edits to that file in that prompt (possible thrashing).
- **Color intensity by count**: 0 = dim dot `·`, 1 = normal, 2+ = bright/bold, 5+ = red (thrashing indicator). Use the terminal's color capability — no Unicode blocks needed, just colored numbers.
- **Rows = files**, sorted by total HeatScore (most active at top). Top rows are the hot spots.
- **Only files with edits shown.** Read-only files are excluded — they're noise in the heatmap.
- **Row labels**: truncated file paths, right-aligned before the grid.
- **Column labels**: prompt indices at the top.

**Navigation:**
- **j/k** scrolls rows (files) when there are more than fit on screen
- **h/l** scrolls columns (prompts) for long sessions
- **enter** on a file row → file history for that file in the detail pane
- **esc** → back to session summary
- **g/G** → jump to top/bottom of file list

**Live behavior:**
- New prompts appear as columns on the LEFT edge
- Cells fill in as edits arrive via `MsgAgentToolCall`
- File rows may re-sort as HeatScore changes (file becomes hotter mid-session)
- The grid stays scrolled to show the leftmost (newest) columns — you watch it fill in

**Terminal rendering:**
- Numbers colored by intensity: `·` dim, `1` normal, `2` yellow, `3+` red
- Active row (cursor) highlighted with `>` prefix and brighter color
- Column width: 4 chars each (space for 2-digit counts + padding)
- File label column: ~25 chars (truncated paths)
- Fits ~12-15 prompt columns on an 80-char terminal

**Checkpoint:** From session summary, press `h`. See the grid with colored numbers. Spot a file with red cells (thrashing). Enter → see file history. Esc → back to grid. During live session, watch new columns appear on the left.

---

## Acceptance Criteria

### Replay engine
- FileHistory built correctly from test fixtures
- Agent tool calls included and attributed (including nested agents)
- HeatScore = total write+edit count per file
- ListFiles sorted by HeatScore descending
- GetHeatmap returns correct files × prompts matrix
- Incremental update works for live sessions

### Stacked left pane
- Turn list and file list both visible and navigable
- Tab switches focus with visual indicator
- Turn list cursor starts at bottom for completed sessions
- File list sorted by HeatScore, updates live
- Left pane default 30% width

### Session summary
- Default detail view when entering a session
- Hot files with intensity bars proportional to max HeatScore
- Esc from turn/file detail returns to session summary
- Stats update during live sessions

### File history
- All write/edit operations chronologically with agent attribution
- Edit entries show inline diffs
- j/k between entries, enter for full diff, esc back
- New entries appear live

### Heatmap grid
- 2D grid: files (rows) × prompts (columns, recent on left)
- Cell = edit count, colored by intensity
- j/k scrolls rows, h/l scrolls columns
- Enter on row → file history
- Live: new columns on left, cells fill as edits arrive
- Files re-sort as HeatScore changes

---

## Notes

- Reads are tracked for file discovery (knowing a file exists) but excluded from HeatScore and heatmap cells. Only writes and edits represent meaningful file mutations.
- The replay engine replaces `renderFileActivity` in detail.go — that function currently rebuilds per-turn file maps on every render. The engine builds once and updates incrementally.
- `GetContentAt(path, promptIdx)` (snapshot reconstruction) is deferred — not needed since we show per-op diffs from OldStr/NewStr.
- The heatmap's "recent on left" orientation is unusual but serves live sessions well. For completed sessions it means the most interesting activity (later prompts where things converge or thrash) is immediately visible.
