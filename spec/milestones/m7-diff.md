# M7: Release Polish & Distribution

**Prerequisites:** M6 complete (unified navigation, replay engine, inline diffs, selection/comparison all working).

**Goal:** Ship the control room. v1.0.0 — `brew install kno-trace` works.

**Deliverable:** Public release with working install instructions and polished experience.

**Use cases served:** All — this milestone polishes the experience.

---

## Scope

- Context pressure:
  - Sparkline in status bar showing context% trajectory across turns
  - Nudge when context exceeds threshold (configurable)
  - Compact markers visible in sparkline
- Polish:
  - Terminal resize propagation at all navigation levels
  - Empty states with clear messages at every level
  - Corrupt JSONL: error message with path, no crash
  - `?` help overlay showing keybindings for the current level
  - `NO_COLOR=1` support: readable without colors
  - Search/filter (`/`) works at every list level
- Distribution:
  - `.goreleaser.yaml`: darwin/amd64, darwin/arm64, linux/amd64, linux/arm64, windows/amd64
  - `.github/workflows/release.yaml`: trigger on `git tag v*`
  - Homebrew tap formula
  - README with hero GIF, install instructions, quick start, keybindings

---

## Acceptance Criteria

- `goreleaser build --snapshot --clean` produces all targets
- `brew install` works on macOS
- Context sparkline renders correctly with ≥3 data points
- Help overlay shows correct keys for current navigation level
- All empty states graceful (no sessions, empty session, turn with no tool calls)
- `NO_COLOR=1` readable output
- Search works at sessions, turns, and items levels
