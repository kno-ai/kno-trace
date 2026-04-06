# M6: Unified Navigation & Rich Detail

**Prerequisites:** M5 complete. Read [spec/ui-model.md](../ui-model.md) — this milestone implements that model.

**Goal:** Rebuild the UI around the stable-list / deep-detail model. Left pane is the navigational anchor (sessions or turns). Right pane is the drill-down space with inline diffs, file intelligence, and live agent activity. This is where kno-trace stops showing the same data as Claude Code and starts adding genuine value.

**Deliverable:** A developer opens kno-trace, sees their sessions, enters one, sees turns with inline diffs and file churn at a glance. They drill into an Edit to see the full diff, into an agent to see what it touched, into a file to see its complete history. Live sessions show agents working in real time. Everything is j/k/enter/esc.

**Use cases served:** UC1, UC2, UC4, UC6, UC7

---

## Build Order

### Layer 1: Navigation refactor — sessions and turns

Refactor from separate picker/timeline into the two-context left pane model.

- **Sessions list** replaces the picker. Flat list, same visual style as the turns list. j/k to navigate, enter to open, esc to quit. Right pane shows session summary card.
- **Turns list** replaces the prompt list. Enter on a session transitions the left pane to turns. Esc from turns goes back to sessions.
- **Detail pane focus**: enter on a turn gives focus to the detail pane. j/k now navigate items within the detail. Esc returns focus to the left pane (turns list). This is the key UX change — the detail pane becomes an interactive space, not just a display.
- **Breadcrumb**: top of right pane. Shows `#14` at turn level, `#14 > subagent-1` when drilled in.
- **Remove**: `P` key, separate `viewMode` enum, separate picker model. Everything is one navigation model.

**Checkpoint:** Open kno-trace. See session list. Enter → turns. j/k moves turns, detail updates. Enter → detail gets focus, j/k moves between tool calls. Esc → back to turn navigation. Esc → back to sessions.

### Layer 2: Replay engine & inline diffs

Build the replay engine and make diffs visible at a glance.

- **Replay engine** (`internal/replay/engine.go`): builds FileHistory per file from all tool calls (parent + agent), implements `GetContentAt()`.
- **Diff computation** (`internal/replay/diff.go`): `Compute(a, b string) []DiffHunk` via go-diff.
- **Inline diffs in turn detail**: Edit tool calls show old→new as colored mini-diff (3-4 lines, truncated). Visible without drilling in.
- **Write deltas**: `+N lines` or `+N -M` if prior state known.

**Checkpoint:** Open a session with Edits. See colored diffs inline in the turn detail. `--dump` also shows diffs.

### Layer 3: Drill-in views

Make items in the detail pane drillable.

- **Enter on Edit** → full diff with context lines. Breadcrumb: `#14 > Edit app.go`.
- **Enter on Bash** → full command + output.
- **Enter on Agent** → agent detail: description, model, status, files with W/R/E counts, tool call list with inline diffs.
- **File activity section** at the bottom of turn detail: all files touched in this turn, sorted by session heat, with op badges and agent attribution.
- **Enter on a file** → file history: every op on this file across the entire session, with diffs.
- Nested agents drillable to arbitrary depth.

**Checkpoint:** Full navigation loop: sessions → session → turn → enter detail → Edit → see diff → esc → agent → see tools → esc → file → see history → esc → esc → turn list → esc → sessions.

### Layer 4: Live behavior

Wire live sessions into the model.

- **Auto-follow**: cursor tracks latest turn, detail auto-scrolls to bottom. `G` re-engages after manual browsing.
- **Running agents inline**: `⬡ subagent-1 — running 14s → Edit app.go` (shows latest tool call without drilling in).
- **Completed agents inline**: `⬡ subagent-1 — done 25s — 4 files, 2 edits`.
- **File activity updates live**: new files appear as agents touch them.
- **Detail items update live**: when focused on a turn in the detail pane during a live session, new tool calls append at the bottom.

**Checkpoint:** Run alongside Claude Code. Watch turns appear, agents show live activity, diffs appear inline. Navigate away, come back with `G`, auto-follow resumes.

### Layer 5: Selection & comparison

Add spacebar selection and comparison views.

- **Spacebar** toggles selection in the left pane. Visual indicator. Status bar shows count.
- **`c`** opens comparison in detail pane for selected items.
- **Two turns selected** → file diff between the two points. All files changed, unified diffs.
- **Esc** from comparison returns to single-item detail.

**Checkpoint:** Select turns #3 and #8. Press `c`. See all files changed between them with diffs. Esc back.

---

## Acceptance Criteria

### Navigation
- Sessions and turns use identical list style and navigation
- Enter on session → turns. Esc from turns → sessions. Esc from sessions → quit.
- Enter on turn → detail gets focus. j/k navigates items in detail. Esc → turn focus.
- Breadcrumb correct at every depth level
- Cursor position preserved when backing out at every level

### Replay & diffs
- Inline diffs for Edit ops: colored add/del, truncated at 3-4 lines
- Write deltas show correct counts
- `GetContentAt()` correct for test fixtures
- Agent edits included in file history
- `WarnReplayGap` on unresolvable Edit, no crash

### Drill-in
- Enter on Edit → full diff. Enter on Bash → output. Enter on Agent → detail.
- Enter on file → session-wide file history with diffs
- Esc always returns to previous level with cursor preserved
- Nested agent drill-in works to arbitrary depth

### Live
- Auto-follow tracks latest turn and scrolls detail to bottom
- Running agents show latest tool call inline
- `G` re-engages auto-follow after manual navigation
- File activity section updates as agent tool calls arrive

### Selection
- Spacebar marks turns, visual indicator shown
- `c` shows diff between two selected turns
- Esc returns to single-item detail
- Spacebar deselects

---

## Notes

- The left pane has exactly two states: sessions list and turns list. It never shows tool calls, agents, or files — those are all in the detail pane's drill-down.
- The detail pane's focus model (enter to activate, esc to deactivate) is like lazygit's panel switching. The status bar key hints should update to reflect which pane has focus.
- File history (UC2) is accessible by drilling into a file from the turn's file activity section. No separate heatmap view needed.
- Session-scoped diff (old M7) is the selection/comparison feature in Layer 5.
