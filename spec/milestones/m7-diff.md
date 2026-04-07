# M7: File Intelligence — Replay Engine, Hot Files, Session Views

**Prerequisites:** M6 complete (unified navigation, inline diffs, detail drill-in working).

**Goal:** Answer "what happened to this file?" and "which files are getting thrashed?" The replay engine builds session-wide file histories. The stacked left pane shows hot files alongside turns. Session-level views (summary, heatmap) provide high-level insights.

**Deliverable:** A developer enters a session and immediately sees a summary with hot files and intensity bars. They can switch to a heatmap grid (files × prompts) to spot patterns. They select a file to see its complete story, or a turn to see what happened.

**Use cases served:** UC2 (what happened to this file?), UC6 (which files are getting thrashed?), UC7 (quickly check a past session)

---

## Navigation Model

The detail pane has three top-level modes based on what's selected:

1. **Session view** (default) — shown when no turn or file is selected. This is the landing page for a session. May have multiple sub-views (summary, heatmap) navigable with tab or keys.
2. **Turn detail** — shown when a turn is selected from the turn list.
3. **File history** — shown when a file is selected from the file list.

Esc from turn detail or file history → back to session view. The session view is the "home" of a session.

---

## Build Order

### Layer 1: Replay engine

Build `internal/replay/engine.go` — the foundation for all file intelligence.

- **FileHistory per file**: collect all Write/Edit/Read operations from `Prompt.ToolCalls` and `Prompt.Agents[*].ToolCalls` (recursing into nested agents), interleaved by timestamp.
- **Agent attribution**: each entry tracks the source agent label (empty = parent).
- **PromptIdx per entry**: which prompt this operation happened in — needed for the heatmap grid.
- **HeatScore**: count of distinct prompts with Write or Edit ops on the file.
- **Session-wide**: built once after full parse, updated incrementally during live sessions.
- **`ListFiles()`**: returns all files sorted by HeatScore descending.
- **`GetFileHistory(path)`**: returns the FileHistory for a file.
- **`GetHeatmap()`**: returns a structure suitable for rendering the 2D grid (files × prompts).
- Bounded: WriteSnapshots capped per config.

**Checkpoint:** `--dump` output shows a "Hot files" section listing files by activity with per-prompt touch indicators.

### Layer 2: Stacked left pane — turns + files

Split the left pane vertically: turn list on top, file list on bottom.

- **Turn list**: compact, cursor starts at bottom for completed sessions (most recent visible).
- **File list**: all files touched in the session, sorted by HeatScore. Path + op badges (W×N E×M).
- **Tab** switches focus between the two lists. Active list has distinct header.
- **j/k** navigates within the focused list.
- **enter** from turn list → turn detail in detail pane. **enter** from file list → file history in detail pane.
- **Default split**: left pane 30% width. Turn list 60% of left height, files 40%.
- When neither list has an item selected (first entering a session), the detail pane shows the session view.

**Checkpoint:** Open a session. See turns at top-left, hot files at bottom-left. Tab between them. Detail shows session summary by default.

### Layer 3: Session summary view

The default detail pane when entering a session (before selecting a turn or file).

- **Session header**: project name, model, duration, total tokens, prompt count, context%
- **Hot files section**: top N files by HeatScore with intensity bars (█ blocks proportional to max). Op badges and agent attribution.
- **Agent summary**: total spawned, parallel, failed
- **Quick stats**: total files touched, total edits, total writes

This is the "dashboard glance" — you see the shape of the session immediately.

**Checkpoint:** Enter a session. See the summary with intensity bars. Select a turn → detail changes. Esc → back to summary.

### Layer 4: File history view

When selecting a file from the file list, the detail pane shows:

- **File path header** with total op count, HeatScore, prompt range
- **Chronological entry list**: every operation across the session
  - Per entry: turn index, timestamp, op type (W/E/R), agent label (if from agent)
  - Edit entries: inline mini-diff
  - Write entries: line count delta
- **Navigable**: j/k moves between entries, enter on an Edit shows full diff
- **esc** returns to session view

**Checkpoint:** Select a hot file. See its complete history with diffs. Drill into an edit. Esc back.

### Layer 5: Heatmap grid (files × prompts)

A 2D grid view accessible from the session view (e.g., `h` key or tab within session view):

```
          #1  #2  #3  #4  #5  #6  #7  #8
app.go     ·   E   ·   E   E   ·   E   W
detail.go  ·   ·   W   E   E   E   E   E
tree.go    ·   ·   ·   ·   E   ·   ·   ·
config.go  W   ·   ·   ·   ·   ·   ·   ·
```

- Rows = files (sorted by total activity)
- Columns = prompts (chronological)
- Cells = operation type (W/E/R/· for none)
- Patterns visible: files thrashed across many prompts (long rows of activity), recent focus (right-heavy), setup files (left-only)
- Inspired by [kno-lens](https://marketplace.visualstudio.com/items?itemName=kno-ai.kno-lens) file heatmap
- Scrollable for large sessions
- Enter on a cell → jump to that turn's detail for that file

**Checkpoint:** Enter a session. See summary. Press `h` → see heatmap grid. Spot a thrashed file. Enter on a cell → see the edit.

### Layer 6: Live updates

During live sessions:
- File list updates as new tool calls arrive (new files appear, heat re-sorts)
- Session summary hot files update in real time
- Heatmap grid adds columns as new prompts seal

---

## Session-Level Views — Extensibility

The session view is a container for session-level insights. M7 builds two:
1. **Summary** (default) — stats + hot files with intensity bars
2. **Heatmap** — 2D files × prompts grid

Future session views could include:
- Token burn rate over time
- Context% trajectory
- Agent activity timeline
- Cost analysis

These would be additional tabs/modes within the session view, all accessible before drilling into any specific turn or file.

---

## Acceptance Criteria

### Replay engine
- FileHistory built correctly from test fixtures
- Agent tool calls included and attributed (including nested agents)
- HeatScore reflects session-wide write+edit activity
- ListFiles sorted by HeatScore descending
- GetHeatmap returns correct files × prompts matrix

### Stacked left pane
- Turn list and file list both visible and navigable
- Tab switches focus between them
- Turn list starts at bottom for completed sessions
- File list sorted by HeatScore
- Left pane default 30% width

### Session summary
- Shows on first entering a session (default detail view)
- Hot files with intensity bars proportional to max HeatScore
- Esc from turn/file detail returns to session summary

### File history
- Shows all operations chronologically with agent attribution
- Edit entries show inline diffs
- j/k between entries, enter for full diff, esc back

### Heatmap
- 2D grid renders correctly (files × prompts)
- Operation type shown per cell
- Scrollable for large sessions
- Enter on cell navigates to relevant detail

### Live updates
- File list updates as MsgAgentToolCall arrives
- Summary and heatmap update as new prompts seal
