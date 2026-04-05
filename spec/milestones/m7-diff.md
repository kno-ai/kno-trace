# M7: Diff View

**Prerequisites:** Read [spec/README.md](../README.md) (core spec) and `SCHEMA.md`. M6 must be complete (replay engine and heatmap working).

**Goal:** The before/after comparison — full codebase diff between any two points in the session.

**Deliverable:** "What changed between then and now?" answered in two keystrokes.

**Use cases served:** UC3 (what changed between then and now?)

---

## Scope

- `internal/ui/diff.go`:
  - From/to prompt selectors at top
  - On selection: enumerate all files with Write/Edit ops touched in either range (including agent file modifications — the replay engine already includes these from M6)
  - For each file: `replay.Engine.GetContentAt(path, from)` and `GetContentAt(path, to)` — these naturally incorporate agent modifications since the replay engine's file history includes agent tool calls
  - Render unified diff via `replay.diff.Compute()`
  - File headers (blue), hunk headers (purple), add lines (teal), del lines (red), context (muted)
  - Summary: `+N -M across K files`
  - If any prompt in the selected range contains a `ToolBash` call: show informational note "bash commands present in this range — file effects not captured" — this is factual, not classified
  - Files only after `from`: shown as fully added
  - Files only before `to` then absent: shown as fully deleted
  - Files with `ErrNoBaseline`: shown as "no baseline available for this file"
  - `d` key activates diff view
- `m` mark workflow (in timeline view — `m` for mark, following vim convention):
  - `m` on prompt P: marks P as `[A]`, shows `[A]` badge on that prompt item
  - `m` on a **different** prompt Q: marks Q as `[B]`, immediately switches to diff view with A->B selected
  - `esc` clears all marks and returns to timeline (from diff view or from a single `[A]` mark)
  - `m` on the **same** prompt as `[A]`: clears the mark (toggle behavior)

---

## Acceptance Criteria

- Diff view correct for all file changes between two selected prompts (including changes made by agents)
- Bash presence note shown when any `ToolBash` in selected range
- `m` mark workflow works as specified
- `[A]` badge visible on marked prompt
- `esc` clears marks predictably
- Files with no baseline show the stated error message
