# M7: Session-Scoped Diff

**Prerequisites:** M6 complete (replay engine, inline diffs, file history working).

**Goal:** "What changed between then and now?" — full codebase diff between any two prompts.

**Deliverable:** Two-keystroke comparison of all file states between any two points in the session.

**Use cases served:** UC3 (what changed between then and now?)

---

## Scope

- `m` mark workflow in the timeline:
  - `m` on prompt P: marks P as `[A]`, badge visible in prompt list
  - `m` on a different prompt Q: marks Q as `[B]`, opens diff in detail pane
  - `m` on the same prompt as `[A]`: clears the mark (toggle)
  - `esc` clears marks
- Diff renders in the detail pane (not a separate view):
  - Uses `replay.GetContentAt(path, fromIdx)` and `GetContentAt(path, toIdx)`
  - Lists all files changed between the two prompts (including agent modifications)
  - Per-file unified diff with add/del/context coloring
  - Summary: `+N -M across K files`
  - Bash presence note when applicable
  - Enter on a file → full diff for that file. Esc back to summary.

---

## Acceptance Criteria

- Diff correct between any two prompts (including agent changes)
- `[A]`/`[B]` badges visible in prompt list
- Mark workflow predictable (toggle, clear on esc)
- Files with no baseline show clear message
- Bash presence note when relevant
