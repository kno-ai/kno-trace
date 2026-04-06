# UI Model: Stable List, Deep Detail

This document defines the interaction model for kno-trace. The left pane is a stable navigational anchor. The right pane is the drill-down space.

---

## Core Principles

1. **Left pane = stable list. Right pane = drill-down detail.** The left pane shows the primary list for the current context (sessions or turns). It doesn't change on enter — it's always there as an anchor. The right pane handles all depth: tool calls, diffs, agents, file history.

2. **One interaction model in the detail pane.** j/k to navigate items within the detail, enter to drill deeper, esc to back out. Breadcrumb at the top shows where you are.

3. **Two left-pane contexts.** Sessions list (top level) and turns list (inside a session). Enter on a session switches the left pane to turns. Esc from turns switches back to sessions. This is the only left-pane transition — like opening a folder.

4. **Live and historical are the same UI.** Auto-follow keeps you on the latest turn with detail scrolled to the bottom. New data streams in. Manual navigation disengages auto-follow. No mode switch.

5. **Selection enables comparison.** Spacebar marks items in the left pane. A dedicated key opens the comparison view in the detail pane.

6. **The detail pane shows what matters at a glance.** Diffs inline, file churn visible, agent activity visible. Drilling in adds depth, but the summary is useful without interaction.

---

## Layout

```
┌─────────────────────┬────────────────────────────────────────┐
│                     │ Breadcrumb: #14 > subagent-1           │
│   Left Pane         │                                        │
│   (stable list)     │   Right Pane (detail / drill-down)     │
│                     │                                        │
│   Sessions          │   Content changes based on:            │
│     or              │   - What's focused in the left pane    │
│   Turns             │   - How deep you've drilled in         │
│                     │                                        │
│                     │                                        │
├─────────────────────┴────────────────────────────────────────┤
│ status bar: keys, stats, selection count                     │
└──────────────────────────────────────────────────────────────┘
```

---

## Left Pane States

### Sessions List

The first thing you see. Flat list of all discovered sessions, most recent first. Each row: project name, start time, duration, size. j/k to navigate, enter to open a session (switches left pane to turns), esc to quit.

Style matches the turns list — same row format, same navigation, same visual weight.

### Turns List

After entering a session. Shows all prompts with badges (tool count, agent count, context%, warnings). j/k to navigate turns. The right pane shows detail for the focused turn. Esc goes back to sessions list.

This list is **stable** — it never changes based on what you do in the detail pane. It's always there as your navigational anchor. You always know which turn you're looking at.

---

## Right Pane: The Detail Stack

The right pane shows detail for whatever's focused in the left pane. Within the detail, you can drill deeper using enter/esc. The breadcrumb at the top shows your current depth.

### Level 0: Session Summary (when sessions list is showing)

The right pane shows a summary card for the focused session:
- Project name, model, start/end time, duration
- Prompt count, total tokens, file count
- Context% of most recent prompt
- Agent summary: total spawned, parallel, failures

### Level 1: Turn Detail (default when inside a session)

The most important view — what you see most. Shows everything about the focused turn at a glance:

**Header**: index, time range, duration, model, tokens in/out, context%

**Human text**: the user's prompt

**Warnings**: loop detected, context high/critical, agent conflicts, interrupted

**Tool calls** (scrollable within the detail pane):
- Edit: path + inline colored diff (3-4 lines, `... N more` if longer)
- Write: path + `+N lines` or `+N -M` delta
- Bash: truncated command, exit code if non-zero
- Read/Glob/Grep: path or pattern (dimmed — low signal)
- Agent: collapsed summary line (see below)

**Agent summaries** (within the tool call list, at their chronological position):
- Running: `⬡ subagent-1 (Explore) — running 14s → Edit app.go`
- Succeeded: `⬡ subagent-1 (Explore) — done 25s — 4 files, 2 edits`
- Failed: `⬡ subagent-1 — ✗ failed 5s`

**File activity** (bottom section):
- All files touched in this turn (parent + agents), sorted by session-wide heat
- Per file: path, op badges (W×N R×M E×K), agent attribution
- Conflict warnings for files touched by multiple agents

Items in the detail pane are navigable: j/k moves between items (tool calls, agents, files) when the detail has focus. Enter drills into the focused item. The detail pane switches between "browsing the turn summary" and "drilled into an item."

### Level 2: Drilled-In Views (enter on an item)

Breadcrumb updates: `#14 > Edit app.go` or `#14 > subagent-1`

**Enter on Edit tool call** → full diff with context lines, colored add/del. Scrollable.

**Enter on Write tool call** → full content, or diff against prior state if available.

**Enter on Bash tool call** → full command, exit code, full output (scrollable).

**Enter on Agent** → agent detail:
- Task description, task prompt text
- Model, status, duration, tokens in/out
- Files touched with per-file W/R/E counts
- Tool call list (same rendering as turn-level tool calls, including inline diffs for Edits)
- Nested agents listed at the bottom

**Enter on a file** (from the file activity section) → file history:
- Complete chronological list of all operations on this file across the entire session
- Per entry: turn index, op type, agent label (if from agent), delta, mini-diff for Edits
- Scrollable

### Level 3: Deeper Drill-In

**Enter on a tool call within an agent** → same as Level 2 tool call detail.

**Enter on a nested agent** → same as Level 2 agent detail, breadcrumb extends.

Esc always pops one level. The breadcrumb shortens. You land back where you were.

---

## Navigation

### Left Pane Keys (always active)

| Key | Action |
|-----|--------|
| `j` / `↓` | Move down in list |
| `k` / `↑` | Move up in list |
| `g` | Jump to top (re-engage auto-follow at turns level) |
| `G` | Jump to bottom (re-engage auto-follow at turns level) |
| `enter` | Sessions: open session. Turns: give focus to detail pane for drill-in. |
| `esc` | Turns → sessions. Sessions → quit. |
| `space` | Toggle selection on focused item |
| `c` | Compare selected items (opens comparison in detail) |
| `/` | Search/filter current list |
| `[` / `]` | Resize left/right panes |
| `?` | Help overlay |
| `q` | Quit |

### Detail Pane Keys (when detail has focus after enter)

| Key | Action |
|-----|--------|
| `j` / `↓` | Move to next item in detail (next tool call, next agent, next file) |
| `k` / `↑` | Move to previous item |
| `enter` | Drill into focused item (diff, agent detail, file history) |
| `esc` | Back up one level. At Level 1: return focus to left pane. |
| `h` / `←` | Scroll detail content up |
| `l` / `→` | Scroll detail content down |

The transition between left-pane focus and detail-pane focus is the key UX moment. When you press enter on a turn in the left pane, the detail pane gains focus — items within it become navigable with j/k. Esc returns focus to the left pane. This is like lazygit's panel switching.

---

## Selection and Comparison

Spacebar marks items in the left pane. Visual indicator (●) on marked items. Status bar shows count.

`c` opens a comparison view in the detail pane:

| Selected | Comparison shows |
|----------|-----------------|
| 2 sessions | Token totals, duration, prompt counts side by side |
| 2 turns | File diff between the two points — all files changed with unified diffs |
| 3+ turns | Changes across the selected range |

Esc from comparison returns to single-item detail. Spacebar deselects.

---

## Live Behavior

**Auto-follow** is on by default for the most recent session. The left pane cursor tracks the latest turn. The detail pane auto-scrolls to the bottom so new content is visible as it arrives.

**Manual navigation disengages auto-follow.** Pressing j/k to move to a different turn means you're browsing history. The session continues updating in the background — new turns appear in the list, you just don't jump to them.

**`G` re-engages auto-follow.** Jump to the bottom of the turn list and resume tracking.

**What updates live in the detail pane:**
- New tool calls appear at the bottom of the tool call list
- Running agents update their status line: `running 14s → Edit app.go`
- Agent completion changes the summary: `done 25s — 4 files`
- File activity section updates as new files are touched
- Conflict warnings appear immediately when two agents touch the same file

**The experience is the same as historical** — you see the same layout, same content. Live just means new lines appear at the bottom and agent status lines pulse. No mode switch.

---

## Sorting and Scrolling

### Left Pane
- **Sessions**: most recent first (by end time)
- **Turns**: chronological (prompt index)
- List auto-scrolls to keep cursor visible
- During auto-follow, cursor moves to new items automatically

### Right Pane
- **Turn detail**: content rendered top-to-bottom: header, text, warnings, tool calls, agents, files
- **Scrolls independently** with h/l
- **During auto-follow**: auto-scrolls to bottom so new content is visible
- **Manual scroll** does not disengage auto-follow for the left pane (they're independent)

---

## Status Bar

Always at the bottom. Shows:
- **Live indicator**: `●` when session is live and auto-following
- **Location echo**: current session name, turn count
- **Selection count**: `2 selected` when items are marked
- **Key hints**: context-appropriate (show `enter` when items are drillable, `esc` when drill-in is active, `space` when items are selectable)
- **Stats**: token count, context%, agent activity
