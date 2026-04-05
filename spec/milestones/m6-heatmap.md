# M6: File Intelligence & Heatmap

**Prerequisites:** Read [spec/README.md](../README.md) (core spec) and `SCHEMA.md`. M5 must be complete (agent tree working, subagent files parsed).

**Goal:** The hot spot detector — which files are getting the most attention, and the complete change history of any file one keypress away. Includes file modifications from both the parent session and all agents.

**Deliverable:** Heatmap reveals session hot spots. File history shows the complete story of any file — including changes made by agents.

**Use cases served:** UC2 (what happened to this file?), UC6 (which files are getting thrashed?)

**Control room role:** The heatmap is the "hot spot detector" — it surfaces which files are getting the most attention. During a live session, it updates as new prompts seal, so the user can glance at it to see if Claude is focused or thrashing. Because it includes agent file modifications, it gives a truthful picture of everything that changed — not just what the parent session touched directly.

---

## Scope

- `internal/replay/engine.go`:
  - Builds `FileHistory` per file after full parse
  - **Includes agent tool calls:** When building the file history, collect file-modifying tool calls (Write, Edit, Read) from both `Prompt.ToolCalls` (parent) and `Prompt.Agents[*].ToolCalls` (agents, including nested agents). All operations are interleaved by timestamp to produce a single chronological history per file. Agent tool calls have timestamps from the subagent JSONL — use these for ordering.
  - **Agent attribution on file history entries:** Each entry in `FileHistory.PromptIdxs` / `FileHistory.Ops` should also track the source agent (empty string = parent). This allows the file history panel and heatmap to show "who" made each change: `#3 Edit internal/parser/jsonl.go (subagent-1)`.
  - `GetContentAt(path string, promptIdx int) (string, error)` — implements Snapshot Reconstruction Algorithm exactly as specified in the core spec Data Model section. Because agent Write/Edit ops are now included in the timeline, the reconstruction naturally incorporates agent modifications. An agent Write becomes a snapshot in `WriteSnapshots`; an agent Edit applies `old_str/new_str` in sequence. This also **fixes replay accuracy** for parent Edits that follow agent Writes — without agent data, the parent's `old_str` wouldn't match the pre-agent content, producing spurious `WarnReplayGap`.
  - `GetFileHistory(path string) *model.FileHistory`
  - `ListFiles() []string` sorted by HeatScore descending
- `internal/replay/diff.go` — `Compute(a, b string) []DiffHunk` via go-diff
- `internal/ui/components/minidiff.go` — renders diff hunks:
  - Max 8 context lines; `... N more lines` beyond that
  - Teal for additions, red for deletions, muted for context
- Inline mini-diffs in timeline detail pane (upgrade to M3/M4 rendering):
  - Edit ops: always show old_str/new_str as mini unified diff inline
  - Write ops where prior state exists: collapsed `> N lines changed — enter to expand`
  - Write ops with no prior state (first write, no read baseline): no diff shown
- `internal/ui/heatmap.go`:
  - Files grouped by directory
  - Per file: icon, name, intensity bar (0–100% proportional to HeatScore vs session max), count, op badges (W/R/E)
  - **Agent badge:** Files that were modified by agents show a small `⬡` indicator next to the op badges. Files modified by both parent and agents show both.
  - Top 3 files by HeatScore: bright name color
  - Pin icon on CLAUDE.md files
  - `enter` or `f` opens file history panel for that file
  - File history panel: per-entry in chronological order — prompt index, op type, **agent label if from agent** (e.g., `subagent-1`), delta (+N/-M), mini-diff for Edit ops, excerpt of human text
  - `/` search/filter in heatmap: filters file list by path (consistent with timeline search)
    - Matches against full file path (partial matches work — e.g., `internal/ui/` shows all files in that directory)
    - `enter` dismisses search bar, keeps filter; `esc` clears filter
    - File count in view header updates to show `"N/M files"` when filtered
  - `esc` / back button returns to heatmap tree
  - `h` key activates heatmap view
  - **Live updates:** During a live session, heatmap re-sorts and updates intensity bars as new prompts seal and as `MsgAgentToolCall` events arrive. A file that was cold can become hot mid-session — the user should see this without refreshing.

---

## Acceptance Criteria

- Heatmap correctly identifies most-modified files — including files modified only by agents
- Files modified by agents show `⬡` indicator in heatmap
- File history shows agent-attributed entries with correct agent label
- Intensity bars scale correctly relative to session max HeatScore
- File history shows all operations (parent and agent) in chronological order by timestamp
- Mini-diffs render correctly for Edit ops (both parent and agent)
- Write diffs correctly collapsed/expandable when prior state exists
- Baseline correctly seeded from Read op (parent or agent)
- Agent Write followed by parent Edit: `GetContentAt()` correctly uses agent Write as base (no spurious `WarnReplayGap`)
- `WarnReplayGap` generated on genuinely unresolvable Edit; no crash
- `GetContentAt()` correct at every prompt index for `simple.jsonl` fixture
- All replay unit tests pass
