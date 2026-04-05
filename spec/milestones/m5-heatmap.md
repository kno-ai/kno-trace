# M5: File Intelligence & Heatmap

**Prerequisites:** Read [spec/README.md](../README.md) (core spec) and `SCHEMA.md`. M4 must be complete (agent tree working).

**Goal:** The hot spot detector — which files are getting the most attention, and the complete change history of any file one keypress away.

**Deliverable:** Heatmap reveals session hot spots. File history shows the complete story of any file.

**Use cases served:** UC2 (what happened to this file?), UC6 (which files are getting thrashed?)

**Control room role:** The heatmap is the "hot spot detector" — it surfaces which files are getting the most attention. During a live session, it updates as new prompts seal, so the user can glance at it to see if Claude is focused or thrashing.

---

## Scope

- `internal/replay/engine.go`:
  - Builds `FileHistory` per file after full parse
  - `GetContentAt(path string, promptIdx int) (string, error)` — implements Snapshot Reconstruction Algorithm exactly as specified in the core spec Data Model section
  - `GetFileHistory(path string) *model.FileHistory`
  - `ListFiles() []string` sorted by HeatScore descending
- `internal/replay/diff.go` — `Compute(a, b string) []DiffHunk` via go-diff
- `internal/ui/components/minidiff.go` — renders diff hunks:
  - Max 8 context lines; `... N more lines` beyond that
  - Teal for additions, red for deletions, muted for context
- Inline mini-diffs in timeline detail pane (upgrade to M3 rendering):
  - Edit ops: always show old_str/new_str as mini unified diff inline
  - Write ops where prior state exists: collapsed `> N lines changed — enter to expand`
  - Write ops with no prior state (first write, no read baseline): no diff shown
- `internal/ui/heatmap.go`:
  - Files grouped by directory
  - Per file: icon, name, intensity bar (0–100% proportional to HeatScore vs session max), count, op badges (W/R/E)
  - Top 3 files by HeatScore: bright name color
  - Pin icon on CLAUDE.md files
  - `enter` or `f` opens file history panel for that file
  - File history panel: per-prompt entries in chronological order — prompt index, op type, delta (+N/-M), mini-diff for Edit ops, excerpt of human text
  - `/` search/filter in heatmap: filters file list by path (consistent with timeline search)
    - Matches against full file path (partial matches work — e.g., `internal/ui/` shows all files in that directory)
    - `enter` dismisses search bar, keeps filter; `esc` clears filter
    - File count in view header updates to show `"N/M files"` when filtered
  - `esc` / back button returns to heatmap tree
  - `h` key activates heatmap view
  - **Live updates:** During a live session, heatmap re-sorts and updates intensity bars as new prompts seal. A file that was cold can become hot mid-session — the user should see this without refreshing.

---

## Acceptance Criteria

- Heatmap correctly identifies most-modified files (verify against `--dump` output)
- Intensity bars scale correctly relative to session max HeatScore
- File history shows all prompts that touched a file in chronological order
- Mini-diffs render correctly for Edit ops
- Write diffs correctly collapsed/expandable when prior state exists
- Baseline correctly seeded from Read op
- `WarnReplayGap` generated on unresolvable Edit; no crash
- `GetContentAt()` correct at every prompt index for `simple.jsonl` fixture
- All replay unit tests pass
