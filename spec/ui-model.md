# UI Model: Hierarchical Drill-Down

This document defines the unified interaction model for kno-trace. All navigation follows the same pattern at every level. There are no separate "views" — only depth in a tree.

---

## Core Principles

1. **One interaction model everywhere.** j/k to navigate, enter to drill in, esc to back out. No exceptions. No special keys for special contexts.

2. **Two panes, always.** Left pane = list of items at the current level. Right pane = detail for the focused item. Breadcrumb at top of the right pane shows where you are.

3. **Live and historical are the same UI.** Live sessions auto-follow (cursor tracks the latest item, detail auto-scrolls). Manual navigation disengages auto-follow. New data streams into the same layout — no mode switch, no different rendering.

4. **Selection enables comparison.** Spacebar marks items. A dedicated key opens the comparison view for marked items. The detail pane shifts from "single item detail" to "comparison across marked items."

5. **The detail pane shows what matters without drilling in.** Diffs inline, file churn visible, agent activity visible. Drilling in adds depth, but the summary is useful at a glance.

---

## The Hierarchy

```
Sessions                              ← left pane: list of sessions
  └── Session                         ← left pane: list of turns (prompts)
       └── Turn                       ← left pane: list of items in this turn
            ├── Tool Call (Edit)      ← detail: inline diff
            ├── Tool Call (Bash)      ← detail: command + output
            ├── Tool Call (Read)      ← detail: file path, content excerpt
            ├── Tool Call (Glob/Grep) ← detail: pattern, results
            ├── ⬡ Agent              ← enter drills in → agent's items
            │    ├── Tool Call        ← same as parent tool calls
            │    └── ⬡ Nested Agent  ← enter drills deeper
            └── [Files summary]       ← summary of all files touched in turn
```

### Level 1: Sessions

The left pane shows all discovered sessions as a flat list (most recent first). Each row shows: project name, start time, duration, size. Same j/k/enter/esc navigation as every other level.

The right pane shows the session summary card: project name, model, duration, prompt count, token totals, context%, file count.

Esc from this level = quit.

### Level 2: Turns (Prompts)

The left pane shows all prompts in the session. Each row shows: index, truncated human text, badges (tool count, agent count, context%, warnings). Breadcrumb: `Sessions > project-name`.

The right pane shows the turn detail: header (time, duration, model, tokens), human text, warnings, then a **rendered summary** of the turn's activity — tool calls with inline diffs for Edits, agent summaries with latest tool call, file activity section at the bottom.

### Level 3: Turn Items

Enter on a turn drills into its items. The left pane becomes a flat list of:
- Tool calls (type icon + path/command)
- Agents (⬡ label + status)

Each item shows a one-line summary. The right pane shows full detail for the focused item:
- **Edit**: the diff (old → new), colored
- **Write**: path, line count, content excerpt or diff against prior state
- **Bash**: full command, exit code, output (truncated)
- **Read**: path, content excerpt
- **Glob/Grep**: pattern, result count
- **Agent**: task description, model, status, duration, tokens, files touched with W/R/E counts, tool call count

Breadcrumb: `Sessions > project-name > #14`

### Level 4: Agent Items

Enter on an agent drills into its items — same layout as Level 3 but scoped to that agent's tool calls. Breadcrumb: `Sessions > project-name > #14 > subagent-1`

Nested agents appear in the list and can be drilled into further.

---

## Navigation

| Key | Action | Notes |
|-----|--------|-------|
| `j` / `↓` | Move down in list | Always moves within the current level's list |
| `k` / `↑` | Move up in list | |
| `g` | Jump to top of list | |
| `G` | Jump to bottom of list | |
| `enter` | Drill into focused item | Pushes a new level onto the navigation stack |
| `esc` | Back up one level | Pops the navigation stack. At sessions level = quit |
| `space` | Toggle selection on focused item | Marked items shown with a selection indicator |
| `c` | Compare selected items | Opens comparison view in detail pane |
| `/` | Search/filter current list | Filters by text content, file path, tool type |
| `[` / `]` | Resize left/right panes | 5% increments, clamped 15-85% |
| `?` | Help overlay | Shows keybindings for current context |
| `q` | Quit | Always quits, from any level |

---

## Selection and Comparison

Spacebar marks items at the current level. Marked items show a visual indicator (e.g., `●` prefix or highlight). The status bar shows the count: `2 selected`.

Pressing `c` with selections opens a comparison view in the detail pane:

| Level | Selection | Comparison view |
|-------|-----------|----------------|
| Sessions | 2+ sessions | Token totals, duration, prompt counts side by side |
| Turns | 2 turns | File diff between the two points (replaces M7's `m` mark) |
| Turns | 3+ turns | File changes across the selected range |
| Items | 2+ tool calls | Side-by-side diffs (if applicable) |

Esc from a comparison view returns to single-item detail. Spacebar on an already-selected item deselects it.

---

## Live Behavior

Live sessions are the same UI with auto-follow:

- **Auto-follow on by default** for the latest session's latest turn
- Cursor stays on the latest turn, detail auto-scrolls to show new content at the bottom
- New tool calls and agent activity appear in the detail pane as they arrive
- New turns appear in the list and the cursor moves to them automatically
- **Manual navigation disengages auto-follow**: pressing j/k or selecting a different turn stops auto-follow. The user is now browsing history while the session continues.
- **Re-engaging auto-follow**: pressing `G` (go to bottom) re-engages auto-follow

When viewing a live turn's items:
- Running agents show their latest tool call: `⬡ subagent-1 (Explore) — running 14s → Edit app.go`
- New tool calls append to the bottom of the list
- The detail pane for a running agent updates as tool calls arrive
- Completed agents show summary: `⬡ subagent-1 — done 25s — 4 files, 2 edits`

---

## Detail Pane Content

### Session detail (Level 1)
- Project name, model, start/end time, duration
- Prompt count, total tokens (in/out), file count
- Context% of most recent prompt
- Agent summary: total agents spawned, parallel count, failures

### Turn detail (Level 2) — the main view
This is what users see most. It must be useful at a glance.

**Header**: index, time range, duration, model, tokens in/out, context%

**Human text**: the user's prompt (truncated, full on scroll)

**Warnings**: loop detected, context high/critical, agent conflicts, interrupted

**Tool calls** (inline, scrollable):
- Edit: path + inline colored diff (3-4 lines max, `... N more` if longer)
- Write: path + line count delta, `+N lines`
- Bash: truncated command, exit code if non-zero
- Read: path (dimmed — reads are low-signal)
- Glob/Grep: pattern (dimmed)
- Agent: collapsed summary with latest tool call if running

**File activity** (bottom section):
- All files touched in this turn (parent + agents), sorted by heat
- Per file: path, ops (W×N R×M E×K), agent attribution, session-wide heat indicator
- Files touched by multiple agents highlighted

### Tool call detail (Level 3)
- **Edit**: full diff with context lines, colored add/del
- **Write**: full content or diff against prior state if available
- **Bash**: full command, full output (scrollable)
- **Read**: file path, content excerpt
- **Agent**: task description, task prompt, model, status, duration, tokens in/out, files touched with per-file W/R/E counts, tool call count

### File history (drill into a file from the activity section)
- Complete chronological history of all operations on this file across the session
- Per entry: prompt index, op type, agent label, delta, mini-diff
- Scrollable, shows the full story of the file

---

## Sorting and Scrolling

### Left pane
- **Sessions**: most recent first (by end time)
- **Turns**: chronological (prompt index)
- **Items**: chronological within the turn (tool call order)
- Scrolling: list auto-scrolls to keep cursor visible. During auto-follow, cursor moves to new items.

### Right pane
- Content scrolls independently with `h`/`l` (or left/right arrows)
- During auto-follow, auto-scrolls to bottom so new content is visible
- Manual scroll disengages detail auto-scroll (but not list auto-follow)
- `g` in detail context scrolls detail to top

---

## Status Bar

Always at the bottom. Shows contextual information:

- **Live indicator**: `●` dot when session is live
- **Breadcrumb echo**: current path in the hierarchy
- **Selection count**: `2 selected` when items are marked
- **Key hints**: context-appropriate, showing available actions
- **Stats**: prompt count, token count, context%, agent activity (when relevant)
