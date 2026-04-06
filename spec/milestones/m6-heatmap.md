# M6: Unified Navigation & Rich Detail

**Prerequisites:** M5 complete. Read [spec/ui-model.md](../ui-model.md) — this milestone implements that model.

**Goal:** Rebuild the UI around the hierarchical drill-down model. Sessions → Turns → Items → Detail. Every level uses the same navigation. The detail pane shows inline diffs, file activity, and live agent status — the content that makes kno-trace worth opening.

**Deliverable:** A developer opens kno-trace, sees their sessions, drills into one, sees turns with inline diffs and file churn, drills into an agent to see what it's doing. Everything is enter/esc/j/k. Live and historical look the same.

**Use cases served:** UC1, UC2, UC4, UC6, UC7

---

## Build Order

### Layer 1: Unified navigation stack

Refactor the UI from separate picker/timeline/detail into a single navigation stack.

- **Navigation stack model**: `navStack []NavLevel` where each level has a list and a selected index. Enter pushes, esc pops. The left pane renders the current level's list. The right pane renders detail for the focused item.
- **Sessions level**: replaces the picker. Flat list with j/k navigation, same style as turn list. Detail pane shows session summary card.
- **Turns level**: replaces the prompt list. Same data, same badges, but now navigated to via enter from session, exited via esc.
- **Breadcrumb**: top of the right pane. Shows current path: `Sessions > project-name > #14 > subagent-1`.
- **Remove**: `P` key for picker (esc does this now), separate picker view mode, `viewMode` enum.

**Checkpoint:** Open kno-trace. See session list. Enter → see turns. Esc → back to sessions. Breadcrumb updates. Navigation feels the same at every level.

### Layer 2: Replay engine & inline diffs

Build the replay engine and make diffs visible in the turn detail.

- **Replay engine** (`internal/replay/engine.go`): builds FileHistory per file from all tool calls (parent + agent), implements `GetContentAt()`, snapshot reconstruction.
- **Diff computation** (`internal/replay/diff.go`): `Compute(a, b string) []DiffHunk` via go-diff.
- **Inline diffs in turn detail**: Edit tool calls show old→new as a colored mini-diff right in the tool call list. No drilling required — the diff is there at a glance.
- **Write deltas**: Write ops show `+N lines` or `+N -M` if prior state available.

**Checkpoint:** Open a completed session with Edits. Navigate to a turn. See colored diffs inline. `--dump` also shows diffs.

### Layer 3: Items level & drill-in

Add Level 3 — the items list within a turn.

- **Enter on a turn** pushes the items level. Left pane becomes: tool calls + agents, flat list, chronological.
- **Detail for each item type**: Edit → full diff. Bash → command + output. Agent → summary with files/tool count. Read/Glob/Grep → path and info.
- **Enter on an agent** pushes Level 4 — the agent's items. Same layout.
- **File activity section**: at the bottom of the turn detail (Level 2), shows all files touched in this turn with op badges, agent attribution, session heat.
- **Enter on a file** in the activity section → file history: all ops across the session, with diffs at each step.

**Checkpoint:** Navigate: sessions → session → turn → Edit tool call → see full diff → esc → select agent → enter → see agent's tool calls → esc → esc → esc → back at sessions.

### Layer 4: Live behavior & auto-follow

Wire live sessions into the unified navigation.

- **Auto-follow**: when on the latest session's latest turn, cursor tracks new turns, detail auto-scrolls to bottom. `G` re-engages after manual browsing.
- **Running agent inline activity**: collapsed agents show latest tool call: `⬡ subagent-1 — running 14s → Edit app.go`
- **Completed agent summary**: `⬡ subagent-1 — done 25s — 4 files, 2 edits`
- **File activity updates live**: new tool calls from agents update the files section in real time.
- **Items level live updates**: when viewing a turn's items during a live session, new tool calls append to the bottom of the list.

**Checkpoint:** Run alongside Claude Code. Watch turns appear, agents show live activity, diffs appear inline as edits complete. Navigate away, come back with `G`, auto-follow resumes.

### Layer 5: Selection & comparison

Add spacebar selection and comparison views.

- **Spacebar** toggles selection on focused item. Visual indicator on selected items. Status bar shows count.
- **`c` key** opens comparison for selected items in the detail pane.
- **Two turns selected**: file diff between the two points. Shows all files changed, with unified diffs.
- **Esc** from comparison returns to single-item detail. Spacebar deselects.

**Checkpoint:** Navigate to turns. Space on #3, space on #8. Press `c`. See diff of all files changed between turns 3 and 8. Esc back.

---

## Acceptance Criteria

### Navigation
- Sessions → Turns → Items → Agent Items: all navigable with enter/esc
- j/k works identically at every level
- Breadcrumb updates correctly at every level
- Esc from sessions quits
- Back navigation preserves cursor position at each level

### Replay & diffs
- Inline diffs render correctly for Edit ops (colored add/del)
- Write deltas show correct line counts
- `GetContentAt()` correct for test fixtures
- Agent edits included in replay engine
- Long diffs truncated with "... N more"

### Detail content
- Turn detail shows: header, human text, warnings, tool calls with inline diffs, agent summaries, file activity section
- Item detail shows: full diff for Edits, command+output for Bash, summary for Agents
- File history shows chronological ops with diffs

### Live
- Auto-follow tracks latest turn and scrolls detail
- Running agents show latest tool call inline
- File activity updates in real time
- `G` re-engages auto-follow

### Selection
- Spacebar marks items, visual indicator shown
- `c` opens comparison view for selected turns
- Diff between two turns correct
- Esc returns to single-item detail

---

## Notes

- This replaces the old separate-view design (picker, timeline, swimlane, heatmap, diff). Everything is one drill-down tree with contextual detail.
- The session list replaces the picker. It uses the same list style and navigation. No special styling for sessions.
- File history (UC2) is accessible by drilling into a file from the turn's file activity section. No separate heatmap view.
- Session-scoped diff (old M7) is folded into this milestone via the selection/comparison feature.
- The `m` mark workflow is replaced by spacebar selection + `c` to compare.
